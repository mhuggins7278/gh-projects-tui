package github

import (
	"context"
	"encoding/json"
	"fmt"
)

const subIssueContentFields = `__typename id number title url state viewerCanSetFields viewerCanClose viewerCanReopen repository { name nameWithOwner } subIssuesSummary { total completed }`

type issueProjectItems struct {
	Nodes    []json.RawMessage
	PageInfo pageInfo
}

type rawSubIssue struct {
	rawContent
	ProjectItems issueProjectItems
}

// PageSubIssues reads direct children in GitHub's hierarchy order, independently
// of the saved filter. Project cells come only from this project's active item.
func (c *Client) PageSubIssues(ctx context.Context, projectID, issueID, after string, fields []Field) (ItemsPage, error) {
	if projectID == "" || issueID == "" {
		return ItemsPage{}, fmt.Errorf("sub-issues require project and issue identities")
	}
	variables := map[string]interface{}{"id": issueID, "after": nullableCursor(after)}
	query := `query IssueSubIssues($id: ID!, $after: String) {
  node(id: $id) { ... on Issue {
    subIssues(first: 20, after: $after) {
      nodes { ` + subIssueContentFields + ` projectItems(first: 100, includeArchived: false) {
        nodes { id project { id } } pageInfo { hasNextPage endCursor }
      } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`
	var response struct {
		Node *struct {
			SubIssues *struct {
				Nodes    []*rawSubIssue
				PageInfo pageInfo
			}
		}
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return ItemsPage{}, err
	}
	if response.Node == nil || response.Node.SubIssues == nil {
		return ItemsPage{}, fmt.Errorf("issue is unavailable while loading sub-issues")
	}
	connection := response.Node.SubIssues
	page := ItemsPage{HasNext: connection.PageInfo.HasNextPage}
	if page.HasNext {
		next, err := nextCursor(connection.PageInfo, after)
		if err != nil {
			return ItemsPage{}, err
		}
		page.EndCursor = next
	}
	// Resolve membership without fetching fields from unrelated projects, then
	// hydrate only current-project items in one batch (at most 20 children).
	ids := []string{}
	indexes := map[string][]int{}
	for _, child := range connection.Nodes {
		if child == nil || child.ID == "" {
			continue
		}
		itemID, err := c.subIssueProjectItemID(ctx, projectID, child)
		if err != nil {
			return ItemsPage{}, err
		}
		item := Item{ID: child.ID, Type: "ISSUE", Content: projectContent(&child.rawContent), OutsideProject: itemID == ""}
		if itemID != "" {
			item.ID = itemID
			if _, exists := indexes[itemID]; !exists {
				ids = append(ids, itemID)
			}
			indexes[itemID] = append(indexes[itemID], len(page.Items))
		}
		page.Items = append(page.Items, item)
	}
	if len(ids) == 0 || len(fields) == 0 {
		return page, nil
	}
	variables = map[string]interface{}{"ids": ids}
	definitions, selection, names := selectedItemFields(fields, true, variables)
	query = fmt.Sprintf(`query SubIssueValues($ids: [ID!]!%s) { nodes(ids: $ids) { ... on ProjectV2Item { %s } } }`, definitions, selection)
	if len(names) > 0 {
		query += fieldValueFragment + linkedPullRequestsFragment
	}
	var values struct{ Nodes []json.RawMessage }
	if err := c.graphql.DoWithContext(ctx, query, variables, &values); err != nil {
		return ItemsPage{}, err
	}
	found := map[string]bool{}
	for _, node := range values.Nodes {
		raw, err := decodeSelectedItem(node, names)
		if err != nil {
			return ItemsPage{}, err
		}
		positions, exists := indexes[raw.ID]
		if !exists || found[raw.ID] {
			return ItemsPage{}, fmt.Errorf("unexpected sub-issue project item while loading values")
		}
		found[raw.ID] = true
		for _, index := range positions {
			raw.Content = nil
			loaded, err := c.projectItems(ctx, rawItemsPage{Nodes: []*rawItem{raw}})
			if err != nil {
				return ItemsPage{}, err
			}
			page.Items[index].FieldValues = loaded.Items[0].FieldValues
		}
	}
	if len(found) != len(ids) {
		return ItemsPage{}, fmt.Errorf("sub-issue project item disappeared while loading values")
	}
	return page, nil
}

func (c *Client) subIssueProjectItemID(ctx context.Context, projectID string, child *rawSubIssue) (string, error) {
	connection := child.ProjectItems
	after := ""
	seen := map[string]bool{}
	for {
		for _, node := range connection.Nodes {
			var identity struct {
				ID      string
				Project struct{ ID string }
			}
			if err := json.Unmarshal(node, &identity); err != nil {
				return "", err
			}
			if identity.Project.ID == projectID {
				if identity.ID == "" {
					return "", fmt.Errorf("sub-issue project item has no identity")
				}
				return identity.ID, nil
			}
		}
		if !connection.PageInfo.HasNextPage {
			return "", nil
		}
		next, err := nextCursor(connection.PageInfo, after)
		if err != nil || seen[next] {
			return "", fmt.Errorf("invalid sub-issue project-item pagination")
		}
		seen[next], after = true, next
		query := `query SubIssueProjectItems($id: ID!, $after: String) {
   node(id: $id) { ... on Issue { projectItems(first: 100, after: $after, includeArchived: false) {
    nodes { id project { id } } pageInfo { hasNextPage endCursor }
   } } }
  }`
		var response struct {
			Node *struct{ ProjectItems issueProjectItems }
		}
		if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": child.ID, "after": next}, &response); err != nil {
			return "", err
		}
		if response.Node == nil {
			return "", fmt.Errorf("sub-issue disappeared while loading project memberships")
		}
		connection = response.Node.ProjectItems
	}
}

// LoadIssueDetail supports nested issues which are not project items.
func (c *Client) LoadIssueDetail(ctx context.Context, issueID string) (ItemDetail, error) {
	query := `query NestedIssueDetail($id: ID!) { node(id: $id) { ... on Issue { ` + subIssueContentFields + ` body } } }`
	var response struct{ Node *rawContent }
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": issueID}, &response); err != nil {
		return ItemDetail{}, err
	}
	if response.Node == nil || response.Node.Kind != "Issue" || response.Node.ID != issueID {
		return ItemDetail{}, fmt.Errorf("nested issue is unavailable")
	}
	detail := ItemDetail{ID: issueID, Content: projectContent(response.Node)}
	detail.Comments, detail.CommentsError = c.loadIssueComments(ctx, issueID)
	detail.CommentsLoaded = detail.CommentsError == ""
	return detail, nil
}
