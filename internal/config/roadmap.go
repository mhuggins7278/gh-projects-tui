package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

// RoadmapMapping explicitly selects date fields for one project view on one host.
// The IDs are opaque GitHub node IDs, not project numbers or field names.
type RoadmapMapping struct {
	Host          string `json:"host"`
	ProjectID     string `json:"project_id"`
	ViewID        string `json:"view_id"`
	StartFieldID  string `json:"start_field_id"`
	TargetFieldID string `json:"target_field_id"`
}

type RoadmapMappings struct {
	Roadmaps []RoadmapMapping `json:"roadmaps"`
}

// LoadRoadmapMappings reads an explicitly supplied mapping file. An empty path
// leaves local mappings disabled; it never discovers or writes a config file.
func LoadRoadmapMappings(path string) (RoadmapMappings, error) {
	if path == "" {
		return RoadmapMappings{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return RoadmapMappings{}, fmt.Errorf("read roadmap mappings %q: %w", path, err)
	}
	defer file.Close()

	// A pointer distinguishes the required array from a missing or null field.
	var document struct {
		Roadmaps *[]RoadmapMapping `json:"roadmaps"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return RoadmapMappings{}, fmt.Errorf("decode roadmap mappings %q: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("unexpected content after the mapping document")
		}
		return RoadmapMappings{}, fmt.Errorf("decode roadmap mappings %q: %w", path, err)
	}
	if document.Roadmaps == nil {
		return RoadmapMappings{}, fmt.Errorf("roadmap mappings %q: roadmaps must be an array", path)
	}

	mappings := RoadmapMappings{Roadmaps: *document.Roadmaps}
	type scope struct{ host, projectID, viewID string }
	seen := make(map[scope]int, len(mappings.Roadmaps))
	for index := range mappings.Roadmaps {
		mapping := &mappings.Roadmaps[index]
		host, ok := roadmapHost(mapping.Host)
		if !ok {
			return RoadmapMappings{}, fmt.Errorf("roadmap mappings %q, entry %d: host must be explicitly supplied", path, index+1)
		}
		mapping.Host = host
		for _, field := range []struct{ name, value string }{
			{"project_id", mapping.ProjectID}, {"view_id", mapping.ViewID},
			{"start_field_id", mapping.StartFieldID}, {"target_field_id", mapping.TargetFieldID},
		} {
			if field.value == "" || strings.IndexFunc(field.value, unicode.IsSpace) >= 0 {
				return RoadmapMappings{}, fmt.Errorf("roadmap mappings %q, entry %d: %s must be a non-empty node ID without whitespace", path, index+1, field.name)
			}
		}
		key := scope{mapping.Host, mapping.ProjectID, mapping.ViewID}
		if previous, ok := seen[key]; ok {
			return RoadmapMappings{}, fmt.Errorf("roadmap mappings %q, entry %d: duplicate host/project/view scope from entry %d", path, index+1, previous)
		}
		seen[key] = index + 1
	}
	return mappings, nil
}

// Find matches an exact project/view scope after applying the usual host
// normalization. It never falls back to another view, project, or host.
func (m RoadmapMappings) Find(host, projectID, viewID string) (RoadmapMapping, bool) {
	if projectID == "" || viewID == "" {
		return RoadmapMapping{}, false
	}
	host, ok := roadmapHost(host)
	if !ok {
		return RoadmapMapping{}, false
	}
	for _, mapping := range m.Roadmaps {
		mappingHost, valid := roadmapHost(mapping.Host)
		if valid && mappingHost == host && mapping.ProjectID == projectID && mapping.ViewID == viewID {
			return mapping, true
		}
	}
	return RoadmapMapping{}, false
}

func roadmapHost(host string) (string, bool) {
	// normalizeHost intentionally defaults empty host values elsewhere. Local
	// mapping entries must instead identify their host explicitly.
	value := strings.TrimSpace(strings.ToLower(host))
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	if index := strings.Index(value, "/"); index >= 0 {
		value = value[:index]
	}
	if value == "" || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return "", false
	}
	return normalizeHost(host), true
}
