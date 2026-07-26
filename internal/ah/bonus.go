package ah

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	appie "github.com/gwillem/appie-go"
)

// appie-go's GetBonusProducts walks the bonus categories and returns the first
// error it meets, so one category answering 424 loses the entire folder. AH
// does that regularly. The version here keeps going and reports what it missed,
// because a folder minus one category is still useful and an error is not.

// SkippedCategory records a bonus category that could not be read.
type SkippedCategory struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

// BonusFolder is this week's promotions.
type BonusFolder struct {
	StartDate string          `json:"startDate,omitempty"`
	EndDate   string          `json:"endDate,omitempty"`
	Count     int             `json:"count"`
	Products  []appie.Product `json:"products"`
	// Skipped lists categories AH refused to serve. Empty on a healthy run.
	Skipped []SkippedCategory `json:"skipped,omitempty"`
}

// bonusMetadata is the response describing the current bonus period.
type bonusMetadata struct {
	Periods []struct {
		BonusStartDate string `json:"bonusStartDate"`
		BonusEndDate   string `json:"bonusEndDate"`
		Tabs           []struct {
			URLMetadataList []struct {
				BonusType   string `json:"bonusType"`
				Description string `json:"description"`
			} `json:"urlMetadataList"`
		} `json:"tabs"`
	} `json:"periods"`
}

// bonusSection is one category of the folder.
type bonusSection struct {
	BonusGroupOrProducts []struct {
		Product    *bonusProduct `json:"product,omitempty"`
		BonusGroup *struct {
			ID                  string        `json:"id"`
			SegmentDescription  string        `json:"segmentDescription"`
			DiscountDescription string        `json:"discountDescription"`
			Category            string        `json:"category"`
			Images              []appie.Image `json:"images"`
			ExampleFromPrice    float64       `json:"exampleFromPrice"`
			ExampleForPrice     float64       `json:"exampleForPrice"`
		} `json:"bonusGroup,omitempty"`
	} `json:"bonusGroupOrProducts"`
}

// bonusProduct mirrors the product shape the bonus endpoint returns.
type bonusProduct struct {
	WebshopID            int           `json:"webshopId"`
	Title                string        `json:"title"`
	Brand                string        `json:"brand"`
	SalesUnitSize        string        `json:"salesUnitSize"`
	UnitPriceDescription string        `json:"unitPriceDescription"`
	Images               []appie.Image `json:"images"`
	CurrentPrice         float64       `json:"currentPrice"`
	PriceBeforeBonus     float64       `json:"priceBeforeBonus"`
	IsBonus              bool          `json:"isBonus"`
	BonusMechanism       string        `json:"bonusMechanism"`
	MainCategory         string        `json:"mainCategory"`
	SubCategory          string        `json:"subCategory"`
	NutriScore           string        `json:"nutriscore"`
	AvailableOnline      bool          `json:"availableOnline"`
	IsOrderable          bool          `json:"isOrderable"`
	IsPreviouslyBought   bool          `json:"isPreviouslyBought"`
	PropertyIcons        []string      `json:"propertyIcons"`
}

func (p *bonusProduct) toProduct() appie.Product {
	price := p.CurrentPrice
	if price == 0 {
		price = p.PriceBeforeBonus
	}
	return appie.Product{
		ID:                   p.WebshopID,
		Title:                p.Title,
		Brand:                p.Brand,
		Category:             p.MainCategory,
		SubCategory:          p.SubCategory,
		Price:                appie.Price{Now: price, Was: p.PriceBeforeBonus},
		Images:               p.Images,
		NutriScore:           p.NutriScore,
		IsBonus:              p.IsBonus,
		BonusMechanism:       p.BonusMechanism,
		IsAvailable:          p.AvailableOnline,
		IsOrderable:          p.IsOrderable,
		IsPreviouslyBought:   p.IsPreviouslyBought,
		UnitSize:             p.SalesUnitSize,
		UnitPriceDescription: p.UnitPriceDescription,
		PropertyIcons:        p.PropertyIcons,
	}
}

// bonusCategoryWorkers bounds how many categories are fetched at once.
const bonusCategoryWorkers = 4

// GetBonusFolder reads the whole bonus folder. Categories that fail are
// reported in Skipped rather than failing the call.
func (c *Client) GetBonusFolder(ctx context.Context) (*BonusFolder, error) {
	var meta bonusMetadata
	if err := c.api.DoRequest(ctx, http.MethodGet, "/mobile-services/bonuspage/v3/metadata", nil, &meta); err != nil {
		return nil, fmt.Errorf("get bonus metadata failed: %w", err)
	}

	folder := &BonusFolder{Products: make([]appie.Product, 0)}
	categories := make([]string, 0)
	seen := make(map[string]bool)

	for _, period := range meta.Periods {
		if folder.StartDate == "" {
			folder.StartDate = period.BonusStartDate
			folder.EndDate = period.BonusEndDate
		}
		for _, tab := range period.Tabs {
			for _, entry := range tab.URLMetadataList {
				if entry.BonusType != "NATIONAL" || seen[entry.Description] {
					continue
				}
				seen[entry.Description] = true
				categories = append(categories, entry.Description)
			}
		}
	}
	if len(categories) == 0 {
		return folder, nil
	}

	results := c.fetchCategories(ctx, categories)

	// Group entries such as "Alle Alpro" have no product id, so titles are the
	// only usable key for deduplication across categories.
	added := make(map[string]bool)
	for i, category := range categories {
		result := results[i]
		if result.err != nil {
			folder.Skipped = append(folder.Skipped, SkippedCategory{
				Category: category,
				Reason:   result.err.Error(),
			})
			continue
		}
		for _, product := range result.products {
			key := product.Title
			if added[key] {
				continue
			}
			added[key] = true
			folder.Products = append(folder.Products, product)
		}
	}

	folder.Count = len(folder.Products)
	return folder, nil
}

type categoryResult struct {
	products []appie.Product
	err      error
}

// fetchCategories reads every category concurrently, keeping input order.
func (c *Client) fetchCategories(ctx context.Context, categories []string) []categoryResult {
	results := make([]categoryResult, len(categories))
	work := make(chan int)

	var wg sync.WaitGroup
	for range min(bonusCategoryWorkers, len(categories)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				products, err := c.bonusSection(ctx, categories[i])
				results[i] = categoryResult{products: products, err: err}
			}
		}()
	}

	for i := range categories {
		select {
		case work <- i:
		case <-ctx.Done():
			close(work)
			wg.Wait()
			return results
		}
	}
	close(work)
	wg.Wait()

	return results
}

// bonusSection reads one category of the folder.
func (c *Client) bonusSection(ctx context.Context, category string) ([]appie.Product, error) {
	params := url.Values{}
	params.Set("application", "AHWEBSHOP")
	params.Set("date", time.Now().Format("2006-01-02"))
	params.Set("promotionType", "NATIONAL")
	params.Set("category", category)

	var section bonusSection
	path := "/mobile-services/bonuspage/v2/section?" + params.Encode()
	if err := c.api.DoRequest(ctx, http.MethodGet, path, nil, &section); err != nil {
		return nil, err
	}

	products := make([]appie.Product, 0, len(section.BonusGroupOrProducts))
	for _, entry := range section.BonusGroupOrProducts {
		switch {
		case entry.Product != nil:
			products = append(products, entry.Product.toProduct())
		case entry.BonusGroup != nil:
			group := entry.BonusGroup
			products = append(products, appie.Product{
				Title:          group.SegmentDescription,
				Category:       group.Category,
				BonusMechanism: group.DiscountDescription,
				IsBonus:        true,
				BonusSegmentID: group.ID,
				Images:         group.Images,
				Price:          appie.Price{Now: group.ExampleForPrice, Was: group.ExampleFromPrice},
			})
		}
	}
	return products, nil
}
