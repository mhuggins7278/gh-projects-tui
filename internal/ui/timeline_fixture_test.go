package ui

import "github.com/mhuggins7278/gh-projects-tui/internal/github"

func mappedRoadmapFixture() (github.View, roadmapMappings) {
	view := github.View{ProjectID: "project-id", ID: "view-id", Number: 3, Name: "Timeline", Layout: github.RoadmapLayout, ViewerCanUpdate: true,
		ProjectFields: []github.Field{{ID: "start-id", Name: "Date A", DataType: "DATE"}, {ID: "target-id", Name: "Date B", DataType: "DATE"}},
	}
	mappings := roadmapMappings{Roadmaps: []roadmapMapping{{Host: "github.com", ProjectID: view.ProjectID, ViewID: view.ID, StartFieldID: "start-id", TargetFieldID: "target-id"}}}
	return view, mappings
}

func fixtureRoadmapMapping() roadmapMappings {
	return roadmapMappings{Roadmaps: []roadmapMapping{{Host: "github.com", ProjectID: "project-id", ViewID: "view-id", StartFieldID: "start-id", TargetFieldID: "target-id"}}}
}
