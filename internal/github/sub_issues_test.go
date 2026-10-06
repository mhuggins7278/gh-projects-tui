package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSubIssuesReadHierarchyAndOwnProjectValuesWithoutSavedFilter(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "subIssues(first: 20, after: $after)") || !strings.Contains(query, "includeArchived: false") || strings.Contains(query, "$filter") || variables["id"] != "parent-issue" || strings.Contains(query, "fieldValueByName") || variables["after"] != nil {
				t.Fatalf("wrong hierarchy query: %s, %v", query, variables)
			}
			return json.Unmarshal([]byte(`{"node":{"subIssues":{"nodes":[null,
{"__typename":"Issue","id":"child-issue","number":42,"title":"Child","state":"CLOSED","subIssuesSummary":{"total":1,"completed":0},"projectItems":{"nodes":[
{"id":"wrong-project-item","project":{"id":"other"},"field0":null,"field1":null},
{"id":"child-item","project":{"id":"project"},"field0":{"__typename":"ProjectV2ItemFieldMultiSelectValue","field":{"id":"sprint","name":"OTC Sprint"},"value":"Sprint 8, Sprint 9","options":[{"id":"eight","name":"Sprint 8"},{"id":"nine","name":"Sprint 9"}]},"field1":{"__typename":"ProjectV2ItemFieldPullRequestValue","field":{"id":"pulls","name":"Linked pull requests"},"pullRequests":{"nodes":[{"number":43,"repository":{"nameWithOwner":"owner/repo"}}]}}}]}},
{"__typename":"Issue","id":"external-issue","number":44,"title":"Outside project","state":"OPEN","projectItems":{"nodes":[]}}
],"pageInfo":{"hasNextPage":true,"endCursor":"children-next"}}}}`), response)
		},
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "query SubIssueValues") || variables["field0"] != "OTC Sprint" || variables["field1"] != "Linked pull requests" {
				t.Fatalf("wrong hydration query: %s, %v", query, variables)
			}
			ids := variables["ids"].([]string)
			if len(ids) != 1 || ids[0] != "child-item" {
				t.Fatalf("hydrated unrelated projects: %v", ids)
			}
			return json.Unmarshal([]byte(`{"nodes":[{"id":"child-item","field0":{"__typename":"ProjectV2ItemFieldMultiSelectValue","field":{"id":"sprint","name":"OTC Sprint"},"options":[{"id":"eight","name":"Sprint 8"},{"id":"nine","name":"Sprint 9"}]},"field1":{"__typename":"ProjectV2ItemFieldPullRequestValue","field":{"id":"pulls","name":"Linked pull requests"},"pullRequests":{"nodes":[{"number":43,"repository":{"nameWithOwner":"owner/repo"}}]}}}]}`), response)
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != "children-next" {
				t.Fatalf("wrong child cursor: %v", variables)
			}
			return json.Unmarshal([]byte(`{"node":{"subIssues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`), response)
		},
	}}
	client := newClient(graphql, &fakeREST{})
	fields := []Field{{Name: "OTC Sprint"}, {Name: "Linked pull requests"}}
	page, err := client.PageSubIssues(context.Background(), "project", "parent-issue", "", fields)
	if err != nil || len(page.Items) != 2 || !page.HasNext || page.EndCursor != "children-next" {
		t.Fatalf("children = %#v, %v", page, err)
	}
	child := page.Items[0]
	if child.ID != "child-item" || child.OutsideProject || child.Content.ID != "child-issue" || child.Content.State != "CLOSED" || child.Content.SubIssueTotal != 1 || child.FieldValues[0].Value != "Sprint 8, Sprint 9" || child.FieldValues[1].Value != "owner/repo #43" {
		t.Fatalf("incorrect child data: %#v", child)
	}
	if outside := page.Items[1]; !outside.OutsideProject || outside.ID != "external-issue" || len(outside.FieldValues) != 0 {
		t.Fatalf("outside-project issue got project values: %#v", outside)
	}
	page, err = client.PageSubIssues(context.Background(), "project", "parent-issue", page.EndCursor, fields)
	if err != nil || page.HasNext || len(page.Items) != 0 {
		t.Fatalf("last child page = %#v, %v", page, err)
	}
}

func TestSubIssueProjectMembershipPagination(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(_ string, _ map[string]interface{}, response interface{}) error {
			return json.Unmarshal([]byte(`{"node":{"subIssues":{"nodes":[{"__typename":"Issue","id":"child","title":"Child","projectItems":{"nodes":[],"pageInfo":{"hasNextPage":true,"endCursor":"projects-next"}}}]}}}`), response)
		},
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "query SubIssueProjectItems") || variables["id"] != "child" || variables["after"] != "projects-next" || strings.Contains(query, "fieldValueByName") {
				t.Fatalf("wrong membership continuation: %s, %v", query, variables)
			}
			return json.Unmarshal([]byte(`{"node":{"projectItems":{"nodes":[{"id":"item","project":{"id":"project"},"field0":{"__typename":"ProjectV2ItemFieldTextValue","field":{"name":"Note"},"text":"child value"}}]}}}`), response)
		},
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "SubIssueValues") || variables["field0"] != "Note" {
				t.Fatalf("wrong hydration: %s %v", query, variables)
			}
			return json.Unmarshal([]byte(`{"nodes":[{"id":"item","field0":{"__typename":"ProjectV2ItemFieldTextValue","field":{"name":"Note"},"text":"child value"}}]}`), response)
		},
	}}
	page, err := newClient(graphql, &fakeREST{}).PageSubIssues(context.Background(), "project", "parent", "", []Field{{Name: "Note"}})
	if err != nil || len(page.Items) != 1 || page.Items[0].OutsideProject || page.Items[0].ID != "item" || page.Items[0].FieldValues[0].Value != "child value" {
		t.Fatalf("paginated membership = %#v, %v", page, err)
	}
}

func TestSubIssuesDoNotTreatFailuresAsEmptyChildren(t *testing.T) {
	apiErr := errors.New("request failed")
	for _, failure := range []string{"api", "inaccessible", "cursor"} {
		t.Run(failure, func(t *testing.T) {
			graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
				func(_ string, _ map[string]interface{}, response interface{}) error {
					switch failure {
					case "api":
						return apiErr
					case "inaccessible":
						return json.Unmarshal([]byte(`{"node":null}`), response)
					default:
						return json.Unmarshal([]byte(`{"node":{"subIssues":{"nodes":[],"pageInfo":{"hasNextPage":true}}}}`), response)
					}
				},
			}}
			_, err := newClient(graphql, &fakeREST{}).PageSubIssues(context.Background(), "project", "parent", "", nil)
			if err == nil || (failure == "api" && !errors.Is(err, apiErr)) {
				t.Fatalf("failure was lost: %v", err)
			}
		})
	}
}

func TestOutsideProjectIssueDetailLoadsBodyAndComments(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "NestedIssueDetail") || variables["id"] != "issue" {
				t.Fatalf("wrong external detail query: %s, %v", query, variables)
			}
			return json.Unmarshal([]byte(`{"node":{"__typename":"Issue","id":"issue","number":42,"title":"Child outside project","body":"Body","state":"OPEN"}}`), response)
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["issueID"] != "issue" {
				t.Fatalf("wrong issue comments identity: %v", variables)
			}
			return json.Unmarshal([]byte(`{"node":{"comments":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`), response)
		},
	}}
	detail, err := newClient(graphql, &fakeREST{}).LoadIssueDetail(context.Background(), "issue")
	if err != nil || detail.ID != "issue" || detail.Content.Body != "Body" || !detail.Content.BodyAvailable || !detail.CommentsLoaded || len(detail.Fields) != 0 {
		t.Fatalf("external detail = %#v, %v", detail, err)
	}
}

func TestSubIssueValuesAreBatchedOnlyForMatchingProjectItems(t *testing.T) {
	calls := 0
	api := reviewGraphQLFunc(func(_ context.Context, query string, vars map[string]interface{}, response interface{}) error {
		calls++
		if strings.Contains(query, "IssueSubIssues(") {
			if strings.Contains(query, "fieldValueByName") || strings.Contains(query, "fieldValues(") {
				t.Fatal("hydrated unrelated memberships")
			}
			children := []interface{}{}
			for _, id := range []string{"one", "two", "three"} {
				children = append(children, map[string]interface{}{"__typename": "Issue", "id": "issue-" + id, "projectItems": map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"id": "other-" + id, "project": map[string]string{"id": "unrelated"}}, map[string]interface{}{"id": id, "project": map[string]string{"id": "project"}}}}})
			}
			b, _ := json.Marshal(map[string]interface{}{"node": map[string]interface{}{"subIssues": map[string]interface{}{"nodes": children}}})
			return json.Unmarshal(b, response)
		}
		if !strings.Contains(query, "SubIssueValues(") {
			t.Fatal(query)
		}
		ids := vars["ids"].([]string)
		if len(ids) != 3 {
			t.Fatalf("batch=%v", ids)
		}
		nodes := []interface{}{}
		for _, id := range ids {
			nodes = append(nodes, map[string]interface{}{"id": id, "field0": map[string]interface{}{"__typename": "ProjectV2ItemFieldTextValue", "field": map[string]string{"id": "notes"}, "text": id}})
		}
		b, _ := json.Marshal(map[string]interface{}{"nodes": nodes})
		return json.Unmarshal(b, response)
	})
	page, err := newClient(api, &fakeREST{}).PageSubIssues(context.Background(), "project", "parent", "", []Field{{Name: "Notes"}})
	if err != nil || calls != 2 || len(page.Items) != 3 {
		t.Fatalf("batch calls=%d page=%#v err=%v", calls, page, err)
	}
	for _, item := range page.Items {
		if item.FieldValues[0].Value != item.ID {
			t.Fatal(item)
		}
	}
}

func TestSubIssueBatchFailureDoesNotSilentlyLoseProjectValues(t *testing.T) {
	for _, payload := range []string{`{"nodes":[]}`, `{"nodes":[null]}`, `{"nodes":[{"id":"wrong"}]}`} {
		api := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
			func(_ string, _ map[string]interface{}, r interface{}) error {
				return json.Unmarshal([]byte(`{"node":{"subIssues":{"nodes":[{"__typename":"Issue","id":"issue","projectItems":{"nodes":[{"id":"item","project":{"id":"project"}}]}}]}}}`), r)
			},
			func(_ string, _ map[string]interface{}, r interface{}) error {
				return json.Unmarshal([]byte(payload), r)
			},
		}}
		if _, err := newClient(api, &fakeREST{}).PageSubIssues(context.Background(), "project", "parent", "", []Field{{Name: "Notes"}}); err == nil {
			t.Fatal("lost batch treated as empty values")
		}
	}
}
