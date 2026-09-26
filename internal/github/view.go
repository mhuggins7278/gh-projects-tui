package github

import (
	"context"
	"fmt"
)

const fieldConfigurationFragment = `
fragment FieldConfiguration on ProjectV2FieldConfiguration {
  __typename
  ... on ProjectV2Field { id name dataType }
  ... on ProjectV2SingleSelectField {
    id name dataType options { id name }
    issueField { ... on IssueFieldSingleSelect { options { id name } } }
  }
  ... on ProjectV2MultiSelectField { id name dataType }
  ... on ProjectV2IterationField {
    id name dataType
    configuration {
      iterations { id title startDate duration }
      completedIterations { id title startDate duration }
    }
  }
}`

// Filter validation needs field names and types, not options or iterations.
const filterFieldFragment = `
fragment FilterFieldConfiguration on ProjectV2FieldConfiguration {
  __typename
  ... on ProjectV2Field { id name dataType }
  ... on ProjectV2SingleSelectField { id name dataType }
  ... on ProjectV2IterationField { id name dataType }
  ... on ProjectV2MultiSelectField { id name dataType }
}`

const viewFieldsFragment = `
fragment ViewFields on ProjectV2View {
  id number name layout filter
  configuration { visibleFields(first: 100, after: $fieldsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } } }
  groupByFields(first: 100, after: $groupsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
  verticalGroupByFields(first: 100, after: $verticalGroupsAfter) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
  sortByFields(first: 100, after: $sortsAfter) { nodes { direction field { ...FieldConfiguration } } pageInfo { hasNextPage endCursor } }
}` + fieldConfigurationFragment

const userViewQuery = `
query UserProjectView($login: String!, $project: Int!, $view: Int!, $fieldsAfter: String, $groupsAfter: String, $verticalGroupsAfter: String, $sortsAfter: String) {
  user(login: $login) {
    projectV2(number: $project) { id viewerCanUpdate projectFields: fields(first: 100) { nodes { ...FilterFieldConfiguration } pageInfo { hasNextPage endCursor } } view(number: $view) { ...ViewFields } }
  }
}` + viewFieldsFragment + filterFieldFragment

const organizationViewQuery = `
query OrganizationProjectView($login: String!, $project: Int!, $view: Int!, $fieldsAfter: String, $groupsAfter: String, $verticalGroupsAfter: String, $sortsAfter: String) {
  organization(login: $login) {
    projectV2(number: $project) { id viewerCanUpdate projectFields: fields(first: 100) { nodes { ...FilterFieldConfiguration } pageInfo { hasNextPage endCursor } } view(number: $view) { ...ViewFields } }
  }
}` + viewFieldsFragment + filterFieldFragment

const viewFieldsPageFragment = `
fragment ViewFieldsPage on ProjectV2View {
  configuration { visibleFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } } }
}` + fieldConfigurationFragment

const viewGroupsPageFragment = `
fragment ViewGroupsPage on ProjectV2View {
  groupByFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
}` + fieldConfigurationFragment

const viewVerticalGroupsPageFragment = `
fragment ViewVerticalGroupsPage on ProjectV2View {
  verticalGroupByFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }
}` + fieldConfigurationFragment

const viewSortsPageFragment = `
fragment ViewSortsPage on ProjectV2View {
  sortByFields(first: 100, after: $after) { nodes { direction field { ...FieldConfiguration } } pageInfo { hasNextPage endCursor } }
}` + fieldConfigurationFragment

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
	ProjectID       string
	ID              string
	Number          int
	Name            string
	Layout          ViewLayout
	Filter          string
	ViewerCanUpdate bool
	ProjectFields   []Field
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
	ProjectID       string          `json:"id"`
	ViewerCanUpdate bool            `json:"viewerCanUpdate"`
	ProjectFields   fieldConnection `json:"projectFields"`
	View            *rawView        `json:"view"`
}

type rawView struct {
	ID              string            `json:"id"`
	Number          int               `json:"number"`
	Name            string            `json:"name"`
	Layout          ViewLayout        `json:"layout"`
	Filter          string            `json:"filter"`
	Configuration   viewConfiguration `json:"configuration"`
	GroupByFields   fieldConnection   `json:"groupByFields"`
	VerticalGroupBy fieldConnection   `json:"verticalGroupByFields"`
	SortByFields    sortConnection    `json:"sortByFields"`
}

type viewConfiguration struct {
	VisibleFields fieldConnection `json:"visibleFields"`
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
	IssueField    *rawIssueField          `json:"issueField"`
	Configuration *iterationConfiguration `json:"configuration"`
}

type rawIssueField struct {
	Options []FieldOption `json:"options"`
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
	for project.ProjectFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(project.ProjectFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		page, fetchErr := c.fetchProjectFieldsPage(ctx, owner, projectNumber, nextAfter)
		if fetchErr != nil {
			return View{}, fetchErr
		}
		project.ProjectFields.Nodes = append(project.ProjectFields.Nodes, page.Nodes...)
		project.ProjectFields.PageInfo = page.PageInfo
		after = nextAfter
	}

	after = ""
	for raw.Configuration.VisibleFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.Configuration.VisibleFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		page, fetchErr := c.fetchViewFieldsPage(ctx, owner, projectNumber, viewNumber, nextAfter)
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.Configuration.VisibleFields.Nodes = append(raw.Configuration.VisibleFields.Nodes, page.Nodes...)
		raw.Configuration.VisibleFields.PageInfo = page.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.GroupByFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.GroupByFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		page, fetchErr := c.fetchViewGroupsPage(ctx, owner, projectNumber, viewNumber, nextAfter)
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.GroupByFields.Nodes = append(raw.GroupByFields.Nodes, page.Nodes...)
		raw.GroupByFields.PageInfo = page.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.VerticalGroupBy.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.VerticalGroupBy.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		page, fetchErr := c.fetchViewVerticalGroupsPage(ctx, owner, projectNumber, viewNumber, nextAfter)
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.VerticalGroupBy.Nodes = append(raw.VerticalGroupBy.Nodes, page.Nodes...)
		raw.VerticalGroupBy.PageInfo = page.PageInfo
		after = nextAfter
	}
	after = ""
	for raw.SortByFields.PageInfo.HasNextPage {
		nextAfter, cursorErr := nextCursor(raw.SortByFields.PageInfo, after)
		if cursorErr != nil {
			return View{}, cursorErr
		}
		page, fetchErr := c.fetchViewSortsPage(ctx, owner, projectNumber, viewNumber, nextAfter)
		if fetchErr != nil {
			return View{}, fetchErr
		}
		raw.SortByFields.Nodes = append(raw.SortByFields.Nodes, page.Nodes...)
		raw.SortByFields.PageInfo = page.PageInfo
		after = nextAfter
	}

	return View{
		ProjectID:       project.ProjectID,
		ID:              raw.ID,
		Number:          raw.Number,
		Name:            raw.Name,
		Layout:          raw.Layout,
		Filter:          raw.Filter,
		ViewerCanUpdate: project.ViewerCanUpdate,
		ProjectFields:   project.ProjectFields.project(),
		Fields:          raw.Configuration.VisibleFields.project(),
		GroupByFields:   raw.GroupByFields.project(),
		VerticalGroupBy: raw.VerticalGroupBy.project(),
		SortByFields:    raw.SortByFields.sorts(),
	}, nil
}

func (c *Client) fetchProjectFieldsPage(ctx context.Context, owner Owner, projectNumber int, after string) (fieldConnection, error) {
	branch := "organization"
	if owner.Kind == UserOwner {
		branch = "user"
	} else if owner.Kind != OrganizationOwner {
		return fieldConnection{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	query := `query ProjectFieldsPage($login: String!, $project: Int!, $after: String) { ` + branch + `(login: $login) { projectV2(number: $project) { fields(first: 100, after: $after) { nodes { ...FilterFieldConfiguration } pageInfo { hasNextPage endCursor } } } } }` + filterFieldFragment
	var response struct {
		User *struct {
			Project *struct {
				Fields fieldConnection `json:"fields"`
			} `json:"projectV2"`
		} `json:"user"`
		Organization *struct {
			Project *struct {
				Fields fieldConnection `json:"fields"`
			} `json:"projectV2"`
		} `json:"organization"`
	}
	if err := c.graphql.DoWithContext(ctx, query, map[string]interface{}{"login": owner.Login, "project": projectNumber, "after": after}, &response); err != nil {
		return fieldConnection{}, err
	}
	selected := response.Organization
	if owner.Kind == UserOwner {
		selected = response.User
	}
	if selected == nil || selected.Project == nil {
		return fieldConnection{}, fmt.Errorf("project fields for %d in %q were not returned", projectNumber, owner.Login)
	}
	return selected.Project.Fields, nil
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

type viewFieldsPageResponse struct {
	User         *viewFieldsPageOwner `json:"user"`
	Organization *viewFieldsPageOwner `json:"organization"`
}

type viewFieldsPageOwner struct {
	Project *viewFieldsPageProject `json:"projectV2"`
}

type viewFieldsPageProject struct {
	View *viewFieldsPageView `json:"view"`
}

type viewFieldsPageView struct {
	Configuration         *viewFieldsPageConfig `json:"configuration"`
	GroupByFields         *fieldConnection      `json:"groupByFields"`
	VerticalGroupByFields *fieldConnection      `json:"verticalGroupByFields"`
	SortByFields          *sortConnection       `json:"sortByFields"`
}

type viewFieldsPageConfig struct {
	VisibleFields *fieldConnection `json:"visibleFields"`
}

func (c *Client) fetchViewPage(ctx context.Context, owner Owner, projectNumber, viewNumber int, selection, after string) (viewFieldsPageView, error) {
	branch := "organization"
	if owner.Kind == UserOwner {
		branch = "user"
	} else if owner.Kind != OrganizationOwner {
		return viewFieldsPageView{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	query := `query ViewConnectionPage($login: String!, $project: Int!, $view: Int!, $after: String) { ` + branch + `(login: $login) { projectV2(number: $project) { view(number: $view) { ` + selection + ` } } } }` + fieldConfigurationFragment
	var response viewFieldsPageResponse
	variables := map[string]interface{}{
		"login":   owner.Login,
		"project": projectNumber,
		"view":    viewNumber,
		"after":   nullableCursor(after),
	}
	if err := c.graphql.DoWithContext(ctx, query, variables, &response); err != nil {
		return viewFieldsPageView{}, err
	}
	ownerResp := response.Organization
	if owner.Kind == UserOwner {
		ownerResp = response.User
	}
	if ownerResp == nil || ownerResp.Project == nil || ownerResp.Project.View == nil {
		return viewFieldsPageView{}, fmt.Errorf("view %d in project %d for %s %q was not found", viewNumber, projectNumber, owner.Kind, owner.Login)
	}
	return *ownerResp.Project.View, nil
}

func (c *Client) fetchViewFieldsPage(ctx context.Context, owner Owner, projectNumber, viewNumber int, after string) (fieldConnection, error) {
	view, err := c.fetchViewPage(ctx, owner, projectNumber, viewNumber, `configuration { visibleFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } } }`, after)
	if err != nil {
		return fieldConnection{}, err
	}
	if view.Configuration == nil || view.Configuration.VisibleFields == nil {
		return fieldConnection{}, fmt.Errorf("visible fields were not returned")
	}
	return *view.Configuration.VisibleFields, nil
}

func (c *Client) fetchViewGroupsPage(ctx context.Context, owner Owner, projectNumber, viewNumber int, after string) (fieldConnection, error) {
	view, err := c.fetchViewPage(ctx, owner, projectNumber, viewNumber, `groupByFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }`, after)
	if err != nil {
		return fieldConnection{}, err
	}
	if view.GroupByFields == nil {
		return fieldConnection{}, fmt.Errorf("group-by fields were not returned")
	}
	return *view.GroupByFields, nil
}

func (c *Client) fetchViewVerticalGroupsPage(ctx context.Context, owner Owner, projectNumber, viewNumber int, after string) (fieldConnection, error) {
	view, err := c.fetchViewPage(ctx, owner, projectNumber, viewNumber, `verticalGroupByFields(first: 100, after: $after) { nodes { ...FieldConfiguration } pageInfo { hasNextPage endCursor } }`, after)
	if err != nil {
		return fieldConnection{}, err
	}
	if view.VerticalGroupByFields == nil {
		return fieldConnection{}, fmt.Errorf("vertical group-by fields were not returned")
	}
	return *view.VerticalGroupByFields, nil
}

func (c *Client) fetchViewSortsPage(ctx context.Context, owner Owner, projectNumber, viewNumber int, after string) (sortConnection, error) {
	view, err := c.fetchViewPage(ctx, owner, projectNumber, viewNumber, `sortByFields(first: 100, after: $after) { nodes { direction field { ...FieldConfiguration } } pageInfo { hasNextPage endCursor } }`, after)
	if err != nil {
		return sortConnection{}, err
	}
	if view.SortByFields == nil {
		return sortConnection{}, fmt.Errorf("sort fields were not returned")
	}
	return *view.SortByFields, nil
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
	options := f.Options
	if len(options) == 0 && f.IssueField != nil {
		options = f.IssueField.Options
	}
	result := Field{ID: f.ID, Name: f.Name, DataType: f.DataType, Kind: f.Kind, Options: options}
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
