package github

import (
	"context"
	"fmt"
)

const fieldConfigurationFragment = `
fragment FieldConfiguration on ProjectV2FieldConfiguration {
  __typename
  ... on ProjectV2Field { id name dataType }
  ... on ProjectV2SingleSelectField { id name dataType options { id name } }
  ... on ProjectV2MultiSelectField { id name dataType }
  ... on ProjectV2IterationField {
    id name dataType
    configuration {
      iterations { id title startDate duration }
      completedIterations { id title startDate duration }
    }
  }
}`

const viewFieldsFragment = `
fragment ViewFields on ProjectV2View {
  id number name layout filter
  fields(first: 100, after: $fieldsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
  groupByFields(first: 100, after: $groupsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
  verticalGroupByFields(first: 100, after: $verticalGroupsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
  sortByFields(first: 100, after: $sortsAfter) { nodes { direction field { ...FieldConfiguration } } pageInfo { hasNextPage endCursor } }
}` + fieldConfigurationFragment

const userViewQuery = `
query UserProjectView($login: String!, $project: Int!, $view: Int!, $fieldsAfter: String, $groupsAfter: String, $verticalGroupsAfter: String, $sortsAfter: String) {
  user(login: $login) {
    projectV2(number: $project) { viewerCanUpdate view(number: $view) { ...ViewFields } }
  }
}` + viewFieldsFragment

const organizationViewQuery = `
query OrganizationProjectView($login: String!, $project: Int!, $view: Int!, $fieldsAfter: String, $groupsAfter: String, $verticalGroupsAfter: String, $sortsAfter: String) {
  organization(login: $login) {
    projectV2(number: $project) { viewerCanUpdate view(number: $view) { ...ViewFields } }
  }
}` + viewFieldsFragment

const userViewsQuery = `
query UserProjectViews($login: String!, $project: Int!, $after: String) {
  user(login: $login) {
    projectV2(number: $project) {
      views(first: 100, after: $after, orderBy: {field: POSITION, direction: ASC}) {
        nodes { number name layout }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

const organizationViewsQuery = `
query OrganizationProjectViews($login: String!, $project: Int!, $after: String) {
  organization(login: $login) {
    projectV2(number: $project) {
      views(first: 100, after: $after, orderBy: {field: POSITION, direction: ASC}) {
        nodes { number name layout }
        pageInfo { hasNextPage endCursor }
      }
    }
  }
}`

type ViewLayout string

const (
	BoardLayout   ViewLayout = "BOARD_LAYOUT"
	TableLayout   ViewLayout = "TABLE_LAYOUT"
	RoadmapLayout ViewLayout = "ROADMAP_LAYOUT"
)

type View struct {
	ID              string
	Number          int
	Name            string
	Layout          ViewLayout
	Filter          string
	ViewerCanUpdate bool
	Fields          []Field
	GroupByFields   []Field
	VerticalGroupBy []Field
	SortByFields    []SortField
}

type Field struct {
	ID         string
	Name       string
	DataType   string
	Kind       string
	Options    []FieldOption
	Iterations []Iteration
}

type FieldOption struct {
	ID   string
	Name string
}

type Iteration struct {
	ID        string
	Title     string
	StartDate string
	Duration  int
	Completed bool
}

type SortField struct {
	Direction string
	Field     Field
}

type viewResponse struct {
	User         *viewOwner `json:"user"`
	Organization *viewOwner `json:"organization"`
}

type viewOwner struct {
	Project *viewProject `json:"projectV2"`
}

type viewProject struct {
	ViewerCanUpdate bool     `json:"viewerCanUpdate"`
	View            *rawView `json:"view"`
}

type rawView struct {
	ID              string          `json:"id"`
	Number          int             `json:"number"`
	Name            string          `json:"name"`
	Layout          ViewLayout      `json:"layout"`
	Filter          string          `json:"filter"`
	Fields          fieldConnection `json:"fields"`
	GroupByFields   fieldConnection `json:"groupByFields"`
	VerticalGroupBy fieldConnection `json:"verticalGroupByFields"`
	SortByFields    sortConnection  `json:"sortByFields"`
}

type fieldConnection struct {
	Nodes    []*rawField `json:"nodes"`
	PageInfo pageInfo    `json:"pageInfo"`
}

type sortConnection struct {
	Nodes    []*rawSortField `json:"nodes"`
	PageInfo pageInfo        `json:"pageInfo"`
}

type rawField struct {
	Kind          string                  `json:"__typename"`
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	DataType      string                  `json:"dataType"`
	Options       []FieldOption           `json:"options"`
	Configuration *iterationConfiguration `json:"configuration"`
}

type iterationConfiguration struct {
	Iterations          []Iteration `json:"iterations"`
	CompletedIterations []Iteration `json:"completedIterations"`
}

type rawSortField struct {
	Direction string    `json:"direction"`
	Field     *rawField `json:"field"`
}

type viewCursors struct {
	Fields         string
	Groups         string
	VerticalGroups string
	Sorts          string
}

func (c *Client) OpenView(ctx context.Context, owner Owner, projectNumber, viewNumber int) (View, error) {
	project, err := c.fetchView(ctx, owner, projectNumber, viewNumber, viewCursors{})
	if err != nil {
		return View{}, err
	}
	raw := project.View

	after := ""
	for raw.Fields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.Fields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		next, fetchErr := c.fetchView(ctx, owner, projectNumber, viewNumber, viewCursors{Fields: nextAfter})
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.Fields.Nodes = append(raw.Fields.Nodes, next.View.Fields.Nodes...)
		raw.Fields.PageInfo = next.View.Fields.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.GroupByFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.GroupByFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		next, fetchErr := c.fetchView(ctx, owner, projectNumber, viewNumber, viewCursors{Groups: nextAfter})
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.GroupByFields.Nodes = append(raw.GroupByFields.Nodes, next.View.GroupByFields.Nodes...)
		raw.GroupByFields.PageInfo = next.View.GroupByFields.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.VerticalGroupBy.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.VerticalGroupBy.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		next, fetchErr := c.fetchView(ctx, owner, projectNumber, viewNumber, viewCursors{VerticalGroups: nextAfter})
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.VerticalGroupBy.Nodes = append(raw.VerticalGroupBy.Nodes, next.View.VerticalGroupBy.Nodes...)
		raw.VerticalGroupBy.PageInfo = next.View.VerticalGroupBy.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.SortByFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.SortByFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		next, fetchErr := c.fetchView(ctx, owner, projectNumber, viewNumber, viewCursors{Sorts: nextAfter})
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.SortByFields.Nodes = append(raw.SortByFields.Nodes, next.View.SortByFields.Nodes...)
		raw.SortByFields.PageInfo = next.View.SortByFields.PageInfo
		after = nextAfter
	}

	return View{
		ID:              raw.ID,
		Number:          raw.Number,
		Name:            raw.Name,
		Layout:          raw.Layout,
		Filter:          raw.Filter,
		ViewerCanUpdate: project.ViewerCanUpdate,
		Fields:          raw.Fields.project(),
		GroupByFields:   raw.GroupByFields.project(),
		VerticalGroupBy: raw.VerticalGroupBy.project(),
		SortByFields:    raw.SortByFields.sorts(),
	}, nil
}

func (c *Client) fetchView(ctx context.Context, owner Owner, projectNumber, viewNumber int, cursors viewCursors) (*viewProject, error) {
	var response viewResponse
	variables := map[string]interface{}{
		"login":               owner.Login,
		"project":             projectNumber,
		"view":                viewNumber,
		"fieldsAfter":         nullableCursor(cursors.Fields),
		"groupsAfter":         nullableCursor(cursors.Groups),
		"verticalGroupsAfter": nullableCursor(cursors.VerticalGroups),
		"sortsAfter":          nullableCursor(cursors.Sorts),
	}
	query := organizationViewQuery
	if owner.Kind == UserOwner {
		query = userViewQuery
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return nil, err
	}

	var project *viewProject
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
		return nil, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if project == nil {
		return nil, fmt.Errorf("project %d for %s %q was not found", projectNumber, owner.Kind, owner.Login)
	}
	if project.View == nil {
		return nil, fmt.Errorf("view %d in project %d for %s %q was not found", viewNumber, projectNumber, owner.Kind, owner.Login)
	}
	return project, nil
}

func (c fieldConnection) project() []Field {
	fields := make([]Field, 0, len(c.Nodes))
	for _, field := range c.Nodes {
		if field != nil {
			fields = append(fields, field.project())
		}
	}
	return fields
}

func (c sortConnection) sorts() []SortField {
	sorts := make([]SortField, 0, len(c.Nodes))
	for _, field := range c.Nodes {
		if field != nil && field.Field != nil {
			sorts = append(sorts, SortField{Direction: field.Direction, Field: field.Field.project()})
		}
	}
	return sorts
}

func (f rawField) project() Field {
	result := Field{ID: f.ID, Name: f.Name, DataType: f.DataType, Kind: f.Kind, Options: f.Options}
	if f.Configuration != nil {
		result.Iterations = append(result.Iterations, f.Configuration.Iterations...)
		for _, iteration := range f.Configuration.CompletedIterations {
			iteration.Completed = true
			result.Iterations = append(result.Iterations, iteration)
		}
	}
	return result
}

// ViewSummary is the lightweight row used by the view picker.
type ViewSummary struct {
	Number int
	Name   string
	Layout ViewLayout
}

type viewsResponse struct {
	User         *viewsOwner `json:"user"`
	Organization *viewsOwner `json:"organization"`
}

type viewsOwner struct {
	Project *viewsProject `json:"projectV2"`
}

type viewsProject struct {
	Views rawViewsPage `json:"views"`
}

type rawViewsPage struct {
	Nodes    []*ViewSummary `json:"nodes"`
	PageInfo pageInfo       `json:"pageInfo"`
}

// ListViews paginates all saved views for a project.
func (c *Client) ListViews(ctx context.Context, owner Owner, projectNumber int) ([]ViewSummary, error) {
	var out []ViewSummary
	after := ""
	for {
		page, err := c.fetchViewsPage(ctx, owner, projectNumber, after)
		if err != nil {
			return nil, err
		}
		for _, node := range page.Nodes {
			if node != nil {
				out = append(out, *node)
			}
		}
		if !page.PageInfo.HasNextPage {
			return out, nil
		}
		after, err = nextCursor(page.PageInfo, after)
		if err != nil {
			return nil, err
		}
	}
}

func (c *Client) fetchViewsPage(ctx context.Context, owner Owner, projectNumber int, after string) (rawViewsPage, error) {
	var response viewsResponse
	variables := map[string]interface{}{
		"login":   owner.Login,
		"project": projectNumber,
		"after":   nullableCursor(after),
	}
	query := organizationViewsQuery
	if owner.Kind == UserOwner {
		query = userViewsQuery
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return rawViewsPage{}, err
	}
	var project *viewsProject
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
		return rawViewsPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if project == nil {
		return rawViewsPage{}, fmt.Errorf("project %d for %s %q was not found", projectNumber, owner.Kind, owner.Login)
	}
	return project.Views, nil
}

func nullableCursor(cursor string) interface{} {
	if cursor == "" {
		return nil
	}
	return cursor
}
