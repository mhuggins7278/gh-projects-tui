package github

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type IssueMetadataKind string

const (
	IssueLabels    IssueMetadataKind = "labels"
	IssueAssignees IssueMetadataKind = "assignees"
)

type IssueMetadataOption struct{ ID, Name string }
type IssueMetadata struct {
	Current    []IssueMetadataOption
	Candidates []IssueMetadataOption
	CanEdit    bool
}

type metadataConnection struct {
	Nodes    []*IssueMetadataOption `json:"nodes"`
	PageInfo pageInfo               `json:"pageInfo"`
}

// ReadIssueMetadata fetches authoritative issue membership and, when requested,
// all repository candidates. Current members are retained even when no longer
// offered by the repository's candidate connection.
func (c *Client) ReadIssueMetadata(ctx context.Context, issueID string, kind IssueMetadataKind, candidates bool) (IssueMetadata, error) {
	if err := validateMutationIDs(issueID); err != nil {
		return IssueMetadata{}, err
	}
	issueField, repoField, nameField, permission := "labels", "labels", "name", "viewerCanLabel"
	switch kind {
	case IssueLabels:
	case IssueAssignees:
		issueField, repoField, nameField, permission = "assignees", "assignableUsers", "name: login", "viewerCanAssign"
	default:
		return IssueMetadata{}, fmt.Errorf("unknown issue metadata type %q", kind)
	}
	read := func(repo bool) ([]IssueMetadataOption, bool, error) {
		selection := fmt.Sprintf("values: %s(first: 100, after: $after) { nodes { id %s } pageInfo { hasNextPage endCursor } }", issueField, nameField)
		if repo {
			selection = fmt.Sprintf("repository { values: %s(first: 100, after: $after) { nodes { id %s } pageInfo { hasNextPage endCursor } } }", repoField, nameField)
		}
		query := fmt.Sprintf("query IssueMetadata($id: ID!, $after: String) { node(id: $id) { ... on Issue { id permission: %s %s } } }", permission, selection)
		var result []IssueMetadataOption
		var cursor interface{}
		seen := map[string]bool{}
		allowed := false
		for {
			var response struct {
				Node *struct {
					ID         string              `json:"id"`
					Permission *bool               `json:"permission"`
					Values     *metadataConnection `json:"values"`
					Repository *struct {
						Values *metadataConnection `json:"values"`
					} `json:"repository"`
				} `json:"node"`
			}
			if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": issueID, "after": cursor}, &response); err != nil {
				return nil, false, err
			}
			if response.Node == nil || response.Node.ID != issueID || response.Node.Permission == nil {
				return nil, false, fmt.Errorf("issue metadata is unavailable")
			}
			allowed = *response.Node.Permission
			connection := response.Node.Values
			if repo {
				if response.Node.Repository == nil {
					return nil, false, fmt.Errorf("issue repository is unavailable")
				}
				connection = response.Node.Repository.Values
			}
			if connection == nil {
				return nil, false, fmt.Errorf("issue metadata connection is unavailable")
			}
			for _, option := range connection.Nodes {
				if option == nil || option.ID == "" || option.Name == "" {
					return nil, false, fmt.Errorf("issue metadata contains an unavailable entry")
				}
				result = append(result, *option)
			}
			if !connection.PageInfo.HasNextPage {
				break
			}
			if connection.PageInfo.EndCursor == nil || *connection.PageInfo.EndCursor == "" || seen[*connection.PageInfo.EndCursor] {
				return nil, false, fmt.Errorf("issue metadata pagination did not advance")
			}
			seen[*connection.PageInfo.EndCursor] = true
			cursor = *connection.PageInfo.EndCursor
		}
		return result, allowed, nil
	}
	current, allowed, err := read(false)
	if err != nil {
		return IssueMetadata{}, err
	}
	result := IssueMetadata{Current: current, CanEdit: allowed}
	if !candidates {
		return result, nil
	}
	options, repoAllowed, err := read(true)
	if err != nil {
		return IssueMetadata{}, err
	}
	result.CanEdit = allowed && repoAllowed
	byID := make(map[string]IssueMetadataOption)
	for _, option := range append(options, current...) {
		byID[option.ID] = option
	}
	for _, option := range byID {
		result.Candidates = append(result.Candidates, option)
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		left, right := strings.ToLower(result.Candidates[i].Name), strings.ToLower(result.Candidates[j].Name)
		if left == right {
			return result.Candidates[i].ID < result.Candidates[j].ID
		}
		return left < right
	})
	return result, nil
}

// ChangeIssueMetadata adds or removes only the selected IDs, avoiding replacement
// of concurrent changes to other labels or assignees.
func (c *Client) ChangeIssueMetadata(ctx context.Context, issueID string, kind IssueMetadataKind, add bool, ids []string) error {
	if err := validateMutationIDs(issueID); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	for _, id := range ids {
		if err := validateMutationIDs(id); err != nil {
			return err
		}
	}
	var name, inputType, targetKey, idsKey, payloadKey string
	switch kind {
	case IssueLabels:
		name, inputType, targetKey, idsKey, payloadKey = "addLabelsToLabelable", "AddLabelsToLabelableInput", "labelableId", "labelIds", "labelable"
		if !add {
			name, inputType = "removeLabelsFromLabelable", "RemoveLabelsFromLabelableInput"
		}
	case IssueAssignees:
		name, inputType, targetKey, idsKey, payloadKey = "addAssigneesToAssignable", "AddAssigneesToAssignableInput", "assignableId", "assigneeIds", "assignable"
		if !add {
			name, inputType = "removeAssigneesFromAssignable", "RemoveAssigneesFromAssignableInput"
		}
	default:
		return fmt.Errorf("unknown issue metadata type %q", kind)
	}
	query := fmt.Sprintf("mutation IssueMetadataChange($input: %s!) { result: %s(input: $input) { target: %s { ... on Issue { id } } } }", inputType, name, payloadKey)
	var response struct {
		Result *struct {
			Target *struct {
				ID string `json:"id"`
			} `json:"target"`
		} `json:"result"`
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"input": map[string]interface{}{targetKey: issueID, idsKey: ids}}, &response); err != nil {
		return classifyMutationError(err)
	}
	if response.Result == nil || response.Result.Target == nil || response.Result.Target.ID != issueID {
		return classifyMutationError(fmt.Errorf("issue metadata mutation returned no matching issue"))
	}
	return nil
}

// SearchIssueAssignees retrieves a single page of users assignable to the
// issue's repository. Current issue assignees are read separately.
type AssigneeSearchPage struct {
	Options   []IssueMetadataOption
	EndCursor string
	HasNext   bool
	CanEdit   bool
}

func (c *Client) SearchIssueAssignees(ctx context.Context, issueID, search, after string) (AssigneeSearchPage, error) {
	if err := validateMutationIDs(issueID); err != nil {
		return AssigneeSearchPage{}, err
	}
	const query = `query SearchIssueAssignees($id: ID!, $query: String!, $after: String) {
  node(id: $id) { ... on Issue { id viewerCanAssign repository {
    assignableUsers(first: 30, query: $query, after: $after) {
      nodes { id name: login } pageInfo { hasNextPage endCursor }
    }
  } } }
}`
	var response struct {
		Node *struct {
			ID         string `json:"id"`
			CanEdit    *bool  `json:"viewerCanAssign"`
			Repository *struct {
				Users *metadataConnection `json:"assignableUsers"`
			} `json:"repository"`
		} `json:"node"`
	}
	var cursor interface{}
	if after != "" {
		cursor = after
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": issueID, "query": search, "after": cursor}, &response); err != nil {
		return AssigneeSearchPage{}, err
	}
	if response.Node == nil || response.Node.ID != issueID || response.Node.CanEdit == nil || response.Node.Repository == nil || response.Node.Repository.Users == nil {
		return AssigneeSearchPage{}, fmt.Errorf("assignable users are unavailable")
	}
	connection := response.Node.Repository.Users
	page := AssigneeSearchPage{CanEdit: *response.Node.CanEdit, HasNext: connection.PageInfo.HasNextPage}
	for _, option := range connection.Nodes {
		if option == nil || option.ID == "" || option.Name == "" {
			return AssigneeSearchPage{}, fmt.Errorf("assignable users contain an unavailable entry")
		}
		page.Options = append(page.Options, *option)
	}
	if page.HasNext {
		if connection.PageInfo.EndCursor == nil || *connection.PageInfo.EndCursor == "" || *connection.PageInfo.EndCursor == after {
			return AssigneeSearchPage{}, fmt.Errorf("assignee search pagination did not advance")
		}
		page.EndCursor = *connection.PageInfo.EndCursor
	}
	return page, nil
}
