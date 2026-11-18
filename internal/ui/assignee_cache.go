package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

const assigneeCacheTTL = 15 * time.Minute

type cachedAssignees struct {
	At   time.Time
	Page github.AssigneeSearchPage
}

// Only candidates are cached. Opening the editor still checks current issue
// membership and permissions before any cached results can be used.
func assigneeCachePath(host, repository, query, after string) string {
	if repository == "" {
		return ""
	}
	root, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	identity, _ := json.Marshal([]string{strings.ToLower(host), strings.ToLower(repository), query, after})
	hash := sha256.Sum256(identity)
	return filepath.Join(root, "gh-projects-tui", "assignees-v1", hex.EncodeToString(hash[:])+".json")
}

func readAssigneeCache(path string) (github.AssigneeSearchPage, bool) {
	if path == "" {
		return github.AssigneeSearchPage{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return github.AssigneeSearchPage{}, false
	}
	var entry cachedAssignees
	if json.Unmarshal(data, &entry) != nil || time.Since(entry.At) < 0 || time.Since(entry.At) >= assigneeCacheTTL || !entry.Page.CanEdit {
		return github.AssigneeSearchPage{}, false
	}
	return entry.Page, true
}

func writeAssigneeCache(path string, page github.AssigneeSearchPage) {
	if path == "" || !page.CanEdit {
		return
	}
	data, err := json.Marshal(cachedAssignees{At: time.Now(), Page: page})
	if err != nil {
		return
	}
	dir := filepath.Dir(path)
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	file, err := os.CreateTemp(dir, ".assignees-*")
	if err != nil {
		return
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	closeErr := file.Close()
	if err == nil && closeErr == nil {
		_ = os.Rename(file.Name(), path)
	}
}
