package github

import (
	"context"
	"fmt"
	"time"
)

const itemDetailFields = `
fragment ItemDetailFields on ProjectV2Item {
  id
  content @include(if: $includeBody) {
    __typename
    ... on Issue { id number title url body state }
    ... on PullRequest { id number title url body state }
    ... on DraftIssue { id title body }
  }
  fieldValues(first: 100, after: $valuesAfter) @include(if: $includeValues) { ...DetailFieldValuesPage }
}`

const userItemDetailQuery = `
query UserProjectItemDetail($login: String!, $project: Int!, $itemID: ID!, $fieldsAfter: String, $valuesAfter: String, $includeFields: Boolean!, $includeBody: Boolean!, $includeValues: Boolean!) {
  user(login: $login) {
    projectV2(number: $project) {
      fields(first: 100, after: $fieldsAfter) @include(if: $includeFields) {
        nodes { ...FieldConfiguration }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
  node(id: $itemID) { ...ItemDetailFields }
}` + itemDetailFields + fieldValueFragment + itemDetailFieldValueFragments + fieldConfigurationFragment

const organizationItemDetailQuery = `
query OrganizationProjectItemDetail($login: String!, $project: Int!, $itemID: ID!, $fieldsAfter: String, $valuesAfter: String, $includeFields: Boolean!, $includeBody: Boolean!, $includeValues: Boolean!) {
  organization(login: $login) {
    projectV2(number: $project) {
      fields(first: 100, after: $fieldsAfter) @include(if: $includeFields) {
        nodes { ...FieldConfiguration }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
  node(id: $itemID) { ...ItemDetailFields }
}` + itemDetailFields + fieldValueFragment + itemDetailFieldValueFragments + fieldConfigurationFragment

const itemNestedFieldValueQuery = `
query ItemNestedFieldValue($itemID: ID!, $fieldName: String!, $labelsAfter: String, $usersAfter: String, $reviewersAfter: String, $pullRequestsAfter: String) {
  node(id: $itemID) {
    ... on ProjectV2Item {
      fieldValueByName(name: $fieldName) {
        __typename
        ... on ProjectV2ItemFieldLabelValue {
          labels(first: 100, after: $labelsAfter) { nodes { name } pageInfo { hasNextPage endCursor } }
        }
        ... on ProjectV2ItemFieldUserValue {
          users(first: 100, after: $usersAfter) { nodes { login } pageInfo { hasNextPage endCursor } }
        }
        ... on ProjectV2ItemFieldReviewerValue {
          reviewers(first: 100, after: $reviewersAfter) {
            nodes { __typename ... on User { login } ... on Team { name } }
            pageInfo { hasNextPage endCursor }
          }
        }
        ... on ProjectV2ItemFieldPullRequestValue {
          pullRequests(first: 100, after: $pullRequestsAfter) {
            nodes { number title url repository { nameWithOwner } }
            pageInfo { hasNextPage endCursor }
          }
        }
      }
    }
  }
}`

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

type nestedFieldValueResponse struct {
	Node *struct {
		Value *rawFieldValue `json:"fieldValueByName"`
	} `json:"node"`
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
	key := fmt.Sprintf("%s/%s/%d", owner.Kind, owner.Login, projectNumber)
	var cached *definitionCacheEntry
	if value, ok := c.definitions.Load(key); ok {
		entry := value.(definitionCacheEntry)
		if time.Since(entry.at) < 30*time.Second {
			cached = &entry
		}
	}
	page, err := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, "", "", cached == nil)
	if err != nil {
		return ItemDetail{}, err
	}
	if page.Item == nil {
		return ItemDetail{}, fmt.Errorf("project item %q was not found or is inaccessible", itemID)
	}
	detail := ItemDetail{ID: page.Item.ID, Content: projectContent(page.Item.Content)}
	if cached != nil {
		page.ProjectFields = fieldConnection{Nodes: cached.fields}
	}

	fields := append([]*rawField(nil), page.ProjectFields.Nodes...)
	values := append([]*rawFieldValue(nil), page.Item.FieldValues.Nodes...)
	fieldsAfter := ""
	for page.ProjectFields.PageInfo.HasNextPage {
		next, cursorErr := nextCursor(page.ProjectFields.PageInfo, fieldsAfter)
		if cursorErr != nil {
			return ItemDetail{}, cursorErr
		}
		nextPage, fetchErr := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, next, "", true)
		if fetchErr != nil {
			return ItemDetail{}, fetchErr
		}
		fields = append(fields, nextPage.ProjectFields.Nodes...)
		page.ProjectFields = nextPage.ProjectFields
		fieldsAfter = next
	}
	if cached == nil {
		// Bounded metadata cache, scoped to this authenticated client instance.
		count := 0
		c.definitions.Range(func(_, _ interface{}) bool { count++; return count < 64 })
		if count >= 64 {
			c.definitions.Clear()
		}
		c.definitions.Store(key, definitionCacheEntry{fields: fields, at: time.Now()})
	}

	valuesAfter := ""
	for page.Item.FieldValues.PageInfo.HasNextPage {
		next, cursorErr := nextCursor(page.Item.FieldValues.PageInfo, valuesAfter)
		if cursorErr != nil {
			return ItemDetail{}, cursorErr
		}
		nextPage, fetchErr := c.fetchItemDetailPage(ctx, owner, projectNumber, itemID, "", next, false)
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
	for _, value := range values {
		if value == nil || value.Field.Name == "" {
			continue
		}
		for nestedHasNext(value) {
			after, err := nestedEndCursor(value)
			if err != nil {
				return ItemDetail{}, fmt.Errorf("field %q nested values: %w", value.Field.Name, err)
			}
			next, fetchErr := c.fetchNestedFieldValuePage(ctx, itemID, value.Field.Name, value.Kind, after)
			if fetchErr != nil {
				return ItemDetail{}, fetchErr
			}
			if err := appendNestedFieldValuePage(value, next, after); err != nil {
				return ItemDetail{}, fmt.Errorf("field %q nested values: %w", value.Field.Name, err)
			}
		}
	}

	valueByField := make(map[string]FieldValue, len(values))
	for _, rawValue := range values {
		if rawValue != nil {
			valueByField[rawValue.Field.ID] = rawValue.project()
		}
	}
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

func (c *Client) fetchNestedFieldValuePage(ctx context.Context, itemID, fieldName, kind, after string) (*rawFieldValue, error) {
	variables := map[string]interface{}{
		"itemID": itemID, "fieldName": fieldName,
		"labelsAfter": nil, "usersAfter": nil, "reviewersAfter": nil, "pullRequestsAfter": nil,
	}
	switch kind {
	case "ProjectV2ItemFieldLabelValue":
		variables["labelsAfter"] = nullableCursor(after)
	case "ProjectV2ItemFieldUserValue":
		variables["usersAfter"] = nullableCursor(after)
	case "ProjectV2ItemFieldReviewerValue":
		variables["reviewersAfter"] = nullableCursor(after)
	case "ProjectV2ItemFieldPullRequestValue":
		variables["pullRequestsAfter"] = nullableCursor(after)
	default:
		return nil, fmt.Errorf("field value type %q has no paginated nested values", kind)
	}
	var response nestedFieldValueResponse
	if err := c.graphql.DoWithContext(ctx, itemNestedFieldValueQuery, variables, &response); err != nil {
		return nil, err
	}
	if response.Node == nil || response.Node.Value == nil {
		return nil, fmt.Errorf("nested value for item %q field %q is unavailable", itemID, fieldName)
	}
	return response.Node.Value, nil
}

func nestedHasNext(value *rawFieldValue) bool {
	switch value.Kind {
	case "ProjectV2ItemFieldLabelValue":
		return value.Labels != nil && value.Labels.PageInfo.HasNextPage
	case "ProjectV2ItemFieldUserValue":
		return value.Users != nil && value.Users.PageInfo.HasNextPage
	case "ProjectV2ItemFieldReviewerValue":
		return value.Reviewers != nil && value.Reviewers.PageInfo.HasNextPage
	case "ProjectV2ItemFieldPullRequestValue":
		return value.PullRequests != nil && value.PullRequests.PageInfo.HasNextPage
	default:
		return false
	}
}

func nestedEndCursor(value *rawFieldValue) (string, error) {
	var info pageInfo
	switch value.Kind {
	case "ProjectV2ItemFieldLabelValue":
		info = value.Labels.PageInfo
	case "ProjectV2ItemFieldUserValue":
		info = value.Users.PageInfo
	case "ProjectV2ItemFieldReviewerValue":
		info = value.Reviewers.PageInfo
	case "ProjectV2ItemFieldPullRequestValue":
		info = value.PullRequests.PageInfo
	}
	return nextCursor(info, "")
}

func appendNestedFieldValuePage(value, next *rawFieldValue, previous string) error {
	if value.Kind != next.Kind {
		return fmt.Errorf("nested value type changed from %q to %q", value.Kind, next.Kind)
	}
	var info pageInfo
	switch value.Kind {
	case "ProjectV2ItemFieldLabelValue":
		if next.Labels == nil {
			return fmt.Errorf("label connection was not returned")
		}
		value.Labels.Nodes = append(value.Labels.Nodes, next.Labels.Nodes...)
		value.Labels.PageInfo = next.Labels.PageInfo
		info = next.Labels.PageInfo
	case "ProjectV2ItemFieldUserValue":
		if next.Users == nil {
			return fmt.Errorf("user connection was not returned")
		}
		value.Users.Nodes = append(value.Users.Nodes, next.Users.Nodes...)
		value.Users.PageInfo = next.Users.PageInfo
		info = next.Users.PageInfo
	case "ProjectV2ItemFieldReviewerValue":
		if next.Reviewers == nil {
			return fmt.Errorf("reviewer connection was not returned")
		}
		value.Reviewers.Nodes = append(value.Reviewers.Nodes, next.Reviewers.Nodes...)
		value.Reviewers.PageInfo = next.Reviewers.PageInfo
		info = next.Reviewers.PageInfo
	case "ProjectV2ItemFieldPullRequestValue":
		if next.PullRequests == nil {
			return fmt.Errorf("pull request connection was not returned")
		}
		value.PullRequests.Nodes = append(value.PullRequests.Nodes, next.PullRequests.Nodes...)
		value.PullRequests.PageInfo = next.PullRequests.PageInfo
		info = next.PullRequests.PageInfo
	default:
		return fmt.Errorf("field value type %q has no paginated nested values", value.Kind)
	}
	if info.HasNextPage {
		_, err := nextCursor(info, previous)
		return err
	}
	return nil
}

type definitionCacheEntry struct {
	fields []*rawField
	at     time.Time
}

func (c *Client) fetchItemDetailPage(ctx context.Context, owner Owner, projectNumber int, itemID, fieldsAfter, valuesAfter string, includeFields bool) (itemDetailPage, error) {
	var response itemDetailResponse
	variables := map[string]interface{}{
		"login":         owner.Login,
		"project":       projectNumber,
		"itemID":        itemID,
		"fieldsAfter":   nullableCursor(fieldsAfter),
		"valuesAfter":   nullableCursor(valuesAfter),
		"includeFields": includeFields,
		"includeValues": fieldsAfter == "",
		"includeBody":   fieldsAfter == "" && valuesAfter == "",
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
