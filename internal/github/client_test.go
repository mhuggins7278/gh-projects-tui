package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
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

func TestDiscoverKeepsSuccessfulOwnersWhenOneFails(t *testing.T) {
	viewer := project(1)
	viewerNext := project(2)
	organizationProject := project(3)
	failure := errors.New("SSO authorization required")
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
	rest := &fakeREST{organizations: [][]organization{{{Login: "org-ok"}, {Login: "org-fails"}}}}

	discovery, err := newClient(graphql, rest).Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if discovery.Viewer != "me" || len(discovery.Owners) != 3 {
		t.Fatalf("discovery identity = %#v", discovery)
	}
	if !reflect.DeepEqual(discovery.Projects["me"], []Project{*viewer, *viewerNext}) {
		t.Fatalf("viewer projects = %#v", discovery.Projects["me"])
	}
	if !reflect.DeepEqual(discovery.Projects["org-ok"], []Project{*organizationProject}) {
		t.Fatalf("organization projects = %#v", discovery.Projects["org-ok"])
	}
	if len(discovery.OwnerErrors) != 1 || discovery.OwnerErrors[0].Owner.Login != "org-fails" || !errors.Is(discovery.OwnerErrors[0].Err, failure) {
		t.Fatalf("owner errors = %#v", discovery.OwnerErrors)
	}
	if len(rest.paths) != 1 || rest.paths[0] != "user/orgs?per_page=100&page=1" || rest.methods[0] != "GET" {
		t.Fatalf("membership requests = %#v %#v", rest.methods, rest.paths)
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
			if variables["project"] != 7 || variables["view"] != 3 || !strings.Contains(query, "completedIterations") || !strings.Contains(query, "IssueFieldSingleSelect") {
				t.Fatalf("view query or variables invalid: %q %#v", query, variables)
			}
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{
				ViewerCanUpdate: true,
				View: &rawView{
					ID: "view-id", Number: 3, Name: "Current", Layout: BoardLayout, Filter: "iteration:@current",
					Fields: fieldConnection{Nodes: []*rawField{
						{Kind: "ProjectV2SingleSelectField", ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []FieldOption{{ID: "todo", Name: "Todo"}}},
						{Kind: "ProjectV2SingleSelectField", ID: "priority", Name: "Priority", DataType: "SINGLE_SELECT", IssueField: &rawIssueField{Options: []FieldOption{{ID: "medium", Name: "Medium"}, {ID: "low", Name: "Low"}}}},
					}},
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
	if !view.ViewerCanUpdate || view.Layout != BoardLayout || view.Filter != "iteration:@current" {
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
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != nil {
				t.Fatalf("initial fields cursor = %#v", variables["fieldsAfter"])
			}
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{View: &rawView{
				Fields: fieldConnection{Nodes: []*rawField{{ID: "one"}}, PageInfo: pageInfo{HasNextPage: true, EndCursor: cursor("field-cursor")}},
			}}}
			return nil
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != "field-cursor" {
				t.Fatalf("next fields cursor = %#v", variables["fieldsAfter"])
			}
			result := response.(*viewResponse)
			result.Organization = &viewOwner{Project: &viewProject{View: &rawView{
				Fields: fieldConnection{Nodes: []*rawField{{ID: "two"}}},
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
			if variables["filter"] != "assignee:@me" || variables["after"] != "cursor" || !strings.Contains(query, "ProjectV2ItemFieldDateValue") || !strings.Contains(query, "IssueFieldSingleSelectValue") || !strings.Contains(query, "repository { name }") || !strings.Contains(query, "subIssuesSummary { total completed }") || !strings.Contains(query, "isDraft merged") {
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

	page, err := newClient(graphql, &fakeREST{}).PageItems(context.Background(), Owner{Login: "me", Kind: UserOwner}, 2, "assignee:@me", "cursor")
	if err != nil {
		t.Fatalf("PageItems() error = %v", err)
	}
	if !page.HasNext || page.EndCursor != "next" || len(page.Items) != 1 || len(page.Items[0].FieldValues) != 4 {
		t.Fatalf("page = %#v", page)
	}
	if page.Items[0].Content == nil || page.Items[0].Content.Title != "Fix it" || page.Items[0].Content.Repository != "example" || page.Items[0].Content.State != "OPEN" || page.Items[0].Content.SubIssueTotal != 8 || page.Items[0].Content.SubIssueDone != 3 || page.Items[0].FieldValues[1].Value != "2.5" || page.Items[0].FieldValues[2].FieldName != "Priority" || page.Items[0].FieldValues[2].OptionID != "medium" || page.Items[0].FieldValues[2].Value != "Medium" || !page.Items[0].FieldValues[2].Available {
		t.Fatalf("item = %#v", page.Items[0])
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

func TestLoadItemDetailPaginatesFieldsAndValues(t *testing.T) {
	body := "## Details\n\nA body"
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if variables["fieldsAfter"] != nil || variables["valuesAfter"] != nil || !strings.Contains(query, "body") || !strings.Contains(query, "FieldConfiguration") {
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
	if detail.ID != "item-1" || detail.Content == nil || !detail.Content.BodyAvailable || detail.Content.Body != body {
		t.Fatalf("detail content = %#v", detail)
	}
	if len(detail.Fields) != 3 || detail.Fields[0].Value == nil || detail.Fields[0].Value.Value != "Todo" || detail.Fields[1].Value != nil || detail.Fields[2].Value == nil || detail.Fields[2].Value.Value != "hello" {
		t.Fatalf("detail fields = %#v", detail.Fields)
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
