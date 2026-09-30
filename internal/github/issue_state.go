package github

import (
	"context"
	"fmt"
	"strings"
)

// IssueState is the minimal authoritative result needed to reconcile a write.
type IssueState struct {
	ID    string
	State string
}

// ReadIssueState bypasses the read cache and does not load item details or
// comments. A missing, inaccessible, or non-issue node cannot confirm a write.
func (c *Client) ReadIssueState(ctx context.Context, issueID string) (IssueState, error) {
	if strings.TrimSpace(issueID) == "" {
		return IssueState{}, fmt.Errorf("issue ID must not be empty")
	}
	const query = `query IssueStateRead($id: ID!) { node(id: $id) { ... on Issue { id state } } }`
	var response struct {
		Node *IssueState
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": issueID}, &response); err != nil {
		return IssueState{}, err
	}
	if response.Node == nil || response.Node.ID != issueID || (response.Node.State != "OPEN" && response.Node.State != "CLOSED") {
		return IssueState{}, fmt.Errorf("issue state is unavailable or does not match the requested issue")
	}
	return *response.Node, nil
}
