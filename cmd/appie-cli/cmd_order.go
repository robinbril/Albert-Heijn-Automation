package main

import (
	"context"
	"flag"

	appie "github.com/gwillem/appie-go"
	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

func orderCommands() []command {
	var quantity int

	return []command{
		{
			name:    "order",
			summary: "Show the current order (the cart)",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.API().GetOrder(ctx)
			},
		},
		{
			name:    "order-summary",
			summary: "Totals and discount for the current order",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.API().GetOrderSummary(ctx)
			},
		},
		{
			name:    "order-add",
			args:    "<product-id> [product-id...]",
			summary: "Add products to the cart",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&quantity, "qty", 1, "quantity per product")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				ids, err := intArgs(fs, "product-id")
				if err != nil {
					return nil, err
				}
				items := make([]appie.OrderItem, 0, len(ids))
				for _, id := range ids {
					items = append(items, appie.OrderItem{ProductID: id, Quantity: max(quantity, 1)})
				}
				if err := c.API().AddToOrder(ctx, items); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "added": len(items)}, nil
			},
		},
		{
			name:    "order-qty",
			args:    "<product-id> <quantity>",
			summary: "Change the quantity of a product in the cart",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id, err := intArg(fs, 0, "product-id")
				if err != nil {
					return nil, err
				}
				qty, err := intArg(fs, 1, "quantity")
				if err != nil {
					return nil, err
				}
				if err := c.API().UpdateOrderItem(ctx, id, qty); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true, "productId": id, "quantity": qty}, nil
			},
		},
		{
			name:    "order-remove",
			args:    "<product-id> [product-id...]",
			summary: "Remove products from the cart",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				ids, err := intArgs(fs, "product-id")
				if err != nil {
					return nil, err
				}
				for _, id := range ids {
					if err := c.API().RemoveFromOrder(ctx, id); err != nil {
						return nil, err
					}
				}
				return map[string]any{"ok": true, "removed": len(ids)}, nil
			},
		},
		{
			name:    "order-clear",
			summary: "Empty the cart",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				if err := c.API().ClearOrder(ctx); err != nil {
					return nil, err
				}
				return map[string]any{"ok": true}, nil
			},
		},
		{
			name:    "list-to-order",
			summary: "Copy the shopping list into the cart",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				added, skipped, err := c.ListToOrder(ctx)
				if err != nil {
					return nil, err
				}
				return map[string]any{
					"ok":      true,
					"added":   added,
					"skipped": skipped,
					"note":    "free-text notes have no product and were skipped; pay and pick a delivery slot in the AH app",
				}, nil
			},
		},
		{
			name:    "deliveries",
			summary: "Scheduled orders with their delivery slots",
			auth:    authRequired,
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.API().GetFulfillments(ctx)
			},
		},
	}
}
