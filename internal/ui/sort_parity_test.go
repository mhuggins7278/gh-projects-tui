package ui

import (
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
	"reflect"
	"testing"
)

// These fixtures pin the client's disclosed ordering policy. They are not a
// substitute for comparing populated saved views with GitHub's web renderer.
func TestSavedSortNullPlacementAndStablePositionTies(t *testing.T) {
	cases := []struct {
		name   string
		field  github.Field
		values []github.FieldValue
	}{
		{"number", github.Field{ID: "f", DataType: "NUMBER"}, []github.FieldValue{{Value: "2"}, {Value: "1"}, {Value: "2"}}},
		{"date", github.Field{ID: "f", DataType: "DATE"}, []github.FieldValue{{Value: "2026-10-05"}, {Value: "2026-10-01"}, {Value: "2026-10-05"}}},
		{"text", github.Field{ID: "f", DataType: "TEXT"}, []github.FieldValue{{Value: "B"}, {Value: "a"}, {Value: "b"}}},
		{"single", github.Field{ID: "f", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "low", Name: "Low"}, {ID: "high", Name: "High"}}}, []github.FieldValue{{OptionID: "high", Value: "High"}, {OptionID: "low", Value: "Low"}, {OptionID: "high", Value: "High"}}},
		{"iteration", github.Field{ID: "f", DataType: "ITERATION", Iterations: []github.Iteration{{ID: "new", StartDate: "2026-10-05"}, {ID: "old", StartDate: "2026-09-01", Completed: true}}}, []github.FieldValue{{IterationID: "new", Value: "Current"}, {IterationID: "old", Value: "Completed"}, {IterationID: "new", Value: "Current"}}},
	}
	for _, tc := range cases {
		for _, direction := range []string{"ASC", "DESC"} {
			t.Run(tc.name+"/"+direction, func(t *testing.T) {
				items := []github.Item{{ID: "unset"}, {ID: "first"}, {ID: "low"}, {ID: "tie"}, {ID: "unavailable", FieldValues: []github.FieldValue{{FieldID: "f", Value: "0", Available: false}}}}
				for i, v := range tc.values {
					v.FieldID = "f"
					v.Available = true
					items[i+1].FieldValues = []github.FieldValue{v}
				}
				view := github.View{SortByFields: []github.SortField{{Field: tc.field, Direction: direction}}}
				ordered := sortedItemsForView(&view, items)
				ids := []string{}
				for _, item := range ordered {
					ids = append(ids, item.ID)
				}
				want := []string{"low", "first", "tie", "unset", "unavailable"}
				if direction == "DESC" {
					want = []string{"first", "tie", "low", "unset", "unavailable"}
				}
				if !reflect.DeepEqual(ids, want) {
					t.Fatalf("order=%v", ids)
				}
				if items[0].ID != "unset" {
					t.Fatal("sort mutated canonical project position")
				}
			})
		}
	}
}

func TestSavedSecondarySortAndUnknownIteration(t *testing.T) {
	number := github.Field{ID: "n", DataType: "NUMBER"}
	title := github.Field{Name: "Title", DataType: "TITLE"}
	view := github.View{SortByFields: []github.SortField{{Field: number, Direction: "ASC"}, {Field: title, Direction: "DESC"}}}
	items := []github.Item{{ID: "a", Content: &github.Content{Title: "Alpha"}, FieldValues: []github.FieldValue{{FieldID: "n", Value: "2", Available: true}}}, {ID: "z", Content: &github.Content{Title: "Zebra"}, FieldValues: []github.FieldValue{{FieldID: "n", Value: "2", Available: true}}}, {ID: "empty", Content: &github.Content{Title: "Middle"}}}
	got := sortedItemsForView(&view, items)
	if got[0].ID != "z" || got[1].ID != "a" || got[2].ID != "empty" {
		t.Fatal(got)
	}
	iteration := github.Field{ID: "iteration", DataType: "ITERATION", Iterations: []github.Iteration{{ID: "known", StartDate: "2026-10-05"}}}
	unknown := github.Item{FieldValues: []github.FieldValue{{FieldID: "iteration", IterationID: "deleted", Value: "Deleted", Available: true}}}
	if itemSortValueFor(iteration, unknown).present {
		t.Fatal("unknown iteration assigned an invented date")
	}
}
