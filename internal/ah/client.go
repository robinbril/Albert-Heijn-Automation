// Package ah wraps github.com/gwillem/appie-go with the parts this skill needs:
// a token file that does not depend on the working directory, HTTP timeouts and
// retries, a read-only guard, and the AH endpoints appie-go does not cover.
package ah

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	appie "github.com/gwillem/appie-go"
)

const (
	// EnvConfig overrides where the AH tokens are stored.
	EnvConfig = "APPIE_CONFIG"
	// EnvReadOnly blocks every mutating request when set to a truthy value.
	EnvReadOnly = "AH_READONLY"

	requestTimeout = 30 * time.Second
)

// ErrNotAuthenticated is returned when a call needs a logged-in account.
var ErrNotAuthenticated = errors.New("not logged in: run `appie-cli login`")

// Client is an AH API client. Every request travels through a transport that
// applies timeouts, retries transient failures, and enforces read-only mode.
type Client struct {
	api        *appie.Client
	configPath string
	readOnly   bool
}

// API exposes the underlying appie-go client. Mutating calls made through it
// are still subject to the read-only guard, which lives in the transport.
func (c *Client) API() *appie.Client { return c.api }

// ConfigPath reports where this client stores its tokens.
func (c *Client) ConfigPath() string { return c.configPath }

// ReadOnly reports whether mutating requests are blocked.
func (c *Client) ReadOnly() bool { return c.readOnly }

// ConfigPath resolves the token file location: APPIE_CONFIG if set, otherwise
// the per-user config directory.
//
// The working directory is deliberately not consulted. Tokens written next to
// whatever project happened to be open are one `git add -A` away from being
// published, and the MCP server is started by its client with no predictable
// working directory anyway. Someone who genuinely wants a project-local token
// file can say so with APPIE_CONFIG.
func ConfigPath() (string, error) {
	if p := os.Getenv(EnvConfig); p != "" {
		return filepath.Abs(p)
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config dir: %w", err)
	}
	return filepath.Join(dir, "appie", "tokens.json"), nil
}

// readOnlyEnabled reports whether EnvReadOnly asks for read-only mode.
func readOnlyEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvReadOnly))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// Open loads stored tokens without verifying them. Use Authenticated when the
// call needs a logged-in account, or Anonymous for public endpoints.
func Open() (*Client, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	readOnly := readOnlyEnabled()
	httpClient := &http.Client{
		Timeout:   requestTimeout,
		Transport: newTransport(http.DefaultTransport, readOnly),
	}

	api, err := appie.NewWithConfig(path, appie.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("load tokens from %s: %w", path, err)
	}

	return &Client{api: api, configPath: path, readOnly: readOnly}, nil
}

// Authenticated returns a client with stored credentials, or ErrNotAuthenticated.
func Authenticated() (*Client, error) {
	c, err := Open()
	if err != nil {
		return nil, err
	}
	if !c.api.IsAuthenticated() {
		return nil, ErrNotAuthenticated
	}
	return c, nil
}

// Anonymous returns a client for endpoints that do not need an account. Stored
// credentials are reused when present so results stay personalised.
func Anonymous(ctx context.Context) (*Client, error) {
	c, err := Open()
	if err != nil {
		return nil, err
	}
	if c.api.IsAuthenticated() {
		return c, nil
	}
	if err := c.api.GetAnonymousToken(ctx); err != nil {
		return nil, fmt.Errorf("get anonymous token: %w", err)
	}
	return c, nil
}

// Login lives in login.go: the browser flow is implemented here rather than
// taken from appie-go, so the callback cannot be driven by another site.
