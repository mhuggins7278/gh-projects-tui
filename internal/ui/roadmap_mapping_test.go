package ui

import (
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func mappedRoadmapFixture() (github.View, config.RoadmapMappings) {
	view := github.View{ProjectID: "project-id", ID: "view-id", Number: 3, Name: "Timeline", Layout: github.RoadmapLayout, ViewerCanUpdate: true,
		ProjectFields: []github.Field{{ID: "start-id", Name: "Date A", DataType: "DATE"}, {ID: "target-id", Name: "Date B", DataType: "DATE"}},
	}
	mappings := config.RoadmapMappings{Roadmaps: []config.RoadmapMapping{{Host: "github.com", ProjectID: view.ProjectID, ViewID: view.ID, StartFieldID: "start-id", TargetFieldID: "target-id"}}}
	return view, mappings
}

func TestLocalRoadmapMappingValidatesThroughPickerWithoutItemsOrWrites(t *testing.T) {
	for _, invalid := range []string{"", "stale", "non-date", "duplicate"} {
		for _, kind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
			t.Run(invalid+string(kind), func(t *testing.T) {
				view, mappings := mappedRoadmapFixture()
				switch invalid {
				case "stale":
					view.ProjectFields = view.ProjectFields[:1]
				case "non-date":
					view.ProjectFields[1].DataType = "ITERATION"
				case "duplicate":
					view.ProjectFields = append(view.ProjectFields, view.ProjectFields[1])
				}
				viewCalls, itemCalls := 0, 0
				mutations := []string{}
				source := fakePickerSource{view: view, viewCalls: &viewCalls, itemCalls: &itemCalls, mutations: &mutations}
				model := NewModelWithHost(source, Selection{}, "github.com")
				model.screen = screenViewPicker
				model.selectedOwner = &github.Owner{Login: "owner", Kind: kind}
				model.selectedProject = &github.Project{ID: view.ProjectID, Number: 7}
				model.views = []github.ViewSummary{{ID: view.ID, Number: view.Number, Name: view.Name, Layout: view.Layout}}
				model.SetRoadmapMappings(mappings)
				if !strings.Contains(string(model.View().Content), "user-configured mapping") {
					t.Fatal("picker omitted the local source label")
				}
				updated, cmd := model.Update(keyPress("enter"))
				model = updated.(Model)
				if cmd == nil {
					t.Fatal("configured roadmap could not read its definitions")
				}
				updated, cmd = model.Update(cmd())
				model = updated.(Model)
				if cmd != nil || model.screen != screenViewPicker || model.view != nil || model.itemsLoading || viewCalls != 1 || itemCalls != 0 || len(mutations) != 0 {
					t.Fatalf("mapping enabled live loading/writes: screen=%v status=%q", model.screen, model.status)
				}
				if invalid == "" {
					if !strings.Contains(model.status, `start "Date A", target "Date B"`) || !strings.Contains(model.status, "placement remains unverified") {
						t.Fatalf("missing resolved roles/remaining gate: %q", model.status)
					}
				} else if !strings.Contains(model.status, "invalid user-configured roadmap mapping") {
					t.Fatalf("invalid mapping had no explanation: %q", model.status)
				}
			})
		}
	}
}

func TestLocalRoadmapMappingsDoNotLeakAcrossScopesOrOtherLayouts(t *testing.T) {
	view, mappings := mappedRoadmapFixture()
	model := NewModelWithHost(nil, Selection{}, "github.com")
	model.SetRoadmapMappings(mappings)
	// The setter owns its entries rather than observing caller mutations.
	mappings.Roadmaps[0].TargetFieldID = "changed"
	if got := model.viewCompatibility(view).summary(); !strings.Contains(got, "placement remains unverified") {
		t.Fatalf("caller changed an installed mapping: %q", got)
	}
	for _, scope := range []string{"host", "project", "view"} {
		candidate := view
		copy := model
		switch scope {
		case "host":
			copy.host = "enterprise.example"
		case "project":
			candidate.ProjectID = "other-project"
		case "view":
			candidate.ID = "other-view"
		}
		if got := copy.viewCompatibility(candidate).summary(); strings.Contains(got, "user-configured") || !strings.Contains(got, "saved start/target") {
			t.Fatalf("mapping leaked across %s: %q", scope, got)
		}
	}
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		view.Layout = layout
		view.ProjectFields = nil
		if got := model.viewCompatibility(view); !got.supported() {
			t.Fatalf("local mapping affected %s: %q", layout, got.summary())
		}
	}
}

func TestLocalRoadmapMappingDirectSelectionKeepsPlacementGate(t *testing.T) {
	view, mappings := mappedRoadmapFixture()
	owner := github.Owner{Login: "owner", Kind: github.UserOwner}
	source := fakePickerSource{views: []github.ViewSummary{{ID: view.ID, Number: view.Number, Layout: view.Layout}}}
	model := NewModelWithHost(source, Selection{OwnerLogin: owner.Login, ProjectNumber: 7, ViewNumber: view.Number}, "github.com")
	model.SetRoadmapMappings(mappings)
	updated, cmd := model.Update(discoveryMsg{generation: model.generation, view: &view, discovery: github.Discovery{
		Owners: []github.Owner{owner}, Projects: map[string][]github.Project{owner.Login: {{ID: view.ProjectID, Number: 7}}},
	}})
	model = updated.(Model)
	if model.screen != screenViewPicker || model.view != nil || model.itemsLoading || !strings.Contains(model.viewPickerNotice, "user-configured") {
		t.Fatalf("direct selection bypassed local-source gate: %q", model.viewPickerNotice)
	}
	if cmd == nil {
		t.Fatal("direct selection did not list alternative saved views")
	}
	updated, next := model.Update(cmd())
	model = updated.(Model)
	if next != nil || model.screen != screenViewPicker || model.itemsLoading || !strings.Contains(model.viewPickerNotice, "placement remains unverified") {
		t.Fatal("view listing discarded the remaining placement gate")
	}
}

func TestRoadmapGroupingIsExplicitlyRejected(t *testing.T) {
	field := github.Field{ID: "group-id", Name: "Group", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "option", Name: "Option"}}}
	for _, axis := range []string{"horizontal", "vertical", "both"} {
		t.Run(axis, func(t *testing.T) {
			view, mappings := mappedRoadmapFixture()
			if axis != "vertical" {
				view.GroupByFields = []github.Field{field}
			}
			if axis != "horizontal" {
				view.VerticalGroupBy = []github.Field{field}
			}
			model := NewModelWithHost(nil, Selection{}, "github.com")
			for _, mapped := range []bool{false, true} {
				if mapped {
					model.SetRoadmapMappings(mappings)
				}
				compatibility := model.viewCompatibility(view)
				if compatibility.supported() || !strings.Contains(compatibility.summary(), "roadmap grouping is not supported") {
					t.Fatalf("%s grouping admitted or hidden (mapped=%t): %s", axis, mapped, compatibility.summary())
				}
			}
		})
	}
}
