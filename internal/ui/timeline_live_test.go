//go:build live

package ui

import (
	"context"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// Opt-in read-only probe. The caller explicitly identifies an accessible demo;
// only aggregate results are logged, never retained project contents.
func TestLiveTimelineDemo(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_TIMELINE_DEMO") != "1" {
		t.Skip("explicit demo read probe")
	}
	owner := github.Owner{Login: os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER"), Kind: github.UserOwner}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "organization" {
		owner.Kind = github.OrganizationOwner
	}
	project, err := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	if err != nil || project < 1 || owner.Login == "" {
		t.Fatal("supply owner/project")
	}
	number, err := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if err != nil || number < 1 {
		t.Fatal("supply view")
	}
	mappings, err := config.LoadRoadmapMappings(os.Getenv("GH_PROJECTS_TUI_LIVE_MAPPINGS"))
	if err != nil || len(mappings.Roadmaps) == 0 {
		t.Fatal("supply explicit mappings")
	}
	var expected []int
	for _, part := range strings.Split(os.Getenv("GH_PROJECTS_TUI_LIVE_EXPECT_ORDER"), ",") {
		n, err := strconv.Atoi(part)
		if err != nil {
			t.Fatal("supply expected web order")
		}
		expected = append(expected, n)
	}
	client, err := github.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	view, err := client.OpenView(ctx, owner, project, number)
	if err != nil {
		t.Fatal(err)
	}
	m := NewModelWithHost(client, Selection{}, config.DefaultHost())
	m.SetRoadmapMappings(mappings)
	if c := m.viewCompatibility(view); !c.supported() {
		t.Fatal(c.summary())
	}
	m.view = &view
	after, pages := "", 0
	seen := map[string]bool{}
	for {
		page, err := client.PageItems(ctx, owner, project, view.Filter, after)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("duplicate identity")
			}
			seen[item.ID] = true
			m.items = append(m.items, item)
		}
		if !page.HasNext {
			break
		}
		if page.EndCursor == "" || page.EndCursor == after {
			t.Fatal("cursor did not advance")
		}
		after = page.EndCursor
	}
	var actual []int
	for _, item := range m.tableItems() {
		if item.Content == nil {
			t.Fatal("missing demo content")
		}
		actual = append(actual, item.Content.Number)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("API membership/local ordering differs from supplied web ordering")
	}
	mapping, _ := m.roadmapMappings.Find(m.host, view.ProjectID, view.ID)
	fields, err := github.ResolveRoadmapFields(view.ProjectFields, mapping.StartFieldID, mapping.TargetFieldID)
	if err != nil {
		t.Fatal(err)
	}
	available := 0
	for _, item := range m.items {
		start, target := timelineMappedEndpoint(fields.Start, item, false), timelineMappedEndpoint(fields.Target, item, true)
		if !start.Available || !target.Available {
			t.Fatal("demo endpoint resolution unavailable")
		}
		if start.Value != "" || target.Value != "" {
			available++
		}
	}
	t.Logf("verified %d rows over %d client pages; %d endpoint-populated rows, %d groups, %d sorts", len(actual), pages, available, len(view.GroupByFields), len(view.SortByFields))
}
