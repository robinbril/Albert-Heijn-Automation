package ah

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// MemberProfile is the member record. Identifying fields are only populated
// when the caller explicitly asks for them; see GetMemberProfile.
type MemberProfile struct {
	// Redacted reports whether identifying fields were withheld.
	Redacted bool `json:"redacted"`

	FirstName   string          `json:"firstName,omitempty"`
	LastName    string          `json:"lastName,omitempty"`
	Email       string          `json:"email,omitempty"`
	PhoneNumber string          `json:"phoneNumber,omitempty"`
	DateOfBirth string          `json:"dateOfBirth,omitempty"`
	Address     *ProfileAddress `json:"address,omitempty"`
	BonusCard   string          `json:"bonusCardNumber,omitempty"`
	GallCard    string          `json:"gallCardNumber,omitempty"`

	// Audiences are the segments AH sorted this member into.
	Audiences []string `json:"audiences,omitempty"`
	// Properties is AH's own profiling: life stage, food profile, price
	// segment, favourite shopping day and so on.
	Properties map[string]any `json:"properties,omitempty"`
	// PropertiesWithheld names the profiling keys that were dropped because
	// they are not on the safe list. Names only, never values.
	PropertiesWithheld []string `json:"propertiesWithheld,omitempty"`
}

// safeProfileProperty reports whether a customerProfileProperties key may be
// returned while identifying fields are being withheld.
//
// AH owns these keys and adds new ones without warning, so the rule is an
// allow-list rather than a list of things to strip: a key nobody has vetted is
// withheld by name instead of shipped into an assistant's context, which is
// what would happen the day AH starts returning a birth year or a postal
// district under a new name.
func safeProfileProperty(key string) bool {
	k := strings.ToLower(key)
	for _, allowed := range []string{
		"food", "diet", "meal", "recipe", "cuisine",
		"lifestage", "life_stage", "household_composition",
		"segment", "audience", "cluster",
		"shopping_day", "shoppingday", "shopping_frequency",
		"price_sensitivity", "price_segment", "promotion",
		"sustainab", "organic", "vegetarian", "vegan",
	} {
		if strings.Contains(k, allowed) {
			return true
		}
	}
	return false
}

// ProfileAddress is a member's delivery address.
type ProfileAddress struct {
	Street           string `json:"street,omitempty"`
	HouseNumber      int    `json:"houseNumber,omitempty"`
	HouseNumberExtra string `json:"houseNumberExtra,omitempty"`
	PostalCode       string `json:"postalCode,omitempty"`
	City             string `json:"city,omitempty"`
	CountryCode      string `json:"countryCode,omitempty"`
}

const memberProfileQuery = `query MemberProfile {
  member {
    id
    name { first last }
    emailAddress
    phoneNumber
    dateOfBirth
    address { street houseNumber houseNumberExtra postalCode city countryCode }
    cards { bonus gall }
    customerProfileAudiences
    customerProfileProperties { key value }
  }
}`

// GetMemberProfile reads the member record. Name, email, phone, date of birth,
// address and loyalty card numbers are withheld unless includePII is true:
// this data ends up in an assistant's context by default, and the segmentation
// data is the part that is actually useful for meal planning.
//
// appie-go's GetMember drops customerProfileProperties, so this issues its own
// query to keep AH's profiling visible.
func (c *Client) GetMemberProfile(ctx context.Context, includePII bool) (*MemberProfile, error) {
	var resp struct {
		Member struct {
			ID   memberID `json:"id"`
			Name struct {
				First string `json:"first"`
				Last  string `json:"last"`
			} `json:"name"`
			EmailAddress string          `json:"emailAddress"`
			PhoneNumber  string          `json:"phoneNumber"`
			DateOfBirth  string          `json:"dateOfBirth"`
			Address      *ProfileAddress `json:"address"`
			Cards        struct {
				Bonus string `json:"bonus"`
				Gall  string `json:"gall"`
			} `json:"cards"`
			Audiences  []string `json:"customerProfileAudiences"`
			Properties []struct {
				Key   string `json:"key"`
				Value any    `json:"value"`
			} `json:"customerProfileProperties"`
		} `json:"member"`
	}

	if err := c.api.DoGraphQL(ctx, memberProfileQuery, nil, &resp); err != nil {
		return nil, fmt.Errorf("get member profile failed: %w", err)
	}

	profile := &MemberProfile{
		Redacted:  !includePII,
		Audiences: resp.Member.Audiences,
	}

	for _, p := range resp.Member.Properties {
		if !includePII && !safeProfileProperty(p.Key) {
			profile.PropertiesWithheld = append(profile.PropertiesWithheld, p.Key)
			continue
		}
		if profile.Properties == nil {
			profile.Properties = make(map[string]any, len(resp.Member.Properties))
		}
		profile.Properties[p.Key] = p.Value
	}
	sort.Strings(profile.PropertiesWithheld)

	if includePII {
		profile.FirstName = resp.Member.Name.First
		profile.LastName = resp.Member.Name.Last
		profile.Email = resp.Member.EmailAddress
		profile.PhoneNumber = resp.Member.PhoneNumber
		profile.DateOfBirth = resp.Member.DateOfBirth
		profile.Address = resp.Member.Address
		profile.BonusCard = resp.Member.Cards.Bonus
		profile.GallCard = resp.Member.Cards.Gall
	}

	return profile, nil
}

// Delivery is a scheduled order. The delivery address is only populated when
// the caller explicitly asks for it; see GetDeliveries.
type Delivery struct {
	// Redacted reports whether the delivery address was withheld.
	Redacted bool `json:"redacted"`

	OrderID           int     `json:"orderId"`
	Status            string  `json:"status,omitempty"`
	StatusDescription string  `json:"statusDescription,omitempty"`
	ShoppingType      string  `json:"shoppingType,omitempty"`
	TotalPrice        float64 `json:"totalPrice,omitempty"`
	Completed         bool    `json:"transactionCompleted"`
	Modifiable        bool    `json:"modifiable"`

	DeliveryStatus string `json:"deliveryStatus,omitempty"`
	DeliveryMethod string `json:"deliveryMethod,omitempty"`
	Date           string `json:"date,omitempty"`
	DateDisplay    string `json:"dateDisplay,omitempty"`
	TimeDisplay    string `json:"timeDisplay,omitempty"`

	Address *ProfileAddress `json:"address,omitempty"`
}

// GetDeliveries lists scheduled orders. The delivery address is withheld unless
// includePII is true.
//
// appie-go returns the full address on every fulfillment, which would put the
// user's home address in an assistant's context as a side effect of asking when
// the groceries arrive. The slot is the useful part; the address is not.
func (c *Client) GetDeliveries(ctx context.Context, includePII bool) ([]Delivery, error) {
	fulfillments, err := c.api.GetFulfillments(ctx)
	if err != nil {
		return nil, fmt.Errorf("get deliveries failed: %w", err)
	}

	deliveries := make([]Delivery, 0, len(fulfillments))
	for _, f := range fulfillments {
		d := Delivery{
			Redacted:          !includePII,
			OrderID:           f.OrderID,
			Status:            f.Status,
			StatusDescription: f.StatusDescription,
			ShoppingType:      f.ShoppingType,
			TotalPrice:        f.TotalPrice,
			Completed:         f.TransactionCompleted,
			Modifiable:        f.Modifiable,
			DeliveryStatus:    f.Delivery.Status,
			DeliveryMethod:    f.Delivery.Method,
			Date:              f.Delivery.Slot.Date,
			DateDisplay:       f.Delivery.Slot.DateDisplay,
			TimeDisplay:       f.Delivery.Slot.TimeDisplay,
		}
		if includePII {
			a := f.Delivery.Address
			d.Address = &ProfileAddress{
				Street:           a.Street,
				HouseNumber:      a.HouseNumber,
				HouseNumberExtra: a.HouseNumberExtra,
				PostalCode:       a.PostalCode,
				City:             a.City,
				CountryCode:      a.CountryCode,
			}
		}
		deliveries = append(deliveries, d)
	}
	return deliveries, nil
}

// memberID accepts an id that the API may send as either a string or a number.
type memberID string

func (m *memberID) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' {
		unquoted, err := strconv.Unquote(s)
		if err != nil {
			return err
		}
		*m = memberID(unquoted)
		return nil
	}
	*m = memberID(s)
	return nil
}
