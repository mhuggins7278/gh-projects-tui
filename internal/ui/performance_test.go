package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type delayedItemsSource struct {
	pages       map[string]github.ItemsPage
	delay       time.Duration
	detailDelay time.Duration
	pageGate    <-chan struct{}
	detailGate  <-chan struct{}
	calls       int
	detailCalls int
}

func (s *delayedItemsSource) Discover(context.Context) (github.Discovery, error) {
	return github.Discovery{}, nil
}

func (s *delayedItemsSource) PageItems(ctx context.Context, _ github.Owner, _ int, _ string, after string) (github.ItemsPage, error) {
	s.calls++
	if after != "" && s.pageGate != nil {
		select {
		case <-ctx.Done():
			return github.ItemsPage{}, ctx.Err()
		case <-s.pageGate:
		}
	}
	timer := time.NewTimer(s.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return github.ItemsPage{}, ctx.Err()
	case <-timer.C:
	}
	return s.pages[after], nil
}

func (s *delayedItemsSource) LoadItemDetail(ctx context.Context, _ github.Owner, _ int, itemID string) (github.ItemDetail, error) {
	s.detailCalls++
	if s.detailGate != nil {
		select {
		case <-ctx.Done():
			return github.ItemDetail{}, ctx.Err()
		case <-s.detailGate:
		}
	}
	timer := time.NewTimer(s.detailDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return github.ItemDetail{}, ctx.Err()
	case <-timer.C:
		return github.ItemDetail{ID: itemID, Content: &github.Content{Kind: "Issue", Title: "Fixture detail"}}, nil
	}
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

func TestLargeBoardP95InputLatencyDuringPageAndDetailReads(t *testing.T) {
	source := newDelayedItemsSource(1000, 100, 3*time.Millisecond)
	source.detailDelay = 80 * time.Millisecond
	pageGate := make(chan struct{})
	detailGate := make(chan struct{})
	source.pageGate = pageGate
	source.detailGate = detailGate
	model := NewModel(source)
	model.screen = screenBoard
	model.view = &github.View{Name: "Performance fixture", Layout: github.BoardLayout}
	model.selectedOwner = &github.Owner{Login: "fixture-org", Kind: github.OrganizationOwner}
	model.selectedProject = &github.Project{Number: 99}
	model.width = 123
	model.height = 63

	firstPageCmd := model.startItemsLoad()
	firstPageStarted := time.Now()
	updated, nextPageCmd := model.Update(firstPageCmd())
	model = updated.(Model)
	firstPageContent := ansi.Strip(model.View().Content)
	firstPageLatency := time.Since(firstPageStarted)
	if len(model.items) != 100 || !model.itemsLoading || nextPageCmd == nil {
		t.Fatalf("progressive first page = items:%d loading:%v next:%v", len(model.items), model.itemsLoading, nextPageCmd != nil)
	}
	if !strings.Contains(firstPageContent, "Large fixture item 0000") || strings.Contains(firstPageContent, "Large fixture item 0999") {
		t.Fatal("first page was not visible before later pages")
	}

	pageResult := make(chan tea.Msg, 1)
	go func() { pageResult <- nextPageCmd() }()
	updated, detailDebounceCmd := model.Update(keyPress("enter"))
	model = updated.(Model)
	if detailDebounceCmd == nil {
		t.Fatal("detail debounce did not start")
	}
	debounceMessage := detailDebounceCmd()
	updated, detailLoadCmd := model.Update(debounceMessage)
	model = updated.(Model)
	if detailLoadCmd == nil {
		t.Fatal("detail read did not start")
	}
	detailResult := make(chan tea.Msg, 1)
	go func() { detailResult <- detailLoadCmd() }()

	// Closing the full-screen detail returns to the board while the request is
	// still in flight. Exercise navigation, search, and rendering during both
	// that detail fetch and a delayed next board page.
	updated, _ = model.Update(keyPress("esc"))
	model = updated.(Model)
	latencies := make([]time.Duration, 0, 110)
	measure := func(key string) {
		started := time.Now()
		updated, _ := model.Update(keyPress(key))
		model = updated.(Model)
		_ = model.View().Content
		latencies = append(latencies, time.Since(started))
	}
	for index := 0; index < 60; index++ {
		measure("j")
	}
	measure("/")
	for index := 0; index < 20; index++ {
		measure("a")
		measure("backspace")
	}
	measure("esc")
	close(pageGate)
	close(detailGate)

	pageMessage := <-pageResult
	updated, nextPageCmd = model.Update(pageMessage)
	model = updated.(Model)
	updated, _ = model.Update(<-detailResult)
	model = updated.(Model)
	if model.detail != nil || !model.itemsLoading || len(model.items) != 200 {
		t.Fatalf("in-flight read reconciliation = detail:%#v loading:%v items:%d", model.detail, model.itemsLoading, len(model.items))
	}
	focusID := model.boardFocusID
	for nextPageCmd != nil {
		updated, nextPageCmd = model.Update(nextPageCmd())
		model = updated.(Model)
	}

	p95, max := latencyPercentiles(latencies)
	t.Logf("reference: linux/amd64, Intel Core Ultra 9 185H, terminal 123x63; first-page=%s input+view p95=%s max=%s; requests: pages=%d detail=%d", firstPageLatency, p95, max, source.calls, source.detailCalls)
	if firstPageLatency >= 100*time.Millisecond {
		t.Fatalf("first-page latency = %s, target <100ms", firstPageLatency)
	}
	if p95 >= 100*time.Millisecond {
		t.Fatalf("input+view p95 = %s, target <100ms", p95)
	}
	if len(model.items) != 1000 || model.itemsLoading || source.calls != 10 || source.detailCalls != 1 {
		t.Fatalf("completed fixture = items:%d loading:%v page requests:%d detail requests:%d", len(model.items), model.itemsLoading, source.calls, source.detailCalls)
	}
	if model.boardFocusID != focusID {
		t.Fatalf("selection identity changed while pages settled: got %q want %q", model.boardFocusID, focusID)
	}

	// The first-page measurement cannot reveal work proportional to all cards
	// in a lane. Measure the same key-to-render path with the full board loaded,
	// including a card at the far end and a saved field sort.
	model.boardCard = len(model.items) - 1
	model.rememberBoardFocus()
	if content := ansi.Strip(model.View().Content); !strings.Contains(content, "Large fixture item 0999") {
		t.Fatalf("last focused card was not rendered in its viewport: %q", content)
	}
	fullLatencies := make([]time.Duration, 0, 90)
	measureFull := func(key string) {
		started := time.Now()
		updated, _ := model.Update(keyPress(key))
		model = updated.(Model)
		_ = model.View().Content
		fullLatencies = append(fullLatencies, time.Since(started))
	}
	for index := 0; index < 60; index++ {
		if index%2 == 0 {
			measureFull("k")
		} else {
			measureFull("j")
		}
	}
	measureFull("/")
	for _, key := range []string{"L", "a", "r", "g", "e"} {
		measureFull(key)
	}
	measureFull("enter")
	positionP95, positionMax := latencyPercentiles(fullLatencies)
	if model.boardFocusID != "item-0999" || model.filter != "Large" || len(model.boardItems()) != 1000 {
		t.Fatalf("full-board search/focus = focus:%q filter:%q matches:%d", model.boardFocusID, model.filter, len(model.boardItems()))
	}

	model.view.SortByFields = []github.SortField{{Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "ASC"}}
	sortedLatencies := make([]time.Duration, 0, 40)
	for index := 0; index < 30; index++ {
		started := time.Now()
		key := "k"
		if index%2 == 1 {
			key = "j"
		}
		updated, _ := model.Update(keyPress(key))
		model = updated.(Model)
		_ = model.View().Content
		sortedLatencies = append(sortedLatencies, time.Since(started))
	}
	sortedP95, sortedMax := latencyPercentiles(sortedLatencies)
	t.Logf("full 1000-card input+view: position p95=%s max=%s; saved-title-sort p95=%s max=%s", positionP95, positionMax, sortedP95, sortedMax)
	if positionP95 >= 100*time.Millisecond || sortedP95 >= 100*time.Millisecond {
		t.Fatalf("full-board p95 above 100ms target: position=%s sorted=%s", positionP95, sortedP95)
	}
	if model.boardFocusID != "item-0999" || source.calls != 10 || source.detailCalls != 1 {
		t.Fatalf("full-board focus/request counts = focus:%q pages:%d details:%d", model.boardFocusID, source.calls, source.detailCalls)
	}
}

func latencyPercentiles(latencies []time.Duration) (time.Duration, time.Duration) {
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	return latencies[(len(latencies)*95+99)/100-1], latencies[len(latencies)-1]
}

func BenchmarkLargeBoardNavigation(b *testing.B) {
	for _, count := range []int{100, 1000} {
		for _, savedSort := range []bool{false, true} {
			name := fmt.Sprintf("cards-%d/position", count)
			if savedSort {
				name = fmt.Sprintf("cards-%d/title-sort", count)
			}
			b.Run(name, func(b *testing.B) {
				source := newDelayedItemsSource(count, 100, 0)
				model := NewModel(source)
				model.screen = screenBoard
				model.view = &github.View{Name: "Performance fixture", Layout: github.BoardLayout}
				if savedSort {
					model.view.SortByFields = []github.SortField{{Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "ASC"}}
				}
				model.selectedOwner = &github.Owner{Login: "fixture-org", Kind: github.OrganizationOwner}
				model.selectedProject = &github.Project{Number: 99}
				model.width, model.height = 123, 63
				for index := 0; index < count; index += 100 {
					cursor := ""
					if index > 0 {
						cursor = fmt.Sprintf("page-%d", index)
					}
					model.items = append(model.items, source.pages[cursor].Items...)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if index%2 == 0 {
						model.moveBoardCard(1)
					} else {
						model.moveBoardCard(-1)
					}
					_ = model.View().Content
				}
			})
		}
	}
}
