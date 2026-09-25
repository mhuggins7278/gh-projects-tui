package github

import (
	"context"
	"errors"
	"fmt"

	"github.com/cli/go-gh/v2/pkg/api"
)

const updateItemFieldValueMutation = `
mutation UpdateProjectItemFieldValue($input: UpdateProjectV2ItemFieldValueInput!) {
  updateProjectV2ItemFieldValue(input: $input) {
    projectV2Item { id }
  }
}`

const clearItemFieldValueMutation = `
mutation ClearProjectItemFieldValue($input: ClearProjectV2ItemFieldValueInput!) {
  clearProjectV2ItemFieldValue(input: $input) {
    projectV2Item { id }
  }
}`

const updateItemPositionMutation = `
mutation UpdateProjectItemPosition($input: UpdateProjectV2ItemPositionInput!) {
  updateProjectV2ItemPosition(input: $input) {
    clientMutationId
  }
}`

// FieldValueInput contains one Projects v2 field value variant. Pointer fields
// distinguish an omitted value from an intentionally empty string or list.
type FieldValueInput struct {
	Text                 *string
	Number               *float64
	Date                 *string
	SingleSelectOptionID *string
	MultiSelectOptionIDs []string
	IterationID          *string
}

type ItemFieldValueUpdate struct {
	ProjectID string
	ItemID    string
	FieldID   string
	Value     FieldValueInput
}

type ItemFieldValueClear struct {
	ProjectID string
	ItemID    string
	FieldID   string
}

type ItemPositionUpdate struct {
	ProjectID string
	ItemID    string
	AfterID   *string
}

const addIssueCommentMutation = `mutation AddIssueComment($input: AddCommentInput!) { addComment(input: $input) { commentEdge { node { id } } } }`
const closeIssueMutation = `mutation CloseIssue($input: CloseIssueInput!) { closeIssue(input: $input) { issue { id state } } }`
const reopenIssueMutation = `mutation ReopenIssue($input: ReopenIssueInput!) { reopenIssue(input: $input) { issue { id state } } }`

func (c *Client) IssueComment(ctx context.Context, issueID, body string) error {
	if err := validateMutationIDs(issueID); err != nil {
		return err
	}
	if body == "" {
		return fmt.Errorf("comment must not be empty")
	}
	var response struct {
		Add *struct {
			Edge *struct {
				Node *struct {
					ID string `json:"id"`
				} `json:"node"`
			} `json:"commentEdge"`
		} `json:"addComment"`
	}
	if err := c.graphql.DoWithContext(ctx, addIssueCommentMutation, map[string]interface{}{"input": map[string]interface{}{"issueId": issueID, "body": body}}, &response); err != nil {
		return classifyMutationError(err)
	}
	if response.Add == nil || response.Add.Edge == nil || response.Add.Edge.Node == nil {
		return classifyMutationError(fmt.Errorf("add comment returned no payload"))
	}
	return nil
}

func (c *Client) SetIssueClosed(ctx context.Context, issueID string, closed bool) error {
	if err := validateMutationIDs(issueID); err != nil {
		return err
	}
	mutation, field := closeIssueMutation, "closeIssue"
	if !closed {
		mutation, field = reopenIssueMutation, "reopenIssue"
	}
	var response struct {
		Close *struct {
			Issue *struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"issue"`
		} `json:"closeIssue"`
		Reopen *struct {
			Issue *struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"issue"`
		} `json:"reopenIssue"`
	}
	if err := c.graphql.DoWithContext(ctx, mutation, map[string]interface{}{"input": map[string]interface{}{"issueId": issueID}}, &response); err != nil {
		return classifyMutationError(err)
	}
	var issue *struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	if field == "closeIssue" && response.Close != nil {
		issue = response.Close.Issue
	}
	if field == "reopenIssue" && response.Reopen != nil {
		issue = response.Reopen.Issue
	}
	wantState := "CLOSED"
	if !closed {
		wantState = "OPEN"
	}
	if issue == nil || issue.ID != issueID || issue.State != wantState {
		return classifyMutationError(fmt.Errorf("issue state mutation returned no payload"))
	}
	return nil
}

// MutationError records whether a failed mutation may have reached GitHub.
// Ambiguous outcomes must be read back before another write can depend on them.
type MutationError struct {
	Err       error
	Ambiguous bool
}

func (e *MutationError) Error() string { return e.Err.Error() }

func (e *MutationError) Unwrap() error { return e.Err }

// IsAmbiguousMutationError reports whether an error lacks a definitive server
// rejection. Unknown errors are treated conservatively as ambiguous.
func IsAmbiguousMutationError(err error) bool {
	if err == nil {
		return false
	}
	var mutationErr *MutationError
	if errors.As(err, &mutationErr) {
		return mutationErr.Ambiguous
	}
	return true
}

func classifyMutationError(err error) error {
	if err == nil {
		return nil
	}
	ambiguous := true
	var graphQLError *api.GraphQLError
	if errors.As(err, &graphQLError) {
		ambiguous = false
	} else {
		var httpError *api.HTTPError
		if errors.As(err, &httpError) && httpError.StatusCode < 500 {
			ambiguous = false
		}
	}
	return &MutationError{Err: err, Ambiguous: ambiguous}
}

type updateItemFieldValueResponse struct {
	Update *struct {
		ProjectV2Item *struct {
			ID string `json:"id"`
		} `json:"projectV2Item"`
	} `json:"updateProjectV2ItemFieldValue"`
}

type clearItemFieldValueResponse struct {
	Clear *struct {
		ProjectV2Item *struct {
			ID string `json:"id"`
		} `json:"projectV2Item"`
	} `json:"clearProjectV2ItemFieldValue"`
}

type updateItemPositionResponse struct {
	Update *struct {
		ClientMutationID *string `json:"clientMutationId"`
	} `json:"updateProjectV2ItemPosition"`
}

// UpdateItemFieldValue sets a single Projects v2 field value variant.
func (c *Client) UpdateItemFieldValue(ctx context.Context, request ItemFieldValueUpdate) error {
	value, err := request.Value.input()
	if err != nil {
		return err
	}
	if err := validateMutationIDs(request.ProjectID, request.ItemID, request.FieldID); err != nil {
		return err
	}

	var response updateItemFieldValueResponse
	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"projectId": request.ProjectID,
			"itemId":    request.ItemID,
			"fieldId":   request.FieldID,
			"value":     value,
		},
	}
	if err := c.graphql.DoWithContext(ctx, updateItemFieldValueMutation, variables, &response); err != nil {
		return classifyMutationError(err)
	}
	if response.Update == nil || response.Update.ProjectV2Item == nil {
		return classifyMutationError(fmt.Errorf("update field value returned no project item"))
	}
	return nil
}

// ClearItemFieldValue removes a Projects v2 field value, producing the
// project's explicit no-value state.
func (c *Client) ClearItemFieldValue(ctx context.Context, request ItemFieldValueClear) error {
	if err := validateMutationIDs(request.ProjectID, request.ItemID, request.FieldID); err != nil {
		return err
	}

	var response clearItemFieldValueResponse
	variables := map[string]interface{}{
		"input": map[string]interface{}{
			"projectId": request.ProjectID,
			"itemId":    request.ItemID,
			"fieldId":   request.FieldID,
		},
	}
	if err := c.graphql.DoWithContext(ctx, clearItemFieldValueMutation, variables, &response); err != nil {
		return classifyMutationError(err)
	}
	if response.Clear == nil || response.Clear.ProjectV2Item == nil {
		return classifyMutationError(fmt.Errorf("clear field value returned no project item"))
	}
	return nil
}

// UpdateItemPosition moves an item after the supplied project-wide anchor.
// A nil AfterID requests the project's top position.
func (c *Client) UpdateItemPosition(ctx context.Context, request ItemPositionUpdate) error {
	if err := validateMutationIDs(request.ProjectID, request.ItemID); err != nil {
		return err
	}

	input := map[string]interface{}{
		"projectId": request.ProjectID,
		"itemId":    request.ItemID,
	}
	if request.AfterID != nil {
		input["afterId"] = *request.AfterID
	}

	var response updateItemPositionResponse
	if err := c.graphql.DoWithContext(ctx, updateItemPositionMutation, map[string]interface{}{"input": input}, &response); err != nil {
		return classifyMutationError(err)
	}
	if response.Update == nil {
		return classifyMutationError(fmt.Errorf("update item position returned no payload"))
	}
	return nil
}

func (value FieldValueInput) input() (map[string]interface{}, error) {
	result := make(map[string]interface{}, 1)
	count := 0
	if value.Text != nil {
		result["text"] = *value.Text
		count++
	}
	if value.Number != nil {
		result["number"] = *value.Number
		count++
	}
	if value.Date != nil {
		result["date"] = *value.Date
		count++
	}
	if value.SingleSelectOptionID != nil {
		result["singleSelectOptionId"] = *value.SingleSelectOptionID
		count++
	}
	if value.MultiSelectOptionIDs != nil {
		result["multiSelectOptionIds"] = value.MultiSelectOptionIDs
		count++
	}
	if value.IterationID != nil {
		result["iterationId"] = *value.IterationID
		count++
	}
	if count != 1 {
		return nil, fmt.Errorf("field value update must contain exactly one value variant")
	}
	return result, nil
}

func validateMutationIDs(values ...string) error {
	for _, value := range values {
		if value == "" {
			return fmt.Errorf("mutation IDs must not be empty")
		}
	}
	return nil
}
