package ui

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

func syntheticTimelineRow(id, start, target string) dateTimelineRow {
	return dateTimelineRow{Item: github.Item{ID: id, Content: &github.Content{Kind: "Issue", Title: id}}, Start: dateTimelineEndpoint{Value: start, Available: true}, Target: dateTimelineEndpoint{Value: target, Available: true}}
}

// These invented dates encode the observed inclusive DATE and one-day rules,
// without retaining the public sample's dates or item metadata. They verify the
// coarse terminal buckets, not pixel-for-pixel GitHub display parity.
func TestTimelineObservedDatePlacementRules(t *testing.T) {
	for _, tc := range []struct {
		name, month, start, target, prefix string
	}{
		{"month boundary", "2028-01", "2028-01-28", "2028-02-07", strings.Repeat("·", 27) + "●━━━|━━━━━━●"},
		{"year boundary", "2028-12", "2028-12-31", "2029-01-01", strings.Repeat("·", 30) + "●|●"},
		{"same day", "2028-12", "2028-12-31", "2028-12-31", strings.Repeat("·", 30) + "●|"},
		{"start only", "2028-12", "2028-12-31", "", strings.Repeat("·", 30) + "●|"},
		{"target only", "2028-12", "", "2028-12-31", strings.Repeat("·", 30) + "●|"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calendar, err := newTimelineCalendar(tc.month, 95)
			if err != nil {
				t.Fatal(err)
			}
			row := syntheticTimelineRow("invented", tc.start, tc.target)
			bar := calendar.bar(timelineSpan(row, calendar))
			if !strings.HasPrefix(bar, tc.prefix) {
				t.Fatalf("placement=%s, want prefix=%s", bar, tc.prefix)
			}
			if tc.name == "same day" || tc.name == "start only" || tc.name == "target only" {
				if strings.Count(bar, "●") != 1 || strings.Contains(bar, "━") {
					t.Fatalf("one-day placement acquired a duration: %s", bar)
				}
			}
		})
	}
	calendar, _ := newTimelineCalendar("2028-01", 95)
	if got := calendar.bar(timelineSpan(syntheticTimelineRow("invented", "", ""), calendar)); got != "Undated" {
		t.Fatalf("unset endpoints acquired placement: %s", got)
	}
}

func TestTimelineDateClassificationAndClipping(t *testing.T) {
	calendar, err := newTimelineCalendar("2024-01", 92)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		start, target, status string
		left, right           bool
	}{
		{"2024-01-01", "2024-03-31", "Dated", false, false},
		{"2024-02-29", "2024-02-29", "Dated", false, false},
		{"2023-12-31", "2024-01-02", "Dated", true, false},
		{"2024-03-31", "2024-04-01", "Dated", false, true},
		{"2020-01-01", "2030-01-01", "Dated", true, true},
		{"2023-12-01", "2023-12-31", "Before range", false, false},
		{"2024-04-01", "2024-04-02", "After range", false, false},
		{"2024-01-02", "", "Dated", false, false},
		{"", "2024-01-02", "Dated", false, false},
		{"", "2024-04-01", "After range", false, false},
		{"", "", "Undated", false, false},
		{"2024-03-01", "2024-02-29", "Invalid dates", false, false},
		{"2023-02-29", "2024-01-01", "Invalid dates", false, false},
		{"2024-01-01T00:00:00Z", "", "Invalid dates", false, false},
		{"0000-01-01", "", "Invalid dates", false, false},
		{"bad", "", "Invalid dates", false, false},
	} {
		t.Run(tc.start+"/"+tc.target, func(t *testing.T) {
			span := timelineSpan(syntheticTimelineRow("row", tc.start, tc.target), calendar)
			if span.status != tc.status {
				t.Fatalf("status=%s want=%s", span.status, tc.status)
			}
			bar := calendar.bar(span)
			if tc.status == "Dated" {
				if ansi.StringWidth(bar) != 92 {
					t.Fatalf("axis width=%d", ansi.StringWidth(bar))
				}
				if strings.HasPrefix(bar, "<") != tc.left || strings.HasSuffix(bar, ">") != tc.right {
					t.Fatalf("clipping=%s", bar)
				}
				if !strings.ContainsAny(bar, "●<>") {
					t.Fatal("bar/point has no endpoint marker")
				}
				if span.first.Equal(span.last) && (strings.Count(bar, "●") != 1 || strings.Contains(bar, "━")) {
					t.Fatal("single-day or partial endpoints acquired a duration")
				}
			} else if bar != tc.status {
				t.Fatalf("invalid/unplaced row acquired a bar: %s", bar)
			}
		})
	}
	row := syntheticTimelineRow("row", "2024-01-01", "")
	row.Target.Available = false
	if span := timelineSpan(row, calendar); span.status != "Unavailable" {
		t.Fatal("unavailable date was treated as unset")
	}
	row.Target.Available = true
	row.Item.Content = nil
	if span := timelineSpan(row, calendar); span.status != "Unavailable" {
		t.Fatal("inaccessible content was treated as undated")
	}
}

func TestTimelineCalendarBucketsAndBoundaries(t *testing.T) {
	for _, tc := range []struct{ month, end string }{
		{"2023-12", "2024-03-01"}, {"2024-01", "2024-04-01"}, {"2026-02", "2026-05-01"}, {"0001-01", "0001-04-01"}, {"9999-10", "10000-01-01"},
	} {
		calendar, err := newTimelineCalendar(tc.month, 92)
		if err != nil {
			t.Fatal(err)
		}
		if calendar.end.Format("2006-01-02") != tc.end || calendar.start.Location() != time.UTC {
			t.Fatalf("calendar=%v", calendar)
		}
		previous := -1
		for day := calendar.start; day.Before(calendar.end); day = day.AddDate(0, 0, 1) {
			index := calendar.cell(day)
			if index < previous || index < 0 || index >= calendar.axisWidth {
				t.Fatalf("nonmonotonic bucket for %s: %d", day.Format("2006-01-02"), index)
			}
			previous = index
		}
	}
	calendar, _ := newTimelineCalendar("0001-01", 92)
	span := timelineSpan(syntheticTimelineRow("first", "0001-01-01", "0001-01-03"), calendar)
	if span.status != "Dated" || span.first.Day() != 1 || span.last.Day() != 3 {
		t.Fatal("year-one date was treated as an unset zero value")
	}
	for _, month := range []string{"", "2026-13", "2026-2", "0000-01", "9999-11", "2026-09\n"} {
		if _, err := newTimelineCalendar(month, 92); err == nil {
			t.Fatalf("bad month accepted: %q", month)
		}
	}
}

func TestTimelinePreservesOrderAndSelectsEveryClassification(t *testing.T) {
	rows := []dateTimelineRow{
		syntheticTimelineRow("z-last-date", "2024-03-12", "2024-03-18"),
		syntheticTimelineRow("a-first-date", "2024-01-01", "2024-01-04"),
		syntheticTimelineRow("outside-item", "2025-01-01", "2025-02-01"),
		syntheticTimelineRow("undated-item", "", ""),
		syntheticTimelineRow("unavailable-item", "", ""),
		syntheticTimelineRow("reversed", "2024-02-01", "2024-01-01"),
	}
	rows[4].Start.Available = false
	rows[1].Item.Content.Kind = "PullRequest"
	rows[2].Item.Content.Kind = "DraftIssue"
	original := append([]dateTimelineRow(nil), rows...)
	view := dateTimelineViewport{Month: "2024-01", Width: 130, Height: 20, Loading: true}
	content := renderDateTimeline(rows, view)
	for _, text := range []string{"Jan 2024", "Feb 2024", "Mar 2024", "Loaded 6", "outside 1", "undated 1", "unavailable 1", "invalid 1", "loading", "read-only"} {
		if !strings.Contains(content, text) {
			t.Fatalf("missing %q:\n%s", text, content)
		}
	}
	previous := -1
	for _, row := range rows {
		index := strings.Index(content, row.Item.Content.Title)
		if index < 0 || index < previous {
			t.Fatal("renderer changed caller row order")
		}
		previous = index
		for _, height := range []int{1, 3, 6, 8} {
			view.Height = height
			view.SelectedID = row.Item.ID
			selected := renderDateTimeline(rows, view)
			if !strings.Contains(selected, "> "+row.Item.Content.Title) {
				t.Fatalf("selection hidden at height %d:\n%s", height, selected)
			}
			if len(strings.Split(selected, "\n")) > height {
				t.Fatal("row window exceeded height")
			}
		}
	}
	if !reflect.DeepEqual(rows, original) {
		t.Fatal("renderer mutated inputs")
	}
	view.Width = 48
	view.Height = 12
	view.SelectedID = "a-first-date"
	fallback := renderDateTimeline(rows, view)
	for _, text := range []string{"Date list", "> a-first-date", "Start: 2024-01-01", "Target: 2024-01-04"} {
		if !strings.Contains(fallback, text) {
			t.Fatalf("fallback missing %q:\n%s", text, fallback)
		}
	}
}

func TestTimelineFitsNarrowAndShortTerminals(t *testing.T) {
	rows := []dateTimelineRow{syntheticTimelineRow("界界界界界界 title\n\x1b[31mcolored\x1b[0m", "2024-01-01", "2024-03-31")}
	for _, width := range []int{0, 1, 8, 24, 48, 59, 60, 80, 120, 250} {
		for _, height := range []int{0, 1, 2, 3, 6, 10} {
			content := renderDateTimeline(rows, dateTimelineViewport{Month: "2024-01", Width: width, Height: height})
			if width == 0 || height == 0 {
				if content != "" {
					t.Fatal("zero dimensions rendered output")
				}
				continue
			}
			lines := strings.Split(content, "\n")
			if len(lines) > height {
				t.Fatalf("%dx%d rendered %d lines", width, height, len(lines))
			}
			for _, line := range lines {
				if ansi.StringWidth(line) > width {
					t.Fatalf("line exceeded width %d: %q", width, line)
				}
			}
			if strings.Contains(content, "\x1b") {
				t.Fatal("item title injected terminal controls")
			}
		}
	}
}

func TestTimelineEmptyAndInvalidViewport(t *testing.T) {
	for _, height := range []int{1, 2, 3, 6} {
		content := renderDateTimeline(nil, dateTimelineViewport{Month: "2026-09", Width: 80, Height: height})
		if len(strings.Split(content, "\n")) > height {
			t.Fatal("empty viewport exceeded height")
		}
	}
	content := renderDateTimeline(nil, dateTimelineViewport{Month: "bad", Width: 80, Height: 10})
	if !strings.Contains(content, "Timeline unavailable") {
		t.Fatal("invalid viewport acquired default dates")
	}
}

func BenchmarkDateTimelineLargeFixture(b *testing.B) {
	rows := make([]dateTimelineRow, 10000)
	for i := range rows {
		rows[i] = syntheticTimelineRow(fmt.Sprintf("fixture-%d", i), "2024-01-01", "2024-03-01")
	}
	view := dateTimelineViewport{Month: "2024-01", Width: 120, Height: 30, SelectedID: "fixture-9999"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderDateTimeline(rows, view)
	}
}

func TestTimelineLargeFixtureKeepsSelectionWhilePaging(t *testing.T) {
	rows := make([]dateTimelineRow, 10000)
	for i := range rows {
		rows[i] = syntheticTimelineRow(fmt.Sprintf("fixture-%d", i), "2024-01-01", "2024-03-01")
	}
	view := dateTimelineViewport{Month: "2024-01", Width: 120, Height: 12, SelectedID: "fixture-98"}
	view.Loading = true
	firstPage := renderDateTimeline(rows[:100], view)
	view.Loading = false
	complete := renderDateTimeline(rows, view)
	for _, text := range []string{firstPage, complete} {
		if !strings.Contains(text, "> fixture-98 ") || len(strings.Split(text, "\n")) > view.Height {
			t.Fatal("paging hid selection or exceeded the row window")
		}
	}
	if !strings.Contains(firstPage, "Loaded 100 ·") || !strings.Contains(firstPage, "loading") || !strings.Contains(complete, "Loaded 10000 ·") || strings.Contains(complete, "loading") {
		t.Fatal("counts did not distinguish progressive loaded items from the complete input")
	}
	for _, row := range []dateTimelineRow{syntheticTimelineRow("start-only", "2024-01-01", ""), syntheticTimelineRow("target-only", "", "2024-01-01")} {
		text := renderDateTimeline([]dateTimelineRow{row}, view)
		if !strings.Contains(text, "unset") || !strings.Contains(text, "2024-01-01") {
			t.Fatal("exact endpoint display invented a missing endpoint")
		}
	}
}

func TestTimelineSyntheticPreview(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_TIMELINE_PREVIEW") != "1" {
		t.Skip("opt-in synthetic terminal preview")
	}
	rows := []dateTimelineRow{
		syntheticTimelineRow("Issue: crosses left boundary", "2023-12-20", "2024-01-18"),
		syntheticTimelineRow("PR: February leap-day point", "", "2024-02-29"),
		syntheticTimelineRow("Draft: spans whole viewport", "2023-12-01", "2024-04-15"),
		syntheticTimelineRow("Outside viewport", "2025-01-01", "2025-03-01"),
		syntheticTimelineRow("Undated work", "", ""),
		syntheticTimelineRow("Unavailable field", "", ""),
		syntheticTimelineRow("Invalid/reversed endpoints", "2024-03-01", "2024-01-01"),
	}
	rows[5].Start.Available = false
	rows[1].Item.Content.Kind = "PullRequest"
	rows[2].Item.Content.Kind = "DraftIssue"
	for _, width := range []int{120, 48} {
		t.Logf("\n%d-column synthetic preview:\n%s", width, renderDateTimeline(rows, dateTimelineViewport{Month: "2024-01", Width: width, Height: 14, SelectedID: rows[3].Item.ID}))
	}
}
