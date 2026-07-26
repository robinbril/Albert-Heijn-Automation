// Command appie-mcp exposes the Albert Heijn skill over the Model Context
// Protocol, so an assistant can plan meals, read your purchase history and
// manage your shopping list directly.
//
// Reads work as soon as the tool is available. Writes are gated twice: every
// mutating tool requires an explicit confirm flag, and setting AH_READONLY=1
// blocks writes at the HTTP layer regardless of what a tool asks for.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

const version = "2.0.0"

func main() {
	// MCP speaks JSON-RPC over stdout; diagnostics must never land there.
	log.SetOutput(os.Stderr)
	log.SetFlags(0)

	ctx := context.Background()
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "albert-heijn",
		Version: version,
	}, &mcp.ServerOptions{
		Instructions: instructions,
	})

	registerReadTools(server)
	registerWriteTools(server)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("albert-heijn mcp: %v", err)
	}
}

const instructions = `Albert Heijn (Dutch supermarket) tools.

Reading is free: products, the bonus folder, Allerhande recipes, store bargains,
your purchase history, receipts and your shopping list.

Writing needs confirm=true on the tool call, and you must have shown the user
what you are about to change and had them agree first. Never add, remove or
reorder anything on a shopping list or in a cart on your own initiative.

Log in once with the appie-cli binary: "appie-cli login". Tools that need an
account return a clear error until that is done.`

// toolHandler is the shape every tool handler in this server has.
type toolHandler[In any] func(ctx context.Context, client *ah.Client, in In) (any, error)

// confirmer is implemented by the input of every mutating tool.
type confirmer interface {
	confirmed() bool
}

// Confirm is embedded in the input of every mutating tool.
type Confirm struct {
	Confirm bool `json:"confirm" jsonschema:"Must be true to apply the change. Ask the user first and show them exactly what changes."`
}

func (c Confirm) confirmed() bool { return c.Confirm }

// addRead registers a tool that only reads.
func addRead[In any](s *mcp.Server, name, description string, needsAuth bool, h toolHandler[In]) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}, wrap(needsAuth, h))
}

// addWrite registers a mutating tool. Every write in this server goes through
// here, so the confirmation check cannot be forgotten on a new tool: it is
// applied at registration rather than inside each handler.
func addWrite[In confirmer](s *mcp.Server, name, description string, h toolHandler[In]) {
	inner := wrap(true, h)

	mcp.AddTool(s, &mcp.Tool{
		Name:        name,
		Description: description + "\n\nRequires confirm=true. Show the user what will change and get their agreement before calling this.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: ptr(true),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		if !in.confirmed() {
			return refusal(name), nil, nil
		}
		return inner(ctx, req, in)
	})
}

// wrap turns a handler into the SDK signature, building the client it needs and
// rendering the result as JSON text.
func wrap[In any](needsAuth bool, h toolHandler[In]) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		client, err := newClient(ctx, needsAuth)
		if err != nil {
			return errorResult(err), nil, nil
		}

		result, err := h(ctx, client, in)
		if err != nil {
			return errorResult(err), nil, nil
		}
		return jsonResult(result)
	}
}

func newClient(ctx context.Context, needsAuth bool) (*ah.Client, error) {
	if needsAuth {
		return ah.Authenticated()
	}
	return ah.Anonymous(ctx)
}

// jsonResult renders a value as pretty JSON text content.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(fmt.Errorf("encode result: %w", err)), nil, nil
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, nil, nil
}

// errorResult reports a failure to the model without killing the session, and
// adds the one hint that resolves the most common causes.
func errorResult(err error) *mcp.CallToolResult {
	message := err.Error()
	switch {
	case errors.Is(err, ah.ErrNotAuthenticated):
		message += "\n\nRun `appie-cli login` in a terminal to connect the Albert Heijn account."
	case errors.Is(err, ah.ErrReadOnly):
		message += "\n\nThis server is running in read-only mode (AH_READONLY)."
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func refusal(tool string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(
			"%s was not executed: confirm was not set.\n\nShow the user exactly what would change, ask them to agree, then call again with confirm=true.",
			tool,
		)}},
	}
}

func ptr[T any](v T) *T { return &v }
