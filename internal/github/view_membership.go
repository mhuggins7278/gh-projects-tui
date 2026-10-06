package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// PageSavedViewMembership is a read-only comparison surface. It is deliberately
// separate from production GraphQL item loading until parity is established.
func (c *Client) PageSavedViewMembership(ctx context.Context, owner Owner, project, view int, after string) (ItemsPage, error) {
	root := "orgs"
	switch owner.Kind {
	case UserOwner:
		root = "users"
	case OrganizationOwner:
	default:
		return ItemsPage{}, fmt.Errorf("unsupported owner kind %q", owner.Kind)
	}
	if owner.Login == "" || project <= 0 || view <= 0 {
		return ItemsPage{}, fmt.Errorf("saved view membership requires an owner and positive project/view numbers")
	}
	reader, ok := c.rest.(interface {
		RequestWithContext(context.Context, string, string, io.Reader) (*http.Response, error)
	})
	if !ok {
		return ItemsPage{}, fmt.Errorf("REST membership reads are unavailable")
	}
	values := url.Values{"per_page": {"100"}}
	if after != "" {
		values.Set("after", after)
	}
	path := fmt.Sprintf("%s/%s/projectsV2/%d/views/%d/items?%s", root, url.PathEscape(owner.Login), project, view, values.Encode())
	response, err := reader.RequestWithContext(ctx, "GET", path, nil)
	if err != nil {
		return ItemsPage{}, err
	}
	defer response.Body.Close()
	var nodes []struct {
		ID string `json:"node_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&nodes); err != nil {
		return ItemsPage{}, err
	}
	page := ItemsPage{}
	seen := map[string]bool{}
	for _, node := range nodes {
		if node.ID == "" || seen[node.ID] {
			return ItemsPage{}, fmt.Errorf("invalid or duplicate REST project item identity")
		}
		seen[node.ID] = true
		page.Items = append(page.Items, Item{ID: node.ID})
	}
	for _, link := range strings.Split(response.Header.Get("Link"), ",") {
		address, attributes, found := strings.Cut(strings.TrimSpace(link), ";")
		if !found || !strings.Contains(attributes, `rel="next"`) {
			continue
		}
		next, err := url.Parse(strings.Trim(address, "<>"))
		if err != nil {
			return ItemsPage{}, fmt.Errorf("invalid REST membership pagination link")
		}
		cursor := next.Query().Get("after")
		if cursor == "" || cursor == after {
			return ItemsPage{}, fmt.Errorf("REST membership pagination did not advance")
		}
		// Only use the cursor: never forward credentials to a Link-provided URL.
		page.HasNext, page.EndCursor = true, cursor
	}
	return page, nil
}
