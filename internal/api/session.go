package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// AccountStatus reports whether the session is authenticated at the account
// level. This is distinct from guest-cart access: the active cart works even
// for an anonymous session, so a working cart is NOT proof the account session
// is live. Account-scoped endpoints (orders, wallet, slots, delivery
// addresses) all require Authenticated to be true.
type AccountStatus struct {
	Authenticated bool `json:"authenticated"`
}

// GetAccountStatus scrapes the homepage SSR state to decide whether the session
// is authenticated as a customer. The signal is the boolean
// window.__INITIAL_STATE__.session.isLoggedIn, which is server-rendered from the
// session cookies. (Note: the sibling session.customerSession.data is a
// client-side-fetched resource that is null in the SSR HTML for logged-in and
// anonymous alike, so it is NOT a usable signal.) Verified live 2026-07-03: a
// logged-in session carried isLoggedIn == true with orders returning 200, while
// an expired session carried isLoggedIn == false with orders returning 401.
func GetAccountStatus(ctx context.Context, c *client.Client) (*AccountStatus, error) {
	b, err := c.DoRaw(ctx, http.MethodGet, "/")
	if err != nil {
		return nil, err
	}
	js, ok := client.ExtractAppState(string(b))
	if !ok {
		return nil, fmt.Errorf("no __INITIAL_STATE__ on homepage")
	}
	var st struct {
		Session struct {
			IsLoggedIn bool `json:"isLoggedIn"`
		} `json:"session"`
	}
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		return nil, fmt.Errorf("parse homepage state: %w", err)
	}
	return &AccountStatus{Authenticated: st.Session.IsLoggedIn}, nil
}
