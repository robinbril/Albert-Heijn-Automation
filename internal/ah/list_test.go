package ah

import (
	"encoding/json"
	"testing"
)

// listPayload mirrors a trimmed v2 shopping list response.
const listPayload = `{
  "id": "list-1",
  "items": [
    {
      "listItemId": 1,
      "quantity": 2,
      "strikedthrough": false,
      "productDetails": {
        "product": {
          "webshopId": 111,
          "title": "Tomatenblokjes",
          "currentPrice": 0.75,
          "priceBeforeBonus": 1.00,
          "isBonus": true
        }
      }
    },
    {
      "listItemId": 2,
      "quantity": 1,
      "strikedthrough": true,
      "productDetails": {
        "product": {
          "webshopId": 222,
          "title": "Melk",
          "currentPrice": 1.50,
          "priceBeforeBonus": 1.50
        }
      }
    },
    {
      "listItemId": 3,
      "quantity": 1,
      "strikedthrough": false,
      "description": "Slager: kipfilet"
    }
  ]
}`

func TestSummarise(t *testing.T) {
	var raw listItemsResponse
	if err := json.Unmarshal([]byte(listPayload), &raw); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	list := summarise(&raw)

	if len(list.Items) != 3 {
		t.Fatalf("items = %d, want 3", len(list.Items))
	}
	// The struck-through line is reported but must not count toward the bill.
	if list.Count != 2 {
		t.Errorf("count = %d, want 2 unchecked lines", list.Count)
	}
	if list.CheckedCount != 1 {
		t.Errorf("checkedCount = %d, want 1", list.CheckedCount)
	}

	if got, want := list.Total, 1.50; got != want {
		t.Errorf("total = %.2f, want %.2f", got, want)
	}
	if got, want := list.TotalBeforeBonus, 2.00; got != want {
		t.Errorf("totalBeforeBonus = %.2f, want %.2f", got, want)
	}
	if got, want := list.TotalDiscount, 0.50; got != want {
		t.Errorf("totalDiscount = %.2f, want %.2f", got, want)
	}
}

func TestSummariseMarksFreeText(t *testing.T) {
	var raw listItemsResponse
	if err := json.Unmarshal([]byte(listPayload), &raw); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	list := summarise(&raw)
	note := list.Items[2]

	if !note.FreeText {
		t.Error("a line without a product must be marked as free text")
	}
	if note.Title != "Slager: kipfilet" {
		t.Errorf("title = %q, want the description", note.Title)
	}
	if ref := note.Ref(); ref.Text != "Slager: kipfilet" || ref.ProductID != 0 {
		t.Errorf("free-text ref = %+v, want a text reference", ref)
	}
}

func TestListEntryRefPrefersProductID(t *testing.T) {
	entry := ListEntry{ProductID: 42, Title: "Melk"}
	if ref := entry.Ref(); ref.ProductID != 42 || ref.Text != "" {
		t.Errorf("ref = %+v, want product reference 42", ref)
	}
}

func TestNewPatchItemRemovalUsesZeroQuantity(t *testing.T) {
	// Quantity 0 is a real delete; strike-through only crosses a line off.
	item := newPatchItem(ItemRef{ProductID: 7}, 0, false)
	if item.Quantity != 0 || item.StrikeThrough {
		t.Errorf("removal patch = %+v, want quantity 0 without strike-through", item)
	}
	if item.ProductID != 7 || item.Description != "" {
		t.Errorf("patch = %+v, want productId 7 and no description", item)
	}

	note := newPatchItem(ItemRef{Text: "Slager: kipfilet"}, 0, false)
	if note.Description != "Slager: kipfilet" || note.ProductID != 0 {
		t.Errorf("free-text patch = %+v, want description only", note)
	}
}

func TestRefKey(t *testing.T) {
	if refKey(ItemRef{ProductID: 5}) == refKey(ItemRef{Text: "5"}) {
		t.Error("a product id and a note with the same text must not collide")
	}
	if refKey(ItemRef{Text: " Melk "}) != refKey(ItemRef{Text: "melk"}) {
		t.Error("note keys must ignore case and surrounding space")
	}
}
