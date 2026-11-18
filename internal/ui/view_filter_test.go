package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type filterEditSource struct {
	fakePickerSource
	writes int
	saved  string
	err    error
}

func (s *filterEditSource) SaveViewFilter(_ context.Context, id, original, filter string) error {
	s.writes++
	s.saved = filter
	return s.err
}

func TestViewFilterPreviewSaveAndDiscard(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		t.Run(string(layout), func(t *testing.T) {
			source := &filterEditSource{fakePickerSource: fakePickerSource{itemPagesByFilter: map[string]map[string]github.ItemsPage{
				"is:open": {"": {Items: []github.Item{{ID: "open"}}}}, "is:closed": {"": {Items: []github.Item{{ID: "closed"}}}},
			}}}
			m := tableActionModel(source)
			m.view.Layout = layout
			m.view.ID = "view"
			m.view.Filter = "is:open"
			m, _ = editorKey(m, "f")
			m, _ = editorKey(m, "ctrl+u")
			m, _ = editorKey(m, "is:closed")
			m, cmd := editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			if source.writes != 0 || m.view.Filter != "is:closed" || len(m.items) != 1 || m.items[0].ID != "closed" || !strings.Contains(m.View().Content, "FILTER PREVIEW") {
				t.Fatal("preview did not load without saving")
			}
			m, cmd = filterKey(m, "ctrl+g")
			m = runMembershipReads(t, m, cmd)
			if m.view.Filter != "is:open" || m.viewFilter != nil || m.items[0].ID != "open" {
				t.Fatal("discard did not restore filter")
			}
			m, _ = editorKey(m, "f")
			m, _ = editorKey(m, "ctrl+u")
			m, _ = editorKey(m, "is:closed")
			m, cmd = editorKey(m, "enter")
			m = runMembershipReads(t, m, cmd)
			source.err = errors.New("conflict")
			m, cmd = filterKey(m, "ctrl+s")
			m = runMembershipReads(t, m, cmd)
			if m.viewFilter == nil || !strings.Contains(m.status, "conflict") {
				t.Fatal("failed save lost preview")
			}
			source.err = nil
			m, cmd = filterKey(m, "ctrl+s")
			m = runMembershipReads(t, m, cmd)
			if m.viewFilter != nil || source.saved != "is:closed" || !strings.Contains(m.status, "saved") {
				t.Fatal("save failed")
			}
		})
	}
}

func TestViewFilterCursorAndCancel(t *testing.T) {
	m := tableActionModel(&filterEditSource{})
	m.view.Filter = "is:open"
	m, _ = editorKey(m, "f")
	m, _ = filterKey(m, "home")
	m, _ = editorKey(m, "-")
	if m.viewFilter.draft != "-is:open" {
		t.Fatal(m.viewFilter.draft)
	}
	m, _ = filterKey(m, "delete")
	if m.viewFilter.draft != "-s:open" {
		t.Fatal(m.viewFilter.draft)
	}
	m, _ = editorKey(m, "esc")
	if m.viewFilter != nil || m.view.Filter != "is:open" {
		t.Fatal("cancel changed active filter")
	}
}

func filterKey(m Model, key string) (Model, tea.Cmd) {
	msg := keyPress(key)
	switch key {
	case "home":
		msg = tea.KeyPressMsg(tea.Key{Code: tea.KeyHome})
	case "delete":
		msg = tea.KeyPressMsg(tea.Key{Code: tea.KeyDelete})
	case "ctrl+s":
		msg = tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl})
	case "ctrl+g":
		msg = tea.KeyPressMsg(tea.Key{Code: 'g', Mod: tea.ModCtrl})
	}
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestViewFilterCannotSaveFailedPreviewOrDiscardDuringWrite(t *testing.T) {
	source := &filterEditSource{}
	m := tableActionModel(source)
	m.view.ID = "view"
	m.viewFilter = &viewFilterState{preview: true, original: "is:open"}
	m.itemsErr = errors.New("bad query")
	m, cmd := filterKey(m, "ctrl+s")
	if cmd != nil || source.writes != 0 {
		t.Fatal("failed preview was saved")
	}
	m.issueActionPending = &issueActionState{}
	m, cmd = filterKey(m, "ctrl+g")
	if cmd != nil || m.viewFilter == nil {
		t.Fatal("discard interrupted pending write")
	}
}
