package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

func historyCommands() []command {
	var (
		limit      int
		kind       string
		includePII bool
		top        int
	)

	return []command{
		{
			name:    "purchases",
			summary: "Recent orders and in-store receipts as one timeline",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "limit", 20, "how many purchases to return")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				purchases, err := c.RecentPurchases(ctx, limit)
				if err != nil {
					return nil, err
				}
				return map[string]any{"count": len(purchases), "purchases": purchases}, nil
			},
		},
		{
			name:    "purchase",
			args:    "<id>",
			summary: "Read one order or receipt line by line, with prices",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&kind, "kind", string(ah.PurchaseReceipt), "receipt or order")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id, err := requireArg(fs, 0, "id")
				if err != nil {
					return nil, err
				}
				return c.PurchaseDetail(ctx, ah.PurchaseKind(kind), id)
			},
		},
		{
			name:    "ordered-items",
			summary: "What you buy most often across recent purchases",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "purchases", 10, "how many purchases to read")
				fs.IntVar(&top, "top", 0, "only return the top N items, 0 for all")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				items, err := c.OrderedItems(ctx, limit)
				if err != nil {
					return nil, err
				}
				if top > 0 && len(items) > top {
					items = items[:top]
				}
				return map[string]any{"count": len(items), "items": items}, nil
			},
		},
		{
			name:    "spend",
			summary: "What you spent, per month and per purchase type",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "limit", 50, "how many purchases to include")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				summary, err := c.Spend(ctx, limit)
				if err != nil {
					return nil, err
				}
				// Emit months in order so the output reads as a series.
				months := make([]map[string]any, 0, len(summary.ByMonth))
				for _, month := range sortedKeys(summary.ByMonth) {
					months = append(months, map[string]any{"month": month, "spend": summary.ByMonth[month]})
				}
				return map[string]any{
					"purchases": summary.Purchases,
					"total":     summary.Total,
					"from":      summary.From,
					"to":        summary.To,
					"byKind":    summary.ByKind,
					"byMonth":   months,
				}, nil
			},
		},
		{
			name:    "patterns",
			summary: "Read your receipts to derive eating and shopping patterns",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "purchases", 20, "how many purchases to analyse")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.Patterns(ctx, limit)
			},
		},
		{
			name:    "member",
			summary: "Your AH profile and the segments AH sorted you into",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&includePII, "pii", false, "include name, email, phone, address and card numbers")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				profile, err := c.GetMemberProfile(ctx, includePII)
				if err != nil {
					return nil, err
				}
				if !includePII {
					fmt.Fprintln(os.Stderr, "note: identifying fields withheld; pass -pii to include them")
				}
				return profile, nil
			},
		},
	}
}
