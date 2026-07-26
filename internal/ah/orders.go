package ah

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	appie "github.com/gwillem/appie-go"
)

// PurchaseKind distinguishes an online order from an in-store receipt.
type PurchaseKind string

const (
	// PurchaseOrder is an online order (bezorging or ophalen).
	PurchaseOrder PurchaseKind = "order"
	// PurchaseReceipt is an in-store receipt (kassabon).
	PurchaseReceipt PurchaseKind = "receipt"
)

// Purchase is one entry in the purchase timeline.
type Purchase struct {
	Kind PurchaseKind `json:"kind"`
	// ID identifies the purchase for PurchaseDetail.
	ID string `json:"id"`
	// Date is the delivery date for orders, the transaction date for receipts.
	Date  string  `json:"date"`
	Total float64 `json:"total"`
	// Status is the order status; empty for receipts.
	Status string `json:"status,omitempty"`
	// Slot is the delivery window for orders.
	Slot string `json:"slot,omitempty"`
	// ShoppingType is DELIVERY or PICKUP for orders.
	ShoppingType string `json:"shoppingType,omitempty"`
	// Modifiable reports whether an order can still be changed.
	Modifiable bool `json:"modifiable,omitempty"`
}

// PurchaseLine is a single product on an order or receipt.
type PurchaseLine struct {
	Description string  `json:"description"`
	ProductID   int     `json:"productId,omitempty"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
	Amount      float64 `json:"amount"`
}

// PurchaseDetail is a purchase with its lines.
type PurchaseDetail struct {
	Purchase
	Lines []PurchaseLine `json:"lines"`
	// Discounts are the promotions applied, receipts only.
	Discounts []appie.ReceiptDiscount `json:"discounts,omitempty"`
	// Payments are the payment methods used, receipts only.
	Payments []appie.ReceiptPayment `json:"payments,omitempty"`
	// Subtotal is the sum of the lines before discounts.
	Subtotal float64 `json:"subtotal,omitempty"`
}

// RecentPurchases returns online orders and in-store receipts as one timeline,
// newest first. A failure on one source does not hide the other: a member who
// only shops in store has no orders, and vice versa.
func (c *Client) RecentPurchases(ctx context.Context, limit int) ([]Purchase, error) {
	if limit <= 0 {
		limit = 20
	}

	purchases := make([]Purchase, 0, limit)
	var orderErr, receiptErr error

	if fulfillments, err := c.api.GetFulfillments(ctx); err == nil {
		for _, f := range fulfillments {
			purchases = append(purchases, Purchase{
				Kind:         PurchaseOrder,
				ID:           strconv.Itoa(f.OrderID),
				Date:         f.Delivery.Slot.Date,
				Total:        f.TotalPrice,
				Status:       firstNonEmpty(f.StatusDescription, f.Status),
				Slot:         strings.TrimSpace(f.Delivery.Slot.DateDisplay + " " + f.Delivery.Slot.TimeDisplay),
				ShoppingType: f.ShoppingType,
				Modifiable:   f.Modifiable,
			})
		}
	} else {
		orderErr = err
	}

	if receipts, err := c.api.GetReceipts(ctx); err == nil {
		for _, r := range receipts {
			purchases = append(purchases, Purchase{
				Kind:  PurchaseReceipt,
				ID:    r.TransactionID,
				Date:  r.Date,
				Total: r.TotalAmount,
			})
		}
	} else {
		receiptErr = err
	}

	if orderErr != nil && receiptErr != nil {
		return nil, fmt.Errorf("no purchase history available: orders: %v; receipts: %v", orderErr, receiptErr)
	}

	sort.SliceStable(purchases, func(i, j int) bool {
		return purchases[i].Date > purchases[j].Date
	})

	if len(purchases) > limit {
		purchases = purchases[:limit]
	}
	return purchases, nil
}

// PurchaseDetail reads the lines of one order or receipt, with prices.
func (c *Client) PurchaseDetail(ctx context.Context, kind PurchaseKind, id string) (*PurchaseDetail, error) {
	switch kind {
	case PurchaseReceipt:
		return c.receiptDetail(ctx, id)
	case PurchaseOrder:
		return c.orderDetail(ctx, id)
	default:
		return nil, fmt.Errorf("unknown purchase kind %q, want %q or %q", kind, PurchaseOrder, PurchaseReceipt)
	}
}

func (c *Client) receiptDetail(ctx context.Context, id string) (*PurchaseDetail, error) {
	receipt, err := c.api.GetReceipt(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get receipt failed: %w", err)
	}

	detail := &PurchaseDetail{
		Purchase: Purchase{
			Kind:  PurchaseReceipt,
			ID:    receipt.TransactionID,
			Date:  receipt.Date,
			Total: receipt.TotalAmount,
		},
		Discounts: receipt.Discounts,
		Payments:  receipt.Payments,
	}

	for _, item := range receipt.Items {
		detail.Lines = append(detail.Lines, PurchaseLine{
			Description: item.Description,
			ProductID:   item.ProductID,
			Quantity:    max(item.Quantity, 1),
			UnitPrice:   item.UnitPrice,
			Amount:      item.Amount,
		})
		detail.Subtotal += item.Amount
	}

	return detail, nil
}

func (c *Client) orderDetail(ctx context.Context, id string) (*PurchaseDetail, error) {
	orderID, err := strconv.Atoi(strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("invalid order id %q: %w", id, err)
	}

	order, err := c.api.GetOrderDetails(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("get order details failed: %w", err)
	}

	detail := &PurchaseDetail{
		Purchase: Purchase{
			Kind:   PurchaseOrder,
			ID:     order.ID,
			Total:  order.TotalPrice,
			Status: order.State,
		},
		Subtotal: order.Subtotal(),
	}
	if !order.LastUpdated.IsZero() {
		detail.Date = order.LastUpdated.Format("2006-01-02")
	}

	for _, item := range order.Items {
		line := PurchaseLine{
			ProductID: item.ProductID,
			Quantity:  max(item.Quantity, 1),
		}
		if item.Product != nil {
			line.Description = item.Product.Title
			line.UnitPrice = item.Product.Price.Now
			line.Amount = item.Product.Price.Now * float64(line.Quantity)
		}
		detail.Lines = append(detail.Lines, line)
	}

	return detail, nil
}

// SpendSummary aggregates what the member spent, per month and in total.
type SpendSummary struct {
	Purchases int                `json:"purchases"`
	Total     float64            `json:"total"`
	ByMonth   map[string]float64 `json:"byMonth"`
	ByKind    map[string]float64 `json:"byKind"`
	// From and To bound the period covered, as YYYY-MM-DD.
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

// Spend summarises spending across the most recent purchases.
func (c *Client) Spend(ctx context.Context, limit int) (*SpendSummary, error) {
	purchases, err := c.RecentPurchases(ctx, limit)
	if err != nil {
		return nil, err
	}

	summary := &SpendSummary{
		ByMonth: make(map[string]float64),
		ByKind:  make(map[string]float64),
	}

	for _, p := range purchases {
		summary.Purchases++
		summary.Total += p.Total
		summary.ByKind[string(p.Kind)] += p.Total
		if len(p.Date) >= 7 {
			summary.ByMonth[p.Date[:7]] += p.Total
		}
		if date := dateOnly(p.Date); date != "" {
			if summary.From == "" || date < summary.From {
				summary.From = date
			}
			if summary.To == "" || date > summary.To {
				summary.To = date
			}
		}
	}

	return summary, nil
}

// OrderedItems counts how often each product appears across recent purchases,
// so a caller can see what is actually bought regularly rather than once.
type OrderedItem struct {
	Description string  `json:"description"`
	ProductID   int     `json:"productId,omitempty"`
	Times       int     `json:"times"`
	Quantity    int     `json:"quantity"`
	Spent       float64 `json:"spent"`
	LastSeen    string  `json:"lastSeen,omitempty"`
}

// OrderedItems reads the lines of the most recent purchases and aggregates them
// per product. Purchases whose detail cannot be fetched are skipped rather than
// failing the whole summary.
func (c *Client) OrderedItems(ctx context.Context, purchaseLimit int) ([]OrderedItem, error) {
	purchases, err := c.RecentPurchases(ctx, purchaseLimit)
	if err != nil {
		return nil, err
	}

	type agg struct {
		item OrderedItem
	}
	byKey := make(map[string]*agg)
	order := make([]string, 0)

	for _, p := range purchases {
		detail, err := c.PurchaseDetail(ctx, p.Kind, p.ID)
		if err != nil {
			continue
		}
		for _, line := range detail.Lines {
			key := strconv.Itoa(line.ProductID)
			if line.ProductID == 0 {
				key = strings.ToLower(strings.TrimSpace(line.Description))
			}
			if key == "" || key == "0" {
				continue
			}

			entry, ok := byKey[key]
			if !ok {
				entry = &agg{item: OrderedItem{
					Description: line.Description,
					ProductID:   line.ProductID,
				}}
				byKey[key] = entry
				order = append(order, key)
			}
			entry.item.Times++
			entry.item.Quantity += line.Quantity
			entry.item.Spent += line.Amount
			if p.Date > entry.item.LastSeen {
				entry.item.LastSeen = dateOnly(p.Date)
			}
		}
	}

	out := make([]OrderedItem, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key].item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Times != out[j].Times {
			return out[i].Times > out[j].Times
		}
		return out[i].Spent > out[j].Spent
	})
	return out, nil
}

func dateOnly(s string) string {
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
