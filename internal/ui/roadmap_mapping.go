package ui

import (
	"strings"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// SetRoadmapMappings installs an explicit local source without changing the
// GitHub client or cached saved-view metadata.
func (m *Model) SetRoadmapMappings(mappings config.RoadmapMappings) {
	m.roadmapMappings.Roadmaps = append([]config.RoadmapMapping(nil), mappings.Roadmaps...)
}

func (m Model) viewCompatibility(view github.View) viewCompatibility {
	if view.Layout != github.RoadmapLayout {
		return evaluateViewCompatibility(view)
	}
	mapping, found := m.roadmapMappings.Find(m.host, view.ProjectID, view.ID)
	if !found {
		return evaluateViewCompatibility(view)
	}
	_, err := github.ResolveRoadmapDateFields(view.ProjectFields, mapping.StartFieldID, mapping.TargetFieldID)
	if err != nil {
		return evaluateViewCompatibilityWithRoadmapReason(view, "invalid user-configured roadmap mapping: "+err.Error())
	}
	compatibility := evaluateViewCompatibilityWithRoadmapReason(view, "")
	filter := strings.TrimSpace(view.Filter)
	if filter != "" && filter != "is:issue" {
		compatibility.reasons = append(compatibility.reasons, "roadmap saved filter remains unverified; supported filters are empty or is:issue")
	}
	if !supportedRoadmapSort(view, mapping.StartFieldID) {
		compatibility.reasons = append(compatibility.reasons, "roadmap saved sorting remains unverified; supported sorting is none or one ascending mapped start DATE field")
	}
	return compatibility
}

// The populated roadmap comparison verifies start-DATE ASC, unset last, and
// project-position ties. It does not establish DESC or secondary-sort behavior.
func supportedRoadmapSort(view github.View, startID string) bool {
	if len(view.SortByFields) == 0 {
		return true
	}
	if len(view.SortByFields) != 1 {
		return false
	}
	sort := view.SortByFields[0]
	return sort.Field.ID == startID && strings.EqualFold(sort.Field.DataType, "DATE") && strings.EqualFold(sort.Direction, "ASC")
}
