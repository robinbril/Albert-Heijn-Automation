package ah

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ErrReadOnly is returned when a mutating request is attempted while read-only
// mode is active.
var ErrReadOnly = fmt.Errorf("blocked by read-only mode (%s): unset it to allow writes", EnvReadOnly)

const (
	maxAttempts  = 3
	baseBackoff  = 500 * time.Millisecond
	maxBackoff   = 8 * time.Second
	maxSniffSize = 1 << 20 // 1 MiB; GraphQL documents are far smaller
)

// transport applies retries and the read-only guard to every request.
//
// Both live here rather than at each call site on purpose: this is the single
// point every AH request passes through, so a mutating endpoint added later
// cannot silently escape the guard.
type transport struct {
	base     http.RoundTripper
	readOnly bool
}

func newTransport(base http.RoundTripper, readOnly bool) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &transport{base: base, readOnly: readOnly}
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, err := peekBody(req)
	if err != nil {
		return nil, err
	}

	if t.readOnly && isMutating(req, body) {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, ErrReadOnly)
	}

	if !isRetryable(req, body) {
		return t.base.RoundTrip(req)
	}
	return t.roundTripWithRetries(req)
}

// roundTripWithRetries retries transient AH failures. Only requests that are
// safe to repeat reach this path; see isRetryable.
func (t *transport) roundTripWithRetries(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 1; ; attempt++ {
		attemptReq := req
		if attempt > 1 {
			replayed, err := replayRequest(req)
			if err != nil {
				return nil, err
			}
			attemptReq = replayed
		}

		resp, err := t.base.RoundTrip(attemptReq)
		if err == nil && !isTransientStatus(resp.StatusCode) {
			return resp, nil
		}
		if attempt == maxAttempts {
			if err != nil {
				return nil, err
			}
			return resp, nil
		}

		wait := backoff(attempt)
		if err != nil {
			lastErr = err
		} else {
			if hinted, ok := retryAfter(resp); ok {
				wait = hinted
			}
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			drainAndClose(resp)
		}
		_ = lastErr

		select {
		case <-time.After(wait):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
}

// peekBody returns the request body and leaves the request able to send it.
func peekBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}

	data, err := io.ReadAll(io.LimitReader(req.Body, maxSniffSize))
	req.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewReader(data))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	}
	req.ContentLength = int64(len(data))
	return data, nil
}

// replayRequest clones a request with a fresh body for a retry attempt.
func replayRequest(req *http.Request) (*http.Request, error) {
	clone := req.Clone(req.Context())
	if req.GetBody == nil {
		return clone, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("rewind request body for retry: %w", err)
	}
	clone.Body = body
	return clone, nil
}

// isMutating reports whether a request changes server-side state.
func isMutating(req *http.Request, body []byte) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	}

	// Token exchange and refresh are POSTs but change nothing in the account.
	if strings.HasPrefix(req.URL.Path, "/mobile-auth/") {
		return false
	}

	if isGraphQLPath(req.URL.Path) {
		return graphQLOperation(body) == "mutation"
	}

	return true
}

// isRetryable reports whether repeating a request is safe. Writes are never
// retried: a duplicated add-to-list or add-to-order is worse than one failure
// surfaced to the caller.
func isRetryable(req *http.Request, body []byte) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodPost:
		return isGraphQLPath(req.URL.Path) && graphQLOperation(body) != "mutation"
	default:
		return false
	}
}

func isGraphQLPath(path string) bool {
	return strings.TrimSuffix(path, "/") == "/graphql"
}

// graphQLOperation returns the leading operation keyword of a GraphQL request
// body: "query", "mutation", "subscription", or "" for an anonymous operation.
// An unparseable body yields "mutation" so the read-only guard fails closed.
func graphQLOperation(body []byte) string {
	if len(body) == 0 {
		return "mutation"
	}

	var payload struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "mutation"
	}

	for _, line := range strings.Split(payload.Query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		word := strings.TrimFunc(line, func(r rune) bool { return !unicode.IsLetter(r) })
		if idx := strings.IndexFunc(word, func(r rune) bool { return !unicode.IsLetter(r) }); idx > 0 {
			word = word[:idx]
		}
		switch strings.ToLower(word) {
		case "mutation":
			return "mutation"
		case "subscription":
			return "subscription"
		case "query":
			return "query"
		default:
			// Anonymous shorthand such as "{ member { id } }" is always a query.
			return ""
		}
	}
	return ""
}

func isTransientStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

// retryAfter reads the Retry-After header, in seconds or as an HTTP date.
func retryAfter(resp *http.Response) (time.Duration, bool) {
	raw := resp.Header.Get("Retry-After")
	if raw == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && secs >= 0 {
		return min(time.Duration(secs)*time.Second, maxBackoff), true
	}
	if when, err := http.ParseTime(raw); err == nil {
		if d := time.Until(when); d > 0 {
			return min(d, maxBackoff), true
		}
	}
	return 0, false
}

// backoff grows exponentially with jitter to avoid synchronised retries.
func backoff(attempt int) time.Duration {
	d := baseBackoff << (attempt - 1)
	d = min(d, maxBackoff)
	jitter := time.Duration(rand.Int63n(int64(d / 2)))
	return d/2 + jitter
}

func drainAndClose(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxSniffSize))
	resp.Body.Close()
}
