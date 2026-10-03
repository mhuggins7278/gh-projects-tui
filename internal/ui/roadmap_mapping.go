package ui

import (
	"strings"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type roadmapMapping struct {
	Host, ProjectID, ViewID     string
	StartFieldID, TargetFieldID string
}

type roadmapMappings struct{ Roadmaps []roadmapMapping }

func (m roadmapMappings) Find(host, projectID, viewID string) (roadmapMapping, bool) {
	for _, mapping := range m.Roadmaps {
		if mapping.Host == host && mapping.ProjectID == projectID && mapping.ViewID == viewID {
			return mapping, true
		}
	}
	return roadmapMapping{}, false
}

// setRoadmapMappings is kept package-private for renderer compatibility tests.
// The application no longer loads endpoint choices from user files.
func (m *Model) setRoadmapMappings(mappings roadmapMappings) {
	m.roadmapMappings.Roadmaps = append([]roadmapMapping(nil), mappings.Roadmaps...)
}

func (m Model) viewCompatibility(view github.View) viewCompatibility {
	if view.Layout != github.RoadmapLayout {
		return evaluateViewCompatibility(view)
	}
	mapping, found := m.roadmapMappings.Find(m.host, view.ProjectID, view.ID)
	if !found {
		return evaluateViewCompatibility(view)
	}
	_, err := github.ResolveRoadmapFields(view.ProjectFields, mapping.StartFieldID, mapping.TargetFieldID)
	if err != nil {
		return evaluateViewCompatibilityWithRoadmapReason(view, "invalid roadmap endpoint selection: "+err.Error())
	}
	_, groupErr := timelineGroupField(&view)
	compatibility := evaluateViewCompatibilityWithRoadmapGrouping(view, "", groupErr == nil)
	if groupErr != nil && (len(view.GroupByFields) > 0 || len(view.VerticalGroupBy) > 0) {
		compatibility.reasons = append(compatibility.reasons, groupErr.Error())
	}
	if !supportedRoadmapSort(view) {
		compatibility.reasons = append(compatibility.reasons, "timeline sorting requires at most two supported distinct fields with consistent definitions")
	}
	return compatibility
}

// Share board/table sort semantics: stable project-position ties and unset last.
func supportedRoadmapSort(view github.View) bool {
	if len(view.SortByFields) > 2 {
		return false
	}
	seen := map[string]bool{}
	for _, sort := range view.SortByFields {
		if !supportedSortDirection(sort.Direction) || !supportedSortField(sort.Field) {
			return false
		}
		if sort.Field.ID == "" {
			continue
		}
		if seen[sort.Field.ID] {
			return false
		}
		seen[sort.Field.ID] = true
		count := 0
		for _, field := range view.ProjectFields {
			if field.ID == sort.Field.ID {
				count++
				if !strings.EqualFold(field.DataType, sort.Field.DataType) {
					return false
				}
			}
		}
		if count != 1 {
			return false
		}
	}
	return true
}
