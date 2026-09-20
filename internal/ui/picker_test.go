package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakePickerSource struct {
	discovery github.Discovery
	views     []github.ViewSummary
	view      github.View
	itemPages map[string]github.ItemsPage
	detail    github.ItemDetail
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
	return f.view, nil
}

func (f fakePickerSource) PageItems(_ context.Context, _ github.Owner, _ int, _, after string) (github.ItemsPage, error) {
	if page, ok := f.itemPages[after]; ok {
		return page, nil
	}
	return github.ItemsPage{}, nil
}

func (f fakePickerSource) LoadItemDetail(context.Context, github.Owner, int, string) (github.ItemDetail, error) {
	return f.detail, nil
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
	model := NewModel(source)
	model.loadPrefs = func(string) (config.Selection, error) { return config.Selection{}, nil }
	model.savePrefs = func(string, config.Selection) error { return nil }
	return model
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
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}

	cmd := model.startItemsLoad()
	updated, next := model.Update(cmd())
	model = updated.(Model)
	if len(model.items) != 1 || !model.itemsLoading || next == nil {
		t.Fatalf("first page state = %#v, next nil = %v", model, next == nil)
	}
	updated, next = model.Update(next())
	model = updated.(Model)
	if len(model.items) != 2 || model.itemsLoading || next != nil {
		t.Fatalf("final page state = %#v, next nil = %v", model, next == nil)
	}
	lanes := model.boardLanes()
	if len(lanes) != 3 || len(lanes[0].Items) != 1 || len(lanes[1].Items) != 1 || len(lanes[2].Items) != 0 {
		t.Fatalf("lanes = %#v", lanes)
	}
	content := model.View().Content
	for _, expected := range []string{"Fix the thing", "Ship the thing", "[Todo]", "[Done]"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("board missing %q: %s", expected, content)
		}
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
	updated, _ := model.Update(keyPress("j"))
	model = updated.(Model)
	if model.boardCard != 1 {
		t.Fatalf("card cursor = %d", model.boardCard)
	}
	updated, _ = model.Update(keyPress("l"))
	model = updated.(Model)
	if model.boardLane != 1 || model.boardCard != 0 {
		t.Fatalf("lane cursor = %d, card cursor = %d", model.boardLane, model.boardCard)
	}
	updated, _ = model.Update(keyPress("k"))
	model = updated.(Model)
	if model.boardCard != 0 {
		t.Fatalf("card cursor after clamp = %d", model.boardCard)
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
	if len(lanes) != 2 || lanes[0].Name != "Todo" || len(lanes[0].Items) != 1 {
		t.Fatalf("vertical lanes = %#v", lanes)
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
		if !strings.Contains(column, "> Issue #") {
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
	updated, _ = model.Update(cmd())
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
}

func TestDetailPopoverExpandsOnWideTerminal(t *testing.T) {
	model := NewModel(nil)
	model.width = 120
	if model.detailPanelWidth() < 90 {
		t.Fatalf("popover content width = %d", model.detailPanelWidth())
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
