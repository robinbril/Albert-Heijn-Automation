package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"

	appie "github.com/gwillem/appie-go"
	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

func catalogCommands() []command {
	var (
		limit     int
		bonusOnly bool
		nutrition bool
		hydrate   bool
		page      int
		segment   string
		postal    string
	)

	return []command{
		{
			name:    "search",
			args:    "<query>",
			summary: "Search products",
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "limit", 10, "maximum results")
				fs.BoolVar(&bonusOnly, "bonus", false, "only products currently on bonus")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				query, err := requireArg(fs, 0, "query")
				if err != nil {
					return nil, err
				}
				return c.API().SearchProductsFiltered(ctx, appie.SearchOptions{
					Query: query,
					Limit: limit,
					Bonus: bonusOnly,
				})
			},
		},
		{
			name:    "product",
			args:    "<id>",
			summary: "Product details, optionally with nutrition",
			flags: func(fs *flag.FlagSet) {
				fs.BoolVar(&nutrition, "nutrition", false, "include nutritional information")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id, err := intArg(fs, 0, "id")
				if err != nil {
					return nil, err
				}
				if nutrition {
					return c.API().GetProductFull(ctx, id)
				}
				return c.API().GetProduct(ctx, id)
			},
		},
		{
			name:    "products",
			args:    "<id> [id...]",
			summary: "Look up several products at once",
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				ids, err := intArgs(fs, "id")
				if err != nil {
					return nil, err
				}
				return c.API().GetProductsByIDs(ctx, ids)
			},
		},
		{
			name:    "bonus",
			summary: "This week's bonus folder",
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "limit", 0, "maximum results, 0 for all")
				fs.BoolVar(&bonusOnly, "spotlight", false, "only the highlighted deals")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				if bonusOnly {
					products, err := c.API().GetSpotlightBonusProducts(ctx)
					if err != nil {
						return nil, err
					}
					if limit > 0 && len(products) > limit {
						products = products[:limit]
					}
					return map[string]any{"count": len(products), "products": products}, nil
				}

				folder, err := c.GetBonusFolder(ctx)
				if err != nil {
					return nil, err
				}
				if limit > 0 && len(folder.Products) > limit {
					folder.Products = folder.Products[:limit]
				}
				return folder, nil
			},
		},
		{
			name:    "bonus-group",
			args:    "<segment-id>",
			summary: "Resolve a bonus group (e.g. \"alle Hak\") into products",
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&segment, "segment", "", "bonus segment id, alternative to the positional argument")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				id := segment
				if id == "" {
					var err error
					if id, err = requireArg(fs, 0, "segment-id"); err != nil {
						return nil, err
					}
				}
				return c.API().GetBonusGroupProducts(ctx, id)
			},
		},
		{
			name:    "bonus-matches",
			summary: "Bonus deals on products you already buy",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "history", 200, "how much purchase history to match against")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.BonusForHistory(ctx, limit)
			},
		},
		{
			name:    "previously-bought",
			summary: "Products you have bought before",
			auth:    authRequired,
			flags: func(fs *flag.FlagSet) {
				fs.IntVar(&limit, "size", 100, "page size, max 200")
				fs.IntVar(&page, "page", 0, "page number, zero based")
				fs.BoolVar(&hydrate, "prices", false, "enrich with current prices and bonus state")
			},
			run: func(ctx context.Context, c *ah.Client, _ *flag.FlagSet) (any, error) {
				return c.PreviouslyBought(ctx, limit, page, hydrate)
			},
		},
		{
			name:    "stores",
			args:    "<postal-code>",
			summary: "Find AH stores near a postal code",
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				code, err := requireArg(fs, 0, "postal-code")
				if err != nil {
					return nil, err
				}
				return c.API().SearchStores(ctx, code)
			},
		},
		{
			name:    "bargains",
			args:    "[store-id]",
			summary: "Last-chance discounts (koopjes) per store",
			flags: func(fs *flag.FlagSet) {
				fs.StringVar(&postal, "postal", "", "search stores near this postal code instead of a store id")
				fs.IntVar(&limit, "stores", 3, "how many nearby stores to check when using -postal")
			},
			run: func(ctx context.Context, c *ah.Client, fs *flag.FlagSet) (any, error) {
				if postal != "" {
					return c.BargainsNearby(ctx, postal, limit)
				}
				id, err := intArg(fs, 0, "store-id")
				if err != nil {
					return nil, fmt.Errorf("%w (or pass -postal <code>)", err)
				}
				return c.API().GetBargains(ctx, id)
			},
		},
	}
}

// intArg parses the nth positional argument as an integer. Unlike the previous
// version of this tool, a value that is not a number is an error rather than a
// silent zero, which used to turn "add-to-list abc" into product 0.
func intArg(fs *flag.FlagSet, n int, name string) (int, error) {
	raw, err := requireArg(fs, n, name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, raw)
	}
	return value, nil
}

// intArgs parses every positional argument as an integer.
func intArgs(fs *flag.FlagSet, name string) ([]int, error) {
	if fs.NArg() == 0 {
		return nil, fmt.Errorf("missing required argument: %s", name)
	}
	values := make([]int, 0, fs.NArg())
	for i := range fs.NArg() {
		value, err := intArg(fs, i, name)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}
