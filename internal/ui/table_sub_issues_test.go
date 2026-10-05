package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type tableSubIssueSource struct {
	fakePickerSource
	children        map[string]map[string]github.ItemsPage
	childErrors     map[string]map[string]error
	issues, cursors []string
	fields          [][]github.Field
	detailIDs       []string
	externalIDs     []string
}

func (s *tableSubIssueSource) PageSubIssues(_ context.Context, projectID, issueID, after string, fields []github.Field) (github.ItemsPage, error) {
	s.issues, s.cursors = append(s.issues, issueID), append(s.cursors, after)
	s.fields = append(s.fields, fields)
	if projectID != s.view.ProjectID {
		return github.ItemsPage{}, errors.New("wrong project")
	}
	if err := s.childErrors[issueID][after]; err != nil {
		return github.ItemsPage{}, err
	}
	return s.children[issueID][after], nil
}

func (s *tableSubIssueSource) LoadItemDetail(_ context.Context, _ github.Owner, _ int, itemID string) (github.ItemDetail, error) {
	s.detailIDs = append(s.detailIDs, itemID)
	return github.ItemDetail{ID: itemID}, nil
}

func (s *tableSubIssueSource) LoadIssueDetail(_ context.Context, issueID string) (github.ItemDetail, error) {
	s.externalIDs = append(s.externalIDs, issueID)
	return github.ItemDetail{ID: issueID, Content: &github.Content{ID: issueID, Kind: "Issue", Title: "Outside project"}}, nil
}

func nestedTableFixture() (Model, *tableSubIssueSource) {
	view := writableStatusView()
	view.ID, view.Number, view.Layout, view.Filter = "table-view", 7, github.TableLayout, "has:otc-sprint"
	view.Fields = []github.Field{{Name: "Title", DataType: "TITLE"}, {ID: "sprint", Name: "OTC Sprint", DataType: "MULTI_SELECT"}, {Name: "Sub-issues progress"}}
	view.GroupByFields = nil
	root := github.Item{ID: "root", Content: &github.Content{ID: "root-issue", Kind: "Issue", Title: "Parent", SubIssueTotal: 2, SubIssueDone: 1}, FieldValues: []github.FieldValue{{FieldID: "sprint", FieldName: "OTC Sprint", Value: "Sprint 8, Sprint 9", Available: true}}}
	other := github.Item{ID: "other", Content: &github.Content{ID: "other-issue", Kind: "Issue", Title: "Other root"}}
	child := github.Item{ID: "child", Content: &github.Content{ID: "child-issue", Kind: "Issue", Title: "Child without sprint", Number: 42, SubIssueTotal: 1, URL: "https://example.invalid/issues/42"}}
	sibling := github.Item{ID: "sibling", Content: &github.Content{ID: "sibling-issue", Kind: "Issue", Title: "Second child", State: "CLOSED"}}
	external := github.Item{ID: "outside-issue", OutsideProject: true, Content: &github.Content{ID: "outside-issue", Kind: "Issue", Title: "Outside project", URL: "https://example.invalid/issues/43"}}
	items := []github.Item{root, other}
	source := &tableSubIssueSource{fakePickerSource: fakePickerSource{view: view, itemPagesByFilter: map[string]map[string]github.ItemsPage{view.Filter: {"": {Items: items}}}}, children: map[string]map[string]github.ItemsPage{
		"root-issue":  {"": {Items: []github.Item{child}, HasNext: true, EndCursor: "next"}, "next": {Items: []github.Item{sibling}}},
		"child-issue": {"": {Items: []github.Item{external}}},
	}}
	m := mutationModel(source.fakePickerSource, view, items)
	m.source, m.width, m.height = source, 150, 40
	return m, source
}

func tableRowIDs(m Model) []string {
	ids := []string{}
	for _, item := range m.tableItems() {
		ids = append(ids, item.ID)
	}
	return ids
}

func tableKey(t *testing.T, m Model, key string) Model {
	t.Helper()
	updated, cmd := m.Update(keyPress(key))
	return runMembershipReads(t, updated.(Model), cmd)
}

func TestTableExpandsPaginatedChildrenAndRecursiveHierarchy(t *testing.T) {
	m, source := nestedTableFixture()
	if len(source.issues) != 0 || !strings.Contains(string(m.View().Content), "▸") {
		t.Fatal("children were loaded eagerly or expand marker was missing")
	}
	m = tableKey(t, m, "right")
	if want := []string{"root", "child", "sibling", "other"}; !reflect.DeepEqual(tableRowIDs(m), want) {
		t.Fatalf("expanded rows = %v, want %v", tableRowIDs(m), want)
	}
	if len(m.items) != 2 || !reflect.DeepEqual(source.issues, []string{"root-issue", "root-issue"}) || !reflect.DeepEqual(source.cursors, []string{"", "next"}) {
		t.Fatal("hierarchy changed query membership or failed to page")
	}
	for _, fields := range source.fields {
		if !reflect.DeepEqual(fields, boardReadFields(*m.view)) {
			t.Fatal("nested row did not request saved visible fields")
		}
	}
	m = tableKey(t, m, "j")
	if item, _ := m.selectedItem(); item.ID != "child" || tableCellValue(m.view.Fields[1], item) != "-" {
		t.Fatal("child navigation or independent field values failed")
	}
	m = tableKey(t, m, "l")
	if want := []string{"root", "child", "outside-issue", "sibling", "other"}; !reflect.DeepEqual(tableRowIDs(m), want) {
		t.Fatalf("recursive rows = %v, want %v", tableRowIDs(m), want)
	}
	rows := m.tableTreeRows()
	if rows[2].depth != 2 || rows[2].parentID != "child" || !strings.Contains(string(m.View().Content), "Outside project") {
		t.Fatal("recursive hierarchy not rendered")
	}
	m = tableKey(t, m, "j")
	if target, _, err := m.browserTarget(); err != nil || target != "https://example.invalid/issues/43" {
		t.Fatalf("nested browser target = %q, %v", target, err)
	}
	if reason := m.tableActionUnavailable(); !strings.Contains(reason, "not an active item") {
		t.Fatalf("outside-project row allowed project mutations: %q", reason)
	}
	m = tableKey(t, m, "left") // leaf -> its parent
	m = tableKey(t, m, "h")    // collapse that parent
	if item, _ := m.selectedItem(); item.ID != "child" || len(m.tableItems()) != 4 {
		t.Fatal("collapse lost focus or retained hidden descendants")
	}
	m = tableKey(t, m, "h") // child -> root
	m = tableKey(t, m, "h") // collapse root
	if want := []string{"root", "other"}; !reflect.DeepEqual(tableRowIDs(m), want) {
		t.Fatalf("collapsed rows = %v", tableRowIDs(m))
	}
	reads := len(source.issues)
	m = tableKey(t, m, "l")
	if len(m.tableItems()) != 4 || len(source.issues) != reads {
		t.Fatal("reopening a cached subtree repeated reads")
	}
}

func TestNestedTableDetailsUseCorrectIdentities(t *testing.T) {
	m, source := nestedTableFixture()
	m = tableKey(t, m, "l")
	m = tableKey(t, m, "j")
	m = tableKey(t, m, "enter")
	if !reflect.DeepEqual(source.detailIDs, []string{"child"}) || !m.detailVisible {
		t.Fatal("nested project detail used a root/issue identity")
	}
	m = tableKey(t, m, "esc")
	m = tableKey(t, m, "l")
	m = tableKey(t, m, "j")
	m = tableKey(t, m, "enter")
	if !reflect.DeepEqual(source.externalIDs, []string{"outside-issue"}) || m.detailErr != nil || m.detail == nil || m.detail.Content.Title != "Outside project" {
		t.Fatalf("outside-project detail failed: %#v, %v", m.detail, m.detailErr)
	}
}

func TestTableHierarchyRefreshRestoresExpansionAndNestedFocus(t *testing.T) {
	m, source := nestedTableFixture()
	m = tableKey(t, m, "l")
	m = tableKey(t, m, "j")
	m = tableKey(t, m, "l")
	m = tableKey(t, m, "j")
	m = tableKey(t, m, "r")
	if item, _ := m.selectedItem(); item.ID != "outside-issue" || len(m.tableItems()) != 5 || len(source.issues) != 6 {
		t.Fatalf("refresh lost nested focus/expansion: focus=%s rows=%v reads=%v", item.ID, tableRowIDs(m), source.issues)
	}
}

func TestTableChildFailureRetainsRowsAndRetriesContinuation(t *testing.T) {
	m, source := nestedTableFixture()
	source.childErrors = map[string]map[string]error{"root-issue": {"next": errors.New("network failed")}}
	m = tableKey(t, m, "l")
	if want := []string{"root", "child", "other"}; !reflect.DeepEqual(tableRowIDs(m), want) || !strings.Contains(m.status, "network failed") || !strings.Contains(string(m.View().Content), "!") {
		t.Fatalf("partial children or error lost: rows=%v status=%s", tableRowIDs(m), m.status)
	}
	source.childErrors = nil
	m = tableKey(t, m, "l")
	if len(m.tableItems()) != 4 || !reflect.DeepEqual(source.cursors, []string{"", "next", "next"}) || m.tableSubIssues["root"].err != nil {
		t.Fatalf("child retry duplicated or restarted rows: %v, %v", tableRowIDs(m), source.cursors)
	}
}

func TestTableSearchIncludesMatchingLoadedDescendantWithAncestors(t *testing.T) {
	m, _ := nestedTableFixture()
	m = tableKey(t, m, "l")
	m = tableKey(t, m, "j")
	m = tableKey(t, m, "l")
	m.filter = "Outside project"
	if want := []string{"root", "child", "outside-issue"}; !reflect.DeepEqual(tableRowIDs(m), want) {
		t.Fatalf("search lost hierarchy context: %v", tableRowIDs(m))
	}
}

func TestTableHierarchyDeduplicatesMatchingRootsAndRejectsStaleReads(t *testing.T) {
	m, source := nestedTableFixture()
	child := source.children["root-issue"][""].Items[0]
	m.items = append([]github.Item{child}, m.items...)
	m.tableFocusID = "root"
	m = tableKey(t, m, "l")
	if want := []string{"root", "child", "sibling", "other"}; !reflect.DeepEqual(tableRowIDs(m), want) {
		t.Fatalf("matching child duplicated: %v", tableRowIDs(m))
	}
	m.tableFocusID = "child"
	updated, cmd := m.Update(keyPress("l"))
	m = updated.(Model)
	msg := cmd()
	m.abandonReads()
	updated, next := m.Update(msg)
	m = updated.(Model)
	if next != nil || len(m.tableSubIssues) != 0 {
		t.Fatal("stale hierarchy response repopulated another selection")
	}
}

func TestTableSubIssueProgressMatchesWholePercentDisplay(t *testing.T) {
	item := github.Item{Content: &github.Content{Kind: "Issue", SubIssueTotal: 3, SubIssueDone: 2}}
	if value := tableCellValue(github.Field{Name: "Sub-issues progress"}, item); value != "2/3 66%" {
		t.Fatalf("progress = %s", value)
	}
}

func TestNestedTableRowsPreserveIndentationAndColumnAlignment(t *testing.T) {
	m := Model{}
	item := github.Item{Content: &github.Content{Kind: "Issue", Title: "Example"}}
	columns := []github.Field{{Name: "Title", DataType: "TITLE"}, {Name: "Status"}}
	widths := []int{40, 12}
	root := ansi.Strip(tableDataRowWithTitlePrefix(columns, item, widths, false, m.tableTreePrefix(tableTreeRow{item: item})))
	for _, depth := range []int{1, 2} {
		child := ansi.Strip(tableDataRowWithTitlePrefix(columns, item, widths, false, m.tableTreePrefix(tableTreeRow{item: item, depth: depth})))
		if offset := strings.Index(child, "Example") - strings.Index(root, "Example"); offset != 2*depth {
			t.Fatalf("depth %d title inset = %d, want %d", depth, offset, 2*depth)
		}
		if strings.Index(child, "|") != strings.Index(root, "|") {
			t.Fatalf("indentation shifted another column: root=%q child=%q", root, child)
		}
	}
}

func TestCollapsingWhileChildrenLoadStopsPagingAndCanResume(t *testing.T) {
	m, source := nestedTableFixture()
	updated, cmd := m.Update(keyPress("l"))
	m = updated.(Model)
	m = tableKey(t, m, "h")
	updated, next := m.Update(cmd())
	m = updated.(Model)
	if next != nil || len(m.tableItems()) != 2 || len(source.issues) != 1 {
		t.Fatal("collapsed subtree kept paging or exposed child rows")
	}
	m = tableKey(t, m, "l")
	if len(m.tableItems()) != 4 || !reflect.DeepEqual(source.cursors, []string{"", "next"}) {
		t.Fatal("reopening partial subtree did not resume its cursor")
	}
}

func TestMetadataRefreshFailureDoesNotLeaveChildLoadingStuck(t *testing.T) {
	m, _ := nestedTableFixture()
	updated, childCmd := m.Update(keyPress("l"))
	m = updated.(Model)
	refreshCmd := m.refresh()
	if refreshCmd == nil || m.tableSubIssues["root"].loading {
		t.Fatal("refresh did not cancel child-loading state")
	}
	updated, _ = m.Update(viewRefreshMsg{scope: m.currentBoardReadScope(), err: errors.New("metadata failed")})
	m = updated.(Model)
	updated, next := m.Update(childCmd())
	m = updated.(Model)
	if next != nil || len(m.tableSubIssues["root"].items) != 0 {
		t.Fatal("cancelled child response survived a failed metadata refresh")
	}
	m = tableKey(t, m, "l")
	if m.tableSubIssues["root"].loading || len(m.tableItems()) != 4 {
		t.Fatal("child-loading retry stayed stuck")
	}
}
