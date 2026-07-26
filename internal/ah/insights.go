package ah

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

// dateLayouts are the formats the AH API has been seen to use for purchase
// dates. They are tried in order.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"02-01-2006",
}

// CategorySpend is what was spent in one product category.
type CategorySpend struct {
	Category string  `json:"category"`
	Spend    float64 `json:"spend"`
	// Share is the fraction of categorised spend, 0..1.
	Share float64 `json:"share"`
	Items int     `json:"items"`
}

// EatingPatterns summarises what and how the member actually eats, derived from
// receipts and orders rather than from what they say they buy.
type EatingPatterns struct {
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Trips int    `json:"trips"`

	// TotalSpend is the sum of the purchase totals, so after discounts.
	TotalSpend float64 `json:"totalSpend"`
	// LineSpend is the sum of the individual lines. It differs from TotalSpend
	// because receipt totals include discounts and deposits that are not lines,
	// so the two are not interchangeable.
	LineSpend float64 `json:"lineSpend"`
	AvgBasket float64 `json:"avgBasket"`
	// AvgDaysBetweenTrips is the mean gap between shopping trips.
	AvgDaysBetweenTrips float64 `json:"avgDaysBetweenTrips,omitempty"`
	// TripsByWeekday counts trips per weekday, in English weekday names.
	TripsByWeekday map[string]int `json:"tripsByWeekday,omitempty"`
	// FavouriteShoppingDay is the weekday with the most trips.
	FavouriteShoppingDay string `json:"favouriteShoppingDay,omitempty"`

	Categories []CategorySpend `json:"categories,omitempty"`
	// UncategorisedSpend is the part of LineSpend on lines that could not be
	// resolved to a product, so it has no category. In-store receipts often
	// carry no product ids at all, in which case this is everything. Reported
	// rather than guessed at from the description.
	UncategorisedSpend float64 `json:"uncategorisedSpend,omitempty"`
	// UncategorisedShare is UncategorisedSpend as a fraction of LineSpend.
	UncategorisedShare float64 `json:"uncategorisedShare,omitempty"`

	// Staples are products bought on at least half of all trips.
	Staples []OrderedItem `json:"staples,omitempty"`
	// Frequent are products bought more than once but below the staple line.
	Frequent []OrderedItem `json:"frequent,omitempty"`
	// OneOffs counts products bought exactly once.
	OneOffs int `json:"oneOffs"`

	// LinesAnalysed is how many purchase lines fed this summary.
	LinesAnalysed int `json:"linesAnalysed"`
	// PurchasesSkipped counts purchases whose detail could not be read.
	PurchasesSkipped int `json:"purchasesSkipped"`
}

// Patterns reads recent receipts and orders line by line and derives eating
// patterns: shopping cadence, category mix, staples versus one-offs.
func (c *Client) Patterns(ctx context.Context, purchaseLimit int) (*EatingPatterns, error) {
	if purchaseLimit <= 0 {
		purchaseLimit = 20
	}

	purchases, err := c.RecentPurchases(ctx, purchaseLimit)
	if err != nil {
		return nil, err
	}

	patterns := &EatingPatterns{TripsByWeekday: make(map[string]int)}
	items := make(map[string]*OrderedItem)
	itemOrder := make([]string, 0)
	categorySpend := make(map[string]*CategorySpend)
	productIDs := make(map[int]bool)
	dates := make([]time.Time, 0, len(purchases))

	for _, purchase := range purchases {
		detail, err := c.PurchaseDetail(ctx, purchase.Kind, purchase.ID)
		if err != nil {
			patterns.PurchasesSkipped++
			continue
		}

		patterns.Trips++
		patterns.TotalSpend += purchase.Total
		trackDate(patterns, &dates, purchase.Date)

		for _, line := range detail.Lines {
			patterns.LinesAnalysed++
			patterns.LineSpend += line.Amount
			key := itemKey(line)
			if key == "" {
				continue
			}
			entry, ok := items[key]
			if !ok {
				entry = &OrderedItem{Description: line.Description, ProductID: line.ProductID}
				items[key] = entry
				itemOrder = append(itemOrder, key)
			}
			entry.Times++
			entry.Quantity += line.Quantity
			entry.Spent += line.Amount
			if date := dateOnly(purchase.Date); date > entry.LastSeen {
				entry.LastSeen = date
			}
			if line.ProductID > 0 {
				productIDs[line.ProductID] = true
			}
		}
	}

	if patterns.Trips == 0 {
		return patterns, nil
	}

	patterns.AvgBasket = patterns.TotalSpend / float64(patterns.Trips)
	patterns.AvgDaysBetweenTrips = averageGapDays(dates)
	patterns.FavouriteShoppingDay = topWeekday(patterns.TripsByWeekday)

	c.categorise(ctx, productIDs, items, categorySpend, patterns)
	patterns.Categories = rankCategories(categorySpend)
	if patterns.LineSpend > 0 {
		patterns.UncategorisedShare = patterns.UncategorisedSpend / patterns.LineSpend
	}
	patterns.Staples, patterns.Frequent, patterns.OneOffs = splitByFrequency(items, itemOrder, patterns.Trips)

	return patterns, nil
}

// categorise resolves product categories in one REST lookup and attributes
// spend to them. Lines without a resolvable product land in UncategorisedSpend.
func (c *Client) categorise(
	ctx context.Context,
	productIDs map[int]bool,
	items map[string]*OrderedItem,
	categories map[string]*CategorySpend,
	patterns *EatingPatterns,
) {
	lookup := make(map[int]string)
	if len(productIDs) > 0 {
		ids := make([]int, 0, len(productIDs))
		for id := range productIDs {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		if products, err := c.api.GetProductsByIDs(ctx, ids); err == nil {
			for _, p := range products {
				if p.Category != "" {
					lookup[p.ID] = p.Category
				}
			}
		}
	}

	for _, item := range items {
		category, ok := lookup[item.ProductID]
		if !ok || category == "" {
			patterns.UncategorisedSpend += item.Spent
			continue
		}
		entry, exists := categories[category]
		if !exists {
			entry = &CategorySpend{Category: category}
			categories[category] = entry
		}
		entry.Spend += item.Spent
		entry.Items += item.Quantity
	}
}

func rankCategories(categories map[string]*CategorySpend) []CategorySpend {
	var total float64
	for _, entry := range categories {
		total += entry.Spend
	}

	out := make([]CategorySpend, 0, len(categories))
	for _, entry := range categories {
		if total > 0 {
			entry.Share = entry.Spend / total
		}
		out = append(out, *entry)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Spend > out[j].Spend })
	return out
}

// splitByFrequency separates staples from incidental purchases. An item bought
// on at least half of the trips is treated as a staple.
func splitByFrequency(items map[string]*OrderedItem, order []string, trips int) (staples, frequent []OrderedItem, oneOffs int) {
	stapleThreshold := max(trips/2, 2)

	for _, key := range order {
		item := *items[key]
		switch {
		case item.Times >= stapleThreshold:
			staples = append(staples, item)
		case item.Times > 1:
			frequent = append(frequent, item)
		default:
			oneOffs++
		}
	}

	byTimes := func(list []OrderedItem) {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Times != list[j].Times {
				return list[i].Times > list[j].Times
			}
			return list[i].Spent > list[j].Spent
		})
	}
	byTimes(staples)
	byTimes(frequent)
	return staples, frequent, oneOffs
}

func trackDate(patterns *EatingPatterns, dates *[]time.Time, raw string) {
	date := dateOnly(raw)
	if date == "" {
		return
	}
	if patterns.From == "" || date < patterns.From {
		patterns.From = date
	}
	if date > patterns.To {
		patterns.To = date
	}
	if parsed, ok := parseDate(raw); ok {
		*dates = append(*dates, parsed)
		patterns.TripsByWeekday[parsed.Weekday().String()]++
	}
}

func parseDate(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func averageGapDays(dates []time.Time) float64 {
	if len(dates) < 2 {
		return 0
	}
	sorted := make([]time.Time, len(dates))
	copy(sorted, dates)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Before(sorted[j]) })

	span := sorted[len(sorted)-1].Sub(sorted[0]).Hours() / 24
	return span / float64(len(sorted)-1)
}

func topWeekday(counts map[string]int) string {
	best := ""
	bestCount := 0
	// Iterate weekdays in a fixed order so ties resolve deterministically.
	for day := time.Sunday; day <= time.Saturday; day++ {
		name := day.String()
		if counts[name] > bestCount {
			best, bestCount = name, counts[name]
		}
	}
	return best
}

func itemKey(line PurchaseLine) string {
	if line.ProductID > 0 {
		return "p:" + strconv.Itoa(line.ProductID)
	}
	return "t:" + strings.TrimSpace(strings.ToLower(line.Description))
}
