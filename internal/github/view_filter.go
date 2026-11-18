package github

import (
	"context"
	"fmt"
)

func (c *Client) readViewFilter(ctx context.Context, id string) (string, error) {
	var response struct{ Node *struct{ ID, Filter string } }
	query := `query ViewFilterRead($id: ID!) { node(id: $id) { ... on ProjectV2View { id filter } } }`
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"id": id}, &response); err != nil {
		return "", err
	}
	if response.Node == nil || response.Node.ID != id {
		return "", fmt.Errorf("saved view is unavailable")
	}
	return response.Node.Filter, nil
}

// SaveViewFilter checks for intervening edits and confirms even uncertain writes
// through a fresh read. It never retries the mutation automatically.
func (c *Client) SaveViewFilter(ctx context.Context, id, original, filter string) error {
	if err := validateMutationIDs(id); err != nil {
		return err
	}
	current, err := c.readViewFilter(ctx, id)
	if err != nil {
		return err
	}
	if current == filter {
		return nil
	}
	if current != original {
		return fmt.Errorf("the saved filter changed on GitHub; refresh the view before saving")
	}
	query := `mutation SaveViewFilter($input: UpdateProjectV2ViewInput!) { updateProjectV2View(input: $input) { projectV2View { id filter } } }`
	var response struct {
		Update *struct {
			View *struct{ ID, Filter string } `json:"projectV2View"`
		} `json:"updateProjectV2View"`
	}
	err = c.graphql.DoWithContext(ctx, query, map[string]interface{}{"input": map[string]interface{}{"viewId": id, "filter": filter}}, &response)
	if err != nil && !IsAmbiguousMutationError(classifyMutationError(err)) {
		return err
	}
	actual, readErr := c.readViewFilter(ctx, id)
	if readErr == nil && actual == filter {
		return nil
	}
	if readErr != nil {
		return fmt.Errorf("save outcome could not be confirmed; refresh before retrying: %w", readErr)
	}
	return fmt.Errorf("GitHub has not confirmed the requested filter; refresh before retrying")
}
