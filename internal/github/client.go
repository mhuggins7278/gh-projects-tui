package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/cli/go-gh/v2/pkg/api"
)

const viewerProjectsQuery = `
query ViewerProjects($after: String) {
  viewer {
    login
    projectsV2(first: 100, after: $after, orderBy: {field: NUMBER, direction: ASC}) {
      nodes { id number title closed url }
      pageInfo { hasNextPage endCursor }
    }
  }
}`

const ownerProjectsFragment = `
fragment OwnerProjects on ProjectV2Connection {
  nodes { id number title closed url }
  pageInfo { hasNextPage endCursor }
}`

const userProjectsQuery = `
query UserProjects($login: String!, $after: String) {
  user(login: $login) {
    projectsV2(first: 100, after: $after, orderBy: {field: NUMBER, direction: ASC}) { ...OwnerProjects }
  }
}` + ownerProjectsFragment

const organizationProjectsQuery = `
query OrganizationProjects($login: String!, $after: String) {
  organization(login: $login) {
    projectsV2(first: 100, after: $after, orderBy: {field: NUMBER, direction: ASC}) { ...OwnerProjects }
  }
}` + ownerProjectsFragment

type graphQLClient interface {
	DoWithContext(context.Context, string, map[string]interface{}, interface{}) error
}

type restClient interface {
	DoWithContext(context.Context, string, string, io.Reader, interface{}) error
}

// OwnerKind identifies which GraphQL owner branch should be queried.
type OwnerKind string

const (
	UserOwner         OwnerKind = "USER"
	OrganizationOwner OwnerKind = "ORGANIZATION"
)

type Owner struct {
	Login string
	Kind  OwnerKind
}

type Project struct {
	ID     string
	Number int
	Title  string
	Closed bool
	URL    string
}

type OwnerFailure struct {
	Owner Owner
	Err   error
}

type Discovery struct {
	Viewer          string
	Owners          []Owner
	Projects        map[string][]Project
	OwnerErrors     []OwnerFailure
	MembershipError error
}

// Client is the Projects v2 adapter. Its methods intentionally expose no
// GraphQL types so the board and UI layers remain host-independent.
type Client struct {
	graphql     graphQLClient
	rest        restClient
	scheduler   *requestScheduler
	reads       *readCache
	definitions sync.Map
}

func newClient(graphql graphQLClient, rest restClient) *Client {
	return &Client{graphql: graphql, rest: rest}
}

func NewClient() (*Client, error) {
	// Resolve gh authentication and Unix-socket routing before wrapping transport.
	httpClient, err := api.NewHTTPClient(api.ClientOptions{})
	if err != nil {
		return nil, err
	}
	scheduler := newRequestScheduler(httpClient.Transport)
	options := api.ClientOptions{Transport: scheduler}
	graphql, err := api.NewGraphQLClient(options)
	if err != nil {
		return nil, err
	}
	rest, err := api.NewRESTClient(options)
	if err != nil {
		return nil, err
	}
	client := newClient(graphql, rest)
	client.reads = newReadCache(graphql)
	client.graphql = client.reads
	client.scheduler = scheduler
	return client, nil
}

type projectsResponse struct {
	Viewer struct {
		Login    string      `json:"login"`
		Projects projectPage `json:"projectsV2"`
	} `json:"viewer"`
	User         *ownerProjects `json:"user"`
	Organization *ownerProjects `json:"organization"`
}

type ownerProjects struct {
	Projects projectPage `json:"projectsV2"`
}

type projectPage struct {
	Nodes    []*Project `json:"nodes"`
	PageInfo pageInfo   `json:"pageInfo"`
}

type pageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

type organization struct {
	Login string `json:"login"`
}

// Discover loads the viewer, the viewer's open projects, and the
// membership-derived organization owner list. Organization projects are loaded
// lazily via OwnerProjects when an owner is selected, so startup costs one
// viewer query plus membership pagination instead of one query per org.
func (c *Client) Discover(ctx context.Context) (Discovery, error) {
	var response projectsResponse
	if err := c.graphql.DoWithContext(ctx, viewerProjectsQuery, map[string]interface{}{"after": nil}, &response); err != nil {
		return Discovery{}, err
	}

	discovery := Discovery{
		Viewer:   response.Viewer.Login,
		Owners:   []Owner{{Login: response.Viewer.Login, Kind: UserOwner}},
		Projects: map[string][]Project{response.Viewer.Login: response.Viewer.Projects.projects()},
	}
	if response.Viewer.Projects.PageInfo.HasNextPage {
		after, err := nextCursor(response.Viewer.Projects.PageInfo, "")
		if err != nil {
			return Discovery{}, err
		}
		for {
			page, fetchErr := c.fetchViewerPage(ctx, after)
			if fetchErr != nil {
				return Discovery{}, fetchErr
			}
			discovery.Projects[response.Viewer.Login] = append(discovery.Projects[response.Viewer.Login], page.projects()...)
			if !page.PageInfo.HasNextPage {
				break
			}
			after, err = nextCursor(page.PageInfo, after)
			if err != nil {
				return Discovery{}, err
			}
		}
	}

	organizations, err := c.memberships(ctx)
	if err != nil {
		discovery.MembershipError = explainMembershipError(err)
		return discovery, nil
	}
	for _, organization := range organizations {
		owner := Owner{Login: organization.Login, Kind: OrganizationOwner}
		discovery.Owners = append(discovery.Owners, owner)
		if _, ok := discovery.Projects[owner.Login]; !ok {
			discovery.Projects[owner.Login] = nil
		}
	}
	return discovery, nil
}

// OwnerProjects loads one owner's open projects on demand for the project
// picker. SAML-protected owners surface a typed error so the picker can show
// an actionable message while keeping other owners usable.
func (c *Client) OwnerProjects(ctx context.Context, owner Owner) ([]Project, error) {
	projects, err := c.projects(ctx, owner)
	if err != nil {
		return nil, explainOwnerProjectsError(owner, err)
	}
	return projects, nil
}

type actionableAccessError struct {
	message string
	cause   error
}

func (e *actionableAccessError) Error() string { return e.message }
func (e *actionableAccessError) Unwrap() error { return e.cause }

type accessFailureKind uint8

const (
	accessFailureUnknown accessFailureKind = iota
	accessFailureSSO
	accessFailureScope
	accessFailureRateLimit
	accessFailureDenied
)

func classifyAccessFailure(err error) accessFailureKind {
	if err == nil {
		return accessFailureUnknown
	}
	message := strings.ToLower(err.Error())
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		if strings.Contains(strings.ToLower(httpErr.Headers.Get("X-GitHub-SSO")), "required") {
			return accessFailureSSO
		}
		if httpErr.StatusCode == 429 {
			return accessFailureRateLimit
		}
	}
	if strings.Contains(message, "saml") || strings.Contains(message, "sso authorization") || strings.Contains(message, "single sign-on") {
		return accessFailureSSO
	}
	if strings.Contains(message, "rate limit") || strings.Contains(message, "abuse detection") || strings.Contains(message, "secondary rate") {
		return accessFailureRateLimit
	}
	if strings.Contains(message, "insufficient scope") || strings.Contains(message, "scope") ||
		strings.Contains(message, "resource not accessible by integration") {
		return accessFailureScope
	}
	if strings.Contains(message, "forbidden") || strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "not authorized") || strings.Contains(message, "bad credentials") ||
		strings.Contains(message, "authentication required") || strings.Contains(message, "requires authentication") {
		return accessFailureDenied
	}
	if errors.As(err, &httpErr) && (httpErr.StatusCode == 401 || httpErr.StatusCode == 403) {
		return accessFailureDenied
	}
	return accessFailureUnknown
}

func explainMembershipError(err error) error {
	var message string
	switch classifyAccessFailure(err) {
	case accessFailureSSO:
		message = "Organization discovery requires SAML/SSO authorization. Authorize GitHub CLI for your organization, then press r to retry."
	case accessFailureScope:
		message = "Organization discovery requires the read:org token scope. Run `gh auth refresh -s read:org`, then press r to retry."
	case accessFailureRateLimit:
		message = "GitHub rate-limited organization discovery. Wait for the rate limit to reset, then press r to retry."
	case accessFailureDenied:
		message = "GitHub denied organization discovery. Check your organization membership and token access, then press r to retry."
	default:
		message = "Could not discover organization memberships. Check GitHub connectivity and your read:org access, then press r to retry."
	}
	return &actionableAccessError{message: message, cause: err}
}

func explainOwnerProjectsError(owner Owner, err error) error {
	ownerLabel := "owner"
	if owner.Kind == OrganizationOwner {
		ownerLabel = "organization"
	} else if owner.Kind == UserOwner {
		ownerLabel = "user"
	}
	var message string
	switch classifyAccessFailure(err) {
	case accessFailureSSO:
		message = fmt.Sprintf("Projects for %s %q are protected by SAML/SSO. Authorize GitHub CLI for this organization, then press r to retry.", ownerLabel, owner.Login)
	case accessFailureScope:
		message = fmt.Sprintf("GitHub denied project access for %s %q because required token scopes are missing. Run `gh auth refresh -s project -s read:org`, authorize SSO if prompted, then press r to retry.", ownerLabel, owner.Login)
	case accessFailureRateLimit:
		message = fmt.Sprintf("GitHub rate-limited project loading for %s %q. Wait for the rate limit to reset, then press r to retry.", ownerLabel, owner.Login)
	case accessFailureDenied:
		message = fmt.Sprintf("GitHub denied project access for %s %q. Confirm membership and project access, authorize SSO if required, then press r to retry.", ownerLabel, owner.Login)
	default:
		message = fmt.Sprintf("Could not load projects for %s %q. Confirm the owner exists and your account can access its projects, then press r to retry.", ownerLabel, owner.Login)
	}
	return &actionableAccessError{message: message, cause: err}
}

func (c *Client) memberships(ctx context.Context) ([]organization, error) {
	var organizations []organization
	for page := 1; ; page++ {
		var batch []organization
		path := "user/orgs?per_page=100&page=" + strconv.Itoa(page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &batch); err != nil {
			return nil, err
		}
		organizations = append(organizations, batch...)
		if len(batch) < 100 {
			return organizations, nil
		}
	}
}

func (c *Client) projects(ctx context.Context, owner Owner) ([]Project, error) {
	page, err := c.fetchPage(ctx, owner, "")
	if err != nil {
		return nil, err
	}
	projects := page.projects()
	after := ""
	for page.PageInfo.HasNextPage {
		after, err = nextCursor(page.PageInfo, after)
		if err != nil {
			return nil, err
		}
		page, err = c.fetchPage(ctx, owner, after)
		if err != nil {
			return nil, err
		}
		projects = append(projects, page.projects()...)
	}
	return projects, nil
}

func (c *Client) fetchPage(ctx context.Context, owner Owner, after string) (projectPage, error) {
	var response projectsResponse
	var cursor interface{}
	if after != "" {
		cursor = after
	}
	variables := map[string]interface{}{"login": owner.Login, "after": cursor}
	var query string
	switch owner.Kind {
	case UserOwner:
		query = userProjectsQuery
	case OrganizationOwner:
		query = organizationProjectsQuery
	default:
		return projectPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return projectPage{}, err
	}
	switch owner.Kind {
	case UserOwner:
		if response.User == nil {
			return projectPage{}, fmt.Errorf("user %q was not found", owner.Login)
		}
		return response.User.Projects, nil
	case OrganizationOwner:
		if response.Organization == nil {
			return projectPage{}, fmt.Errorf("organization %q was not found", owner.Login)
		}
		return response.Organization.Projects, nil
	}
	return projectPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
}

func (c *Client) fetchViewerPage(ctx context.Context, after string) (projectPage, error) {
	var response projectsResponse
	var cursor interface{}
	if after != "" {
		cursor = after
	}
	if err := c.graphql.DoWithContext(ctx, viewerProjectsQuery, map[string]interface{}{"after": cursor}, &response); err != nil {
		return projectPage{}, err
	}
	return response.Viewer.Projects, nil
}

func (p projectPage) projects() []Project {
	projects := make([]Project, 0, len(p.Nodes))
	for _, project := range p.Nodes {
		if project != nil && !project.Closed {
			projects = append(projects, *project)
		}
	}
	return projects
}

func nextCursor(info pageInfo, previous string) (string, error) {
	if !info.HasNextPage {
		return "", nil
	}
	if info.EndCursor == nil || *info.EndCursor == "" {
		return "", fmt.Errorf("GraphQL connection reported another page without an end cursor")
	}
	if *info.EndCursor == previous {
		return "", fmt.Errorf("GraphQL connection repeated cursor %q", previous)
	}
	return *info.EndCursor, nil
}

// DirectOwner loads one owner without depending on membership discovery.
func (c *Client) DirectOwner(ctx context.Context, owner Owner) ([]Project, error) {
	return c.projects(ctx, owner)
}

// ResolveOwner tries the organization and user namespaces independently so a
// direct owner selection does not depend on membership enumeration.
func (c *Client) ResolveOwner(ctx context.Context, login string) (Owner, []Project, error) {
	organization := Owner{Login: login, Kind: OrganizationOwner}
	projects, organizationErr := c.DirectOwner(ctx, organization)
	if organizationErr == nil {
		return organization, projects, nil
	}

	user := Owner{Login: login, Kind: UserOwner}
	projects, userErr := c.DirectOwner(ctx, user)
	if userErr == nil {
		return user, projects, nil
	}
	preferredOwner, preferredErr := organization, organizationErr
	if classifyAccessFailure(preferredErr) == accessFailureUnknown && classifyAccessFailure(userErr) != accessFailureUnknown {
		preferredOwner, preferredErr = user, userErr
	}
	explained := explainOwnerProjectsError(preferredOwner, preferredErr)
	return Owner{}, nil, &actionableAccessError{
		message: fmt.Sprintf("Could not resolve owner %q. %s", login, explained.Error()),
		cause:   errors.Join(organizationErr, userErr),
	}
}
