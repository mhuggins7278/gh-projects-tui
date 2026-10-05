package github

import (
	"context"
	"encoding/json"
	"fmt"
)

const subIssueContentFields = `__typename id number title url state repository { name nameWithOwner } subIssuesSummary { total completed }`

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
	definitions, selection, names := selectedItemFields(fields, true, variables)
	query := fmt.Sprintf(`query IssueSubIssues($id: ID!, $after: String%s) {
  node(id: $id) { ... on Issue {
    subIssues(first: 20, after: $after) {
      nodes { %s projectItems(first: 10, includeArchived: false) {
        nodes { project { id } %s } pageInfo { hasNextPage endCursor }
      } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`, definitions, subIssueContentFields, selection)
	if len(names) > 0 {
		query += fieldValueFragment + linkedPullRequestsFragment
	}
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
	for _, child := range connection.Nodes {
		if child == nil || child.ID == "" {
			continue
		}
		item, err := c.subIssueProjectItem(ctx, projectID, child, fields, names)
		if err != nil {
			return ItemsPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func (c *Client) subIssueProjectItem(ctx context.Context, projectID string, child *rawSubIssue, fields []Field, names []string) (Item, error) {
	connection := child.ProjectItems
	after := ""
	seen := map[string]bool{}
	for {
		for _, node := range connection.Nodes {
			var identity struct {
				Project struct{ ID string }
			}
			if err := json.Unmarshal(node, &identity); err != nil {
				return Item{}, err
			}
			if identity.Project.ID != projectID {
				continue
			}
			raw, err := decodeSelectedItem(node, names)
			if err != nil {
				return Item{}, err
			}
			raw.Type, raw.Content = "ISSUE", &child.rawContent
			page, err := c.projectItems(ctx, rawItemsPage{Nodes: []*rawItem{raw}})
			if err != nil {
				return Item{}, err
			}
			return page.Items[0], nil
		}
		if !connection.PageInfo.HasNextPage {
			return Item{ID: child.ID, Type: "ISSUE", Content: projectContent(&child.rawContent), OutsideProject: true}, nil
		}
		next, err := nextCursor(connection.PageInfo, after)
		if err != nil || seen[next] {
			return Item{}, fmt.Errorf("invalid sub-issue project-item pagination")
		}
		seen[next], after = true, next
		variables := map[string]interface{}{"id": child.ID, "after": next}
		definitions, selection, _ := selectedItemFields(fields, true, variables)
		query := fmt.Sprintf(`query SubIssueProjectItems($id: ID!, $after: String%s) {
  node(id: $id) { ... on Issue { projectItems(first: 10, after: $after, includeArchived: false) {
    nodes { project { id } %s } pageInfo { hasNextPage endCursor }
  } } }
}`, definitions, selection)
		if len(names) > 0 {
			query += fieldValueFragment + linkedPullRequestsFragment
		}
		var response struct {
			Node *struct{ ProjectItems issueProjectItems }
		}
		if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
			return Item{}, err
		}
		if response.Node == nil {
			return Item{}, fmt.Errorf("sub-issue disappeared while loading project values")
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
