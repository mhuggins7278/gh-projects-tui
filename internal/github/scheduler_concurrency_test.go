package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerAllowsTwoReadsButKeepsWritesExclusive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	readStarted, writeStarted := make(chan struct{}, 2), make(chan struct{}, 2)
	releaseReads, releaseWrites := make(chan struct{}), make(chan struct{}, 2)
	var reads, writes atomic.Int32
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		write := req.Method == http.MethodPost
		if write {
			if writes.Add(1) != 1 || reads.Load() != 0 {
				return nil, fmt.Errorf("write overlapped another request")
			}
			defer writes.Add(-1)
			writeStarted <- struct{}{}
			select {
			case <-releaseWrites:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		} else {
			if reads.Add(1) > 2 || writes.Load() != 0 {
				return nil, fmt.Errorf("read exceeded capacity or overlapped a write")
			}
			defer reads.Add(-1)
			readStarted <- struct{}{}
			select {
			case <-releaseReads:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}
		return testResponse(200, map[string]string{"X-RateLimit-Remaining": "100"}), nil
	})
	scheduler := newRequestScheduler(base)
	scheduler.writeInterval = 0
	done := make(chan error, 5)
	start := func(method string) {
		go func() {
			req, _ := http.NewRequestWithContext(ctx, method, "https://api.github.com/example", nil)
			response, err := scheduler.RoundTrip(req)
			if response != nil {
				response.Body.Close()
			}
			done <- err
		}()
	}
	wait := func(events <-chan struct{}) {
		t.Helper()
		select {
		case <-events:
		case <-ctx.Done():
			t.Fatal("request did not acquire its expected scheduler slot")
		}
	}
	start(http.MethodGet)
	start(http.MethodGet)
	wait(readStarted)
	wait(readStarted)
	queuedCtx, cancelQueued := context.WithCancel(ctx)
	cancelQueued()
	queued, _ := http.NewRequestWithContext(queuedCtx, http.MethodGet, "https://api.github.com/queued", nil)
	if _, err := scheduler.RoundTrip(queued); err != context.Canceled {
		t.Fatalf("queued cancellation was not preserved: %v", err)
	}
	start(http.MethodPost)
	start(http.MethodPost)
	select {
	case <-writeStarted:
		t.Fatal("write started while reads were in flight")
	default:
	}
	close(releaseReads)
	wait(writeStarted)
	select {
	case <-writeStarted:
		t.Fatal("two writes acquired the transport simultaneously")
	default:
	}
	// A read must also remain blocked while a write owns the transport.
	checkCtx, cancelCheck := context.WithCancel(ctx)
	cancelCheck()
	check, _ := http.NewRequestWithContext(checkCtx, http.MethodGet, "https://api.github.com/check", nil)
	if _, err := scheduler.RoundTrip(check); err != context.Canceled {
		t.Fatalf("read bypassed an exclusive write: %v", err)
	}
	start(http.MethodGet)
	releaseWrites <- struct{}{}
	wait(writeStarted)
	select {
	case <-readStarted:
		t.Fatal("read bypassed the queued exclusive write")
	default:
	}
	releaseWrites <- struct{}{}
	wait(readStarted)
	for range 5 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("scheduled requests did not finish")
		}
	}
	if scheduler.snapshot().Requests != 5 {
		t.Fatal("cancelled queued requests reached the transport")
	}
}

func TestSchedulerTreatsCommentedMutationAsExclusive(t *testing.T) {
	var scheduler *requestScheduler
	scheduler = newRequestScheduler(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// A write consumes the entire weighted gate, even when it was not
		// recognizable as a plain generated mutation.
		if scheduler.permit.TryAcquire(1) {
			scheduler.permit.Release(1)
			return nil, fmt.Errorf("mutation did not hold both scheduler slots")
		}
		return testResponse(200, nil), nil
	}))
	req, _ := http.NewRequest(http.MethodPost, "https://api.github.com/graphql", strings.NewReader(`{"query":"# comment\nmutation CloseIssue { closeIssue(input: {issueId:\"issue\"}) { issue { id } } }"}`))
	response, err := scheduler.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if scheduler.snapshot().Operation != "GraphQL mutation" {
		t.Fatal("unrecognized GraphQL text was allowed to share read slots")
	}
}
