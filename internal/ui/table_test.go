package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestTableViewRendersSavedFieldsAndRows(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{
		Name:   "Table",
		Number: 1,
		Layout: github.TableLayout,
		Fields: []github.Field{
			{Name: "Title", DataType: "TITLE"},
			{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}},
		},
	}
	model.items = []github.Item{
		{ID: "one", Content: &github.Content{Kind: "Issue", Title: "First item"}, FieldValues: []github.FieldValue{{FieldID: "status", Value: "Todo", Available: true}}},
		{ID: "two", Content: &github.Content{Kind: "Issue", Title: "Second item"}},
	}

	content := ansi.Strip(model.View().Content)
	for _, expected := range []string{"Table", "Title", "Status", "First item", "Todo", "Second item", "-"} {
		if !strings.Contains(content, expected) {
			t.Fatalf("table missing %q: %s", expected, content)
		}
	}
	if !strings.Contains(content, "> ○ First item") {
		t.Fatalf("selected table row missing: %s", content)
	}
}

func TestTableActionsRespectProjectPermissions(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Layout: github.TableLayout, ViewerCanUpdate: false, ProjectID: "project"}
	model.selectedOwner = &github.Owner{Login: "owner"}
	model.selectedProject = &github.Project{Number: 1}
	model.items = []github.Item{{ID: "one", Content: &github.Content{Kind: "Issue", Title: "First item"}}}

	updated, cmd := model.Update(keyPress("J"))
	result := updated.(Model)
	if cmd != nil || result.mutationLoading || result.status != "This project is read-only" {
		t.Fatalf("table mutation state = %#v, cmd nil = %v", result, cmd == nil)
	}
	for _, key := range []string{"H", "L", "K", "a", "D", "h", "l", "left", "right"} {
		updated, cmd = result.Update(keyPress(key))
		result = updated.(Model)
		if cmd != nil || result.mutationLoading || result.tableAction != nil || (result.status != "This project is read-only" && result.status != "") {
			t.Fatalf("table key %s triggered action", key)
		}
	}
}

func TestTableSelectionTargetsVisibleRowAcrossSearchSortAndPages(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Layout: github.TableLayout, GroupByFields: []github.Field{status}, SortByFields: []github.SortField{{Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "ASC"}}}
	model.selectedOwner = &github.Owner{Login: "owner"}
	model.selectedProject = &github.Project{Number: 1}
	model.items = []github.Item{
		{ID: "z", Content: &github.Content{Kind: "Issue", Title: "Zebra", URL: "https://example.com/z"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}},
		{ID: "a", Content: &github.Content{Kind: "Issue", Title: "Alpha", URL: "https://example.com/a"}},
	}
	model.height = 20
	updated, _ := model.Update(keyPress("j"))
	model = updated.(Model)
	if item, ok := model.selectedItem(); !ok || item.ID != "z" {
		t.Fatalf("selected item after down = %#v, %v", item, ok)
	}
	if target, _, _ := model.browserTarget(); target != "https://example.com/z" {
		t.Fatalf("browser target = %q", target)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "> ○ Zebra") {
		t.Fatalf("highlight does not follow sorted selection: %s", ansi.Strip(model.View().Content))
	}
	updated, cmd := model.Update(keyPress("enter"))
	model = updated.(Model)
	if cmd == nil || model.detailItemID != "z" {
		t.Fatalf("detail selection = %q, command nil = %v", model.detailItemID, cmd == nil)
	}
	model.detailVisible = false
	model.filter = "Alpha"
	model.clampTableRow(true)
	if item, ok := model.selectedItem(); !ok || item.ID != "a" {
		t.Fatalf("filtered selection = %#v, %v", item, ok)
	}
	model.filter = ""
	model.clampTableRow(true)
	if item, ok := model.selectedItem(); !ok || item.ID != "z" {
		t.Fatalf("restored focus = %#v, %v", item, ok)
	}
	model.items = model.items[1:]
	model.clampTableRow(false)
	if model.tableFocusID != "z" {
		t.Fatalf("focus lost before later page arrived: %q", model.tableFocusID)
	}
	model.items = append(model.items, github.Item{ID: "z", Content: &github.Content{Kind: "Issue", Title: "Zebra"}})
	model.clampTableRow(true)
	if item, ok := model.selectedItem(); !ok || item.ID != "z" {
		t.Fatalf("paged focus = %#v, %v", item, ok)
	}
	model.items = []github.Item{{ID: "a", Content: &github.Content{Kind: "Issue", Title: "Alpha"}}}
	model.clampTableRow(true)
	if model.tableFocusID != "a" || model.tableRow != 0 {
		t.Fatalf("missing focused item did not fall back: focus=%q row=%d", model.tableFocusID, model.tableRow)
	}
}

func TestTableRefreshPreservesRowIdentity(t *testing.T) {
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.view = &github.View{Layout: github.TableLayout}
	model.selectedOwner = &github.Owner{Login: "owner"}
	model.selectedProject = &github.Project{Number: 1}
	model.items = []github.Item{{ID: "one"}, {ID: "two"}}
	model.moveTableRow(1)
	if cmd := model.startItemsLoad(); cmd == nil || model.tableFocusID != "two" {
		t.Fatalf("refresh command/focus = %v, %q", cmd, model.tableFocusID)
	}
	model.items = []github.Item{{ID: "one"}}
	model.clampTableRow(false)
	model.items = append(model.items, github.Item{ID: "two"})
	model.clampTableRow(true)
	if item, ok := model.selectedItem(); !ok || item.ID != "two" {
		t.Fatalf("refreshed row selection = %#v, %v", item, ok)
	}
}

func TestTableGroupsFollowSavedOptionsAndSelection(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{
		{ID: "todo", Name: "Todo"}, {ID: "doing", Name: "Doing"}, {ID: "done", Name: "Done"},
	}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.height = 100
	model.view = &github.View{Layout: github.TableLayout, GroupByFields: []github.Field{status}, Fields: []github.Field{{Name: "Title", DataType: "TITLE"}}}
	model.items = []github.Item{
		{ID: "doing", Content: &github.Content{Title: "Working"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "doing", Available: true}}},
		{ID: "unset", Content: &github.Content{Title: "Unassigned"}},
		{ID: "todo", Content: &github.Content{Title: "Queued"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}},
	}
	content := ansi.Strip(model.View().Content)
	positions := []int{
		strings.Index(content, "[No Status]  1"), strings.Index(content, "Unassigned"),
		strings.Index(content, "[Todo]  1"), strings.Index(content, "Queued"),
		strings.Index(content, "[Doing]  1"), strings.Index(content, "Working"),
	}
	for i, position := range positions {
		if position < 0 || (i > 0 && position <= positions[i-1]) {
			t.Fatalf("group order/counts incorrect: %s", content)
		}
	}
	if strings.Contains(content, "[Done]") {
		t.Fatalf("empty section rendered: %s", content)
	}
	model.moveTableRow(1)
	if item, ok := model.selectedItem(); !ok || item.ID != "todo" {
		t.Fatalf("row navigation after heading = %#v, %v", item, ok)
	}
	model.filter = "Working"
	model.clampTableRow(true)
	filtered := ansi.Strip(model.View().Content)
	if !strings.Contains(filtered, "[Doing]  1") || strings.Contains(filtered, "[Todo]") || strings.Contains(filtered, "[No Status]") || !strings.Contains(filtered, "> • Working") {
		t.Fatalf("filtered groups = %s", filtered)
	}
	model.filter = "absent title"
	filtered = ansi.Strip(model.View().Content)
	if !strings.Contains(filtered, "No items match the current search.") || strings.Contains(filtered, "[Doing]") || strings.Contains(filtered, "[Todo]") {
		t.Fatalf("empty search groups = %s", filtered)
	}
}

func TestTableGroupSortingAndUnknownValues(t *testing.T) {
	priority := github.Field{ID: "priority", Name: "Priority", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.height = 100
	model.view = &github.View{Layout: github.TableLayout, GroupByFields: []github.Field{priority}, SortByFields: []github.SortField{{Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "ASC"}}}
	model.items = []github.Item{
		{ID: "b", Content: &github.Content{Title: "Beta"}, FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "high", Available: true}}},
		{ID: "unknown", Content: &github.Content{Title: "Other item"}, FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "new", Available: true}}},
		{ID: "a", Content: &github.Content{Title: "Alpha"}, FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "high", Available: true}}},
	}
	content := ansi.Strip(model.View().Content)
	positions := []int{strings.Index(content, "[High]  2"), strings.Index(content, "Alpha"), strings.Index(content, "Beta"), strings.Index(content, "[Other]  1"), strings.Index(content, "Other item")}
	for i, position := range positions {
		if position < 0 || (i > 0 && position <= positions[i-1]) {
			t.Fatalf("sorted/fallback groups incorrect: %s", content)
		}
	}
	if strings.Contains(content, "[Low]") || strings.Contains(content, "[No value]") {
		t.Fatalf("empty sections rendered: %s", content)
	}
}

func TestTableWindowAccountsForGroupHeadingsDuringLoading(t *testing.T) {
	status := github.Field{ID: "status", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "todo", Name: "Todo"}, {ID: "doing", Name: "In Progress"}}}
	model := newPickerModel(fakePickerSource{})
	model.screen = screenBoard
	model.height = 20
	model.view = &github.View{Layout: github.TableLayout, GroupByFields: []github.Field{status}}
	model.itemsLoading = true
	for i := 0; i < 35; i++ {
		model.items = append(model.items, github.Item{ID: fmt.Sprintf("item-%d", i), Content: &github.Content{Title: fmt.Sprintf("Row %d", i)}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "todo", Available: true}}})
	}
	for i := 0; i < 25; i++ {
		model.moveTableRow(1)
	}
	content := ansi.Strip(model.View().Content)
	if !strings.Contains(content, "[Todo]  35") || !strings.Contains(content, "> • Row 25") || !strings.Contains(content, "loading") {
		t.Fatalf("windowed partially loaded group = %s", content)
	}
	if strings.Contains(content, "[In Progress]") {
		t.Fatalf("empty group shown while loading: %s", content)
	}
	model.items = append(model.items, github.Item{ID: "new", Content: &github.Content{Title: "New work"}, FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "doing", Available: true}}})
	model.moveTableRow(20)
	content = ansi.Strip(model.View().Content)
	if !strings.Contains(content, "[In Progress]  1") || !strings.Contains(content, "> • New work") {
		t.Fatalf("populated group did not appear during loading: %s", content)
	}
}

func TestTableCellsShowIdentityProgressAndDistinctMissingValues(t *testing.T) {
	columns := []github.Field{
		{Name: "Title", DataType: "TITLE"},
		{ID: "assignees", Name: "Assignees"},
		{ID: "status", Name: "Status", DataType: "SINGLE_SELECT"},
		{Name: "Sub-issues progress"},
	}
	widths := tableColumnWidths(columns, 110)
	issue := github.Item{Content: &github.Content{Kind: "Issue", Number: 21, Title: "Improve table rendering", SubIssueTotal: 4, SubIssueDone: 3}, FieldValues: []github.FieldValue{
		{FieldID: "assignees", Value: "alice, bob", Available: true},
		{FieldID: "status", Value: "Done", Available: true},
	}}
	row := tableDataRow(columns, issue, widths, true)
	for _, want := range []string{"○", "Improve table rendering", "#21", "alice, bob", "● Done", "3/4 75%"} {
		if !strings.Contains(ansi.Strip(row), want) {
			t.Fatalf("issue row missing %q: %s", want, ansi.Strip(row))
		}
	}
	pr := github.Item{Content: &github.Content{Kind: "PullRequest", Number: 5, Title: "Fix", IsDraft: true}, FieldValues: []github.FieldValue{{FieldID: "assignees", Available: false}}}
	plain := ansi.Strip(tableDataRow(columns, pr, widths, false))
	if !strings.Contains(plain, "◌ Fix #5") || !strings.Contains(plain, "Unavailable") || !strings.Contains(plain, " | -") {
		t.Fatalf("draft PR row = %s", plain)
	}
	draft := github.Item{Content: &github.Content{Kind: "DraftIssue", Title: "Idea"}}
	if plain = ansi.Strip(tableDataRow(columns, draft, widths, false)); !strings.Contains(plain, "◇ Idea") {
		t.Fatalf("draft issue row = %s", plain)
	}
}

func TestTableWidthsKeepTitleAndRowsInsideTerminal(t *testing.T) {
	columns := []github.Field{
		{Name: "Title", DataType: "TITLE"}, {Name: "Assignees"}, {Name: "Status", DataType: "SINGLE_SELECT"},
		{Name: "Linked pull requests"}, {Name: "Sub-issues progress"},
	}
	item := github.Item{Content: &github.Content{Kind: "Issue", Number: 1234, Title: "An unusually long title for a narrow terminal"}}
	for _, width := range []int{34, 52, 80, 180} {
		widths := tableColumnWidths(columns, width)
		row := tableDataRow(columns, item, widths, true)
		if got := lipgloss.Width(row); got > width {
			t.Fatalf("row at width %d occupies %d cells: %s", width, got, ansi.Strip(row))
		}
		if widths[0] <= widths[1] {
			t.Fatalf("title not prioritized at width %d: %#v", width, widths)
		}
		if width >= 52 && !strings.Contains(ansi.Strip(row), "#1234") {
			t.Fatalf("issue identity lost at width %d: %s", width, ansi.Strip(row))
		}
	}
}

func TestTableWidthsGiveTitleRoomWithManyFields(t *testing.T) {
	columns := []github.Field{
		{Name: "Title", DataType: "TITLE"},
		{Name: "Assignees"},
		{Name: "Status", DataType: "SINGLE_SELECT"},
		{Name: "Priority", DataType: "SINGLE_SELECT"},
		{Name: "Iteration", DataType: "ITERATION"},
		{Name: "Due date", DataType: "DATE"},
		{Name: "Repository"},
		{Name: "Sub-issues progress"},
	}
	const terminalWidth = 120
	widths := tableColumnWidths(columns, terminalWidth)
	if widths[0] < 32 {
		t.Fatalf("title column did not get the wider allocation: %#v", widths)
	}
	for index, width := range widths[1:] {
		if width < 8 {
			t.Fatalf("non-title column %q became too narrow: %#v", columns[index+1].Name, widths)
		}
	}

	item := github.Item{Content: &github.Content{Kind: "Issue", Number: 123, Title: "A representative task title that stays visible"}}
	row := tableDataRow(columns, item, widths, false)
	plain := ansi.Strip(row)
	if !strings.Contains(plain, "A representative task") {
		t.Fatalf("title was truncated despite available title space: %s", plain)
	}
	if got := lipgloss.Width(row); got > terminalWidth {
		t.Fatalf("row occupies %d cells in a %d-cell terminal: %s", got, terminalWidth, plain)
	}
}

func TestTableTitleWidthRespectsSavedOrderAndUnicode(t *testing.T) {
	fields := []github.Field{
		{Name: "Assignees"},
		{Name: "Status", DataType: "SINGLE_SELECT"},
		{Name: "Priority", DataType: "SINGLE_SELECT"},
		{Name: "Iteration", DataType: "ITERATION"},
		{Name: "Due date", DataType: "DATE"},
		{Name: "Repository"},
		{Name: "子任务进度"},
	}
	item := github.Item{Content: &github.Content{
		Kind: "Issue", Number: 1234,
		Title: "修复计划 👩🏽‍💻 e\u0301 — keep Unicode titles readable when several fields are visible",
	}}
	for _, titleIndex := range []int{0, 3, 7} {
		columns := append([]github.Field(nil), fields[:titleIndex]...)
		columns = append(columns, github.Field{Name: "Title", DataType: "TITLE"})
		columns = append(columns, fields[titleIndex:]...)
		for _, width := range []int{40, 80, 120, 240} {
			t.Run(fmt.Sprintf("title_%d_width_%d", titleIndex, width), func(t *testing.T) {
				view := &github.View{Fields: columns}
				displayed := tableColumns(view)
				for index, column := range displayed {
					if column.Name != columns[index].Name || column.DataType != columns[index].DataType {
						t.Fatalf("saved column order changed at %d", index)
					}
				}
				widths := tableColumnWidths(displayed, width)
				if width >= 120 && widths[titleIndex] < 32 {
					t.Fatalf("title did not retain readable space: %v", widths)
				}
				for index, columnWidth := range widths {
					if index != titleIndex && widths[titleIndex] <= columnWidth {
						t.Fatalf("title not prioritized over column %d: %v", index, widths)
					}
					if width >= 120 && columnWidth < 8 {
						t.Fatalf("column %d became unreadable: %v", index, widths)
					}
				}
				for _, row := range []string{
					tableHeaderRow(displayed, widths), tableRule(widths),
					tableDataRow(displayed, item, widths, false),
					tableDataRow(displayed, item, widths, true),
				} {
					if got := ansi.StringWidth(row); got > width {
						t.Fatalf("row occupies %d cells in %d cells: %s", got, width, ansi.Strip(row))
					}
				}
				if width >= 80 {
					row := ansi.Strip(tableDataRow(displayed, item, widths, true))
					if !strings.Contains(row, "修复计划") || !strings.Contains(row, "#1234") {
						t.Fatalf("Unicode title or issue number lost: %s", row)
					}
					if width >= 120 && (!strings.Contains(row, "👩🏽‍💻") || !strings.Contains(row, "e\u0301")) {
						t.Fatalf("title split an emoji or combining character: %s", row)
					}
				}
			})
		}
	}
}
