package ah

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"sort"
	"strings"
)

// Allerhande's GraphQL schema changed after this skill was first written:
// recipeSearch lost its text argument in favour of recipeSearchV2, Recipe lost
// slug, prepTime, tags-as-strings and nutrition, and the ingredient unit moved
// into the printed text. Introspection is disabled on api.ah.nl, so the shapes
// below were established against the live API. Keep them in sync by running
// `appie-cli graphql` when a query starts failing with GRAPHQL_VALIDATION_FAILED.

// Recipe is an Allerhande recipe.
type Recipe struct {
	ID          int                `json:"id"`
	Title       string             `json:"title"`
	URL         string             `json:"url,omitempty"`
	Description string             `json:"description,omitempty"`
	Courses     []string           `json:"courses,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Servings    *Servings          `json:"servings,omitempty"`
	Ingredients []RecipeIngredient `json:"ingredients,omitempty"`
	Steps       []string           `json:"steps,omitempty"`
	Image       string             `json:"image,omitempty"`
}

// Servings is how many people a recipe feeds, and the range it scales to.
type Servings struct {
	Number int `json:"number"`
	Min    int `json:"min,omitempty"`
	Max    int `json:"max,omitempty"`
}

// RecipeIngredient is one line on a recipe's ingredient list. The API no longer
// returns a separate unit, so Unit is derived from the printed text and is
// empty when it could not be identified.
type RecipeIngredient struct {
	ID       int     `json:"id,omitempty"`
	Text     string  `json:"text,omitempty"`
	Name     string  `json:"name,omitempty"`
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
}

// RecipeTime breaks a recipe's duration into its parts, in minutes.
type RecipeTime struct {
	Cook  int `json:"cookMinutes"`
	Oven  int `json:"ovenMinutes,omitempty"`
	Wait  int `json:"waitMinutes,omitempty"`
	Total int `json:"totalMinutes"`
	// Active is cooking plus oven time: the time you actually spend on it.
	Active int `json:"activeMinutes"`
}

// RecipeSummary is a search hit.
type RecipeSummary struct {
	ID      int        `json:"id"`
	Title   string     `json:"title"`
	Slug    string     `json:"slug,omitempty"`
	URL     string     `json:"url,omitempty"`
	Courses []string   `json:"courses,omitempty"`
	Time    RecipeTime `json:"time"`
	Image   string     `json:"image,omitempty"`
}

// RecipeSearchOptions configures a recipe search.
type RecipeSearchOptions struct {
	// Query is the search term. Empty returns popular recipes.
	Query string
	// Limit caps the number of results returned after filtering.
	Limit int
	// Start is the offset into the result set, for paging.
	Start int
	// MaxCookTime drops recipes whose active time exceeds this, in minutes.
	// 0 disables the filter.
	MaxCookTime int
}

// SearchRecipes searches Allerhande. MaxCookTime is applied client-side, so the
// search over-fetches to still return Limit usable results.
func (c *Client) SearchRecipes(ctx context.Context, opts RecipeSearchOptions) ([]RecipeSummary, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 10
	}
	start := max(opts.Start, 0)

	fetch := limit
	if opts.MaxCookTime > 0 {
		fetch = min(limit*4, 200)
	}

	query := fmt.Sprintf(`query RecipeSearch {
  recipeSearchV2(searchText: %s, size: %d, start: %d) {
    result {
      id
      title
      slug
      courses
      time { cook oven wait }
      images { url width }
    }
    page { total hasNextPage }
  }
}`, gqlString(opts.Query), fetch, start)

	var resp struct {
		RecipeSearchV2 struct {
			Result []struct {
				ID      int      `json:"id"`
				Title   string   `json:"title"`
				Slug    string   `json:"slug"`
				Courses []string `json:"courses"`
				Time    struct {
					Cook int `json:"cook"`
					Oven int `json:"oven"`
					Wait int `json:"wait"`
				} `json:"time"`
				Images []recipeImage `json:"images"`
			} `json:"result"`
		} `json:"recipeSearchV2"`
	}
	if err := c.api.DoGraphQL(ctx, query, nil, &resp); err != nil {
		return nil, fmt.Errorf("recipe search failed: %w", err)
	}

	out := make([]RecipeSummary, 0, limit)
	for _, r := range resp.RecipeSearchV2.Result {
		timing := newRecipeTime(r.Time.Cook, r.Time.Oven, r.Time.Wait)
		if opts.MaxCookTime > 0 && timing.Active > opts.MaxCookTime {
			continue
		}
		out = append(out, RecipeSummary{
			ID:      r.ID,
			Title:   html.UnescapeString(r.Title),
			Slug:    r.Slug,
			URL:     recipeURL(r.ID, r.Slug),
			Courses: r.Courses,
			Time:    timing,
			Image:   pickImage(r.Images),
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// GetRecipe reads a full recipe: ingredients and preparation steps.
func (c *Client) GetRecipe(ctx context.Context, id int) (*Recipe, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid recipe id %d", id)
	}

	query := fmt.Sprintf(`query Recipe {
  recipe(id: %d) {
    id
    title
    description
    courses
    servings { number min max }
    ingredients {
      id
      text
      quantity
      name { singular plural }
    }
    preparation { steps }
    images { url width }
    tags { value }
  }
}`, id)

	var resp struct {
		Recipe *struct {
			ID          int      `json:"id"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Courses     []string `json:"courses"`
			Servings    *struct {
				Number int `json:"number"`
				Min    int `json:"min"`
				Max    int `json:"max"`
			} `json:"servings"`
			Ingredients []struct {
				ID       int     `json:"id"`
				Text     string  `json:"text"`
				Quantity float64 `json:"quantity"`
				Name     struct {
					Singular string `json:"singular"`
					Plural   string `json:"plural"`
				} `json:"name"`
			} `json:"ingredients"`
			Preparation struct {
				Steps []string `json:"steps"`
			} `json:"preparation"`
			Images []recipeImage `json:"images"`
			Tags   []struct {
				Value string `json:"value"`
			} `json:"tags"`
		} `json:"recipe"`
	}
	if err := c.api.DoGraphQL(ctx, query, nil, &resp); err != nil {
		return nil, fmt.Errorf("get recipe failed: %w", err)
	}
	if resp.Recipe == nil {
		return nil, fmt.Errorf("recipe %d not found", id)
	}

	r := resp.Recipe
	recipe := &Recipe{
		ID:          r.ID,
		Title:       html.UnescapeString(r.Title),
		URL:         recipeURL(r.ID, ""),
		Description: html.UnescapeString(r.Description),
		Courses:     r.Courses,
		Image:       pickImage(r.Images),
	}

	if r.Servings != nil {
		recipe.Servings = &Servings{Number: r.Servings.Number, Min: r.Servings.Min, Max: r.Servings.Max}
	}

	for _, tag := range r.Tags {
		if tag.Value != "" {
			recipe.Tags = append(recipe.Tags, tag.Value)
		}
	}

	for _, ing := range r.Ingredients {
		name := ing.Name.Singular
		if ing.Quantity > 1 && ing.Name.Plural != "" {
			name = ing.Name.Plural
		}
		text := html.UnescapeString(ing.Text)
		recipe.Ingredients = append(recipe.Ingredients, RecipeIngredient{
			ID:       ing.ID,
			Text:     text,
			Name:     html.UnescapeString(name),
			Quantity: ing.Quantity,
			Unit:     unitFromText(text),
		})
	}

	for _, step := range r.Preparation.Steps {
		recipe.Steps = append(recipe.Steps, html.UnescapeString(step))
	}

	return recipe, nil
}

type recipeImage struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

// pickImage returns the largest available rendition.
func pickImage(images []recipeImage) string {
	best := ""
	widest := -1
	for _, img := range images {
		if img.URL != "" && img.Width > widest {
			best, widest = img.URL, img.Width
		}
	}
	return best
}

func newRecipeTime(cook, oven, wait int) RecipeTime {
	return RecipeTime{
		Cook:   cook,
		Oven:   oven,
		Wait:   wait,
		Active: cook + oven,
		Total:  cook + oven + wait,
	}
}

// recipeUnits are the measures Allerhande prints in its ingredient lines.
var recipeUnits = map[string]string{
	"g": "g", "gram": "g", "kg": "kg", "ml": "ml", "cl": "cl", "dl": "dl",
	"l": "l", "liter": "l", "el": "el", "eetlepel": "el", "eetlepels": "el",
	"tl": "tl", "theelepel": "tl", "theelepels": "tl",
	"snee": "snee", "sneetje": "snee", "sneetjes": "snee",
	"stuk": "stuks", "stuks": "stuks",
	"blik": "blik", "blikje": "blik", "blikjes": "blik", "blikken": "blik",
	"pak": "pak", "pakje": "pak", "pakjes": "pak", "pakken": "pak",
	"zak": "zak", "zakje": "zak", "zakjes": "zak", "zakken": "zak",
	"bos": "bos", "bosje": "bos", "bosjes": "bos",
	"teen": "teen", "tenen": "teen",
	"plak": "plak", "plakje": "plak", "plakjes": "plak", "plakken": "plak",
	"bakje": "bakje", "bakjes": "bakje",
	"pot": "pot", "potje": "pot", "potjes": "pot",
	"fles": "fles", "flesje": "fles", "flesjes": "fles",
	"krop": "krop", "struik": "struik",
	"handvol": "handvol", "mespunt": "mespunt", "scheut": "scheut",
	"snufje": "snufje", "takje": "takje", "takjes": "takje",
}

// unitFromText reads the measure out of a printed ingredient line such as
// "300 g bloem" or "1 el olijfolie". Returns "" when the line names no measure.
func unitFromText(text string) string {
	for _, field := range strings.Fields(strings.ToLower(text)) {
		field = strings.Trim(field, ".,()")
		if field == "" {
			continue
		}
		if unit, ok := recipeUnits[field]; ok {
			return unit
		}
		// Stop at the first word that is neither a number nor a measure: from
		// there on the line is the ingredient name.
		if !isQuantityToken(field) {
			return ""
		}
	}
	return ""
}

// isQuantityToken reports whether a token is part of the amount rather than the
// ingredient name, covering digits, fractions and ranges.
func isQuantityToken(token string) bool {
	for _, r := range token {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' || r == ',' || r == '/' || r == '-' || r == '½' || r == '¼' || r == '¾':
		default:
			return false
		}
	}
	return token != ""
}

// MergedIngredient is one ingredient combined across several recipes.
type MergedIngredient struct {
	Name string `json:"name"`
	// Quantity is the summed amount when every occurrence shares a unit.
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
	// Occurrences lists the original lines, so a caller can see what was merged.
	Occurrences []string `json:"occurrences"`
	// Recipes names the recipes that need this ingredient.
	Recipes []string `json:"recipes"`
	// MixedUnits is true when amounts could not be summed because the recipes
	// expressed them differently, e.g. "2 stuks" and "200 g".
	MixedUnits bool `json:"mixedUnits,omitempty"`
}

// MergeIngredients combines the ingredient lists of several recipes into one
// deduplicated shopping list. Amounts are summed per unit; when units disagree
// the occurrences are kept side by side and MixedUnits is set, so the caller
// decides rather than the code silently picking a wrong quantity.
func MergeIngredients(recipes []Recipe) []MergedIngredient {
	type acc struct {
		merged MergedIngredient
		units  map[string]float64
	}

	order := make([]string, 0)
	byKey := make(map[string]*acc)

	for _, recipe := range recipes {
		for _, ing := range recipe.Ingredients {
			name := ing.Name
			if name == "" {
				name = ing.Text
			}
			key := normalizeIngredient(name)
			if key == "" {
				continue
			}

			entry, ok := byKey[key]
			if !ok {
				entry = &acc{
					merged: MergedIngredient{Name: name},
					units:  make(map[string]float64),
				}
				byKey[key] = entry
				order = append(order, key)
			}

			text := ing.Text
			if text == "" {
				text = name
			}
			entry.merged.Occurrences = append(entry.merged.Occurrences, text)
			if !containsString(entry.merged.Recipes, recipe.Title) {
				entry.merged.Recipes = append(entry.merged.Recipes, recipe.Title)
			}
			entry.units[ing.Unit] += ing.Quantity
		}
	}

	out := make([]MergedIngredient, 0, len(order))
	for _, key := range order {
		entry := byKey[key]
		if len(entry.units) == 1 {
			for unit, qty := range entry.units {
				entry.merged.Quantity = qty
				entry.merged.Unit = unit
			}
		} else {
			entry.merged.MixedUnits = true
		}
		out = append(out, entry.merged)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].Recipes) > len(out[j].Recipes)
	})
	return out
}

// normalizeIngredient reduces an ingredient name to a comparison key so that
// singular and plural spellings merge into one line.
//
// Dutch plurals do more than add a suffix: they open the syllable
// ("tomaat" / "tomaten") and voice the final consonant ("kaas" / "kazen").
// Stripping the suffix alone therefore leaves the two forms apart, so the stem
// is folded as well.
func normalizeIngredient(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Trim(s, ".,;:")
	if s == "" {
		return ""
	}

	switch {
	case strings.HasSuffix(s, "en") && len(s) > 3:
		s = strings.TrimSuffix(s, "en")
	case strings.HasSuffix(s, "s") && takesPluralS(s):
		s = strings.TrimSuffix(s, "s")
	}

	return foldStem(s)
}

// takesPluralS reports whether a trailing "s" is a plural ending rather than
// part of the word. Dutch takes plural -s after an unstressed syllable
// ("wortels", "bakjes"), so a word ending in vowel + s is singular: stripping
// it would turn "kaas" into "kaa".
func takesPluralS(s string) bool {
	if len(s) < 4 {
		return false
	}
	switch s[len(s)-2] {
	case 'l', 'r', 'n', 'm', 'e':
		return true
	default:
		return false
	}
}

// foldStem undoes the spelling changes Dutch plurals make to a stem.
func foldStem(s string) string {
	// "kazen" -> "kaz" -> "kas", "brieven" -> "briev" -> "brief".
	if n := len(s); n > 2 {
		switch s[n-1] {
		case 'z':
			s = s[:n-1] + "s"
		case 'v':
			s = s[:n-1] + "f"
		}
	}

	// "tomaat" -> "tomat", so it matches "tomaten" -> "tomat".
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if i > 0 && c == s[i-1] && isVowel(c) {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isVowel(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func recipeURL(id int, slug string) string {
	if slug == "" {
		return fmt.Sprintf("https://www.ah.nl/allerhande/recept/R-R%d", id)
	}
	return fmt.Sprintf("https://www.ah.nl/allerhande/recept/R-R%d/%s", id, slug)
}

// gqlString renders a Go string as a GraphQL string literal.
func gqlString(s string) string {
	encoded, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(encoded)
}
