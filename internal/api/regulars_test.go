package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetRegulars_FlattensProductGroups(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"productGroups":[{"type":"regular","products":[
			{"productId":"p1","product":{"productId":"p1","name":"Patates","brand":"LA COLLITA","price":{"currency":"EUR","amount":"3.79"},"regular":{"quantity":1,"frequency":"WEEKLY"}}},
			{"productId":"p2","product":{"productId":"p2","name":"Llet","regular":{"quantity":6,"frequency":"FORTNIGHTLY"}}}
		]}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	items, err := GetRegulars(context.Background(), c)
	if err != nil {
		t.Fatalf("GetRegulars: %v", err)
	}
	if gotPath != "/api/webproductpagews/v5/product-pages/regulars" {
		t.Errorf("path = %q", gotPath)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].Name != "Patates" || items[0].Quantity != 1 || items[0].Frequency != "WEEKLY" || items[0].Price.Amount != "3.79" {
		t.Errorf("items[0] = %+v", items[0])
	}
	if items[1].Name != "Llet" || items[1].Quantity != 6 || items[1].Frequency != "FORTNIGHTLY" {
		t.Errorf("items[1] = %+v", items[1])
	}
}

func TestGetRegulars_IgnoresFavoriteGroup(t *testing.T) {
	// The same endpoint returns both a "regular" and a "favorite" productGroup
	// in one call (see GetFavorites); GetRegulars must only surface the former.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"productGroups":[
			{"type":"regular","products":[{"productId":"p1","product":{"productId":"p1","name":"Patates","regular":{"quantity":1,"frequency":"WEEKLY"}}}]},
			{"type":"favorite","products":[{"productId":"p2","product":{"productId":"p2","name":"Cervesa"}}]}
		]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	items, err := GetRegulars(context.Background(), c)
	if err != nil {
		t.Fatalf("GetRegulars: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Patates" {
		t.Fatalf("expected only the regular-group item, got %+v", items)
	}
}

func TestGetRegulars_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"productGroups":[]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	items, err := GetRegulars(context.Background(), c)
	if err != nil || len(items) != 0 {
		t.Fatalf("GetRegulars: %v %+v", err, items)
	}
}

func TestInstantShop(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"addedProducts":[{"productId":"p1","quantity":1},{"productId":"p2","quantity":3}],` +
			`"basketUpdateResult":{"totals":{"itemPriceAfterPromos":{"currency":"EUR","amount":"74.46"}}}}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	res, err := InstantShop(context.Background(), c)
	if err != nil {
		t.Fatalf("InstantShop: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/cart/v2/instant-shop" {
		t.Errorf("method/path = %s %s", gotMethod, gotPath)
	}
	if gotBody != "{}" {
		t.Errorf("body = %q, want {}", gotBody)
	}
	if len(res.AddedProducts) != 2 || res.AddedProducts[1].Quantity != 3 {
		t.Errorf("addedProducts = %+v", res.AddedProducts)
	}
	if res.Total != "74.46" {
		t.Errorf("total = %q", res.Total)
	}
}

func TestRegularsEndpointsPropagateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	ctx := context.Background()

	if _, err := GetRegulars(ctx, c); err == nil {
		t.Error("GetRegulars should propagate 500")
	}
	if _, err := InstantShop(ctx, c); err == nil {
		t.Error("InstantShop should propagate 500")
	}
}
