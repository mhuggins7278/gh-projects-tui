package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
)

func allowIssuePermission(t *testing.T, permission string) func(string, map[string]interface{}, interface{}) error {
	t.Helper()
	return func(query string, variables map[string]interface{}, result interface{}) error {
		t.Helper()
		if !strings.HasPrefix(strings.TrimSpace(query), "query ") || !strings.Contains(query, "allowed: "+permission) || variables["id"] != "issue" {
			t.Fatal("missing issue permission preflight")
		}
		return json.Unmarshal([]byte(`{"node":{"id":"issue","allowed":true}}`), result)
	}
}

func TestBoardReadsDeferIssuePermissions(t *testing.T) {
	for _, owner := range []Owner{{Login: "example", Kind: OrganizationOwner}, {Login: "example", Kind: UserOwner}} {
		calls := 0
		client := newClient(reviewGraphQLFunc(func(_ context.Context, query string, variables map[string]interface{}, result interface{}) error {
			calls++
			for _, permission := range []string{"viewerCanSetFields", "viewerCanClose", "viewerCanReopen"} {
				if strings.Contains(query, permission) {
					t.Fatalf("board read eagerly fetched %s", permission)
				}
			}
			if !strings.Contains(query, "subIssuesSummary { total completed }") ||
				!strings.Contains(query, "repository { name nameWithOwner }") ||
				!strings.Contains(query, "fieldValueByName") ||
				variables["filter"] != "is:open" || variables["after"] != "cursor" {
				t.Fatal("board read lost card data, saved filter, or pagination")
			}
			branch := "organization"
			if owner.Kind == UserOwner {
				branch = "user"
			}
			return json.Unmarshal([]byte(`{"`+branch+`":{"projectV2":{"items":{"nodes":[{"id":"item","field0":null,"content":{"__typename":"Issue","id":"issue","title":"Card","state":"OPEN","subIssuesSummary":{"total":3,"completed":1}}}],"pageInfo":{"hasNextPage":false}}}}}`), result)
		}), &fakeREST{})
		page, err := client.PageBoardItems(context.Background(), owner, 1, "is:open", "cursor", []Field{{Name: "Status", DataType: "SINGLE_SELECT"}})
		if err != nil || calls != 1 || len(page.Items) != 1 || page.Items[0].Content.SubIssueTotal != 3 {
			t.Fatalf("board read failed: calls=%d error=%v", calls, err)
		}
		if boardProbePermissions(page.Items) != 0 {
			t.Fatal("board read invented permission values")
		}
	}
}

func TestSetIssueClosedChecksOnlyRequestedPermission(t *testing.T) {
	for _, closed := range []bool{true, false} {
		permission, mutation, payload := "viewerCanClose", "mutation CloseIssue", `{"closeIssue":{"issue":{"id":"issue","state":"CLOSED"}}}`
		if !closed {
			permission, mutation, payload = "viewerCanReopen", "mutation ReopenIssue", `{"reopenIssue":{"issue":{"id":"issue","state":"OPEN"}}}`
		}
		calls := 0
		client := newClient(newReadCache(reviewGraphQLFunc(func(_ context.Context, query string, variables map[string]interface{}, result interface{}) error {
			calls++
			if calls%2 == 1 {
				if !strings.Contains(query, "allowed: "+permission) || strings.Contains(query, "body") || strings.Contains(query, "projectV2") || strings.Count(query, "viewerCan") != 1 || variables["id"] != "issue" {
					t.Fatalf("unexpected permission preflight: %s", query)
				}
				return json.Unmarshal([]byte(`{"node":{"id":"issue","allowed":true}}`), result)
			}
			if !strings.Contains(query, mutation) || variables["input"].(map[string]interface{})["issueId"] != "issue" {
				t.Fatal("mutation changed after permission preflight")
			}
			return json.Unmarshal([]byte(payload), result)
		})), &fakeREST{})
		for range 2 {
			if err := client.SetIssueClosed(context.Background(), "issue", closed); err != nil {
				t.Fatal(err)
			}
		}
		if calls != 4 {
			t.Fatalf("each action needs a fresh permission read and one write: calls=%d", calls)
		}
	}
}

func TestPermissionPreflightFailureNeverSubmitsMutation(t *testing.T) {
	for _, action := range []struct {
		name       string
		permission string
		run        func(*Client) error
	}{
		{"close", "viewerCanClose", func(c *Client) error { return c.SetIssueClosed(context.Background(), "issue", true) }},
		{"reopen", "viewerCanReopen", func(c *Client) error { return c.SetIssueClosed(context.Background(), "issue", false) }},
		{"set field", "viewerCanSetFields", func(c *Client) error {
			text := "new"
			return c.UpdateItemFieldValue(context.Background(), ItemFieldValueUpdate{IssueID: "issue", IssueFieldID: "field", Value: FieldValueInput{Text: &text}})
		}},
		{"clear field", "viewerCanSetFields", func(c *Client) error {
			return c.ClearItemFieldValue(context.Background(), ItemFieldValueClear{IssueID: "issue", IssueFieldID: "field"})
		}},
	} {
		for _, failure := range []struct {
			name, payload string
			err           error
		}{
			{name: "denied", payload: `{"node":{"id":"issue","allowed":false}}`},
			{name: "unknown", payload: `{"node":{"id":"issue"}}`},
			{name: "missing", payload: `{"node":null}`},
			{name: "non-issue", payload: `{"node":{}}`},
			{name: "wrong identity", payload: `{"node":{"id":"other","allowed":true}}`},
			{name: "timeout", err: context.DeadlineExceeded},
			{name: "cancelled", err: context.Canceled},
			{name: "server error", err: &api.HTTPError{StatusCode: 502}},
		} {
			t.Run(action.name+"/"+failure.name, func(t *testing.T) {
				calls := 0
				client := newClient(reviewGraphQLFunc(func(_ context.Context, query string, variables map[string]interface{}, result interface{}) error {
					calls++
					if !strings.HasPrefix(strings.TrimSpace(query), "query ") || !strings.Contains(query, "allowed: "+action.permission) || variables["id"] != "issue" {
						t.Fatal("write reached GitHub without an affirmative permission preflight")
					}
					if failure.err != nil {
						return failure.err
					}
					return json.Unmarshal([]byte(failure.payload), result)
				}), &fakeREST{})
				err := action.run(client)
				if err == nil || IsAmbiguousMutationError(err) || calls != 1 || failure.err != nil && !errors.Is(err, failure.err) {
					t.Fatalf("preflight failure must be definitive, preserve its cause, and stop the write: calls=%d err=%v", calls, err)
				}
			})
		}
	}
}

func TestDeferredPermissionPreservesSubmittedWriteErrorClassification(t *testing.T) {
	for _, writeErr := range []error{context.DeadlineExceeded, &api.GraphQLError{Errors: []api.GraphQLErrorItem{{Message: "rejected"}}}} {
		calls := 0
		client := newClient(reviewGraphQLFunc(func(_ context.Context, query string, _ map[string]interface{}, result interface{}) error {
			calls++
			if calls == 1 {
				if strings.HasPrefix(strings.TrimSpace(query), "mutation ") {
					t.Fatal("write bypassed permission preflight")
				}
				return json.Unmarshal([]byte(`{"node":{"id":"issue","allowed":true}}`), result)
			}
			return writeErr
		}), &fakeREST{})
		err := client.SetIssueClosed(context.Background(), "issue", true)
		if !errors.Is(err, writeErr) || IsAmbiguousMutationError(err) != IsAmbiguousMutationError(classifyMutationError(writeErr)) || calls != 2 {
			t.Fatalf("submitted mutation classification changed: calls=%d err=%v", calls, err)
		}
	}
}

func TestPermissionPreflightDoesNotCacheDenials(t *testing.T) {
	calls := 0
	client := newClient(newReadCache(reviewGraphQLFunc(func(_ context.Context, query string, _ map[string]interface{}, result interface{}) error {
		calls++
		if strings.HasPrefix(strings.TrimSpace(query), "mutation ") {
			return json.Unmarshal([]byte(`{"closeIssue":{"issue":{"id":"issue","state":"CLOSED"}}}`), result)
		}
		allowed := "false"
		if calls > 1 {
			allowed = "true"
		}
		return json.Unmarshal([]byte(`{"node":{"id":"issue","allowed":`+allowed+`}}`), result)
	})), &fakeREST{})
	if err := client.SetIssueClosed(context.Background(), "issue", true); err == nil || IsAmbiguousMutationError(err) {
		t.Fatal("denied action was not rejected before writing")
	}
	if err := client.SetIssueClosed(context.Background(), "issue", true); err != nil || calls != 3 {
		t.Fatalf("permission was not reread after a denial: calls=%d err=%v", calls, err)
	}
}

func TestSelectedItemFieldsStillLoadIssuePermissions(t *testing.T) {
	client := newClient(reviewGraphQLFunc(func(_ context.Context, query string, _ map[string]interface{}, result interface{}) error {
		for _, permission := range []string{"viewerCanSetFields", "viewerCanClose", "viewerCanReopen"} {
			if !strings.Contains(query, permission) {
				t.Fatalf("selected-item read omitted %s", permission)
			}
		}
		return json.Unmarshal([]byte(`{"organization":{"projectV2":{"fields":{"nodes":[],"pageInfo":{"hasNextPage":false}}}},"node":{"id":"item","content":{"__typename":"Issue","id":"issue","viewerCanSetFields":false,"viewerCanClose":false,"viewerCanReopen":false},"fieldValues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`), result)
	}), &fakeREST{})
	detail, err := client.LoadItemFields(context.Background(), Owner{Login: "example", Kind: OrganizationOwner}, 1, "item")
	if err != nil || detail.Content == nil {
		t.Fatalf("selected item fields failed: %v", err)
	}
	for _, permission := range []*bool{detail.Content.ViewerCanSetFields, detail.Content.ViewerCanClose, detail.Content.ViewerCanReopen} {
		if permission == nil || *permission {
			t.Fatal("selected-item read lost a permission denial")
		}
	}
}
