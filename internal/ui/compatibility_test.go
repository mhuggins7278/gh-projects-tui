package ui

import (
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func compatibilityStatusField() github.Field {
	return github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "todo", Name: "Todo"}},
	}
}

func TestViewCompatibilityAcceptsSupportedBoard(t *testing.T) {
	view := github.View{
		Layout:        github.BoardLayout,
		Filter:        "iteration:@current",
		GroupByFields: []github.Field{compatibilityStatusField()},
		SortByFields:  []github.SortField{{Direction: "ASC", Field: github.Field{Name: "Title", DataType: "TITLE"}}},
	}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("supported view rejected: %s", compatibility.summary())
	}
}

func TestViewCompatibilityAcceptsVerticalPrimaryGrouping(t *testing.T) {
	view := github.View{
		Layout:          github.BoardLayout,
		VerticalGroupBy: []github.Field{compatibilityStatusField()},
	}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("vertical primary grouping rejected: %s", compatibility.summary())
	}
}

func TestViewCompatibilityAcceptsCombinedGrouping(t *testing.T) {
	view := github.View{
		Layout:        github.BoardLayout,
		GroupByFields: []github.Field{compatibilityStatusField()},
		VerticalGroupBy: []github.Field{{
			ID:       "priority",
			Name:     "Priority",
			Kind:     "ProjectV2SingleSelectField",
			DataType: "SINGLE_SELECT",
			Options:  []github.FieldOption{{ID: "p0", Name: "P0"}},
		}},
	}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("combined view rejected: %s", compatibility.summary())
	}
}

func TestIterationLanesPreserveSavedActiveAndCompletedOrder(t *testing.T) {
	iteration := github.Field{
		ID: "iteration", Name: "Iteration", Kind: "ProjectV2IterationField", DataType: "ITERATION",
		Iterations: []github.Iteration{
			{ID: "current", Title: "Current"},
			{ID: "next", Title: "Next"},
			{ID: "done-a", Title: "Completed A", Completed: true},
			{ID: "done-b", Title: "Completed B", Completed: true},
		},
	}
	lanes, supported := laneDefinitions(iteration)
	if !supported {
		t.Fatal("iteration grouping was not supported")
	}
	want := []string{"no-value", "iteration:current", "iteration:next", "iteration:done-a", "iteration:done-b"}
	if len(lanes) != len(want) {
		t.Fatalf("iteration lanes = %#v", lanes)
	}
	for index, key := range want {
		if lanes[index].key != key {
			t.Fatalf("lane %d = %q, want %q", index, lanes[index].key, key)
		}
	}
}

func TestViewCompatibilityAcceptsTable(t *testing.T) {
	view := github.View{Layout: github.TableLayout, Fields: []github.Field{{Name: "Title", DataType: "TITLE"}}}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("table view rejected: %s", compatibility.summary())
	}
}

func TestViewCompatibilityRejectsUnsupportedSemantics(t *testing.T) {
	status := compatibilityStatusField()
	cases := []struct {
		name   string
		view   github.View
		reason string
	}{
		{name: "roadmap layout", view: github.View{Layout: github.RoadmapLayout}, reason: "roadmap views are not supported"},
		{name: "unverified filter", view: github.View{Layout: github.BoardLayout, Filter: "assignee:@me"}, reason: "has not been verified"},
		{name: "multiple columns", view: github.View{Layout: github.BoardLayout, GroupByFields: []github.Field{status, status}}, reason: "multiple column grouping"},
		{name: "unsupported vertical grouping", view: github.View{Layout: github.BoardLayout, VerticalGroupBy: []github.Field{{Name: "Labels", DataType: "MULTI_SELECT"}}}, reason: "vertical grouping field"},
		{name: "unsupported grouping", view: github.View{Layout: github.BoardLayout, GroupByFields: []github.Field{{Name: "Labels", DataType: "MULTI_SELECT"}}}, reason: "not single-select or iteration"},
		{name: "missing options", view: github.View{Layout: github.BoardLayout, GroupByFields: []github.Field{{Name: "Status", DataType: "SINGLE_SELECT"}}}, reason: "has no options"},
		{name: "unsupported sort", view: github.View{Layout: github.BoardLayout, SortByFields: []github.SortField{{Direction: "ASC", Field: github.Field{Name: "Labels", DataType: "MULTI_SELECT"}}}}, reason: "sort field"},
		{name: "invalid direction", view: github.View{Layout: github.BoardLayout, SortByFields: []github.SortField{{Direction: "RANDOM", Field: github.Field{Name: "Title", DataType: "TITLE"}}}}, reason: "sort direction"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compatibility := evaluateViewCompatibility(tc.view)
			if compatibility.supported() || !strings.Contains(compatibility.summary(), tc.reason) {
				t.Fatalf("compatibility = %#v, want reason containing %q", compatibility, tc.reason)
			}
		})
	}
}

func TestIncompatibleViewStaysInPicker(t *testing.T) {
	model := NewModel(nil)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1, Title: "Project"}
	model.views = []github.ViewSummary{{Number: 2, Name: "Roadmap", Layout: github.RoadmapLayout}}

	updated, cmd := model.Update(viewDetailMsg{
		generation: model.generation,
		view:       &github.View{Number: 2, Name: "Roadmap", Layout: github.RoadmapLayout},
	})
	result := updated.(Model)
	if cmd != nil || result.screen != screenViewPicker || result.view != nil || result.itemsLoading {
		t.Fatalf("incompatible view entered board: screen=%v view=%#v loading=%v cmd nil=%v", result.screen, result.view, result.itemsLoading, cmd == nil)
	}
	if !strings.Contains(result.status, "roadmap views are not supported") {
		t.Fatalf("status = %q", result.status)
	}
}

func TestViewPickerLoadsTableLayout(t *testing.T) {
	model := NewModel(nil)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1, Title: "Project"}
	model.views = []github.ViewSummary{{Number: 3, Name: "Table", Layout: github.TableLayout}}

	updated, cmd := model.Update(keyPress("enter"))
	result := updated.(Model)
	if cmd == nil || !result.loadingDetail {
		t.Fatalf("table view did not start loading: cmd nil=%v loading=%v", cmd == nil, result.loadingDetail)
	}
	if result.status != "Loading view #3..." {
		t.Fatalf("status = %q", result.status)
	}
}
