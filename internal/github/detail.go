package github

import (
	"context"
	"fmt"
)

const itemDetailFields = `
fragment ItemDetailFields on ProjectV2Item {
  id
  content {
    __typename
    ... on Issue { id number title url body }
    ... on PullRequest { id number title url body }
    ... on DraftIssue { id title body }
  }
  fieldValues(first: 100, after: $valuesAfter) { ...FieldValuesPage }
}`

const userItemDetailQuery = `
query UserProjectItemDetail($login: String!, $project: Int!, $itemID: ID!, $fieldsAfter: String, $valuesAfter: String) {
  user(login: $login) {
    projectV2(number: $project) {
      fields(first: 100, after: $fieldsAfter) {
        nodes { ...FieldConfiguration }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
  node(id: $itemID) { ...ItemDetailFields }
}` + itemDetailFields + itemFieldValueFragments + fieldConfigurationFragment

const organizationItemDetailQuery = `
query OrganizationProjectItemDetail($login: String!, $project: Int!, $itemID: ID!, $fieldsAfter: String, $valuesAfter: String) {
  organization(login: $login) {
    projectV2(number: $project) {
      fields(first: 100, after: $fieldsAfter) {
        nodes { ...FieldConfiguration }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
  node(id: $itemID) { ...ItemDetailFields }
}` + itemDetailFields + itemFieldValueFragments + fieldConfigurationFragment

type ItemDetail struct {
	ID      string
	Content *Content
	Fields  []DetailField
}

type DetailField struct {
	Field Field
	Value *FieldValue
}

type itemDetailResponse struct {
	User         *detailOwner   `json:"user"`
	Organization *detailOwner   `json:"organization"`
	Node         *rawDetailItem `json:"node"`
}

type detailOwner struct {
	Project *detailProject `json:"projectV2"`
}

type detailProject struct {
	Fields fieldConnection `json:"fields"`
}

type rawDetailItem struct {
	ID          string            `json:"id"`
	Content     *rawContent       `json:"content"`
	FieldValues rawFieldValuePage `json:"fieldValues"`
}

type itemDetailPage struct {
	ProjectFields fieldConnection
	Item          *rawDetailItem
}

// LoadItemDetail fetches the selected item's full project field definitions,
// field values, and accessible issue/PR/draft body. Definitions and values
// are paginated independently because either connection may exceed one page.
func (c *Client) LoadItemDetail(ctx context.Context, owner Owner, projectNumber int, itemID string) (ItemDetail, error) {
	page, err := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, "", "")
	if err != nil {
		return ItemDetail{}, err
	}
	if page.Item == nil {
		return ItemDetail{}, fmt.Errorf("project item %q was not found or is inaccessible", itemID)
	}

	fields := append([]*rawField(nil), page.ProjectFields.Nodes...)
	values := append([]*rawFieldValue(nil), page.Item.FieldValues.Nodes...)
	fieldsAfter := ""
	for page.ProjectFields.PageInfo.HasNextPage {
		next, cursorErr := nextCursor(page.ProjectFields.PageInfo, fieldsAfter)
		if cursorErr != nil {
			return ItemDetail{}, cursorErr
		}
		nextPage, fetchErr := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, next, "")
		if fetchErr != nil {
			return ItemDetail{}, fetchErr
		}
		fields = append(fields, nextPage.ProjectFields.Nodes...)
		page.ProjectFields = nextPage.ProjectFields
		fieldsAfter = next
	}

	valuesAfter := ""
	for page.Item.FieldValues.PageInfo.HasNextPage {
		next, cursorErr := nextCursor(page.Item.FieldValues.PageInfo, valuesAfter)
		if cursorErr != nil {
			return ItemDetail{}, cursorErr
		}
		nextPage, fetchErr := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, "", next)
		if fetchErr != nil {
			return ItemDetail{}, fetchErr
		}
		if nextPage.Item == nil {
			return ItemDetail{}, fmt.Errorf("project item %q disappeared while loading detail", itemID)
		}
		values = append(values, nextPage.Item.FieldValues.Nodes...)
		page.Item.FieldValues = nextPage.Item.FieldValues
		valuesAfter = next
	}

	valueByField := make(map[string]FieldValue, len(values))
	for _, rawValue := range values {
		if rawValue != nil {
			valueByField[rawValue.Field.ID] = rawValue.project()
		}
	}
	detail := ItemDetail{ID: page.Item.ID, Content: projectContent(page.Item.Content)}
	seen := make(map[string]bool, len(fields))
	for _, rawField := range fields {
		if rawField == nil {
			continue
		}
		field := rawField.project()
		seen[field.ID] = true
		value, ok := valueByField[field.ID]
		if ok {
			valueCopy := value
			detail.Fields = append(detail.Fields, DetailField{Field: field, Value: &valueCopy})
		} else {
			detail.Fields = append(detail.Fields, DetailField{Field: field})
		}
	}
	// Keep values whose definition is unavailable visible rather than silently
	// dropping them. This can happen when a field was deleted or inaccessible.
	for _, rawValue := range values {
		if rawValue == nil || seen[rawValue.Field.ID] {
			continue
		}
		value := rawValue.project()
		detail.Fields = append(detail.Fields, DetailField{
			Field: Field{ID: value.FieldID, Name: value.FieldName, Kind: value.Kind},
			Value: &value,
		})
	}
	return detail, nil
}

func (c *Client) fetchItemDetailPage(ctx context.Context, owner Owner, projectNumber int, itemID, fieldsAfter, valuesAfter string) (itemDetailPage, error) {
	var response itemDetailResponse
	variables := map[string]interface{}{
		"login":       owner.Login,
		"project":     projectNumber,
		"itemID":      itemID,
		"fieldsAfter": nullableCursor(fieldsAfter),
		"valuesAfter": nullableCursor(valuesAfter),
	}
	query := organizationItemDetailQuery
	if owner.Kind == UserOwner {
		query = userItemDetailQuery
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return itemDetailPage{}, err
	}
	var project *detailProject
	switch owner.Kind {
	case UserOwner:
		if response.User != nil {
			project = response.User.Project
		}
	case OrganizationOwner:
		if response.Organization != nil {
			project = response.Organization.Project
		}
	default:
		return itemDetailPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if project == nil {
		return itemDetailPage{}, fmt.Errorf("project %d for %s %q was not found", projectNumber, owner.Kind, owner.Login)
	}
	return itemDetailPage{ProjectFields: project.Fields, Item: response.Node}, nil
}
