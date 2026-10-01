package ui

import (
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func groupedLiveTimelineFixture() (Model, *viewRefreshSource) {
	m, source := liveTimelineFixture()
	field := github.Field{ID: "group", Name: "Status", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "ready", Name: "Ready"}, {ID: "done", Name: "Done"}}}
	m.view.ProjectFields = append(m.view.ProjectFields, field)
	m.view.GroupByFields = []github.Field{field}
	source.view = *m.view
	return m, source
}

func groupedItem(id, option string) github.Item {
	item := github.Item{ID: id, Content: &github.Content{Title: id}}
	if option != "" {
		item.FieldValues = []github.FieldValue{{FieldID: "group", OptionID: option, Available: true}}
	}
	return item
}

func TestTimelineGroupsUseIDsAndKeepEveryItem(t *testing.T) {
	m, _ := groupedLiveTimelineFixture()
	duplicate := groupedItem("duplicate", "ready")
	duplicate.FieldValues = append(duplicate.FieldValues, duplicate.FieldValues[0])
	unavailable := groupedItem("unavailable", "ready")
	unavailable.FieldValues[0].Available = false
	m.items = []github.Item{groupedItem("done-item", "done"), groupedItem("unset", ""), groupedItem("ready-item", "ready"), groupedItem("unknown", "stale"), duplicate, unavailable}
	groups, err := timelineGroups(m.view, m.items)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 4 || groups[0].Items[0].ID != "ready-item" || groups[1].Items[0].ID != "done-item" || groups[2].Items[0].ID != "unset" || len(groups[3].Items) != 3 {
		t.Fatalf("unexpected groups: %+v", groups)
	}
	items := m.tableItems()
	if len(items) != len(m.items) {
		t.Fatal("grouping dropped rows")
	}
	m.tableFocusID = "done-item"
	m.moveTableRow(1)
	if item, _ := m.selectedItem(); item.ID != "unset" {
		t.Fatal("navigation did not follow rendered group order")
	}
	for _, width := range []int{48, 120} {
		m.width = width
		display := string(m.View().Content)
		for _, text := range []string{"Local groups:", "[Ready]", "[Done]", "[No value]", "[Unavailable group]", "> unset"} {
			if !strings.Contains(display, text) {
				t.Fatalf("missing %q at %d: %s", text, width, display)
			}
		}
	}
}

func TestTimelineGroupingCompatibilityUsesCompleteDefinitions(t *testing.T) {
	for _, mode := range []string{"valid", "vertical", "multiple", "iteration", "stale", "duplicate-option", "duplicate-field"} {
		t.Run(mode, func(t *testing.T) {
			m, _ := groupedLiveTimelineFixture()
			switch mode {
			case "vertical":
				m.view.VerticalGroupBy = m.view.GroupByFields
			case "multiple":
				m.view.GroupByFields = append(m.view.GroupByFields, m.view.GroupByFields[0])
			case "iteration":
				m.view.ProjectFields[2].DataType = "ITERATION"
			case "stale":
				m.view.ProjectFields = m.view.ProjectFields[:2]
			case "duplicate-option":
				m.view.ProjectFields[2].Options = append(m.view.ProjectFields[2].Options, m.view.ProjectFields[2].Options[0])
			case "duplicate-field":
				m.view.ProjectFields = append(m.view.ProjectFields, m.view.ProjectFields[2])
			}
			c := m.viewCompatibility(*m.view)
			if c.supported() != (mode == "valid") {
				t.Fatalf("%s: %s", mode, c.summary())
			}
		})
	}
}

func TestGroupedTimelinePagingSearchRefreshAndReadOnly(t *testing.T) {
	m, source := groupedLiveTimelineFixture()
	first := groupedItem("selected", "done")
	second := groupedItem("new-ready", "ready")
	source.itemPages = map[string]github.ItemsPage{"": {Items: []github.Item{first}, HasNext: true, EndCursor: "next"}, "next": {Items: []github.Item{second}}}
	m = runMembershipReads(t, m, m.startItemsLoad())
	m.tableFocusID = first.ID
	if item, _ := m.selectedItem(); item.ID != first.ID {
		t.Fatal("page insertion lost selection")
	}
	m.filter = "selected"
	if item, _ := m.selectedItem(); item.ID != first.ID || len(m.tableItems()) != 1 {
		t.Fatal("search lost selection")
	}
	m.filter = ""
	source.view.ProjectFields[2].Options[0], source.view.ProjectFields[2].Options[1] = source.view.ProjectFields[2].Options[1], source.view.ProjectFields[2].Options[0]
	m = runMembershipReads(t, m, m.refresh())
	if item, _ := m.selectedItem(); item.ID != first.ID || m.tableItems()[0].ID != first.ID {
		t.Fatal("refresh/order change lost selection")
	}
	for _, key := range []string{"H", "L", "J", "K", "m", "a", "D", "c", "x"} {
		updated, cmd := m.Update(keyPress(key))
		m = updated.(Model)
		if cmd != nil || m.status != "Timelines are read-only" {
			t.Fatalf("write enabled: %s", key)
		}
	}
}

func TestGroupedTimelinePickerAndDirectStartup(t *testing.T) {
	for _, kind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
		for _, direct := range []bool{false, true} {
			fixture, _ := groupedLiveTimelineFixture()
			view := *fixture.view
			item := groupedItem("selected", "ready")
			source := fakePickerSource{view: view, itemPages: map[string]github.ItemsPage{"": {Items: []github.Item{item}}}, detail: github.ItemDetail{ID: item.ID, Content: item.Content}}
			m := NewModelWithHost(source, Selection{}, "github.com")
			m.SetRoadmapMappings(fixture.roadmapMappings)
			owner := github.Owner{Login: "owner", Kind: kind}
			var updatedModel Model
			if direct {
				m.selection = Selection{OwnerLogin: owner.Login, ProjectNumber: 7, ViewNumber: view.Number}
				updated, cmd := m.Update(discoveryMsg{generation: m.generation, view: &view, discovery: github.Discovery{Owners: []github.Owner{owner}, Projects: map[string][]github.Project{owner.Login: {{ID: view.ProjectID, Number: 7}}}}})
				updatedModel = runMembershipReads(t, updated.(Model), cmd)
			} else {
				m.screen = screenViewPicker
				m.selectedOwner = &owner
				m.selectedProject = &github.Project{ID: view.ProjectID, Number: 7}
				m.views = []github.ViewSummary{{ID: view.ID, Number: view.Number, Layout: view.Layout}}
				updated, cmd := m.Update(keyPress("enter"))
				updatedModel = runMembershipReads(t, updated.(Model), cmd)
			}
			if !updatedModel.isTimeline() || updatedModel.screen != screenBoard || len(updatedModel.tableItems()) != 1 {
				t.Fatalf("grouped startup failed: direct=%t kind=%s status=%s", direct, kind, updatedModel.status)
			}
			updated, cmd := updatedModel.Update(keyPress("enter"))
			updatedModel = runMembershipReads(t, updated.(Model), cmd)
			if !updatedModel.detailVisible || updatedModel.detailItemID != item.ID {
				t.Fatal("grouped detail lost selection")
			}
		}
	}
}
