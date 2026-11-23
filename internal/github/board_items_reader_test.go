package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestBoardReaderRestartsInconsistentParallelReadsWithoutMixingItems(t *testing.T) {
	for _, kind := range []string{"count", "join", "duplicate tail", "duplicate front", "cursor cycle", "short page", "tail failure"} {
		t.Run(kind, func(t *testing.T) {
			var calls, fresh atomic.Int32
			client := boardProbeFixture(660, nil, &calls)
			base := client.graphql
			client.graphql = newReadCache(reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
				if strings.Contains(query, "query FreshProjectItems(") {
					fresh.Add(1)
					if strings.Contains(query, "totalCount") || strings.Contains(query, "items(last:") {
						return fmt.Errorf("fallback reused a counted or backward query")
					}
				}
				back := strings.Contains(query, "items(last:")
				corrupt := strings.Contains(query, "totalCount") && (back || kind == "duplicate front" || kind == "cursor cycle")
				if corrupt && kind == "tail failure" {
					return fmt.Errorf("tail unavailable")
				}
				var data json.RawMessage
				if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
					return err
				}
				if !corrupt {
					return json.Unmarshal(data, response)
				}
				var root map[string]interface{}
				if err := json.Unmarshal(data, &root); err != nil {
					return err
				}
				items := root["organization"].(map[string]interface{})["projectV2"].(map[string]interface{})["items"].(map[string]interface{})
				nodes := items["nodes"].([]interface{})
				switch kind {
				case "count":
					items["totalCount"] = 661
				case "short page":
					items["nodes"] = nodes[:len(nodes)-1]
				case "cursor cycle":
					if !back && variables["after"] == "cursor:199" {
						items["pageInfo"].(map[string]interface{})["endCursor"] = "cursor:99"
					}
				default:
					for _, raw := range nodes {
						node := raw.(map[string]interface{})
						switch {
						case kind == "join" && node["id"] == "item-379":
							node["id"] = "changed-boundary"
						case kind == "duplicate tail" && node["id"] == "item-600":
							node["id"] = "item-599"
						case kind == "duplicate front" && !back && node["id"] == "item-100":
							node["id"] = "item-99"
						}
					}
				}
				data, err := json.Marshal(root)
				if err != nil {
					return err
				}
				return json.Unmarshal(data, response)
			}))
			reader := client.OpenBoardItems(context.Background(), Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
			defer reader.Close()
			items := []Item{}
			resets := 0
			for {
				batch, err := reader.Next()
				if err != nil {
					t.Fatal(err)
				}
				if batch.Reset {
					resets++
					items = nil
				}
				items = append(items, batch.Items...)
				if !batch.HasNext {
					break
				}
			}
			if resets != 1 || fresh.Load() != 7 || len(items) != 660 {
				t.Fatalf("fallback did not replace the partial read exactly once: resets=%d fresh=%d items=%d", resets, fresh.Load(), len(items))
			}
			for index, item := range items {
				if item.ID != fmt.Sprintf("item-%d", index) {
					t.Fatal("fallback mixed parallel and authoritative items")
				}
			}
		})
	}
}

func TestBoardReaderDoesNotRetryFailedAuthoritativeFallback(t *testing.T) {
	failure := errors.New("authoritative read failed")
	var calls, fresh atomic.Int32
	client := boardProbeFixture(660, nil, &calls)
	base := client.graphql
	client.graphql = reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
		if strings.Contains(query, "query FreshProjectItems(") {
			fresh.Add(1)
			return failure
		}
		var data json.RawMessage
		if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
			return err
		}
		// An older host may not return totalCount. The fallback must not need it.
		data = json.RawMessage(strings.Replace(string(data), `"totalCount":660`, `"ignoredCount":660`, 1))
		return json.Unmarshal(data, response)
	})
	reader := client.OpenBoardItems(context.Background(), Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
	defer reader.Close()
	if _, err := reader.Next(); !errors.Is(err, failure) {
		t.Fatalf("fallback failure was hidden: %v", err)
	}
	if _, err := reader.Next(); !errors.Is(err, io.EOF) || fresh.Load() != 1 {
		t.Fatalf("failed fallback was retried: fresh=%d err=%v", fresh.Load(), err)
	}
}

func TestBoardReaderCancellationDoesNotStartFallback(t *testing.T) {
	var calls atomic.Int32
	client := boardProbeFixture(660, nil, &calls)
	ctx, cancel := context.WithCancel(context.Background())
	reader := client.OpenBoardItems(ctx, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
	cancel()
	if _, err := reader.Next(); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("cancelled load performed work: calls=%d err=%v", calls.Load(), err)
	}
	reader.Close()
}

func TestBoardReaderFallbackInvalidatesInconsistentCachedPages(t *testing.T) {
	var calls atomic.Int32
	var corrupt atomic.Bool
	corrupt.Store(true)
	client := boardProbeFixture(660, nil, &calls)
	base := client.graphql
	cache := newReadCache(reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
		var data json.RawMessage
		if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
			return err
		}
		if corrupt.Load() && strings.Contains(query, "items(last:") {
			data = json.RawMessage(strings.Replace(string(data), `"totalCount":660`, `"totalCount":661`, 1))
		}
		return json.Unmarshal(data, response)
	}))
	client.graphql, client.reads = cache, cache
	for round := range 2 {
		reader := client.OpenBoardItems(context.Background(), Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
		resets := 0
		for {
			batch, err := reader.Next()
			if err != nil {
				t.Fatal(err)
			}
			if batch.Reset {
				resets++
			}
			if !batch.HasNext {
				break
			}
		}
		reader.Close()
		want := 1 - round
		if resets != want {
			t.Fatalf("inconsistent pages survived recovery: round=%d resets=%d", round, resets)
		}
		corrupt.Store(false)
	}
}

func TestUserBoardReaderPreservesOwnerBranch(t *testing.T) {
	var calls atomic.Int32
	client := boardProbeFixture(660, nil, &calls)
	base := client.graphql
	client.graphql = reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
		if !strings.Contains(query, "user(login: $login)") || variables["login"] != "example" {
			return fmt.Errorf("reader selected the wrong owner branch")
		}
		var data json.RawMessage
		if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
			return err
		}
		data = json.RawMessage(strings.Replace(string(data), `"organization":`, `"user":`, 1))
		return json.Unmarshal(data, response)
	})
	reader := client.OpenBoardItems(context.Background(), Owner{Login: "example", Kind: UserOwner}, 1, "is:open", nil)
	defer reader.Close()
	count := 0
	for {
		batch, err := reader.Next()
		if err != nil || batch.Reset {
			t.Fatalf("user-owned reader failed or fell back unexpectedly: %v", err)
		}
		count += len(batch.Items)
		if !batch.HasNext {
			break
		}
	}
	if count != 660 || calls.Load() != 7 {
		t.Fatalf("user-owned read lost items or requests: items=%d requests=%d", count, calls.Load())
	}
}
