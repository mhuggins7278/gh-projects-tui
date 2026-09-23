package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PageBoardItems selects only the fields needed by this view. Field names are
// variables, never interpolated into GraphQL source.
func (c *Client) PageBoardItems(ctx context.Context, owner Owner, project int, filter, after string, fields []Field) (ItemsPage, error) {
	return c.pageSelectedItems(ctx, owner, project, filter, after, fields, true)
}

// PageMutationItems is an uncached authoritative order/grouping read. It omits
// card content and all nested user/label/reviewer connections.
func (c *Client) PageMutationItems(ctx context.Context, owner Owner, project int, after string, fields []Field) (ItemsPage, error) {
	return c.pageSelectedItems(ctx, owner, project, "", after, fields, false)
}

// ReadMutationItem verifies a field-only change without traversing project order.
func (c *Client) ReadMutationItem(ctx context.Context, itemID string, field Field) (Item, error) {
	query := `query MutationItem($id: ID!, $name: String!) { node(id: $id) { ... on ProjectV2Item { id value: fieldValueByName(name: $name) { ...FieldValue } } } }` + fieldValueFragment
	var response struct {
		Node *struct {
			ID    string
			Value *rawFieldValue
		}
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": itemID, "name": field.Name}, &response); err != nil {
		return Item{}, err
	}
	if response.Node == nil || response.Node.ID != itemID {
		return Item{}, fmt.Errorf("mutation item is unavailable")
	}
	item := Item{ID: itemID}
	if response.Node.Value != nil {
		item.FieldValues = []FieldValue{response.Node.Value.project()}
	}
	return item, nil
}

func (c *Client) pageSelectedItems(ctx context.Context, owner Owner, project int, filter, after string, fields []Field, content bool) (ItemsPage, error) {
	branch := "organization"
	if owner.Kind == UserOwner {
		branch = "user"
	} else if owner.Kind != OrganizationOwner {
		return ItemsPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	variables := map[string]interface{}{"login": owner.Login, "project": project, "filter": nullableCursor(filter), "after": nullableCursor(after)}
	var definitions, selections strings.Builder
	seen := map[string]bool{}
	names := []string{}
	for _, field := range fields {
		if field.Name == "" || seen[field.Name] {
			continue
		}
		seen[field.Name] = true
		name := fmt.Sprintf("field%d", len(names))
		names = append(names, name)
		fmt.Fprintf(&definitions, ", $%s: String!", name)
		fmt.Fprintf(&selections, "%s: fieldValueByName(name: $%s) { ...FieldValue", name, name)
		if content {
			selections.WriteString(" ... on ProjectV2ItemFieldUserValue { users(first: 10) { nodes { login } pageInfo { hasNextPage endCursor } } }")
		}
		selections.WriteString(" } ")
		variables[name] = field.Name
	}
	contentSelection := ""
	if content {
		contentSelection = `content { __typename ... on Issue { id number title url repository { name } state subIssuesSummary { total completed } } ... on PullRequest { id number title url repository { name } state isDraft merged } ... on DraftIssue { id title } }`
	}
	query := fmt.Sprintf(`query SelectedProjectItems($login: String!, $project: Int!, $filter: String, $after: String%s) { %s(login: $login) { projectV2(number: $project) { items(first: 100, after: $after, query: $filter, orderBy: {field: POSITION, direction: ASC}) { nodes { id %s %s } pageInfo { hasNextPage endCursor } } } } }`, definitions.String(), branch, contentSelection, selections.String())
	if len(names) > 0 {
		query += fieldValueFragment
	}
	if !content {
		query = strings.Replace(query, "query SelectedProjectItems(", "query MutationProjectItems(", 1)
	}
	type selectedOwner struct {
		Project *struct {
			Items struct {
				Nodes    []json.RawMessage
				PageInfo pageInfo
			}
		} `json:"projectV2"`
	}
	var response struct {
		User         *selectedOwner
		Organization *selectedOwner
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return ItemsPage{}, err
	}
	selected := response.Organization
	if owner.Kind == UserOwner {
		selected = response.User
	}
	if selected == nil || selected.Project == nil {
		return ItemsPage{}, fmt.Errorf("project %d for %q was not found", project, owner.Login)
	}
	page := rawItemsPage{PageInfo: selected.Project.Items.PageInfo}
	if page.PageInfo.HasNextPage {
		if _, err := nextCursor(page.PageInfo, after); err != nil {
			return ItemsPage{}, err
		}
	}
	for _, node := range selected.Project.Items.Nodes {
		if string(node) == "null" {
			continue
		}
		var item rawItem
		if err := json.Unmarshal(node, &item); err != nil {
			return ItemsPage{}, err
		}
		var values map[string]json.RawMessage
		if err := json.Unmarshal(node, &values); err != nil {
			return ItemsPage{}, err
		}
		for _, name := range names {
			var value *rawFieldValue
			if err := json.Unmarshal(values[name], &value); err != nil {
				return ItemsPage{}, err
			}
			if value != nil {
				item.FieldValues.Nodes = append(item.FieldValues.Nodes, value)
			}
		}
		page.Nodes = append(page.Nodes, &item)
	}
	return c.projectItems(ctx, page)
}
