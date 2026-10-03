package ui

import (
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestUnmappedRoadmapPickerClearlyMarksUnsupported(t *testing.T) {
	view, _ := mappedRoadmapFixture()
	model := NewModelWithHost(nil, Selection{}, "github.com")
	model.screen = screenViewPicker
	model.selectedOwner = &github.Owner{Login: "owner", Kind: github.UserOwner}
	model.selectedProject = &github.Project{ID: view.ProjectID, Number: 7, Title: "Project"}
	model.views = []github.ViewSummary{{ID: view.ID, Number: view.Number, Name: view.Name, Layout: view.Layout}}

	content := string(model.View().Content)
	for _, want := range []string{"Roadmap views are unsupported in this release", "UNSUPPORTED"} {
		if !strings.Contains(content, want) {
			t.Fatalf("roadmap picker omitted %q: %s", want, content)
		}
	}
	if strings.Contains(content, "mapping") || strings.Contains(content, "roadmaps.json") {
		t.Fatalf("picker exposed endpoint configuration details: %s", content)
	}
}

func TestLocalRoadmapMappingDirectSelectionLoadsTimeline(t *testing.T) {
	view, mappings := mappedRoadmapFixture()
	owner := github.Owner{Login: "owner", Kind: github.UserOwner}
	source := fakePickerSource{}
	model := NewModelWithHost(source, Selection{OwnerLogin: owner.Login, ProjectNumber: 7, ViewNumber: view.Number}, "github.com")
	model.setRoadmapMappings(mappings)
	updated, cmd := model.Update(discoveryMsg{generation: model.generation, view: &view, discovery: github.Discovery{
		Owners: []github.Owner{owner}, Projects: map[string][]github.Project{owner.Login: {{ID: view.ProjectID, Number: 7}}},
	}})
	model = updated.(Model)
	if cmd == nil || model.screen != screenBoard || !model.isTimeline() || !model.itemsLoading {
		t.Fatal("valid direct selection did not load the timeline")
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
					model.setRoadmapMappings(mappings)
				}
				compatibility := model.viewCompatibility(view)
				if compatibility.supported() || !strings.Contains(compatibility.summary(), "roadmap grouping is not supported") {
					t.Fatalf("%s grouping admitted or hidden (mapped=%t): %s", axis, mapped, compatibility.summary())
				}
			}
		})
	}
}
