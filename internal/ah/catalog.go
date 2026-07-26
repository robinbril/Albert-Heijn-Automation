package ah

import (
	"context"
	"fmt"
	"sort"

	appie "github.com/gwillem/appie-go"
)

// PreviouslyBoughtPage is one page of the purchase history.
type PreviouslyBoughtPage struct {
	Products      []appie.Product `json:"products"`
	Page          int             `json:"page"`
	Size          int             `json:"size"`
	TotalElements int             `json:"totalElements"`
	TotalPages    int             `json:"totalPages"`
}

// PreviouslyBought returns products the member has bought before. This uses the
// undocumented previouslyBought flag on productSearch; appie-go does not expose it.
//
// When hydrate is true the results are enriched with current prices and bonus
// state via a single REST lookup, which is what makes history usable for
// matching against this week's promotions.
func (c *Client) PreviouslyBought(ctx context.Context, size, page int, hydrate bool) (*PreviouslyBoughtPage, error) {
	if size <= 0 || size > 200 {
		size = 100
	}
	if page < 0 {
		page = 0
	}

	// size and page are validated ints, so inlining them cannot inject.
	query := fmt.Sprintf(`query PreviouslyBought {
  productSearch(input: { query: "" previouslyBought: true size: %d page: %d }) {
    products { id title brand category }
    page { totalElements totalPages }
  }
}`, size, page)

	var resp struct {
		ProductSearch struct {
			Products []appie.Product `json:"products"`
			Page     struct {
				TotalElements int `json:"totalElements"`
				TotalPages    int `json:"totalPages"`
			} `json:"page"`
		} `json:"productSearch"`
	}
	if err := c.api.DoGraphQL(ctx, query, nil, &resp); err != nil {
		return nil, fmt.Errorf("previously bought failed: %w", err)
	}

	result := &PreviouslyBoughtPage{
		Products:      resp.ProductSearch.Products,
		Page:          page,
		Size:          size,
		TotalElements: resp.ProductSearch.Page.TotalElements,
		TotalPages:    resp.ProductSearch.Page.TotalPages,
	}

	if hydrate && len(result.Products) > 0 {
		hydrated, err := c.hydrate(ctx, result.Products)
		if err != nil {
			return nil, err
		}
		result.Products = hydrated
	}

	return result, nil
}

// hydrate replaces bare GraphQL products with full REST product records.
// Products the REST endpoint does not return are kept as-is rather than dropped.
func (c *Client) hydrate(ctx context.Context, products []appie.Product) ([]appie.Product, error) {
	ids := make([]int, 0, len(products))
	for _, p := range products {
		if p.ID > 0 {
			ids = append(ids, p.ID)
		}
	}
	if len(ids) == 0 {
		return products, nil
	}

	full, err := c.api.GetProductsByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("hydrate products failed: %w", err)
	}

	byID := make(map[int]appie.Product, len(full))
	for _, p := range full {
		byID[p.ID] = p
	}

	out := make([]appie.Product, 0, len(products))
	for _, p := range products {
		if enriched, ok := byID[p.ID]; ok {
			out = append(out, enriched)
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// BonusMatch is a promotion on a product the member already buys.
type BonusMatch struct {
	appie.Product
	// Discount is the amount saved per unit in EUR, 0 when unknown.
	Discount float64 `json:"discount"`
}

// BonusMatches pairs the deals worth proposing with the categories that could
// not be read, so a caller can tell "no deals" apart from "half the folder
// failed to load".
type BonusMatches struct {
	Count   int               `json:"count"`
	Matches []BonusMatch      `json:"matches"`
	Skipped []SkippedCategory `json:"skipped,omitempty"`
}

// BonusForHistory cross-references the current bonus folder with the member's
// purchase history. These are the deals actually worth proposing.
func (c *Client) BonusForHistory(ctx context.Context, historySize int) (*BonusMatches, error) {
	history, err := c.PreviouslyBought(ctx, historySize, 0, false)
	if err != nil {
		return nil, err
	}

	bought := make(map[int]bool, len(history.Products))
	for _, p := range history.Products {
		bought[p.ID] = true
	}

	folder, err := c.GetBonusFolder(ctx)
	if err != nil {
		return nil, err
	}

	result := &BonusMatches{
		Matches: make([]BonusMatch, 0),
		Skipped: folder.Skipped,
	}
	for _, p := range folder.Products {
		if p.ID == 0 || !bought[p.ID] {
			continue
		}
		var discount float64
		if p.Price.Was > 0 && p.Price.Now > 0 {
			discount = p.Price.Was - p.Price.Now
		}
		result.Matches = append(result.Matches, BonusMatch{Product: p, Discount: discount})
	}

	sort.SliceStable(result.Matches, func(i, j int) bool {
		return result.Matches[i].Discount > result.Matches[j].Discount
	})
	result.Count = len(result.Matches)
	return result, nil
}

// StoreBargains pairs a store with its last-chance discounts.
type StoreBargains struct {
	Store    appie.Store     `json:"store"`
	Bargains []appie.Bargain `json:"bargains"`
}

// BargainsNearby finds the stores around a postal code and returns the
// last-chance discounts ("koopjes") for each. Passing limit <= 0 covers every
// store the search returns.
func (c *Client) BargainsNearby(ctx context.Context, postalCode string, limit int) ([]StoreBargains, error) {
	stores, err := c.api.SearchStores(ctx, postalCode)
	if err != nil {
		return nil, fmt.Errorf("store search failed: %w", err)
	}
	if limit > 0 && len(stores) > limit {
		stores = stores[:limit]
	}

	out := make([]StoreBargains, 0, len(stores))
	for _, store := range stores {
		bargains, err := c.api.GetBargains(ctx, store.ID)
		if err != nil {
			// One store without a bargain feed must not sink the whole answer.
			out = append(out, StoreBargains{Store: store, Bargains: nil})
			continue
		}
		out = append(out, StoreBargains{Store: store, Bargains: bargains})
	}
	return out, nil
}
