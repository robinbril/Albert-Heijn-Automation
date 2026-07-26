package main

import (
	"context"
	"fmt"
	"strings"

	appie "github.com/gwillem/appie-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

// listItemInput is one thing to put on the shopping list: either a product or
// a free-text note, for example something to buy at the butcher instead.
type listItemInput struct {
	ProductID int    `json:"product_id,omitempty" jsonschema:"Webshop product id. Leave empty for a free-text note."`
	Text      string `json:"text,omitempty" jsonschema:"Free-text note, e.g. 'Slager: kipfilet'. Use instead of product_id."`
	Quantity  int    `json:"quantity,omitempty" jsonschema:"How many. Defaults to 1."`
}

type listAddInput struct {
	Confirm
	Items []listItemInput `json:"items" jsonschema:"The products and notes to add."`
}

type listRemoveInput struct {
	Confirm
	ProductIDs []int  `json:"product_ids,omitempty" jsonschema:"Product ids to delete from the list."`
	Match      string `json:"match,omitempty" jsonschema:"Delete every line whose title contains this text. Use instead of product_ids."`
}

type listQuantityInput struct {
	Confirm
	ProductID int    `json:"product_id,omitempty" jsonschema:"Product id already on the list."`
	Text      string `json:"text,omitempty" jsonschema:"Target a free-text note by its text instead of a product id. Note that Albert Heijn ignores quantity changes on notes."`
	Quantity  int    `json:"quantity" jsonschema:"New quantity, at least 1. To delete a line use list_remove."`
}

type listCheckInput struct {
	Confirm
	ProductIDs []int  `json:"product_ids,omitempty" jsonschema:"Product ids to cross off."`
	Match      string `json:"match,omitempty" jsonschema:"Cross off every line whose title contains this text."`
	Undo       bool   `json:"undo,omitempty" jsonschema:"Restore crossed-off items instead of crossing them off."`
}

type confirmOnlyInput struct {
	Confirm
}

type cartAddInput struct {
	Confirm
	Items []cartItemInput `json:"items" jsonschema:"The products to add to the cart."`
}

type cartItemInput struct {
	ProductID int `json:"product_id" jsonschema:"Webshop product id."`
	Quantity  int `json:"quantity,omitempty" jsonschema:"How many. Defaults to 1."`
}

type cartQuantityInput struct {
	Confirm
	ProductID int `json:"product_id" jsonschema:"Product id in the cart."`
	Quantity  int `json:"quantity" jsonschema:"New quantity. 0 removes the product."`
}

type cartRemoveInput struct {
	Confirm
	ProductIDs []int `json:"product_ids" jsonschema:"Product ids to remove from the cart."`
}

func registerWriteTools(s *mcp.Server) {
	registerListWriteTools(s)
	registerCartWriteTools(s)
}

func registerListWriteTools(s *mcp.Server) {
	addWrite(s, "list_add",
		"Add products or free-text notes to the Albert Heijn shopping list.",
		func(ctx context.Context, c *ah.Client, in listAddInput) (any, error) {
			items, err := toListItems(in.Items)
			if err != nil {
				return nil, err
			}
			if err := c.AddToList(ctx, items); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "added": len(items)}, nil
		})

	addWrite(s, "list_remove",
		"Delete lines from the shopping list. This really removes them, unlike crossing an item off.",
		func(ctx context.Context, c *ah.Client, in listRemoveInput) (any, error) {
			refs, err := refsFrom(ctx, c, in.ProductIDs, in.Match)
			if err != nil {
				return nil, err
			}
			remaining, err := c.Remove(ctx, refs)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "removed": len(refs), "remaining": remaining}, nil
		})

	addWrite(s, "list_set_quantity",
		"Change how many of a product are on the shopping list.",
		func(ctx context.Context, c *ah.Client, in listQuantityInput) (any, error) {
			ref := ah.ItemRef{ProductID: in.ProductID, Text: in.Text}
			entry, err := c.SetQuantity(ctx, ref, in.Quantity)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "item": entry}, nil
		})

	addWrite(s, "list_check_off",
		"Cross items off the shopping list without deleting them, the way ticking them in the app does.",
		func(ctx context.Context, c *ah.Client, in listCheckInput) (any, error) {
			refs, err := refsFrom(ctx, c, in.ProductIDs, in.Match)
			if err != nil {
				return nil, err
			}
			if err := c.CheckOff(ctx, refs, !in.Undo); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "checked": !in.Undo, "items": len(refs)}, nil
		})

	addWrite(s, "list_clear",
		"Delete every line from the shopping list. Show the user what is on it before doing this.",
		func(ctx context.Context, c *ah.Client, _ confirmOnlyInput) (any, error) {
			removed, remaining, err := c.ClearList(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "removed": removed, "remaining": remaining}, nil
		})

	addWrite(s, "list_to_cart",
		"Copy every unchecked product from the shopping list into the online cart. Free-text notes are skipped.",
		func(ctx context.Context, c *ah.Client, _ confirmOnlyInput) (any, error) {
			added, skipped, err := c.ListToOrder(ctx)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"ok":      true,
				"added":   added,
				"skipped": skipped,
				"note":    "Paying and choosing a delivery slot has no public API; finish the order in the AH app.",
			}, nil
		})
}

func registerCartWriteTools(s *mcp.Server) {
	addWrite(s, "cart_add",
		"Add products to the online cart.",
		func(ctx context.Context, c *ah.Client, in cartAddInput) (any, error) {
			if len(in.Items) == 0 {
				return nil, fmt.Errorf("no items given")
			}
			items := make([]appie.OrderItem, 0, len(in.Items))
			for _, item := range in.Items {
				if item.ProductID <= 0 {
					return nil, fmt.Errorf("every cart item needs a product_id")
				}
				items = append(items, appie.OrderItem{
					ProductID: item.ProductID,
					Quantity:  max(item.Quantity, 1),
				})
			}
			if err := c.API().AddToOrder(ctx, items); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "added": len(items)}, nil
		})

	addWrite(s, "cart_set_quantity",
		"Change the quantity of a product in the cart. Quantity 0 removes it.",
		func(ctx context.Context, c *ah.Client, in cartQuantityInput) (any, error) {
			if err := c.API().UpdateOrderItem(ctx, in.ProductID, in.Quantity); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true, "productId": in.ProductID, "quantity": in.Quantity}, nil
		})

	addWrite(s, "cart_remove",
		"Remove products from the cart.",
		func(ctx context.Context, c *ah.Client, in cartRemoveInput) (any, error) {
			if len(in.ProductIDs) == 0 {
				return nil, fmt.Errorf("no product ids given")
			}
			for _, id := range in.ProductIDs {
				if err := c.API().RemoveFromOrder(ctx, id); err != nil {
					return nil, err
				}
			}
			return map[string]any{"ok": true, "removed": len(in.ProductIDs)}, nil
		})

	addWrite(s, "cart_clear",
		"Empty the online cart. Show the user what is in it before doing this.",
		func(ctx context.Context, c *ah.Client, _ confirmOnlyInput) (any, error) {
			if err := c.API().ClearOrder(ctx); err != nil {
				return nil, err
			}
			return map[string]any{"ok": true}, nil
		})
}

// toListItems converts tool input into library items, rejecting entries that
// name neither a product nor a note.
func toListItems(inputs []listItemInput) ([]appie.ListItem, error) {
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no items given")
	}

	items := make([]appie.ListItem, 0, len(inputs))
	for i, in := range inputs {
		quantity := max(in.Quantity, 1)
		switch {
		case strings.TrimSpace(in.Text) != "":
			items = append(items, appie.ListItem{Name: in.Text, Quantity: quantity})
		case in.ProductID > 0:
			items = append(items, appie.ListItem{ProductID: in.ProductID, Quantity: quantity})
		default:
			return nil, fmt.Errorf("item %d has neither product_id nor text", i)
		}
	}
	return items, nil
}

// refsFrom resolves explicit product ids or a title match into item references.
func refsFrom(ctx context.Context, c *ah.Client, productIDs []int, match string) ([]ah.ItemRef, error) {
	if strings.TrimSpace(match) != "" {
		entries, err := c.FindInList(ctx, match)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("no lines on the shopping list match %q", match)
		}
		refs := make([]ah.ItemRef, 0, len(entries))
		for _, entry := range entries {
			refs = append(refs, entry.Ref())
		}
		return refs, nil
	}

	if len(productIDs) == 0 {
		return nil, fmt.Errorf("pass product_ids or match")
	}
	refs := make([]ah.ItemRef, 0, len(productIDs))
	for _, id := range productIDs {
		refs = append(refs, ah.ItemRef{ProductID: id})
	}
	return refs, nil
}
