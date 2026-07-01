package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestApplyVouchers_Invalid(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pricingNotifications":[],"vouchersAddResult":[` +
			`{"inBasket":false,"newlyAdded":false,"valid":false,"validationErrorCode":"CODE_NOT_FOUND","voucherId":"test5"}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	results, err := ApplyVouchers(context.Background(), c, []string{"TEST5"})
	if err != nil {
		t.Fatalf("ApplyVouchers: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/cart/v1/carts/active/vouchers" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if len(gotBody) != 1 || gotBody[0] != "TEST5" {
		t.Errorf("request body = %+v", gotBody)
	}
	if len(results) != 1 || results[0].Valid || results[0].ValidationErrorCode != "CODE_NOT_FOUND" || results[0].VoucherID != "test5" {
		t.Errorf("results = %+v", results)
	}
}

func TestApplyVouchers_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"vouchersAddResult":[{"inBasket":true,"newlyAdded":true,"valid":true,"voucherId":"welcome10"}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	results, err := ApplyVouchers(context.Background(), c, []string{"WELCOME10"})
	if err != nil {
		t.Fatalf("ApplyVouchers: %v", err)
	}
	if len(results) != 1 || !results[0].Valid || !results[0].NewlyAdded || !results[0].InBasket || results[0].ValidationErrorCode != "" {
		t.Errorf("results = %+v", results)
	}
}

func TestApplyVouchers_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	if _, err := ApplyVouchers(context.Background(), c, []string{"X"}); err == nil {
		t.Fatal("expected error to propagate")
	}
}
