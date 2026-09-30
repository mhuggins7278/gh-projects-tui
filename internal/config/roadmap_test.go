package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validRoadmapEntry = `{"host":"github.com","project_id":"PVT_project","view_id":"PVTV_view","start_field_id":"PVTF_start","target_field_id":"PVTF_target"}`

func roadmapFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roadmaps.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRoadmapMappingsExplicitOptIn(t *testing.T) {
	mappings, err := LoadRoadmapMappings("")
	if err != nil || len(mappings.Roadmaps) != 0 {
		t.Fatalf("disabled mappings = %+v, %v", mappings, err)
	}
	mappings, err = LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[]}`))
	if err != nil || len(mappings.Roadmaps) != 0 {
		t.Fatalf("empty mappings = %+v, %v", mappings, err)
	}
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, err := LoadRoadmapMappings(path); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("missing file error = %v", err)
	}
}

func TestRoadmapMappingsMatchHostProjectAndView(t *testing.T) {
	entries := []string{
		strings.Replace(validRoadmapEntry, `"github.com"`, `" HTTPS://GitHub.COM/ "`, 1),
		strings.Replace(validRoadmapEntry, `"github.com"`, `"enterprise.example"`, 1),
		strings.Replace(validRoadmapEntry, `"PVTV_view"`, `"PVTV_other"`, 1),
		strings.Replace(validRoadmapEntry, `"PVT_project"`, `"PVT_other"`, 1),
	}
	mappings, err := LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[`+strings.Join(entries, ",")+`]}`))
	if err != nil {
		t.Fatal(err)
	}
	if mappings.Roadmaps[0].Host != "github.com" {
		t.Fatalf("normalized host = %q", mappings.Roadmaps[0].Host)
	}
	for _, test := range []struct {
		host, project, view string
		want                bool
	}{
		{"http://GITHUB.com/", "PVT_project", "PVTV_view", true},
		{"enterprise.example", "PVT_project", "PVTV_view", true},
		{"github.com", "PVT_other", "PVTV_view", true},
		{"github.com", "PVT_project", "PVTV_other", true},
		{"unknown.example", "PVT_project", "PVTV_view", false},
		{"github.com", "PVT_missing", "PVTV_view", false},
		{"github.com", "PVT_project", "PVTV_missing", false},
		{"github.com", "pvt_project", "PVTV_view", false},
		{"", "PVT_project", "PVTV_view", false},
	} {
		mapping, found := mappings.Find(test.host, test.project, test.view)
		if found != test.want || found && (mapping.ProjectID != test.project || mapping.ViewID != test.view) {
			t.Errorf("Find(%q, %q, %q) = %+v, %v; want found %v", test.host, test.project, test.view, mapping, found, test.want)
		}
	}
	invalid := RoadmapMappings{Roadmaps: []RoadmapMapping{{ProjectID: "PVT_project", ViewID: "PVTV_view"}}}
	if _, found := invalid.Find("github.com", "PVT_project", "PVTV_view"); found {
		t.Fatal("missing entry host defaulted to github.com")
	}
	invalid = RoadmapMappings{Roadmaps: []RoadmapMapping{{Host: "github.com"}}}
	if _, found := invalid.Find("github.com", "", ""); found {
		t.Fatal("missing project/view identity matched an unscoped mapping")
	}
}

func TestLoadRoadmapMappingsRejectsMalformedDocuments(t *testing.T) {
	for _, test := range []struct{ name, content, errorText string }{
		{"invalid JSON", `{"roadmaps":`, "decode roadmap mappings"},
		{"unknown root field", `{"roadmaps":[],"extra":true}`, `unknown field "extra"`},
		{"unknown entry field", `{"roadmaps":[` + strings.Replace(validRoadmapEntry, `"host":`, `"extra":true,"host":`, 1) + `]}`, `unknown field "extra"`},
		{"trailing document", `{"roadmaps":[]} {}`, "unexpected content"},
		{"trailing invalid JSON", `{"roadmaps":[]} garbage`, "decode roadmap mappings"},
		{"missing array", `{}`, "roadmaps must be an array"},
		{"null document", `null`, "roadmaps must be an array"},
		{"null array", `{"roadmaps":null}`, "roadmaps must be an array"},
		{"object array", `{"roadmaps":{}}`, "decode roadmap mappings"},
		{"null entry", `{"roadmaps":[null]}`, "entry 1: host"},
		{"duplicate scope", `{"roadmaps":[` + validRoadmapEntry + `,` + strings.Replace(validRoadmapEntry, `"github.com"`, `"HTTPS://GITHUB.COM/"`, 1) + `]}`, "duplicate host/project/view scope from entry 1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := LoadRoadmapMappings(roadmapFile(t, test.content)); err == nil || !strings.Contains(err.Error(), test.errorText) {
				t.Fatalf("error = %v; want %q", err, test.errorText)
			}
		})
	}
}

func TestLoadRoadmapMappingsRequiresExplicitFields(t *testing.T) {
	missingHost := strings.Replace(validRoadmapEntry, `"host":"github.com",`, "", 1)
	if _, err := LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[`+missingHost+`]}`)); err == nil || !strings.Contains(err.Error(), "host must be explicitly supplied") {
		t.Errorf("missing host: error = %v", err)
	}
	for _, host := range []string{"", " ", "/", "https://", "git hub.com"} {
		entry := strings.Replace(validRoadmapEntry, `"github.com"`, `"`+host+`"`, 1)
		if _, err := LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[`+entry+`]}`)); err == nil || !strings.Contains(err.Error(), "host must be explicitly supplied") {
			t.Errorf("host %q: error = %v", host, err)
		}
	}
	for _, field := range []struct{ name, value string }{
		{"project_id", "PVT_project"}, {"view_id", "PVTV_view"},
		{"start_field_id", "PVTF_start"}, {"target_field_id", "PVTF_target"},
	} {
		for _, value := range []string{"", " " + field.value, field.value + " ", "bad id", "bad\\tid", "bad\u00a0id"} {
			entry := strings.Replace(validRoadmapEntry, `"`+field.value+`"`, `"`+value+`"`, 1)
			if _, err := LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[`+entry+`]}`)); err == nil || !strings.Contains(err.Error(), field.name) {
				t.Errorf("%s %q: error = %v", field.name, value, err)
			}
		}
		entry := strings.Replace(validRoadmapEntry, `,"`+field.name+`":"`+field.value+`"`, "", 1)
		if _, err := LoadRoadmapMappings(roadmapFile(t, `{"roadmaps":[`+entry+`]}`)); err == nil || !strings.Contains(err.Error(), field.name) {
			t.Errorf("missing %s: error = %v", field.name, err)
		}
	}
}
