package github

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// These helpers deliberately stay test-only. REST projection is limited to the
// sampled board's field types, not a proposed production adapter.
type boardProbeTransport struct {
	base  http.RoundTripper
	bytes atomic.Int64
}

func (p *boardProbeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := p.base.RoundTrip(req)
	if err == nil {
		response.Body = &boardProbeBody{ReadCloser: response.Body, bytes: &p.bytes}
	}
	return response, err
}

type boardProbeBody struct {
	io.ReadCloser
	bytes *atomic.Int64
}

type boardProbeGraphQL struct {
	base            graphQLClient
	omitPermissions bool
	omitProgress    bool
	pageSize        int
	backwards       bool
	count           *int
}

func (p boardProbeGraphQL) DoWithContext(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
	if p.pageSize > 0 || p.backwards {
		size := p.pageSize
		if size == 0 {
			size = 100
		}
		arguments := fmt.Sprintf("items(first: %d, after: $after", size)
		if p.backwards {
			arguments = fmt.Sprintf("items(last: %d, before: $after", size)
			// Change only the outer items pageInfo, not nested card metadata.
			query = strings.Replace(query, " } pageInfo { hasNextPage endCursor } } } } }",
				" } pageInfo { hasNextPage: hasPreviousPage endCursor: startCursor } } } } }", 1)
		}
		query = strings.Replace(query, "items(first: 100, after: $after", arguments, 1)
	}
	if p.omitPermissions {
		query = strings.ReplaceAll(query, "viewerCanSetFields viewerCanClose viewerCanReopen", "")
	} else {
		// Preserve the pre-optimization read as a reproducible comparison
		// after production selected-item reads stop requesting these flags.
		query = strings.ReplaceAll(query, "state subIssuesSummary", "state viewerCanSetFields viewerCanClose viewerCanReopen subIssuesSummary")
	}
	if p.omitProgress {
		query = strings.ReplaceAll(query, "subIssuesSummary { total completed }", "")
	}
	if p.count != nil {
		query = strings.Replace(query, ") { nodes {", ") { totalCount nodes {", 1)
		return boardProbeReadCount(ctx, p.base, query, variables, response, p.count)
	}
	return p.base.DoWithContext(ctx, query, variables, response)
}

func boardProbeContentItems(ctx context.Context, client *Client, owner Owner, project int, filter, after string, fields []Field, omitPermissions, omitProgress bool) (ItemsPage, error) {
	base := client.graphql
	client.graphql = boardProbeGraphQL{base: base, omitPermissions: omitPermissions, omitProgress: omitProgress}
	defer func() { client.graphql = base }()
	return client.PageBoardItems(ctx, owner, project, filter, after, fields)
}

func (b *boardProbeBody) Read(data []byte) (int, error) {
	n, err := b.ReadCloser.Read(data)
	b.bytes.Add(int64(n))
	return n, err
}

func boardProbeREST(ctx context.Context, client *Client, path string, target interface{}) (string, error) {
	reader, ok := client.rest.(interface {
		RequestWithContext(context.Context, string, string, io.Reader) (*http.Response, error)
	})
	if !ok {
		return "", fmt.Errorf("REST probe reads are unavailable")
	}
	response, err := reader.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return "", err
	}
	for _, link := range strings.Split(response.Header.Get("Link"), ",") {
		address, attributes, found := strings.Cut(strings.TrimSpace(link), ";")
		if !found || !strings.Contains(attributes, `rel="next"`) {
			continue
		}
		next, err := url.Parse(strings.Trim(address, "<>"))
		if err != nil || next.Query().Get("after") == "" {
			return "", fmt.Errorf("unsupported REST probe pagination link")
		}
		// Like the production comparison helper, never follow a Link-provided host.
		return next.Query().Get("after"), nil
	}
	return "", nil
}

func boardProbeProjectPath(owner Owner, project int) (string, error) {
	root := "orgs"
	switch owner.Kind {
	case UserOwner:
		root = "users"
	case OrganizationOwner:
	default:
		return "", fmt.Errorf("unsupported probe owner kind")
	}
	return fmt.Sprintf("%s/%s/projectsV2/%d", root, url.PathEscape(owner.Login), project), nil
}

func boardProbeRESTFields(ctx context.Context, client *Client, root string, fields []Field) (string, map[int64]Field, error) {
	definitions := map[string]int64{}
	after, seen := "", map[string]bool{}
	for {
		var page []struct {
			ID     int64  `json:"id"`
			NodeID string `json:"node_id"`
		}
		next, err := boardProbeREST(ctx, client, root+"/fields?"+url.Values{"per_page": {"100"}, "after": {after}}.Encode(), &page)
		if err != nil {
			return "", nil, err
		}
		for _, field := range page {
			definitions[field.NodeID] = field.ID
		}
		if next == "" {
			break
		}
		if seen[next] || next == after {
			return "", nil, fmt.Errorf("REST field cursor did not advance")
		}
		seen[next], after = true, next
	}
	ids, selected := []string{}, map[int64]Field{}
	for _, field := range fields {
		switch strings.ToUpper(field.DataType) {
		case "TITLE", "SUB_ISSUES_PROGRESS", "ASSIGNEES", "SINGLE_SELECT", "LINKED_PULL_REQUESTS", "PULL_REQUEST":
		default:
			return "", nil, fmt.Errorf("REST probe does not project this field type")
		}
		id, ok := definitions[field.ID]
		if !ok {
			return "", nil, fmt.Errorf("selected GraphQL field has no REST identity")
		}
		if _, exists := selected[id]; !exists {
			ids = append(ids, strconv.FormatInt(id, 10))
			selected[id] = field
		}
	}
	if len(ids) == 0 || len(ids) > 50 {
		return "", nil, fmt.Errorf("REST probe requires 1-50 selected fields")
	}
	return strings.Join(ids, ","), selected, nil
}

type boardProbeRESTContent struct {
	NodeID        string `json:"node_id"`
	Number        int
	Title         string
	HTMLURL       string `json:"html_url"`
	RepositoryURL string `json:"repository_url"`
	State         string
	Draft         bool
	MergedAt      *string `json:"merged_at"`
	Repository    *struct {
		Name     string
		FullName string `json:"full_name"`
	}
	Base struct {
		Repo *struct {
			FullName string `json:"full_name"`
		}
	}
	SubIssues *rawSubIssues `json:"sub_issues_summary"`
}

func (c boardProbeRESTContent) repository() *rawRepository {
	fullName := ""
	if c.Repository != nil {
		fullName = c.Repository.FullName
	} else if c.Base.Repo != nil {
		fullName = c.Base.Repo.FullName
	} else if parsed, err := url.Parse(c.RepositoryURL); err == nil {
		_, fullName, _ = strings.Cut(parsed.Path, "/repos/")
	}
	if fullName == "" {
		return nil
	}
	_, name, _ := strings.Cut(fullName, "/")
	return &rawRepository{Name: name, NameWithOwner: fullName}
}

func boardProbeRESTItems(ctx context.Context, client *Client, root string, view int, after, ids string, fields map[int64]Field) (ItemsPage, error) {
	var nodes []struct {
		NodeID      string                 `json:"node_id"`
		ContentType string                 `json:"content_type"`
		Content     *boardProbeRESTContent `json:"content"`
		Fields      []struct {
			ID    int64
			Value json.RawMessage
		}
	}
	values := url.Values{"per_page": {"100"}, "fields": {ids}}
	if after != "" {
		values.Set("after", after)
	}
	next, err := boardProbeREST(ctx, client, fmt.Sprintf("%s/views/%d/items?%s", root, view, values.Encode()), &nodes)
	if err != nil {
		return ItemsPage{}, err
	}
	page := ItemsPage{HasNext: next != "", EndCursor: next}
	for _, node := range nodes {
		item := Item{ID: node.NodeID}
		if c := node.Content; c != nil {
			state := strings.ToUpper(c.State)
			if node.ContentType == "PullRequest" && c.MergedAt != nil {
				state = "MERGED"
			}
			item.Content = projectContent(&rawContent{
				Kind: node.ContentType, ID: c.NodeID, Number: c.Number,
				Title: c.Title, URL: c.HTMLURL, Repository: c.repository(),
				State: state, IsDraft: c.Draft,
				Merged: c.MergedAt != nil, SubIssues: c.SubIssues,
			})
		}
		for _, cell := range node.Fields {
			field, ok := fields[cell.ID]
			if !ok {
				return ItemsPage{}, fmt.Errorf("REST returned an unrequested field")
			}
			if len(cell.Value) == 0 || string(cell.Value) == "null" {
				continue
			}
			value, err := boardProbeRESTValue(field, cell.Value)
			if err != nil {
				return ItemsPage{}, err
			}
			if value != nil {
				item.FieldValues = append(item.FieldValues, *value)
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func boardProbeRESTValue(field Field, data json.RawMessage) (*FieldValue, error) {
	value := rawFieldValue{Field: rawFieldRef{ID: field.ID, Name: field.Name}}
	switch strings.ToUpper(field.DataType) {
	case "TITLE", "SUB_ISSUES_PROGRESS":
		// Both are compared through Content, just as the board renders them.
		return nil, nil
	case "SINGLE_SELECT":
		var option struct {
			ID   string
			Name struct{ Raw string }
		}
		if err := json.Unmarshal(data, &option); err != nil {
			return nil, err
		}
		value.Kind, value.OptionID, value.Name = "ProjectV2ItemFieldSingleSelectValue", option.ID, option.Name.Raw
	case "ASSIGNEES":
		var users []rawUser
		if err := json.Unmarshal(data, &users); err != nil {
			return nil, err
		}
		if len(users) == 0 {
			return nil, nil // GraphQL omits unset multi-valued project cells.
		}
		value.Kind = "ProjectV2ItemFieldUserValue"
		value.Users = &rawUserConnection{Nodes: users[:min(len(users), 10)], PageInfo: pageInfo{HasNextPage: len(users) > 10}}
	case "LINKED_PULL_REQUESTS", "PULL_REQUEST":
		var requests []boardProbeRESTContent
		if err := json.Unmarshal(data, &requests); err != nil {
			return nil, err
		}
		if len(requests) == 0 {
			return nil, nil
		}
		value.Kind = "ProjectV2ItemFieldPullRequestValue"
		value.PullRequests = &rawPullRequestConnection{PageInfo: pageInfo{HasNextPage: len(requests) > 10}}
		for _, request := range requests[:min(len(requests), 10)] {
			value.PullRequests.Nodes = append(value.PullRequests.Nodes, rawPullRequest{
				Number: request.Number, Title: request.Title, URL: request.HTMLURL, Repository: request.repository(),
			})
		}
	default:
		return nil, fmt.Errorf("unsupported REST probe value")
	}
	projected := value.project()
	return &projected, nil
}

type boardProbeProjection struct {
	Content *Content
	Values  map[string]FieldValue
}

// Compare card data, excluding action permissions (which REST does not supply),
// transport-specific union names, and fields not requested by this candidate.
func boardProbeProject(item Item, fields []Field) boardProbeProjection {
	var content *Content
	if item.Content != nil {
		c := *item.Content
		c.ViewerCanSetFields, c.ViewerCanClose, c.ViewerCanReopen = nil, nil, nil
		c.Author, c.Body, c.BodyAvailable = "", "", false
		content = &c
	}
	values := map[string]FieldValue{}
	for _, field := range fields {
		if strings.EqualFold(field.DataType, "TITLE") && item.Content != nil ||
			strings.EqualFold(field.DataType, "SUB_ISSUES_PROGRESS") {
			continue
		}
		for _, value := range item.FieldValues {
			if value.FieldID == field.ID || field.ID == "" && value.FieldName == field.Name {
				value.Kind = ""
				values[field.ID] = value
				break
			}
		}
	}
	return boardProbeProjection{content, values}
}

func boardProbeCard(item Item, fields []Field) [32]byte {
	data, _ := json.Marshal(boardProbeProject(item, fields))
	return sha256.Sum256(data)
}

func boardProbeDifferences(left, right Item, fields []Field) map[string]int {
	a, b := boardProbeProject(left, fields), boardProbeProject(right, fields)
	differences := map[string]int{}
	if a.Content == nil || b.Content == nil {
		if a.Content != b.Content {
			differences["content/presence"]++
		}
	} else {
		x, y := reflect.ValueOf(*a.Content), reflect.ValueOf(*b.Content)
		for i := 0; i < x.NumField(); i++ {
			if !reflect.DeepEqual(x.Field(i).Interface(), y.Field(i).Interface()) {
				differences["content/"+x.Type().Field(i).Name]++
			}
		}
	}
	seen := map[string]bool{}
	for _, field := range fields {
		if seen[field.ID] {
			continue
		}
		seen[field.ID] = true
		x, xOK := a.Values[field.ID]
		y, yOK := b.Values[field.ID]
		if xOK != yOK {
			differences["cell/"+field.DataType+"/presence"]++
		} else if !reflect.DeepEqual(x, y) {
			xValue, yValue := reflect.ValueOf(x), reflect.ValueOf(y)
			for i := 0; i < xValue.NumField(); i++ {
				if !reflect.DeepEqual(xValue.Field(i).Interface(), yValue.Field(i).Interface()) {
					differences["cell/"+field.DataType+"/"+xValue.Type().Field(i).Name]++
				}
			}
		}
	}
	return differences
}

func boardProbePermissions(items []Item) int {
	known := 0
	for _, item := range items {
		if c := item.Content; c != nil {
			for _, permission := range []*bool{c.ViewerCanSetFields, c.ViewerCanClose, c.ViewerCanReopen} {
				if permission != nil {
					known++
				}
			}
		}
	}
	return known
}

func TestBoardProbeRESTProjectsSampledCardValues(t *testing.T) {
	fields := []Field{
		{ID: "status", Name: "Status", DataType: "SINGLE_SELECT"},
		{ID: "users", Name: "Assignees", DataType: "ASSIGNEES"},
		{ID: "pulls", Name: "Linked pull requests", DataType: "LINKED_PULL_REQUESTS"},
	}
	inputs := []string{
		`{"id":"option","name":{"raw":"Todo"}}`,
		`[{"login":"someone"}]`,
		`[{"number":7,"base":{"repo":{"full_name":"example/repo"}}}]`,
	}
	want := []FieldValue{
		{FieldID: "status", FieldName: "Status", OptionID: "option", Value: "Todo", Available: true},
		{FieldID: "users", FieldName: "Assignees", Value: "someone", Available: true},
		{FieldID: "pulls", FieldName: "Linked pull requests", Value: "example/repo #7", Available: true},
	}
	item := Item{ID: "item"}
	for i, field := range fields {
		value, err := boardProbeRESTValue(field, json.RawMessage(inputs[i]))
		if err != nil || value == nil {
			t.Fatalf("field %d projection failed", i)
		}
		item.FieldValues = append(item.FieldValues, *value)
	}
	baseline := Item{ID: "item", FieldValues: want}
	if boardProbeCard(item, fields) != boardProbeCard(baseline, fields) {
		t.Fatal("REST card values differ from GraphQL projection")
	}
	baseline.FieldValues = slices.Clone(want)
	baseline.FieldValues[0].OptionID = "different"
	if boardProbeCard(item, fields) == boardProbeCard(baseline, fields) {
		t.Fatal("comparison missed a grouping identity difference")
	}
}

func TestBoardProbeRESTPreservesContentAndUnavailableItems(t *testing.T) {
	rest := &membershipREST{
		body: `[
			{"node_id":"item","content_type":"Issue","content":{
				"node_id":"issue","number":7,"title":"Synthetic title",
				"html_url":"https://github.com/example/repo/issues/7",
				"repository":{"name":"repo","full_name":"example/repo"},
				"state":"open","sub_issues_summary":{"total":2,"completed":1}
			},"fields":[{"id":1,"value":[]},{"id":2,"value":null}]},
			{"node_id":"unavailable","content_type":"Issue","content":null,"fields":[]},
			{"node_id":"merged","content_type":"PullRequest","content":{
				"node_id":"pr","title":"Synthetic PR","state":"closed","merged_at":"2026-11-20T12:00:00Z",
				"base":{"repo":{"full_name":"example/repo"}}
			},"fields":[]}
		]`,
		link: `<https://unexpected.invalid/items?after=next>; rel="next"`,
	}
	fields := map[int64]Field{
		1: {ID: "users", Name: "Assignees", DataType: "ASSIGNEES"},
		2: {ID: "status", Name: "Status", DataType: "SINGLE_SELECT"},
	}
	page, err := boardProbeRESTItems(context.Background(), newClient(nil, rest), "orgs/example/projectsV2/1", 2, "", "1,2", fields)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasNext || page.EndCursor != "next" || len(page.Items) != 3 {
		t.Fatal("REST probe lost pagination or membership")
	}
	want := Item{
		Content: &Content{
			Kind: "Issue", ID: "issue", Number: 7, Title: "Synthetic title",
			URL: "https://github.com/example/repo/issues/7", State: "OPEN",
			Repository: "repo", RepositoryFullName: "example/repo",
			SubIssueTotal: 2, SubIssueDone: 1,
		},
	}
	if boardProbeCard(want, []Field{fields[1], fields[2]}) != boardProbeCard(page.Items[0], []Field{fields[1], fields[2]}) {
		t.Fatal("REST probe lost content or null/empty field semantics")
	}
	if page.Items[1].ID != "unavailable" || page.Items[1].Content != nil || boardProbePermissions(page.Items) != 0 {
		t.Fatal("REST probe invented content or permissions")
	}
	if c := page.Items[2].Content; c == nil || c.State != "MERGED" || !c.Merged || c.RepositoryFullName != "example/repo" {
		t.Fatal("REST probe lost merged PR state or repository")
	}
	if strings.Contains(rest.paths[0], "unexpected.invalid") {
		t.Fatal("REST probe followed an untrusted pagination host")
	}
}

func TestBoardProbeContentSelectionOmitsOnlyRequestedMetadata(t *testing.T) {
	for _, flags := range []struct{ permissions, progress bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		base := &fakeGraphQL{responses: []func(string, map[string]interface{}, interface{}) error{
			func(query string, variables map[string]interface{}, response interface{}) error {
				if strings.Contains(query, "viewerCanSetFields") == flags.permissions ||
					strings.Contains(query, "subIssuesSummary") == flags.progress ||
					!strings.Contains(query, "title") || !strings.Contains(query, "fieldValueByName") ||
					variables["filter"] != "is:open" || variables["field0"] != "Status" {
					t.Fatal("content probe changed more than its requested metadata")
				}
				return json.Unmarshal([]byte(`{"organization":{"projectV2":{"items":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`), response)
			},
		}}
		client := newClient(base, nil)
		_, err := boardProbeContentItems(context.Background(), client, Owner{Login: "example", Kind: OrganizationOwner}, 1, "is:open", "", []Field{{Name: "Status"}}, flags.permissions, flags.progress)
		if err != nil || client.graphql != base {
			t.Fatal("content probe failed or did not restore the normal query path")
		}
	}
}

func TestBoardProbeDifferencesReportCategoriesNotValues(t *testing.T) {
	field := Field{ID: "status", Name: "Private field name", DataType: "SINGLE_SELECT"}
	left := Item{Content: &Content{Title: "Before"}, FieldValues: []FieldValue{{FieldID: field.ID, OptionID: "before"}}}
	right := Item{Content: &Content{Title: "After"}, FieldValues: []FieldValue{{FieldID: field.ID, OptionID: "after"}}}
	want := map[string]int{"content/Title": 1, "cell/SINGLE_SELECT/OptionID": 1}
	if got := boardProbeDifferences(left, right, []Field{field}); !reflect.DeepEqual(got, want) {
		t.Fatal("difference categories do not describe the changed content and grouping identity")
	}
	right.FieldValues = nil
	want = map[string]int{"content/Title": 1, "cell/SINGLE_SELECT/presence": 1}
	if got := boardProbeDifferences(left, right, []Field{field}); !reflect.DeepEqual(got, want) {
		t.Fatal("difference categories missed a missing field")
	}
}

func TestBoardProbeComparesTitleSortWithoutContent(t *testing.T) {
	field := Field{ID: "title", Name: "Title", DataType: "TITLE"}
	left := Item{FieldValues: []FieldValue{{FieldID: field.ID, Value: "Before", Available: true}}}
	right := Item{FieldValues: []FieldValue{{FieldID: field.ID, Value: "After", Available: true}}}
	if boardProbeCard(left, []Field{field}) == boardProbeCard(right, []Field{field}) {
		t.Fatal("structure-only comparison ignored a title sort value")
	}
}
