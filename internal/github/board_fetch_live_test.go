//go:build live

package github

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
)

// TestLiveBoardFetchComparison makes read-only comparisons with cold application
// caches. GH_PROJECTS_TUI_LIVE_ROUNDS (default 1, max 5) rotates candidate order.
// The expensive full field-connection query requires
// GH_PROJECTS_TUI_LIVE_FULL_FIELDS=1.
// Logs never include item identities, titles, field names, or response bodies.
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
	rounds := 1
	if value := os.Getenv("GH_PROJECTS_TUI_LIVE_ROUNDS"); value != "" {
		var err error
		rounds, err = strconv.Atoi(value)
		if err != nil || rounds < 1 || rounds > 5 {
			t.Fatal("GH_PROJECTS_TUI_LIVE_ROUNDS must be between 1 and 5")
		}
	}
	client, err := NewClientWithHost(config.DefaultHost())
	if err != nil {
		t.Fatal("could not initialize the authenticated GitHub client")
	}
	transport := &boardProbeTransport{base: client.scheduler.base}
	client.scheduler.base = transport
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(rounds)*5*time.Minute)
	defer cancel()

	leanFields := func(view View) []Field {
		fields := append(slices.Clone(view.GroupByFields), view.VerticalGroupBy...)
		for _, sort := range view.SortByFields {
			fields = append(fields, sort.Field)
		}
		return fields
	}
	fullFields := func(view View) []Field {
		return append(slices.Clone(view.Fields), leanFields(view)...)
	}
	type candidate struct {
		name    string
		fields  func(View) []Field // nil means membership-only, not usable cards.
		content bool
		read    func(View, string) (ItemsPage, error)
		prepare func(View) (func(string) (boardProbePage, error), func())
	}
	paths := []candidate{
		{"current GraphQL", fullFields, true, nil, func(view View) (func(string) (boardProbePage, error), func()) {
			return boardProbeBidirectional(ctx, client, owner, project, view.Filter, fullFields(view))
		}},
		{"sequential GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
			return client.PageBoardItems(ctx, owner, project, view.Filter, after, fullFields(view))
		}, nil},
		{"legacy-permissions GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
			return boardProbeContentItems(ctx, client, owner, project, view.Filter, after, fullFields(view), false, false)
		}, nil},
		{"lean GraphQL", leanFields, true, func(view View, after string) (ItemsPage, error) {
			return client.PageBoardItems(ctx, owner, project, view.Filter, after, leanFields(view))
		}, nil},
		{"IDs-only GraphQL", nil, false, func(view View, after string) (ItemsPage, error) {
			return client.pageSelectedItems(ctx, owner, project, view.Filter, after, nil, false)
		}, nil},
		{"saved-view REST", nil, false, func(_ View, after string) (ItemsPage, error) {
			return client.PageSavedViewMembership(ctx, owner, project, number, after)
		}, nil},
		{"selected-fields REST", fullFields, true, nil, nil},
		{"structure-only GraphQL", leanFields, false, func(view View, after string) (ItemsPage, error) {
			return client.pageSelectedItems(ctx, owner, project, view.Filter, after, leanFields(view), false)
		}, nil},
		{"legacy-no-progress GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
			return boardProbeContentItems(ctx, client, owner, project, view.Filter, after, fullFields(view), false, true)
		}, nil},
		{"no-progress GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
			return boardProbeContentItems(ctx, client, owner, project, view.Filter, after, fullFields(view), true, true)
		}, nil},
	}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_LOADING") == "1" {
		paths = append(paths,
			candidate{"50-per-page GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
				return boardProbeSizedItems(ctx, client, owner, project, view.Filter, after, fullFields(view), 50, false, nil)
			}, nil},
			candidate{"25-first GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
				size := 100
				if after == "" {
					size = 25
				}
				return boardProbeSizedItems(ctx, client, owner, project, view.Filter, after, fullFields(view), size, false, nil)
			}, nil},
			candidate{"bidirectional GraphQL", fullFields, true, nil, func(view View) (func(string) (boardProbePage, error), func()) {
				return boardProbeBidirectional(ctx, client, owner, project, view.Filter, fullFields(view))
			}},
		)
	}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_FULL_FIELDS") == "1" {
		paths = append(paths, candidate{"field-connection GraphQL", fullFields, true, func(view View, after string) (ItemsPage, error) {
			// Fetches extra fields, but provides the same card projection without
			// repeatedly resolving fieldValueByName for each selected field.
			return client.PageItems(ctx, owner, project, view.Filter, after)
		}, nil})
	}
	type sample struct {
		items                         []Item
		view                          View
		setup, first, fetch, total    time.Duration
		setupRequests, requests, cost int
		bytes                         int64
	}
	run := func(path candidate) *sample {
		var result *sample
		t.Run(path.name, func(t *testing.T) {
			client.InvalidateReads()
			start := time.Now()
			before := client.APIStatus()
			view, err := client.OpenView(ctx, owner, project, number)
			if err != nil {
				t.Fatalf("view metadata read failed (%T)", err)
			}
			if view.Layout != BoardLayout {
				t.Fatal("select a Kanban board view")
			}
			read := func(after string) (ItemsPage, error) { return path.read(view, after) }
			var prepared func(string) (boardProbePage, error)
			if path.prepare != nil {
				var cleanup func()
				prepared, cleanup = path.prepare(view)
				defer cleanup()
			} else if path.read == nil {
				root, err := boardProbeProjectPath(owner, project)
				if err != nil {
					t.Fatal("invalid REST probe project")
				}
				ids, fields, err := boardProbeRESTFields(ctx, client, root, fullFields(view))
				if err != nil {
					t.Fatalf("REST field mapping failed (%T)", err)
				}
				read = func(after string) (ItemsPage, error) {
					return boardProbeRESTItems(ctx, client, root, number, after, ids, fields)
				}
			}
			s := &sample{view: view, setup: time.Since(start)}
			fetchStatus, fetchBytes := client.APIStatus(), transport.bytes.Load()
			s.setupRequests = fetchStatus.Requests - before.Requests
			fetchStart := time.Now()
			seen, cursors := map[string]bool{}, map[string]bool{}
			var slowest time.Duration
			after, pages, restarts := "", 0, 0
			for {
				pageStart := time.Now()
				var page ItemsPage
				if prepared != nil {
					var result boardProbePage
					result, err = prepared(after)
					page = result.ItemsPage
					if result.Reset {
						s.items = nil
						seen, cursors = map[string]bool{}, map[string]bool{}
						restarts++
					}
				} else {
					page, err = read(after)
				}
				if err != nil {
					t.Fatalf("page %d failed (%T); check endpoint availability and token access", pages+1, err)
				}
				pages++
				if pages == 1 {
					s.first = time.Since(fetchStart)
				}
				slowest = max(slowest, time.Since(pageStart))
				for _, item := range page.Items {
					if item.ID == "" || seen[item.ID] {
						t.Fatal("missing or duplicate item identity")
					}
					s.items = append(s.items, item)
					seen[item.ID] = true
				}
				if !page.HasNext {
					break
				}
				after = page.EndCursor
				if after == "" || cursors[after] {
					t.Fatal("pagination cursor did not advance")
				}
				cursors[after] = true
			}
			s.fetch, s.total = time.Since(fetchStart), time.Since(start)
			status := client.APIStatus()
			s.requests, s.cost = status.Requests-fetchStatus.Requests, status.TotalCost-fetchStatus.TotalCost
			s.bytes = transport.bytes.Load() - fetchBytes
			t.Logf("%d items, %d pages; setup %s (%d requests), first %s, fetch %s, end-to-end %s, slowest %s",
				len(s.items), pages, s.setup.Round(time.Millisecond), s.setupRequests, s.first.Round(time.Millisecond),
				s.fetch.Round(time.Millisecond), s.total.Round(time.Millisecond), slowest.Round(time.Millisecond))
			t.Logf("item reads: %d HTTP requests, %.2f MiB decoded responses, %d GraphQL points; known action permissions %d",
				s.requests, float64(s.bytes)/(1<<20), s.cost, boardProbePermissions(s.items))
			t.Logf("safe sequential restarts: %d", restarts)
			result = s
		})
		return result
	}
	compare := func(label string, baseline, other *sample, fields []Field, content bool) {
		if baseline == nil || other == nil {
			return
		}
		baseIDs, otherIDs := []string{}, []string{}
		baseItems := map[string]Item{}
		for _, item := range baseline.items {
			baseIDs = append(baseIDs, item.ID)
			baseItems[item.ID] = item
		}
		mismatches := 0
		differences := map[string]int{}
		for _, item := range other.items {
			otherIDs = append(otherIDs, item.ID)
			base, ok := baseItems[item.ID]
			if !content {
				base.Content, item.Content = nil, nil
			}
			if !ok || (content || fields != nil) && boardProbeCard(base, fields) != boardProbeCard(item, fields) {
				mismatches++
				for key, count := range boardProbeDifferences(base, item, fields) {
					differences[key] += count
				}
			}
		}
		order := slices.Equal(baseIDs, otherIDs)
		slices.Sort(baseIDs)
		slices.Sort(otherIDs)
		t.Logf("%s: membership=%t, page order=%t, view metadata=%t, compare content=%t, compare fields=%t, mismatched returned items=%d",
			label, slices.Equal(baseIDs, otherIDs), order, reflect.DeepEqual(baseline.view, other.view), content, fields != nil, mismatches)
		if len(differences) > 0 {
			t.Logf("aggregate differences (no private values): %v", differences)
		}
	}
	all := make([][]*sample, len(paths))
	for round := 0; round < rounds; round++ {
		results := make([]*sample, len(paths))
		for offset := range paths {
			index := (offset + round*max(1, len(paths)/rounds)) % len(paths)
			if round%2 != 0 {
				index = len(paths) - 1 - index
			}
			path := paths[index]
			path.name = fmt.Sprintf("round_%d/%s", round+1, path.name)
			results[index] = run(path)
			if results[index] != nil {
				all[index] = append(all[index], results[index])
			}
		}
		for index, path := range paths {
			if index == 0 || results[0] == nil {
				continue
			}
			var fields []Field
			if path.fields != nil {
				fields = path.fields(results[0].view)
			}
			compare(fmt.Sprintf("round %d %s vs current", round+1, path.name), results[0], results[index], fields, path.content)
		}
	}
	// Bracket the entire comparison, since live membership or card values may
	// change while a multi-minute run is in flight. No private diffs are printed.
	if len(all[0]) > 0 {
		verification := paths[0]
		verification.name = "verification/current GraphQL"
		first := all[0][0]
		compare("initial vs final baseline", first, run(verification), fullFields(first.view), true)
	}
	for index, samples := range all {
		if len(samples) == 0 {
			continue
		}
		median := func(value func(*sample) time.Duration) time.Duration {
			durations := make([]time.Duration, len(samples))
			for i, s := range samples {
				durations[i] = value(s)
			}
			slices.Sort(durations)
			return durations[len(durations)/2].Round(time.Millisecond)
		}
		t.Logf("summary %s (%d runs): median first %s, fetch %s, end-to-end %s",
			paths[index].name, len(samples), median(func(s *sample) time.Duration { return s.first }),
			median(func(s *sample) time.Duration { return s.fetch }), median(func(s *sample) time.Duration { return s.total }))
	}
}
