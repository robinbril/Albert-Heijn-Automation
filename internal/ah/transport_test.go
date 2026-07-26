package ah

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGraphQLOperation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"anonymous shorthand", `{"query":"{ member { id } }"}`, ""},
		{"explicit query", `{"query":"query FetchMember { member { id } }"}`, "query"},
		{"mutation", `{"query":"mutation OrderRevert($id: Int!) { orderRevert(id: $id) { status } }"}`, "mutation"},
		{"leading blank lines", "{\"query\":\"\\n\\n  mutation X { y }\"}", "mutation"},
		{"comment first", "{\"query\":\"# adds a product\\nmutation X { y }\"}", "mutation"},
		{"field named mutation", `{"query":"query Q { mutationLog { id } }"}`, "query"},
		{"unparseable fails closed", `not json`, "mutation"},
		{"empty fails closed", ``, "mutation"},

		// A document may open with something other than its operation. Reading
		// only the first keyword let a mutation through behind a fragment.
		{"fragment then mutation", "{\"query\":\"fragment F on Product { id }\\nmutation Evil { cartAdd { id } }\"}", "mutation"},
		{"fragment then query", "{\"query\":\"fragment F on Product { id }\\nquery Q { member { ...F } }\"}", "query"},
		{"query then mutation", "{\"query\":\"query Q { member { id } }\\nmutation Evil { cartAdd { id } }\"}", "mutation"},
		{"blank query fails closed", `{"query":"   "}`, "mutation"},
		{"missing query field fails closed", `{"variables":{}}`, "mutation"},
		{"unknown top-level construct fails closed", `{"query":"directive @x on FIELD"}`, "mutation"},
		{"unbalanced braces fail closed", `{"query":"query Q { member { id }"}`, "mutation"},

		// Multi-line documents must still classify on the operation line only.
		{"multiline query", "{\"query\":\"query Q {\\n  member {\\n    id\\n  }\\n}\"}", "query"},
		{"multiline shorthand", "{\"query\":\"{\\n  member {\\n    id\\n  }\\n}\"}", ""},
		{"multiline mutation", "{\"query\":\"mutation M {\\n  cartAdd {\\n    id\\n  }\\n}\"}", "mutation"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := graphQLOperation([]byte(tc.body)); got != tc.want {
				t.Errorf("graphQLOperation() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestIsMutating(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   bool
	}{
		{"get is safe", http.MethodGet, "/mobile-services/product/search/v2", "", false},
		{"patch mutates", http.MethodPatch, "/mobile-services/shoppinglist/v2/items", "", true},
		{"post mutates", http.MethodPost, "/mobile-services/order/v1/items", "", true},
		{"token refresh is safe", http.MethodPost, "/mobile-auth/v1/auth/token/refresh", "", false},
		{"graphql query is safe", http.MethodPost, "/graphql", `{"query":"{ member { id } }"}`, false},
		{"graphql mutation is not", http.MethodPost, "/graphql", `{"query":"mutation M { x }"}`, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "https://api.ah.nl"+tc.path, strings.NewReader(tc.body))
			if got := isMutating(req, []byte(tc.body)); got != tc.want {
				t.Errorf("isMutating() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadOnlyBlocksMutations(t *testing.T) {
	var reached atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		io.WriteString(w, `{}`)
	}))
	defer server.Close()

	client := &http.Client{Transport: newTransport(http.DefaultTransport, true)}

	t.Run("write is refused", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodPatch, server.URL+"/mobile-services/shoppinglist/v2/items", strings.NewReader(`{"items":[]}`))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Do(req); !errors.Is(err, ErrReadOnly) {
			t.Fatalf("expected ErrReadOnly, got %v", err)
		}
		if reached.Load() != 0 {
			t.Fatal("request reached the server despite read-only mode")
		}
	})

	t.Run("read passes through", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/mobile-services/product/search/v2")
		if err != nil {
			t.Fatalf("read was blocked: %v", err)
		}
		resp.Body.Close()
		if reached.Load() != 1 {
			t.Fatalf("expected 1 request to reach the server, got %d", reached.Load())
		}
	})
}

func TestRetriesTransientFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer server.Close()

	client := &http.Client{Transport: newTransport(http.DefaultTransport, false)}
	resp, err := client.Get(server.URL + "/mobile-services/product/search/v2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want 3", got)
	}
}

func TestWritesAreNotRetried(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := &http.Client{Transport: newTransport(http.DefaultTransport, false)}
	req, err := http.NewRequest(http.MethodPatch, server.URL+"/mobile-services/shoppinglist/v2/items", strings.NewReader(`{"items":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// A duplicated add-to-list is worse than one surfaced failure.
	if got := attempts.Load(); got != 1 {
		t.Errorf("attempts = %d, want 1: writes must not be retried", got)
	}
}

func TestRetriedRequestKeepsItsBody(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(data))
		if len(bodies) < 2 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, `{"data":{}}`)
	}))
	defer server.Close()

	client := &http.Client{Transport: newTransport(http.DefaultTransport, false)}
	payload := `{"query":"{ member { id } }"}`
	resp, err := client.Post(server.URL+"/graphql", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if len(bodies) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(bodies))
	}
	for i, body := range bodies {
		if body != payload {
			t.Errorf("attempt %d body = %q, want %q", i+1, body, payload)
		}
	}
}
