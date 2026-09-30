package github

import (
	"fmt"
	"strings"
)

// RoadmapDateFields identifies explicit endpoint roles. Resolving field IDs
// does not establish their source, saved-view identity, or placement semantics.
type RoadmapDateFields struct {
	Start, Target Field
}

// ResolveRoadmapDateFields requires complete project definitions and explicitly
// supplied IDs. Display fields, names, order, and populated values are not
// fallbacks. A source must supply both field roles before either is resolved.
func ResolveRoadmapDateFields(fields []Field, startID, targetID string) (RoadmapDateFields, error) {
	if strings.TrimSpace(startID) == "" || strings.TrimSpace(targetID) == "" {
		return RoadmapDateFields{}, fmt.Errorf("roadmap mapping requires explicit start and target field IDs")
	}
	resolve := func(id, role string) (Field, error) {
		var result Field
		matches := 0
		for _, field := range fields {
			if field.ID == id {
				result = field
				matches++
			}
		}
		if matches == 0 {
			return Field{}, fmt.Errorf("roadmap %s field ID is absent from this project's definitions; reselect its field", role)
		}
		if matches != 1 {
			return Field{}, fmt.Errorf("roadmap %s field ID resolves to multiple definitions", role)
		}
		if result.DataType != "DATE" {
			return Field{}, fmt.Errorf("roadmap %s field must be DATE, got %q", role, result.DataType)
		}
		return result, nil
	}
	start, err := resolve(startID, "start")
	if err != nil {
		return RoadmapDateFields{}, err
	}
	target, err := resolve(targetID, "target")
	if err != nil {
		return RoadmapDateFields{}, err
	}
	return RoadmapDateFields{Start: start, Target: target}, nil
}
