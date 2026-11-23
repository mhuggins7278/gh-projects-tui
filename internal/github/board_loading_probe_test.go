package github

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Loading-strategy experiments stay outside production query construction and
// transport policy. Cursor values always come from GitHub, never from counts.
func boardProbeSizedItems(ctx context.Context, client *Client, owner Owner, project int, filter, after string, fields []Field, size int, backwards bool, count *int) (ItemsPage, error) {
	reader := newClient(boardProbeGraphQL{
		base: client.graphql, omitPermissions: true,
		pageSize: size, backwards: backwards, count: count,
	}, client.rest)
	return reader.PageBoardItems(ctx, owner, project, filter, after, fields)
}

func boardProbeReadCount(ctx context.Context, base graphQLClient, query string, variables map[string]interface{}, response interface{}, count *int) error {
	var data json.RawMessage
	if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
		return err
	}
	var branches map[string]struct {
		Project *struct {
			Items struct{ TotalCount *int }
		} `json:"projectV2"`
	}
	if err := json.Unmarshal(data, &branches); err != nil {
		return err
	}
	for _, branch := range branches {
		if branch.Project != nil && branch.Project.Items.TotalCount != nil {
			*count = *branch.Project.Items.TotalCount
			return json.Unmarshal(data, response)
		}
	}
	return fmt.Errorf("filtered item count was not returned")
}

type boardProbePage struct {
	ItemsPage
	Reset bool
}

// The benchmark now exercises the real production reader, including its safe
// restart contract, rather than maintaining a second pagination implementation.
func boardProbeBidirectional(ctx context.Context, client *Client, owner Owner, project int, filter string, fields []Field) (func(string) (boardProbePage, error), func()) {
	reader := client.OpenBoardItems(ctx, owner, project, filter, fields)
	number := 0
	read := func(string) (boardProbePage, error) {
		batch, err := reader.Next()
		number++
		page := ItemsPage{Items: batch.Items, HasNext: batch.HasNext}
		if batch.HasNext {
			page.EndCursor = fmt.Sprintf("batch:%d", number)
		}
		return boardProbePage{ItemsPage: page, Reset: batch.Reset}, err
	}
	return read, reader.Close
}

func TestBoardProbePageSizesPreserveNestedConnectionsAndCount(t *testing.T) {
	for _, backwards := range []bool{false, true} {
		client := newClient(reviewGraphQLFunc(func(_ context.Context, query string, variables map[string]interface{}, response interface{}) error {
			direction, cursor, aliases := "first", "after", 0
			if backwards {
				direction, cursor, aliases = "last", "before", 1
			}
			if !strings.Contains(query, fmt.Sprintf("items(%s: 50, %s: $after", direction, cursor)) ||
				strings.Count(query, "hasNextPage: hasPreviousPage") != aliases ||
				!strings.Contains(query, "users(first: 10) { nodes { login } pageInfo { hasNextPage endCursor } }") ||
				!strings.Contains(query, "totalCount nodes {") ||
				variables["filter"] != "is:open" || variables["after"] != "cursor" {
				t.Fatal("page-size probe changed nested pagination, filtering, or the wrong cursor")
			}
			return json.Unmarshal([]byte(`{"organization":{"projectV2":{"items":{"totalCount":12,"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`), response)
		}), &fakeREST{})
		count := -1
		_, err := boardProbeSizedItems(context.Background(), client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", "cursor", []Field{{Name: "Assignees", DataType: "ASSIGNEES"}}, 50, backwards, &count)
		if err != nil || count != 12 {
			t.Fatalf("page-size/count probe failed: count=%d err=%v", count, err)
		}
	}
}

// This fake treats cursors as opaque to the reader. Only the fake server knows
// which position each cursor denotes, including before/startCursor semantics.
func boardProbeFixture(total int, observe func(context.Context, bool) error, calls *atomic.Int32) *Client {
	connection := regexp.MustCompile(`items\((first|last): ([0-9]+), (after|before): \$after`)
	return newClient(reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
		calls.Add(1)
		match := connection.FindStringSubmatch(query)
		if len(match) != 4 || variables["filter"] != "is:open" {
			return fmt.Errorf("fixture received an unexpected query")
		}
		backwards := match[1] == "last"
		if backwards && (match[3] != "before" || !strings.Contains(query, "hasNextPage: hasPreviousPage endCursor: startCursor")) {
			return fmt.Errorf("backward probe did not use GitHub's backward cursor fields")
		}
		if observe != nil {
			if err := observe(ctx, backwards); err != nil {
				return err
			}
		}
		size, _ := strconv.Atoi(match[2])
		start, end := 0, total
		if raw := variables["after"]; raw != nil {
			cursor, ok := raw.(string)
			if !ok || !strings.HasPrefix(cursor, "cursor:") {
				return fmt.Errorf("reader sent a fabricated cursor to the fixture")
			}
			index, err := strconv.Atoi(strings.TrimPrefix(cursor, "cursor:"))
			if err != nil {
				return err
			}
			if backwards {
				end = index
			} else {
				start = index + 1
			}
		}
		if backwards {
			start = max(0, end-size)
		} else {
			end = min(total, start+size)
		}
		nodes := []map[string]interface{}{}
		for i := start; i < end; i++ {
			nodes = append(nodes, map[string]interface{}{
				"id":      fmt.Sprintf("item-%d", i),
				"content": map[string]interface{}{"__typename": "Issue", "id": fmt.Sprintf("issue-%d", i), "title": "Synthetic card"},
			})
		}
		hasNext, cursor := end < total, end-1
		if backwards {
			hasNext, cursor = start > 0, start
		}
		page := map[string]interface{}{
			"totalCount": total, "nodes": nodes,
			"pageInfo": map[string]interface{}{"hasNextPage": hasNext, "endCursor": fmt.Sprintf("cursor:%d", cursor)},
		}
		data, err := json.Marshal(map[string]interface{}{"organization": map[string]interface{}{"projectV2": map[string]interface{}{"items": page}}})
		if err != nil {
			return err
		}
		return json.Unmarshal(data, response)
	}), &fakeREST{})
}

func TestBoardProbeBidirectionalPreservesOrderWhilePrefetchingTail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	backStarted, releaseBack := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	client := boardProbeFixture(659, func(ctx context.Context, backwards bool) error {
		if backwards {
			once.Do(func() { close(backStarted) })
			select {
			case <-releaseBack:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}, &calls)
	client.scheduler = newRequestScheduler(nil)
	permit := client.scheduler.permit
	read, cleanup := boardProbeBidirectional(ctx, client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
	defer cleanup()
	page, err := read("")
	if err != nil || len(page.Items) != 100 {
		t.Fatalf("first page failed: %v", err)
	}
	select {
	case <-backStarted:
	case <-ctx.Done():
		t.Fatal("tail was not prefetched")
	}
	items := slices.Clone(page.Items)
	// The next three front pages must remain usable while the tail is blocked.
	for range 3 {
		page, err = read(page.EndCursor)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, page.Items...)
	}
	close(releaseBack)
	for page.HasNext {
		page, err = read(page.EndCursor)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, page.Items...)
	}
	if len(items) != 659 || calls.Load() != 7 {
		t.Fatalf("count or request total changed: items=%d requests=%d", len(items), calls.Load())
	}
	for i, item := range items {
		if item.ID != fmt.Sprintf("item-%d", i) {
			t.Fatal("front/tail merge changed canonical order or duplicated an item")
		}
	}
	cleanup()
	if client.scheduler.permit != permit {
		t.Fatal("test-only read concurrency leaked into later candidates")
	}
}

func TestBoardProbeBidirectionalCleanupCancelsTail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started, stopped := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	client := boardProbeFixture(300, func(ctx context.Context, backwards bool) error {
		if backwards {
			close(started)
			<-ctx.Done()
			close(stopped)
			return ctx.Err()
		}
		return nil
	}, &calls)
	read, cleanup := boardProbeBidirectional(ctx, client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
	defer cleanup()
	if _, err := read(""); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("tail was not started")
	}
	cleanup()
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("cleanup left a tail read in flight")
	}
}

func TestBoardProbeBidirectionalHandlesPartitionEdges(t *testing.T) {
	for _, total := range []int{0, 1, 25, 99, 100, 101, 102, 199, 200, 201, 299, 300, 301, 659} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var calls atomic.Int32
			client := boardProbeFixture(total, nil, &calls)
			read, cleanup := boardProbeBidirectional(ctx, client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
			defer cleanup()
			items, after := []Item{}, ""
			for {
				page, err := read(after)
				if err != nil {
					t.Fatal(err)
				}
				items = append(items, page.Items...)
				if !page.HasNext {
					break
				}
				after = page.EndCursor
			}
			if len(items) != total {
				t.Fatalf("lost partition items: got=%d want=%d", len(items), total)
			}
			for i, item := range items {
				if item.ID != fmt.Sprintf("item-%d", i) {
					t.Fatal("partition edge changed canonical order")
				}
			}
			requests := max(1, (total+99)/100)
			if total >= 300 {
				remaining := total - 100
				front, back := (remaining+1)/2, remaining/2
				requests = 1 + (front+99)/100 + (back+1+99)/100
			}
			if int(calls.Load()) != requests {
				t.Fatalf("unexpected request count: got=%d want=%d", calls.Load(), requests)
			}
		})
	}
}

func TestBoardProbeBidirectionalRejectsShortCountedPartitions(t *testing.T) {
	var calls atomic.Int32
	client := boardProbeFixture(150, nil, &calls)
	base := client.graphql
	client.graphql = reviewGraphQLFunc(func(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
		var data json.RawMessage
		if err := base.DoWithContext(ctx, query, variables, &data); err != nil {
			return err
		}
		// Simulate a count that no longer matches the available items.
		data = json.RawMessage(strings.Replace(string(data), `"totalCount":150`, `"totalCount":500`, 1))
		return json.Unmarshal(data, response)
	})
	read, cleanup := boardProbeBidirectional(context.Background(), client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", nil)
	defer cleanup()
	first, err := read("")
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := read(first.EndCursor)
	if err != nil || !restarted.Reset || len(restarted.Items) != 100 {
		t.Fatalf("short partition did not restart the read: %v", err)
	}
}
