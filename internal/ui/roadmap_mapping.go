package ui

import (
	"fmt"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// SetRoadmapMappings installs an explicit local source without changing the
// GitHub client, cached saved-view metadata, or the live placement gate.
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
	fields, err := github.ResolveRoadmapDateFields(view.ProjectFields, mapping.StartFieldID, mapping.TargetFieldID)
	if err != nil {
		return evaluateViewCompatibilityWithRoadmapReason(view, "invalid user-configured roadmap mapping: "+err.Error())
	}
	reason := fmt.Sprintf("user-configured roadmap mapping: start %q, target %q; live date placement remains unverified; open this view in GitHub", fields.Start.Name, fields.Target.Name)
	return evaluateViewCompatibilityWithRoadmapReason(view, reason)
}
