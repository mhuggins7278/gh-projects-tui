package ui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type viewRefreshSource struct {
	fakePickerSource
	err                  error
	reads, invalidations int
	ctx                  context.Context
	filters, cursors     []string
	fields               [][]github.Field
}

func (s *viewRefreshSource) InvalidateReads() { s.invalidations++ }
func (s *viewRefreshSource) OpenView(ctx context.Context, owner github.Owner, project, number int) (github.View, error) {
	s.reads++
	s.ctx = ctx
	return s.view, s.err
}
func (s *viewRefreshSource) PageBoardItems(ctx context.Context, owner github.Owner, project int, filter, after string, fields []github.Field) (github.ItemsPage, error) {
	s.filters = append(s.filters, filter)
	s.cursors = append(s.cursors, after)
	s.fields = append(s.fields, fields)
	return s.PageItems(ctx, owner, project, filter, after)
}

func refreshTestModel(layout github.ViewLayout) (Model, *viewRefreshSource) {
	view := writableStatusView()
	view.ID, view.Name, view.Layout, view.Filter = "view", "Old name", layout, "is:open"
	view.Fields = []github.Field{{ID: "old", Name: "Old column", DataType: "TEXT"}}
	items := []github.Item{{ID: "selected", Content: &github.Content{Title: "Selected"}}}
	source := &viewRefreshSource{fakePickerSource: fakePickerSource{view: view}}
	m := mutationModel(source.fakePickerSource, view, items)
	m.source = source
	m.views = []github.ViewSummary{{ID: view.ID, Number: view.Number, Name: view.Name, Layout: view.Layout}}
	m.viewsCache = make(map[string]viewsCacheEntry)
	m.viewsCache[viewsCacheKey(*m.selectedOwner, m.selectedProject.Number)] = viewsCacheEntry{views: append([]github.ViewSummary(nil), m.views...)}
	m.boardFocusID, m.tableFocusID = "selected", "selected"
	return m, source
}

func TestRefreshReadsCurrentSettingsBeforeItemsAndPreservesSelection(t *testing.T) {
	for _, oldLayout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, newLayout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
			for _, ownerKind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
				t.Run(string(oldLayout)+"/"+string(newLayout)+"/"+string(ownerKind), func(t *testing.T) {
					m, source := refreshTestModel(oldLayout)
					m.selectedOwner.Kind = ownerKind
					fresh := *m.view
					fresh.Name, fresh.Layout, fresh.Filter = "New name", newLayout, "is:closed"
					fresh.Fields = []github.Field{{ID: "estimate", Name: "Estimate", DataType: "NUMBER"}, {ID: "title", Name: "Title", DataType: "TITLE"}}
					fresh.GroupByFields = []github.Field{{ID: "priority", Name: "Priority", DataType: "SINGLE_SELECT", Options: []github.FieldOption{{ID: "high", Name: "High"}, {ID: "low", Name: "Low"}}}}
					fresh.SortByFields = []github.SortField{{Direction: "DESC", Field: fresh.Fields[0]}}
					source.view = fresh
					source.itemPagesByFilter = map[string]map[string]github.ItemsPage{"is:closed": {
						"":     {Items: []github.Item{{ID: "first", FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "high", Available: true}}}}, HasNext: true, EndCursor: "next"},
						"next": {Items: []github.Item{{ID: "selected", FieldValues: []github.FieldValue{{FieldID: "priority", OptionID: "low", Available: true}}}}},
					}}
					updated, cmd := m.Update(keyPress("r"))
					m = updated.(Model)
					if cmd == nil || !m.loadingDetail || !m.itemsLoading || len(m.items) != 1 || m.view.Filter != "is:open" || len(source.filters) != 0 {
						t.Fatal("refresh did not retain rows while awaiting metadata")
					}
					updated, repeat := m.Update(keyPress("r"))
					m = updated.(Model)
					if repeat != nil || source.invalidations != 1 {
						t.Fatal("repeated refresh restarted the read")
					}
					if m.openItemDetailCmd() != nil {
						t.Fatal("opened detail from stale metadata")
					}
					var display strings.Builder
					m.renderBoardContent(&display)
					if !strings.Contains(display.String(), "Selected") {
						t.Fatal("metadata loading hid the existing rows")
					}
					m = runMembershipReads(t, m, cmd)
					item, ok := m.selectedItem()
					if !ok || item.ID != "selected" || m.loadingDetail || m.itemsLoading || m.refreshFocusID != "" || !reflect.DeepEqual(*m.view, fresh) {
						t.Fatalf("fresh settings/selection not applied: selected=%v view=%v", item.ID, m.view)
					}
					if source.reads != 1 || !reflect.DeepEqual(source.filters, []string{"is:closed", "is:closed"}) || !reflect.DeepEqual(source.cursors, []string{"", "next"}) {
						t.Fatalf("incorrect metadata/items pipeline: reads=%d filters=%v cursors=%v", source.reads, source.filters, source.cursors)
					}
					for _, fields := range source.fields {
						if !reflect.DeepEqual(fields, boardReadFields(fresh)) {
							t.Fatal("used stale grouping/sort/visible fields")
						}
					}
					if m.views[0].Name != fresh.Name || m.views[0].Layout != fresh.Layout {
						t.Fatal("picker summary stayed stale")
					}
				})
			}
		}
	}
}

func TestRefreshRejectsNewUnsupportedSettings(t *testing.T) {
	for _, change := range []string{"roadmap", "filter", "group", "sort"} {
		t.Run(change, func(t *testing.T) {
			m, source := refreshTestModel(github.BoardLayout)
			switch change {
			case "roadmap":
				source.view.Layout = github.RoadmapLayout
			case "filter":
				source.view.Filter = "unknown:value"
			case "group":
				source.view.GroupByFields = []github.Field{{Name: "Text", DataType: "TEXT"}}
			case "sort":
				source.view.SortByFields = []github.SortField{{Direction: "SIDEWAYS", Field: github.Field{Name: "Title"}}}
			}
			updated, cmd := m.Update(keyPress("r"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if m.screen != screenViewPicker || m.view != nil || len(m.items) != 0 || m.loadingDetail || m.itemsLoading || len(source.filters) != 0 || !strings.Contains(m.viewPickerNotice, "unsupported") {
				t.Fatal("incompatible settings bypassed the existing rejection flow")
			}
			if m.views[0].Layout != source.view.Layout {
				t.Fatal("picker has stale layout")
			}
		})
	}
}

func TestRefreshMetadataFailureRetainsRowsAndAllowsRetry(t *testing.T) {
	for _, failure := range []string{"read", "identity"} {
		t.Run(failure, func(t *testing.T) {
			m, source := refreshTestModel(github.TableLayout)
			old := *m.view
			if failure == "read" {
				source.err = errors.New("network unavailable")
			} else {
				source.view.ID = "another-view"
			}
			updated, cmd := m.Update(keyPress("r"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if !reflect.DeepEqual(*m.view, old) || len(m.items) != 1 || m.items[0].ID != "selected" || m.itemsErr == nil || m.loadingDetail || m.itemsLoading || len(source.filters) != 0 || !strings.Contains(m.status, "press r to retry") {
				t.Fatal("metadata failure lost rows, read items, or left refresh stuck")
			}
			if m.tableActionUnavailable() == "" {
				t.Fatal("writes allowed after failed metadata refresh")
			}
			source.err, source.view = nil, old
			source.itemPages = map[string]github.ItemsPage{"": {Items: []github.Item{{ID: "selected"}}}}
			updated, cmd = m.Update(keyPress("r"))
			m = runMembershipReads(t, updated.(Model), cmd)
			if m.itemsErr != nil || m.itemsLoading || m.loadingDetail || source.reads != 2 || len(source.filters) != 1 {
				t.Fatal("retry failed to reload")
			}
		})
	}
}

func TestRefreshDropsStaleMetadataAndItemPages(t *testing.T) {
	for _, change := range []string{"navigation", "host", "project", "view", "generation"} {
		t.Run(change, func(t *testing.T) {
			m, source := refreshTestModel(github.BoardLayout)
			oldScope := m.currentBoardReadScope()
			updated, cmd := m.Update(keyPress("r"))
			m = updated.(Model)
			msg := cmd()
			for _, scope := range []boardReadScope{oldScope, m.currentBoardReadScope()} {
				updated, next := m.Update(itemsPageMsg{scope: scope, generation: scope.generation, reset: true, page: github.ItemsPage{Items: []github.Item{{ID: "stale"}}}})
				m = updated.(Model)
				if next != nil || len(m.items) != 1 || m.items[0].ID != "selected" || !m.loadingDetail {
					t.Fatal("page installed before metadata")
				}
			}
			switch change {
			case "navigation":
				updated, _ = m.Update(keyPress("esc"))
				m = updated.(Model)
			case "host":
				m.host = "other.example"
			case "project":
				m.selectedProject = &github.Project{Number: 99}
			case "view":
				replacement := *m.view
				replacement.Number++
				m.view = &replacement
			case "generation":
				m.generation++
			}
			before := m.view
			updated, next := m.Update(msg)
			m = updated.(Model)
			if next != nil || m.view != before || len(source.filters) != 0 {
				t.Fatal("stale metadata installed after destination changed")
			}
			if change == "navigation" && source.ctx.Err() == nil {
				t.Fatal("navigation did not cancel refresh")
			}
		})
	}
}

func TestSubmittedSaveCompletesWhileRefreshAwaitsMetadata(t *testing.T) {
	m, save, canonical := submittedReviewSave(t)
	updated, refresh := m.Update(keyPress("r"))
	m = updated.(Model)
	updated, next := m.Update(save())
	m = updated.(Model)
	if next != nil || !m.loadingDetail || !m.itemsLoading || m.pendingMutationReload == nil || (*canonical)[0].FieldValues[0].OptionID != "b" {
		t.Fatal("submitted save interrupted metadata refresh or was discarded")
	}
	m = runMembershipReads(t, m, refresh)
	if m.loadingDetail || m.itemsLoading || m.pendingMutationReload != nil || len(m.items) != 1 || m.items[0].FieldValues[0].OptionID != "b" {
		t.Fatal("refresh lost the completed save")
	}
}

func TestRefreshSelectionRestorationYieldsToNavigationAndSearch(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, action := range []string{"k", "h", "/"} {
			t.Run(string(layout)+"/"+action, func(t *testing.T) {
				m, source := refreshTestModel(layout)
				source.view.GroupByFields = nil
				updated, refresh := m.Update(keyPress("r"))
				m = updated.(Model)
				updated, _ = m.Update(refresh())
				m = updated.(Model)
				scope := m.currentBoardReadScope()
				updated, _ = m.Update(itemsPageMsg{scope: scope, generation: m.generation, reset: true, page: github.ItemsPage{Items: []github.Item{{ID: "first"}, {ID: "second"}}, HasNext: true, EndCursor: "next"}})
				m = updated.(Model)
				key := action
				if key == "h" && layout == github.TableLayout {
					key = "j"
				}
				updated, _ = m.Update(keyPress(key))
				m = updated.(Model)
				if m.refreshFocusID != "" {
					t.Fatal("restoration target survived deliberate navigation/search")
				}
				chosen, _ := m.selectedItem()
				updated, _ = m.Update(itemsPageMsg{scope: scope, generation: m.generation, page: github.ItemsPage{Items: []github.Item{{ID: "selected"}}}})
				m = updated.(Model)
				actual, _ := m.selectedItem()
				if actual.ID != chosen.ID {
					t.Fatalf("later page changed user's selection: got %s want %s", actual.ID, chosen.ID)
				}
			})
		}
	}
}
