package ui

import (
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
	"reflect"
	"testing"
)

func multiSelectFixture() (github.Field, []github.Item) {
	field := github.Field{ID: "tags", Name: "Tags", DataType: "MULTI_SELECT", Options: []github.FieldOption{{ID: "a", Name: "One, two"}, {ID: "b", Name: "Three"}}}
	items := []github.Item{{ID: "both", Content: &github.Content{Kind: "Issue", Title: "Both"}, FieldValues: []github.FieldValue{{FieldID: "tags", Available: true, Value: "One, two, Three", Options: []github.FieldOption{{ID: "b", Name: "Three"}, {ID: "a", Name: "One, two"}, {ID: "a", Name: "One, two"}}}}}, {ID: "none"}, {ID: "unknown", FieldValues: []github.FieldValue{{FieldID: "tags", Available: true, Options: []github.FieldOption{{ID: "removed"}}}}}}
	return field, items
}

func TestMultiSelectGroupsPreserveEveryMembershipWithoutDuplicateOptions(t *testing.T) {
	field, items := multiSelectFixture()
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		view := github.View{Layout: layout, GroupByFields: []github.Field{field}, ViewerCanUpdate: true}
		if c := evaluateViewCompatibility(view); !c.supported() {
			t.Fatal(c.summary())
		}
		lanes := lanesForView(&view, items)
		want := map[string]string{"no-value": "none", "option:a": "both", "option:b": "both", "other": "unknown"}
		for _, lane := range lanes {
			if len(lane.Items) != 1 || want[lane.Key] != lane.Items[0].ID {
				t.Fatalf("lane %#v", lane)
			}
		}
		if len(lanes) != 4 {
			t.Fatal(lanes)
		}
		if layout == github.TableLayout {
			m := tableActionModel(fakePickerSource{})
			m.view, m.items = &view, items
			if ids := tableRowIDs(m); !reflect.DeepEqual(ids, []string{"none", "both", "both", "unknown"}) {
				t.Fatal(ids)
			}
			m.moveTableRow(1)
			m.moveTableRow(1)
			if m.tableRow != 2 || m.selectedTableRow(m.tableItems()) != 2 {
				t.Fatal("focus jumped to first duplicate")
			}
			m.moveTableRow(1)
			if m.tableRow != 3 {
				t.Fatal("could not navigate past duplicate")
			}
		}
		if _, err := resolveBoardMutationPlan(view, items, boardMutationIntent{kind: boardMutationReorder, itemID: "both", direction: 1}); err == nil {
			t.Fatal("multi-select reorder allowed")
		}
	}
}

func TestMultiSelectCombinedGroupsAndBoardFocus(t *testing.T) {
	field, items := multiSelectFixture()
	vertical := field
	vertical.ID = "vertical"
	items[0].FieldValues = append(items[0].FieldValues, github.FieldValue{FieldID: "vertical", Available: true, Options: field.Options})
	view := github.View{Layout: github.BoardLayout, GroupByFields: []github.Field{field}, VerticalGroupBy: []github.Field{vertical}}
	lanes := lanesForView(&view, items[:1])
	memberships := 0
	for _, lane := range lanes {
		memberships += len(lane.Items)
	}
	if memberships != 4 {
		t.Fatalf("combined membership=%d", memberships)
	}
	m := tableActionModel(fakePickerSource{})
	m.view, m.items = &view, items[:1]
	for index, lane := range lanes {
		if len(lane.Items) > 0 {
			m.boardLane = index
		}
	}
	m.boardFocusID = "both"
	lane := m.boardLane
	m.clampBoardCursor()
	if m.boardLane != lane {
		t.Fatal("focus jumped between duplicate lanes")
	}
}
