//go:build live

package github

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// Compare the saved filter through two independently implemented GitHub APIs.
// Read twice around REST to detect a project changing while it is being sampled.
func TestLiveSavedViewMembershipParity(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_PARITY") != "1" {
		t.Skip("set GH_PROJECTS_TUI_LIVE_PARITY=1 for read-only REST/GraphQL parity checks")
	}
	owner := Owner{Login: os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER"), Kind: OrganizationOwner}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "user" {
		owner.Kind = UserOwner
	}
	project, e1 := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	viewNumber, e2 := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if owner.Login == "" || e1 != nil || e2 != nil || project <= 0 || viewNumber <= 0 {
		t.Fatal("set owner and positive project/view numbers")
	}
	client, err := NewClientWithHost("github.com")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	view, err := client.OpenView(ctx, owner, project, viewNumber)
	if err != nil {
		t.Fatal(err)
	}
	read := func(rest bool) map[string]bool {
		result, cursors := map[string]bool{}, map[string]bool{}
		after := ""
		for {
			var page ItemsPage
			var err error
			if rest {
				page, err = client.PageSavedViewMembership(ctx, owner, project, viewNumber, after)
			} else {
				page, err = client.PageBoardItems(ctx, owner, project, view.Filter, after, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range page.Items {
				if result[item.ID] {
					t.Fatal("duplicate membership across pages")
				}
				result[item.ID] = true
			}
			if !page.HasNext {
				return result
			}
			if page.EndCursor == "" || cursors[page.EndCursor] {
				t.Fatal("membership cursor did not advance")
			}
			cursors[page.EndCursor] = true
			after = page.EndCursor
		}
	}
	before, rest := read(false), read(true)
	client.InvalidateReads()
	after := read(false)
	freshView, err := client.OpenView(ctx, owner, project, viewNumber)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || freshView.Filter != view.Filter {
		t.Fatal("project/filter changed during parity sampling; retry on a stable view")
	}
	if !reflect.DeepEqual(before, rest) {
		t.Fatalf("membership differs: GraphQL=%d REST=%d; inspect differences privately", len(before), len(rest))
	}
	t.Logf("stable saved-view membership agrees: %d items", len(before))
}

func TestLiveEnhancedRowReads(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_PARITY") != "1" {
		t.Skip("enable read-only parity probes")
	}
	owner := Owner{Login: os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER"), Kind: OrganizationOwner}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "user" {
		owner.Kind = UserOwner
	}
	project, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	viewNumber, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if owner.Login == "" || project <= 0 || viewNumber <= 0 {
		t.Fatal("set owner and project/view numbers")
	}
	c, err := NewClientWithHost("github.com")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	view, err := c.OpenView(ctx, owner, project, viewNumber)
	if err != nil {
		t.Fatal(err)
	}
	fields := []Field{}
	for _, field := range view.ProjectFields {
		switch field.DataType {
		case "LABELS", "MILESTONE", "REPOSITORY", "REVIEWERS", "MULTI_SELECT":
			fields = append(fields, field)
		}
	}
	page, err := c.PageBoardItems(ctx, owner, project, view.Filter, "", fields)
	if err != nil {
		t.Fatal(err)
	}
	available := 0
	for _, item := range page.Items {
		for _, v := range item.FieldValues {
			if v.Available {
				available++
			}
		}
	}
	t.Logf("selected standard/multi-select row reads: %d fields, %d items, %d populated cells", len(fields), len(page.Items), available)
	// Verify the shared full-row fragment as well as the selective path.
	if _, err = c.PageItems(ctx, owner, project, view.Filter, ""); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.Content != nil && item.Content.Kind == "Issue" && item.Content.SubIssueTotal > 0 {
			children, err := c.PageSubIssues(ctx, view.ProjectID, item.Content.ID, "", fields)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("identity-first sub-issue read: %d children; continuation=%v", len(children.Items), children.HasNext)
			return
		}
	}
	t.Log("no parent with children in first sampled page; hierarchy fixture tests cover the batched path")
}
