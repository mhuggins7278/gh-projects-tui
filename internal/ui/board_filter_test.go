package ui

import (
	"reflect"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

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
		{`(-status:Triage)`, nil},
		{`-status:"Triage`, nil},
		{`-priority:Triage`, map[string]bool{}},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			if got := excludedStatusValues(tc.filter); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
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

func TestFilteredMoveSkipsExcludedOccupiedCanonicalLane(t *testing.T) {
	view := writableStatusView()
	view.Layout = github.BoardLayout
	view.Filter = "-status:B"
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
}
