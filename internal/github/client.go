package github

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

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
	graphql graphQLClient
	rest    restClient
}

func newClient(graphql graphQLClient, rest restClient) *Client {
	return &Client{graphql: graphql, rest: rest}
}

func NewClient() (*Client, error) {
	graphql, err := api.DefaultGraphQLClient()
	if err != nil {
		return nil, err
	}
	rest, err := api.DefaultRESTClient()
	if err != nil {
		return nil, err
	}
	return newClient(graphql, rest), nil
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

// Discover loads the viewer, membership-derived organization owners, and each
// owner's open project list. Owners whose projects are inaccessible are omitted
// from the picker; unexpected owner failures are kept alongside successful results.
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
		discovery.MembershipError = err
		return discovery, nil
	}
	for _, organization := range organizations {
		owner := Owner{Login: organization.Login, Kind: OrganizationOwner}
		projects, err := c.projects(ctx, owner)
		if err != nil {
			if !isSAMLProtectedError(err) {
				discovery.OwnerErrors = append(discovery.OwnerErrors, OwnerFailure{Owner: owner, Err: err})
			}
			continue
		}
		discovery.Owners = append(discovery.Owners, owner)
		discovery.Projects[owner.Login] = projects
	}
	return discovery, nil
}

func isSAMLProtectedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "organization saml enforcement")
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
	return Owner{}, nil, fmt.Errorf("owner %q could not be loaded as an organization (%v) or user (%v)", login, organizationErr, userErr)
}
