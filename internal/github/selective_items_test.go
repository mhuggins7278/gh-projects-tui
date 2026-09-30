package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIDOnlyProjectReadAvoidsFieldPagination(t *testing.T) {
	for _, lean := range []bool{false, true} {
		calls := 0
		base := reviewGraphQLFunc(func(_ context.Context, q string, _ map[string]interface{}, r interface{}) error {
			calls++
			if strings.Contains(q, "ItemFieldValues") {
				return json.Unmarshal([]byte(`{"node":{"fieldValues":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}`), r)
			}
			nodes := make([]map[string]interface{}, 100)
			for i := range nodes {
				nodes[i] = map[string]interface{}{"id": fmt.Sprintf("item-%d", i)}
				if !strings.Contains(q, "MutationProjectItems") {
					nodes[i]["content"] = map[string]interface{}{"__typename": "Issue", "id": fmt.Sprintf("issue-%d", i), "number": i + 1, "title": "A card", "state": "OPEN"}
					nodes[i]["fieldValues"] = map[string]interface{}{"nodes": []interface{}{}, "pageInfo": map[string]interface{}{"hasNextPage": true, "endCursor": "more-fields"}}
				} else if strings.Contains(q, "fieldValue") || strings.Contains(q, "content") {
					t.Fatal("membership query included unused data")
				}
			}
			payload := map[string]interface{}{"organization": map[string]interface{}{"projectV2": map[string]interface{}{"items": map[string]interface{}{"nodes": nodes, "pageInfo": map[string]interface{}{"hasNextPage": false}}}}}
			b, err := json.Marshal(payload)
			if err != nil {
				return err
			}
			return json.Unmarshal(b, r)
		})
		client := newClient(base, &fakeREST{})
		owner := Owner{Login: "fixture", Kind: OrganizationOwner}
		var page ItemsPage
		var err error
		if lean {
			page, err = client.PageMutationItems(context.Background(), owner, 1, "", nil)
		} else {
			page, err = client.PageItems(context.Background(), owner, 1, "", "")
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 100 {
			t.Fatal("lost IDs")
		}
		for i, item := range page.Items {
			if item.ID != fmt.Sprintf("item-%d", i) {
				t.Fatal("order changed")
			}
		}
		want := 101
		if lean {
			want = 1
		}
		if calls != want {
			t.Fatalf("requests=%d want=%d", calls, want)
		}
		t.Logf("membership lean=%v: %d requests for 100 IDs with paginated field values", lean, calls)
	}
}
