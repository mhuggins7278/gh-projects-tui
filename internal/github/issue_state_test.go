package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReadIssueStateIsSmallAndUncached(t *testing.T) {
	calls := 0
	base := reviewGraphQLFunc(func(ctx context.Context, q string, v map[string]interface{}, r interface{}) error {
		calls++
		if v["id"] != "issue" || !strings.Contains(q, "... on Issue { id state }") || strings.Contains(q, "comments") || strings.Contains(q, "body") || strings.Contains(q, "projectV2") {
			t.Fatalf("unexpected query: %s variables:%v", q, v)
		}
		state := "OPEN"
		if calls > 1 {
			state = "CLOSED"
		}
		return json.Unmarshal([]byte(`{"node":{"id":"issue","state":"`+state+`"}}`), r)
	})
	client := newClient(newReadCache(base), &fakeREST{})
	for _, want := range []string{"OPEN", "CLOSED"} {
		got, err := client.ReadIssueState(context.Background(), "issue")
		if err != nil || got.ID != "issue" || got.State != want {
			t.Fatalf("state=%v error=%v", got, err)
		}
	}
	if calls != 2 {
		t.Fatalf("authoritative reads were cached: calls=%d", calls)
	}
}

func TestReadIssueStateRejectsUnavailableResults(t *testing.T) {
	for _, payload := range []string{`{"node":null}`, `{"node":{}}`, `{"node":{"id":"other","state":"CLOSED"}}`, `{"node":{"id":"issue","state":"UNKNOWN"}}`} {
		client := newClient(reviewGraphQLFunc(func(_ context.Context, _ string, _ map[string]interface{}, r interface{}) error {
			return json.Unmarshal([]byte(payload), r)
		}), &fakeREST{})
		if _, err := client.ReadIssueState(context.Background(), "issue"); err == nil {
			t.Fatalf("unavailable issue accepted: %s", payload)
		}
	}
}

func TestReadIssueStatePreservesCancellationAndValidatesID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	client := newClient(reviewGraphQLFunc(func(ctx context.Context, _ string, _ map[string]interface{}, _ interface{}) error {
		calls++
		return ctx.Err()
	}), &fakeREST{})
	if _, err := client.ReadIssueState(ctx, "issue"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if _, err := client.ReadIssueState(context.Background(), " "); err == nil || calls != 1 {
		t.Fatalf("empty ID made request: calls=%d error=%v", calls, err)
	}
}
