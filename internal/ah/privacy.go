package ah

import (
	"context"
	"fmt"
	"strconv"
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

	if len(resp.Member.Properties) > 0 {
		profile.Properties = make(map[string]any, len(resp.Member.Properties))
		for _, p := range resp.Member.Properties {
			profile.Properties[p.Key] = p.Value
		}
	}

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
