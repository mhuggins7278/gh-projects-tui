package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
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
		"no:status",
		`-status:"Todo"`,
		"assignee:@me",
		`status:"Done" assignee:@me`,
		`-status:"Todo" assignee:@me`,
		`iteration:@current status:"Todo"`,
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
		"status:Todo",
		`status:""`,
		`status:"quote\"value"`,
		`status:"unterminated`,
		"assignee:octocat",
		"iteration:@next",
		"no:assignee",
		`status:"Todo" OR assignee:@me`,
		`status:"Todo" status:"Todo"`,
		`status:"Todo" and assignee:@me`,
		`no:status assignee:@me`,
		`status:"Done" -status:"Todo"`,
		`status:"Done" assignee:@me iteration:@current`,
	}
	for _, filter := range filters {
		t.Run(filter, func(t *testing.T) {
			if err := validateSavedFilter(filter); err == nil {
				t.Fatalf("unverified filter accepted: %q", filter)
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
		{name: "unverified filter", view: github.View{Layout: github.BoardLayout, Filter: "assignee:octocat"}, reason: "has not been verified"},
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

func TestViewPickerOffersExplicitStatusFallbackOnlyWhenNoBoardIsCompatible(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	fallback := github.View{ProjectID: "project", Name: "Unfiltered Status fallback", Layout: github.BoardLayout, GroupByFields: []github.Field{status}, Fallback: true, ViewerCanUpdate: true}
	source := fakeStatusFallbackSource{
		fakePickerSource: fakePickerSource{
			view:      github.View{Number: 1, Name: "Filtered board", Layout: github.BoardLayout, Filter: "assignee:octocat"},
			itemPages: map[string]github.ItemsPage{"": {}},
		},
		fallback: fallback,
	}
	model := newPickerModel(source)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7, Title: "Project"}
	updated, probe := model.Update(viewsMsg{generation: model.generation, views: []github.ViewSummary{{Number: 1, Name: "Filtered board", Layout: github.BoardLayout}}})
	model = updated.(Model)
	if probe == nil || !model.checkingBoardViews {
		t.Fatal("board compatibility probe did not start")
	}
	updated, _ = model.Update(probe())
	model = updated.(Model)
	views := model.filteredViews()
	if len(views) != 2 || !views[1].Fallback || views[1].Number != 0 || !strings.Contains(views[1].Name, "Status fallback") {
		t.Fatalf("picker entries = %#v", views)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "explicit, unfiltered fallback") {
		t.Fatal("fallback was not clearly identified in the picker")
	}

	saved := config.Selection{}
	model.savePrefs = func(_ string, selection config.Selection) error { saved = selection; return nil }
	model.cursor = 1
	updated, load := model.Update(keyPress("enter"))
	model = updated.(Model)
	if model.screen != screenBoard || model.view == nil || !model.view.Fallback || model.view.Number != 0 || load == nil {
		t.Fatalf("fallback selection = screen:%v view:%#v load:%v", model.screen, model.view, load != nil)
	}
	if !saved.Fallback || saved.View != 0 || saved.Project != 7 {
		t.Fatalf("remembered fallback = %#v", saved)
	}

	updated, refresh := model.Update(keyPress("r"))
	model = updated.(Model)
	if refresh == nil || model.screen != screenBoard || !model.loadingDetail {
		t.Fatal("fallback refresh did not reload fallback metadata")
	}
	updated, items := model.Update(refresh())
	model = updated.(Model)
	if items == nil || model.screen != screenBoard || model.view == nil || !model.view.Fallback {
		t.Fatalf("fallback refresh result = screen:%v view:%#v items:%v", model.screen, model.view, items != nil)
	}
}

func TestViewPickerDoesNotOfferFallbackWhenCompatibleBoardExists(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	source := fakeStatusFallbackSource{
		fakePickerSource: fakePickerSource{view: github.View{Name: "Board", Layout: github.BoardLayout, GroupByFields: []github.Field{status}}},
		fallback:         github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true},
	}
	model := newPickerModel(source)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7}
	updated, probe := model.Update(viewsMsg{generation: model.generation, views: []github.ViewSummary{{Number: 1, Name: "Board", Layout: github.BoardLayout}}})
	model = updated.(Model)
	updated, _ = model.Update(probe())
	model = updated.(Model)
	if model.statusFallback != nil || len(model.filteredViews()) != 1 {
		t.Fatalf("fallback offered despite compatible board: %#v %#v", model.statusFallback, model.filteredViews())
	}
}

func TestViewPickerDoesNotTreatMetadataFailureAsNoCompatibleBoard(t *testing.T) {
	source := fakeStatusFallbackSource{
		fakePickerSource: fakePickerSource{
			viewErr: errors.New("network unavailable"),
		},
		fallback: github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true},
	}
	model := newPickerModel(source)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7}
	updated, probe := model.Update(viewsMsg{generation: model.generation, views: []github.ViewSummary{{Number: 1, Name: "Board", Layout: github.BoardLayout}}})
	model = updated.(Model)
	updated, _ = model.Update(probe())
	model = updated.(Model)
	if model.statusFallback != nil || model.boardProbeErr == nil {
		t.Fatalf("fallback offered after failed compatibility read: fallback=%#v err=%v", model.statusFallback, model.boardProbeErr)
	}
}

func TestViewPickerOffersFallbackForTableOnlyProject(t *testing.T) {
	source := fakeStatusFallbackSource{
		fakePickerSource: fakePickerSource{},
		fallback:         github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true},
	}
	model := newPickerModel(source)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7}
	updated, probe := model.Update(viewsMsg{generation: model.generation, views: []github.ViewSummary{{Number: 2, Name: "Table", Layout: github.TableLayout}}})
	model = updated.(Model)
	updated, _ = model.Update(probe())
	model = updated.(Model)
	if model.statusFallback == nil || len(model.filteredViews()) != 2 || !model.filteredViews()[1].Fallback {
		t.Fatalf("table-only project picker entries = %#v fallback=%#v", model.filteredViews(), model.statusFallback)
	}
}

func TestFallbackRefreshBlocksMovesUntilMetadataIsVerified(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}}}
	view := github.View{ProjectID: "project", Name: "Unfiltered Status fallback", Layout: github.BoardLayout, ViewerCanUpdate: true, GroupByFields: []github.Field{status}, Fallback: true}
	mutations := []string{}
	model := newPickerModel(fakeStatusFallbackSource{
		fakePickerSource: fakePickerSource{mutations: &mutations},
		fallbackErr:      errors.New("project access removed"),
	})
	model.screen = screenBoard
	model.view = &view
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 7}
	model.items = []github.Item{{ID: "item", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}}}
	model.boardLane = 1
	updated, refresh := model.Update(keyPress("r"))
	model = updated.(Model)
	if refresh == nil || !model.loadingDetail {
		t.Fatal("fallback refresh did not start metadata verification")
	}
	updated, move := model.Update(keyPress("L"))
	model = updated.(Model)
	if move != nil || !strings.Contains(model.status, "metadata to finish refreshing") {
		t.Fatalf("move allowed during fallback refresh: cmd=%v status=%q", move != nil, model.status)
	}
	updated, _ = model.Update(refresh())
	model = updated.(Model)
	updated, move = model.Update(keyPress("L"))
	model = updated.(Model)
	if move != nil || !strings.Contains(model.status, "metadata could not be refreshed") || len(mutations) != 0 {
		t.Fatalf("move allowed after failed fallback refresh: cmd=%v status=%q calls=%#v", move != nil, model.status, mutations)
	}
}
