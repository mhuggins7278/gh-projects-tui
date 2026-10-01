package ui

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func syntheticTimelineSections() []dateTimelineSection {
	return []dateTimelineSection{
		{ID: "ready", Name: "Ready", Rows: []dateTimelineRow{syntheticTimelineRow("late-first", "2024-03-01", "2024-03-10"), syntheticTimelineRow("early-second", "2024-01-01", "")}},
		{ID: "empty", Name: "Empty"},
		{ID: "unset", Name: "No value", Rows: []dateTimelineRow{syntheticTimelineRow("undated", "", ""), syntheticTimelineRow("outside", "2025-01-01", "")}},
	}
}

func TestGroupedTimelineKeepsCallerOrderCountsAndEveryItem(t *testing.T) {
	sections := syntheticTimelineSections()
	before := syntheticTimelineSections()
	content := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: 120, Height: 30, SelectedID: "outside", Loading: true})
	for _, want := range []string{"Synthetic groups", "collapsed/custom order unavailable", "Loaded 4", "groups 3", "outside 1", "undated 1", "loading", "[Ready] · 2 loaded", "[Empty] · 0 loaded", "[No value] · 2 loaded", "Start: 2025-01-01", "Target: unset"} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q: %s", want, content)
		}
	}
	previous := -1
	for _, want := range []string{"[Ready]", "late-first", "early-second", "[Empty]", "[No value]", "undated", "outside"} {
		index := strings.Index(content, want)
		// Classification counts use 'undated'/'outside'; compare actual row lines.
		if want == "undated" {
			index = strings.Index(content, "  undated ")
		}
		if want == "outside" {
			index = strings.Index(content, "> outside ")
		}
		if index <= previous {
			t.Fatalf("caller order lost at %s: %s", want, content)
		}
		previous = index
	}
	if strings.Count(content, "> ") != 1 || !reflect.DeepEqual(sections, before) {
		t.Fatal("selection duplicated or input mutated")
	}
}

func TestGroupedTimelineCarriesHeadingWithoutExceedingHeight(t *testing.T) {
	sections := []dateTimelineSection{{ID: "group", Name: "A long section"}}
	for i := 0; i < 100; i++ {
		sections[0].Rows = append(sections[0].Rows, syntheticTimelineRow(fmt.Sprintf("row-%03d", i), "2024-02-01", ""))
	}
	for _, width := range []int{48, 100} {
		for _, height := range []int{4, 5, 8, 12} {
			content := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: width, Height: height, SelectedID: "row-090"})
			if len(strings.Split(content, "\n")) > height || !strings.Contains(content, "> row-090") || !strings.Contains(content, "[A long section]") || !strings.Contains(content, "100 loaded") {
				t.Fatalf("lost carried heading/selection at %dx%d: %s", width, height, content)
			}
			if strings.Count(content, "[A long section]") != 1 || strings.Contains(content, "row-000") {
				t.Fatal("heading duplicated or row window not bounded")
			}
		}
	}
}

func TestGroupedTimelineSelectionSurvivesPagesSearchAndSectionChanges(t *testing.T) {
	sections := syntheticTimelineSections()
	view := dateTimelineViewport{Month: "2024-01", Width: 100, Height: 12, SelectedID: "early-second", Loading: true}
	assertSelected := func(groups []dateTimelineSection) {
		t.Helper()
		content := renderGroupedDateTimeline(groups, view)
		if !strings.Contains(content, "> early-second") || strings.Count(content, "> ") != 1 || !strings.Contains(content, "Start: 2024-01-01") {
			t.Fatalf("identity lost: %s", content)
		}
	}
	assertSelected(sections)
	for i := 0; i < 100; i++ {
		sections[0].Rows = append(sections[0].Rows, syntheticTimelineRow(fmt.Sprintf("page-%d", i), "2024-02-01", ""))
	}
	view.Loading = false
	assertSelected(sections)
	// Search and sorted/grouped caller projections can change row/section indexes;
	// renderer selection remains by ID rather than selecting a heading.
	search := []dateTimelineSection{{ID: "ready", Name: "Ready", Rows: []dateTimelineRow{sections[0].Rows[1]}}}
	assertSelected(search)
	sections[2].Rows = append(sections[2].Rows, sections[0].Rows[1])
	sections[0].Rows = append(sections[0].Rows[:1], sections[0].Rows[2:]...)
	sections[0], sections[2] = sections[2], sections[0]
	assertSelected(sections)
}

func TestGroupedTimelineAllDimensionsAndClassifications(t *testing.T) {
	rows := []dateTimelineRow{
		syntheticTimelineRow("clipped", "2023-12-01", "2024-04-01"),
		syntheticTimelineRow("outside", "2025-01-01", ""),
		syntheticTimelineRow("undated", "", ""),
		syntheticTimelineRow("invalid", "2024-03-01", "2024-01-01"),
		syntheticTimelineRow("unavailable", "", ""),
	}
	rows[4].Start.Available = false
	sections := []dateTimelineSection{{ID: "unicode", Name: "界界\n\x1b[31mgroup", Rows: rows}}
	for _, width := range []int{0, 1, 4, 20, 48, 60, 120} {
		for height := 0; height < 20; height++ {
			for _, selected := range []string{"clipped", "invalid", "unavailable"} {
				content := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: width, Height: height, SelectedID: selected})
				if width == 0 || height == 0 {
					if content != "" {
						t.Fatal("zero viewport emitted content")
					}
					continue
				}
				lines := strings.Split(content, "\n")
				if len(lines) > height {
					t.Fatalf("height overflow %dx%d: %s", width, height, content)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) > width || strings.Contains(line, "\x1b") {
						t.Fatalf("unsafe/overflow line %dx%d: %q", width, height, line)
					}
				}
				if width >= 48 && height >= 4 && !strings.Contains(content, "> "+selected) {
					t.Fatalf("selected row lost at %dx%d: %s", width, height, content)
				}
			}
		}
	}
	text := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: 120, Height: 20, SelectedID: "clipped"})
	for _, want := range []string{"<", "After range", "Undated", "Invalid dates", "Unavailable"} {
		if !strings.Contains(text, want) {
			t.Fatalf("classification %q missing", want)
		}
	}
	narrow := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: 48, Height: 15, SelectedID: "clipped"})
	if !strings.Contains(narrow, "grouped date list") {
		t.Fatal("narrow fallback missing")
	}
}

func TestGroupedTimelineRejectsAmbiguousIdentityAndHandlesEmptySections(t *testing.T) {
	row := syntheticTimelineRow("same", "", " ")
	for _, sections := range [][]dateTimelineSection{
		{{ID: "", Name: "missing"}},
		{{ID: "same"}, {ID: "same"}},
		{{ID: "first", Rows: []dateTimelineRow{row}}, {ID: "second", Rows: []dateTimelineRow{row}}},
		{{ID: "first", Rows: []dateTimelineRow{syntheticTimelineRow("", "", "")}}},
	} {
		if text := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: 100, Height: 10}); !strings.Contains(text, "unavailable:") {
			t.Fatalf("ambiguous identities silently admitted: %s", text)
		}
	}
	for _, sections := range [][]dateTimelineSection{nil, {{ID: "empty", Name: "Empty"}}} {
		for height := 1; height < 10; height++ {
			text := renderGroupedDateTimeline(sections, dateTimelineViewport{Month: "2024-01", Width: 100, Height: height})
			if len(strings.Split(text, "\n")) > height || strings.Contains(text, "> ") || strings.Contains(text, "Start:") {
				t.Fatal("empty groups invented selection or overflow")
			}
		}
	}
}

func BenchmarkGroupedTimelineLargeFixture(b *testing.B) {
	sections := make([]dateTimelineSection, 100)
	for s := range sections {
		sections[s] = dateTimelineSection{ID: fmt.Sprintf("section-%d", s), Name: fmt.Sprintf("Group %d", s)}
		for i := 0; i < 100; i++ {
			sections[s].Rows = append(sections[s].Rows, syntheticTimelineRow(fmt.Sprintf("group-%d-row-%d", s, i), "2024-01-01", "2024-03-01"))
		}
	}
	view := dateTimelineViewport{Month: "2024-01", Width: 120, Height: 30, SelectedID: "group-99-row-99"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = renderGroupedDateTimeline(sections, view)
	}
}

func TestGroupedTimelineSyntheticPreview(t *testing.T) {
	if os.Getenv("GH_PROJECTS_TUI_TIMELINE_PREVIEW") != "1" {
		t.Skip("opt-in synthetic grouped preview")
	}
	for _, width := range []int{120, 48} {
		t.Logf("\n%d-column grouped synthetic preview:\n%s", width, renderGroupedDateTimeline(syntheticTimelineSections(), dateTimelineViewport{Month: "2024-01", Width: width, Height: 15, SelectedID: "outside", Loading: true}))
	}
}
