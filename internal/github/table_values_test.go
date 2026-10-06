package github

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMultiSelectUsesSelectedOptionsOnce(t *testing.T) {
	options := []FieldOption{{ID: "eight", Name: "Sprint 8"}, {ID: "nine", Name: "Sprint 9"}, {ID: "eight", Name: "Sprint 8"}}
	cases := []struct {
		name string
		raw  rawFieldValue
		want string
	}{
		{name: "project options and value", raw: rawFieldValue{Kind: "ProjectV2ItemFieldMultiSelectValue", Options: options, Value: "Sprint 8, Sprint 9"}, want: "Sprint 8, Sprint 9"},
		{name: "value fallback", raw: rawFieldValue{Kind: "ProjectV2ItemFieldMultiSelectValue", Value: "Sprint 9"}, want: "Sprint 9"},
		{name: "empty", raw: rawFieldValue{Kind: "ProjectV2ItemFieldMultiSelectValue"}},
		{name: "issue backed", raw: rawFieldValue{Kind: "ProjectV2ItemIssueFieldValue", IssueFieldValue: &rawIssueFieldValue{Kind: "IssueFieldMultiSelectValue", Options: options}}, want: "Sprint 8, Sprint 9"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := test.raw.project()
			if !value.Available || value.Value != test.want {
				t.Fatalf("value = %#v, want %q", value, test.want)
			}
		})
	}
}

func TestVisibleLinkedPullRequestsAreFetchedWithoutDetailReads(t *testing.T) {
	for _, ownerKind := range []OwnerKind{UserOwner, OrganizationOwner} {
		for _, selective := range []bool{false, true} {
			for _, truncated := range []bool{false, true} {
				t.Run(string(ownerKind)+"/"+map[bool]string{true: "selective", false: "full"}[selective]+"/"+map[bool]string{true: "truncated", false: "complete"}[truncated], func(t *testing.T) {
					branch := "user"
					if ownerKind == OrganizationOwner {
						branch = "organization"
					}
					calls := 0
					base := reviewGraphQLFunc(func(_ context.Context, query string, variables map[string]interface{}, response interface{}) error {
						calls++
						if !strings.Contains(query, "pullRequests(first: 10)") || strings.Contains(query, "ItemDetail") || variables["filter"] != "has:otc-sprint" {
							t.Fatalf("incorrect row query: %s, %v", query, variables)
						}
						value := map[string]interface{}{"__typename": "ProjectV2ItemFieldPullRequestValue", "field": map[string]string{"id": "pulls", "name": "Linked pull requests"}, "pullRequests": map[string]interface{}{"nodes": []interface{}{map[string]interface{}{"number": 20781, "repository": map[string]string{"nameWithOwner": "owner/repo"}}}, "pageInfo": map[string]interface{}{"hasNextPage": truncated}}}
						node := map[string]interface{}{"id": "item"}
						if selective {
							node["field0"] = value
						} else {
							node["fieldValues"] = map[string]interface{}{"nodes": []interface{}{value}}
						}
						payload := map[string]interface{}{branch: map[string]interface{}{"projectV2": map[string]interface{}{"items": map[string]interface{}{"nodes": []interface{}{node}}}}}
						data, err := json.Marshal(payload)
						if err != nil {
							return err
						}
						return json.Unmarshal(data, response)
					})
					client := newClient(base, &fakeREST{})
					owner := Owner{Login: "owner", Kind: ownerKind}
					var page ItemsPage
					var err error
					if selective {
						page, err = client.PageBoardItems(context.Background(), owner, 7, "has:otc-sprint", "", []Field{{Name: "Linked pull requests", DataType: "PULL_REQUEST"}})
					} else {
						page, err = client.PageItems(context.Background(), owner, 7, "has:otc-sprint", "")
					}
					if err != nil || len(page.Items) != 1 || len(page.Items[0].FieldValues) != 1 || !page.Items[0].FieldValues[0].Available || calls != 1 {
						t.Fatalf("linked PR read = %#v, %v, calls=%d", page, err, calls)
					}
					want := "owner/repo #20781"
					if truncated {
						want += ", ..."
					}
					if value := page.Items[0].FieldValues[0].Value; value != want {
						t.Fatalf("linked PR value = %q, want %q", value, want)
					}
				})
			}
		}
	}
}

func TestSelectiveRowsHydrateOnlyVisibleStandardMetadata(t *testing.T) {
	fields := []Field{{ID: "labels", Name: "Labels", DataType: "LABELS"}, {ID: "milestone", Name: "Milestone", DataType: "MILESTONE"}, {ID: "repo", Name: "Repository", DataType: "REPOSITORY"}, {ID: "reviewers", Name: "Reviewers", DataType: "REVIEWERS"}}
	for _, owner := range []Owner{{Login: "owner", Kind: UserOwner}, {Login: "owner", Kind: OrganizationOwner}} {
		branch := "user"
		if owner.Kind == OrganizationOwner {
			branch = "organization"
		}
		calls := 0
		api := reviewGraphQLFunc(func(_ context.Context, query string, vars map[string]interface{}, response interface{}) error {
			calls++
			for _, required := range []string{"labels(first: 10)", "milestone { title }", "repository { name nameWithOwner }", "reviewers(first: 10)", "... on Team { name }"} {
				if !strings.Contains(query, required) {
					t.Fatalf("missing %s", required)
				}
			}
			if strings.Contains(query, "ItemDetail") || strings.Contains(query, "first: 100) { nodes { login }") {
				t.Fatal("unbounded detail read")
			}
			node := map[string]interface{}{"id": "item", "field0": map[string]interface{}{"__typename": "ProjectV2ItemFieldLabelValue", "field": map[string]string{"id": "labels"}, "labels": map[string]interface{}{"nodes": []map[string]string{{"name": "bug"}}, "pageInfo": map[string]bool{"hasNextPage": true}}}, "field1": map[string]interface{}{"__typename": "ProjectV2ItemFieldMilestoneValue", "field": map[string]string{"id": "milestone"}, "milestone": map[string]string{"title": "Release"}}, "field2": map[string]interface{}{"__typename": "ProjectV2ItemFieldRepositoryValue", "field": map[string]string{"id": "repo"}, "repository": map[string]string{"nameWithOwner": "owner/repo"}}, "field3": map[string]interface{}{"__typename": "ProjectV2ItemFieldReviewerValue", "field": map[string]string{"id": "reviewers"}, "reviewers": map[string]interface{}{"nodes": []map[string]string{{"__typename": "User", "login": "alice"}, {"__typename": "Team", "name": "Platform"}}}}}
			payload := map[string]interface{}{branch: map[string]interface{}{"projectV2": map[string]interface{}{"items": map[string]interface{}{"nodes": []interface{}{node}}}}}
			b, _ := json.Marshal(payload)
			return json.Unmarshal(b, response)
		})
		page, err := newClient(api, &fakeREST{}).PageBoardItems(context.Background(), owner, 7, "", "", fields)
		if err != nil || calls != 1 {
			t.Fatalf("rows: %v calls=%d", err, calls)
		}
		for i, want := range []string{"bug, ...", "Release", "owner/repo", "alice, Platform"} {
			if !page.Items[0].FieldValues[i].Available || page.Items[0].FieldValues[i].Value != want {
				t.Fatalf("value %d: %#v", i, page.Items[0].FieldValues[i])
			}
		}
	}
	vars := map[string]interface{}{}
	_, selection, _ := selectedItemFields([]Field{{Name: "Status", DataType: "SINGLE_SELECT"}}, true, vars)
	if strings.Contains(selection, "labels(") || strings.Contains(selection, "reviewers(") {
		t.Fatal("unrelated nested metadata selected")
	}
}
