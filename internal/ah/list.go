package ah

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	appie "github.com/gwillem/appie-go"
)

// shoppingListPath is the v2 endpoint backing the list in the AH app.
// appie-go reads the favourite lists (v3) instead, which is a different
// collection: items added to the app's main list only appear here. The
// ordering parameters mirror what the app sends; without them the API returns
// the list in an unstable order.
const shoppingListPath = "/mobile-services/shoppinglist/v2/items?orderBy=userInput&orderByParam=0"

// ItemRef identifies a line on the shopping list, either by product or by the
// text of a free-text note.
type ItemRef struct {
	ProductID int    `json:"productId,omitempty"`
	Text      string `json:"text,omitempty"`
}

// ListEntry is one line on the main shopping list.
type ListEntry struct {
	ListItemID int `json:"listItemId,omitempty"`
	// ProductID is 0 for free-text entries.
	ProductID int     `json:"productId,omitempty"`
	Title     string  `json:"title"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unitPrice,omitempty"`
	LinePrice float64 `json:"linePrice,omitempty"`
	// PriceBeforeBonus is the undiscounted unit price.
	PriceBeforeBonus float64 `json:"priceBeforeBonus,omitempty"`
	IsBonus          bool    `json:"isBonus,omitempty"`
	// Checked is true for items struck through in the app.
	Checked bool `json:"checked"`
	// FreeText marks notes that are not linked to a product.
	FreeText bool `json:"freeText,omitempty"`
}

// Ref returns the reference used to update or remove this entry.
func (e ListEntry) Ref() ItemRef {
	if e.ProductID > 0 {
		return ItemRef{ProductID: e.ProductID}
	}
	return ItemRef{Text: e.Title}
}

// ShoppingList is the main list with its totals.
type ShoppingList struct {
	Items []ListEntry `json:"items"`
	Count int         `json:"count"`
	// TotalBeforeBonus is what the list would cost without promotions.
	TotalBeforeBonus float64 `json:"totalBeforeBonus"`
	// TotalDiscount is the amount saved by current promotions.
	TotalDiscount float64 `json:"totalDiscount"`
	// Total is what the list costs today.
	Total float64 `json:"total"`
	// CheckedCount counts struck-through items, which are excluded from totals.
	CheckedCount int `json:"checkedCount"`
}

// listItemsResponse is the raw v2 shopping list payload.
type listItemsResponse struct {
	ID    string `json:"id"`
	Items []struct {
		ListItemID     int    `json:"listItemId"`
		Quantity       int    `json:"quantity"`
		Strikedthrough bool   `json:"strikedthrough"`
		Description    string `json:"description"`
		ProductDetails *struct {
			Product struct {
				WebshopID        int     `json:"webshopId"`
				Title            string  `json:"title"`
				CurrentPrice     float64 `json:"currentPrice"`
				PriceBeforeBonus float64 `json:"priceBeforeBonus"`
				IsBonus          bool    `json:"isBonus"`
			} `json:"product"`
		} `json:"productDetails"`
	} `json:"items"`
}

// patchItem is the v2 update payload. Quantity 0 removes a line; that is the
// only real delete the API offers. StrikeThrough merely crosses an item off.
type patchItem struct {
	Description   string `json:"description,omitempty"`
	ProductID     int    `json:"productId,omitempty"`
	Quantity      int    `json:"quantity"`
	Type          string `json:"type"`
	OriginCode    string `json:"originCode"`
	StrikeThrough bool   `json:"strikeThrough"`
}

func newPatchItem(ref ItemRef, quantity int, strike bool) patchItem {
	item := patchItem{
		Quantity:      quantity,
		Type:          "SHOPPABLE",
		OriginCode:    "PRD",
		StrikeThrough: strike,
	}
	if ref.ProductID > 0 {
		item.ProductID = ref.ProductID
	} else {
		item.Description = ref.Text
	}
	return item
}

func (c *Client) patchList(ctx context.Context, items []patchItem) (*listItemsResponse, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no items to update")
	}
	var updated listItemsResponse
	body := map[string]any{"items": items}
	if err := c.api.DoRequest(ctx, http.MethodPatch, shoppingListPath, body, &updated); err != nil {
		return nil, fmt.Errorf("update shopping list failed: %w", err)
	}
	return &updated, nil
}

// GetShoppingList reads the main shopping list with prices and totals.
// Struck-through items are returned but excluded from the totals: they are the
// items already picked.
func (c *Client) GetShoppingList(ctx context.Context) (*ShoppingList, error) {
	var raw listItemsResponse
	if err := c.api.DoRequest(ctx, http.MethodGet, shoppingListPath, nil, &raw); err != nil {
		return nil, fmt.Errorf("get shopping list failed: %w", err)
	}
	return summarise(&raw), nil
}

// summarise converts a raw v2 payload into a priced list.
func summarise(raw *listItemsResponse) *ShoppingList {
	list := &ShoppingList{Items: make([]ListEntry, 0, len(raw.Items))}

	for _, item := range raw.Items {
		entry := ListEntry{
			ListItemID: item.ListItemID,
			Title:      item.Description,
			Quantity:   max(item.Quantity, 1),
			Checked:    item.Strikedthrough,
			FreeText:   true,
		}

		if item.ProductDetails != nil && item.ProductDetails.Product.WebshopID > 0 {
			product := item.ProductDetails.Product
			unitPrice := product.CurrentPrice
			if unitPrice == 0 {
				unitPrice = product.PriceBeforeBonus
			}
			entry.FreeText = false
			entry.ProductID = product.WebshopID
			entry.UnitPrice = unitPrice
			entry.LinePrice = unitPrice * float64(entry.Quantity)
			entry.PriceBeforeBonus = product.PriceBeforeBonus
			entry.IsBonus = product.IsBonus
			if product.Title != "" {
				entry.Title = product.Title
			}
		}

		list.Items = append(list.Items, entry)

		if entry.Checked {
			list.CheckedCount++
			continue
		}
		list.Count++
		list.Total += entry.LinePrice
		before := entry.PriceBeforeBonus
		if before == 0 {
			before = entry.UnitPrice
		}
		list.TotalBeforeBonus += before * float64(entry.Quantity)
	}

	list.TotalDiscount = list.TotalBeforeBonus - list.Total
	return list
}

// AddToList adds products and free-text notes to the main shopping list.
func (c *Client) AddToList(ctx context.Context, items []appie.ListItem) error {
	if len(items) == 0 {
		return fmt.Errorf("no items to add")
	}
	if err := c.api.AddToShoppingList(ctx, items); err != nil {
		return fmt.Errorf("add to shopping list failed: %w", err)
	}
	return nil
}

// SetQuantity changes the amount of a line and returns the line as it stands
// afterwards. Quantity 0 removes it; use Remove for that so the intent is
// explicit at the call site.
//
// The result is read back because the API answers 200 to updates it does not
// apply: quantity changes on free-text notes are accepted and ignored. Without
// the check this reported a success that never happened.
func (c *Client) SetQuantity(ctx context.Context, ref ItemRef, quantity int) (*ListEntry, error) {
	if quantity < 1 {
		return nil, fmt.Errorf("quantity must be at least 1; use Remove to delete a line")
	}
	if ref.ProductID <= 0 && strings.TrimSpace(ref.Text) == "" {
		return nil, fmt.Errorf("item reference needs a productId or text")
	}

	updated, err := c.patchList(ctx, []patchItem{newPatchItem(ref, quantity, false)})
	if err != nil {
		return nil, err
	}

	want := refKey(ref)
	for _, entry := range summarise(updated).Items {
		if refKey(entry.Ref()) != want {
			continue
		}
		if entry.Quantity != quantity {
			return &entry, fmt.Errorf("albert heijn kept the quantity at %d instead of %d; it ignores quantity changes on free-text notes", entry.Quantity, quantity)
		}
		return &entry, nil
	}
	return nil, fmt.Errorf("line not found on the list after the update")
}

// Remove deletes lines from the shopping list. This sends quantity 0, which is
// a real delete: striking an item through only crosses it off and leaves it on
// the list. Returns how many lines remain.
func (c *Client) Remove(ctx context.Context, refs []ItemRef) (remaining int, err error) {
	if len(refs) == 0 {
		return 0, fmt.Errorf("no items to remove")
	}

	items := make([]patchItem, 0, len(refs))
	for _, ref := range refs {
		if ref.ProductID <= 0 && strings.TrimSpace(ref.Text) == "" {
			continue
		}
		items = append(items, newPatchItem(ref, 0, false))
	}
	if len(items) == 0 {
		return 0, fmt.Errorf("no valid item references")
	}

	updated, err := c.patchList(ctx, items)
	if err != nil {
		return 0, err
	}
	return len(updated.Items), nil
}

// CheckOff crosses items off the list, or restores them when checked is false.
// Unlike Remove, the lines stay on the list.
func (c *Client) CheckOff(ctx context.Context, refs []ItemRef, checked bool) error {
	if len(refs) == 0 {
		return fmt.Errorf("no items to check off")
	}

	current, err := c.GetShoppingList(ctx)
	if err != nil {
		return err
	}
	quantities := make(map[string]int, len(current.Items))
	for _, item := range current.Items {
		quantities[refKey(item.Ref())] = item.Quantity
	}

	items := make([]patchItem, 0, len(refs))
	for _, ref := range refs {
		quantity := 1
		if q, ok := quantities[refKey(ref)]; ok {
			quantity = q
		}
		items = append(items, newPatchItem(ref, quantity, checked))
	}

	_, err = c.patchList(ctx, items)
	return err
}

// FindInList returns every line whose title contains term, crossed off or not.
//
// Checked lines are included on purpose: skipping them stranded them, because
// restoring or deleting a line first requires finding it.
func (c *Client) FindInList(ctx context.Context, term string) ([]ListEntry, error) {
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return nil, fmt.Errorf("empty search term")
	}

	list, err := c.GetShoppingList(ctx)
	if err != nil {
		return nil, err
	}

	matches := make([]ListEntry, 0)
	for _, item := range list.Items {
		if strings.Contains(strings.ToLower(item.Title), needle) {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

// ClearList removes every line from the main shopping list.
func (c *Client) ClearList(ctx context.Context) (removed int, remaining int, err error) {
	list, err := c.GetShoppingList(ctx)
	if err != nil {
		return 0, 0, err
	}

	refs := make([]ItemRef, 0, len(list.Items))
	for _, item := range list.Items {
		refs = append(refs, item.Ref())
	}
	if len(refs) == 0 {
		return 0, 0, nil
	}

	remaining, err = c.Remove(ctx, refs)
	if err != nil {
		return 0, 0, err
	}
	return len(refs), remaining, nil
}

// ListToOrder copies every unchecked product from the shopping list into the
// order (the cart). Free-text notes are skipped: they have no product to order.
func (c *Client) ListToOrder(ctx context.Context) (added int, skipped int, err error) {
	list, err := c.GetShoppingList(ctx)
	if err != nil {
		return 0, 0, err
	}

	items := make([]appie.OrderItem, 0, len(list.Items))
	for _, item := range list.Items {
		if item.Checked {
			continue
		}
		if item.ProductID <= 0 {
			skipped++
			continue
		}
		items = append(items, appie.OrderItem{ProductID: item.ProductID, Quantity: item.Quantity})
	}
	if len(items) == 0 {
		return 0, skipped, nil
	}
	if err := c.api.AddToOrder(ctx, items); err != nil {
		return 0, skipped, fmt.Errorf("add to order failed: %w", err)
	}
	return len(items), skipped, nil
}

func refKey(ref ItemRef) string {
	if ref.ProductID > 0 {
		return fmt.Sprintf("p:%d", ref.ProductID)
	}
	return "t:" + strings.ToLower(strings.TrimSpace(ref.Text))
}
