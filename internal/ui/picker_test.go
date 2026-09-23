package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakePickerSource struct {
	discovery          github.Discovery
	views              []github.ViewSummary
	view               github.View
	itemPages          map[string]github.ItemsPage
	itemPagesByFilter  map[string]map[string]github.ItemsPage
	itemErrorsByFilter map[string]map[string]error
	detail             github.ItemDetail
	mutations          *[]string
	mutationErr        error
	viewCalls          *int
	itemCalls          *int
}

func (f fakePickerSource) Discover(context.Context) (github.Discovery, error) {
	return f.discovery, nil
}

func (f fakePickerSource) ResolveOwner(_ context.Context, login string) (github.Owner, []github.Project, error) {
	for _, owner := range f.discovery.Owners {
		if owner.Login == login {
			return owner, f.discovery.Projects[login], nil
		}
	}
	return github.Owner{}, nil, errors.New("owner not found")
}

func (f fakePickerSource) ListViews(context.Context, github.Owner, int) ([]github.ViewSummary, error) {
	return f.views, nil
}

func (f fakePickerSource) OpenView(context.Context, github.Owner, int, int) (github.View, error) {
	if f.viewCalls != nil {
		*f.viewCalls++
	}
	return f.view, nil
}

func (f fakePickerSource) PageItems(_ context.Context, _ github.Owner, _ int, filter, after string) (github.ItemsPage, error) {
	if f.itemCalls != nil {
		*f.itemCalls++
	}
	if errorsByCursor, ok := f.itemErrorsByFilter[filter]; ok {
		if err := errorsByCursor[after]; err != nil {
			return github.ItemsPage{}, err
		}
	}
	if pages, ok := f.itemPagesByFilter[filter]; ok {
		return pages[after], nil
	}
	if page, ok := f.itemPages[after]; ok {
		return page, nil
	}
	return github.ItemsPage{}, nil
}

func (f fakePickerSource) LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error) {
	return f.detail, nil
}

func (f fakePickerSource) UpdateItemFieldValue(_ context.Context, request github.ItemFieldValueUpdate) error {
	if f.mutations != nil {
		value := ""
		if request.Value.SingleSelectOptionID != nil {
			value = *request.Value.SingleSelectOptionID
		}
		*f.mutations = append(*f.mutations, "field:"+value)
	}
	return f.mutationErr
}

func (f fakePickerSource) ClearItemFieldValue(_ context.Context, _ github.ItemFieldValueClear) error {
	if f.mutations != nil {
		*f.mutations = append(*f.mutations, "clear")
	}
	return f.mutationErr
}

func (f fakePickerSource) UpdateItemPosition(_ context.Context, request github.ItemPositionUpdate) error {
	if f.mutations != nil {
		anchor := "top"
		if request.AfterID != nil {
			anchor = *request.AfterID
		}
		*f.mutations = append(*f.mutations, "position:"+anchor)
	}
	return f.mutationErr
}

func keyPress(s string) tea.KeyPressMsg {
	if len(s) == 1 {
		return tea.KeyPressMsg(tea.Key{Code: rune(s[0]), Text: s})
	}
	switch s {
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "backspace":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyBackspace})
	default:
		return tea.KeyPressMsg(tea.Key{Code: rune(s[0]), Text: s})
	}
}

func testDiscovery() github.Discovery {
	return github.Discovery{
		Viewer: "me",
		Owners: []github.Owner{
			{Login: "acme", Kind: github.OrganizationOwner},
			{Login: "me", Kind: github.UserOwner},
		},
		Projects: map[string][]github.Project{
			"acme": {{Number: 7, Title: "Website", URL: "https://example.invalid/acme/7"}},
			"me":   {{Number: 1, Title: "Personal"}},
		},
	}
}

func newPickerModel(source DiscoverySource) Model {
	return NewModel(source)
}

func feedDiscovery(t *testing.T, model Model, source fakePickerSource) Model {
	t.Helper()
	msg := discoveryMsg{discovery: source.discovery, generation: model.generation}
	updated, _ := model.Update(msg)
	return updated.(Model)
}

func TestOwnerPickerSelectAdvancesToProjects(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	if model.screen != screenOwnerPicker {
		t.Fatalf("screen = %v", model.screen)
	}
	updated, _ := model.Update(keyPress("enter"))
	result := updated.(Model)
	if result.screen != screenProjectPicker || result.selectedOwner == nil || result.selectedOwner.Login != "acme" {
		t.Fatalf("after enter = %#v", result)
	}
}

func TestPickerFilterNarrowsOwners(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("/"))
	result := updated.(Model)
	if !result.filtering {
		t.Fatal("filter mode not entered")
	}
	updated, _ = result.Update(keyPress("a"))
	result = updated.(Model)
	updated, _ = result.Update(keyPress("c"))
	result = updated.(Model)
	updated, _ = result.Update(keyPress("m"))
	result = updated.(Model)
	if len(result.filteredOwners()) != 1 || result.filteredOwners()[0].Login != "acme" {
		t.Fatalf("filtered = %#v", result.filteredOwners())
	}
}

func TestProjectSelectLoadsViews(t *testing.T) {
	source := fakePickerSource{
		discovery: testDiscovery(),
		views:     []github.ViewSummary{{Number: 3, Name: "Board", Layout: github.BoardLayout}},
	}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("enter"))
	model = updated.(Model)
	updated, cmd := model.Update(keyPress("enter"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected ListViews command")
	}
	msg := cmd()
	updated, _ = model.Update(msg)
	model = updated.(Model)
	if model.screen != screenViewPicker || len(model.views) != 1 {
		t.Fatalf("views state = %#v", model)
	}
}

func TestViewSelectOpensBoard(t *testing.T) {
	source := fakePickerSource{
		discovery: testDiscovery(),
		views:     []github.ViewSummary{{Number: 3, Name: "Board", Layout: github.BoardLayout}},
		view:      github.View{Number: 3, Name: "Board", Layout: github.BoardLayout},
	}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("enter"))
	model = updated.(Model)
	updated, cmd := model.Update(keyPress("enter"))
	model = updated.(Model)
	model, _ = applyCmd(t, model, cmd)
	// Views loaded; select the view.
	updated, cmd = model.Update(keyPress("enter"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("expected OpenView command")
	}
	model, _ = applyCmd(t, model, cmd)
	if model.screen != screenBoard || model.view == nil || model.view.Name != "Board" {
		t.Fatalf("board state = %#v", model)
	}
	content := model.View().Content
	if !strings.Contains(content, "Board") || !strings.Contains(content, "BOARD") {
		t.Fatalf("board view = %q", content)
	}
}

func applyCmd(t *testing.T, model Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return model, nil
	}
	msg := cmd()
	updated, next := model.Update(msg)
	return updated.(Model), next
}

func TestPickerBackNavigation(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("enter"))
	model = updated.(Model)
	if model.screen != screenProjectPicker {
		t.Fatalf("screen = %v", model.screen)
	}
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(Model)
	if model.screen != screenOwnerPicker {
		t.Fatalf("after esc = %v", model.screen)
	}
}

func TestPickerStaleViewsIgnored(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	model.generation = 5
	updated, _ := model.Update(viewsMsg{generation: 4, views: []github.ViewSummary{{Number: 1}}})
	if result := updated.(Model); len(result.views) != 0 {
		t.Fatalf("stale views applied: %#v", result)
	}
	updated, _ = model.Update(viewDetailMsg{generation: 4, view: &github.View{Name: "stale"}})
	if result := updated.(Model); result.view != nil {
		t.Fatalf("stale view applied: %#v", result)
	}
}

func TestPickerHelpAndShortcuts(t *testing.T) {
	source := fakePickerSource{discovery: testDiscovery()}
	model := feedDiscovery(t, newPickerModel(source), source)
	updated, _ := model.Update(keyPress("?"))
	model = updated.(Model)
	if !model.showHelp {
		t.Fatal("help not shown")
	}
	if content := model.View().Content; !strings.Contains(content, "j/k") {
		t.Fatalf("help view = %q", content)
	}
}

func TestBoardLoadsPagesAndGroupsItems(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options: []github.FieldOption{
			{ID: "todo", Name: "Todo"},
			{ID: "done", Name: "Done"},
		},
	}
	view := github.View{Name: "Backlog", Layout: github.BoardLayout, GroupByFields: []github.Field{status}}
	first := github.Item{ID: "one", Content: &github.Content{Kind: "Issue", Number: 42, Title: "Fix the thing"}, FieldValues: []github.FieldValue{{FieldID: "status", FieldName: "Status", OptionID: "todo", Value: "Todo", Available: true}}}
	second := github.Item{ID: "two", Content: &github.Content{Kind: "PullRequest", Number: 8, Title: "Ship the thing"}, FieldValues: []github.FieldValue{{FieldID: "status", FieldName: "Status", OptionID: "done", Value: "Done", Available: true}}}
	source := fakePickerSource{itemPages: map[string]github.ItemsPage{
		"":     {Items: []github.Item{first}, HasNext: true, EndCursor: "next"},
		"next": {Items: []github.Item{second}},
	}}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.width = 120
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}

	cmd := model.startItemsLoad()
	updated, next := model.Update(cmd())
	model = updated.(Model)
	if len(model.items) != 1 || !model.itemsLoading || next == nil {
		t.Fatalf("first page state = %#v, next nil = %v", model, next == nil)
	}
	loading := ansi.Strip(model.View().Content)
	if !strings.Contains(loading, "Fix the thing") || strings.Contains(loading, "Ship the thing") || !strings.Contains(loading, "Loading cards...") || strings.Count(loading, "⠋") < 2 {
		t.Fatalf("partial board did not show lane loading states: %q", loading)
	}
	updated, tick := model.Update(itemsLoadingTickMsg{generation: model.generation})
	model = updated.(Model)
	if tick == nil || model.itemsLoadFrame != 1 || !strings.Contains(ansi.Strip(model.View().Content), "⠙") {
		t.Fatalf("loading spinner did not advance: frame=%d, tick nil=%v", model.itemsLoadFrame, tick == nil)
	}
	updated, next = model.Update(next())
	model = updated.(Model)
	if len(model.items) != 2 || model.itemsLoading || next != nil {
		t.Fatalf("final page state = %#v, next nil = %v", model, next == nil)
	}
	lanes := model.boardLanes()
	if len(lanes) != 3 || len(lanes[0].Items) != 0 || len(lanes[1].Items) != 1 || len(lanes[2].Items) != 1 {
		t.Fatalf("lanes = %#v", lanes)
	}
	if names := []string{lanes[0].Name, lanes[1].Name, lanes[2].Name}; !reflect.DeepEqual(names, []string{"No Status", "Todo", "Done"}) {
		t.Fatalf("lane order = %#v", names)
	}
	content := model.View().Content
	if strings.Contains(content, "Loading cards...") || strings.Contains(content, "⠙") {
		t.Fatalf("loading state remained after final page: %s", content)
	}
	for _, expected := range []string{"Fix the thing", "Ship the thing", "[Todo]", "[Done]"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("board missing %q: %s", expected, content)
		}
	}
}

func TestBoardLoadsStatusLanesInParallel(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options: []github.FieldOption{
			{ID: "todo", Name: "Todo"},
			{ID: "progress", Name: "In progress"},
		},
	}
	view := github.View{Name: "Board", Layout: github.BoardLayout, GroupByFields: []github.Field{status}}
	todo := github.Item{ID: "todo-item", Content: &github.Content{Kind: "Issue", Title: "Todo item"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Value: "Todo", Available: true}}}
	progressOne := github.Item{ID: "progress-1", Content: &github.Content{Kind: "Issue", Title: "Progress one"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "progress", Value: "In progress", Available: true}}}
	progressTwo := github.Item{ID: "progress-2", Content: &github.Content{Kind: "Issue", Title: "Progress two"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "progress", Value: "In progress", Available: true}}}
	other := github.Item{ID: "other", Content: &github.Content{Kind: "Issue", Title: "Archived status"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "archived", Value: "Archived", Available: true}}}
	source := fakePickerSource{itemPagesByFilter: map[string]map[string]github.ItemsPage{
		`status:"Todo"`: {
			"": {Items: []github.Item{todo}},
		},
		`status:"In progress"`: {
			"":              {Items: []github.Item{progressOne}, HasNext: true, EndCursor: "progress-next"},
			"progress-next": {Items: []github.Item{progressTwo}},
		},
		"no:status": {
			"": {},
		},
		`-status:"Todo" -status:"In progress" -no:status`: {
			"": {Items: []github.Item{other}},
		},
	}}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.width = 180
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}

	cmd := model.startItemsLoadWithSpinner()
	message := cmd()
	batch, ok := message.(tea.BatchMsg)
	if !ok || len(batch) != 5 {
		t.Fatalf("parallel load command = %T, commands = %d", message, len(batch))
	}
	updated, _ := model.Update(batch[1]())
	model = updated.(Model)
	loading := ansi.Strip(model.View().Content)
	if !strings.Contains(loading, "Todo item") || !strings.Contains(loading, "Loading cards...") || !model.itemsLoading {
		t.Fatalf("completed lane was not revealed independently: %q", loading)
	}

	updated, _ = model.Update(batch[2]())
	model = updated.(Model)
	updated, _ = model.Update(batch[3]())
	model = updated.(Model)
	updated, _ = model.Update(batch[4]())
	model = updated.(Model)
	if model.itemsLoading || len(model.items) != 4 {
		t.Fatalf("parallel load final state = %#v", model)
	}
	content := ansi.Strip(model.View().Content)
	for _, title := range []string{"Todo item", "Progress one", "Progress two"} {
		if !strings.Contains(content, title) {
			t.Fatalf("parallel board missing %q: %s", title, content)
		}
	}
	lanes := model.boardLanes()
	if lanes[len(lanes)-1].Name != "Other" || len(lanes[len(lanes)-1].Items) != 1 || lanes[len(lanes)-1].Items[0].ID != "other" {
		t.Fatalf("unknown status lane = %#v", lanes)
	}
}

func TestSavedFilteredStatusBoardPagesUnchangedQuery(t *testing.T) {
	const filter = `status:"Done" assignee:@me`
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}}}
	itemCalls := 0
	source := fakePickerSource{itemCalls: &itemCalls, itemPagesByFilter: map[string]map[string]github.ItemsPage{
		filter: {
			"":     {Items: []github.Item{{ID: "one", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "done", Available: true}}}}, HasNext: true, EndCursor: "next"},
			"next": {Items: []github.Item{{ID: "two", FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "done", Available: true}}}}},
		},
	}}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &github.View{Layout: github.BoardLayout, Filter: filter, GroupByFields: []github.Field{status}}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	batch := model.startItemsLoadWithSpinner()().(tea.BatchMsg)
	if len(batch) != 2 || model.itemsLanePending != 0 || model.itemsLoadingLanes != nil {
		t.Fatalf("saved filter used lane queries: commands=%d pending=%d", len(batch), model.itemsLanePending)
	}
	updated, next := model.Update(batch[1]())
	model = updated.(Model)
	if next == nil || !model.itemsLoading || len(model.items) != 1 {
		t.Fatalf("first filtered page = items:%d loading:%v next:%v", len(model.items), model.itemsLoading, next != nil)
	}
	updated, _ = model.Update(next())
	model = updated.(Model)
	if itemCalls != 2 || model.itemsLoading || len(model.items) != 2 || len(model.boardLanes()[2].Items) != 2 {
		t.Fatalf("filtered pages = calls:%d items:%d loading:%v", itemCalls, len(model.items), model.itemsLoading)
	}
}

func TestParallelLaneFailureRemainsVisibleAsIncomplete(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	view := github.View{Name: "Board", Layout: github.BoardLayout, GroupByFields: []github.Field{status}}
	partial := github.Item{ID: "partial", Content: &github.Content{Kind: "Issue", Title: "Partial item"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Value: "Todo", Available: true}}}
	source := fakePickerSource{
		itemPagesByFilter: map[string]map[string]github.ItemsPage{
			`status:"Todo"`:             {"": {Items: []github.Item{partial}, HasNext: true, EndCursor: "next"}},
			"no:status":                 {"": {}},
			`-status:"Todo" -no:status`: {"": {}},
		},
		itemErrorsByFilter: map[string]map[string]error{
			`status:"Todo"`: {"next": errors.New("page failed")},
		},
	}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}

	batch := model.startItemsLoadWithSpinner()().(tea.BatchMsg)
	for _, command := range batch[1:] {
		updated, _ := model.Update(command())
		model = updated.(Model)
	}
	content := ansi.Strip(model.View().Content)
	if model.itemsLoading || !strings.Contains(content, "Partial item") || !strings.Contains(content, "incomplete; press r") || !strings.Contains(content, "Lane loading failed: Todo: page failed") {
		t.Fatalf("failed lane state = %#v\n%s", model, content)
	}
}

func TestLeavingBoardCancelsLaneLoading(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Name: "Board", GroupByFields: []github.Field{status}}
	model.views = []github.ViewSummary{{Number: 1, Name: "Board"}}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	_ = model.startItemsLoadWithSpinner()
	oldContext := model.ctx
	oldGeneration := model.generation

	updated, _ := model.Update(keyPress("esc"))
	model = updated.(Model)
	if model.screen != screenViewPicker || model.itemsLoading {
		t.Fatalf("board exit state = %#v", model)
	}
	select {
	case <-oldContext.Done():
	default:
		t.Fatal("leaving the board did not cancel lane requests")
	}
	updated, next := model.Update(itemsLoadingTickMsg{generation: oldGeneration})
	if next != nil || updated.(Model).itemsLoadFrame != model.itemsLoadFrame {
		t.Fatal("stale loading tick continued after leaving the board")
	}
}

func TestBoardLaneAndCardNavigation(t *testing.T) {
	status := github.Field{ID: "status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}}
	view := github.View{Name: "Board", Layout: github.BoardLayout, GroupByFields: []github.Field{status}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "b", Content: &github.Content{Kind: "Issue", Title: "B"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "c", Content: &github.Content{Kind: "Issue", Title: "C"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "two", Available: true}}},
	}
	model.boardLane = 1
	updated, _ := model.Update(keyPress("j"))
	model = updated.(Model)
	if model.boardCard != 1 {
		t.Fatalf("card cursor = %d", model.boardCard)
	}
	updated, _ = model.Update(keyPress("l"))
	model = updated.(Model)
	if model.boardLane != 2 || model.boardCard != 0 {
		t.Fatalf("lane cursor = %d, card cursor = %d", model.boardLane, model.boardCard)
	}
	updated, _ = model.Update(keyPress("k"))
	model = updated.(Model)
	if model.boardCard != 0 {
		t.Fatalf("card cursor after clamp = %d", model.boardCard)
	}
}

func TestWritableBoardMoveUpdatesFieldAndAppendsDestination(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
	}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{status}}
	mutations := []string{}
	model := newPickerModel(fakePickerSource{mutations: &mutations})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "b", Content: &github.Content{Kind: "Issue", Title: "B"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "two", Available: true}}},
	}
	model.boardLane = 1

	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if cmd == nil || !model.mutationLoading {
		t.Fatalf("move state = %#v, cmd nil = %v", model, cmd == nil)
	}
	if len(mutations) != 0 || model.boardLane != 2 || model.boardCard != 1 {
		t.Fatalf("optimistic move state = lane %d card %d mutations %#v", model.boardLane, model.boardCard, mutations)
	}
	if lanes := model.boardLanes(); len(lanes[2].Items) != 2 || lanes[2].Items[1].ID != "a" || lanes[2].Items[1].FieldValues[0].OptionID != "two" {
		t.Fatalf("optimistic destination lane = %#v", lanes[2])
	}
	message, ok := cmd().(boardMutationMsg)
	if !ok || message.err != nil {
		t.Fatalf("move message = %#v, type ok = %v", message, ok)
	}
	if !reflect.DeepEqual(mutations, []string{"field:two", "position:b"}) {
		t.Fatalf("mutation calls = %#v", mutations)
	}
}

func TestWritableVerticalBoardMoveUpdatesFieldAndAppendsDestination(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
	}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, VerticalGroupBy: []github.Field{status}}
	mutations := []string{}
	model := newPickerModel(fakePickerSource{mutations: &mutations})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "b", Content: &github.Content{Kind: "Issue", Title: "B"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "two", Available: true}}},
	}
	model.boardLane = 1

	updated, cmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if cmd == nil || !model.mutationLoading {
		t.Fatalf("vertical move state = %#v, cmd nil = %v", model, cmd == nil)
	}
	message, ok := cmd().(boardMutationMsg)
	if !ok || message.err != nil {
		t.Fatalf("vertical move message = %#v, type ok = %v", message, ok)
	}
	if !reflect.DeepEqual(mutations, []string{"field:two", "position:b"}) {
		t.Fatalf("vertical mutation calls = %#v", mutations)
	}
}

func TestSuccessfulMutationKeepsOptimisticStateWithoutReloading(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
	}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{status}}
	mutations := []string{}
	viewCalls := 0
	itemCalls := 0
	source := fakePickerSource{
		mutations: &mutations,
		viewCalls: &viewCalls,
		itemCalls: &itemCalls,
	}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	model.items = []github.Item{{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}}}
	model.boardLane = 1

	updated, mutationCmd := model.Update(keyPress("L"))
	model = updated.(Model)
	mutationMessage := mutationCmd().(boardMutationMsg)
	updated, next := model.Update(mutationMessage)
	model = updated.(Model)
	if next != nil || model.itemsLoading {
		t.Fatalf("successful mutation triggered refresh: state=%#v, next nil=%v", model, next == nil)
	}
	if viewCalls != 0 || model.view != &view {
		t.Fatalf("mutation reloaded view: calls=%d view=%p want=%p", viewCalls, model.view, &view)
	}
	if itemCalls != 0 || model.status != "Move card saved" || model.pendingStatus != "" || model.mutationLoading {
		t.Fatalf("successful mutation state = items %d status %q pending %q loading=%v", itemCalls, model.status, model.pendingStatus, model.mutationLoading)
	}
}

func TestFailedMutationRollsBackOptimisticMove(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}},
	}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{status}}
	model := newPickerModel(fakePickerSource{mutationErr: errors.New("permission denied")})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "b", Content: &github.Content{Kind: "Issue", Title: "B"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "two", Available: true}}},
	}
	model.boardLane = 1

	updated, mutationCmd := model.Update(keyPress("L"))
	model = updated.(Model)
	if model.boardLane != 2 || len(model.items) != 2 || model.items[1].ID != "a" || model.items[1].FieldValues[0].OptionID != "two" {
		t.Fatalf("optimistic state was not applied: lane=%d items=%#v", model.boardLane, model.items)
	}
	message := mutationCmd().(boardMutationMsg)
	updated, _ = model.Update(message)
	model = updated.(Model)
	if model.boardLane != 1 || model.boardCard != 0 || model.items[0].FieldValues[0].OptionID != "one" || model.optimisticRollback != nil {
		t.Fatalf("rollback state = lane %d card %d items %#v rollback=%#v", model.boardLane, model.boardCard, model.items, model.optimisticRollback)
	}
	if !strings.Contains(model.status, "Move card failed") {
		t.Fatalf("rollback status = %q", model.status)
	}
}

func TestCombinedSwimlaneBoardDoesNotOfferMutations(t *testing.T) {
	priority := github.Field{ID: "priority", Name: "Priority", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "p0", Name: "P0"}, {ID: "p1", Name: "P1"}}}
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}}}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{priority}, VerticalGroupBy: []github.Field{status}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{{ID: "item", Content: &github.Content{Kind: "Issue", Title: "Item"}}}

	updated, cmd := model.Update(keyPress("L"))
	result := updated.(Model)
	if cmd != nil || result.mutationLoading || !strings.Contains(result.status, "swimlane") {
		t.Fatalf("combined mutation state = %#v, cmd nil = %v", result, cmd == nil)
	}
}

func TestSavedFilteredBoardDoesNotOfferMutations(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}}}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, Filter: "iteration:@current", GroupByFields: []github.Field{status}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{{ID: "item", Content: &github.Content{Kind: "Issue", Title: "Item"}}}

	updated, cmd := model.Update(keyPress("L"))
	result := updated.(Model)
	if cmd != nil || result.mutationLoading || !strings.Contains(result.status, "filtered") {
		t.Fatalf("filtered mutation state = %#v, cmd nil = %v", result, cmd == nil)
	}
}

func TestWritableBoardReorderUsesVisibleAnchors(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "one", Name: "One"}}}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{status}}
	mutations := []string{}
	model := newPickerModel(fakePickerSource{mutations: &mutations})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "A"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "b", Content: &github.Content{Kind: "Issue", Title: "B"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "c", Content: &github.Content{Kind: "Issue", Title: "C"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
	}
	model.boardLane = 1
	model.boardCard = 1
	updated, cmd := model.Update(keyPress("J"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("downward reorder did not start")
	}
	_ = cmd()
	if !reflect.DeepEqual(mutations, []string{"position:c"}) {
		t.Fatalf("downward reorder calls = %#v", mutations)
	}

	mutations = nil
	model.mutationLoading = false
	model.boardCard = 2
	updated, cmd = model.Update(keyPress("K"))
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("upward reorder did not start")
	}
	_ = cmd()
	if !reflect.DeepEqual(mutations, []string{"position:a"}) {
		t.Fatalf("upward reorder calls = %#v", mutations)
	}
}

func TestWritableBoardReorderMovesSecondCardAboveFirst(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", Kind: "ProjectV2SingleSelectField", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "one", Name: "One"}}}
	view := github.View{ProjectID: "project", ViewerCanUpdate: true, GroupByFields: []github.Field{status}}
	mutations := []string{}
	model := newPickerModel(fakePickerSource{mutations: &mutations})
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{
		{ID: "anchor", Content: &github.Content{Kind: "Issue", Title: "Anchor"}},
		{ID: "first", Content: &github.Content{Kind: "Issue", Title: "First"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
		{ID: "second", Content: &github.Content{Kind: "Issue", Title: "Second"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "one", Available: true}}},
	}
	model.boardLane = 1
	model.boardCard = 1

	updated, cmd := model.Update(keyPress("K"))
	model = updated.(Model)
	if cmd == nil || model.boardCard != 0 {
		t.Fatalf("second-card move state = lane %d card %d cmd nil=%v", model.boardLane, model.boardCard, cmd == nil)
	}
	if lanes := model.boardLanes(); len(lanes[1].Items) != 2 || lanes[1].Items[0].ID != "second" || lanes[1].Items[1].ID != "first" {
		t.Fatalf("optimistic lane order = %#v", lanes[1].Items)
	}
	_ = cmd()
	if !reflect.DeepEqual(mutations, []string{"position:anchor"}) {
		t.Fatalf("second-card mutation = %#v", mutations)
	}
}

func TestBoardAppliesSavedFieldSortAndPreservesPositionTies(t *testing.T) {
	priority := github.Field{
		ID:       "priority",
		Name:     "Priority",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}},
	}
	view := github.View{SortByFields: []github.SortField{{Direction: "ASC", Field: priority}}}
	items := []github.Item{
		{ID: "high", Content: &github.Content{Kind: "Issue", Title: "High"}, FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "high", Value: "High", Available: true}}},
		{ID: "unset", Content: &github.Content{Kind: "Issue", Title: "Unset"}},
		{ID: "low", Content: &github.Content{Kind: "Issue", Title: "Low"}, FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "low", Value: "Low", Available: true}}},
		{ID: "unset-2", Content: &github.Content{Kind: "Issue", Title: "Unset again"}},
	}

	lanes := lanesForView(&view, sortedItemsForView(&view, items))
	if got := []string{lanes[0].Items[0].ID, lanes[0].Items[1].ID, lanes[0].Items[2].ID, lanes[0].Items[3].ID}; !reflect.DeepEqual(got, []string{"high", "low", "unset", "unset-2"}) {
		t.Fatalf("sorted items = %#v", got)
	}
	view.SortByFields[0].Direction = "DESC"
	lanes = lanesForView(&view, sortedItemsForView(&view, items))
	if got := []string{lanes[0].Items[0].ID, lanes[0].Items[1].ID, lanes[0].Items[2].ID, lanes[0].Items[3].ID}; !reflect.DeepEqual(got, []string{"low", "high", "unset", "unset-2"}) {
		t.Fatalf("descending sorted items = %#v", got)
	}
}

func TestBoardSavedSortKeepsUnsetValuesLastInBothDirectionsAndPreservesTies(t *testing.T) {
	field := github.Field{ID: "priority", Name: "Priority", DataType: "NUMBER"}
	items := []github.Item{
		{ID: "first-tie", FieldValues: []github.FieldValue{{FieldID: "priority", Value: "2", Available: true}}},
		{ID: "unset-a"},
		{ID: "second-tie", FieldValues: []github.FieldValue{{FieldID: "priority", Value: "2", Available: true}}},
		{ID: "lower", FieldValues: []github.FieldValue{{FieldID: "priority", Value: "1", Available: true}}},
		{ID: "unset-b"},
	}
	view := github.View{SortByFields: []github.SortField{{Direction: "ASC", Field: field}}}
	assertOrder := func(want ...string) {
		t.Helper()
		got := sortedItemsForView(&view, items)
		ids := make([]string, len(got))
		for index := range got {
			ids[index] = got[index].ID
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("sorted IDs = %#v, want %#v", ids, want)
		}
	}
	assertOrder("lower", "first-tie", "second-tie", "unset-a", "unset-b")
	view.SortByFields[0].Direction = "DESC"
	assertOrder("first-tie", "second-tie", "lower", "unset-a", "unset-b")
}

func TestBoardSortsIterationValuesByChronologicalStartDate(t *testing.T) {
	iteration := github.Field{
		ID: "iteration", Name: "Iteration", Kind: "ProjectV2IterationField", DataType: "ITERATION",
		Iterations: []github.Iteration{
			{ID: "current", Title: "Current", StartDate: "2026-09-22"},
			{ID: "past", Title: "Past", StartDate: "2026-09-15", Completed: true},
		},
	}
	items := []github.Item{
		{ID: "current", FieldValues: []github.FieldValue{{FieldID: "iteration", IterationID: "current", Value: "Current", Available: true}}},
		{ID: "past", FieldValues: []github.FieldValue{{FieldID: "iteration", IterationID: "past", Value: "Past", Available: true}}},
		{ID: "unset"},
	}
	view := github.View{SortByFields: []github.SortField{{Direction: "ASC", Field: iteration}}}
	assertOrder := func(want ...string) {
		t.Helper()
		got := sortedItemsForView(&view, items)
		ids := make([]string, len(got))
		for index := range got {
			ids[index] = got[index].ID
		}
		if !reflect.DeepEqual(ids, want) {
			t.Fatalf("sorted IDs = %#v, want %#v", ids, want)
		}
	}
	assertOrder("past", "current", "unset")
	view.SortByFields[0].Direction = "DESC"
	assertOrder("current", "past", "unset")
}

func TestBoardSearchFiltersLoadedCards(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Name: "Board", Layout: github.BoardLayout}
	model.items = []github.Item{
		{ID: "alpha", Content: &github.Content{Kind: "Issue", Title: "Fix alpha"}},
		{ID: "beta", Content: &github.Content{Kind: "Issue", Title: "Fix beta"}},
	}

	updated, _ := model.Update(keyPress("/"))
	model = updated.(Model)
	for _, key := range []string{"a", "l", "p", "h", "a"} {
		updated, _ = model.Update(keyPress(key))
		model = updated.(Model)
	}
	updated, _ = model.Update(keyPress("enter"))
	model = updated.(Model)

	lanes := model.boardLanes()
	if len(lanes) != 1 || len(lanes[0].Items) != 1 || lanes[0].Items[0].ID != "alpha" {
		t.Fatalf("filtered lanes = %#v", lanes)
	}
	if len(model.items) != 2 || model.filtering || model.filter != "alpha" {
		t.Fatalf("search state = %#v", model)
	}
	content := ansi.Strip(model.View().Content)
	if !strings.Contains(content, "Search: alpha") || !strings.Contains(content, "Items: 1/2") || strings.Contains(content, "Fix beta") {
		t.Fatalf("search rendering = %q", content)
	}
}

func TestBoardFocusFollowsItemAcrossPages(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Name: "Board", Layout: github.BoardLayout}
	model.items = []github.Item{{ID: "focused", Content: &github.Content{Kind: "Issue", Title: "Focused"}}}
	model.clampBoardCursor()
	if model.boardFocusID != "focused" {
		t.Fatalf("initial focus = %q", model.boardFocusID)
	}

	model.items = []github.Item{
		{ID: "new", Content: &github.Content{Kind: "Issue", Title: "New item"}},
		{ID: "focused", Content: &github.Content{Kind: "Issue", Title: "Focused"}},
	}
	model.clampBoardCursor()
	selected, ok := model.selectedBoardItem()
	if !ok || selected.ID != "focused" || model.boardCard != 1 {
		t.Fatalf("focus after page arrival = item=%#v lane=%d card=%d", selected, model.boardLane, model.boardCard)
	}
}

func TestBoardCardsShowMetadataAndClampTitles(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	cases := []struct {
		name     string
		content  *github.Content
		want     []string
		selected bool
	}{
		{
			name:    "open issue",
			content: &github.Content{Kind: "Issue", Number: 42, Title: "Fix the project card", Repository: "example", State: "OPEN", SubIssueTotal: 8, SubIssueDone: 3},
			want:    []string{"○ example #42", "Fix the project card", "Sub-issues: 3/8 (38%)"},
		},
		{
			name:    "closed issue",
			content: &github.Content{Kind: "Issue", Number: 43, Title: "Close the project card", Repository: "example", State: "CLOSED"},
			want:    []string{"✓ example #43"},
		},
		{
			name:    "open pull request",
			content: &github.Content{Kind: "PullRequest", Number: 6, Title: "Review the project card", Repository: "example", State: "OPEN"},
			want:    []string{"↗ example #6"},
		},
		{
			name:    "merged pull request",
			content: &github.Content{Kind: "PullRequest", Number: 7, Title: "Merge the project card", Repository: "example", State: "MERGED", Merged: true},
			want:    []string{"↔ example #7"},
		},
		{
			name:    "draft pull request",
			content: &github.Content{Kind: "PullRequest", Number: 8, Title: "Work in progress", Repository: "example", State: "OPEN", IsDraft: true},
			want:    []string{"◌ example #8"},
		},
		{
			name:    "missing metadata",
			content: &github.Content{Kind: "Issue", Number: 9, Title: "Fallback identity"},
			want:    []string{"○ #9"},
		},
		{
			name:     "selected",
			content:  &github.Content{Kind: "Issue", Number: 10, Title: "Selected card"},
			want:     []string{"> ○ #10"},
			selected: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			card := ansi.Strip(model.renderLaneCard(github.Item{Content: tc.content}, tc.selected, 40))
			for _, expected := range tc.want {
				if !strings.Contains(card, expected) {
					t.Fatalf("card missing %q: %q", expected, card)
				}
			}
		})
	}

	longTitle := "A title with enough words to continue beyond the second visible card line"
	card := ansi.Strip(model.renderLaneCard(github.Item{Content: &github.Content{Kind: "Issue", Title: longTitle}}, false, 24))
	if strings.Contains(card, "second visible card line") || !strings.Contains(card, "...") {
		t.Fatalf("title was not clamped to two lines: %q", card)
	}
}

func TestBoardCardsShowSavedVisibleFields(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	view := &github.View{Fields: []github.Field{
		{Name: "Title", DataType: "TITLE"},
		{ID: "status", Name: "Status", DataType: "SINGLE_SELECT"},
		{ID: "priority", Name: "Priority", DataType: "SINGLE_SELECT"},
		{ID: "labels", Name: "Labels", DataType: "MULTI_SELECT"},
	}}
	item := github.Item{
		Content: &github.Content{Kind: "Issue", Title: "Visible fields"},
		FieldValues: []github.FieldValue{
			{FieldID: "status", FieldName: "Status", Value: "In progress", Available: true},
			{FieldID: "priority", FieldName: "Priority", Value: "High", Available: true},
			{FieldID: "labels", FieldName: "Labels", Available: false},
		},
	}
	model.view = view
	card := ansi.Strip(model.renderLaneCard(item, false, 60))
	for _, expected := range []string{"Status: In progress", "Priority: High"} {
		if !strings.Contains(card, expected) {
			t.Fatalf("card missing saved field %q: %q", expected, card)
		}
	}
	if strings.Contains(card, "Labels") {
		t.Fatalf("card rendered unavailable field: %q", card)
	}
}

func TestBoardCardsOmitGroupingFieldsFromInlineSummary(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT"}
	assignees := github.Field{ID: "assignees", Name: "Assignees", DataType: "ASSIGNEES"}
	view := &github.View{
		Fields:          []github.Field{{Name: "Title", DataType: "TITLE"}, status, assignees},
		VerticalGroupBy: []github.Field{status},
	}
	item := github.Item{
		Content: &github.Content{Kind: "Issue", Title: "Grouped card"},
		FieldValues: []github.FieldValue{
			{FieldID: "status", FieldName: "Status", Value: "In Progress", Available: true},
			{FieldID: "assignees", FieldName: "Assignees", Value: "octocat", Available: true},
		},
	}
	card := ansi.Strip((&Model{view: view}).renderLaneCard(item, false, 60))
	if strings.Contains(card, "Status: In Progress") || !strings.Contains(card, "Assignees: octocat") {
		t.Fatalf("grouping summary = %q", card)
	}
	if fields := cardFieldSummary(item, &github.View{Fields: view.Fields}); !strings.Contains(fields, "Status: In Progress") {
		t.Fatalf("ungrouped field summary omitted status: %q", fields)
	}
}

func TestBoardUsesVerticalGroupingWhenColumnsAreUnset(t *testing.T) {
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "todo", Name: "Todo"}},
	}
	view := github.View{VerticalGroupBy: []github.Field{status}}
	item := github.Item{Content: &github.Content{Kind: "Issue", Title: "Vertical item"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}}
	lanes := lanesForView(&view, []github.Item{item})
	if len(lanes) != 2 || lanes[0].Name != "No Status" || len(lanes[0].Items) != 0 || lanes[1].Name != "Todo" || len(lanes[1].Items) != 1 {
		t.Fatalf("vertical lanes = %#v", lanes)
	}
}

func TestBoardProjectsCombinedGroupingIntoSwimlaneRows(t *testing.T) {
	priority := github.Field{
		ID:       "priority",
		Name:     "Priority",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "p0", Name: "P0"}, {ID: "p1", Name: "P1"}},
	}
	status := github.Field{
		ID:       "status",
		Name:     "Status",
		Kind:     "ProjectV2SingleSelectField",
		DataType: "SINGLE_SELECT",
		Options:  []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "done", Name: "Done"}},
	}
	view := github.View{GroupByFields: []github.Field{priority}, VerticalGroupBy: []github.Field{status}}
	items := []github.Item{
		{ID: "todo-p0", Content: &github.Content{Kind: "Issue", Title: "Todo P0"}, FieldValues: []github.FieldValue{
			{FieldID: "priority", OptionID: "p0", Available: true},
			{FieldID: "status", OptionID: "todo", Available: true},
		}},
		{ID: "done-p1", Content: &github.Content{Kind: "Issue", Title: "Done P1"}, FieldValues: []github.FieldValue{
			{FieldID: "priority", OptionID: "p1", Available: true},
			{FieldID: "status", OptionID: "done", Available: true},
		}},
		{ID: "todo-unset", Content: &github.Content{Kind: "Issue", Title: "Todo unset"}, FieldValues: []github.FieldValue{
			{FieldID: "status", OptionID: "todo", Available: true},
		}},
	}

	lanes := lanesForView(&view, items)
	if len(lanes) != 9 {
		t.Fatalf("combined lanes = %d, want 9: %#v", len(lanes), lanes)
	}
	if lanes[4].RowName != "Todo" || lanes[4].Name != "P0" || lanes[4].Items[0].ID != "todo-p0" {
		t.Fatalf("first populated combined lane = %#v", lanes[4])
	}
	if lanes[8].RowName != "Done" || lanes[8].Name != "P1" || lanes[8].Items[0].ID != "done-p1" {
		t.Fatalf("second populated combined lane = %#v", lanes[8])
	}
	if lanes[3].Name != "No value" || lanes[3].Items[0].ID != "todo-unset" {
		t.Fatalf("unset column lane = %#v", lanes[3])
	}

	model := newPickerModel(fakePickerSource{})
	model.view = &view
	model.width = 180
	model.items = items
	rendered := ansi.Strip(model.renderLaneGrid(lanes))
	for _, expected := range []string{"Swimlane: Todo", "Swimlane: Done", "Todo P0", "Done P1"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("combined board missing %q: %q", expected, rendered)
		}
	}
}

func TestBoardViewportKeepsFocusedCardVisible(t *testing.T) {
	view := github.View{
		Name:   "Board",
		Layout: github.BoardLayout,
		Fields: []github.Field{{ID: "status", Name: "Status"}},
	}
	model := newPickerModel(fakePickerSource{})
	model.view = &view
	model.height = 24
	lane := boardLane{Name: "All items"}
	for i := 0; i < 20; i++ {
		lane.Items = append(lane.Items, github.Item{
			Content:     &github.Content{Kind: "Issue", Number: i + 1, Title: "Card"},
			FieldValues: []github.FieldValue{{FieldID: "status", Value: "Todo", Available: true}},
		})
	}
	for _, selected := range []int{0, 5, 19} {
		column := model.renderLaneColumn(lane, 0, 24, model.boardViewportHeight())
		start, end := model.boardWindowForLane(lane, selected, 24, model.boardViewportHeight(), true)
		if selected < start || selected >= end {
			t.Fatalf("selected card %d outside window [%d,%d)", selected, start, end)
		}
		if lipgloss.Height(column) > model.boardViewportHeight() {
			t.Fatalf("column height = %d, viewport = %d", lipgloss.Height(column), model.boardViewportHeight())
		}
		model.boardCard = selected
		column = model.renderLaneColumn(lane, 0, 24, model.boardViewportHeight())
		if !strings.Contains(column, "> ○ #") {
			t.Fatalf("focused card marker missing for %d: %q", selected, column)
		}
	}
	model.items = lane.Items
	model.boardCard = 19
	if height := lipgloss.Height(model.View().Content); height > model.height {
		t.Fatalf("full board height = %d, terminal height = %d", height, model.height)
	}
}

func TestEnterLoadsAndShowsItemDetail(t *testing.T) {
	item := github.Item{ID: "item-1", Content: &github.Content{Kind: "Issue", Number: 42, Title: "Fix the thing"}}
	view := github.View{Name: "Board", Layout: github.BoardLayout}
	source := fakePickerSource{detail: github.ItemDetail{
		ID:      "item-1",
		Content: &github.Content{Kind: "Issue", Number: 42, Title: "Fix the thing", Body: "# Details\n\n- one", BodyAvailable: true},
		Fields: []github.DetailField{
			{Field: github.Field{ID: "status", Name: "Status"}},
			{Field: github.Field{ID: "priority", Name: "Priority"}, Value: &github.FieldValue{FieldID: "priority", Value: "High", Available: true}},
			{Field: github.Field{ID: "labels", Name: "Labels"}, Value: &github.FieldValue{FieldID: "labels", Available: false}},
		},
	}}
	model := newPickerModel(source)
	model.screen = screenBoard
	model.view = &view
	model.items = []github.Item{item}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}

	updated, cmd := model.Update(keyPress("enter"))
	model = updated.(Model)
	if cmd == nil || !model.detailVisible || !model.detailLoading {
		t.Fatalf("detail loading state = %#v, cmd nil = %v", model, cmd == nil)
	}
	updated, detailCmd := model.Update(cmd())
	model = updated.(Model)
	if detailCmd == nil {
		t.Fatal("debounced detail load did not start")
	}
	updated, _ = model.Update(detailCmd())
	model = updated.(Model)
	if model.detailLoading || model.detail == nil || model.detailErr != nil {
		t.Fatalf("detail result = %#v", model)
	}
	content := model.View().Content
	for _, expected := range []string{"Fix the thing", "Properties", "Priority: High", "Details", "one"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("detail missing %q: %s", expected, content)
		}
	}
	panel := model.renderDetailPanel()
	if strings.Contains(panel, "Status: No value") {
		t.Fatalf("unset fields should be hidden by default: %s", panel)
	}
	if strings.Contains(panel, "Labels: Unavailable") {
		t.Fatalf("unavailable fields should be hidden by default: %s", panel)
	}
	updated, _ = model.Update(keyPress("f"))
	model = updated.(Model)
	if !strings.Contains(model.renderDetailPanel(), "Status: No value") || !strings.Contains(model.renderDetailPanel(), "Labels: Unavailable") {
		t.Fatalf("all-fields toggle did not show unset fields: %s", model.renderDetailPanel())
	}
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(Model)
	if model.detailVisible {
		t.Fatal("detail overlay did not close")
	}
	updated, cmd = model.Update(keyPress("enter"))
	model = updated.(Model)
	if cmd != nil || model.detailLoading || model.detail == nil || !model.detailVisible {
		t.Fatalf("cached detail was not used: loading=%v detail=%#v cmd nil=%v", model.detailLoading, model.detail, cmd == nil)
	}
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(Model)
}

func TestClosingDetailInvalidatesPendingDebounceAndResponse(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Name: "Board", Layout: github.BoardLayout}
	model.items = []github.Item{{ID: "item-1", Content: &github.Content{Kind: "Issue", Title: "One"}}, {ID: "item-2", Content: &github.Content{Kind: "Issue", Title: "Two"}}}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	updated, pending := model.Update(keyPress("enter"))
	model = updated.(Model)
	debounceMsg := pending().(itemDetailDebounceMsg)
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(Model)
	updated, detailCmd := model.Update(debounceMsg)
	model = updated.(Model)
	if detailCmd != nil || model.detailLoading || model.detailVisible {
		t.Fatal("closing detail did not invalidate pending debounce")
	}
	model.moveBoardCard(1)
	updated, _ = model.Update(keyPress("enter"))
	model = updated.(Model)
	updated, _ = model.Update(itemDetailMsg{itemID: "item-1", requestID: debounceMsg.requestID, detail: &github.ItemDetail{ID: "item-1"}, generation: model.generation})
	result := updated.(Model)
	if result.detail != nil || !result.detailLoading || result.detailItemID != "item-2" {
		t.Fatalf("stale item detail was applied: id=%q loading=%v detail=%#v", result.detailItemID, result.detailLoading, result.detail)
	}
}

func TestDetailCacheClearsWhenBoardReloads(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.view = &github.View{Name: "Board", Layout: github.BoardLayout}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	model.detailCache["item-1"] = github.ItemDetail{ID: "item-1"}

	if cmd := model.startItemsLoad(); cmd == nil {
		t.Fatal("board reload did not return an item command")
	}
	if model.detailCache != nil {
		t.Fatalf("detail cache survived board reload: %#v", model.detailCache)
	}
}

func TestDetailPopoverExpandsOnWideTerminal(t *testing.T) {
	model := NewModel(nil)
	model.width = 120
	if model.detailPanelWidth() < 90 {
		t.Fatalf("popover content width = %d", model.detailPanelWidth())
	}
}

func TestDetailPanelReachesBottomAfterViewportMarkerAdjustment(t *testing.T) {
	model := NewModel(nil)
	model.width = 80
	model.height = 24
	model.screen = screenBoard
	model.detailVisible = true
	model.detail = &github.ItemDetail{
		Content: &github.Content{
			Kind:          "Issue",
			Title:         "Long detail",
			Body:          strings.Repeat("detail line\n", 40) + "last detail line",
			BodyAvailable: true,
		},
	}
	lines := detailLines(*model.detail, model.detailPanelWidth(), false)
	model.detailOffset = len(lines)

	panel := ansi.Strip(model.renderDetailPanel())
	if strings.Contains(panel, "... scroll down for more") {
		t.Fatalf("bottom panel still advertises more content: %q", panel)
	}
	if !strings.Contains(panel, "last detail line") {
		t.Fatalf("bottom panel omitted final detail line: %q", panel)
	}

	model.detailOffset = 0
	maxOffset := model.detailMaxOffset()
	for i := 0; i < maxOffset+3; i++ {
		updated, _ := model.Update(keyPress("j"))
		model = updated.(Model)
	}
	if model.detailOffset != maxOffset {
		t.Fatalf("detail offset passed bottom: got %d, want %d", model.detailOffset, maxOffset)
	}
	updated, _ := model.Update(keyPress("k"))
	model = updated.(Model)
	if model.detailOffset != maxOffset-1 {
		t.Fatalf("detail offset did not move up immediately: got %d, want %d", model.detailOffset, maxOffset-1)
	}
}

func TestMarkdownRendererWrapsGitHubBody(t *testing.T) {
	lines := renderMarkdown("A paragraph with enough words to require wrapping in the terminal detail popover.", 24)
	if len(lines) < 2 {
		t.Fatalf("markdown was not wrapped: %#v", lines)
	}
	joined := strings.Join(strings.Fields(ansi.Strip(strings.Join(lines, " "))), " ")
	if !strings.Contains(joined, "terminal detail popover") {
		t.Fatalf("markdown content was lost: %q", joined)
	}
}

func TestStaleItemDetailIsIgnored(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.detailVisible = true
	model.detailItemID = "current"
	model.generation = 4
	updated, _ := model.Update(itemDetailMsg{itemID: "old", detail: &github.ItemDetail{ID: "old"}, generation: 4})
	result := updated.(Model)
	if result.detail != nil || result.detailLoading {
		t.Fatalf("stale detail applied: %#v", result)
	}
}

func TestPopoverCentersDetailWithoutChangingCanvas(t *testing.T) {
	base := strings.Repeat(".", 40) + "\n" + strings.Repeat(".", 40) + "\n" + strings.Repeat(".", 40) + "\n" + strings.Repeat(".", 40) + "\n" + strings.Repeat(".", 40)
	popover := renderPopover(base, "POPUP", 40, 5)
	lines := strings.Split(popover, "\n")
	if len(lines) != 5 || lines[2] != strings.Repeat(".", 17)+"POPUP"+strings.Repeat(".", 18) {
		t.Fatalf("popover composition = %#v", lines)
	}
	for i, line := range lines {
		if lipgloss.Width(line) != 40 {
			t.Fatalf("line %d width = %d", i, lipgloss.Width(line))
		}
	}
}
