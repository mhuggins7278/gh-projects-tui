package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Selection holds non-sensitive identifiers only. Titles, bodies, field
// values, and tokens must never be persisted here.
type Selection struct {
	Owner   string `json:"owner,omitempty"`
	Project int    `json:"project,omitempty"`
	View    int    `json:"view,omitempty"`
}

type fileShape struct {
	Hosts map[string]Selection `json:"hosts"`
}

// DefaultHost mirrors go-gh host resolution minimally: GH_HOST (or
// GH_ENTERPRISE_HOST) wins, otherwise github.com. The value is normalized to
// lowercase without scheme or trailing slash.
func DefaultHost() string {
	for _, key := range []string{"GH_HOST", "GH_ENTERPRISE_HOST"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return normalizeHost(value)
		}
	}
	return "github.com"
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	if idx := strings.Index(host, "/"); idx >= 0 {
		host = host[:idx]
	}
	if host == "" {
		return "github.com"
	}
	return host
}

// Dir returns the configuration directory. GH_PROJECTS_TUI_CONFIG_DIR
// overrides the XDG location for tests.
func Dir() (string, error) {
	if override := strings.TrimSpace(os.Getenv("GH_PROJECTS_TUI_CONFIG_DIR")); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gh-projects-tui"), nil
}

func filePath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load returns the remembered selection for host, or an empty Selection when
// nothing was stored.
func Load(host string) (Selection, error) {
	host = normalizeHost(host)
	path, err := filePath()
	if err != nil {
		return Selection{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Selection{}, nil
		}
		return Selection{}, err
	}
	var shape fileShape
	if err := json.Unmarshal(data, &shape); err != nil {
		return Selection{}, err
	}
	if shape.Hosts == nil {
		return Selection{}, nil
	}
	return shape.Hosts[host], nil
}

// Save remembers owner/project/view identifiers for host. Empty owner clears
// the entry's descendants per ancestor-discards-descendants rule.
func Save(host string, sel Selection) error {
	host = normalizeHost(host)
	path, err := filePath()
	if err != nil {
		return err
	}
	var shape fileShape
	if data, readErr := os.ReadFile(path); readErr == nil {
		_ = json.Unmarshal(data, &shape)
	}
	if shape.Hosts == nil {
		shape.Hosts = make(map[string]Selection)
	}
	// Normalize: no owner means no project/view; no project means no view.
	if sel.Owner == "" {
		sel.Project = 0
		sel.View = 0
	}
	if sel.Project == 0 {
		sel.View = 0
	}
	if sel.Owner == "" && sel.Project == 0 && sel.View == 0 {
		delete(shape.Hosts, host)
	} else {
		shape.Hosts[host] = sel
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(shape, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Resolve merges explicit CLI flags over remembered selections.
// Changing an ancestor discards remembered descendants unless the descendant
// was also explicitly provided.
func Resolve(explicit, remembered Selection) Selection {
	owner := remembered.Owner
	if explicit.Owner != "" {
		owner = explicit.Owner
	}
	ownerChanged := explicit.Owner != "" && explicit.Owner != remembered.Owner

	project := 0
	view := 0
	if ownerChanged {
		// Ancestor changed: only explicit descendants survive.
		project = explicit.Project
		view = 0
		if project != 0 {
			view = explicit.View
		}
	} else {
		project = remembered.Project
		if explicit.Project != 0 {
			project = explicit.Project
		}
		projectChanged := explicit.Project != 0 && explicit.Project != remembered.Project
		if projectChanged {
			view = explicit.View
		} else {
			view = remembered.View
			if explicit.View != 0 {
				view = explicit.View
			}
		}
		// View requires a project; owner requires presence.
		if project == 0 {
			view = 0
		}
	}
	if owner == "" {
		project = 0
		view = 0
	}
	return Selection{Owner: owner, Project: project, View: view}
}
