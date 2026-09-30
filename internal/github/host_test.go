package github

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientUsesResolvedHostForGraphQLAndREST(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hosts.yml"), []byte("wrong.example:\n  user: fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_CONFIG_DIR", dir)
	t.Setenv("GH_HOST", "wrong.example")
	t.Setenv("GH_ENTERPRISE_TOKEN", "fixture-token")
	t.Setenv("GH_DEBUG", "")
	previous := http.DefaultTransport
	var paths []string
	http.DefaultTransport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "chosen.example" {
			t.Fatalf("request host = %q", request.URL.Host)
		}
		paths = append(paths, request.URL.Path)
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"data":{}}`)), Request: request}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = previous })
	client, err := NewClientWithHost("chosen.example")
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err = client.graphql.DoWithContext(context.Background(), "query HostFixture { viewer { login } }", nil, &result); err != nil {
		t.Fatal(err)
	}
	if err = client.rest.DoWithContext(context.Background(), "GET", "user", nil, &result); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/api/graphql" || paths[1] != "/api/v3/user" {
		t.Fatalf("paths=%#v", paths)
	}
}
