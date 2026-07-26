package ah

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The callback is the whole security boundary of the login flow: whoever can
// deliver a code to it decides which account the tokens belong to.
func TestLoginCallbackRejectsUnauthorisedCallers(t *testing.T) {
	const (
		secret = "0123456789abcdef"
		state  = "fedcba9876543210"
	)
	callbackPath := "/callback/" + secret

	tests := []struct {
		name       string
		path       string
		headers    map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "the browser we opened",
			path:       callbackPath + "?code=good&state=" + state,
			headers:    map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantStatus: http.StatusOK,
			wantCode:   "good",
		},
		{
			name:       "top-level navigation without fetch metadata",
			path:       callbackPath + "?code=good",
			wantStatus: http.StatusOK,
			wantCode:   "good",
		},
		{
			name:       "another site driving the callback",
			path:       callbackPath + "?code=attacker",
			headers:    map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "another site with an Origin header",
			path:       callbackPath + "?code=attacker",
			headers:    map[string]string{"Origin": "https://evil.example"},
			wantStatus: http.StatusForbidden,
		},
		{
			// Both of these fall through to the proxy, which is the login page
			// and not a place a code can be handed in.
			name:       "guessed the port but not the path",
			path:       "/callback?code=attacker",
			wantStatus: http.StatusTeapot,
		},
		{
			name:       "wrong secret in the path",
			path:       "/callback/deadbeef?code=attacker",
			wantStatus: http.StatusTeapot,
		},
		{
			name:       "state echoed back wrong",
			path:       callbackPath + "?code=attacker&state=wrong",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "no code at all",
			path:       callbackPath,
			wantStatus: http.StatusBadRequest,
		},
	}

	// Stands in for login.ah.nl, so the tests never touch the network. Anything
	// that reaches it took the proxy path instead of the callback.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			codes := make(chan string, 1)
			mux := loginMux(target, "http://127.0.0.1:1", callbackPath, state, codes)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			var got string
			select {
			case got = <-codes:
			default:
			}
			if got != tc.wantCode {
				t.Errorf("accepted code = %q, want %q", got, tc.wantCode)
			}
		})
	}
}

func TestLocaliseCookieKeepsSameSite(t *testing.T) {
	// Cookies are scoped by host and not by port, so one left on 127.0.0.1
	// without SameSite is reachable from every other local server.
	got := localiseCookie("session=abc; Path=/; Domain=login.ah.nl; Secure; SameSite=None; HttpOnly")

	for _, unwanted := range []string{"Domain", "Secure", "SameSite=None"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("cookie still carries %q: %s", unwanted, got)
		}
	}
	for _, wanted := range []string{"session=abc", "HttpOnly", "SameSite=Lax"} {
		if !strings.Contains(got, wanted) {
			t.Errorf("cookie lost %q: %s", wanted, got)
		}
	}
}
