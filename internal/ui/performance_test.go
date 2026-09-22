package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type delayedItemsSource struct {
	pages map[string]github.ItemsPage
	delay time.Duration
	calls int
}

func (s *delayedItemsSource) Discover(context.Context) (github.Discovery, error) {
	return github.Discovery{}, nil
}

func (s *delayedItemsSource) PageItems(ctx context.Context, _ github.Owner, _ int, _ string, after string) (github.ItemsPage, error) {
	s.calls++
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return github.ItemsPage{}, ctx.Err()
	case <-timer.C:
	}
	return s.pages[after], nil
}

func newDelayedItemsSource(total, pageSize int, delay time.Duration) *delayedItemsSource {
	source := &delayedItemsSource{pages: make(map[string]github.ItemsPage), delay: delay}
	items := make([]github.Item, 0, total)
	for index := 0; index < total; index++ {
		items = append(items, github.Item{
			ID: fmt.Sprintf("item-%04d", index),
			Content: &github.Content{
				Kind:  "Issue",
				Title: fmt.Sprintf("Large fixture item %04d", index),
			},
		})
	}
	for start := 0; start < len(items); start += pageSize {
		end := start + pageSize
		if end > len(items) {
			end = len(items)
		}
		page := github.ItemsPage{Items: items[start:end]}
		if end < len(items) {
			page.HasNext = true
			page.EndCursor = fmt.Sprintf("page-%d", end)
		}
		cursor := ""
		if start > 0 {
			cursor = fmt.Sprintf("page-%d", start)
		}
		source.pages[cursor] = page
	}
	return source
}

func TestLargeBoardShowsFirstPageBeforeFinalPage(t *testing.T) {
	source := newDelayedItemsSource(1000, 100, 2*time.Millisecond)
	model := NewModel(source)
	model.screen = screenBoard
	model.view = &github.View{Name: "Large fixture", Layout: github.BoardLayout}
	model.selectedOwner = &github.Owner{Login: "org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 1}
	model.width = 100
	model.height = 32

	next := model.startItemsLoad()
	if next == nil {
		t.Fatal("large fixture did not start loading")
	}
	started := time.Now()
	updated, next := model.Update(next())
	firstPageLatency := time.Since(started)
	model = updated.(Model)
	if firstPageLatency >= 100*time.Millisecond {
		t.Fatalf("first page latency = %s", firstPageLatency)
	}
	if len(model.items) != 100 || !model.itemsLoading || next == nil {
		t.Fatalf("first page state = items:%d loading:%v next:%v", len(model.items), model.itemsLoading, next != nil)
	}
	content := ansi.Strip(model.View().Content)
	if !strings.Contains(content, "Large fixture item 0000") || strings.Contains(content, "Large fixture item 0999") {
		t.Fatalf("partial board rendering = %q", content)
	}

	inputStarted := time.Now()
	updated, _ = model.Update(keyPress("j"))
	model = updated.(Model)
	_ = model.View().Content
	if inputLatency := time.Since(inputStarted); inputLatency >= 100*time.Millisecond {
		t.Fatalf("navigation latency during loading = %s", inputLatency)
	}

	for next != nil {
		updated, next = model.Update(next())
		model = updated.(Model)
	}
	if len(model.items) != 1000 || model.itemsLoading {
		t.Fatalf("final fixture state = items:%d loading:%v", len(model.items), model.itemsLoading)
	}
	if source.calls != 10 {
		t.Fatalf("page request count = %d, want 10", source.calls)
	}
	if model.items[len(model.items)-1].ID != "item-0999" {
		t.Fatal("final page item was not loaded")
	}
}
