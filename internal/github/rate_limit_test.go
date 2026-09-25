package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(status int, header map[string]string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(`{"data":{}}`)),
		Request:    &http.Request{},
	}
	for key, value := range header {
		response.Header.Set(key, value)
	}
	return response
}

func TestSchedulerPacesWritesAndHonorsCooldown(t *testing.T) {
	var calls atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testResponse(200, map[string]string{"X-RateLimit-Remaining": "100"}), nil
	})
	scheduler := newRequestScheduler(base)
	now := time.Now()
	scheduler.now = func() time.Time { return now }
	scheduler.wait = func(context.Context, time.Duration) error { return nil }

	mutation := `mutation UpdateProjectItemPosition($input: UpdateProjectV2ItemPositionInput!) { updateProjectV2ItemPosition(input: $input) { clientMutationId } }`
	newRequest := func() *http.Request {
		req, _ := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(`{"query":"mutation test"}`))
		_ = mutation
		return req
	}
	// First write sets the next-write deadline; second write must wait.
	if _, err := scheduler.RoundTrip(newRequest()); err != nil {
		t.Fatalf("first write error = %v", err)
	}
	var waited time.Duration
	scheduler.wait = func(_ context.Context, delay time.Duration) error { waited = delay; return nil }
	if _, err := scheduler.RoundTrip(newRequest()); err != nil {
		t.Fatalf("second write error = %v", err)
	}
	if waited <= 0 {
		t.Fatalf("writes were not paced: wait=%v", waited)
	}
	if got := scheduler.snapshot().Requests; got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}
}

func TestSchedulerInstrumentsQueryCostWithoutDroppingVariables(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Query     string                 `json:"query"`
			Variables map[string]interface{} `json:"variables"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Query, "rateLimit { cost remaining resetAt }") {
			t.Fatalf("query missing rateLimit selection: %s", payload.Query)
		}
		if payload.Variables["login"] != "org" {
			t.Fatalf("variables were dropped: %#v", payload.Variables)
		}
		header := http.Header{}
		header.Set("X-RateLimit-Remaining", "4200")
		header.Set("X-RateLimit-Resource", "graphql")
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(`{"data":{"rateLimit":{"cost":17,"remaining":4200,"resetAt":"2026-09-23T16:00:00Z"}}}`)), Request: req}, nil
	})
	scheduler := newRequestScheduler(base)
	scheduler.wait = func(context.Context, time.Duration) error { return nil }
	req, _ := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(`{"query":"query UserProjectViews($login: String!) { user(login: $login) { id } }","variables":{"login":"org"}}`))
	if _, err := scheduler.RoundTrip(req); err != nil {
		t.Fatalf("RoundTrip() error = %v", err)
	}
	status := scheduler.snapshot()
	if status.LastCost != 17 || status.TotalCost != 17 || status.Remaining != 4200 || status.CostByOperation["query UserProjectViews"] != 17 {
		t.Fatalf("query telemetry = %#v", status)
	}
}

func TestSchedulerLedgerGroupsRequestsByFingerprint(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testResponse(200, map[string]string{"X-RateLimit-Remaining": "100"}), nil
	})
	scheduler := newRequestScheduler(base)
	scheduler.wait = func(context.Context, time.Duration) error { return nil }

	graphql := func(query string) *http.Request {
		req, _ := http.NewRequest("POST", "https://api.github.com/graphql", strings.NewReader(`{"query":`+quoteForLedger(query)+`}`))
		return req
	}
	rest := func() *http.Request {
		req, _ := http.NewRequest("GET", "https://api.github.com/user/orgs?per_page=100&page=2", nil)
		return req
	}
	if _, err := scheduler.RoundTrip(graphql(`query UserProjectViews($login: String!) { user(login: $login) { id } }`)); err != nil {
		t.Fatalf("views error = %v", err)
	}
	if _, err := scheduler.RoundTrip(graphql(`query UserProjectViews($login: String!) { user(login: $login) { id } }`)); err != nil {
		t.Fatalf("views error = %v", err)
	}
	if _, err := scheduler.RoundTrip(rest()); err != nil {
		t.Fatalf("rest error = %v", err)
	}
	status := scheduler.snapshot()
	if status.Breakdown["query UserProjectViews"] != 2 {
		t.Fatalf("breakdown = %#v", status.Breakdown)
	}
	if status.Breakdown["GET user/orgs"] != 1 {
		t.Fatalf("breakdown = %#v", status.Breakdown)
	}
	if len(status.Recent) != 3 {
		t.Fatalf("recent = %#v", status.Recent)
	}
}

func quoteForLedger(query string) string {
	escaped := strings.ReplaceAll(query, `"`, `\"`)
	return `"` + escaped + `"`
}

func TestSelectiveBoardQueryOmitsUnrelatedConnections(t *testing.T) {
	graphql := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
		func(query string, variables map[string]interface{}, response interface{}) error {
			if strings.Contains(query, "type content") {
				t.Fatalf("board query has invalid content selection: %q", query)
			}
			if strings.Contains(query, "body") || strings.Contains(query, "fieldValues(first:") {
				t.Fatalf("board query fetched bodies or all field values: %q", query)
			}
			if !strings.Contains(query, "fieldValueByName") {
				t.Fatalf("board query missing selective field lookup: %q", query)
			}
			if strings.Contains(query, "labels(first:") || strings.Contains(query, "reviewers(first:") {
				t.Fatalf("board query should omit nested detail connections: %q", query)
			}
			if variables["field0"] != "Status" {
				t.Fatalf("field name should be a variable, got %#v", variables)
			}
			payload := `{"organization":{"projectV2":{"items":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}}`
			if err := json.Unmarshal([]byte(payload), response); err != nil {
				t.Fatalf("unmarshal selective response: %v", err)
			}
			return nil
		},
	}}
	client := newClient(graphql, &fakeREST{})
	fields := []Field{{Name: "Status"}}
	if _, err := client.PageBoardItems(context.Background(), Owner{Login: "org", Kind: OrganizationOwner}, 1, "", "", fields); err != nil {
		t.Fatalf("PageBoardItems() error = %v", err)
	}
}
