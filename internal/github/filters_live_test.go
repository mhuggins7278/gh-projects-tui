//go:build live

package github

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	query := `query SavedFilterProbe($login: String!, $project: Int!, $filter: String!, $after: String, $first: Int!) {
  ` + branch + `(login: $login) { projectV2(number: $project) {
    items(first: $first, after: $after, query: $filter, orderBy: {field: POSITION, direction: ASC}) {
      nodes { id updatedAt
        status: fieldValueByName(name: "Status") { ... on ProjectV2ItemFieldSingleSelectValue { name } }
        content {
        __typename
        ... on Issue { createdAt updatedAt closedAt parent { number repository { nameWithOwner } } }
        ... on PullRequest { createdAt updatedAt closedAt }
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
				t.Fatalf("saved-filter probe failed: %v", err)
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
					t.Fatal("probe returned duplicate item IDs")
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

	baseline, baselinePages := read(t, "", 100)
	t.Logf("baseline: %d unique items over %d pages", len(baseline), baselinePages)
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
	type probe struct {
		name   string
		filter string
		match  func(liveFilterItem) bool
	}
	tests := []probe{
		{name: "closed exact", filter: "closed:" + day, match: sameDay},
		{name: "closed negated exact", filter: "-closed:" + day, match: func(item liveFilterItem) bool { return !sameDay(item) }},
		{name: "closed comparison", filter: "closed:>=" + day, match: func(item liveFilterItem) bool {
			return closed(item) && item.Content.ClosedAt.UTC().Format("2006-01-02") >= day
		}},
		{name: "closed equal-bound range", filter: "closed:" + day + ".." + day, match: sameDay},
		{name: "closed relative date", filter: "closed:" + relativeDay, match: sameDay},
		{name: "closed relative range", filter: "closed:" + relativeDay + ".." + relativeDay, match: sameDay},
		{name: "closed present", filter: "has:closed", match: closed},
		{name: "closed negated missing", filter: "-no:closed", match: closed},
		{name: "closed missing", filter: "no:closed", match: func(item liveFilterItem) bool { return !closed(item) }},
		{name: "parent unquoted", filter: "parent-issue:" + parent, match: child},
		{name: "parent quoted", filter: fmt.Sprintf("parent-issue:%q", parent), match: child},
		{name: "parent negated quoted", filter: fmt.Sprintf("-parent-issue:%q", parent), match: func(item liveFilterItem) bool { return !child(item) }},
		{name: "parent negated unquoted", filter: "-parent-issue:" + parent, match: func(item liveFilterItem) bool { return !child(item) }},
		{name: "parent present", filter: "has:parent-issue", match: parented},
		{name: "parent negated missing", filter: "-no:parent-issue", match: parented},
		{name: "parent missing", filter: "no:parent-issue", match: func(item liveFilterItem) bool { return !parented(item) }},
	}
	// Reproduce multi-value OR, repeated qualifier AND, negation, and an
	// intersection with another qualifier against the complete baseline. The
	// standard Status field is optional; do not manufacture populated samples.
	statusNames := make(map[string]bool)
	for _, item := range baseline {
		if item.Status != nil && item.Status.Name != "" && !strings.ContainsAny(item.Status.Name, "\\\",\n\r") {
			statusNames[item.Status.Name] = true
		}
	}
	names := make([]string, 0, len(statusNames))
	for name := range statusNames {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) >= 2 {
		first, second := names[0], names[1]
		term := fmt.Sprintf("status:%q,%q", first, second)
		hasEither := func(item liveFilterItem) bool {
			return item.Status != nil && (item.Status.Name == first || item.Status.Name == second)
		}
		tests = append(tests,
			probe{"status comma OR", term, hasEither},
			probe{"status negated OR", "-" + term, func(item liveFilterItem) bool { return !hasEither(item) }},
			probe{"status repeated AND", fmt.Sprintf("status:%q status:%q", first, second), func(liveFilterItem) bool { return false }},
			probe{"status OR with closed AND", term + " is:closed", func(item liveFilterItem) bool { return hasEither(item) && closed(item) }},
		)
	} else {
		t.Log("Status combinations not exercised: baseline needs two populated Status values")
	}
	for _, field := range []string{"created", "updated"} {
		dateFor := func(item liveFilterItem) string {
			// Projects updated: membership matched the project item's timestamp,
			// including items whose Issue.updatedAt differed. Keep that explicit
			// instead of assuming repository content and project edits coincide.
			date := item.UpdatedAt
			if field == "created" {
				date = nil
				if item.Content != nil {
					date = item.Content.CreatedAt
				}
			}
			if date == nil {
				return ""
			}
			return date.UTC().Format("2006-01-02")
		}
		sample := ""
		for _, item := range baseline {
			if date := dateFor(item); date != "" && (sample == "" || date < sample) {
				sample = date
			}
		}
		if sample == "" {
			t.Logf("%s forms not exercised: baseline has no usable timestamp", field)
			continue
		}
		equal := func(item liveFilterItem) bool { return dateFor(item) == sample }
		tests = append(tests,
			probe{field + " exact", field + ":" + sample, equal},
			probe{field + " equal-bound range", field + ":" + sample + ".." + sample, equal},
			probe{field + " comparison", field + ":>=" + sample, func(item liveFilterItem) bool { return dateFor(item) >= sample }},
		)
		if field == "updated" {
			sampleDate, err := time.Parse("2006-01-02", sample)
			if err != nil {
				t.Fatal(err)
			}
			days := int(sampleDate.Sub(time.Now().UTC().Truncate(24*time.Hour)).Hours() / 24)
			relative := fmt.Sprintf("@today%+dd", days)
			tests = append(tests,
				probe{"updated negated exact", "-updated:" + sample, func(item liveFilterItem) bool { return !equal(item) }},
				probe{"updated relative date", "updated:" + relative, equal},
				probe{"updated relative range", "updated:" + relative + ".." + relative, equal},
			)
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
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
	ID        string
	UpdatedAt *time.Time
	Status    *struct{ Name string }
	Content   *struct {
		CreatedAt *time.Time
		UpdatedAt *time.Time
		ClosedAt  *time.Time
		Parent    *liveFilterParent
	}
}

type liveFilterParent struct {
	Number     int
	Repository struct{ NameWithOwner string }
}

func (p liveFilterParent) reference() string {
	return fmt.Sprintf("%s#%d", p.Repository.NameWithOwner, p.Number)
}
