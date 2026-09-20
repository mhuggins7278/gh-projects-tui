package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakeDiscoverySource struct {
	contexts []context.Context
}

func (f *fakeDiscoverySource) Discover(ctx context.Context) (github.Discovery, error) {
	f.contexts = append(f.contexts, ctx)
	return github.Discovery{}, nil
}

type fakeSelectionSource struct{}

func (fakeSelectionSource) Discover(context.Context) (github.Discovery, error) {
	return github.Discovery{}, errors.New("membership discovery should not run")
}

func (fakeSelectionSource) ResolveOwner(context.Context, string) (github.Owner, []github.Project, error) {
	return github.Owner{Login: "org", Kind: github.OrganizationOwner}, []github.Project{{Number: 7}}, nil
}

func (fakeSelectionSource) OpenView(context.Context, github.Owner, int, int) (github.View, error) {
	return github.View{Number: 3, Name: "Board", Layout: github.BoardLayout}, nil
}

func TestModelIgnoresStaleDiscoveryMessages(t *testing.T) {
	model := NewModel(nil)
	model.generation = 2
	updated, _ := model.Update(discoveryMsg{generation: 1, err: errors.New("stale")})
	result := updated.(Model)
	if !result.loading || result.err != nil {
		t.Fatalf("stale message changed model: %#v", result)
	}
}

func TestRefreshCancelsPreviousDiscovery(t *testing.T) {
	source := &fakeDiscoverySource{}
	model := NewModel(source)
	oldContext := model.ctx
	updated, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
	result := updated.(Model)
	if result.generation != 1 || !result.loading || cmd == nil {
		t.Fatalf("refresh state = %#v, command nil = %v", result, cmd == nil)
	}
	select {
	case <-oldContext.Done():
	default:
		t.Fatal("refresh did not cancel the previous context")
	}
	cmd()
	if len(source.contexts) != 1 || source.contexts[0] != result.ctx {
		t.Fatalf("refresh context = %#v", source.contexts)
	}
}

func TestViewNamesMembershipAndOwnerErrors(t *testing.T) {
	model := NewModel(nil)
	model.loading = false
	model.discovery = github.Discovery{
		Owners:          []github.Owner{{Login: "me", Kind: github.UserOwner}},
		Projects:        map[string][]github.Project{"me": {{Number: 1}}},
		OwnerErrors:     []github.OwnerFailure{{Owner: github.Owner{Login: "org"}, Err: errors.New("SSO required")}},
		MembershipError: errors.New("membership unavailable"),
	}
	view := model.View().Content
	if !strings.Contains(view, "org: SSO required") || !strings.Contains(view, "membership unavailable") {
		t.Fatalf("view = %q", view)
	}
}

func TestDirectSelectionSkipsMembershipDiscovery(t *testing.T) {
	model := NewModelWithSelection(fakeSelectionSource{}, Selection{OwnerLogin: "org", ProjectNumber: 7, ViewNumber: 3})
	message := model.Init()()
	updated, _ := model.Update(message)
	result := updated.(Model)
	if result.err != nil || result.view == nil || result.view.Name != "Board" {
		t.Fatalf("direct selection result = %#v", result)
	}
}
