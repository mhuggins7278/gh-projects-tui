package ui

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// Compare cached keys with the prior comparison-based sort, including stable
// ties, unset values, and numeric edge cases in both directions.
func TestSortKeysPreserveComparisonOrder(t *testing.T) {
	for _, typ := range []string{"TITLE", "NUMBER", "TEXT", "SINGLE_SELECT", "ITERATION"} {
		field := github.Field{ID: "f", Name: "Custom", DataType: typ, Options: []github.FieldOption{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, Iterations: []github.Iteration{{ID: "a", Title: "First", StartDate: "2026-01-01"}, {ID: "b", Title: "Second", StartDate: "2026-02-01"}}}
		if typ == "TITLE" {
			field.Name = "Title"
		}
		for _, dir := range []string{"ASC", "DESC"} {
			for seed := int64(0); seed < 40; seed++ {
				rng := rand.New(rand.NewSource(seed))
				view := &github.View{SortByFields: []github.SortField{{Field: field, Direction: dir}, {Field: github.Field{Name: "Title", DataType: "TITLE"}, Direction: "DESC"}}}
				var items []github.Item
				for i := 0; i < 150; i++ {
					v := []string{"", "9", "12", "-1.5", "invalid", "NaN", "Hello", "hello", "ÉTÉ", "a", "b"}[rng.Intn(11)]
					item := github.Item{ID: fmt.Sprint(i), Content: &github.Content{Title: fmt.Sprintf("Title %d", rng.Intn(12))}, FieldValues: []github.FieldValue{{FieldID: "f", Value: v, OptionID: v, IterationID: v, Available: rng.Intn(5) != 0}}}
					if rng.Intn(10) == 0 {
						item.Content = nil
					}
					items = append(items, item)
				}
				got, want := sortedItemsForView(view, items), reviewReferenceSorted(view, items)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("sort differs: type=%s direction=%s seed=%d", typ, dir, seed)
				}
			}
		}
	}
}

func reviewReferenceSorted(view *github.View, items []github.Item) []github.Item {
	ordered := append([]github.Item(nil), items...)
	if view == nil || positionOnly(view) || !sortFieldsSupported(view) {
		return ordered
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		for _, field := range view.SortByFields {
			comparison := reviewReferenceCompare(field.Field, ordered[left], ordered[right], strings.EqualFold(field.Direction, "DESC"))
			if comparison == 0 {
				continue
			}
			return comparison < 0
		}
		return false
	})
	return ordered
}

func reviewReferenceCompare(field github.Field, left, right github.Item, descending bool) int {
	leftValue := itemSortValueFor(field, left)
	rightValue := itemSortValueFor(field, right)
	if !leftValue.present && !rightValue.present {
		return 0
	}
	// GitHub keeps unset values together after populated values in a board
	// sort; stable sorting preserves their existing project-position order.
	if !leftValue.present {
		return 1
	}
	if !rightValue.present {
		return -1
	}
	comparison := 0
	if leftValue.byText || rightValue.byText {
		comparison = strings.Compare(strings.ToLower(leftValue.text), strings.ToLower(rightValue.text))
	} else if leftValue.number < rightValue.number {
		comparison = -1
	} else if leftValue.number > rightValue.number {
		comparison = 1
	}
	if descending {
		return -comparison
	}
	return comparison
}

func TestSharedMarkdownRendererPreservesIndependentDocuments(t *testing.T) {
	bodies := []string{
		"plain text with Unicode 界 and **bold**",
		"# Header\n\n[one](https://example.com) [two][ref]\n\n[ref]: https://example.org",
		"[ref] without a definition in this document",
		"| A | B |\n|---|---|\n| x | y |",
		"```go\nfmt.Println(\"Hello\")\n```",
		"> quote\n\n- one\n- two\n\n### Heading",
		"", "\r\n", "  ",
	}
	for _, width := range []int{0, 4, 17, 80, 140} {
		renderer := newMarkdownRenderer(width)
		for _, body := range bodies {
			if got, want := renderer.render(body), renderMarkdown(body, width); !reflect.DeepEqual(got, want) {
				t.Fatalf("width %d body %q: reused rendering differs", width, body)
			}
		}
	}
}

func TestLocalSearchPreservesSortAndInputItems(t *testing.T) {
	m := benchmarkSortedBoardModel(200, false)
	original := cloneItems(m.items)
	for _, dir := range []string{"ASC", "DESC"} {
		m.view.SortByFields[0].Direction = dir
		for _, filter := range []string{"", "  ", "needle", "mixed CASE", "absent"} {
			m.filter = filter
			var want []github.Item
			for _, item := range reviewReferenceSorted(m.view, m.items) {
				if itemMatchesBoardSearch(item, filter) {
					want = append(want, item)
				}
			}
			got := m.boardItems()
			if len(got) != len(want) {
				t.Fatalf("%s %q: match count differs", dir, filter)
			}
			for i, item := range got {
				if item.ID != want[i].ID {
					t.Fatalf("%s %q: match ordering differs", dir, filter)
				}
			}
			if !reflect.DeepEqual(m.items, original) {
				t.Fatal("search/sort mutated source items")
			}
			m.boardLane, m.boardCard = 0, 0
			m.moveBoardCard(1)
			if len(got) > 1 && m.boardFocusID != got[1].ID {
				t.Fatal("navigation focus no longer follows filtered order")
			}
		}
	}
}
