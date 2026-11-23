package ui

import (
	"context"
	"io"
	"reflect"
	"testing"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

type fakeBoardItemsReader struct {
	batches []github.ItemsBatch
	next    int
	closed  int
	ctx     context.Context
}

func (r *fakeBoardItemsReader) Next() (github.ItemsBatch, error) {
	if r.ctx != nil && r.ctx.Err() != nil {
		return github.ItemsBatch{}, r.ctx.Err()
	}
	if r.next >= len(r.batches) {
		return github.ItemsBatch{}, io.EOF
	}
	batch := r.batches[r.next]
	r.next++
	return batch, nil
}

func (r *fakeBoardItemsReader) Close() { r.closed++ }

type streamedBoardSource struct {
	fakePickerSource
	readers []*fakeBoardItemsReader
	opened  int
	ctxs    []context.Context
	filters []string
	fields  [][]github.Field
}

func (s *streamedBoardSource) OpenBoardItems(ctx context.Context, _ github.Owner, _ int, filter string, fields []github.Field) github.BoardItemsReader {
	reader := s.readers[s.opened]
	reader.ctx = ctx
	s.opened++
	s.ctxs = append(s.ctxs, ctx)
	s.filters = append(s.filters, filter)
	s.fields = append(s.fields, fields)
	return reader
}

func readerItem(id string) github.Item {
	return github.Item{
		ID: id, Content: &github.Content{ID: "issue-" + id, Kind: "Issue", Title: id},
		FieldValues: []github.FieldValue{{FieldID: "status", OptionID: "a", Available: true}},
	}
}

func TestBoardAndTableUseOrderedReaderAndReplaceFallbackPrefix(t *testing.T) {
	for _, layout := range []github.ViewLayout{github.BoardLayout, github.TableLayout} {
		t.Run(string(layout), func(t *testing.T) {
			reader := &fakeBoardItemsReader{batches: []github.ItemsBatch{
				{Items: []github.Item{readerItem("one"), readerItem("two")}, HasNext: true},
				{Items: []github.Item{readerItem("one")}, HasNext: true, Reset: true},
				{Items: []github.Item{readerItem("two"), readerItem("three")}},
			}}
			source := &streamedBoardSource{readers: []*fakeBoardItemsReader{reader}}
			m := tableActionModel(source)
			m.view.Layout, m.view.Filter = layout, "is:open"
			wantFields := boardReadFields(*m.view)
			cmd := m.startItemsLoad()
			updated, next := m.Update(cmd())
			m = updated.(Model)
			if len(m.items) != 2 || !m.itemsLoading || next == nil {
				t.Fatal("first reader batch was not displayed progressively")
			}
			m.boardFocusID, m.boardCard = "two", 1
			m.tableFocusID, m.tableRow = "two", 1
			updated, next = m.Update(next())
			m = updated.(Model)
			if len(m.items) != 1 || m.items[0].ID != "one" || !m.itemsLoading {
				t.Fatal("safe fallback appended to, rather than replaced, the old prefix")
			}
			updated, _ = m.Update(next())
			m = updated.(Model)
			if len(m.items) != 3 || m.itemsLoading || m.itemsReader != nil || reader.closed == 0 {
				t.Fatal("reader completion did not close the load")
			}
			if item, ok := m.selectedItem(); !ok || item.ID != "two" {
				t.Fatal("safe fallback lost selection identity")
			}
			if source.opened != 1 || !reflect.DeepEqual(source.filters, []string{"is:open"}) || !reflect.DeepEqual(source.fields[0], wantFields) {
				t.Fatal("reader lost the saved filter or requested fields")
			}
		})
	}
}

func TestObsoleteReaderResponseCannotClearNewReader(t *testing.T) {
	first := &fakeBoardItemsReader{batches: []github.ItemsBatch{{Items: []github.Item{readerItem("old")}, HasNext: true}}}
	second := &fakeBoardItemsReader{batches: []github.ItemsBatch{{Items: []github.Item{readerItem("new")}}}}
	source := &streamedBoardSource{readers: []*fakeBoardItemsReader{first, second}}
	m := tableActionModel(source)
	firstCmd := m.startItemsLoad()
	message := firstCmd()
	secondCmd := m.startItemsLoad()
	updated, _ := m.Update(message)
	m = updated.(Model)
	if len(m.items) != 0 || m.itemsReader != second || first.closed == 0 || source.ctxs[0].Err() != context.Canceled {
		t.Fatal("obsolete result survived or cleared the new reader")
	}
	updated, _ = m.Update(secondCmd())
	m = updated.(Model)
	if len(m.items) != 1 || m.items[0].ID != "new" {
		t.Fatal("new reader could not finish after an obsolete response")
	}
}

func TestMutationRefreshUsesReaderAndDiscardsAbandonedPrefix(t *testing.T) {
	reader := &fakeBoardItemsReader{batches: []github.ItemsBatch{
		{Items: []github.Item{readerItem("abandoned")}, HasNext: true},
		{Items: []github.Item{readerItem("one")}, HasNext: true, Reset: true},
		{Items: []github.Item{readerItem("two")}},
	}}
	source := &streamedBoardSource{readers: []*fakeBoardItemsReader{reader}}
	m := tableActionModel(source)
	message := m.startMutationViewRefresh()().(mutationViewRefreshMsg)
	if message.err != nil || len(message.items) != 2 || message.items[0].ID != "one" || message.items[1].ID != "two" || reader.closed == 0 {
		t.Fatal("post-action refresh did not use the safe ordered reader")
	}
}

func TestNavigationCancelsReadOnlyPostActionRefresh(t *testing.T) {
	reader := &fakeBoardItemsReader{batches: []github.ItemsBatch{{Items: []github.Item{readerItem("old")}}}}
	source := &streamedBoardSource{readers: []*fakeBoardItemsReader{reader}}
	m := tableActionModel(source)
	command := m.startMutationViewRefresh()
	m.abandonReads()
	message := command().(mutationViewRefreshMsg)
	if message.err != context.Canceled || reader.next != 0 || reader.closed == 0 {
		t.Fatal("post-action read survived navigation cancellation")
	}
}
