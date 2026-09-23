package github

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"
)

type cachedRead struct {
	data []byte
	at   time.Time
}
type readFlight struct {
	done chan struct{}
	data []byte
	err  error
}
type readCache struct {
	base       graphQLClient
	mu         sync.Mutex
	entries    map[string]cachedRead
	flights    map[string]*readFlight
	generation uint64
	lastHit    time.Time
}

func newReadCache(base graphQLClient) *readCache {
	return &readCache{base: base, entries: map[string]cachedRead{}, flights: map[string]*readFlight{}}
}

func (c *Client) InvalidateReads() {
	c.definitions.Clear()
	if c.reads != nil {
		c.reads.invalidate()
	}
}

func (c *Client) CachedReadAt() time.Time {
	if c.reads == nil {
		return time.Time{}
	}
	c.reads.mu.Lock()
	defer c.reads.mu.Unlock()
	return c.reads.lastHit
}

func (c *readCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.entries = map[string]cachedRead{}
	c.flights = map[string]*readFlight{}
	c.lastHit = time.Time{}
}

func (c *readCache) DoWithContext(ctx context.Context, query string, variables map[string]interface{}, response interface{}) error {
	mutation := strings.HasPrefix(strings.TrimSpace(query), "mutation")
	if mutation {
		c.invalidate()
		err := c.base.DoWithContext(ctx, query, variables, response)
		c.invalidate() // Reads overlapping submission cannot survive completion.
		return err
	}
	// Explicit allowlist: reconciliation and item detail bodies are never cached.
	cacheable := strings.Contains(query, "query SelectedProjectItems(") || strings.Contains(query, "query ProjectFieldDefinitions(") || strings.Contains(query, "query UserProjectView(") || strings.Contains(query, "query OrganizationProjectView(") || strings.Contains(query, "query UserProjectViews(") || strings.Contains(query, "query OrganizationProjectViews(")
	if !cacheable {
		return c.base.DoWithContext(ctx, query, variables, response)
	}
	encoded, err := json.Marshal(variables)
	if err != nil {
		return err
	}
	key := query + string(encoded)
	c.mu.Lock()
	if entry, ok := c.entries[key]; ok && time.Since(entry.at) < 30*time.Second {
		c.lastHit = entry.at
		c.mu.Unlock()
		return json.Unmarshal(entry.data, response)
	}
	if flight, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-flight.done:
		}
		if flight.err != nil {
			return flight.err
		}
		return json.Unmarshal(flight.data, response)
	}
	flight := &readFlight{done: make(chan struct{})}
	c.flights[key] = flight
	generation := c.generation
	c.mu.Unlock()
	err = c.base.DoWithContext(ctx, query, variables, response)
	var data []byte
	if err == nil {
		data, err = json.Marshal(response)
	}
	c.mu.Lock()
	flight.data, flight.err = data, err
	if generation == c.generation {
		delete(c.flights, key)
		if err == nil {
			if len(c.entries) >= 128 {
				c.entries = map[string]cachedRead{}
			}
			c.entries[key] = cachedRead{data: data, at: time.Now()}
		}
	}
	close(flight.done)
	c.mu.Unlock()
	return err
}
