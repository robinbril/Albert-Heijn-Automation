package ah

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The login flow is implemented here rather than taken from appie-go because
// the upstream one accepts an authorization code at a fixed /callback path with
// nothing tying the callback to the session that started it.
//
// That is a login CSRF: any page open in another tab can walk the loopback
// ports and post its own code, and the victim ends up holding tokens for
// somebody else's Albert Heijn account, with their own agent filling a stranger's
// basket. The fix here is a callback path that contains a secret, plus a state
// parameter and fetch-metadata checks on top.

const (
	loginClientID  = "appie-ios"
	loginBaseURL   = "https://login.ah.nl"
	loginTimeout   = 10 * time.Minute
	tokenEndpoint  = "/mobile-auth/v1/auth/token"
	appieRedirect  = "appie://login-exit"
	loginSuccessJS = `<!DOCTYPE html>
<html><head><title>Logged in</title><meta charset="utf-8"></head>
<body style="font-family:system-ui;max-width:32rem;margin:5rem auto;text-align:center">
<h1>Logged in</h1><p>You can close this tab and go back to your terminal.</p>
<script>setTimeout(function(){window.close()},500)</script>
</body></html>`
)

// Login runs the browser login flow and stores the resulting tokens.
//
// It serves a local reverse proxy to the AH login page, because the app's OAuth
// flow redirects to a custom appie:// scheme that a browser cannot follow. The
// redirect is rewritten to a loopback URL whose path carries a 256-bit secret,
// so only the browser that was handed the login URL can deliver a code.
func Login(ctx context.Context) (*Client, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	secret, err := randomHex()
	if err != nil {
		return nil, err
	}
	state, err := randomHex()
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start login server: %w", err)
	}
	localOrigin := "http://" + listener.Addr().String()
	callbackPath := "/callback/" + secret

	ctx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	target, err := url.Parse(loginBaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse login URL: %w", err)
	}

	codes := make(chan string, 1)
	srv := &http.Server{
		Handler:           loginMux(target, localOrigin, callbackPath, state, codes),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go srv.Serve(listener)
	defer srv.Shutdown(context.WithoutCancel(ctx))

	loginURL := fmt.Sprintf("%s/login?client_id=%s&response_type=code&redirect_uri=%s&state=%s",
		localOrigin, loginClientID, url.QueryEscape(appieRedirect), state)
	openBrowser(loginURL)

	select {
	case code := <-codes:
		return exchangeCode(ctx, path, code)
	case <-ctx.Done():
		return nil, fmt.Errorf("login: %w", ctx.Err())
	}
}

// loginMux serves the callback on a secret path and proxies everything else to
// the AH login host.
func loginMux(target *url.URL, localOrigin, callbackPath, state string, codes chan<- string) http.Handler {
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
			req.Header.Del("Accept-Encoding")
		},
		ModifyResponse: func(resp *http.Response) error {
			return rewriteLoginResponse(resp, localOrigin, callbackPath, target.Host)
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "login proxy error", http.StatusBadGateway)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		if !sameSiteRequest(r) {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		// AH is not required to echo the state back, and the secret in the path
		// is what actually authenticates this callback. A state that comes back
		// wrong is still a reason to stop.
		if got := r.URL.Query().Get("state"); got != "" &&
			subtle.ConstantTimeCompare([]byte(got), []byte(state)) != 1 {
			http.Error(w, "state mismatch", http.StatusForbidden)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no authorization code in callback", http.StatusBadRequest)
			return
		}

		select {
		case codes <- code:
		default:
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Referrer-Policy", "no-referrer")
		io.WriteString(w, loginSuccessJS)
	})

	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The proxy exists for the browser this flow just opened. Anything
		// arriving as a cross-site request is something else entirely.
		if !sameSiteRequest(r) {
			http.Error(w, "cross-site request refused", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	return mux
}

// sameSiteRequest rejects requests another site drove the browser into making.
// Fetch metadata is set by the browser and cannot be spoofed by page script;
// a request without it is a top-level navigation or a non-browser client.
func sameSiteRequest(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "cross-site", "same-site":
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != strings.TrimSuffix(originOf(r), "/") {
		return false
	}
	return true
}

func originOf(r *http.Request) string {
	return "http://" + r.Host
}

// rewriteLoginResponse points the login page's appie:// redirect at the local
// callback and rewrites absolute links to the login host.
func rewriteLoginResponse(resp *http.Response, localOrigin, callbackPath, targetHost string) error {
	callbackURL := localOrigin + callbackPath

	if loc := resp.Header.Get("Location"); loc != "" {
		switch {
		case strings.HasPrefix(loc, "appie://"):
			u, err := url.Parse(loc)
			if err != nil {
				return fmt.Errorf("parse appie redirect %q: %w", loc, err)
			}
			resp.Header.Set("Location", callbackURL+"?"+u.Query().Encode())
			return nil
		case strings.Contains(loc, targetHost):
			resp.Header.Set("Location", strings.ReplaceAll(loc, "https://"+targetHost, localOrigin))
		}
	}

	// The page is served from a different origin than it was written for, so
	// its CSP no longer describes reality and would block the rewritten assets.
	// HSTS has to go too: pinning 127.0.0.1 to https would break every other
	// local server on this machine. X-Frame-Options is deliberately kept.
	resp.Header.Del("Content-Security-Policy")
	resp.Header.Del("Content-Security-Policy-Report-Only")
	resp.Header.Del("Strict-Transport-Security")

	if cookies := resp.Header.Values("Set-Cookie"); len(cookies) > 0 {
		resp.Header.Del("Set-Cookie")
		for _, cookie := range cookies {
			resp.Header.Add("Set-Cookie", localiseCookie(cookie))
		}
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "javascript") && !strings.Contains(ct, "json") {
		return nil
	}

	body, err := readBody(resp)
	if err != nil {
		return err
	}
	body = bytes.ReplaceAll(body, []byte(appieRedirect), []byte(callbackURL))
	body = bytes.ReplaceAll(body, []byte("https://"+targetHost), []byte(localOrigin))

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", fmt.Sprint(len(body)))
	resp.Header.Del("Content-Encoding")
	return nil
}

// localiseCookie drops the attributes that would stop a cookie working over
// plain HTTP on loopback, and keeps SameSite rather than removing it.
//
// Safari does not treat localhost as a secure context, so a Secure cookie is
// dropped there and the login session breaks. SameSite is a different matter:
// cookies are scoped by host and not by port, so anything left on 127.0.0.1
// without it is reachable from every other local server and from any site that
// can drive a request at one.
func localiseCookie(cookie string) string {
	parts := strings.Split(cookie, ";")
	out := parts[:1]
	for _, part := range parts[1:] {
		lower := strings.ToLower(strings.TrimSpace(part))
		if lower == "secure" || strings.HasPrefix(lower, "domain") || strings.HasPrefix(lower, "samesite") {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, ";") + "; SameSite=Lax"
}

func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("decompress login response: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxSniffSize))
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read login response: %w", err)
	}
	return data, nil
}

// exchangeCode trades the authorization code for tokens and writes them to the
// token file, which only the user can read.
func exchangeCode(ctx context.Context, path, code string) (*Client, error) {
	client, err := Open()
	if err != nil {
		return nil, err
	}

	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		MemberID     string `json:"member_id,omitempty"`
		ExpiresIn    int    `json:"expires_in"`
	}
	body := map[string]string{"clientId": loginClientID, "code": code}
	if err := client.api.DoRequest(ctx, http.MethodPost, tokenEndpoint, body, &tok); err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("exchange authorization code: no access token in response")
	}

	stored := struct {
		AccessToken  string    `json:"access_token"`
		RefreshToken string    `json:"refresh_token"`
		MemberID     string    `json:"member_id,omitempty"`
		ExpiresAt    time.Time `json:"expires_at,omitempty"`
	}{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		MemberID:     tok.MemberID,
	}
	if tok.ExpiresIn > 0 {
		stored.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}

	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode tokens: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("write tokens to %s: %w", path, err)
	}

	return Open()
}

func randomHex() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate login secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// openBrowser opens the login URL, and prints it when it cannot.
func openBrowser(target string) {
	fmt.Fprintln(os.Stderr, "Opening your browser. If nothing happens, go to:\n"+target)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	_ = cmd.Start()
}
