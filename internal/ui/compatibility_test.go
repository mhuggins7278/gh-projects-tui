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

func TestValidateSavedFilterAllowsProbedTermsAndConjunctions(t *testing.T) {
	filters := []string{
		"",
		"iteration:@current",
		`status:"Todo"`,
		"status:Todo",
		`status:"Todo","Done"`,
		"no:status",
		"has:status",
		`-status:"Todo"`,
		`-status:"Todo","Done"`,
		"assignee:@me",
		"assignee:octocat",
		"assignee:@me,octocat",
		"assignee:octocat assignee:stevecat",
		"no:assignee",
		"has:assignee",
		"-no:assignee",
		"label:bug,support",
		`label:"bug fix"`,
		"-label:bug",
		"no:label",
		"-no:label",
		"repo:octocat/game",
		"is:issue is:open",
		"is:pr",
		"is:draft",
		"is:merged",
		"created:2026-09-23", "created:>=2026-09-24", "created:2026-09-23..2026-09-24",
		"updated:@today-1d", "updated:>@today-2d", "updated:@today-3d..@today-1d",
		"updated:@today",
		`title:"Expand saved-view support to broader GitHub filter grammar"`,
		"title:Expand*", "title:*filter*", "filter grammar",
		"created:*..2026-09-24", "created:<@today",
		`status:"Done" assignee:@me`,
		`-status:"Todo" assignee:@me`,
		`iteration:@current status:"Todo"`,
		`label:bug is:issue is:closed`,
		`has:status label:bug`,
	}
	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			if err := validateSavedFilter(filter); err != nil {
				t.Fatalf("verified filter rejected: %v", err)
			}
		})
	}
}

func TestValidateSavedFilterRejectsUnverifiedSyntax(t *testing.T) {
	filters := []string{
		`status:""`,
		`status:"quote\"value"`,
		`status:"unterminated`,
		`status:"Todo", "Done"`,
		`status:"Todo",`,
		`status:,"Done"`,
		`status:"Done".."Todo"`,
		`status:"Work, blocked"`,
		`status:'Done'`,
		`label:*bug*`,
		`label:"bug" ,support`,
		"assignee:@here",
		"assignee:",
		"repo:octocat",
		"repo:octocat/game,octocat/other",
		"has:priority",
		"-has:label",
		"is:review",
		"is:open,closed",
		"iteration:@next",
		"date:>=@today",
		"created:2026-02-30", "created:2026-9-2", "created:>=", "updated:@today-",
		"updated:@today-1month", "updated:@today--1d", "created:*..*",
		"points:1..3",
		"reviewers:@me",
		"title:API**", "title:foo*bar", "title:\"\"", "*",
		"OR",
		`status:"Todo" OR assignee:@me`,
		`status:"Todo" and assignee:@me`,
	}
	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			if err := validateSavedFilter(filter); err == nil {
				t.Fatalf("unverified filter accepted: %q", filter)
			}
		})
	}
}

func TestSavedFilterUsesProjectFieldsEvenWhenNotVisible(t *testing.T) {
	fields := []github.Field{
		{Name: "TUI Probe Points", DataType: "NUMBER"},
		{Name: "TUI Probe Iteration", DataType: "ITERATION"},
		{Name: "TUI Probe Note", DataType: "TEXT"},
		{Name: "Phase", DataType: "SINGLE_SELECT"},
	}
	valid := []string{
		"tui-probe-points:>=2", "tui-probe-points:>2", "tui-probe-points:1..2",
		"tui-probe-points:*..2", "tui-probe-points:2,5", "-tui-probe-points:2",
		"no:tui-probe-points", "has:tui-probe-points",
		"tui-probe-iteration:@current", "tui-probe-iteration:@previous",
		"tui-probe-iteration:<@current", "tui-probe-iteration:@previous..@current",
		`has:tui-probe-iteration tui-probe-iteration:"Probe past"`,
		`phase:"Phase I","Phase II"`, "no:phase", "has:phase", `-phase:"Phase I"`,
	}
	for _, filter := range valid {
		t.Run(filter, func(t *testing.T) {
			view := github.View{Layout: github.BoardLayout, Filter: filter, ProjectFields: fields}
			if result := evaluateViewCompatibility(view); !result.supported() {
				t.Fatalf("known project field rejected: %s", result.summary())
			}
		})
	}
	invalid := []string{
		"tui-probe-points:>=2", // no field metadata means no guessing
		"tui-probe-points:1...2", "tui-probe-points:*..*", "tui-probe-points:>=NaN",
		"tui-probe-points:>2..5", "tui-probe-points:>=", "tui-probe-iteration:@next",
		"tui-probe-iteration:@current+3", "tui-probe-iteration:>=2026-09-22",
		"tui-probe-note:hello", "has:tui-probe-note", "tui-unknown-field:2",
		`phase:""`, "phase:>=1",
	}
	for index, filter := range invalid {
		t.Run(filter, func(t *testing.T) {
			selected := fields
			if index == 0 {
				selected = nil
			}
			if err := validateSavedFilterWithFields(filter, selected); err == nil {
				t.Fatalf("unverified project filter accepted: %q", filter)
			}
		})
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
		{name: "unverified filter", view: github.View{Layout: github.BoardLayout, Filter: "reviewers:@me"}, reason: "has not been verified"},
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
