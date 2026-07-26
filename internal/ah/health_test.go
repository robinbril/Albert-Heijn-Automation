package ah

import (
	"testing"

	appie "github.com/gwillem/appie-go"
)

func TestClassifyGroup(t *testing.T) {
	tests := []struct {
		category string
		want     string
	}{
		// AH writes categories as lists, so matching is on substrings.
		{"Aardappelen, groente, fruit", "groente en fruit"},
		{"Zuivel, eieren", "zuivel en eieren"},
		{"Snoep, koek, chips", "snoep en snacks"},
		{"Bier, wijn, aperitieven", "alcohol"},
		{"Frisdrank, sappen, koffie, thee", "frisdrank en sappen"},
		{"Vlees, kip, vis, vega", "vlees en vis"},
		{"Drogisterij", "non-food"},
		{"Iets heel anders", "overig"},
		{"", "overig"},
	}

	for _, tc := range tests {
		t.Run(tc.category, func(t *testing.T) {
			if got := classifyGroup(tc.category); got != tc.want {
				t.Errorf("classifyGroup(%q) = %q, want %q", tc.category, got, tc.want)
			}
		})
	}
}

func TestClassifyGroupOrderMatters(t *testing.T) {
	// "Bier, wijn" also contains no drink keyword, but a category naming both
	// alcohol and soft drinks must land on alcohol, which is why alcohol is
	// checked first.
	if got := classifyGroup("Bier, wijn, frisdrank"); got != "alcohol" {
		t.Errorf("mixed drinks category = %q, want alcohol", got)
	}
}

func TestSummariseSharesAndScores(t *testing.T) {
	products := []appie.Product{
		{ID: 1, Title: "Appels", Category: "Aardappelen, groente, fruit", NutriScore: "A"},
		{ID: 2, Title: "Wortels", Category: "Aardappelen, groente, fruit", NutriScore: "A"},
		{ID: 3, Title: "Chips", Category: "Snoep, koek, chips", NutriScore: "E"},
		{ID: 4, Title: "Shampoo", Category: "Drogisterij"},
		{ID: 5, Title: "Onbekend", Category: ""},
	}
	frequency := map[int]OrderedItem{
		3: {ProductID: 3, Times: 7, Spent: 14.0},
	}

	reflection := &HealthReflection{HistoryProducts: len(products)}
	reflection.summarise(products, frequency)

	if reflection.Resolved != 4 {
		t.Errorf("resolved = %d, want 4: the product without a category is not resolvable", reflection.Resolved)
	}
	if reflection.FoodProducts != 3 || reflection.NonFoodProducts != 1 {
		t.Errorf("food/non-food = %d/%d, want 3/1", reflection.FoodProducts, reflection.NonFoodProducts)
	}
	if got, want := reflection.Coverage, 0.8; got != want {
		t.Errorf("coverage = %v, want %v", got, want)
	}

	shares := make(map[string]float64)
	for _, group := range reflection.Groups {
		shares[group.Group] = group.Share
	}
	// Shampoo must not dilute the diet.
	if got := shares["groente en fruit"]; got < 0.66 || got > 0.67 {
		t.Errorf("produce share = %.3f, want 2/3 of food products", got)
	}

	if got, want := reflection.HealthyShare, 2.0/3.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("healthyShare = %.3f, want %.3f", got, want)
	}
	if got, want := reflection.UnhealthyShare, 1.0/3.0; got < want-0.01 || got > want+0.01 {
		t.Errorf("unhealthyShare = %.3f, want %.3f", got, want)
	}

	if len(reflection.Repeaters) != 1 || reflection.Repeaters[0].ProductID != 3 {
		t.Fatalf("repeaters = %+v, want the E-scored product", reflection.Repeaters)
	}
	if reflection.Repeaters[0].Times != 7 {
		t.Errorf("repeat count = %d, want 7 from the receipts", reflection.Repeaters[0].Times)
	}
	if len(reflection.Wins) != 2 {
		t.Errorf("wins = %d, want both A products", len(reflection.Wins))
	}
}

func TestSummariseIgnoresUnscoredProductsInBands(t *testing.T) {
	// Fresh produce and meat usually carry no Nutri-Score. They must count
	// towards the group mix but not towards the score bands, or the coverage
	// figure would be meaningless.
	products := []appie.Product{
		{ID: 1, Category: "Aardappelen, groente, fruit", NutriScore: "A"},
		{ID: 2, Category: "Vlees, kip, vis, vega"},
	}

	reflection := &HealthReflection{HistoryProducts: len(products)}
	reflection.summarise(products, nil)

	if reflection.FoodProducts != 2 {
		t.Errorf("foodProducts = %d, want 2", reflection.FoodProducts)
	}
	if got, want := reflection.NutriScoreCoverage, 0.5; got != want {
		t.Errorf("nutriScoreCoverage = %v, want %v", got, want)
	}
	if got := reflection.HealthyShare; got != 1 {
		t.Errorf("healthyShare = %v, want 1: the only scored product is an A", got)
	}
}

func TestObserveReportsThinData(t *testing.T) {
	empty := &HealthReflection{HistoryProducts: 10}
	empty.observe()
	if len(empty.Observations) == 0 {
		t.Fatal("a reflection with nothing resolved must say so")
	}

	partial := &HealthReflection{HistoryProducts: 10, Resolved: 5, Coverage: 0.5}
	partial.observe()
	found := false
	for _, o := range partial.Observations {
		if len(o) > 0 && o[0] == '5' {
			found = true
		}
	}
	if !found {
		t.Errorf("partial coverage must be stated, got %v", partial.Observations)
	}
}
