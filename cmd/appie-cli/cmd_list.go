package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	appie "github.com/gwillem/appie-go"
	"github.com/robinbril/Albert-Heijn-Automation/internal/ah"
)

func listCommands() []command {
	var (
		text     string
		quantity int
		fromJSON bool
		uncheck  bool
	)

	return []command{
		{
			name:    "list",
			summary: "Show the shopping list with prices and totals",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.GetShoppingList(ctx)
			},
		},
		{
			name:    "list-add",
			args:    "[product-id...]",
			summary: "Add products or a note to the shopping list",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&text, "text", "", "add a free-text note instead of a product")
				fs.IntVar(&quantity, "qty", 1, "quantity per item")
				fs.BoolVar(&fromJSON, "stdin", false, `read a JSON array from stdin: [{"id":123,"qty":2},{"text":"Slager: kipfilet"}]`)
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				items, err := collectItems(fs, text, quantity, fromJSON)
				if err != nil {
					return nil, err
				}
				if err := c.AddToList(ctx, items); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "added": len(items)}, nil
			},
		},
		{
			name:    "list-remove",
			args:    "[product-id...]",
			summary: "Delete lines from the shopping list",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&text, "match", "", "delete every line whose title contains this text")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				refs, err := resolveRefs(ctx, c, fs, text)
				if err != nil {
					return nil, err
				}
				remaining, err := c.Remove(ctx, refs)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "removed": len(refs), "remaining": remaining}, nil
			},
		},
		{
			name:    "list-qty",
			args:    "<product-id> <quantity>",
			summary: "Change how many of an item are on the list",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&text, "text", "", "target a free-text note by its text instead of a product id")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				// A note has no product id, so it can only be addressed by text.
				// A note has no product id, so it can only be addressed by text.
				if strings.TrimSpace(text) != "" {
					qty, err := intArg(fs, 0, "quantity")
					if err != nil {
						return nil, err
					}
					entry, err := c.SetQuantity(ctx, ah.ItemRef{Text: text}, qty)
					if err != nil {
						return nil, err
					}
					return map[string]any{"ok": true, "item": entry}, nil
				}

				id, err := intArg(fs, 0, "product-id")
				if err != nil {
					return nil, fmt.Errorf("%w (or pass -text \"note\")", err)
				}
				qty, err := intArg(fs, 1, "quantity")
				if err != nil {
					return nil, err
				}
				entry, err := c.SetQuantity(ctx, ah.ItemRef{ProductID: id}, qty)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "item": entry}, nil
			},
		},
		{
			name:    "list-check",
			args:    "[product-id...]",
			summary: "Cross items off the list without deleting them",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&text, "match", "", "match lines by title")
				fs.BoolVar(&uncheck, "undo", false, "restore crossed-off items instead")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				refs, err := resolveRefs(ctx, c, fs, text)
				if err != nil {
					return nil, err
				}
				if err := c.CheckOff(ctx, refs, !uncheck); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "checked": !uncheck, "items": len(refs)}, nil
			},
		},
		{
			name:    "list-find",
			args:    "<text>",
			summary: "Find lines on the shopping list by title",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				term, err := requireArg(fs, 0, "text")
				if err != nil {
					return nil, err
				}
				matches, err := c.FindInList(ctx, term)
				if err != nil {
					return nil, err
				}
				return map[string]any{"count": len(matches), "matches": matches}, nil
			},
		},
		{
			name:    "list-clear",
			summary: "Delete every line from the shopping list",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				removed, remaining, err := c.ClearList(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "removed": removed, "remaining": remaining}, nil
			},
		},
		{
			name:    "lists",
			summary: "Show the saved favourite lists",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.API().GetShoppingLists(ctx, 0)
			},
		},
		{
			name:    "list-items",
			args:    "<list-id>",
			summary: "Show the items in a favourite list",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id, err := requireArg(fs, 0, "list-id")
				if err != nil {
					return nil, err
				}
				return c.API().GetShoppingListItems(ctx, id)
			},
		},
	}
}

// collectItems builds the items to add from flags, positional ids or stdin.
func collectItems(fs *flag.FlagSet, text string, quantity int, fromJSON bool) ([]appie.ListItem, error) {
	if quantity < 1 {
		return nil, fmt.Errorf("quantity must be at least 1")
	}

	if fromJSON {
		return decodeItems(os.Stdin)
	}

	if strings.TrimSpace(text) != "" {
		return []appie.ListItem{{Name: text, Quantity: quantity}}, nil
	}

	ids, err := intArgs(fs, "product-id")
	if err != nil {
		return nil, fmt.Errorf("%w (or pass -text \"note\" or -stdin)", err)
	}
	items := make([]appie.ListItem, 0, len(ids))
	for _, id := range ids {
		items = append(items, appie.ListItem{ProductID: id, Quantity: quantity})
	}
	return items, nil
}

// decodeItems reads a JSON batch: [{"id":123,"qty":2},{"text":"note","qty":1}].
func decodeItems(r *os.File) ([]appie.ListItem, error) {
	var batch []struct {
		ID   int    `json:"id"`
		Text string `json:"text"`
		Qty  int    `json:"qty"`
	}
	if err := json.NewDecoder(r).Decode(&batch); err != nil {
		return nil, fmt.Errorf("invalid JSON on stdin: %w", err)
	}

	items := make([]appie.ListItem, 0, len(batch))
	for i, entry := range batch {
		qty := max(entry.Qty, 1)
		switch {
		case entry.Text != "":
			items = append(items, appie.ListItem{Name: entry.Text, Quantity: qty})
		case entry.ID > 0:
			items = append(items, appie.ListItem{ProductID: entry.ID, Quantity: qty})
		default:
			return nil, fmt.Errorf("item %d has neither an id nor text", i)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no items on stdin")
	}
	return items, nil
}

// resolveRefs turns positional product ids or a -match term into item references.
func resolveRefs(ctx context.Context, c *ah.Client, fs *flag.FlagSet, match string) ([]ah.ItemRef, error) {
	if strings.TrimSpace(match) != "" {
		entries, err := c.FindInList(ctx, match)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("no items on the list match %q", match)
		}
		refs := make([]ah.ItemRef, 0, len(entries))
		for _, entry := range entries {
			refs = append(refs, entry.Ref())
		}
		return refs, nil
	}

	if fs.NArg() == 0 {
		return nil, fmt.Errorf("pass one or more product ids, or -match <text>")
	}

	refs := make([]ah.ItemRef, 0, fs.NArg())
	for i := range fs.NArg() {
		raw := fs.Arg(i)
		id, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("product-id must be a number, got %q", raw)
		}
		refs = append(refs, ah.ItemRef{ProductID: id})
	}
	return refs, nil
}
