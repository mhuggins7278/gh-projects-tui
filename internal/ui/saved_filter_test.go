package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type savedFilterSource struct {
	fakePickerSource
	filters, cursors []string
}

func (s *savedFilterSource) PageItems(ctx context.Context, owner github.Owner, project int, filter, after string) (github.ItemsPage, error) {
	s.filters = append(s.filters, filter)
	s.cursors = append(s.cursors, after)
	return s.fakePickerSource.PageItems(ctx, owner, project, filter, after)
}

func savedFilterModel(layout github.ViewLayout, ownerKind github.OwnerKind, filter string, fields []github.Field) (Model, *savedFilterSource) {
	view, mappings := mappedRoadmapFixture()
	view.Layout, view.Filter = layout, filter
	if layout != github.RoadmapLayout {
		view.ProjectFields = nil
	}
	view.ProjectFields = append(view.ProjectFields, fields...)
	source := &savedFilterSource{fakePickerSource: fakePickerSource{
		view: view,
		itemPagesByFilter: map[string]map[string]github.ItemsPage{filter: {
			"":     {Items: []github.Item{{ID: "first"}}, HasNext: true, EndCursor: "next"},
			"next": {Items: []github.Item{{ID: "second"}}},
		}},
	}}
	model := NewModelWithHost(source, Selection{}, "github.com")
	model.setRoadmapMappings(mappings)
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "owner", Kind: ownerKind}
	model.selectedProject = &github.Project{ID: view.ProjectID, Number: 7}
	return model, source
}

// Test the loading contract, not GitHub's grammar: even unfamiliar or malformed
// expressions must reach GitHub unchanged so the server determines the result.
func TestSavedViewsForwardOpaqueFiltersThroughLoadingAndRefresh(t *testing.T) {
	cases := []struct {
		name, filter string
		fields       []github.Field
	}{
		{name: "empty"},
		{name: "whitespace", filter: " \t\n"},
		{name: "parent issue regression", filter: `parent-issue:"glg/5mp#47" is:open`},
		{name: "custom presence without metadata", filter: "has:otc-sprint"},
		{name: "custom presence with metadata", filter: "has:otc-sprint", fields: []github.Field{{Name: "OTC Sprint", DataType: "TEXT"}}},
		{name: "custom fields", filter: `note:"hello world" target:>=@today teams:Core,Platform`, fields: []github.Field{{Name: "Note", DataType: "TEXT"}, {Name: "Target", DataType: "DATE"}, {Name: "Teams", DataType: "MULTI_SELECT"}}},
		{name: "arbitrary names", filter: `"Estimate (days)":>=2 has:équipe_🚀`, fields: []github.Field{{Name: "Estimate (days)", DataType: "NUMBER"}, {Name: "Équipe_🚀", DataType: "TEXT"}}},
		{name: "colliding names", filter: "has:phase", fields: []github.Field{{Name: "Phase", DataType: "SINGLE_SELECT"}, {Name: "phase", DataType: "TEXT"}}},
		{name: "relative iterations", filter: "-iteration:@next sprint:@current+3"},
		{name: "builtins", filter: `reviewers:@me milestone:"QA release" repo:owner/one,owner/two`},
		{name: "quoted punctuation", filter: `status:"Work, blocked" title:"quote\"value"`},
		{name: "future syntax", filter: "new-qualifier:some-value"},
		{name: "malformed syntax", filter: `status:"unterminated`},
		{name: "boolean syntax", filter: `(status:Todo OR label:bug)`},
		{name: "surrounding whitespace", filter: "\tclosed:>=2026-09-24 is:closed  \n"},
	}
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout, github.RoadmapLayout} {
		for _, ownerKind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
			for _, test := range cases {
				t.Run(string(layout)+"/"+string(ownerKind)+"/"+test.name, func(t *testing.T) {
					model, source := savedFilterModel(layout, ownerKind, test.filter, test.fields)
					updated, cmd := model.Update(viewDetailMsg{scope: model.currentPickerReadScope(), generation: model.generation, view: &source.view})
					model = updated.(Model)
					if cmd == nil || model.screen != screenBoard || !model.itemsLoading {
						t.Fatalf("saved filter blocked loading: %s", model.status)
					}
					model = runMembershipReads(t, model, cmd)
					if model.itemsErr != nil || model.itemsLoading || len(model.items) != 2 {
						t.Fatalf("items=%v loading=%v error=%v", model.items, model.itemsLoading, model.itemsErr)
					}
					model = runMembershipReads(t, model, model.refresh())
					if model.screen != screenBoard || model.itemsErr != nil || model.itemsLoading || len(model.items) != 2 {
						t.Fatalf("refresh failed: %s, %v", model.status, model.itemsErr)
					}
					if !reflect.DeepEqual(source.filters, []string{test.filter, test.filter, test.filter, test.filter}) || !reflect.DeepEqual(source.cursors, []string{"", "next", "", "next"}) {
						t.Fatalf("queries changed: filters=%q cursors=%q", source.filters, source.cursors)
					}
				})
			}
		}
	}
}

func TestSavedFilterAPIErrorsSurfaceWithoutUnfilteredFallback(t *testing.T) {
	const filter = "has:otc-sprint"
	apiErr := errors.New("GitHub rejected the query")
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout, github.RoadmapLayout} {
		for _, failedCursor := range []string{"", "next"} {
			t.Run(string(layout)+"/"+failedCursor, func(t *testing.T) {
				model, source := savedFilterModel(layout, github.OrganizationOwner, filter, nil)
				source.itemErrorsByFilter = map[string]map[string]error{filter: {failedCursor: apiErr}}
				updated, cmd := model.Update(viewDetailMsg{scope: model.currentPickerReadScope(), generation: model.generation, view: &source.view})
				model = runMembershipReads(t, updated.(Model), cmd)
				wantFilters, wantItems := []string{filter}, 0
				if failedCursor != "" {
					wantFilters, wantItems = []string{filter, filter}, 1
				}
				if !errors.Is(model.itemsErr, apiErr) || model.itemsLoading || len(model.items) != wantItems || !reflect.DeepEqual(source.filters, wantFilters) {
					t.Fatalf("error lost or query retried: error=%v items=%v filters=%q", model.itemsErr, model.items, source.filters)
				}
				if !strings.Contains(string(model.View().Content), apiErr.Error()) {
					t.Fatal("item error was not displayed")
				}
				source.itemErrorsByFilter = nil
				model = runMembershipReads(t, model, model.refresh())
				wantFilters = append(wantFilters, filter, filter)
				if model.itemsErr != nil || model.itemsLoading || len(model.items) != 2 || !reflect.DeepEqual(source.filters, wantFilters) {
					t.Fatalf("retry changed query or failed: error=%v filters=%q", model.itemsErr, source.filters)
				}
			})
		}
	}
}
