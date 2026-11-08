package github

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func metadataResponse(t *testing.T, payload, queryPart string, cursor interface{}) func(string, map[string]interface{}, interface{}) error {
	t.Helper()
	return func(query string, variables map[string]interface{}, response interface{}) error {
		if !strings.Contains(query, queryPart) || variables["id"] != "issue" || variables["after"] != cursor {
			t.Fatalf("unexpected metadata query %s, variables %#v", query, variables)
		}
		return json.Unmarshal([]byte(payload), response)
	}
}

func TestIssueMetadataPaginatesAndRetainsCurrentMembers(t *testing.T) {
	g := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		metadataResponse(t, `{"node":{"id":"issue","permission":true,"values":{"nodes":[{"id":"a","name":"Already"}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}`, "values: assignees", nil),
		metadataResponse(t, `{"node":{"id":"issue","permission":true,"values":{"nodes":[{"id":"b","name":"Beta"}],"pageInfo":{"hasNextPage":false}}}}`, "values: assignees", "next"),
		metadataResponse(t, `{"node":{"id":"issue","permission":true,"repository":{"values":{"nodes":[{"id":"b","name":"Beta"},{"id":"c","name":"Charlie"}],"pageInfo":{"hasNextPage":false}}}}}`, "values: assignableUsers", nil),
	}}
	got, err := newClient(g, &fakeREST{}).ReadIssueMetadata(context.Background(), "issue", IssueAssignees, true)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CanEdit || !reflect.DeepEqual(got.Current, []IssueMetadataOption{{"a", "Already"}, {"b", "Beta"}}) || !reflect.DeepEqual(got.Candidates, []IssueMetadataOption{{"a", "Already"}, {"b", "Beta"}, {"c", "Charlie"}}) {
		t.Fatalf("metadata: %#v", got)
	}
}

func TestIssueMetadataRejectsIncompleteConnections(t *testing.T) {
	for _, payload := range []string{
		`{"node":{"id":"issue","permission":true,"values":null}}`,
		`{"node":{"id":"other","permission":true,"values":{"nodes":[],"pageInfo":{}}}}`,
		`{"node":{"id":"issue","permission":true,"values":{"nodes":[],"pageInfo":{"hasNextPage":true}}}}`,
	} {
		g := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{metadataResponse(t, payload, "values: labels", nil)}}
		if _, err := newClient(g, &fakeREST{}).ReadIssueMetadata(context.Background(), "issue", IssueLabels, false); err == nil {
			t.Fatalf("accepted incomplete metadata: %s", payload)
		}
	}
}

func TestIssueMetadataMutationTargetsOnlyRequestedIDs(t *testing.T) {
	for _, tc := range []struct {
		kind                   IssueMetadataKind
		add                    bool
		operation, target, ids string
	}{
		{IssueLabels, true, "addLabelsToLabelable", "labelableId", "labelIds"},
		{IssueLabels, false, "removeLabelsFromLabelable", "labelableId", "labelIds"},
		{IssueAssignees, true, "addAssigneesToAssignable", "assignableId", "assigneeIds"},
		{IssueAssignees, false, "removeAssigneesFromAssignable", "assignableId", "assigneeIds"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			g := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{func(query string, variables map[string]interface{}, response interface{}) error {
				if !strings.Contains(query, tc.operation) || !reflect.DeepEqual(variables["input"], map[string]interface{}{tc.target: "issue", tc.ids: []string{"a", "b"}}) {
					t.Fatalf("query=%s variables=%#v", query, variables)
				}
				return json.Unmarshal([]byte(`{"result":{"target":{"id":"issue"}}}`), response)
			}}}
			if err := newClient(g, &fakeREST{}).ChangeIssueMetadata(context.Background(), "issue", tc.kind, tc.add, []string{"a", "b"}); err != nil {
				t.Fatal(err)
			}
		})
	}
	g := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{func(string, map[string]interface{}, interface{}) error { return errors.New("timeout") }}}
	if err := newClient(g, &fakeREST{}).ChangeIssueMetadata(context.Background(), "issue", IssueLabels, true, []string{"a"}); !IsAmbiguousMutationError(err) {
		t.Fatalf("timeout not ambiguous: %v", err)
	}
}

func TestSearchIssueAssigneesUsesServerQueryAndReturnsOnePage(t *testing.T) {
	g := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if !strings.Contains(query, "assignableUsers(first: 30, query: $query, after: $after)") || variables["id"] != "issue" || variables["query"] != "Ada" || variables["after"] != nil {
				t.Fatalf("query=%s vars=%#v", query, variables)
			}
			return json.Unmarshal([]byte(`{"node":{"id":"issue","viewerCanAssign":true,"repository":{"assignableUsers":{"nodes":[{"id":"u","name":"adalovelace"}],"pageInfo":{"hasNextPage":true,"endCursor":"next"}}}}}`), response)
		},
		func(_ string, variables map[string]interface{}, response interface{}) error {
			if variables["after"] != "next" || variables["query"] != "Ada" {
				t.Fatalf("vars=%#v", variables)
			}
			return json.Unmarshal([]byte(`{"node":{"id":"issue","viewerCanAssign":true,"repository":{"assignableUsers":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`), response)
		},
	}}
	client := newClient(g, &fakeREST{})
	page, err := client.SearchIssueAssignees(context.Background(), "issue", "Ada", "")
	if err != nil || !page.CanEdit || !page.HasNext || page.EndCursor != "next" || len(page.Options) != 1 {
		t.Fatalf("page=%#v err=%v", page, err)
	}
	page, err = client.SearchIssueAssignees(context.Background(), "issue", "Ada", page.EndCursor)
	if err != nil || page.HasNext || len(g.queries) != 2 {
		t.Fatalf("page=%#v err=%v queries=%d", page, err, len(g.queries))
	}
}
