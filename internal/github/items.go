package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

const fieldValueFragment = `
fragment FieldValue on ProjectV2ItemFieldValue {
  __typename
  ... on ProjectV2ItemFieldDateValue {
    field { ... on ProjectV2Field { id name } }
    date
  }
  ... on ProjectV2ItemFieldIterationValue {
    field { ... on ProjectV2IterationField { id name } }
    iterationId title
  }
  ... on ProjectV2ItemFieldLabelValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemFieldMilestoneValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemFieldMultiSelectValue {
    field { ... on ProjectV2MultiSelectField { id name } }
    options { id name }
    value
  }
  ... on ProjectV2ItemFieldNumberValue {
    field { ... on ProjectV2Field { id name } }
    number
  }
  ... on ProjectV2ItemFieldPullRequestValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemFieldRepositoryValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemFieldReviewerValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemFieldSingleSelectValue {
    field { ... on ProjectV2SingleSelectField { id name } }
    optionId name
  }
  ... on ProjectV2ItemFieldTextValue {
    field { ... on ProjectV2Field { id name } }
    text
  }
  ... on ProjectV2ItemFieldUserValue {
    field { ... on ProjectV2Field { id name } }
  }
  ... on ProjectV2ItemIssueFieldValue {
    field {
      ... on ProjectV2Field { id name }
      ... on ProjectV2SingleSelectField { id name }
      ... on ProjectV2MultiSelectField { id name }
    }
    issueFieldValue {
      __typename
      ... on IssueFieldDateValue { date: value }
      ... on IssueFieldMultiSelectValue { options { id name } }
      ... on IssueFieldNumberValue { number: value }
      ... on IssueFieldSingleSelectValue { optionId name }
      ... on IssueFieldTextValue { text: value }
    }
  }
}`

const itemFieldValueFragments = `
fragment FieldValuesPage on ProjectV2ItemFieldValueConnection {
  nodes {
    ...FieldValue
    ... on ProjectV2ItemFieldUserValue {
      users(first: 10) { nodes { login } pageInfo { hasNextPage endCursor } }
    }
  }
  pageInfo { hasNextPage endCursor }
}
` + fieldValueFragment

const itemDetailFieldValueFragments = `
fragment DetailFieldValuesPage on ProjectV2ItemFieldValueConnection {
  nodes { ...DetailFieldValue }
  pageInfo { hasNextPage endCursor }
}

fragment DetailFieldValue on ProjectV2ItemFieldValue {
  ...FieldValue
  ... on ProjectV2ItemFieldLabelValue {
    labels(first: 100) { nodes { name } pageInfo { hasNextPage endCursor } }
  }
  ... on ProjectV2ItemFieldMilestoneValue {
    milestone { title }
  }
  ... on ProjectV2ItemFieldPullRequestValue {
    pullRequests(first: 100) {
      nodes { number title url repository { nameWithOwner } }
      pageInfo { hasNextPage endCursor }
    }
  }
  ... on ProjectV2ItemFieldRepositoryValue {
    repository { name nameWithOwner }
  }
  ... on ProjectV2ItemFieldReviewerValue {
    reviewers(first: 100) {
      nodes {
        __typename
        ... on User { login }
        ... on Team { name }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
  ... on ProjectV2ItemFieldUserValue {
    users(first: 100) { nodes { login } pageInfo { hasNextPage endCursor } }
  }
}`

const itemsPageFragment = `
fragment ItemsPage on ProjectV2ItemConnection {
  nodes {
    id type
    content {
      __typename
      ... on Issue { id number title url repository { name } state subIssuesSummary { total completed } }
      ... on PullRequest { id number title url repository { name } state isDraft merged }
      ... on DraftIssue { id title }
    }
    fieldValues(first: 100) { ...FieldValuesPage }
  }
  pageInfo { hasNextPage endCursor }
}`

const userItemsQuery = `
query UserProjectItems($login: String!, $project: Int!, $filter: String, $after: String) {
  user(login: $login) {
    projectV2(number: $project) { items(first: 100, after: $after, query: $filter, orderBy: {field: POSITION, direction: ASC}) { ...ItemsPage } }
  }
}` + itemsPageFragment + itemFieldValueFragments

const organizationItemsQuery = `
query OrganizationProjectItems($login: String!, $project: Int!, $filter: String, $after: String) {
  organization(login: $login) {
    projectV2(number: $project) { items(first: 100, after: $after, query: $filter, orderBy: {field: POSITION, direction: ASC}) { ...ItemsPage } }
  }
}` + itemsPageFragment + itemFieldValueFragments

const itemFieldValuesQuery = `
query ProjectItemFieldValues($id: ID!, $after: String) {
  node(id: $id) {
    ... on ProjectV2Item { fieldValues(first: 100, after: $after) { ...FieldValuesPage } }
  }
}
` + itemFieldValueFragments

type Item struct {
	ID          string
	Type        string
	Content     *Content
	FieldValues []FieldValue
}

type Content struct {
	Kind          string
	ID            string
	Number        int
	Title         string
	URL           string
	Repository    string
	State         string
	IsDraft       bool
	Merged        bool
	SubIssueTotal int
	SubIssueDone  int
	Body          string
	BodyAvailable bool
}

type FieldValue struct {
	Kind        string
	FieldID     string
	FieldName   string
	OptionID    string
	Value       string
	IterationID string
	Available   bool
}

type ItemsPage struct {
	Items     []Item
	HasNext   bool
	EndCursor string
}

type itemsResponse struct {
	User         *itemOwner `json:"user"`
	Organization *itemOwner `json:"organization"`
}

type itemOwner struct {
	Project *struct {
		Items rawItemsPage `json:"items"`
	} `json:"projectV2"`
}

type rawItemsPage struct {
	Nodes    []*rawItem `json:"nodes"`
	PageInfo pageInfo   `json:"pageInfo"`
}

type rawItem struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Content     *rawContent       `json:"content"`
	FieldValues rawFieldValuePage `json:"fieldValues"`
}

type rawContent struct {
	Kind       string         `json:"__typename"`
	ID         string         `json:"id"`
	Number     int            `json:"number"`
	Title      string         `json:"title"`
	URL        string         `json:"url"`
	Repository *rawRepository `json:"repository"`
	State      string         `json:"state"`
	IsDraft    bool           `json:"isDraft"`
	Merged     bool           `json:"merged"`
	SubIssues  *rawSubIssues  `json:"subIssuesSummary"`
	Body       *string        `json:"body"`
}

type rawRepository struct {
	Name          string `json:"name"`
	NameWithOwner string `json:"nameWithOwner"`
}

type rawSubIssues struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
}

type rawFieldValuePage struct {
	Nodes    []*rawFieldValue `json:"nodes"`
	PageInfo pageInfo         `json:"pageInfo"`
}

type rawFieldValue struct {
	Kind            string                          `json:"__typename"`
	Field           rawFieldRef                     `json:"field"`
	OptionID        string                          `json:"optionId"`
	Name            string                          `json:"name"`
	Text            string                          `json:"text"`
	Date            string                          `json:"date"`
	Number          *float64                        `json:"number"`
	IterationID     string                          `json:"iterationId"`
	Title           string                          `json:"title"`
	Value           string                          `json:"value"`
	Options         []FieldOption                   `json:"options"`
	IssueFieldValue *rawIssueFieldValue             `json:"issueFieldValue"`
	Labels          *rawLabelConnection             `json:"labels"`
	Milestone       *rawMilestone                   `json:"milestone"`
	PullRequests    *rawPullRequestConnection       `json:"pullRequests"`
	Repository      *rawRepository                  `json:"repository"`
	Reviewers       *rawRequestedReviewerConnection `json:"reviewers"`
	Users           *rawUserConnection              `json:"users"`
}

type rawLabelConnection struct {
	Nodes    []rawLabel `json:"nodes"`
	PageInfo pageInfo   `json:"pageInfo"`
}

type rawLabel struct {
	Name string `json:"name"`
}

type rawMilestone struct {
	Title string `json:"title"`
}

type rawPullRequestConnection struct {
	Nodes    []rawPullRequest `json:"nodes"`
	PageInfo pageInfo         `json:"pageInfo"`
}

type rawPullRequest struct {
	Number     int            `json:"number"`
	Title      string         `json:"title"`
	URL        string         `json:"url"`
	Repository *rawRepository `json:"repository"`
}

type rawRequestedReviewerConnection struct {
	Nodes    []rawRequestedReviewer `json:"nodes"`
	PageInfo pageInfo               `json:"pageInfo"`
}

type rawRequestedReviewer struct {
	Kind  string `json:"__typename"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type rawUserConnection struct {
	Nodes    []rawUser `json:"nodes"`
	PageInfo pageInfo  `json:"pageInfo"`
}

type rawUser struct {
	Login string `json:"login"`
}

type rawIssueFieldValue struct {
	Kind     string        `json:"__typename"`
	OptionID string        `json:"optionId"`
	Name     string        `json:"name"`
	Text     string        `json:"text"`
	Date     string        `json:"date"`
	Number   *float64      `json:"number"`
	Options  []FieldOption `json:"options"`
}

type rawFieldRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type itemFieldValuesResponse struct {
	Node *struct {
		FieldValues rawFieldValuePage `json:"fieldValues"`
	} `json:"node"`
}

func (c *Client) PageItems(ctx context.Context, owner Owner, projectNumber int, filter, after string) (ItemsPage, error) {
	var response itemsResponse
	variables := map[string]interface{}{
		"login":   owner.Login,
		"project": projectNumber,
		"filter":  nullableCursor(filter),
		"after":   nullableCursor(after),
	}
	query := organizationItemsQuery
	if owner.Kind == UserOwner {
		query = userItemsQuery
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return ItemsPage{}, err
	}

	var page rawItemsPage
	var found bool
	switch owner.Kind {
	case UserOwner:
		if response.User != nil && response.User.Project != nil {
			page = response.User.Project.Items
			found = true
		}
	case OrganizationOwner:
		if response.Organization != nil && response.Organization.Project != nil {
			page = response.Organization.Project.Items
			found = true
		}
	default:
		return ItemsPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if !found {
		return ItemsPage{}, fmt.Errorf("project %d for %s %q was not found", projectNumber, owner.Kind, owner.Login)
	}
	if page.PageInfo.HasNextPage {
		if _, err := nextCursor(page.PageInfo, after); err != nil {
			return ItemsPage{}, err
		}
	}
	return c.projectItems(ctx, page)
}

func (c *Client) projectItems(ctx context.Context, page rawItemsPage) (ItemsPage, error) {
	items := make([]Item, 0, len(page.Nodes))
	for _, rawItem := range page.Nodes {
		if rawItem == nil {
			continue
		}
		fieldValues := rawItem.FieldValues
		previous := ""
		for fieldValues.PageInfo.HasNextPage {
			after, err := nextCursor(fieldValues.PageInfo, previous)
			if err != nil {
				return ItemsPage{}, fmt.Errorf("item %q field values: %w", rawItem.ID, err)
			}
			next, err := c.fetchItemFieldValues(ctx, rawItem.ID, after)
			if err != nil {
				return ItemsPage{}, err
			}
			fieldValues.Nodes = append(fieldValues.Nodes, next.Nodes...)
			fieldValues.PageInfo = next.PageInfo
			previous = after
		}

		projected := Item{ID: rawItem.ID, Type: rawItem.Type}
		projected.Content = projectContent(rawItem.Content)
		for _, value := range fieldValues.Nodes {
			if value != nil {
				projected.FieldValues = append(projected.FieldValues, value.project())
			}
		}
		items = append(items, projected)
	}
	endCursor := ""
	if page.PageInfo.EndCursor != nil {
		endCursor = *page.PageInfo.EndCursor
	}
	return ItemsPage{Items: items, HasNext: page.PageInfo.HasNextPage, EndCursor: endCursor}, nil
}

func projectContent(content *rawContent) *Content {
	if content == nil {
		return nil
	}
	projected := &Content{
		Kind:    content.Kind,
		ID:      content.ID,
		Number:  content.Number,
		Title:   content.Title,
		URL:     content.URL,
		State:   content.State,
		IsDraft: content.IsDraft,
		Merged:  content.Merged,
	}
	if content.Repository != nil {
		projected.Repository = content.Repository.Name
	}
	if content.SubIssues != nil {
		projected.SubIssueTotal = content.SubIssues.Total
		projected.SubIssueDone = content.SubIssues.Completed
	}
	if content.Body != nil {
		projected.Body = *content.Body
		projected.BodyAvailable = true
	}
	return projected
}

func (c *Client) fetchItemFieldValues(ctx context.Context, itemID, after string) (rawFieldValuePage, error) {
	var response itemFieldValuesResponse
	if err := c.graphql.DoWithContext(ctx, itemFieldValuesQuery, map[string]interface{}{"id": itemID, "after": after}, &response); err != nil {
		return rawFieldValuePage{}, err
	}
	if response.Node == nil {
		return rawFieldValuePage{}, fmt.Errorf("project item %q was not found while loading field values", itemID)
	}
	return response.Node.FieldValues, nil
}

func (value rawFieldValue) project() FieldValue {
	if value.Kind == "ProjectV2ItemIssueFieldValue" && value.IssueFieldValue != nil {
		return FieldValue{
			Kind:      value.IssueFieldValue.Kind,
			FieldID:   value.Field.ID,
			FieldName: value.Field.Name,
			OptionID:  value.IssueFieldValue.OptionID,
			Value:     issueFieldValueValue(*value.IssueFieldValue),
			Available: value.IssueFieldValue.hasValue(),
		}
	}
	projected := FieldValue{
		Kind:        value.Kind,
		FieldID:     value.Field.ID,
		FieldName:   value.Field.Name,
		OptionID:    value.OptionID,
		IterationID: value.IterationID,
	}
	switch value.Kind {
	case "ProjectV2ItemFieldLabelValue":
		if value.Labels != nil {
			projected.Value = joinNames(value.Labels.Nodes, func(label rawLabel) string { return label.Name }, value.Labels.PageInfo.HasNextPage)
			projected.Available = true
		}
	case "ProjectV2ItemFieldMilestoneValue":
		if value.Milestone != nil {
			projected.Value = value.Milestone.Title
			projected.Available = true
		}
	case "ProjectV2ItemFieldPullRequestValue":
		if value.PullRequests != nil {
			projected.Value = joinNames(value.PullRequests.Nodes, func(request rawPullRequest) string {
				label := ""
				if request.Repository != nil {
					label = request.Repository.NameWithOwner
					if label == "" {
						label = request.Repository.Name
					}
				}
				if request.Number > 0 {
					if label != "" {
						label += " "
					}
					label += fmt.Sprintf("#%d", request.Number)
				}
				if label == "" {
					label = request.Title
				}
				return label
			}, value.PullRequests.PageInfo.HasNextPage)
			projected.Available = true
		}
	case "ProjectV2ItemFieldRepositoryValue":
		if value.Repository != nil {
			projected.Value = value.Repository.NameWithOwner
			if projected.Value == "" {
				projected.Value = value.Repository.Name
			}
			projected.Available = true
		}
	case "ProjectV2ItemFieldReviewerValue":
		if value.Reviewers != nil {
			projected.Value = joinNames(value.Reviewers.Nodes, func(reviewer rawRequestedReviewer) string {
				if reviewer.Login != "" {
					return reviewer.Login
				}
				return reviewer.Name
			}, value.Reviewers.PageInfo.HasNextPage)
			projected.Available = true
		}
	case "ProjectV2ItemFieldUserValue":
		if value.Users != nil {
			projected.Value = joinNames(value.Users.Nodes, func(user rawUser) string { return user.Login }, value.Users.PageInfo.HasNextPage)
			projected.Available = true
		}
	default:
		projected.Value = valueValue(value)
		projected.Available = value.hasScalarValue()
	}
	return projected
}

func joinNames[T any](values []T, name func(T) string, truncated bool) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if valueName := name(value); valueName != "" {
			parts = append(parts, valueName)
		}
	}
	result := strings.Join(parts, ", ")
	if truncated {
		if result != "" {
			result += ", "
		}
		result += "..."
	}
	return result
}

func (value rawIssueFieldValue) hasValue() bool {
	switch value.Kind {
	case "IssueFieldDateValue", "IssueFieldMultiSelectValue", "IssueFieldNumberValue", "IssueFieldSingleSelectValue", "IssueFieldTextValue":
		return true
	default:
		return false
	}
}

func issueFieldValueValue(value rawIssueFieldValue) string {
	parts := make([]string, 0, len(value.Options)+4)
	if value.Text != "" {
		parts = append(parts, value.Text)
	}
	if value.Date != "" {
		parts = append(parts, value.Date)
	}
	if value.Number != nil {
		parts = append(parts, formatNumber(*value.Number))
	}
	if value.Name != "" {
		parts = append(parts, value.Name)
	}
	for _, option := range value.Options {
		parts = append(parts, option.Name)
	}
	return strings.Join(parts, ", ")
}

func (value rawFieldValue) hasScalarValue() bool {
	switch value.Kind {
	case "ProjectV2ItemFieldDateValue", "ProjectV2ItemFieldIterationValue", "ProjectV2ItemFieldMultiSelectValue", "ProjectV2ItemFieldNumberValue", "ProjectV2ItemFieldSingleSelectValue", "ProjectV2ItemFieldTextValue":
		return true
	}
	return false
}

func valueValue(value rawFieldValue) string {
	parts := make([]string, 0, 4)
	if value.Text != "" {
		parts = append(parts, value.Text)
	}
	if value.Date != "" {
		parts = append(parts, value.Date)
	}
	if value.Number != nil {
		parts = append(parts, formatNumber(*value.Number))
	}
	if value.Title != "" {
		parts = append(parts, value.Title)
	}
	if value.Value != "" {
		parts = append(parts, value.Value)
	}
	if value.Name != "" {
		parts = append(parts, value.Name)
	}
	for _, option := range value.Options {
		parts = append(parts, option.Name)
	}
	return strings.Join(parts, ", ")
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
