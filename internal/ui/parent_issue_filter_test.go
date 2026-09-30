package ui

import (
	"context"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type parentIssueFilterSource struct {
	fakePickerSource
	filters []string
	cursors []string
}

func (source *parentIssueFilterSource) PageItems(ctx context.Context, owner github.Owner, project int, filter, after string) (github.ItemsPage, error) {
	source.filters = append(source.filters, filter)
	source.cursors = append(source.cursors, after)
	return source.fakePickerSource.PageItems(ctx, owner, project, filter, after)
}

// Issue #23 failed when opening this saved view, before any item request.
// Exercise that gate and both item pages together using its exact filter.
func TestParentIssueSavedViewOpensAndPagesExactFilter(t *testing.T) {
	const filter = `parent-issue:"glg/5mp#47" is:open`
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		for _, ownerKind := range []github.OwnerKind{github.UserOwner, github.OrganizationOwner} {
			t.Run(string(layout)+"/"+string(ownerKind), func(t *testing.T) {
				source := &parentIssueFilterSource{fakePickerSource: fakePickerSource{
					itemPagesByFilter: map[string]map[string]github.ItemsPage{
						filter: {
							"":     {Items: []github.Item{{ID: "first-child"}}, HasNext: true, EndCursor: "next"},
							"next": {Items: []github.Item{{ID: "second-child"}}},
						},
					},
				}}
				model := newPickerModel(source)
				model.screen = screenViewPicker
				model.selectedOwner = &github.Owner{Login: "glg", Kind: ownerKind}
				model.selectedProject = &github.Project{Number: 1}
				view := github.View{Number: 2, Name: "Sidekick MVP", Layout: layout, Filter: filter}
				updated, command := model.Update(viewDetailMsg{
					scope: model.currentPickerReadScope(), generation: model.generation, view: &view,
				})
				model = updated.(Model)
				if command == nil || model.screen != screenBoard || model.view == nil || !model.itemsLoading {
					t.Fatalf("parent-issue view did not enter item loading: status=%q", model.status)
				}
				batch := command().(tea.BatchMsg)
				if len(batch) != 2 {
					t.Fatalf("item load commands = %d, want spinner and one saved-filter stream", len(batch))
				}
				updated, next := model.Update(batch[1]())
				model = updated.(Model)
				if next == nil || len(model.items) != 1 || !model.itemsLoading {
					t.Fatal("first saved-filter page did not schedule its continuation")
				}
				updated, _ = model.Update(next())
				model = updated.(Model)
				if model.err != nil || model.itemsLoading || len(model.items) != 2 {
					t.Fatalf("saved-filter items = %d, loading=%v, error=%v", len(model.items), model.itemsLoading, model.err)
				}
				if !reflect.DeepEqual(source.filters, []string{filter, filter}) || !reflect.DeepEqual(source.cursors, []string{"", "next"}) {
					t.Fatalf("queries changed: filters=%q cursors=%q", source.filters, source.cursors)
				}
			})
		}
	}
}
