package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type membershipREST struct {
	fakeREST
	body, link string
	paths      []string
}

func (r *membershipREST) RequestWithContext(_ context.Context, method, path string, _ io.Reader) (*http.Response, error) {
	if method != "GET" {
		panic("membership must be read-only")
	}
	r.paths = append(r.paths, path)
	return &http.Response{Body: io.NopCloser(strings.NewReader(r.body)), Header: http.Header{"Link": []string{r.link}}}, nil
}
func TestRESTSavedViewMembershipUsesHostRelativePathsAndCursorOnly(t *testing.T) {
	for _, owner := range []Owner{{Login: "owner", Kind: UserOwner}, {Login: "owner", Kind: OrganizationOwner}} {
		rest := &membershipREST{body: `[{"id":123,"node_id":"one"},{"id":456,"node_id":"two"}]`, link: `<https://unexpected.invalid/items?after=a%2Bb%2Fc>; rel="next"`}
		c := newClient(nil, rest)
		page, err := c.PageSavedViewMembership(context.Background(), owner, 7, 3, "")
		if err != nil || !page.HasNext || page.EndCursor != "a+b/c" || len(page.Items) != 2 || page.Items[0].ID != "one" {
			t.Fatalf("page=%#v %v", page, err)
		}
		rest.link = ""
		rest.body = `[]`
		_, err = c.PageSavedViewMembership(context.Background(), owner, 7, 3, page.EndCursor)
		if err != nil || !strings.HasSuffix(rest.paths[1], "after=a%2Bb%2Fc&per_page=100") || strings.Contains(rest.paths[1], "unexpected") {
			t.Fatalf("paths=%v %v", rest.paths, err)
		}
		prefix := "orgs/"
		if owner.Kind == UserOwner {
			prefix = "users/"
		}
		if !strings.HasPrefix(rest.paths[0], prefix+"owner/projectsV2/7/views/3/items?") {
			t.Fatal(rest.paths)
		}
	}
}
func TestRESTMembershipRejectsMissingIdentitiesAndNonadvancingLinks(t *testing.T) {
	for _, tc := range []struct{ body, link string }{{`[{"id":1}]`, ""}, {`[{"node_id":"same"},{"node_id":"same"}]`, ""}, {`[]`, `<https://api.github.com/items>; rel="next"`}, {`[]`, `<https://api.github.com/items?after=same>; rel="next"`}} {
		rest := &membershipREST{body: tc.body, link: tc.link}
		_, err := newClient(nil, rest).PageSavedViewMembership(context.Background(), Owner{Login: "owner", Kind: UserOwner}, 1, 1, "same")
		if err == nil {
			t.Fatalf("accepted invalid response %#v", tc)
		}
	}
}
