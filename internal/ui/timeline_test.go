package ui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func liveTimelineFixture() (Model, *viewRefreshSource) {
	view, mappings := mappedRoadmapFixture()
	source := &viewRefreshSource{fakePickerSource: fakePickerSource{view: view}}
	m := mutationModel(source.fakePickerSource, view, nil)
	m.source = source
	m.setRoadmapMappings(mappings)
	m.width, m.height, m.timelineMonth = 120, 35, "2024-01"
	return m, source
}

func TestTimelineCompatibilityRejectsFiltersAndSorts(t *testing.T) {
	for _, mode := range []string{"filter", "sort"} {
		m, _ := liveTimelineFixture()
		if mode == "filter" {
			m.view.Filter = "is:unknown"
		} else {
			m.view.SortByFields = []github.SortField{{Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "INVALID"}}
		}
		if c := m.viewCompatibility(*m.view); c.supported() || c.summary() == "" {
			t.Fatalf("admitted %s: %s", mode, c.summary())
		}
	}
}

func TestTimelinePagesKeepOrderSelectionAndFullEndpointReads(t *testing.T) {
	m, source := liveTimelineFixture()
	first := syntheticTimelineRow("later-date", "2024-03-01", "2024-03-02").Item
	first.FieldValues = []github.FieldValue{{FieldID: "start-id", Value: "2024-03-01", Available: true}, {FieldID: "target-id", Value: "2024-03-02", Available: true}}
	second := syntheticTimelineRow("earlier-date", "2024-01-01", "").Item
	second.FieldValues = []github.FieldValue{{FieldID: "start-id", Value: "2024-01-01", Available: true}}
	source.itemPages = map[string]github.ItemsPage{"": {Items: []github.Item{first}, HasNext: true, EndCursor: "next"}, "next": {Items: []github.Item{second}}}
	cmd := m.startItemsLoad()
	updated, next := m.Update(cmd())
	m = updated.(Model)
	if next == nil || !m.itemsLoading {
		t.Fatal("pagination stopped early")
	}
	m.tableFocusID = first.ID
	updated, next = m.Update(next())
	m = updated.(Model)
	if next != nil || m.itemsLoading || len(source.fields) != 0 || len(m.items) != 2 {
		t.Fatal("did not read complete item fields over both pages")
	}
	if m.tableItems()[0].ID != first.ID {
		t.Fatal("timeline changed project-position order")
	}
	if item, ok := m.selectedItem(); !ok || item.ID != first.ID {
		t.Fatal("page changed selection")
	}
	display := string(m.View().Content)
	for _, want := range []string{"Start: Date A", "Start: 2024-03-01", "Target: 2024-03-02", "Local range"} {
		if !strings.Contains(display, want) {
			t.Fatalf("missing %q: %s", want, display)
		}
	}
	updated, _ = m.Update(keyPress("j"))
	m = updated.(Model)
	if item, _ := m.selectedItem(); item.ID != second.ID {
		t.Fatal("row navigation missed highlighted item")
	}
	updated, _ = m.Update(keyPress("l"))
	m = updated.(Model)
	if m.timelineMonth != "2024-02" {
		t.Fatal("month navigation failed")
	}
	updated, _ = m.Update(keyPress("h"))
	m = updated.(Model)
	if m.timelineMonth != "2024-01" {
		t.Fatal("month navigation failed")
	}
	m.width = 48
	if !strings.Contains(string(m.View().Content), "Date list") {
		t.Fatal("narrow fallback missing")
	}
}

func TestTimelineRefreshPreservesFocusAndRejectsChangedSettings(t *testing.T) {
	for _, changed := range []string{"none", "filter", "sort", "group", "mapping"} {
		t.Run(changed, func(t *testing.T) {
			m, source := liveTimelineFixture()
			source.itemPages = map[string]github.ItemsPage{"": {Items: []github.Item{{ID: "first", Content: &github.Content{Title: "first"}}, {ID: "selected", Content: &github.Content{Title: "selected"}}}}}
			m.items = source.itemPages[""].Items
			m.tableFocusID = "selected"
			switch changed {
			case "filter":
				source.view.Filter = "is:unknown"
			case "sort":
				source.view.SortByFields = []github.SortField{{Field: github.Field{Name: "Title"}, Direction: "INVALID"}}
			case "group":
				source.view.GroupByFields = []github.Field{compatibilityStatusField()}
			case "mapping":
				source.view.ProjectFields = nil
			}
			m = runMembershipReads(t, m, m.refresh())
			if changed == "none" {
				if item, ok := m.selectedItem(); !ok || item.ID != "selected" || m.timelineMonth != "2024-01" {
					t.Fatal("refresh lost selection or local range")
				}
			} else if m.screen != screenViewPicker || m.view != nil || m.itemsLoading {
				t.Fatal("refresh admitted unsupported settings")
			}
		})
	}
}

func TestTimelineSearchAndStalePages(t *testing.T) {
	m, _ := liveTimelineFixture()
	m.items = []github.Item{{ID: "first", Content: &github.Content{Title: "first"}}, {ID: "selected", Content: &github.Content{Title: "selected"}}}
	m.tableFocusID = "selected"
	updated, _ := m.Update(keyPress("/"))
	m = updated.(Model)
	m.appendInputText("first")
	if item, _ := m.selectedItem(); item.ID != "first" {
		t.Fatal("search did not select visible row")
	}
	updated, _ = m.Update(keyPress("esc"))
	m = updated.(Model)
	if item, _ := m.selectedItem(); item.ID != "selected" {
		t.Fatal("clearing search lost identity")
	}
	stale := itemsPageMsg{scope: m.currentBoardReadScope(), generation: m.generation, reset: true, page: github.ItemsPage{Items: []github.Item{{ID: "stale"}}}}
	m.abandonReads()
	updated, cmd := m.Update(stale)
	m = updated.(Model)
	if cmd != nil || len(m.items) != 2 {
		t.Fatal("stale result replaced timeline")
	}
}

func TestTimelineNeverEnablesWritesIncludingDetails(t *testing.T) {
	m, _ := liveTimelineFixture()
	m.items = []github.Item{{ID: "issue", Content: &github.Content{Kind: "Issue", ID: "issue-id", State: "OPEN"}}}
	for _, key := range []string{"H", "L", "J", "K", "a", "D", "m", "c", "x"} {
		updated, cmd := m.Update(keyPress(key))
		m = updated.(Model)
		if cmd != nil || m.mutationLoading || m.commentEditing || m.issueActionConfirm {
			t.Fatalf("write key %q enabled mutation", key)
		}
	}
	if m.boardMutationUnavailable() != "Timelines are read-only" {
		t.Fatal("missing mutation guard")
	}
	m.detailVisible = true
	m.detail = &github.ItemDetail{Content: m.items[0].Content}
	for _, key := range []string{"c", "x"} {
		updated, cmd := m.Update(keyPress(key))
		m = updated.(Model)
		if cmd != nil || m.commentEditing || m.issueActionConfirm {
			t.Fatal("detail enabled issue writes")
		}
	}
}

func TestTimelineEndpointPolicies(t *testing.T) {
	for _, tc := range []struct {
		values []github.FieldValue
		want   dateTimelineEndpoint
	}{
		{nil, dateTimelineEndpoint{Available: true}},
		{[]github.FieldValue{{FieldID: "start-id", Available: false}}, dateTimelineEndpoint{}},
		{[]github.FieldValue{{FieldID: "start-id", Value: "2024-01-01", Available: true}}, dateTimelineEndpoint{Value: "2024-01-01", Available: true}},
		{[]github.FieldValue{{FieldID: "start-id", Available: true}, {FieldID: "start-id", Available: true}}, dateTimelineEndpoint{}},
	} {
		if got := timelineEndpoint("start-id", github.Item{FieldValues: tc.values}); got != tc.want {
			t.Fatalf("endpoint = %+v; want %+v", got, tc.want)
		}
	}
}

func TestTimelinePickerOpensForBothOwnerKindsAndDetailsTargetSelection(t *testing.T) {
	for _, kind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
		t.Run(string(kind), func(t *testing.T) {
			view, mappings := mappedRoadmapFixture()
			item := github.Item{ID: "selected", Content: &github.Content{ID: "issue", Kind: "Issue", Title: "Selected issue", URL: "https://github.com/owner/repo/issues/1"}}
			source := fakePickerSource{view: view, itemPages: map[string]github.ItemsPage{"": {Items: []github.Item{item}}}, detail: github.ItemDetail{ID: item.ID, Content: item.Content}}
			m := NewModelWithHost(source, Selection{}, "github.com")
			m.setRoadmapMappings(mappings)
			m.screen = screenViewPicker
			m.selectedOwner = &github.Owner{Login: "owner", Kind: kind}
			m.selectedProject = &github.Project{ID: view.ProjectID, Number: 7}
			m.views = []github.ViewSummary{{ID: view.ID, Number: view.Number, Layout: view.Layout}}
			updated, cmd := m.Update(keyPress("enter"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if !m.isTimeline() || m.screen != screenBoard || len(m.items) != 1 {
				t.Fatal("picker did not open eligible roadmap")
			}
			target, fallback, err := m.browserTarget()
			if err != nil || fallback || target != item.Content.URL {
				t.Fatal("browser did not target selected row")
			}
			updated, cmd = m.Update(keyPress("enter"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if !m.detailVisible || m.detailLoading || m.detail == nil || m.detailItemID != item.ID || m.tableFocusID != item.ID {
				t.Fatal("detail did not follow selected row")
			}
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(Model)
			if m.detailVisible {
				t.Fatal("detail did not close")
			}
		})
	}
}

func TestTimelineAdmitsSharedFiltersAndSupportedSavedSorts(t *testing.T) {
	for _, tc := range []struct {
		name, filter, direction, fieldID, dataType string
		secondary                                  bool
		supported                                  bool
	}{
		{name: "empty", supported: true},
		{name: "issue filter", filter: "is:issue", supported: true},
		{name: "whitespace preserved", filter: " is:issue ", supported: true},
		{name: "start ASC", direction: "ASC", fieldID: "start-id", dataType: "DATE", supported: true},
		{name: "filtered start ASC", filter: "is:issue", direction: "ASC", fieldID: "start-id", dataType: "DATE", supported: true},
		{name: "conjunction", filter: "is:issue is:open", supported: true},
		{name: "other filter", filter: "is:pr", supported: true},
		{name: "DESC", direction: "DESC", fieldID: "start-id", dataType: "DATE", supported: true},
		{name: "target ASC", direction: "ASC", fieldID: "target-id", dataType: "DATE", supported: true},
		{name: "wrong type", direction: "ASC", fieldID: "start-id", dataType: "TEXT"},
		{name: "duplicate secondary sort", direction: "ASC", fieldID: "start-id", dataType: "DATE", secondary: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := liveTimelineFixture()
			m.view.Filter = tc.filter
			if tc.direction != "" {
				m.view.SortByFields = []github.SortField{{Field: github.Field{ID: tc.fieldID, Name: "Date A", DataType: tc.dataType}, Direction: tc.direction}}
			}
			if tc.secondary {
				m.view.SortByFields = append(m.view.SortByFields, m.view.SortByFields[0])
			}
			if c := m.viewCompatibility(*m.view); c.supported() != tc.supported {
				t.Fatalf("supported=%t; reasons=%s", c.supported(), c.summary())
			}
		})
	}
}

type filteredTimelineSource struct {
	*viewRefreshSource
	fullFilters, fullCursors []string
	detailReads              int
}

func (s *filteredTimelineSource) PageItems(ctx context.Context, owner github.Owner, project int, filter, after string) (github.ItemsPage, error) {
	s.fullFilters = append(s.fullFilters, filter)
	s.fullCursors = append(s.fullCursors, after)
	return s.fakePickerSource.PageItems(ctx, owner, project, filter, after)
}
func (s *filteredTimelineSource) LoadItemDetail(_ context.Context, _ github.Owner, _ int, id string) (github.ItemDetail, error) {
	s.detailReads++
	return github.ItemDetail{ID: id, Content: &github.Content{Kind: "Issue", Title: id}}, nil
}
func TestFilteredTimelinePagingSortsNullsAndTiesWithoutLosingFocus(t *testing.T) {
	for _, kind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
		t.Run(string(kind), func(t *testing.T) {
			m, base := liveTimelineFixture()
			source := &filteredTimelineSource{viewRefreshSource: base}
			m.source = source
			m.selectedOwner.Kind = kind
			m.view.Filter = " is:issue "
			m.view.SortByFields = []github.SortField{{Field: m.view.ProjectFields[0], Direction: "ASC"}}
			source.view = *m.view
			row := func(id, date string) github.Item {
				item := github.Item{ID: id, Content: &github.Content{Kind: "Issue", Title: id}}
				if date != "" {
					item.FieldValues = []github.FieldValue{{FieldID: "start-id", Value: date, Available: true}}
				}
				return item
			}
			source.itemPagesByFilter = map[string]map[string]github.ItemsPage{m.view.Filter: {
				"":     {Items: []github.Item{row("tie-first", "2024-02-01"), row("late", "2024-03-01"), row("unset-first", "")}, HasNext: true, EndCursor: "next"},
				"next": {Items: []github.Item{row("early", "2024-01-01"), row("tie-second", "2024-02-01"), row("unset-second", "")}},
			}}
			updated, next := m.Update(m.startItemsLoad()())
			m = updated.(Model)
			m.tableFocusID = "late"
			m = runMembershipReads(t, m, next)
			ids := []string{}
			for _, item := range m.tableItems() {
				ids = append(ids, item.ID)
			}
			if !reflect.DeepEqual(ids, []string{"early", "tie-first", "tie-second", "late", "unset-first", "unset-second"}) {
				t.Fatalf("order=%v", ids)
			}
			if item, _ := m.selectedItem(); item.ID != "late" {
				t.Fatal("sorting appended page changed selection")
			}
			if !reflect.DeepEqual(source.fullFilters, []string{" is:issue ", " is:issue "}) || !reflect.DeepEqual(source.fullCursors, []string{"", "next"}) {
				t.Fatal("saved filter/cursor was changed across pages")
			}
			if len(source.fields) != 0 {
				t.Fatal("used name-based projection for endpoints")
			}
			display := string(m.View().Content)
			if !strings.Contains(display, "Saved filter:") || !strings.Contains(display, "Saved order:") {
				t.Fatal("saved filter/order omitted")
			}
			updated, cmd := m.Update(keyPress("enter"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if m.detail == nil || m.detail.ID != "late" || source.detailReads != 1 {
				t.Fatal("detail did not follow sorted selection")
			}
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(Model)
			m = runMembershipReads(t, m, m.refresh())
			if item, _ := m.selectedItem(); item.ID != "late" {
				t.Fatal("refresh lost sorted selection")
			}
			if len(source.fullFilters) != 4 {
				t.Fatal("refresh did not forward saved filter on both pages")
			}
		})
	}
}

func TestFilteredTimelineStandardTerminalKeepsCountsAndDatesVisible(t *testing.T) {
	m, _ := liveTimelineFixture()
	m.width, m.height = 80, 24
	m.view.Filter = "is:issue"
	m.view.SortByFields = []github.SortField{{Field: m.view.ProjectFields[0], Direction: "ASC"}}
	m.items = []github.Item{{ID: "one", Content: &github.Content{Kind: "Issue", Title: "one"}, FieldValues: []github.FieldValue{{FieldID: "start-id", Value: "2024-01-01", Available: true}}}}
	text := string(m.View().Content)
	for _, want := range []string{"Loaded 1", "Local 3 months", "Start: 2024-01-01", "Target: unset"} {
		if !strings.Contains(text, want) {
			t.Fatalf("standard terminal lost %q: %s", want, text)
		}
	}
}

func TestTimelineBroaderFiltersForwardUnchangedAcrossPages(t *testing.T) {
	for _, filter := range []string{` is:issue label:demo status:"In Progress" `, `label:demo -status:Done`, `is:open repo:owner/repo`} {
		m, base := liveTimelineFixture()
		m.view.Filter = filter
		first, second := github.Item{ID: "first", Content: &github.Content{Title: "first"}}, github.Item{ID: "second", Content: &github.Content{Title: "second"}}
		base.itemPages = map[string]github.ItemsPage{"": {Items: []github.Item{first}, HasNext: true, EndCursor: "next"}, "next": {Items: []github.Item{second}}}
		source := &filteredTimelineSource{viewRefreshSource: base}
		m.source = source
		if c := m.viewCompatibility(*m.view); !c.supported() {
			t.Fatal(c.summary())
		}
		cmd := m.startItemsLoad()
		updated, next := m.Update(cmd())
		m = updated.(Model)
		m.tableFocusID = "first"
		m = runMembershipReads(t, m, next)
		if len(source.fullFilters) != 2 || source.fullFilters[0] != filter || source.fullFilters[1] != filter {
			t.Fatal("saved filter changed across pages")
		}
		if item, _ := m.selectedItem(); item.ID != "first" || len(m.items) != 2 {
			t.Fatal("page changed selection/membership")
		}
	}
}

func TestTimelineTwoDateSortsPreserveTiesNullsAndEndpointRoles(t *testing.T) {
	m, _ := liveTimelineFixture()
	m.view.SortByFields = []github.SortField{{Field: m.view.ProjectFields[0], Direction: "DESC"}, {Field: m.view.ProjectFields[1], Direction: "ASC"}}
	for _, tc := range []struct{ id, start, target string }{{"tie-later", "2026-10-05", "2026-11-02"}, {"null", "", ""}, {"tie-earlier", "2026-10-05", "2026-10-16"}, {"latest", "2026-10-26", "2026-11-20"}, {"tie-stable", "2026-10-05", "2026-10-16"}} {
		item := github.Item{ID: tc.id, Content: &github.Content{Title: tc.id}, FieldValues: []github.FieldValue{{FieldID: "start-id", Value: tc.start, Available: true}, {FieldID: "target-id", Value: tc.target, Available: true}}}
		m.items = append(m.items, item)
	}
	if c := m.viewCompatibility(*m.view); !c.supported() {
		t.Fatal(c.summary())
	}
	for i, id := range []string{"latest", "tie-earlier", "tie-stable", "tie-later", "null"} {
		if m.tableItems()[i].ID != id {
			t.Fatal("two-field order/ties/nulls changed")
		}
	}
	m.tableFocusID = "tie-later"
	display := string(m.View().Content)
	if !strings.Contains(display, "Date A descending, Date B ascending") || !strings.Contains(display, "Target: 2026-11-02") {
		t.Fatal("saved order/endpoint role mislabeled")
	}
}
