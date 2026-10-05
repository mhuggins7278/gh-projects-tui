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

func TestViewCompatibilityAcceptsIssueTypeFilteredBoard(t *testing.T) {
	view := github.View{Layout: github.BoardLayout, Filter: "-type:Epic"}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("issue-type-filtered board rejected: %s", compatibility.summary())
	}
}

func TestViewCompatibilityAcceptsParentIssueFilteredBoard(t *testing.T) {
	view := github.View{Layout: github.BoardLayout, Filter: `parent-issue:"glg/5mp#47" is:open`}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("parent-issue-filtered board rejected: %s", compatibility.summary())
	}
}

func TestViewCompatibilityAcceptsExpandedFiltersOnBoardsAndTables(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, filter := range []string{
			"closed:>=2026-09-24", "has:closed", "no:parent-issue",
			"reviewers:octocat,stevecat", `reason:completed,"not planned"`,
			"-parent-issue:octocat/game#12", "label:*bug*", `title:"Title, with comma"`,
		} {
			t.Run(string(layout)+"/"+filter, func(t *testing.T) {
				view := github.View{Layout: layout, Filter: filter, GroupByFields: []github.Field{compatibilityStatusField()}}
				if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
					t.Fatalf("verified filter rejected: %s", compatibility.summary())
				}
			})
		}
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

func TestViewCompatibilityRequiresIterationDatesForIterationSort(t *testing.T) {
	view := github.View{Layout: github.BoardLayout, SortByFields: []github.SortField{{
		Direction: "ASC",
		Field:     github.Field{Name: "Iteration", DataType: "ITERATION", Iterations: []github.Iteration{{ID: "current", Title: "Current", StartDate: "2026-09-22"}}},
	}}}
	if compatibility := evaluateViewCompatibility(view); !compatibility.supported() {
		t.Fatalf("configured iteration sort rejected: %s", compatibility.summary())
	}
	view.SortByFields[0].Field.Iterations[0].StartDate = ""
	if compatibility := evaluateViewCompatibility(view); compatibility.supported() || !strings.Contains(compatibility.summary(), "sort field") {
		t.Fatalf("iteration sort without dates was accepted: %#v", compatibility)
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
		{name: "roadmap with date metadata", view: github.View{
			Layout:        github.RoadmapLayout,
			ProjectFields: []github.Field{{ID: "start", Name: "Start", DataType: "DATE"}, {ID: "target", Name: "Target", DataType: "DATE"}},
			Fields:        []github.Field{{ID: "title", Name: "Title", DataType: "TITLE"}, {ID: "start", Name: "Start", DataType: "DATE"}, {ID: "target", Name: "Target", DataType: "DATE"}},
		}, reason: "roadmap views are not supported"},
		{name: "multiple columns", view: github.View{Layout: github.BoardLayout, GroupByFields: []github.Field{status, status}}, reason: "multiple column grouping"},
		{name: "multiple table groups", view: github.View{Layout: github.TableLayout, GroupByFields: []github.Field{status, status}}, reason: "multiple column grouping"},
		{name: "vertical table groups", view: github.View{Layout: github.TableLayout, VerticalGroupBy: []github.Field{status}}, reason: "vertical grouping is not supported in table views"},
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

	updated, cmd := model.Update(viewDetailMsg{scope: model.currentPickerReadScope(),
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

func TestRoadmapDateAndIterationMetadataDoesNotEnableLoading(t *testing.T) {
	for _, dataType := range []string{"DATE", "ITERATION"} {
		t.Run(dataType, func(t *testing.T) {
			field := github.Field{ID: "endpoint", Name: "Start", DataType: dataType}
			if dataType == "ITERATION" {
				field.Iterations = []github.Iteration{{ID: "current", Title: "Current", StartDate: "2026-09-01", Duration: 14}}
			}
			view := github.View{
				Number: 2, Name: "Roadmap", Layout: github.RoadmapLayout, ViewerCanUpdate: true,
				ProjectFields: []github.Field{field}, Fields: []github.Field{field},
			}
			model := NewModel(nil)
			model.screen = screenViewPicker
			model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
			model.selectedProject = &github.Project{Number: 1, Title: "Project"}
			updated, cmd := model.Update(viewDetailMsg{scope: model.currentPickerReadScope(), generation: model.generation, view: &view})
			result := updated.(Model)
			if cmd != nil || result.screen != screenViewPicker || result.view != nil || result.itemsLoading {
				t.Fatal("roadmap metadata enabled item loading or entered a writable view")
			}
			if !strings.Contains(result.status, "roadmap views are not supported") {
				t.Fatalf("missing roadmap explanation: %q", result.status)
			}
			if !strings.Contains(result.status, "required timeline settings") || !strings.Contains(result.status, "open this view in GitHub") {
				t.Fatalf("missing actionable roadmap limitation: %q", result.status)
			}
		})
	}
}

func TestRoadmapLayoutsStayBlockedRegardlessOfDateFields(t *testing.T) {
	start := github.Field{ID: "start", Name: "Start", DataType: "DATE"}
	target := github.Field{ID: "target", Name: "Target", DataType: "DATE"}
	for _, test := range []struct {
		name    string
		project []github.Field
		visible []github.Field
	}{
		{name: "missing definitions"},
		{name: "stale visible definition", project: []github.Field{target}, visible: []github.Field{start, target}},
		{name: "ambiguous names", project: []github.Field{start, {ID: "other-start", Name: "Start", DataType: "DATE"}, target}},
		{name: "two plausible dates", project: []github.Field{start, target}, visible: []github.Field{start, target}},
		{name: "unsupported definitions", project: []github.Field{{ID: "start", Name: "Start", DataType: "TEXT"}, {ID: "target", Name: "Target", DataType: "TEXT"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			view := github.View{
				Number: 2, Name: "Roadmap", Layout: github.RoadmapLayout, ViewerCanUpdate: true,
				ProjectFields: test.project, Fields: test.visible,
			}
			model := NewModel(nil)
			model.screen = screenViewPicker
			model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
			model.selectedProject = &github.Project{Number: 1, Title: "Project"}
			updated, cmd := model.Update(viewDetailMsg{scope: model.currentPickerReadScope(), generation: model.generation, view: &view})
			result := updated.(Model)
			if cmd != nil || result.screen != screenViewPicker || result.view != nil || result.itemsLoading {
				t.Fatal("unavailable endpoint mapping enabled a roadmap")
			}
			if !strings.Contains(result.status, "roadmap views are not supported in this release") {
				t.Fatalf("missing unsupported-view explanation: %q", result.status)
			}
		})
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
