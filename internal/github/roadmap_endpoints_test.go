package github

import (
	"reflect"
	"strings"
	"testing"
)

func TestRoadmapDateFieldsRequireExplicitUniqueProjectIDs(t *testing.T) {
	start := Field{ID: "start-id", Name: "Arbitrary A", DataType: "DATE"}
	target := Field{ID: "target-id", Name: "Arbitrary B", DataType: "DATE"}
	for _, tc := range []struct {
		name           string
		fields         []Field
		startID, endID string
		wantError      string
	}{
		{"missing mapping", []Field{start, target}, "", "", "explicit start and target"},
		{"partial mapping", []Field{start, target}, start.ID, "", "explicit start and target"},
		{"blank mapping", []Field{start, target}, " ", target.ID, "explicit start and target"},
		{"no definitions", nil, start.ID, target.ID, "start field ID is absent"},
		{"stale start", []Field{target}, start.ID, target.ID, "start field ID is absent"},
		{"stale target", []Field{start}, start.ID, target.ID, "target field ID is absent"},
		{"duplicate start ID", []Field{start, start, target}, start.ID, target.ID, "start field ID resolves to multiple"},
		{"duplicate target ID", []Field{start, target, target}, start.ID, target.ID, "target field ID resolves to multiple"},
		{"field name is not an ID", []Field{start, target}, start.Name, target.ID, "start field ID is absent"},
		{"unsupported start", []Field{{ID: start.ID, DataType: "ITERATION"}, target}, start.ID, target.ID, "start field must be DATE"},
		{"unsupported target", []Field{start, {ID: target.ID, DataType: "TEXT"}}, start.ID, target.ID, "target field must be DATE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRoadmapDateFields(tc.fields, tc.startID, tc.endID)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) || !reflect.DeepEqual(got, RoadmapDateFields{}) {
				t.Fatalf("mapping=%#v error=%v, want %q with no partial result", got, err, tc.wantError)
			}
		})
	}
	// Renaming or reordering definitions cannot change explicit endpoint roles.
	start.Name, target.Name = "Target", "Start"
	got, err := ResolveRoadmapDateFields([]Field{target, start}, start.ID, target.ID)
	if err != nil || !reflect.DeepEqual(got, RoadmapDateFields{Start: start, Target: target}) {
		t.Fatalf("explicit roles changed: %#v, %v", got, err)
	}
	// Explicitly selecting one field for both roles is not the same as guessing
	// that a project's only date field is its start/target mapping.
	got, err = ResolveRoadmapDateFields([]Field{start}, start.ID, start.ID)
	if err != nil || got.Start.ID != start.ID || got.Target.ID != start.ID {
		t.Fatalf("explicit shared field rejected: %#v, %v", got, err)
	}
}
