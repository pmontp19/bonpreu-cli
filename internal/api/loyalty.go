package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// LoyaltyBalance is the Guardiola (loyalty wallet) balance shown at
// /settings/loyalty on the site.
type LoyaltyBalance struct {
	Money      Money `json:"money"`
	Registered bool  `json:"registered"`
}

// loyaltyPageState mirrors the slice of `window.__INITIAL_STATE__` that
// carries the balance on the /settings/loyalty page.
type loyaltyPageState struct {
	Data struct {
		Customer struct {
			Loyalty struct {
				Balance struct {
					Money Money `json:"money"`
				} `json:"balance"`
				Registered bool `json:"registered"`
			} `json:"loyalty"`
		} `json:"customer"`
	} `json:"data"`
}

// GetLoyaltyBalance scrapes the Guardiola balance from the /settings/loyalty
// page. There is no dedicated JSON API for it (confirmed via HAR capture
// 2026-07-01): the balance is server-rendered into
// `window.__INITIAL_STATE__.data.customer.loyalty`, the same mechanism the
// homepage uses to embed the CSRF token.
func GetLoyaltyBalance(ctx context.Context, c *client.Client) (*LoyaltyBalance, error) {
	b, err := c.DoRaw(ctx, http.MethodGet, "/settings/loyalty")
	if err != nil {
		return nil, err
	}
	js, ok := client.ExtractAppState(string(b))
	if !ok {
		return nil, fmt.Errorf("no __INITIAL_STATE__ on /settings/loyalty page")
	}
	var st loyaltyPageState
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &LoyaltyBalance{
		Money:      st.Data.Customer.Loyalty.Balance.Money,
		Registered: st.Data.Customer.Loyalty.Registered,
	}, nil
}
