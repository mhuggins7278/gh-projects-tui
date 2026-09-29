//go:build live

package github

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// Membership predicates are test-only: the client still forwards saved filters
// unchanged and never evaluates them locally. All operations here are reads.
func TestLiveSavedFilterContract(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_FILTERS") != "1" {
		t.Skip("set GH_PROJECTS_TUI_LIVE_FILTERS=1 to run read-only saved-filter probes")
	}
	login := os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER")
	project, err := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	if login == "" || err != nil || project <= 0 {
		t.Fatal("set GH_PROJECTS_TUI_LIVE_OWNER and a positive GH_PROJECTS_TUI_LIVE_PROJECT")
	}
	branch := "organization"
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "user" {
		branch = "user"
	}
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	query := `query SavedFilterProbe($login: String!, $project: Int!, $filter: String!, $after: String, $first: Int!) {
  ` + branch + `(login: $login) { projectV2(number: $project) {
    items(first: $first, after: $after, query: $filter, orderBy: {field: POSITION, direction: ASC}) {
      nodes { id content {
        __typename
        ... on Issue { closedAt parent { number repository { nameWithOwner } } }
        ... on PullRequest { closedAt }
      } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`
	read := func(t *testing.T, filter string, first int) (map[string]liveFilterItem, int) {
		t.Helper()
		items := make(map[string]liveFilterItem)
		after, pages := "", 0
		for {
			var response struct {
				User         *liveFilterOwner `json:"user"`
				Organization *liveFilterOwner `json:"organization"`
			}
			var cursor interface{}
			if after != "" {
				cursor = after
			}
			variables := map[string]interface{}{"login": login, "project": project, "filter": filter, "after": cursor, "first": first}
			if err := client.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
				t.Fatalf("saved-filter probe %q: %v", filter, err)
			}
			owner := response.Organization
			if branch == "user" {
				owner = response.User
			}
			if owner == nil || owner.Project == nil {
				t.Fatal("probe project is inaccessible")
			}
			page := owner.Project.Items
			pages++
			for _, item := range page.Nodes {
				if _, exists := items[item.ID]; exists {
					t.Fatalf("probe %q returned duplicate item IDs", filter)
				}
				items[item.ID] = item
			}
			if !page.PageInfo.HasNextPage {
				return items, pages
			}
			var err error
			after, err = nextCursor(page.PageInfo, after)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	baseline, _ := read(t, "", 100)
	day, parent := "", ""
	for _, item := range baseline {
		if item.Content == nil {
			continue
		}
		if item.Content.ClosedAt != nil {
			date := item.Content.ClosedAt.UTC().Format("2006-01-02")
			if day == "" || date < day {
				day = date
			}
		}
		if item.Content.Parent != nil {
			reference := item.Content.Parent.reference()
			if parent == "" || reference < parent {
				parent = reference
			}
		}
	}
	if day == "" || parent == "" {
		t.Fatal("use a project with populated closure dates and parent-linked child issues")
	}
	sampleDate, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	offsetDays := int(sampleDate.Sub(time.Now().UTC().Truncate(24*time.Hour)).Hours() / 24)
	relativeDay := fmt.Sprintf("@today%+dd", offsetDays)
	closed := func(item liveFilterItem) bool { return item.Content != nil && item.Content.ClosedAt != nil }
	parented := func(item liveFilterItem) bool { return item.Content != nil && item.Content.Parent != nil }
	child := func(item liveFilterItem) bool { return parented(item) && item.Content.Parent.reference() == parent }
	sameDay := func(item liveFilterItem) bool {
		return closed(item) && item.Content.ClosedAt.UTC().Format("2006-01-02") == day
	}
	for _, test := range []struct {
		filter string
		match  func(liveFilterItem) bool
	}{
		{filter: "closed:" + day, match: sameDay},
		{filter: "-closed:" + day, match: func(item liveFilterItem) bool { return !sameDay(item) }},
		{filter: "closed:>=" + day, match: func(item liveFilterItem) bool {
			return closed(item) && item.Content.ClosedAt.UTC().Format("2006-01-02") >= day
		}},
		{filter: "closed:" + day + ".." + day, match: sameDay},
		{filter: "closed:" + relativeDay, match: sameDay},
		{filter: "closed:" + relativeDay + ".." + relativeDay, match: sameDay},
		{filter: "has:closed", match: closed},
		{filter: "-no:closed", match: closed},
		{filter: "no:closed", match: func(item liveFilterItem) bool { return !closed(item) }},
		{filter: "parent-issue:" + parent, match: child},
		{filter: fmt.Sprintf("parent-issue:%q", parent), match: child},
		{filter: fmt.Sprintf("-parent-issue:%q", parent), match: func(item liveFilterItem) bool { return !child(item) }},
		{filter: "-parent-issue:" + parent, match: func(item liveFilterItem) bool { return !child(item) }},
		{filter: "has:parent-issue", match: parented},
		{filter: "-no:parent-issue", match: parented},
		{filter: "no:parent-issue", match: func(item liveFilterItem) bool { return !parented(item) }},
	} {
		t.Run(test.filter, func(t *testing.T) {
			want := make(map[string]bool)
			for id, item := range baseline {
				if test.match(item) {
					want[id] = true
				}
			}
			result, pages := read(t, test.filter, 2)
			got := make(map[string]bool)
			for id := range result {
				got[id] = true
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("membership differs from read-only baseline: got %d items, want %d", len(got), len(want))
			}
			t.Logf("verified %d unique items over %d pages", len(got), pages)
		})
	}
}

type liveFilterOwner struct {
	Project *struct {
		Items struct {
			Nodes    []liveFilterItem
			PageInfo pageInfo
		}
	} `json:"projectV2"`
}

type liveFilterItem struct {
	ID      string
	Content *struct {
		ClosedAt *time.Time
		Parent   *liveFilterParent
	}
}

type liveFilterParent struct {
	Number     int
	Repository struct{ NameWithOwner string }
}

func (p liveFilterParent) reference() string {
	return fmt.Sprintf("%s#%d", p.Repository.NameWithOwner, p.Number)
}
