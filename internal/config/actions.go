package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Action is an explicitly configured executable and its arguments. Arguments
// are expanded independently, without invoking a shell.
type Action struct {
	Key  string   `yaml:"key"`
	Name string   `yaml:"name"`
	On   string   `yaml:"on"`
	Run  []string `yaml:"run"`
}

type ActionsConfig struct {
	Actions         []Action          `yaml:"actions"`
	RepoPaths       map[string]string `yaml:"repoPaths"`
	RepoSearchRoots []string          `yaml:"repoSearchRoots"`
}

var actionKeyPattern = regexp.MustCompile(`^(?:[A-Za-z0-9]|(?:ctrl|alt)\+[a-z]|f(?:[1-9]|1[0-2]))$`)
var reservedActionKeys = map[string]bool{
	"j": true, "k": true, "h": true, "l": true, "H": true, "L": true, "J": true, "K": true,
	"g": true, "f": true, "F": true, "e": true, "m": true, "a": true, "D": true, "c": true, "x": true,
	"q": true, "A": true, "p": true, "v": true, "r": true, "o": true,
	"ctrl+c": true, "ctrl+h": true, "ctrl+i": true, "ctrl+j": true, "ctrl+m": true, "ctrl+u": true, "ctrl+z": true, "ctrl+s": true, "ctrl+q": true,
}

var actionTemplateFields = map[string]string{
	"RepoName": "owner/repo", "Host": "github.com", "Number": "1", "IssueNumber": "1", "IssueTitle": "Title",
	"PRNumber": "1", "PrNumber": "1", "URL": "https://github.com/owner/repo/issues/1", "Title": "Title",
	"ProjectOwner": "owner", "ProjectNumber": "1", "ProjectID": "project", "ContentID": "content", "ProjectItemID": "item", "RepoPath": "/path/to/repo",
}

func DefaultActionsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-projects-tui", "config.yml"), nil
}

// LoadActions reads an optional default config, or requires an explicitly
// supplied path. Invalid configuration is reported rather than silently ignored.
func LoadActions(path string, required bool) ([]Action, error) {
	file, err := LoadActionConfig(path, required)
	if err != nil {
		return nil, err
	}
	return file.Actions, nil
}

func LoadActionConfig(path string, required bool) (*ActionsConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) && !required {
		return &ActionsConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read actions config %s: %w", path, err)
	}
	var file ActionsConfig
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err == io.EOF {
		return &ActionsConfig{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("parse actions config %s: %w", path, err)
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, fmt.Errorf("parse actions config %s: %w", path, err)
		}
		return nil, fmt.Errorf("parse actions config %s: multiple YAML documents are not supported", path)
	}
	seen := map[string]bool{}
	for i, action := range file.Actions {
		where := fmt.Sprintf("action %d", i+1)
		if !actionKeyPattern.MatchString(action.Key) || reservedActionKeys[action.Key] {
			return nil, fmt.Errorf("%s: key %q is invalid or reserved", where, action.Key)
		}
		if seen[action.Key] {
			return nil, fmt.Errorf("%s: duplicate key %q", where, action.Key)
		}
		seen[action.Key] = true
		if strings.TrimSpace(action.Name) == "" {
			return nil, fmt.Errorf("%s: name is required", where)
		}
		if action.On != "issue" && action.On != "pr" {
			return nil, fmt.Errorf("%s: on must be issue or pr", where)
		}
		if len(action.Run) == 0 || strings.TrimSpace(action.Run[0]) == "" {
			return nil, fmt.Errorf("%s: run must contain an executable and optional arguments", where)
		}
		for _, arg := range action.Run {
			t, err := template.New("argument").Option("missingkey=error").Parse(arg)
			if err != nil {
				return nil, fmt.Errorf("%s: invalid argument template: %w", where, err)
			}
			if err := t.Execute(&bytes.Buffer{}, actionTemplateFields); err != nil {
				return nil, fmt.Errorf("%s: invalid argument template: %w", where, err)
			}
		}
	}
	if err := ValidateRepoPaths(file.RepoPaths, file.RepoSearchRoots); err != nil {
		return nil, err
	}
	return &file, nil
}
