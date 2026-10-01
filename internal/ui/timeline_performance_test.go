package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// Invented dates/IDs only. Logical source calls are measured separately from
// network latency or GraphQL request counts, which this fixture cannot establish.
func largeTimelineModel(total int, sorted bool) (Model, *delayedItemsSource) {
	m, _ := liveTimelineFixture()
	source := newDelayedItemsSource(total, 100, 0)
	for _, page := range source.pages {
		for i := range page.Items {
			item := &page.Items[i]
			var index int
			fmt.Sscanf(item.ID, "item-%d", &index)
			if index%20 != 0 {
				date := time.Date(2024, time.January, 1+index%90, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				item.FieldValues = []github.FieldValue{{FieldID: "start-id", Value: date, Available: true}, {FieldID: "target-id", Value: date, Available: true}}
			}
		}
	}
	m.source = source
	m.width, m.height = 123, 40
	if sorted {
		m.view.Filter = "is:issue"
		m.view.SortByFields = []github.SortField{{Field: m.view.ProjectFields[0], Direction: "ASC"}}
	}
	return m, source
}

func TestLargeTimelineProgressiveReadsAndInputResponsiveness(t *testing.T) {
	for _, sorted := range []bool{false, true} {
		t.Run(fmt.Sprintf("sorted-%t", sorted), func(t *testing.T) {
			m, source := largeTimelineModel(10000, sorted)
			pageGate, detailGate := make(chan struct{}), make(chan struct{})
			source.pageGate, source.detailGate = pageGate, detailGate
			defer func() { m.abandonReads() }()
			started := time.Now()
			updated, next := m.Update(m.startItemsLoad()())
			m = updated.(Model)
			firstLatency := time.Since(started)
			if next == nil || !m.itemsLoading || len(m.items) != 100 || source.calls != 1 {
				t.Fatal("first page did not render independently")
			}
			if display := string(m.View().Content); !strings.Contains(display, "Loaded 100") || !strings.Contains(display, "loading") {
				t.Fatal("progressive count missing")
			}
			pageResult := make(chan tea.Msg, 1)
			go func() { pageResult <- next() }()
			updated, debounce := m.Update(keyPress("enter"))
			m = updated.(Model)
			if debounce == nil {
				t.Fatal("detail did not start")
			}
			updated, detailCmd := m.Update(debounce())
			m = updated.(Model)
			if detailCmd == nil {
				t.Fatal("detail did not pass debounce")
			}
			detailResult := make(chan tea.Msg, 1)
			go func() { detailResult <- detailCmd() }()
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(Model)
			latencies := []time.Duration{}
			measure := func(key string) {
				begin := time.Now()
				updated, _ := m.Update(keyPress(key))
				m = updated.(Model)
				_ = m.View().Content
				latencies = append(latencies, time.Since(begin))
			}
			for i := 0; i < 40; i++ {
				measure("j")
				measure("k")
			}
			measure("/")
			for _, key := range []string{"L", "a", "r", "g", "e", "enter"} {
				measure(key)
			}
			if m.filter != "Large" {
				t.Fatal("search input was not handled while reads waited")
			}
			updated, _ = m.Update(keyPress("/"))
			m = updated.(Model)
			updated, _ = m.Update(keyPress("esc"))
			m = updated.(Model)
			selected, _ := m.selectedItem()
			focus := selected.ID
			close(pageGate)
			close(detailGate)
			updated, next = m.Update(<-pageResult)
			m = updated.(Model)
			updated, _ = m.Update(<-detailResult)
			m = updated.(Model)
			if m.detail != nil {
				t.Fatal("closed detail accepted obsolete result")
			}
			for steps := 0; next != nil; steps++ {
				if steps >= 100 {
					t.Fatal("pagination failed to settle")
				}
				updated, next = m.Update(next())
				m = updated.(Model)
			}
			selected, _ = m.selectedItem()
			if len(m.items) != 10000 || m.itemsLoading || source.calls != 100 || source.detailCalls != 1 || selected.ID != focus {
				t.Fatalf("settled state: items=%d pages=%d details=%d focus=%s want=%s", len(m.items), source.calls, source.detailCalls, selected.ID, focus)
			}
			m.tableFocusID = "item-9999"
			if !strings.Contains(string(m.View().Content), "Large fixture item 9999") {
				t.Fatal("far selection fell outside row window")
			}
			full := []time.Duration{}
			for i := 0; i < 30; i++ {
				begin := time.Now()
				updated, _ = m.Update(keyPress("k"))
				m = updated.(Model)
				_ = m.View().Content
				full = append(full, time.Since(begin))
			}
			p95, maxLatency := latencyPercentiles(latencies)
			fullP95, fullMax := latencyPercentiles(full)
			t.Logf("invented 10000-row fixture, 123x40: first page=%s; input+render while page/detail reads wait p95=%s max=%s; fully loaded p95=%s max=%s; logical source calls: pages=%d detail=%d", firstLatency, p95, maxLatency, fullP95, fullMax, source.calls, source.detailCalls)
		})
	}
}

func BenchmarkTimelineModelNavigation(b *testing.B) {
	for _, total := range []int{1000, 10000} {
		for _, sorted := range []bool{false, true} {
			for _, search := range []bool{false, true} {
				b.Run(fmt.Sprintf("rows-%d/sorted-%t/search-%t", total, sorted, search), func(b *testing.B) {
					m, source := largeTimelineModel(total, sorted)
					for after := ""; ; {
						page := source.pages[after]
						m.items = append(m.items, page.Items...)
						if !page.HasNext {
							break
						}
						after = page.EndCursor
					}
					m.tableFocusID = "item-0999"
					if search {
						m.filter = "Large fixture item 09"
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						m.moveTableRow(1 - 2*(i%2))
						_ = m.View().Content
					}
				})
			}
		}
	}
}
