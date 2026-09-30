package github

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
)

type reviewGraphQLFunc func(context.Context, string, map[string]interface{}, interface{}) error

func (f reviewGraphQLFunc) DoWithContext(ctx context.Context, q string, v map[string]interface{}, r interface{}) error {
	return f(ctx, q, v, r)
}

type reviewJoinContext struct {
	context.Context
	joined chan struct{}
	once   sync.Once
}

func (c *reviewJoinContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

func TestReviewCacheFollowerSurvivesLeaderCancellation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	base := reviewGraphQLFunc(func(ctx context.Context, _ string, _ map[string]interface{}, response interface{}) error {
		if calls.Add(1) == 1 {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		*response.(*map[string]interface{}) = map[string]interface{}{"ok": true}
		return nil
	})
	cache := newReadCache(base)
	leader, cancel := context.WithCancel(context.Background())
	defer cancel()
	query := "query SelectedProjectItems($x: String) { viewer { login } }"
	first := make(chan error, 1)
	go func() { var r map[string]interface{}; first <- cache.DoWithContext(leader, query, nil, &r) }()
	<-started
	follower := &reviewJoinContext{Context: context.Background(), joined: make(chan struct{})}
	second := make(chan error, 1)
	go func() { var r map[string]interface{}; second <- cache.DoWithContext(follower, query, nil, &r) }()
	<-follower.joined
	cancel()
	if err := <-first; err != context.Canceled {
		t.Fatalf("leader error = %v", err)
	}
	if err := <-second; err != nil && follower.Err() == nil {
		t.Fatalf("healthy caller inherited cancelled request: %v (base calls %d)", err, calls.Load())
	}
	if calls.Load() != 2 {
		t.Fatalf("healthy follower did not retry: calls = %d", calls.Load())
	}
}

func TestCacheCancelledFollowerDoesNotCancelLeader(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	cache := newReadCache(reviewGraphQLFunc(func(ctx context.Context, _ string, _ map[string]interface{}, response interface{}) error {
		calls.Add(1)
		close(started)
		<-release
		*response.(*map[string]interface{}) = map[string]interface{}{"ok": true}
		return nil
	}))
	query := "query SelectedProjectItems($x: String) { viewer { login } }"
	leaderResult := make(chan error, 1)
	go func() {
		var response map[string]interface{}
		leaderResult <- cache.DoWithContext(context.Background(), query, nil, &response)
	}()
	<-started
	follower, cancel := context.WithCancel(context.Background())
	defer cancel()
	joined := &reviewJoinContext{Context: follower, joined: make(chan struct{})}
	followerResult := make(chan error, 1)
	go func() {
		var response map[string]interface{}
		followerResult <- cache.DoWithContext(joined, query, nil, &response)
	}()
	<-joined.joined
	cancel()
	if err := <-followerResult; err != context.Canceled {
		t.Fatalf("cancelled follower: %v", err)
	}
	var response map[string]interface{}
	close(release)
	if err := <-leaderResult; err != nil {
		t.Fatal(err)
	}
	if err := cache.DoWithContext(context.Background(), query, nil, &response); err != nil || response["ok"] != true {
		t.Fatalf("cache result = %#v %v", response, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("base calls = %d", calls.Load())
	}
}

func TestCacheInvalidationDoesNotCacheOldFlight(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	cache := newReadCache(reviewGraphQLFunc(func(ctx context.Context, _ string, _ map[string]interface{}, response interface{}) error {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			<-release
		}
		*response.(*map[string]interface{}) = map[string]interface{}{"version": float64(call)}
		return nil
	}))
	query := "query SelectedProjectItems($x: String) { viewer { login } }"
	first := make(chan error, 1)
	go func() {
		var response map[string]interface{}
		first <- cache.DoWithContext(context.Background(), query, nil, &response)
	}()
	<-started
	cache.invalidate()
	var response map[string]interface{}
	if err := cache.DoWithContext(context.Background(), query, nil, &response); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := cache.DoWithContext(context.Background(), query, nil, &response); err != nil {
		t.Fatal(err)
	}
	if response["version"] != float64(2) || calls.Load() != 2 {
		t.Fatalf("old flight replaced new cache: %#v calls=%d", response, calls.Load())
	}
}
