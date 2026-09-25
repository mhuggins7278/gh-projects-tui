package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

type fakeGraphQL struct {
	responses []func(string, map[string]interface{}, interface{}) error
	queries   []string
	variables []map[string]interface{}
}

func (f *fakeGraphQL) DoWithContext(_ context.Context, query string, variables map[string]interface{}, response interface{}) error {
	f.queries = append(f.queries, query)
	f.variables = append(f.variables, variables)
	if len(f.responses) == 0 {
		return errors.New("unexpected GraphQL request")
	}
	responseFn := f.responses[0]
	f.responses = f.responses[1:]
	return responseFn(query, variables, response)
}

type fakeREST struct {
	organizations [][]organization
	err           error
	methods       []string
	paths         []string
}

func (f *fakeREST) DoWithContext(_ context.Context, method, path string, _ io.Reader, response interface{}) error {
	f.methods = append(f.methods, method)
	f.paths = append(f.paths, path)
	if f.err != nil {
		return f.err
	}
	batch := []organization(nil)
	if len(f.organizations) > 0 {
		batch = f.organizations[0]
		f.organizations = f.organizations[1:]
	}
	*(response.(*[]organization)) = append([]organization(nil), batch...)
	return nil
}

func project(number int) *Project {
	return &Project{ID: "project-" + string(rune('0'+number)), Number: number}
}

func closedProject(number int) *Project {
	result := project(number)
	result.Closed = true
	return result
}

func cursor(value string) *string {
	return &value
}

func TestDiscoverListsOwnersWithoutPreloadingProjects(t *testing.T) {
	viewer := project(1)
	viewerNext := project(2)
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != nil {
				t.Fatalf("initial viewer cursor = %#v", variables["after"])
			}
			result := response.(*projectsResponse)
			result.Viewer.Login = "me"
			result.Viewer.Projects = projectPage{Nodes: []*Project{viewer, closedProject(9), nil}, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("viewer-cursor")}}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != "viewer-cursor" {
				t.Fatalf("viewer cursor = %#v", variables["after"])
			}
			result := response.(*projectsResponse)
			result.Viewer.Login = "me"
			result.Viewer.Projects = projectPage{Nodes: []*Project{viewerNext}}
			return nil
		},
	}}
	rest := &fakeREST{organizations: [][]organization{{{Login: "org-ok"}, {Login: "org-fails"}}}}

	client := newClient(graphql, rest)
	discovery, err := client.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if discovery.Viewer != "me" || len(discovery.Owners) != 3 {
		t.Fatalf("discovery identity = %#v", discovery)
	}
	if !reflect.DeepEqual(discovery.Projects["me"], []Project{*viewer, *viewerNext}) {
		t.Fatalf("viewer projects = %#v", discovery.Projects["me"])
	}
	if len(graphql.queries) != 2 {
		t.Fatalf("discover should not preload org projects, queries = %#v", graphql.queries)
	}
}

func TestOwnerProjectsLoadsOnDemand(t *testing.T) {
	organizationProject := project(3)
	failure := errors.New("SSO authorization required")
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["login"] != "org-ok" || variables["after"] != nil {
				t.Fatalf("organization variables = %#v", variables)
			}
			result := response.(*projectsResponse)
			result.Organization = &ownerProjects{Projects: projectPage{Nodes: []*Project{organizationProject, closedProject(8)}}}
			return nil
		},
		func(_ string, variables map[string]interface{}, _ interface{}) error {
			if variables["login"] != "org-fails" {
				t.Fatalf("failing organization login = %v", variables["login"])
			}
			return failure
		},
	}}
	client := newClient(graphql, &fakeREST{})

	projects, err := client.OwnerProjects(context.Background(), Owner{Login: "org-ok", Kind: OrganizationOwner})
	if err != nil {
		t.Fatalf("OwnerProjects() error = %v", err)
	}
	if !reflect.DeepEqual(projects, []Project{*organizationProject}) {
		t.Fatalf("organization projects = %#v", projects)
	}
	if _, err := client.OwnerProjects(context.Background(), Owner{Login: "org-fails", Kind: OrganizationOwner}); !errors.Is(err, failure) {
		t.Fatalf("expected SSO error, got %v", err)
	} else if !strings.Contains(err.Error(), "org-fails") || !strings.Contains(err.Error(), "SAML/SSO") || !strings.Contains(err.Error(), "press r") {
		t.Fatalf("SSO error is not owner-specific/actionable: %v", err)
	}
}

func TestAccessErrorsAreActionableAndDoNotExposeRawDetails(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "SAML header",
			err:  &api.HTTPError{StatusCode: http.StatusForbidden, Headers: http.Header{"X-Github-Sso": []string{"required; url=https://github.com/orgs/acme/sso"}}, Message: "token=ghp_private issue body"},
			want: "SAML/SSO",
		},
		{name: "insufficient scope", err: errors.New("Resource not accessible by integration: token=ghp_private issue body"), want: "gh auth refresh -s read:org"},
		{name: "rate limited", err: errors.New("API rate limit exceeded: token=ghp_private issue body"), want: "rate-limited"},
		{name: "membership unavailable", err: errors.New("network unavailable: token=ghp_private issue body"), want: "Could not discover organization memberships"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := explainMembershipError(test.err)
			if !errors.Is(got, test.err) {
				t.Fatalf("actionable error lost its cause: %v", got)
			}
			if !strings.Contains(got.Error(), test.want) || strings.Contains(got.Error(), "ghp_private") || strings.Contains(got.Error(), "issue body") {
				t.Fatalf("unsafe or unactionable error = %q", got)
			}
		})
	}
}

func TestDiscoverListsSAMLProtectedOrganizationForLazyLoad(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			result := response.(*projectsResponse)
			result.Viewer.Login = "me"
			return nil
		},
	}}
	discovery, err := newClient(graphql, &fakeREST{organizations: [][]organization{{{Login: "glg"}}}}).Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(discovery.Owners) != 2 || discovery.Owners[1].Login != "glg" {
		t.Fatalf("owners = %#v", discovery.Owners)
	}
	if len(discovery.OwnerErrors) != 0 {
		t.Fatalf("discover should defer owner errors to lazy load, got %#v", discovery.OwnerErrors)
	}
	if len(graphql.queries) != 1 {
		t.Fatalf("discover should not preload org projects, queries = %#v", graphql.queries)
	}
}

func TestDiscoverPreservesViewerWhenMembershipFails(t *testing.T) {
	failure := errors.New("membership unavailable")
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			result := response.(*projectsResponse)
			result.Viewer.Login = "me"
			result.Viewer.Projects = projectPage{Nodes: []*Project{project(1)}}
			return nil
		},
	}}
	discovery, err := newClient(graphql, &fakeREST{err: failure}).Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if !errors.Is(discovery.MembershipError, failure) || len(discovery.Projects["me"]) != 1 || len(discovery.Owners) != 1 {
		t.Fatalf("partial discovery = %#v", discovery)
	}
	if !strings.Contains(discovery.MembershipError.Error(), "read:org") || !strings.Contains(discovery.MembershipError.Error(), "press r") {
		t.Fatalf("membership failure lacks retry guidance: %v", discovery.MembershipError)
	}
}

func TestDirectOwnerDoesNotRequireMemberships(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["login"] != "direct-org" || variables["after"] != nil {
				t.Fatalf("variables = %#v", variables)
			}
			result := response.(*projectsResponse)
			result.Organization = &ownerProjects{Projects: projectPage{Nodes: []*Project{project(4)}}}
			return nil
		},
	}}
	rest := &fakeREST{err: errors.New("membership endpoint must not be called")}

	projects, err := newClient(graphql, rest).DirectOwner(context.Background(), Owner{Login: "direct-org", Kind: OrganizationOwner})
	if err != nil {
		t.Fatalf("DirectOwner() error = %v", err)
	}
	if len(projects) != 1 || projects[0].Number != 4 {
		t.Fatalf("projects = %#v", projects)
	}
}

func TestResolveOwnerFallsBackFromOrganizationToUser(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, variables map[string]interface{}, _ interface{}) error {
			if variables["login"] != "person" {
				t.Fatalf("organization login = %#v", variables["login"])
			}
			return errors.New("organization not found")
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["login"] != "person" {
				t.Fatalf("user login = %#v", variables["login"])
			}
			result := response.(*projectsResponse)
			result.User = &ownerProjects{Projects: projectPage{Nodes: []*Project{project(5)}}}
			return nil
		},
	}}

	owner, projects, err := newClient(graphql, &fakeREST{}).ResolveOwner(context.Background(), "person")
	if err != nil {
		t.Fatalf("ResolveOwner() error = %v", err)
	}
	if owner.Kind != UserOwner || len(projects) != 1 || projects[0].Number != 5 {
		t.Fatalf("resolved owner = %#v, projects = %#v", owner, projects)
	}
}

func TestOpenViewProjectsFieldConfigurations(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if variables["project"] != 7 || variables["view"] != 3 || !strings.Contains(query, "projectV2(number: $project) { id viewerCanUpdate") || !strings.Contains(query, "visibleFields(first: 100, after: $fieldsAfter)") || !strings.Contains(query, "completedIterations") || !strings.Contains(query, "IssueFieldSingleSelect") {
				t.Fatalf("view query or variables invalid: %q %#v", query, variables)
			}
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{
				ProjectID:       "project-id",
				ViewerCanUpdate: true,
				View: &rawView{
					ID: "view-id", Number: 3, Name: "Current", Layout: BoardLayout, Filter: "iteration:@current",
					Configuration: viewConfiguration{VisibleFields: fieldConnection{Nodes: []*rawField{
						{Kind: "ProjectV2SingleSelectField", ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []FieldOption{{ID: "todo", Name: "Todo"}}},
						{Kind: "ProjectV2SingleSelectField", ID: "priority", Name: "Priority", DataType: "SINGLE_SELECT", IssueField: &rawIssueField{Options: []FieldOption{{ID: "medium", Name: "Medium"}, {ID: "low", Name: "Low"}}}},
					}}},
					VerticalGroupBy: fieldConnection{Nodes: []*rawField{{Kind: "ProjectV2IterationField", ID: "iteration", Name: "Iteration", DataType: "ITERATION", Configuration: &iterationConfiguration{Iterations: []Iteration{{ID: "current", Title: "Current"}}, CompletedIterations: []Iteration{{ID: "old", Title: "Old"}}}}}},
					SortByFields:    sortConnection{Nodes: []*rawSortField{{Direction: "ASC", Field: &rawField{Kind: "ProjectV2Field", ID: "position", Name: "Position", DataType: "POSITION"}}}},
				},
			}}
			return nil
		},
	}}

	view, err := newClient(graphql, &fakeREST{}).OpenView(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 7, 3)
	if err != nil {
		t.Fatalf("OpenView() error = %v", err)
	}
	if view.ProjectID != "project-id" || !view.ViewerCanUpdate || view.Layout != BoardLayout || view.Filter != "iteration:@current" {
		t.Fatalf("view metadata = %#v", view)
	}
	if len(view.Fields) != 2 || len(view.Fields[0].Options) != 1 || view.Fields[0].Options[0].Name != "Todo" || len(view.Fields[1].Options) != 2 || view.Fields[1].Options[0].Name != "Medium" {
		t.Fatalf("fields = %#v", view.Fields)
	}
	if len(view.VerticalGroupBy) != 1 || len(view.VerticalGroupBy[0].Iterations) != 2 || !view.VerticalGroupBy[0].Iterations[1].Completed {
		t.Fatalf("vertical grouping = %#v", view.VerticalGroupBy)
	}
	if len(view.SortByFields) != 1 || view.SortByFields[0].Field.Name != "Position" {
		t.Fatalf("sorts = %#v", view.SortByFields)
	}
}

func TestOpenViewRejectsMissingView(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{View: nil}}
			return nil
		},
	}}
	_, err := newClient(graphql, &fakeREST{}).OpenView(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 7, 99)
	if err == nil || !strings.Contains(err.Error(), "view 99") {
		t.Fatalf("missing view error = %v", err)
	}
}

func TestOpenViewPaginatesFieldMetadata(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "visibleFields") {
				t.Fatalf("initial view query should request visible fields: %q", query)
			}
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{View: &rawView{
				Configuration: viewConfiguration{VisibleFields: fieldConnection{Nodes: []*rawField{{ID: "one"}}, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("field-cursor")}}},
			}}}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != "field-cursor" {
				t.Fatalf("next fields cursor = %#v", variables["after"])
			}
			result := response.(*viewFieldsPageResponse)
			result.Organization = &viewFieldsPageOwner{Project: &viewFieldsPageProject{View: &viewFieldsPageView{
				Configuration: &viewFieldsPageConfig{VisibleFields: &fieldConnection{Nodes: []*rawField{{ID: "two"}}}},
			}}}
			return nil
		},
	}}

	view, err := newClient(graphql, &fakeREST{}).OpenView(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 1, 1)
	if err != nil {
		t.Fatalf("OpenView() error = %v", err)
	}
	if len(view.Fields) != 2 || view.Fields[0].ID != "one" || view.Fields[1].ID != "two" {
		t.Fatalf("fields = %#v", view.Fields)
	}
}

func TestListViewsPaginates(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != nil || !strings.Contains(query, "views(first:") {
				t.Fatalf("initial views query invalid: %q %#v", query, variables)
			}
			result := response.(*viewsResponse)
			result.Organization = &viewsOwner{Project: &viewsProject{Views: rawViewsPage{
				Nodes:    []*ViewSummary{{Number: 1, Name: "Board", Layout: BoardLayout}, nil},
				PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("views-cursor")},
			}}}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != "views-cursor" {
				t.Fatalf("next views cursor = %#v", variables["after"])
			}
			result := response.(*viewsResponse)
			result.Organization = &viewsOwner{Project: &viewsProject{Views: rawViewsPage{
				Nodes: []*ViewSummary{{Number: 2, Name: "Table", Layout: TableLayout}},
			}}}
			return nil
		},
	}}

	views, err := newClient(graphql, &fakeREST{}).ListViews(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 7)
	if err != nil {
		t.Fatalf("ListViews() error = %v", err)
	}
	if len(views) != 2 || views[0].Number != 1 || views[1].Layout != TableLayout {
		t.Fatalf("views = %#v", views)
	}
}

func TestPageItemsProjectsContentAndValues(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if variables["filter"] != `status:"Done" assignee:@me` || variables["after"] != "cursor" || !strings.Contains(query, "ProjectV2ItemFieldDateValue") || !strings.Contains(query, "IssueFieldSingleSelectValue") || !strings.Contains(query, "users(first: 10) { nodes { login }") || !strings.Contains(query, "repository { name }") || !strings.Contains(query, "subIssuesSummary { total completed }") || !strings.Contains(query, "isDraft merged") {
				t.Fatalf("item query or variables invalid: %q %#v", query, variables)
			}
			result := response.(*itemsResponse)
			result.User = &itemOwner{Project: &struct {
				Items rawItemsPage `json:"items"`
			}{Items: rawItemsPage{
				Nodes: []*rawItem{{
					ID: "item-1", Type: "ISSUE", Content: &rawContent{Kind: "Issue", ID: "issue-1", Number: 42, Title: "Fix it", URL: "https://example.invalid/42", Repository: &rawRepository{Name: "example"}, State: "OPEN", SubIssues: &rawSubIssues{Total: 8, Completed: 3}},
					FieldValues: rawFieldValuePage{Nodes: []*rawFieldValue{
						{Kind: "ProjectV2ItemFieldSingleSelectValue", Field: rawFieldRef{ID: "status", Name: "Status"}, OptionID: "todo", Name: "Todo"},
						{Kind: "ProjectV2ItemFieldNumberValue", Field: rawFieldRef{ID: "estimate", Name: "Estimate"}, Number: floatPtr(2.5)},
						{Kind: "ProjectV2ItemIssueFieldValue", Field: rawFieldRef{ID: "priority", Name: "Priority"}, IssueFieldValue: &rawIssueFieldValue{Kind: "IssueFieldSingleSelectValue", OptionID: "medium", Name: "Medium"}},
						{Kind: "ProjectV2ItemFieldUserValue", Field: rawFieldRef{ID: "assignees", Name: "Assignees"}, Users: &rawUserConnection{Nodes: []rawUser{{Login: "octocat"}, {Login: "hubot"}}}},
					}, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("next")}},
				}},
				PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("next")},
			},
			},
			}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["id"] != "item-1" || variables["after"] != "next" {
				t.Fatalf("field value variables = %#v", variables)
			}
			result := response.(*itemFieldValuesResponse)
			result.Node = &struct {
				FieldValues rawFieldValuePage `json:"fieldValues"`
			}{FieldValues: rawFieldValuePage{Nodes: []*rawFieldValue{{Kind: "ProjectV2ItemFieldTextValue", Field: rawFieldRef{ID: "notes", Name: "Notes"}, Text: "hello"}}}}
			return nil
		},
	}}

	page, err := newClient(graphql, &fakeREST{}).PageItems(context.Background(), Owner{Login: "me", Kind: UserOwner}, 2, `status:"Done" assignee:@me`, "cursor")
	if err != nil {
		t.Fatalf("PageItems() error = %v", err)
	}
	if !page.HasNext || page.EndCursor != "next" || len(page.Items) != 1 || len(page.Items[0].FieldValues) != 5 {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].Content == nil || page.Items[0].Content.Title != "Fix it" || page.Items[0].Content.Repository != "example" || page.Items[0].Content.State != "OPEN" || page.Items[0].Content.SubIssueTotal != 8 || page.Items[0].Content.SubIssueDone != 3 || page.Items[0].FieldValues[1].Value != "2.5" || page.Items[0].FieldValues[2].FieldName != "Priority" || page.Items[0].FieldValues[2].OptionID != "medium" || page.Items[0].FieldValues[2].Value != "Medium" || !page.Items[0].FieldValues[2].Available {
		t.Fatalf("item = %#v", page.Items[0])
	}
	if got := page.Items[0].FieldValues[3]; got.FieldName != "Assignees" || got.Value != "octocat, hubot" || !got.Available {
		t.Fatalf("assignees = %#v", got)
	}
}

func TestPageItemsProjectsPullRequestMetadata(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			result := response.(*itemsResponse)
			result.Organization = &itemOwner{Project: &struct {
				Items rawItemsPage `json:"items"`
			}{Items: rawItemsPage{Nodes: []*rawItem{
				{ID: "pr-1", Type: "PULL_REQUEST", Content: &rawContent{Kind: "PullRequest", ID: "pull-1", Number: 7, Title: "Merge it", Repository: &rawRepository{Name: "example"}, State: "MERGED", Merged: true}},
				{ID: "pr-2", Type: "PULL_REQUEST", Content: &rawContent{Kind: "PullRequest", ID: "pull-2", Number: 8, Title: "Work in progress", Repository: &rawRepository{Name: "example"}, State: "OPEN", IsDraft: true}},
			}}}}
			return nil
		},
	}}

	page, err := newClient(graphql, &fakeREST{}).PageItems(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 2, "", "")
	if err != nil {
		t.Fatalf("PageItems() error = %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("items = %#v", page.Items)
	}
	merged := page.Items[0].Content
	draft := page.Items[1].Content
	if merged.Repository != "example" || merged.State != "MERGED" || !merged.Merged {
		t.Fatalf("merged pull request = %#v", merged)
	}
	if draft.Repository != "example" || draft.State != "OPEN" || !draft.IsDraft {
		t.Fatalf("draft pull request = %#v", draft)
	}
}

func TestNestedDetailFieldValuesProjectToDisplayStrings(t *testing.T) {
	cases := []struct {
		name      string
		value     rawFieldValue
		want      string
		available bool
	}{
		{
			name: "labels",
			value: rawFieldValue{
				Kind:   "ProjectV2ItemFieldLabelValue",
				Labels: &rawLabelConnection{Nodes: []rawLabel{{Name: "bug"}, {Name: "urgent"}}},
			},
			want:      "bug, urgent",
			available: true,
		},
		{
			name:      "milestone",
			value:     rawFieldValue{Kind: "ProjectV2ItemFieldMilestoneValue", Milestone: &rawMilestone{Title: "v1.0"}},
			want:      "v1.0",
			available: true,
		},
		{
			name: "pull requests",
			value: rawFieldValue{
				Kind:         "ProjectV2ItemFieldPullRequestValue",
				PullRequests: &rawPullRequestConnection{Nodes: []rawPullRequest{{Number: 12, Repository: &rawRepository{NameWithOwner: "org/repo"}}}},
			},
			want:      "org/repo #12",
			available: true,
		},
		{
			name:      "repository",
			value:     rawFieldValue{Kind: "ProjectV2ItemFieldRepositoryValue", Repository: &rawRepository{NameWithOwner: "org/repo"}},
			want:      "org/repo",
			available: true,
		},
		{
			name: "reviewers",
			value: rawFieldValue{
				Kind:      "ProjectV2ItemFieldReviewerValue",
				Reviewers: &rawRequestedReviewerConnection{Nodes: []rawRequestedReviewer{{Login: "octocat"}, {Name: "Platform"}}},
			},
			want:      "octocat, Platform",
			available: true,
		},
		{
			name:      "users",
			value:     rawFieldValue{Kind: "ProjectV2ItemFieldUserValue", Users: &rawUserConnection{Nodes: []rawUser{{Login: "octocat"}}}},
			want:      "octocat",
			available: true,
		},
		{
			name:      "inaccessible labels",
			value:     rawFieldValue{Kind: "ProjectV2ItemFieldLabelValue"},
			available: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projected := tc.value.project()
			if projected.Value != tc.want || projected.Available != tc.available {
				t.Fatalf("projected = %#v, want value %q available %v", projected, tc.want, tc.available)
			}
		})
	}
}

func TestLoadItemDetailPaginatesFieldsAndValues(t *testing.T) {
	body := "## Details\n\nA body"
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != nil || variables["valuesAfter"] != nil || !strings.Contains(query, "body") || !strings.Contains(query, "FieldConfiguration") || !strings.Contains(query, "labels(first: 100)") || !strings.Contains(query, "pullRequests(first: 100)") {
				t.Fatalf("detail query = %q, variables = %#v", query, variables)
			}
			result := response.(*itemDetailResponse)
			result.Organization = &detailOwner{Project: &detailProject{Fields: fieldConnection{
				Nodes:    []*rawField{{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", Options: []FieldOption{{ID: "todo", Name: "Todo"}}}},
				PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("fields-next")},
			}}}
			result.Node = &rawDetailItem{
				ID:      "item-1",
				Content: &rawContent{Kind: "Issue", Number: 42, Title: "Fix it", Body: &body},
				FieldValues: rawFieldValuePage{
					Nodes:    []*rawFieldValue{{Kind: "ProjectV2ItemFieldSingleSelectValue", Field: rawFieldRef{ID: "status", Name: "Status"}, OptionID: "todo", Name: "Todo"}},
					PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("values-next")},
				},
			}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != "fields-next" || variables["valuesAfter"] != nil {
				t.Fatalf("field detail cursor = %#v", variables)
			}
			result := response.(*itemDetailResponse)
			result.Organization = &detailOwner{Project: &detailProject{Fields: fieldConnection{
				Nodes: []*rawField{{ID: "priority", Name: "Priority", Kind: "ProjectV2Field", DataType: "TEXT"}},
			}}}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != nil || variables["valuesAfter"] != "values-next" {
				t.Fatalf("value detail cursor = %#v", variables)
			}
			result := response.(*itemDetailResponse)
			result.Organization = &detailOwner{Project: &detailProject{Fields: fieldConnection{}}}
			result.Node = &rawDetailItem{FieldValues: rawFieldValuePage{Nodes: []*rawFieldValue{{Kind: "ProjectV2ItemFieldTextValue", Field: rawFieldRef{ID: "notes", Name: "Notes"}, Text: "hello"}}}}
			return nil
		},
	}}

	detail, err := newClient(graphql, &fakeREST{}).LoadItemDetail(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 7, "item-1")
	if err != nil {
		t.Fatalf("LoadItemDetail() error = %v", err)
	}
	if detail.ID != "item-1" || detail.Content == nil || detail.Content.Number != 42 || !detail.Content.BodyAvailable || detail.Content.Body != body {
		t.Fatalf("detail content = %#v", detail)
	}
	if len(detail.Fields) != 3 || detail.Fields[0].Value == nil || detail.Fields[0].Value.Value != "Todo" || detail.Fields[1].Value != nil || detail.Fields[2].Value == nil || detail.Fields[2].Value.Value != "hello" {
		t.Fatalf("detail fields = %#v", detail.Fields)
	}
}

func TestLoadItemDetailPaginatesNestedMultiValueConnections(t *testing.T) {
	labels := make([]rawLabel, 100)
	users := make([]rawUser, 100)
	reviewers := make([]rawRequestedReviewer, 100)
	pullRequests := make([]rawPullRequest, 100)
	for index := range labels {
		labels[index] = rawLabel{Name: fmt.Sprintf("label-%03d", index)}
		users[index] = rawUser{Login: fmt.Sprintf("user-%03d", index)}
		reviewers[index] = rawRequestedReviewer{Login: fmt.Sprintf("reviewer-%03d", index)}
		pullRequests[index] = rawPullRequest{Number: index + 1}
	}
	type nestedCase struct {
		name   string
		kind   string
		cursor string
		first  *rawFieldValue
		second *rawFieldValue
		want   string
	}
	cases := []nestedCase{
		{
			name: "Labels", kind: "ProjectV2ItemFieldLabelValue", cursor: "labels-next",
			first:  &rawFieldValue{Kind: "ProjectV2ItemFieldLabelValue", Labels: &rawLabelConnection{Nodes: labels, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("labels-next")}}},
			second: &rawFieldValue{Kind: "ProjectV2ItemFieldLabelValue", Labels: &rawLabelConnection{Nodes: []rawLabel{{Name: "label-last"}}}}, want: "label-last",
		},
		{
			name: "Assignees", kind: "ProjectV2ItemFieldUserValue", cursor: "users-next",
			first:  &rawFieldValue{Kind: "ProjectV2ItemFieldUserValue", Users: &rawUserConnection{Nodes: users, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("users-next")}}},
			second: &rawFieldValue{Kind: "ProjectV2ItemFieldUserValue", Users: &rawUserConnection{Nodes: []rawUser{{Login: "user-last"}}}}, want: "user-last",
		},
		{
			name: "Reviewers", kind: "ProjectV2ItemFieldReviewerValue", cursor: "reviewers-next",
			first:  &rawFieldValue{Kind: "ProjectV2ItemFieldReviewerValue", Reviewers: &rawRequestedReviewerConnection{Nodes: reviewers, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("reviewers-next")}}},
			second: &rawFieldValue{Kind: "ProjectV2ItemFieldReviewerValue", Reviewers: &rawRequestedReviewerConnection{Nodes: []rawRequestedReviewer{{Name: "reviewer-last"}}}}, want: "reviewer-last",
		},
		{
			name: "Linked pull requests", kind: "ProjectV2ItemFieldPullRequestValue", cursor: "pulls-next",
			first:  &rawFieldValue{Kind: "ProjectV2ItemFieldPullRequestValue", PullRequests: &rawPullRequestConnection{Nodes: pullRequests, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("pulls-next")}}},
			second: &rawFieldValue{Kind: "ProjectV2ItemFieldPullRequestValue", PullRequests: &rawPullRequestConnection{Nodes: []rawPullRequest{{Number: 101}}}}, want: "#101",
		},
	}
	responses := []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			result := response.(*itemDetailResponse)
			values := make([]*rawFieldValue, 0, len(cases))
			for _, tc := range cases {
				value := *tc.first
				value.Field = rawFieldRef{ID: tc.name, Name: tc.name}
				values = append(values, &value)
			}
			result.Organization = &detailOwner{Project: &detailProject{}}
			result.Node = &rawDetailItem{ID: "item", FieldValues: rawFieldValuePage{Nodes: values}}
			return nil
		},
	}
	for _, tc := range cases {
		current := tc
		responses = append(responses, func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "fieldValueByName(name: $fieldName)") || variables["fieldName"] != current.name {
				t.Fatalf("nested query for %q: query=%s vars=%#v", current.name, query, variables)
			}
			var afterKey string
			switch current.kind {
			case "ProjectV2ItemFieldLabelValue":
				afterKey = "labelsAfter"
			case "ProjectV2ItemFieldUserValue":
				afterKey = "usersAfter"
			case "ProjectV2ItemFieldReviewerValue":
				afterKey = "reviewersAfter"
			case "ProjectV2ItemFieldPullRequestValue":
				afterKey = "pullRequestsAfter"
			}
			if variables[afterKey] != current.cursor {
				t.Fatalf("nested cursor for %q = %#v", current.name, variables[afterKey])
			}
			result := response.(*nestedFieldValueResponse)
			result.Node = &struct {
				Value *rawFieldValue `json:"fieldValueByName"`
			}{Value: current.second}
			return nil
		})
	}
	detail, err := newClient(&fakeGraphQL{responses: responses}, &fakeREST{}).LoadItemDetail(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 1, "item")
	if err != nil {
		t.Fatalf("LoadItemDetail() error = %v", err)
	}
	if len(detail.Fields) != len(cases) {
		t.Fatalf("detail fields = %#v", detail.Fields)
	}
	for index, tc := range cases {
		if detail.Fields[index].Value == nil || !strings.HasSuffix(detail.Fields[index].Value.Value, tc.want) || strings.Count(detail.Fields[index].Value.Value, ",") != 100 {
			t.Fatalf("%s = %#v, want %q", tc.name, detail.Fields[index].Value, tc.want)
		}
	}
}

func TestJSONFixtureSkipsNullNodes(t *testing.T) {
	var response projectsResponse
	if err := json.Unmarshal([]byte(`{"viewer":{"login":"me","projectsV2":{"nodes":[null,{"id":"p1","number":1,"closed":false}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`), &response); err != nil {
		t.Fatal(err)
	}
	projects := response.Viewer.Projects.projects()
	if len(projects) != 1 || projects[0].Number != 1 {
		t.Fatalf("projects = %#v", projects)
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
