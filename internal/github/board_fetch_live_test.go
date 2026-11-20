//go:build live

package github

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
)

// TestLiveBoardFetchComparison prints timings and parity only. It reads one
// saved view through each path using cold client caches.
func TestLiveBoardFetchComparison(t *testing.T) {
	owner := Owner{Login: os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER"), Kind: OrganizationOwner}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "user" {
		owner.Kind = UserOwner
	}
	project, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	number, _ := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if owner.Login == "" || project <= 0 || number <= 0 {
		t.Skip("set GH_PROJECTS_TUI_LIVE_OWNER, GH_PROJECTS_TUI_LIVE_PROJECT, and GH_PROJECTS_TUI_LIVE_VIEW")
	}
	client, err := NewClientWithHost(config.DefaultHost())
	if err != nil {
		t.Fatal("could not initialize the authenticated GitHub client")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	view, err := client.OpenView(ctx, owner, project, number)
	if err != nil {
		t.Fatal("could not load the saved view; check the owner kind, numbers, host and authentication")
	}
	if view.Layout != BoardLayout {
		t.Fatal("select a Kanban board view")
	}
	leanFields := append(slices.Clone(view.GroupByFields), view.VerticalGroupBy...)
	for _, sort := range view.SortByFields {
		leanFields = append(leanFields, sort.Field)
	}
	fullFields := append(slices.Clone(view.Fields), leanFields...)

	page := func(fields []Field) func(string) (ItemsPage, error) {
		return func(after string) (ItemsPage, error) {
			return client.PageBoardItems(ctx, owner, project, view.Filter, after, fields)
		}
	}
	paths := []struct {
		name string
		read func(string) (ItemsPage, error)
	}{
		{"current GraphQL", page(fullFields)},
		{"lean GraphQL", page(leanFields)},
		{"saved-view REST", func(after string) (ItemsPage, error) {
			return client.PageSavedViewMembership(ctx, owner, project, number, after)
		}},
	}

	var baseline []string
	var baselineMembership []string
	baselineReady := false
	for i, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			client.InvalidateReads()
			ids, seen, cursors := []string{}, map[string]bool{}, map[string]bool{}
			start := time.Now()
			var first, slowest time.Duration
			after, pages := "", 0
			for {
				pageStart := time.Now()
				result, err := path.read(after)
				if err != nil {
					t.Fatalf("page %d failed (%T); check endpoint availability and token access", pages+1, err)
				}
				pages++
				if pages == 1 {
					first = time.Since(start)
				}
				slowest = max(slowest, time.Since(pageStart))
				for _, item := range result.Items {
					if item.ID == "" || seen[item.ID] {
						t.Fatal("missing or duplicate item identity")
					}
					ids = append(ids, item.ID)
					seen[item.ID] = true
				}
				if !result.HasNext {
					break
				}
				after = result.EndCursor
				if after == "" || cursors[after] {
					t.Fatal("pagination cursor did not advance")
				}
				cursors[after] = true
			}
			total := time.Since(start)
			t.Logf("%d items, %d pages, first %s, total %s, slowest %s", len(ids), pages,
				first.Round(time.Millisecond), total.Round(time.Millisecond), slowest.Round(time.Millisecond))
			membership := slices.Clone(ids)
			slices.Sort(membership)
			if i == 0 {
				baseline, baselineMembership, baselineReady = ids, membership, true
			} else if baselineReady {
				t.Logf("matches current GraphQL: membership=%t, page order=%t", slices.Equal(membership, baselineMembership), slices.Equal(ids, baseline))
			}
		})
	}
}
