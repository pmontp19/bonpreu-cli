package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestSearchProducts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/product-pages/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "iogurt" {
			t.Errorf("q = %q", r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"productGroups":[{"type":"featured","decoratedProducts":[` +
			`{"productId":"uuid-1","retailerProductId":"111","name":"Iogurt natural","price":{"currency":"EUR","amount":"1.20"}},` +
			`{"productId":"uuid-2","retailerProductId":"222","name":"Iogurt grec","price":{"currency":"EUR","amount":"2.50"}}]}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	prods, err := SearchProducts(context.Background(), c, "iogurt", 10)
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	if len(prods) != 2 || prods[0].RetailerProductID != "111" || prods[0].Price.Amount != "1.20" {
		t.Fatalf("unexpected: %+v", prods)
	}
}

func TestResolveProductID_UUIDPassthrough(t *testing.T) {
	c, _ := client.New(&config.Session{}, nil)
	cache := &config.IDCache{RetailerToProduct: map[string]string{}}
	got, err := ResolveProductID(context.Background(), c, "98dc2105-04ed-4cd3-9e0b-5fa77dab0176", cache)
	if err != nil || got != "98dc2105-04ed-4cd3-9e0b-5fa77dab0176" {
		t.Fatalf("uuid passthrough: %v %q", err, got)
	}
}

func TestResolveProductID_CacheHit(t *testing.T) {
	c, _ := client.New(&config.Session{}, nil)
	cache := &config.IDCache{RetailerToProduct: map[string]string{"74927": "uuid-x"}}
	got, err := ResolveProductID(context.Background(), c, "74927", cache)
	if err != nil || got != "uuid-x" {
		t.Fatalf("cache hit: %v %q", err, got)
	}
}

func TestScrapeProductID_FromHTML(t *testing.T) {
	html := `<html><script>window.__INITIAL_STATE__={"queries":[{"state":{"data":{"product":{"productId":"98dc2105-04ed-4cd3-9e0b-5fa77dab0176","retailerProductId":"74927"}}}}]}</script></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	got, err := scrapeProductID(context.Background(), c, "74927")
	if err != nil {
		t.Fatalf("scrape: %v", err)
	}
	if got != "98dc2105-04ed-4cd3-9e0b-5fa77dab0176" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveProductID_NumericNotInCacheErrors(t *testing.T) {
	// Page has no matching state, so the scrape fallback fails and the
	// resolver surfaces an error rather than hitting the live site.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>not found</body></html>`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	cache := &config.IDCache{RetailerToProduct: map[string]string{}}
	_, err := ResolveProductID(context.Background(), c, "99999", cache)
	if err == nil {
		t.Fatal("expected error for uncached numeric id")
	}
}

func TestResolveProductID_ScrapeFallback(t *testing.T) {
	html := `<html><script>window.__INITIAL_STATE__={"queries":[{"state":{"data":{"product":` +
		`{"productId":"98dc2105-04ed-4cd3-9e0b-5fa77dab0176","retailerProductId":"74927"}}}}]}</script></html>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	// nil cache: resolver must still scrape and return the uuid.
	got, err := ResolveProductID(context.Background(), c, "74927", nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "98dc2105-04ed-4cd3-9e0b-5fa77dab0176" {
		t.Fatalf("got %q", got)
	}
}

func TestIsUUID(t *testing.T) {
	cases := map[string]bool{
		"98dc2105-04ed-4cd3-9e0b-5fa77dab0176": true,
		"74927":                                false,
		"":                                     false,
		"ABC-DEF":                              false,
	}
	for in, want := range cases {
		if got := IsUUID(in); got != want {
			t.Errorf("IsUUID(%q) = %v, want %v", in, got, want)
		}
	}
}
