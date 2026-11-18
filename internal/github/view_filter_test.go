package github

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSaveViewFilterChecksConflictAndReconcilesTimeout(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		current := "is:open"
		if conflict {
			current = "assignee:@me"
		}
		writes := 0
		client := newClient(reviewGraphQLFunc(func(_ context.Context, q string, v map[string]interface{}, r interface{}) error {
			if strings.HasPrefix(q, "mutation") {
				writes++
				input := v["input"].(map[string]interface{})
				if len(input) != 2 || input["viewId"] != "view" || input["filter"] != "" {
					t.Fatal(input)
				}
				current = ""
				return errors.New("timeout")
			}
			data, _ := json.Marshal(map[string]interface{}{"node": map[string]string{"id": "view", "filter": current}})
			return json.Unmarshal(data, r)
		}), &fakeREST{})
		err := client.SaveViewFilter(context.Background(), "view", "is:open", "")
		if conflict {
			if err == nil || writes != 0 {
				t.Fatal("conflicting filter overwritten")
			}
		} else if err != nil || writes != 1 {
			t.Fatalf("timeout not reconciled: %v writes=%d", err, writes)
		}
	}
}
