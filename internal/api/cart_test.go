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

func TestApplyQuantity_SignedDeltaBodyAndPath(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []api__item
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"basketUpdateResult":{"itemGroups":[{"items":[{"productId":"u1","quantity":3,"totalPrices":{"finalPrice":{"currency":"EUR","amount":"5.70"}}}]}]}}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	cart, err := ApplyQuantity(context.Background(), c, []CartItemInput{
		{ProductID: "u1", Quantity: 2},
		{ProductID: "u2", Quantity: -1},
	})
	if err != nil {
		t.Fatalf("ApplyQuantity: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/cart/v1/carts/active/apply-quantity" {
		t.Errorf("path = %q", gotPath)
	}
	if len(gotBody) != 2 || gotBody[0].Quantity != 2 || gotBody[1].Quantity != -1 {
		t.Errorf("signed-delta body = %+v", gotBody)
	}
	if cart.TotalAmount() != "5.70" {
		t.Errorf("total from POST shape = %q, want 5.70", cart.TotalAmount())
	}
	if cart.QtyOf("u1") != 3 {
		t.Errorf("QtyOf = %d", cart.QtyOf("u1"))
	}
}

type api__item struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

func TestGetActiveCartAndHelpers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cart/v1/carts/active" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cartId":"cart-1","items":[
			{"productId":"u1","quantity":2,"totalPrice":{"currency":"EUR","amount":"3.80"}},
			{"productId":"u2","quantity":1,"totalPrice":{"currency":"EUR","amount":"1.00"}}
		],"totals":{"checkoutTotalPrice":{"currency":"EUR","amount":"4.80"}}}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	cart, err := GetActiveCart(context.Background(), c)
	if err != nil {
		t.Fatalf("GetActiveCart: %v", err)
	}
	if len(cart.Lines()) != 2 {
		t.Fatalf("lines = %d, want 2", len(cart.Lines()))
	}
	if cart.QtyOf("u1") != 2 {
		t.Errorf("QtyOf(u1) = %d, want 2", cart.QtyOf("u1"))
	}
	if cart.QtyOf("missing") != 0 {
		t.Errorf("QtyOf(missing) = %d, want 0", cart.QtyOf("missing"))
	}
}
