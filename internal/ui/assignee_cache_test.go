package ui

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func TestAssigneePickerReusesRepositoryCandidates(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m, source := metadataModel()
	m.items[0].Content.Repository = "owner/repo"
	m = openMetadata(t, m, "a")
	if len(source.searches) != 1 {
		t.Fatal("initial candidates not fetched")
	}
	m, _ = editorKey(m, "esc")
	// Reopen in a fresh model to exercise the persisted cache as well.
	fresh, _ := metadataModel()
	fresh.source = source
	fresh.items[0].Content.Repository = "owner/repo"
	fresh = openMetadata(t, fresh, "a")
	if len(source.searches) != 1 || fresh.issueMetadataEditor.searchLoading || len(fresh.issueMetadataEditor.options) == 0 {
		t.Fatalf("cached reopen searched again: %v", source.searches)
	}
	fresh, _ = editorKey(fresh, "esc")
	source.canEdit = false
	fresh = openMetadata(t, fresh, "a")
	if fresh.issueMetadataEditor != nil {
		t.Fatal("cache bypassed fresh permissions")
	}
}

func TestAssigneeCacheExpiryAndIsolation(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path := assigneeCachePath("github.com", "owner/repo", "", "")
	page := github.AssigneeSearchPage{CanEdit: true, Options: []github.IssueMetadataOption{{ID: "user", Name: "Alice"}}}
	writeAssigneeCache(path, page)
	if _, ok := readAssigneeCache(path); !ok {
		t.Fatal("fresh cache missed")
	}
	for _, other := range []string{assigneeCachePath("work.example", "owner/repo", "", ""), assigneeCachePath("github.com", "owner/other", "", ""), assigneeCachePath("github.com", "owner/repo", "Alice", ""), assigneeCachePath("github.com", "owner/repo", "", "next")} {
		if _, ok := readAssigneeCache(other); ok {
			t.Fatal("cache leaked across scope or query")
		}
	}
	data, _ := json.Marshal(cachedAssignees{At: time.Now().Add(-assigneeCacheTTL), Page: page})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readAssigneeCache(path); ok {
		t.Fatal("expired candidates reused")
	}
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := readAssigneeCache(path); ok {
		t.Fatal("corrupt cache accepted")
	}
}
