package main

import (
	"context"
	"fmt"
	"os"

	appie "github.com/gwillem/appie-go"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/robinbril/openclaw-skill-albert-heijn/internal/ah"
)

type emptyInput struct{}

type searchInput struct {
	Query     string `json:"query" jsonschema:"What to search for, in Dutch, e.g. 'halfvolle melk'."`
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of products. Defaults to 10."`
	BonusOnly bool   `json:"bonus_only,omitempty" jsonschema:"Only return products currently on promotion."`
}

type productInput struct {
	ID        int  `json:"id" jsonschema:"Albert Heijn webshop product id."`
	Nutrition bool `json:"nutrition,omitempty" jsonschema:"Include nutritional values."`
}

type productsInput struct {
	IDs []int `json:"ids" jsonschema:"Webshop product ids to look up in one call."`
}

type bonusInput struct {
	Limit     int  `json:"limit,omitempty" jsonschema:"Maximum number of products. 0 returns the whole folder."`
	Spotlight bool `json:"spotlight,omitempty" jsonschema:"Only the highlighted deals instead of the full folder."`
}

type bonusGroupInput struct {
	SegmentID string `json:"segment_id" jsonschema:"Bonus segment id from a group entry such as 'Alle Alpro'."`
}

type historyInput struct {
	Size       int  `json:"size,omitempty" jsonschema:"Page size, max 200. Defaults to 100."`
	Page       int  `json:"page,omitempty" jsonschema:"Zero based page number."`
	WithPrices bool `json:"with_prices,omitempty" jsonschema:"Enrich with current prices and bonus state."`
}

type matchesInput struct {
	HistorySize int `json:"history_size,omitempty" jsonschema:"How much purchase history to match against. Defaults to 200."`
}

type recipeSearchInput struct {
	Query          string `json:"query,omitempty" jsonschema:"Search term in Dutch. Empty returns popular recipes."`
	Limit          int    `json:"limit,omitempty" jsonschema:"Maximum number of recipes. Defaults to 10."`
	Start          int    `json:"start,omitempty" jsonschema:"Offset into the results, for paging."`
	MaxCookMinutes int    `json:"max_cook_minutes,omitempty" jsonschema:"Drop recipes whose active cooking time exceeds this."`
}

type recipeInput struct {
	ID int `json:"id" jsonschema:"Allerhande recipe id."`
}

type planInput struct {
	RecipeIDs []int `json:"recipe_ids" jsonschema:"Recipe ids to combine into one deduplicated ingredient list."`
}

type listIDInput struct {
	ListID string `json:"list_id" jsonschema:"Favourite list id."`
}

type storesInput struct {
	PostalCode string `json:"postal_code" jsonschema:"Dutch postal code, e.g. '1012AB'."`
}

type bargainsInput struct {
	StoreID    int    `json:"store_id,omitempty" jsonschema:"Store id. Use this or postal_code."`
	PostalCode string `json:"postal_code,omitempty" jsonschema:"Check the stores nearest this postal code."`
	Stores     int    `json:"stores,omitempty" jsonschema:"How many nearby stores to check when using postal_code. Defaults to 3."`
}

type purchasesInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"How many purchases to return. Defaults to 20."`
}

type purchaseInput struct {
	ID   string `json:"id" jsonschema:"Order id or receipt transaction id."`
	Kind string `json:"kind" jsonschema:"Either 'order' for an online order or 'receipt' for an in-store receipt."`
}

type orderedItemsInput struct {
	Purchases int `json:"purchases,omitempty" jsonschema:"How many purchases to read. Defaults to 10."`
	Top       int `json:"top,omitempty" jsonschema:"Only return the top N items. 0 returns all."`
}

type patternsInput struct {
	Purchases int `json:"purchases,omitempty" jsonschema:"How many purchases to analyse. Defaults to 20."`
}

type memberInput struct {
	IncludePersonalDetails bool `json:"include_personal_details,omitempty" jsonschema:"Include name, email, phone, address and loyalty card numbers. Off by default; only ask for these when the user needs them."`
}

func registerReadTools(s *mcp.Server) {
	registerCatalogTools(s)
	registerRecipeTools(s)
	registerHistoryTools(s)

	addRead(s, "shopping_list",
		"Read the Albert Heijn shopping list with prices, per-line totals and the total bill.",
		true, func(ctx context.Context, c *ah.Client, _ emptyInput) (any, error) {
			return c.GetShoppingList(ctx)
		})

	addRead(s, "favourite_lists",
		"List the saved favourite lists (the named lists in the AH app).",
		true, func(ctx context.Context, c *ah.Client, _ emptyInput) (any, error) {
			return c.API().GetShoppingLists(ctx, 0)
		})

	addRead(s, "favourite_list_items",
		"Read the items in one favourite list.",
		true, func(ctx context.Context, c *ah.Client, in listIDInput) (any, error) {
			return c.API().GetShoppingListItems(ctx, in.ListID)
		})

	addRead(s, "cart",
		"Read the current online order (the cart), with its items.",
		true, func(ctx context.Context, c *ah.Client, _ emptyInput) (any, error) {
			return c.API().GetOrder(ctx)
		})

	addRead(s, "cart_summary",
		"Totals and discount for the current cart.",
		true, func(ctx context.Context, c *ah.Client, _ emptyInput) (any, error) {
			return c.API().GetOrderSummary(ctx)
		})

	addRead(s, "deliveries",
		"Scheduled orders with their delivery slots and status.",
		true, func(ctx context.Context, c *ah.Client, _ emptyInput) (any, error) {
			return c.API().GetFulfillments(ctx)
		})

	addRead(s, "auth_status",
		"Check whether an Albert Heijn account is connected and whether writes are allowed.",
		false, func(ctx context.Context, _ *ah.Client, _ emptyInput) (any, error) {
			path, err := ah.ConfigPath()
			if err != nil {
				return nil, err
			}
			status := map[string]any{
				"tokenStore":    path,
				"authenticated": false,
				"readOnly":      os.Getenv(ah.EnvReadOnly) != "",
				"loginCommand":  "appie-cli login",
			}
			client, err := ah.Authenticated()
			if err != nil {
				return status, nil
			}
			status["authenticated"] = true
			status["readOnly"] = client.ReadOnly()
			if _, err := client.GetMemberProfile(ctx, false); err != nil {
				status["tokenValid"] = false
				status["tokenError"] = err.Error()
				return status, nil
			}
			status["tokenValid"] = true
			return status, nil
		})
}

func registerCatalogTools(s *mcp.Server) {
	addRead(s, "search_products",
		"Search the Albert Heijn assortment. Returns prices, bonus state and product ids to use elsewhere.",
		false, func(ctx context.Context, c *ah.Client, in searchInput) (any, error) {
			return c.API().SearchProductsFiltered(ctx, appie.SearchOptions{
				Query: in.Query,
				Limit: in.Limit,
				Bonus: in.BonusOnly,
			})
		})

	addRead(s, "get_product",
		"Look up one product by id, optionally with nutritional values.",
		false, func(ctx context.Context, c *ah.Client, in productInput) (any, error) {
			if in.Nutrition {
				return c.API().GetProductFull(ctx, in.ID)
			}
			return c.API().GetProduct(ctx, in.ID)
		})

	addRead(s, "get_products",
		"Look up several products by id in one call.",
		false, func(ctx context.Context, c *ah.Client, in productsInput) (any, error) {
			return c.API().GetProductsByIDs(ctx, in.IDs)
		})

	addRead(s, "bonus_folder",
		"This week's bonus folder. Group entries such as 'Alle Alpro' have id 0 and a bonusSegmentId; resolve those with bonus_group.",
		false, func(ctx context.Context, c *ah.Client, in bonusInput) (any, error) {
			if in.Spotlight {
				products, err := c.API().GetSpotlightBonusProducts(ctx)
				if err != nil {
					return nil, err
				}
				if in.Limit > 0 && len(products) > in.Limit {
					products = products[:in.Limit]
				}
				return map[string]any{"count": len(products), "products": products}, nil
			}

			folder, err := c.GetBonusFolder(ctx)
			if err != nil {
				return nil, err
			}
			if in.Limit > 0 && len(folder.Products) > in.Limit {
				folder.Products = folder.Products[:in.Limit]
			}
			return folder, nil
		})

	addRead(s, "bonus_group",
		"Resolve a bonus group entry into the individual products it covers.",
		false, func(ctx context.Context, c *ah.Client, in bonusGroupInput) (any, error) {
			return c.API().GetBonusGroupProducts(ctx, in.SegmentID)
		})

	addRead(s, "bonus_matches",
		"Promotions on products the user actually buys, by matching the bonus folder against their purchase history. The best starting point for a weekly plan.",
		true, func(ctx context.Context, c *ah.Client, in matchesInput) (any, error) {
			size := in.HistorySize
			if size <= 0 {
				size = 200
			}
			return c.BonusForHistory(ctx, size)
		})

	addRead(s, "previously_bought",
		"Products the user has bought before. Use with_prices to also get today's price and bonus state.",
		true, func(ctx context.Context, c *ah.Client, in historyInput) (any, error) {
			return c.PreviouslyBought(ctx, in.Size, in.Page, in.WithPrices)
		})

	addRead(s, "find_stores",
		"Find Albert Heijn stores near a Dutch postal code.",
		false, func(ctx context.Context, c *ah.Client, in storesInput) (any, error) {
			return c.API().SearchStores(ctx, in.PostalCode)
		})

	addRead(s, "store_bargains",
		"Last-chance discounts (koopjes) in a physical store: items marked down because they expire soon.",
		false, func(ctx context.Context, c *ah.Client, in bargainsInput) (any, error) {
			if in.PostalCode != "" {
				stores := in.Stores
				if stores <= 0 {
					stores = 3
				}
				return c.BargainsNearby(ctx, in.PostalCode, stores)
			}
			if in.StoreID <= 0 {
				return nil, fmt.Errorf("pass either store_id or postal_code")
			}
			return c.API().GetBargains(ctx, in.StoreID)
		})
}

func registerRecipeTools(s *mcp.Server) {
	addRead(s, "search_recipes",
		"Search Allerhande recipes. Times are split into active cooking time and waiting time.",
		false, func(ctx context.Context, c *ah.Client, in recipeSearchInput) (any, error) {
			recipes, err := c.SearchRecipes(ctx, ah.RecipeSearchOptions{
				Query:       in.Query,
				Limit:       in.Limit,
				Start:       in.Start,
				MaxCookTime: in.MaxCookMinutes,
			})
			if err != nil {
				return nil, err
			}
			return map[string]any{"count": len(recipes), "recipes": recipes}, nil
		})

	addRead(s, "get_recipe",
		"Read one recipe in full: servings, ingredients with amounts, and the preparation steps.",
		false, func(ctx context.Context, c *ah.Client, in recipeInput) (any, error) {
			return c.GetRecipe(ctx, in.ID)
		})

	addRead(s, "plan_ingredients",
		"Combine several recipes into one deduplicated ingredient list, summing amounts per unit. Use this before adding a week of meals to the shopping list, so shared ingredients are not added twice.",
		false, func(ctx context.Context, c *ah.Client, in planInput) (any, error) {
			if len(in.RecipeIDs) == 0 {
				return nil, fmt.Errorf("pass at least one recipe id")
			}
			recipes := make([]ah.Recipe, 0, len(in.RecipeIDs))
			for _, id := range in.RecipeIDs {
				recipe, err := c.GetRecipe(ctx, id)
				if err != nil {
					return nil, err
				}
				recipes = append(recipes, *recipe)
			}

			merged := ah.MergeIngredients(recipes)
			titles := make([]string, 0, len(recipes))
			for _, r := range recipes {
				titles = append(titles, r.Title)
			}
			return map[string]any{
				"recipes":         titles,
				"ingredientCount": len(merged),
				"ingredients":     merged,
			}, nil
		})
}

func registerHistoryTools(s *mcp.Server) {
	addRead(s, "purchases",
		"Recent online orders and in-store receipts as one timeline, newest first.",
		true, func(ctx context.Context, c *ah.Client, in purchasesInput) (any, error) {
			purchases, err := c.RecentPurchases(ctx, in.Limit)
			if err != nil {
				return nil, err
			}
			return map[string]any{"count": len(purchases), "purchases": purchases}, nil
		})

	addRead(s, "purchase_detail",
		"Read one order or receipt line by line, with quantities and prices.",
		true, func(ctx context.Context, c *ah.Client, in purchaseInput) (any, error) {
			return c.PurchaseDetail(ctx, ah.PurchaseKind(in.Kind), in.ID)
		})

	addRead(s, "ordered_items",
		"What the user buys most often across recent purchases, with how often and how much they spent on each.",
		true, func(ctx context.Context, c *ah.Client, in orderedItemsInput) (any, error) {
			items, err := c.OrderedItems(ctx, in.Purchases)
			if err != nil {
				return nil, err
			}
			if in.Top > 0 && len(items) > in.Top {
				items = items[:in.Top]
			}
			return map[string]any{"count": len(items), "items": items}, nil
		})

	addRead(s, "spend",
		"What the user spent, split per month and per purchase type.",
		true, func(ctx context.Context, c *ah.Client, in purchasesInput) (any, error) {
			return c.Spend(ctx, in.Limit)
		})

	addRead(s, "eating_patterns",
		"Derive eating and shopping patterns from receipts: shopping cadence, favourite shopping day, category mix, staples versus one-offs.",
		true, func(ctx context.Context, c *ah.Client, in patternsInput) (any, error) {
			return c.Patterns(ctx, in.Purchases)
		})

	addRead(s, "member_profile",
		"The user's Albert Heijn profile, including the segments and food profile AH derived about them. Personal details are withheld unless explicitly requested.",
		true, func(ctx context.Context, c *ah.Client, in memberInput) (any, error) {
			return c.GetMemberProfile(ctx, in.IncludePersonalDetails)
		})
}
