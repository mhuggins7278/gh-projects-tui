package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// APIStatus contains request metadata only; no query variables or item content.
type APIStatus struct {
	Requests  int
	Remaining int
	LastCost  int
	TotalCost int
	Resource  string
	Reset     time.Time
	Cooldown  time.Time
	Operation string
	Duration  time.Duration
	RequestID string
	// Breakdown counts completed HTTP requests by fingerprint (GraphQL
	// operation name or REST method + path template). Bounded.
	Breakdown       map[string]int
	CostByOperation map[string]int
	// Recent lists the most recent request fingerprints, oldest first.
	// Bounded to the last 20.
	Recent []string
}

// Ledger returns a copy of the per-fingerprint request breakdown.
func (c *Client) Ledger() map[string]int {
	if c.scheduler == nil {
		return nil
	}
	return c.scheduler.ledger()
}

// requestScheduler is shared by this authenticated client's REST and GraphQL
// transports. It holds the permit through response consumption, not just headers.
type requestScheduler struct {
	base          http.RoundTripper
	permit        chan struct{}
	mu            sync.Mutex
	status        APIStatus
	nextWrite     time.Time
	writeInterval time.Duration
	now           func() time.Time
	wait          func(context.Context, time.Duration) error
}

func newRequestScheduler(base http.RoundTripper) *requestScheduler {
	return &requestScheduler{base: base, permit: make(chan struct{}, 1), writeInterval: time.Second, now: time.Now, wait: waitRequest, status: APIStatus{Remaining: -1}}
}

func waitRequest(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (s *requestScheduler) snapshot() APIStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.status
	if out.Breakdown != nil {
		copied := make(map[string]int, len(out.Breakdown))
		for key, value := range out.Breakdown {
			copied[key] = value
		}
		out.Breakdown = copied
	}
	if out.CostByOperation != nil {
		copied := make(map[string]int, len(out.CostByOperation))
		for key, value := range out.CostByOperation {
			copied[key] = value
		}
		out.CostByOperation = copied
	}
	out.Recent = append([]string(nil), out.Recent...)
	return out
}

func (s *requestScheduler) ledger() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int, len(s.status.Breakdown))
	for key, value := range s.status.Breakdown {
		out[key] = value
	}
	return out
}

// graphqlFingerprint names the GraphQL operation without variables or bodies,
// e.g. "query UserProjectViews" or "mutation UpdateProjectItemPosition".
func graphqlFingerprint(query string) string {
	fields := strings.Fields(query)
	if len(fields) < 2 {
		return "GraphQL query"
	}
	name := fields[1]
	if index := strings.Index(name, "("); index >= 0 {
		name = name[:index]
	}
	if name == "" {
		return "GraphQL query"
	}
	return fields[0] + " " + name
}

// restPathTemplate strips pagination values so repeated pages group together,
// e.g. "GET user/orgs" instead of "GET user/orgs?per_page=100&page=7".
func restPathTemplate(path, rawQuery string) string {
	trimmed := strings.Trim(path, "/")
	if strings.HasSuffix(path, "/graphql") {
		return "graphql"
	}
	if index := strings.Index(rawQuery, "per_page"); index >= 0 {
		return trimmed
	}
	if rawQuery != "" {
		return trimmed + "?" + rawQuery
	}
	return trimmed
}

func (c *Client) APIStatus() APIStatus {
	if c.scheduler == nil {
		return APIStatus{Remaining: -1}
	}
	return c.scheduler.snapshot()
}

func (s *requestScheduler) RoundTrip(req *http.Request) (*http.Response, error) {
	select {
	case s.permit <- struct{}{}:
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
	defer func() { <-s.permit }()
	var body []byte
	if req.Body != nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
	}
	write := req.Method != http.MethodGet && req.Method != http.MethodHead
	operation := req.Method
	fingerprint := req.Method + " " + restPathTemplate(req.URL.Path, req.URL.RawQuery)
	if strings.HasSuffix(req.URL.Path, "/graphql") {
		var payload map[string]json.RawMessage
		if json.Unmarshal(body, &payload) == nil {
			var query string
			_ = json.Unmarshal(payload["query"], &query)
			query = strings.TrimSpace(query)
			write = strings.HasPrefix(query, "mutation")
			if !write {
				var original string
				_ = json.Unmarshal(payload["query"], &original)
				updated, _ := json.Marshal(addRateLimitSelection(original))
				payload["query"] = updated
				body, _ = json.Marshal(payload)
			}
			fingerprint = graphqlFingerprint(query)
			if write {
				operation = "GraphQL mutation"
			} else {
				operation = "GraphQL query"
			}
		}
	}
	for attempt := 0; ; attempt++ {
		s.mu.Lock()
		deadline := s.status.Cooldown
		if write && s.nextWrite.After(deadline) {
			deadline = s.nextWrite
		}
		s.mu.Unlock()
		if err := s.wait(req.Context(), deadline.Sub(s.now())); err != nil {
			return nil, err
		}
		// Time spent waiting for the quota is not network request time.
		ctx, cancel := context.WithTimeout(req.Context(), 45*time.Second)
		clone := req.Clone(ctx)
		if req.Body != nil {
			clone.Body = io.NopCloser(bytes.NewReader(body))
			clone.ContentLength = int64(len(body))
		}
		started := s.now()
		response, err := s.base.RoundTrip(clone)
		var data []byte
		if err == nil {
			data, err = io.ReadAll(response.Body)
			response.Body.Close()
		}
		cancel()
		s.mu.Lock()
		s.status.Requests++
		s.status.Operation = operation
		s.status.Duration = s.now().Sub(started)
		if s.status.Breakdown == nil {
			s.status.Breakdown = map[string]int{}
		}
		s.status.Breakdown[fingerprint]++
		s.status.Recent = append(s.status.Recent, fingerprint)
		if len(s.status.Recent) > 20 {
			s.status.Recent = append([]string(nil), s.status.Recent[len(s.status.Recent)-20:]...)
		}
		if write {
			s.nextWrite = s.now().Add(s.writeInterval)
		}
		if response != nil {
			s.status.RequestID = response.Header.Get("X-Github-Request-Id")
			if remaining, parseErr := strconv.Atoi(response.Header.Get("X-RateLimit-Remaining")); parseErr == nil {
				s.status.Remaining = remaining
				s.status.Resource = response.Header.Get("X-RateLimit-Resource")
			}
			if reset, parseErr := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); parseErr == nil {
				s.status.Reset = time.Unix(reset, 0)
			}
		}
		cost, hasCost := responseQueryCost(data)
		if hasCost {
			s.status.LastCost = cost
			s.status.TotalCost += cost
			if s.status.CostByOperation == nil {
				s.status.CostByOperation = map[string]int{}
			}
			s.status.CostByOperation[fingerprint] += cost
		}
		s.mu.Unlock()
		if err != nil {
			return nil, err
		} // A submitted write is never replayed.
		response.Body = io.NopCloser(bytes.NewReader(data))
		response.ContentLength = int64(len(data))
		limited := rateLimited(response, data)
		if limited || response.Header.Get("X-RateLimit-Remaining") == "0" {
			until := s.now().Add(time.Minute * time.Duration(1<<min(attempt, 3)))
			if seconds, parseErr := strconv.Atoi(response.Header.Get("Retry-After")); parseErr == nil {
				until = s.now().Add(time.Duration(seconds) * time.Second)
			} else if date, parseErr := http.ParseTime(response.Header.Get("Retry-After")); parseErr == nil {
				until = date
			}
			if response.Header.Get("X-RateLimit-Remaining") == "0" {
				if reset, parseErr := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); parseErr == nil {
					until = time.Unix(reset, 0).Add(time.Second)
				}
			}
			// Small bounded jitter avoids synchronized retry bursts.
			until = until.Add(time.Duration(s.now().UnixNano()%250) * time.Millisecond)
			s.mu.Lock()
			if until.After(s.status.Cooldown) {
				s.status.Cooldown = until
			}
			s.mu.Unlock()
		}
		if !limited || write || attempt >= 2 {
			return response, nil
		}
		response.Body.Close()
	}
}

func responseQueryCost(body []byte) (int, bool) {
	var payload struct {
		Data struct {
			RateLimit *struct {
				Cost int `json:"cost"`
			} `json:"rateLimit"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Data.RateLimit == nil {
		return 0, false
	}
	return payload.Data.RateLimit.Cost, true
}

func addRateLimitSelection(query string) string {
	if strings.Contains(query, "rateLimit {") {
		return query
	}
	depth := 0
	opened := false
	inString, inBlockString, escaped, inComment := false, false, false, false
	for index := 0; index < len(query); index++ {
		char := query[index]
		if inComment {
			if char == '\n' {
				inComment = false
			}
			continue
		}
		if inBlockString {
			if index+2 < len(query) && query[index:index+3] == `"""` {
				inBlockString = false
				index += 2
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if char == '\\' {
				escaped = true
				continue
			}
			if char == '"' {
				inString = false
			}
			continue
		}
		if char == '#' {
			inComment = true
			continue
		}
		if index+2 < len(query) && query[index:index+3] == `"""` {
			inBlockString = true
			index += 2
			continue
		}
		if char == '"' {
			inString = true
			continue
		}
		if char == '{' {
			depth++
			opened = true
			continue
		}
		if char == '}' && opened {
			depth--
			if depth == 0 {
				return query[:index] + " rateLimit { cost remaining resetAt } " + query[index:]
			}
		}
	}
	return query
}

func rateLimited(response *http.Response, body []byte) bool {
	if response.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusForbidden {
		return false
	}
	var payload struct {
		Message string `json:"message"`
		Errors  []struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	messages := []string{payload.Message}
	for _, e := range payload.Errors {
		if e.Type == "RATE_LIMITED" {
			return true
		}
		messages = append(messages, e.Message)
	}
	for _, message := range messages {
		message = strings.ToLower(message)
		if strings.Contains(message, "rate limit") || strings.Contains(message, "abuse detection") {
			return true
		}
	}
	return false
}
