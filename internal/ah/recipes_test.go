package ah

import "testing"

func TestUnitFromText(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"300 g bloem voor verse pasta", "g"},
		{"1 el extra vierge olijfolie", "el"},
		{"½ tl zout", "tl"},
		{"400 ml kokosmelk", "ml"},
		{"2 blikjes tomatenblokjes", "blik"},
		{"1 teen knoflook", "teen"},
		{"3 middelgrote scharreleieren", ""},
		{"verse basilicum", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			if got := unitFromText(tc.text); got != tc.want {
				t.Errorf("unitFromText(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

func TestNormalizeIngredient(t *testing.T) {
	same := [][2]string{
		{"gele ui", "gele uien"},
		{"tomaat", "tomaten"},
		{"Wortel", "wortels"},
	}
	for _, pair := range same {
		if a, b := normalizeIngredient(pair[0]), normalizeIngredient(pair[1]); a != b {
			t.Errorf("%q and %q normalise to %q and %q, want the same key", pair[0], pair[1], a, b)
		}
	}

	if normalizeIngredient("ui") == normalizeIngredient("rijst") {
		t.Error("unrelated ingredients must not share a key")
	}
}

func TestMergeIngredients(t *testing.T) {
	recipes := []Recipe{
		{
			Title: "Pasta bolognese",
			Ingredients: []RecipeIngredient{
				{Name: "gele ui", Text: "2 gele uien", Quantity: 2},
				{Name: "tomatenblokjes", Text: "400 g tomatenblokjes", Quantity: 400, Unit: "g"},
			},
		},
		{
			Title: "Chili sin carne",
			Ingredients: []RecipeIngredient{
				{Name: "gele uien", Text: "1 gele ui", Quantity: 1},
				{Name: "tomatenblokjes", Text: "400 g tomatenblokjes", Quantity: 400, Unit: "g"},
				{Name: "kidneybonen", Text: "1 blik kidneybonen", Quantity: 1, Unit: "blik"},
			},
		},
	}

	merged := MergeIngredients(recipes)

	byName := make(map[string]MergedIngredient, len(merged))
	for _, item := range merged {
		byName[normalizeIngredient(item.Name)] = item
	}

	onion, ok := byName[normalizeIngredient("gele ui")]
	if !ok {
		t.Fatal("onions were not merged into a single line")
	}
	if onion.Quantity != 3 {
		t.Errorf("onion quantity = %v, want 3", onion.Quantity)
	}
	if len(onion.Recipes) != 2 {
		t.Errorf("onion appears in %d recipes, want 2", len(onion.Recipes))
	}

	tomatoes := byName[normalizeIngredient("tomatenblokjes")]
	if tomatoes.Quantity != 800 || tomatoes.Unit != "g" {
		t.Errorf("tomatoes = %v %s, want 800 g", tomatoes.Quantity, tomatoes.Unit)
	}

	beans := byName[normalizeIngredient("kidneybonen")]
	if len(beans.Recipes) != 1 {
		t.Errorf("beans appear in %d recipes, want 1", len(beans.Recipes))
	}

	// Shared ingredients rank first, so a caller sees the overlap immediately.
	if len(merged) > 0 && len(merged[0].Recipes) < 2 {
		t.Errorf("expected a shared ingredient first, got %q", merged[0].Name)
	}
}

func TestMergeIngredientsFlagsMixedUnits(t *testing.T) {
	recipes := []Recipe{
		{Title: "A", Ingredients: []RecipeIngredient{{Name: "ui", Text: "2 uien", Quantity: 2}}},
		{Title: "B", Ingredients: []RecipeIngredient{{Name: "ui", Text: "200 g ui", Quantity: 200, Unit: "g"}}},
	}

	merged := MergeIngredients(recipes)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged ingredient, got %d", len(merged))
	}
	if !merged[0].MixedUnits {
		t.Error("mixing 'stuks' and grams must be flagged, not silently summed")
	}
	if merged[0].Quantity != 0 {
		t.Errorf("quantity = %v, want 0 when units disagree", merged[0].Quantity)
	}
	if len(merged[0].Occurrences) != 2 {
		t.Errorf("occurrences = %d, want both lines preserved", len(merged[0].Occurrences))
	}
}

func TestRecipeURL(t *testing.T) {
	if got, want := recipeURL(123, "kip-saltimbocca"), "https://www.ah.nl/allerhande/recept/R-R123/kip-saltimbocca"; got != want {
		t.Errorf("recipeURL() = %q, want %q", got, want)
	}
	if got, want := recipeURL(123, ""), "https://www.ah.nl/allerhande/recept/R-R123"; got != want {
		t.Errorf("recipeURL() without slug = %q, want %q", got, want)
	}
}

func TestGQLString(t *testing.T) {
	if got, want := gqlString(`say "hi"`), `"say \"hi\""`; got != want {
		t.Errorf("gqlString() = %s, want %s", got, want)
	}
}

func TestNormalizeIngredientDutchPlurals(t *testing.T) {
	pairs := [][2]string{
		{"tomaat", "tomaten"},
		{"kaas", "kazen"},
		{"peer", "peren"},
		{"ui", "uien"},
		{"wortel", "wortels"},
		{"aardappel", "aardappelen"},
	}
	for _, pair := range pairs {
		if a, b := normalizeIngredient(pair[0]), normalizeIngredient(pair[1]); a != b {
			t.Errorf("%q -> %q but %q -> %q; want one key", pair[0], a, pair[1], b)
		}
	}
}

func TestRecipeTimeSeparatesWorkFromWaiting(t *testing.T) {
	// A dish needing 20 minutes of work and 15 in the oven is a 20-minute job;
	// counting the oven towards hands-on time hid it from a 30-minute search.
	timing := newRecipeTime(20, 15, 0)

	if timing.HandsOn != 20 {
		t.Errorf("handsOn = %d, want 20: oven time is unattended", timing.HandsOn)
	}
	if timing.Total != 35 {
		t.Errorf("total = %d, want 35", timing.Total)
	}

	resting := newRecipeTime(20, 0, 30)
	if resting.HandsOn != 20 {
		t.Errorf("handsOn = %d, want 20: waiting is unattended", resting.HandsOn)
	}
	if resting.Total != 50 {
		t.Errorf("total = %d, want 50", resting.Total)
	}
}
