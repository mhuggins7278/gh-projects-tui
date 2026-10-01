//go:build live

package github

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"
)

// This investigation reports only schema names and aggregate counts. It never
// submits mutations or stores project metadata or item content in fixtures.
func TestLiveRoadmapReadContract(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_ROADMAP") != "1" {
		t.Skip("set GH_PROJECTS_TUI_LIVE_ROADMAP=1 to run read-only roadmap probes")
	}
	login := os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER")
	project, projectErr := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	viewNumber, viewErr := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if login == "" || projectErr != nil || project <= 0 || viewErr != nil || viewNumber <= 0 {
		t.Fatal("set an owner and positive project/view numbers for an existing roadmap")
	}
	kind := OrganizationOwner
	switch os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") {
	case "", "organization":
	case "user":
		kind = UserOwner
	default:
		t.Fatal("owner kind must be user or organization")
	}
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var schema struct {
		View, Configuration *struct {
			Fields []struct{ Name string }
		}
	}
	// GitHub limits __Type.fields to two occurrences per introspection query.
	const query = `query RoadmapSchema {
	  view: __type(name: "ProjectV2View") { fields { name } }
	  configuration: __type(name: "ProjectV2ViewConfiguration") { fields { name } }
	}`
	if err := client.graphql.DoWithContext(ctx, query, nil, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.View == nil || schema.Configuration == nil {
		t.Fatal("roadmap view schema is unavailable on this host")
	}
	var configurationFields, viewFields []string
	for _, field := range schema.Configuration.Fields {
		configurationFields = append(configurationFields, field.Name)
	}
	for _, field := range schema.View.Fields {
		viewFields = append(viewFields, field.Name)
	}
	t.Logf("view schema fields: %v; configuration fields: %v", viewFields, configurationFields)
	slices.Sort(viewFields)
	slices.Sort(configurationFields)
	wantViewFields := []string{"configuration", "createdAt", "fields", "filter", "fullDatabaseId", "groupByFields", "id", "layout", "name", "number", "project", "sortByFields", "updatedAt", "verticalGroupByFields"}
	if !slices.Equal(configurationFields, []string{"visibleFields"}) || !slices.Equal(viewFields, wantViewFields) {
		t.Fatal("view/configuration schema changed; re-investigate endpoint mapping before updating the compatibility note")
	}
	owner := Owner{Login: login, Kind: kind}
	view, err := client.OpenView(ctx, owner, project, viewNumber)
	if err != nil {
		t.Fatal(err)
	}
	if view.Layout != RoadmapLayout {
		t.Fatal("selected view is not a saved roadmap")
	}
	dateFields, iterationFields := 0, 0
	for _, field := range view.ProjectFields {
		switch field.DataType {
		case "DATE":
			dateFields++
		case "ITERATION":
			iterationFields++
		}
	}
	t.Logf("metadata: %d project fields (%d date, %d iteration), %d visible, %d group, %d vertical group, %d sort; filter present: %t",
		len(view.ProjectFields), dateFields, iterationFields, len(view.Fields), len(view.GroupByFields), len(view.VerticalGroupBy), len(view.SortByFields), view.Filter != "")
	seen := make(map[string]bool)
	after, pages, dateValues, iterationValues := "", 0, 0, 0
	for {
		// Unfiltered baseline: do not imply the roadmap's filter was validated.
		page, err := client.PageItems(ctx, owner, project, "", after)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatal("baseline returned duplicate item IDs")
			}
			seen[item.ID] = true
			for _, value := range item.FieldValues {
				if value.Available && value.Value != "" && value.Kind == "ProjectV2ItemFieldDateValue" {
					dateValues++
				}
				if value.IterationID != "" && value.Kind == "ProjectV2ItemFieldIterationValue" {
					iterationValues++
				}
			}
		}
		if !page.HasNext {
			break
		}
		if page.EndCursor == "" || page.EndCursor == after {
			t.Fatal("baseline pagination did not advance")
		}
		after = page.EndCursor
	}
	t.Logf("unfiltered POSITION baseline: %d unique items over %d pages, %d populated date values, %d populated iteration values", len(seen), pages, dateValues, iterationValues)
	t.Log("this unfiltered baseline alone does not verify endpoint mapping, web placement, or saved-filter/sort parity")
}

// This optional probe verifies a populated exact is:issue saved filter against
// the same project's complete POSITION baseline. Web ordering remains a separate
// DOM comparison; success here does not admit other filters or sorts.
func TestLiveRoadmapSavedIssueFilter(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_LIVE_ROADMAP_FILTER") != "1" {
		t.Skip("opt-in read-only saved roadmap filter comparison")
	}
	owner := Owner{Login: os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER"), Kind: UserOwner}
	if os.Getenv("GH_PROJECTS_TUI_LIVE_OWNER_KIND") == "organization" {
		owner.Kind = OrganizationOwner
	}
	project, err := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_PROJECT"))
	if err != nil || project <= 0 || owner.Login == "" {
		t.Fatal("supply an existing owner and positive project number")
	}
	viewNumber, err := strconv.Atoi(os.Getenv("GH_PROJECTS_TUI_LIVE_VIEW"))
	if err != nil || viewNumber <= 0 {
		t.Fatal("supply an existing saved roadmap number")
	}
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Visibility is checked before reading configuration or item content.
	branch := "user"
	if owner.Kind == OrganizationOwner {
		branch = "organization"
	}
	var visibility struct {
		User, Organization *struct {
			Project *struct{ Public bool } `json:"projectV2"`
		}
	}
	query := `query PublicRoadmap($login:String!,$project:Int!){` + branch + `(login:$login){projectV2(number:$project){public}}}`
	if err := client.graphql.DoWithContext(ctx, query, map[string]interface{}{"login": owner.Login, "project": project}, &visibility); err != nil {
		t.Fatal("public visibility check failed")
	}
	visible := visibility.User
	if owner.Kind == OrganizationOwner {
		visible = visibility.Organization
	}
	if visible == nil || visible.Project == nil || !visible.Project.Public {
		t.Fatal("probe requires an existing public project")
	}
	view, err := client.OpenView(ctx, owner, project, viewNumber)
	if err != nil {
		t.Fatal(err)
	}
	if view.Layout != RoadmapLayout || view.Filter != "is:issue" || len(view.GroupByFields) != 0 || len(view.VerticalGroupBy) != 0 || len(view.SortByFields) != 1 || view.SortByFields[0].Field.DataType != "DATE" || view.SortByFields[0].Direction != "ASC" {
		t.Fatal("sample must be an ungrouped saved is:issue roadmap with one DATE ASC sort")
	}
	read := func(filter string) ([]Item, int) {
		var items []Item
		seen := map[string]bool{}
		cursors := map[string]bool{}
		after := ""
		pages := 0
		for {
			page, err := client.PageItems(ctx, owner, project, filter, after)
			if err != nil {
				t.Fatal(err)
			}
			pages++
			for _, item := range page.Items {
				if seen[item.ID] {
					t.Fatal("duplicate item ID")
				}
				seen[item.ID] = true
				items = append(items, item)
			}
			if !page.HasNext {
				break
			}
			if page.EndCursor == "" || cursors[page.EndCursor] || page.EndCursor == after {
				t.Fatal("cursor did not advance")
			}
			after = page.EndCursor
			cursors[after] = true
		}
		return items, pages
	}
	baseline, baselinePages := read("")
	filtered, filteredPages := read(view.Filter)
	expected := map[string]bool{}
	excluded := 0
	for _, item := range baseline {
		if item.Content == nil {
			t.Fatal("inaccessible baseline content prevents full membership comparison")
		}
		if item.Content.Kind == "Issue" {
			expected[item.ID] = true
		} else {
			excluded++
		}
	}
	if len(filtered) == 0 || excluded == 0 || len(filtered) != len(expected) {
		t.Fatal("sample lacks populated inclusion/exclusion parity")
	}
	dates := map[string]int{}
	unset := 0
	for _, item := range filtered {
		if !expected[item.ID] || item.Content == nil || item.Content.Kind != "Issue" {
			t.Fatal("saved filter membership differs from baseline issue membership")
		}
		date := ""
		for _, value := range item.FieldValues {
			if value.FieldID == view.SortByFields[0].Field.ID && value.Available {
				date = value.Value
			}
		}
		if date == "" {
			unset++
		} else {
			dates[date]++
		}
	}
	ties := 0
	for _, count := range dates {
		if count > 1 {
			ties++
		}
	}
	t.Logf("baseline: %d unique items, %d pages; exact saved filter: %d issues, %d pages; excluded non-issues: %d; unset sort values: %d; tied date groups: %d", len(baseline), baselinePages, len(filtered), filteredPages, excluded, unset, ties)
}
