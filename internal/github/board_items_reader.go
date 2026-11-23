package github

import (
	"context"
	"fmt"
	"io"
	"slices"
)

// ItemsBatch is an ordered board prefix extension. Reset replaces all previously
// delivered items after an inconsistent optimized read has restarted safely.
type ItemsBatch struct {
	Items   []Item
	HasNext bool
	Reset   bool
}

// BoardItemsReader owns pagination and background reads. Call Next serially;
// Close may be called concurrently and cancels without blocking the UI.
type BoardItemsReader interface {
	Next() (ItemsBatch, error)
	Close()
}

// OpenBoardItems does no network work until Next. Large views read from both
// ends, validating counts and an overlapping join item. An inconsistent read
// restarts once using uncached forward pagination with the saved filter intact.
func (c *Client) OpenBoardItems(ctx context.Context, owner Owner, project int, filter string, fields []Field) BoardItemsReader {
	ctx, cancel := context.WithCancel(ctx)
	return &boardItemsReader{
		client: c, ctx: ctx, cancel: cancel, owner: owner, project: project,
		filter: filter, fields: slices.Clone(fields),
		seen: map[string]bool{}, cursors: map[string]bool{},
	}
}

type boardTail struct {
	pages []ItemsPage
	err   error
}

type boardItemsReader struct {
	client  *Client
	ctx     context.Context
	cancel  context.CancelFunc
	owner   Owner
	project int
	filter  string
	fields  []Field

	started, finished, serial, fresh, reset bool
	total, delivered, frontRemaining        int
	cursor, lastFront                       string
	seen, cursors                           map[string]bool
	tail                                    chan boardTail
	tailDone                                chan struct{}
	tailCancel                              context.CancelFunc
	readyTail                               []ItemsPage
}

func (r *boardItemsReader) Close() { r.cancel() }

func (r *boardItemsReader) Next() (ItemsBatch, error) {
	if r.finished {
		return ItemsBatch{}, io.EOF
	}
	if err := r.ctx.Err(); err != nil {
		return ItemsBatch{}, err
	}
	var batch ItemsBatch
	var err error
	switch {
	case r.serial:
		batch, err = r.nextSerial()
	case !r.started:
		batch, err = r.first()
	default:
		batch, err = r.nextParallel()
	}
	if err != nil && !r.fresh && r.ctx.Err() == nil {
		r.stopTail()
		// Do not leave inconsistent counted pages cached for the next visit.
		r.client.InvalidateReads()
		r.serial, r.fresh, r.reset = true, true, true
		r.cursor, r.delivered = "", 0
		r.seen, r.cursors = map[string]bool{}, map[string]bool{}
		r.readyTail = nil
		batch, err = r.nextSerial()
	}
	if err != nil || !batch.HasNext {
		r.finished = true
		r.cancel()
	}
	return batch, err
}

func (r *boardItemsReader) read(ctx context.Context, cursor string, options selectedItemRead) (selectedItemsPage, error) {
	return r.client.readSelectedItems(ctx, r.owner, r.project, r.filter, cursor, r.fields, true, options)
}

func (r *boardItemsReader) first() (ItemsBatch, error) {
	r.started = true
	page, err := r.read(r.ctx, "", selectedItemRead{count: true})
	if err != nil {
		return ItemsBatch{}, err
	}
	if page.count == nil || *page.count < 0 || len(page.Items) != min(*page.count, 100) || page.HasNext != (*page.count > len(page.Items)) {
		return ItemsBatch{}, fmt.Errorf("board count changed or the first page was incomplete")
	}
	r.total, r.cursor = *page.count, page.EndCursor
	if page.HasNext {
		r.cursors[page.EndCursor] = true
	}
	if r.total < 300 {
		r.serial = true
		return r.deliver(page.Items, page.HasNext)
	}
	remaining := r.total - len(page.Items)
	r.frontRemaining = (remaining + 1) / 2
	r.lastFront = page.Items[len(page.Items)-1].ID
	r.startTail(remaining/2 + 1) // One overlapping item validates the join.
	return r.deliver(page.Items, true)
}

func (r *boardItemsReader) nextSerial() (ItemsBatch, error) {
	page, err := r.read(r.ctx, r.cursor, selectedItemRead{fresh: r.fresh})
	if err != nil {
		return ItemsBatch{}, err
	}
	if page.HasNext {
		if r.cursors[page.EndCursor] {
			return ItemsBatch{}, fmt.Errorf("board pagination cursor repeated")
		}
		r.cursors[page.EndCursor] = true
	}
	r.cursor = page.EndCursor
	return r.deliver(page.Items, page.HasNext)
}

func (r *boardItemsReader) nextParallel() (ItemsBatch, error) {
	if r.frontRemaining > 0 {
		size := min(r.frontRemaining, 100)
		page, err := r.read(r.ctx, r.cursor, selectedItemRead{size: size, count: true})
		if err != nil {
			return ItemsBatch{}, err
		}
		if err := r.validateCounted(page, size); err != nil {
			return ItemsBatch{}, err
		}
		if r.cursors[page.EndCursor] || !page.HasNext {
			return ItemsBatch{}, fmt.Errorf("front pagination changed before the board join")
		}
		r.cursors[page.EndCursor] = true
		r.cursor = page.EndCursor
		r.frontRemaining -= len(page.Items)
		r.lastFront = page.Items[len(page.Items)-1].ID
		return r.deliver(page.Items, true)
	}
	if r.readyTail == nil {
		select {
		case <-r.ctx.Done():
			return ItemsBatch{}, r.ctx.Err()
		case result := <-r.tail:
			if result.err != nil {
				return ItemsBatch{}, result.err
			}
			if err := r.acceptTail(result.pages); err != nil {
				return ItemsBatch{}, err
			}
		}
	}
	if len(r.readyTail) == 0 {
		return ItemsBatch{}, fmt.Errorf("board tail ended before the counted total")
	}
	page := r.readyTail[0]
	r.readyTail = r.readyTail[1:]
	return r.deliver(page.Items, len(r.readyTail) > 0)
}

func (r *boardItemsReader) validateCounted(page selectedItemsPage, size int) error {
	if page.count == nil || *page.count != r.total || len(page.Items) != size {
		return fmt.Errorf("board membership changed during parallel loading")
	}
	return nil
}

func (r *boardItemsReader) startTail(remaining int) {
	ctx, cancel := context.WithCancel(r.ctx)
	r.tailCancel = cancel
	r.tail, r.tailDone = make(chan boardTail, 1), make(chan struct{})
	go func() {
		defer close(r.tailDone)
		result := boardTail{}
		cursor := ""
		cursors := map[string]bool{}
		for remaining > 0 {
			size := min(remaining, 100)
			page, err := r.read(ctx, cursor, selectedItemRead{size: size, backwards: true, count: true})
			if err == nil {
				err = r.validateCounted(page, size)
			}
			if err != nil {
				result.err = err
				break
			}
			if !page.HasNext || cursors[page.EndCursor] {
				result.err = fmt.Errorf("tail pagination changed before the board join")
				break
			}
			cursors[page.EndCursor] = true
			cursor = page.EndCursor
			remaining -= len(page.Items)
			result.pages = append(result.pages, page.ItemsPage)
		}
		slices.Reverse(result.pages)
		r.tail <- result
	}()
}

func (r *boardItemsReader) stopTail() {
	if r.tailCancel != nil {
		r.tailCancel()
		<-r.tailDone
		r.tailCancel = nil
		r.tail, r.tailDone = nil, nil
	}
}

func (r *boardItemsReader) acceptTail(pages []ItemsPage) error {
	if len(pages) == 0 || len(pages[0].Items) == 0 || pages[0].Items[0].ID != r.lastFront {
		return fmt.Errorf("board position changed at the parallel read join")
	}
	pages[0].Items = pages[0].Items[1:]
	if len(pages[0].Items) == 0 {
		pages = pages[1:]
	}
	seen := map[string]bool{}
	count := r.delivered
	for _, page := range pages {
		for _, item := range page.Items {
			if item.ID == "" || seen[item.ID] || r.seen[item.ID] {
				return fmt.Errorf("board membership overlapped during parallel loading")
			}
			seen[item.ID] = true
			count++
		}
	}
	if count != r.total {
		return fmt.Errorf("board total changed during parallel loading")
	}
	r.readyTail = pages
	return nil
}

func (r *boardItemsReader) deliver(items []Item, more bool) (ItemsBatch, error) {
	for _, item := range items {
		if item.ID == "" || r.seen[item.ID] {
			return ItemsBatch{}, fmt.Errorf("board returned a missing or duplicate item identity")
		}
		r.seen[item.ID] = true
	}
	r.delivered += len(items)
	batch := ItemsBatch{Items: items, HasNext: more, Reset: r.reset}
	r.reset = false
	return batch, nil
}
