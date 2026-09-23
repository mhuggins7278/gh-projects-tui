package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/config"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakeDiscoverySource struct {
	contexts []context.Context
}

func (f *fakeDiscoverySource) Discover(ctx context.Context) (github.Discovery, error) {
	f.contexts = append(f.contexts, ctx)
	return github.Discovery{}, nil
}

type rememberedDiscoverySource struct {
	discovery     github.Discovery
	discoveries   int
	ownerResolves int
	fallbackView  github.View
	fallbackCalls int
	savedViews    []github.ViewSummary
	savedBoard    github.View
	viewCalls     int
}

func (s *rememberedDiscoverySource) Discover(context.Context) (github.Discovery, error) {
	s.discoveries++
	return s.discovery, nil
}

func (s *rememberedDiscoverySource) ResolveOwner(context.Context, string) (github.Owner, []github.Project, error) {
	s.ownerResolves++
	return github.Owner{}, nil, errors.New("direct owner path should not run")
}

func (s *rememberedDiscoverySource) OpenStatusFallback(context.Context, github.Owner, int) (github.View, error) {
	s.fallbackCalls++
	return s.fallbackView, nil
}

func (s *rememberedDiscoverySource) ListViews(context.Context, github.Owner, int) ([]github.ViewSummary, error) {
	return s.savedViews, nil
}

func (s *rememberedDiscoverySource) OpenView(context.Context, github.Owner, int, int) (github.View, error) {
	s.viewCalls++
	return s.savedBoard, nil
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

func (fakeSelectionSource) OpenStatusFallback(context.Context, github.Owner, int) (github.View, error) {
	return github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true}, nil
}

func (fakeSelectionSource) ListViews(context.Context, github.Owner, int) ([]github.ViewSummary, error) {
	return nil, nil
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

func TestDirectFallbackSelectionOpensFallbackInsteadOfViewZero(t *testing.T) {
	model := NewModelWithSelection(fakeSelectionSource{}, Selection{OwnerLogin: "org", ProjectNumber: 7, Fallback: true})
	message := model.Init()()
	updated, cmd := model.Update(message)
	result := updated.(Model)
	if cmd == nil || result.screen != screenViewPicker || result.selection.ViewNumber != 0 || !result.selection.Fallback {
		t.Fatalf("direct fallback resolution = %#v, cmd nil=%v", result, cmd == nil)
	}
	updated, probe := result.Update(cmd())
	result = updated.(Model)
	if probe == nil || !result.checkingBoardViews {
		t.Fatal("direct fallback did not check saved board compatibility")
	}
	updated, items := result.Update(probe())
	result = updated.(Model)
	if items == nil || result.screen != screenBoard || result.view == nil || !result.view.Fallback {
		t.Fatalf("direct fallback open = %#v, items nil=%v", result, items == nil)
	}
}

func TestNoArgumentStartupDiscoversAllOwnersBeforeRememberedSelection(t *testing.T) {
	source := &rememberedDiscoverySource{discovery: github.Discovery{
		Owners: []github.Owner{
			{Login: "org-one", Kind: github.OrganizationOwner},
			{Login: "org-two", Kind: github.OrganizationOwner},
		},
		Projects: map[string][]github.Project{},
	}}
	model := NewModelWithHost(source, Selection{}, "github.com")
	model.loadPrefs = func(string) (config.Selection, error) {
		return config.Selection{Owner: "stale-org", Project: 7, View: 3}, nil
	}

	message := model.Init()()
	if source.discoveries != 1 || source.ownerResolves != 0 {
		t.Fatalf("startup calls = discoveries %d, direct resolves %d", source.discoveries, source.ownerResolves)
	}
	updated, _ := model.Update(message)
	result := updated.(Model)
	if result.screen != screenOwnerPicker || len(result.discovery.Owners) != 2 {
		t.Fatalf("startup discovery state = screen %v, owners %#v", result.screen, result.discovery.Owners)
	}
}

func TestRememberedFallbackSelectionRestoresAfterDiscovery(t *testing.T) {
	fallback := github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true}
	source := &rememberedDiscoverySource{
		discovery: github.Discovery{
			Owners:   []github.Owner{{Login: "org", Kind: github.OrganizationOwner}},
			Projects: map[string][]github.Project{"org": {{Number: 7, Title: "Project"}}},
		},
		fallbackView: fallback,
	}
	model := NewModelWithHost(source, Selection{}, "github.com")
	model.loadPrefs = func(string) (config.Selection, error) {
		return config.Selection{Owner: "org", Project: 7, Fallback: true}, nil
	}
	updated, fallbackCmd := model.Update(model.Init()())
	result := updated.(Model)
	if fallbackCmd == nil || !result.selection.Fallback || result.selection.ViewNumber != 0 {
		t.Fatalf("remembered fallback selection = %#v, cmd nil=%v", result.selection, fallbackCmd == nil)
	}
	updated, probe := result.Update(fallbackCmd())
	result = updated.(Model)
	if probe == nil || !result.checkingBoardViews {
		t.Fatal("remembered fallback did not check saved board compatibility")
	}
	updated, items := result.Update(probe())
	result = updated.(Model)
	if items == nil || result.screen != screenBoard || result.view == nil || !result.view.Fallback || source.fallbackCalls != 1 {
		t.Fatalf("remembered fallback restore = screen:%v view:%#v calls:%d", result.screen, result.view, source.fallbackCalls)
	}
}

func TestRememberedFallbackIsRetiredWhenSavedBoardBecomesCompatible(t *testing.T) {
	source := &rememberedDiscoverySource{
		discovery: github.Discovery{
			Owners:   []github.Owner{{Login: "org", Kind: github.OrganizationOwner}},
			Projects: map[string][]github.Project{"org": {{Number: 7}}},
		},
		fallbackView: github.View{Name: "Unfiltered Status fallback", Layout: github.BoardLayout, Fallback: true},
		savedViews:   []github.ViewSummary{{Number: 2, Name: "New board", Layout: github.BoardLayout}},
		savedBoard:   github.View{Number: 2, Name: "New board", Layout: github.BoardLayout},
	}
	model := NewModelWithHost(source, Selection{}, "github.com")
	model.loadPrefs = func(string) (config.Selection, error) {
		return config.Selection{Owner: "org", Project: 7, Fallback: true}, nil
	}
	var saved config.Selection
	model.savePrefs = func(_ string, selection config.Selection) error { saved = selection; return nil }
	updated, viewsCmd := model.Update(model.Init()())
	model = updated.(Model)
	updated, probe := model.Update(viewsCmd())
	model = updated.(Model)
	updated, next := model.Update(probe())
	model = updated.(Model)
	if next != nil || model.screen != screenViewPicker || model.view != nil || model.statusFallback != nil || source.fallbackCalls != 0 || source.viewCalls != 1 || model.selection.Fallback || saved != (config.Selection{Owner: "org", Project: 7}) {
		t.Fatalf("new saved board failed to retire remembered fallback: screen=%v view=%#v fallbackCalls=%d saved=%#v", model.screen, model.view, source.fallbackCalls, saved)
	}
	if !strings.Contains(model.viewPickerNotice, "compatible saved board") {
		t.Fatalf("missing explanation for retired fallback: %q", model.viewPickerNotice)
	}
}
