package ui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestBoardIncludedStatusLanes(t *testing.T) {
	view := writableStatusView()
	view.Layout = github.BoardLayout
	view.GroupByFields[0].Options = []github.FieldOption{
		{ID: "a", Name: "Triage"}, {ID: "b", Name: "Backlog"}, {ID: "c", Name: "Ready"},
		{ID: "d", Name: "In progress"}, {ID: "e", Name: "In review"}, {ID: "f", Name: "Done"},
	}
	all := []string{"Triage", "Backlog", "Ready", "In progress", "In review", "Done"}
	for _, tc := range []struct {
		filter string
		want   []string
	}{
		{`status:Triage,Backlog,Ready is:issue`, []string{"Triage", "Backlog", "Ready"}},
		{`STATUS:"Triage","Ready" is:issue`, []string{"Triage", "Ready"}},
		{`status:Triage,Backlog,Ready status:Backlog,Ready`, []string{"Backlog", "Ready"}},
		{`status:Triage,Backlog,Ready -status:Backlog`, []string{"Triage", "Ready"}},
		{`status:Triage,Ready OR status:Done`, all},
		{`NOT status:Triage,Ready`, all},
		{`(status:Triage,Ready)`, all},
		{`status:"Triage`, all},
		{`status:Ready,`, all},
		{`status:* status:Ready`, all},
		{`status:Ready,Unknown`, all},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			view.Filter = tc.filter
			items := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "c", Available: true}}}}
			lanes := lanesForView(&view, items)
			var names []string
			for _, lane := range lanes {
				names = append(names, lane.Name)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Fatalf("lanes = %v, want %v", names, tc.want)
			}
			if tc.filter == `status:Triage,Backlog,Ready is:issue` {
				m := mutationModel(fakePickerSource{}, view, items)
				m.width, m.height = 180, 40
				content := m.View().Content
				for _, hidden := range []string{"In progress", "In review", "Done"} {
					if strings.Contains(content, "["+hidden+"]") {
						t.Fatalf("filtered column %q is still rendered", hidden)
					}
				}
				// Returned/optimistic cards must never disappear because of
				// this presentation-only interpretation of the saved filter.
				items = append(items, github.Item{ID: "pending", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "f", Available: true}}})
				lanes = lanesForView(&view, items)
				if len(lanes) != 4 || lanes[3].Name != "Done" || len(lanes[3].Items) != 1 {
					t.Fatalf("populated lane lost: %#v", lanes)
				}
				view.Layout = github.TableLayout
				if lanes := lanesForView(&view, items); len(lanes) != len(all) {
					t.Fatalf("board-only pruning changed table groups: %#v", lanes)
				}
				view.Layout = github.BoardLayout
			}
		})
	}
}

func TestBoardExcludedStatusLanes(t *testing.T) {
	view := writableStatusView()
	view.Layout = github.BoardLayout
	view.GroupByFields[0].Options = []github.FieldOption{{ID: "a", Name: "Triage"}, {ID: "b", Name: "Backlog"}, {ID: "c", Name: "Todo"}, {ID: "d", Name: "Done"}}
	view.Filter = "-status:Triage,Backlog"
	items := []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "c", Available: true}}}}
	lanes := lanesForView(&view, items)
	var names []string
	for _, lane := range lanes {
		names = append(names, lane.Name)
	}
	if !reflect.DeepEqual(names, []string{"Todo", "Done"}) {
		t.Fatalf("lanes = %v", names)
	}
	// A populated lane is retained until GitHub confirms the item's membership.
	items = append(items, github.Item{ID: "pending", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}})
	lanes = lanesForView(&view, items)
	if lanes[0].Name != "Triage" || len(lanes[0].Items) != 1 {
		t.Fatalf("populated lane lost: %#v", lanes)
	}
}

func TestExcludedStatusFilterParsing(t *testing.T) {
	cases := []struct {
		filter string
		want   map[string]bool
	}{
		{`-status:Triage,Backlog`, map[string]bool{"triage": true, "backlog": true}},
		{`-STATUS:"Needs Review","Triage, waiting"`, map[string]bool{"needs review": true, "triage, waiting": true}},
		{`-status:Triage OR status:Backlog`, nil},
		{`NOT -status:Triage`, nil},
		{`(-status:Triage)`, nil},
		{`-status:"Triage`, nil},
		{`-priority:Triage`, map[string]bool{}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			if got := excludedStatusValues(tc.filter, nil); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIncludedStatusFilterParsing(t *testing.T) {
	options := []github.FieldOption{
		{Name: "Needs Review"}, {Name: "Triage, waiting"}, {Name: `Review "ready"`}, {Name: "Done"},
	}
	for _, tc := range []struct {
		filter string
		want   map[string]bool
	}{
		{`status:"Needs Review","Triage, waiting"`, map[string]bool{`review "ready"`: true, "done": true}},
		{`STATUS:"Review \"ready\""`, map[string]bool{"needs review": true, "triage, waiting": true, "done": true}},
		{`status:Unknown`, nil},
	} {
		t.Run(tc.filter, func(t *testing.T) {
			if got := excludedStatusValues(tc.filter, options); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	if got := excludedStatusValues("status:Done", nil); got != nil {
		t.Fatalf("missing option metadata was treated as an empty inclusion list: %v", got)
	}
}

func TestCombinedBoardExcludedStatusAxis(t *testing.T) {
	for _, statusColumn := range []bool{false, true} {
		view := writableCombinedView()
		if statusColumn {
			view.GroupByFields, view.VerticalGroupBy = view.VerticalGroupBy, view.GroupByFields
		}
		view.Filter = "-status:Done"
		lanes := lanesForView(&view, []github.Item{combinedItem("item", "p0", "todo")})
		if len(lanes) == 0 {
			t.Fatal("all lanes hidden")
		}
		for _, lane := range lanes {
			if (statusColumn && lane.Name == "Done") || (!statusColumn && lane.RowName == "Done") {
				t.Fatalf("excluded axis remains: %#v", lane)
			}
		}
	}
}

func TestCombinedBoardIncludedStatusAxis(t *testing.T) {
	for _, statusColumn := range []bool{false, true} {
		view := writableCombinedView()
		if statusColumn {
			view.GroupByFields, view.VerticalGroupBy = view.VerticalGroupBy, view.GroupByFields
		}
		view.Filter = "status:Todo"
		lanes := lanesForView(&view, []github.Item{combinedItem("item", "p0", "todo")})
		if len(lanes) == 0 {
			t.Fatal("all lanes hidden")
		}
		for _, lane := range lanes {
			if (statusColumn && lane.Name == "Done") || (!statusColumn && lane.RowName == "Done") {
				t.Fatalf("excluded axis remains: %#v", lane)
			}
		}
	}
}

func TestFilteredMoveSkipsExcludedOccupiedCanonicalLane(t *testing.T) {
	for _, filter := range []string{"-status:B", "status:A,C"} {
		t.Run(filter, func(t *testing.T) {
			testFilteredMoveSkipsExcludedOccupiedCanonicalLane(t, filter)
		})
	}
}

func testFilteredMoveSkipsExcludedOccupiedCanonicalLane(t *testing.T, filter string) {
	t.Helper()
	view := writableStatusView()
	view.Layout = github.BoardLayout
	view.Filter = filter
	canonical := []github.Item{
		{ID: "moving", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}}},
		{ID: "hidden", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "b", Available: true}}},
	}
	mutations := []string{}
	model := mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical[:1])
	updated, cmd := model.Update(keyPress("L"))
	model = runMutationCommands(t, updated.(Model), cmd)
	if canonical[0].FieldValues[0].OptionID != "c" {
		t.Fatalf("moved into hidden lane: %#v, mutations %v, status %s", canonical[0], mutations, model.status)
	}
	before := len(mutations)
	// Start from a fully loaded saved-view snapshot at the last allowed lane.
	model = mutationModel(fakePickerSource{mutations: &mutations, canonicalItems: &canonical}, view, canonical[:1])
	model.boardFocusID = "moving"
	model.clampBoardCursor()
	updated, cmd = model.Update(keyPress("L"))
	model = runMutationCommands(t, updated.(Model), cmd)
	if len(mutations) != before || !strings.Contains(model.status, "No destination lane") {
		t.Fatalf("move past the last allowed lane was not blocked: mutations %v, status %s", mutations, model.status)
	}
}
