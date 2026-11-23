package github

import (
	"context"
	"fmt"
)

type issuePermission string

const (
	issueClosePermission     issuePermission = "viewerCanClose"
	issueReopenPermission    issuePermission = "viewerCanReopen"
	issueSetFieldsPermission issuePermission = "viewerCanSetFields"
)

// requireIssuePermission makes a small, uncached read for the issue being
// changed, instead of computing permissions for every card during board load.
// Every failure is definitive: this preflight has not submitted a mutation.
func (c *Client) requireIssuePermission(ctx context.Context, issueID string, permission issuePermission) error {
	// The selected field is an internal constant, never caller-supplied text.
	query := fmt.Sprintf(`query IssuePermission($id: ID!) { node(id: $id) { ... on Issue { id allowed: %s } } }`, permission)
	var response struct {
		Node *struct {
			ID      string
			Allowed *bool
		}
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": issueID}, &response); err != nil {
		return &MutationError{Err: fmt.Errorf("could not check issue permission: %w", err)}
	}
	if response.Node == nil || response.Node.ID != issueID || response.Node.Allowed == nil {
		return &MutationError{Err: fmt.Errorf("issue permission is unavailable")}
	}
	if !*response.Node.Allowed {
		return &MutationError{Err: fmt.Errorf("you do not have permission to change this issue")}
	}
	return nil
}
