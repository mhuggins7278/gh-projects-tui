package github

import (
	"context"
	"fmt"
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
		return err
	}
	if response.Update == nil || response.Update.ProjectV2Item == nil {
		return fmt.Errorf("update field value returned no project item")
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
		return err
	}
	if response.Clear == nil || response.Clear.ProjectV2Item == nil {
		return fmt.Errorf("clear field value returned no project item")
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
		return err
	}
	if response.Update == nil {
		return fmt.Errorf("update item position returned no payload")
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
