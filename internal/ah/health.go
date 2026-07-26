package ah

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appie "github.com/gwillem/appie-go"
)

// foodGroups maps AH's own category names onto the groups a person recognises.
// Matching is on substrings because AH writes categories as lists
// ("Aardappelen, groente, fruit"), and the first match wins, so the order here
// is deliberate: alcohol before drinks, sweets before bread.
var foodGroups = []struct {
	Group    string
	Keywords []string
}{
	{"alcohol", []string{"bier", "wijn", "aperitie", "sterke drank", "gedistilleerd"}},
	{"groente en fruit", []string{"groente", "fruit", "aardappel", "sla", "salade"}},
	{"snoep en snacks", []string{"snoep", "koek", "chips", "chocolade", "snack", "gebak", "ijs", "zoet"}},
	{"frisdrank en sappen", []string{"frisdrank", "sap", "limonade", "siroop", "water"}},
	{"vlees en vis", []string{"vlees", "vis", "kip", "gehakt", "vleeswaren"}},
	{"zuivel en eieren", []string{"zuivel", "kaas", "eieren", "melk"}},
	{"brood en granen", []string{"brood", "bakkerij", "pasta", "rijst", "granen", "ontbijt", "beleg"}},
	{"kant-en-klaar", []string{"maaltijd", "gemak", "diepvries", "soep", "conserven", "pizza"}},
	{"koffie en thee", []string{"koffie", "thee"}},
	{"non-food", []string{"drogisterij", "huishouden", "baby", "huisdier", "wassen", "verzorging", "papier"}},
}

// classifyGroup returns the food group for an AH category, or "overig".
func classifyGroup(category string) string {
	lower := strings.ToLower(category)
	for _, group := range foodGroups {
		for _, keyword := range group.Keywords {
			if strings.Contains(lower, keyword) {
				return group.Group
			}
		}
	}
	return "overig"
}

// GroupShare is what one food group accounts for.
type GroupShare struct {
	Group string `json:"group"`
	// Products is how many different products fall in this group.
	Products int `json:"products"`
	// Share is Products as a fraction of resolved food products, 0..1.
	Share float64 `json:"share"`
	// Spend is what was spent here in the purchases that could be priced. It is
	// 0 when no receipt line resolved to a product in this group.
	Spend float64 `json:"spend,omitempty"`
}

// NutriScoreBand is how many products carry one Nutri-Score.
type NutriScoreBand struct {
	Grade    string  `json:"grade"`
	Products int     `json:"products"`
	Share    float64 `json:"share"`
}

// HealthItem is one product with the score it carries.
type HealthItem struct {
	ProductID  int    `json:"productId"`
	Title      string `json:"title"`
	NutriScore string `json:"nutriScore,omitempty"`
	Group      string `json:"group,omitempty"`
	// Times is how often it appears in recent receipts, 0 when unknown.
	Times int     `json:"times,omitempty"`
	Spent float64 `json:"spent,omitempty"`
}

// HealthReflection is what a purchase history says about how someone eats.
//
// It is built from the products the member has bought before, which carry real
// product ids and therefore resolve to categories and Nutri-Scores. Receipts
// are layered on top for frequency and spend, but only where they carry ids:
// in-store receipts usually do not, so those figures cover less and say so.
//
// Shares count distinct products, not volume. Buying one bag of crisps and one
// bag of carrots reads as an even split regardless of how often each was
// repurchased; the frequency figures are what distinguish habit from incident.
type HealthReflection struct {
	// HistoryProducts is how many different products the member has bought.
	HistoryProducts int `json:"historyProducts"`
	// Resolved is how many of those could be looked up in the catalogue.
	Resolved int `json:"resolved"`
	// Coverage is Resolved over HistoryProducts, 0..1.
	Coverage float64 `json:"coverage"`

	FoodProducts    int `json:"foodProducts"`
	NonFoodProducts int `json:"nonFoodProducts,omitempty"`

	Groups []GroupShare `json:"groups,omitempty"`

	NutriScore []NutriScoreBand `json:"nutriScore,omitempty"`
	// NutriScoreCoverage is the share of food products carrying a score. Fresh
	// produce, meat and many store items carry none.
	NutriScoreCoverage float64 `json:"nutriScoreCoverage,omitempty"`
	// HealthyShare is the share of scored products at A or B.
	HealthyShare float64 `json:"healthyShare,omitempty"`
	// UnhealthyShare is the same for D and E.
	UnhealthyShare float64 `json:"unhealthyShare,omitempty"`

	// Repeaters are D and E products bought repeatedly: habits rather than
	// one-offs, and therefore the ones worth looking at first.
	Repeaters []HealthItem `json:"repeaters,omitempty"`
	// Wins are the A-scored products bought repeatedly.
	Wins []HealthItem `json:"wins,omitempty"`

	// ReceiptsAnalysed is how many receipts and orders were read for frequency.
	ReceiptsAnalysed int `json:"receiptsAnalysed"`
	// FrequencyKnown is how many products the receipts could put a count on.
	FrequencyKnown int `json:"frequencyKnown"`

	// Observations state what the numbers show, including where they show too
	// little. They describe a basket and are not medical advice.
	Observations []string `json:"observations,omitempty"`
}

// Health reflects the purchase history back: what the basket is made of, how it
// scores, and which repeat purchases drive that.
func (c *Client) Health(ctx context.Context, historySize, receiptLimit int) (*HealthReflection, error) {
	if historySize <= 0 || historySize > 200 {
		historySize = 200
	}
	if receiptLimit < 0 {
		receiptLimit = 0
	}

	history, err := c.PreviouslyBought(ctx, historySize, 0, true)
	if err != nil {
		return nil, err
	}

	reflection := &HealthReflection{HistoryProducts: len(history.Products)}
	frequency := c.frequencyFromReceipts(ctx, receiptLimit, reflection)
	reflection.summarise(history.Products, frequency)
	reflection.observe()
	return reflection, nil
}

// frequencyFromReceipts counts how often each product appears in recent
// purchases. Receipts without product ids simply contribute nothing.
func (c *Client) frequencyFromReceipts(ctx context.Context, limit int, reflection *HealthReflection) map[int]OrderedItem {
	if limit == 0 {
		return nil
	}

	items, err := c.OrderedItems(ctx, limit)
	if err != nil {
		return nil
	}

	frequency := make(map[int]OrderedItem, len(items))
	for _, item := range items {
		if item.ProductID > 0 {
			frequency[item.ProductID] = item
		}
	}
	reflection.ReceiptsAnalysed = limit
	reflection.FrequencyKnown = len(frequency)
	return frequency
}

// summarise fills in the group and Nutri-Score breakdowns.
func (reflection *HealthReflection) summarise(products []appie.Product, frequency map[int]OrderedItem) {
	groups := make(map[string]*GroupShare)
	scores := make(map[string]*NutriScoreBand)
	scored := 0

	for _, product := range products {
		// A product the catalogue did not fill in has no category to classify.
		if product.Category == "" {
			continue
		}
		reflection.Resolved++

		group := classifyGroup(product.Category)
		if group == "non-food" {
			reflection.NonFoodProducts++
			continue
		}
		reflection.FoodProducts++

		entry, exists := groups[group]
		if !exists {
			entry = &GroupShare{Group: group}
			groups[group] = entry
		}
		entry.Products++
		if seen, ok := frequency[product.ID]; ok {
			entry.Spend += seen.Spent
		}

		grade := strings.ToUpper(strings.TrimSpace(product.NutriScore))
		if grade == "" {
			continue
		}
		band, exists := scores[grade]
		if !exists {
			band = &NutriScoreBand{Grade: grade}
			scores[grade] = band
		}
		band.Products++
		scored++

		item := HealthItem{
			ProductID:  product.ID,
			Title:      product.Title,
			NutriScore: grade,
			Group:      group,
		}
		if seen, ok := frequency[product.ID]; ok {
			item.Times = seen.Times
			item.Spent = seen.Spent
		}
		switch grade {
		case "D", "E":
			reflection.Repeaters = append(reflection.Repeaters, item)
		case "A":
			reflection.Wins = append(reflection.Wins, item)
		}
	}

	if reflection.HistoryProducts > 0 {
		reflection.Coverage = float64(reflection.Resolved) / float64(reflection.HistoryProducts)
	}

	for _, entry := range groups {
		if reflection.FoodProducts > 0 {
			entry.Share = float64(entry.Products) / float64(reflection.FoodProducts)
		}
		reflection.Groups = append(reflection.Groups, *entry)
	}
	sort.SliceStable(reflection.Groups, func(i, j int) bool {
		return reflection.Groups[i].Products > reflection.Groups[j].Products
	})

	for _, band := range scores {
		if scored > 0 {
			band.Share = float64(band.Products) / float64(scored)
		}
		reflection.NutriScore = append(reflection.NutriScore, *band)
		switch band.Grade {
		case "A", "B":
			reflection.HealthyShare += band.Share
		case "D", "E":
			reflection.UnhealthyShare += band.Share
		}
	}
	sort.SliceStable(reflection.NutriScore, func(i, j int) bool {
		return reflection.NutriScore[i].Grade < reflection.NutriScore[j].Grade
	})

	if reflection.FoodProducts > 0 {
		reflection.NutriScoreCoverage = float64(scored) / float64(reflection.FoodProducts)
	}

	// Repeatedly bought items first: those are habits, the rest are incidents.
	byFrequency := func(list []HealthItem) {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Times != list[j].Times {
				return list[i].Times > list[j].Times
			}
			return list[i].Title < list[j].Title
		})
	}
	byFrequency(reflection.Repeaters)
	byFrequency(reflection.Wins)
}

// observe states what the numbers show, including when they show too little.
func (reflection *HealthReflection) observe() {
	if reflection.Resolved == 0 {
		reflection.Observations = append(reflection.Observations,
			"No products from the purchase history could be resolved, so there is nothing to reflect on yet.")
		return
	}

	if reflection.Coverage < 0.8 {
		reflection.Observations = append(reflection.Observations, fmt.Sprintf(
			"%.0f%% of the purchase history could be identified; the rest is not in this picture.",
			reflection.Coverage*100))
	}

	shareOf := func(group string) float64 {
		for _, entry := range reflection.Groups {
			if entry.Group == group {
				return entry.Share
			}
		}
		return 0
	}

	for _, group := range []string{"groente en fruit", "snoep en snacks", "alcohol", "frisdrank en sappen"} {
		if share := shareOf(group); share > 0 {
			reflection.Observations = append(reflection.Observations, fmt.Sprintf(
				"%s: %.0f%% of the different food products bought.", group, share*100))
		}
	}

	if reflection.NutriScoreCoverage > 0 {
		reflection.Observations = append(reflection.Observations, fmt.Sprintf(
			"Nutri-Score is known for %.0f%% of food products; fresh produce and meat usually carry none.",
			reflection.NutriScoreCoverage*100))
		reflection.Observations = append(reflection.Observations, fmt.Sprintf(
			"Of the products that do carry one, %.0f%% score A or B and %.0f%% score D or E.",
			reflection.HealthyShare*100, reflection.UnhealthyShare*100))
	}

	if reflection.FrequencyKnown == 0 && reflection.ReceiptsAnalysed > 0 {
		reflection.Observations = append(reflection.Observations,
			"None of the recent receipts carried product ids, so nothing here distinguishes a habit from a one-off purchase.")
	}
}

// productWithScore fetches a product from the endpoint that actually returns a
// Nutri-Score. The detail endpoint omits it while the search endpoint includes
// it for the very same product, so a lookup by id is the reliable route and the
// detail call is only a fallback.
func (c *Client) productWithScore(ctx context.Context, productID int) (*appie.Product, error) {
	products, err := c.api.GetProductsByIDs(ctx, []int{productID})
	if err == nil {
		for _, product := range products {
			if product.ID == productID {
				return &product, nil
			}
		}
	}
	return c.api.GetProduct(ctx, productID)
}

// Alternative is a comparable product with a better Nutri-Score.
type Alternative struct {
	appie.Product
	// PriceDifference is this product's price minus the original's, in EUR.
	PriceDifference float64 `json:"priceDifference"`
}

// Swap suggests better-scoring products in the same subcategory. It only
// compares products that carry a Nutri-Score, so an unscored product yields
// nothing rather than a guess.
func (c *Client) Swap(ctx context.Context, productID, limit int) (map[string]any, error) {
	if limit <= 0 {
		limit = 5
	}

	original, err := c.productWithScore(ctx, productID)
	if err != nil {
		return nil, err
	}

	grade := strings.ToUpper(strings.TrimSpace(original.NutriScore))
	switch grade {
	case "":
		return map[string]any{
			"product":      original,
			"alternatives": []Alternative{},
			"note":         "This product has no Nutri-Score, so there is nothing to compare it against.",
		}, nil
	case "A":
		return map[string]any{
			"product":      original,
			"alternatives": []Alternative{},
			"note":         "This product already scores an A.",
		}, nil
	}

	// Search the subcategory rather than the title: "Campina halfvolle melk"
	// only finds itself, while "Houdbare melk" finds what stands next to it.
	term := firstNonEmpty(original.SubCategory, original.Category, original.Title)

	candidates, err := c.api.SearchProductsFiltered(ctx, appie.SearchOptions{Query: term, Limit: 60})
	if err != nil {
		return nil, err
	}

	alternatives := make([]Alternative, 0, limit)
	for _, candidate := range candidates {
		if candidate.ID == original.ID {
			continue
		}
		candidateGrade := strings.ToUpper(strings.TrimSpace(candidate.NutriScore))
		if candidateGrade == "" || candidateGrade >= grade {
			continue
		}
		alternatives = append(alternatives, Alternative{
			Product:         candidate,
			PriceDifference: candidate.Price.Now - original.Price.Now,
		})
	}

	// Best score first, then the cheapest of equals.
	sort.SliceStable(alternatives, func(i, j int) bool {
		if alternatives[i].NutriScore != alternatives[j].NutriScore {
			return alternatives[i].NutriScore < alternatives[j].NutriScore
		}
		return alternatives[i].PriceDifference < alternatives[j].PriceDifference
	})
	if len(alternatives) > limit {
		alternatives = alternatives[:limit]
	}

	result := map[string]any{
		"product":      original,
		"searchedIn":   term,
		"alternatives": alternatives,
	}
	if len(alternatives) == 0 {
		result["note"] = fmt.Sprintf(
			"Nothing in %q scores better than %s. Within a category that is a normal outcome: a better option usually means a different kind of product.",
			term, grade)
	}
	return result, nil
}
