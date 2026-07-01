package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetLoyaltyBalance_FromHTML(t *testing.T) {
	// Real /settings/loyalty pages carry two blobs: __INITIAL_STATE__ (the
	// app state, holding data.customer.loyalty) and __QUERY_INITIAL_STATE__
	// (an unrelated, near-empty React-Query cache on this page). The scraper
	// must resolve from __INITIAL_STATE__, not be tripped up by the decoy.
	html := `<html><script>window.__INITIAL_STATE__={"data":{"customer":{"loyalty":` +
		`{"balance":{"units":461,"money":{"amount":"4.61","currency":"EUR"}},"registered":true}}}}</script>` +
		`<script>window.__QUERY_INITIAL_STATE__={}</script></html>`
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	bal, err := GetLoyaltyBalance(context.Background(), c)
	if err != nil {
		t.Fatalf("GetLoyaltyBalance: %v", err)
	}
	if gotPath != "/settings/loyalty" {
		t.Errorf("path = %q", gotPath)
	}
	if bal.Money.Amount != "4.61" || bal.Money.Currency != "EUR" || !bal.Registered {
		t.Fatalf("unexpected: %+v", bal)
	}
}

func TestGetLoyaltyBalance_NoInitialState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>not found</body></html>`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	if _, err := GetLoyaltyBalance(context.Background(), c); err == nil {
		t.Fatal("expected error when __INITIAL_STATE__ is absent")
	}
}

func TestGetLoyaltyBalance_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	if _, err := GetLoyaltyBalance(context.Background(), c); err == nil {
		t.Fatal("expected error to propagate")
	}
}
