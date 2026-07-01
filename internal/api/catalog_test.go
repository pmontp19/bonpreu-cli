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
	html := `<html><script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"product":{"productId":"98dc2105-04ed-4cd3-9e0b-5fa77dab0176","retailerProductId":"74927"}}}}]}</script></html>`
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

func TestScrapeProductID_PrefersQueryStateOverInitialState(t *testing.T) {
	// Real product pages carry two blobs: __INITIAL_STATE__ (app state with
	// recommendations — a decoy that lacks the target id) and
	// __QUERY_INITIAL_STATE__ (the React-Query cache holding the page product).
	// The scraper must resolve from the query blob, not the decoy.
	html := `<html><script>window.__INITIAL_STATE__={"recommendations":[{"productId":"WRONG","retailerProductId":"74927"}]}</script>` +
		`<script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"product":{"productId":"98dc2105-04ed-4cd3-9e0b-5fa77dab0176","retailerProductId":"74927"}}}}]}</script></html>`
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
		t.Fatalf("got %q, want the query-state uuid (not the decoy)", got)
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
	html := `<html><script>window.__QUERY_INITIAL_STATE__={"queries":[{"state":{"data":{"product":` +
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

func TestGetCategories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/categories") {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.URL.Query().Get("categoryDepth") != "2" {
			t.Errorf("depth = %q", r.URL.Query().Get("categoryDepth"))
		}
		_, _ = w.Write([]byte(`[{"categoryId":"c1","name":"Frescos","productCount":10,"childCategories":[{"categoryId":"c2","name":"Fruita"}]}]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	cats, err := GetCategories(context.Background(), c, 2)
	if err != nil {
		t.Fatalf("GetCategories: %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "Frescos" || len(cats[0].ChildCategories) != 1 {
		t.Fatalf("unexpected: %+v", cats)
	}
}

func TestGetRelated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("retailerProductId") != "74927" {
			t.Errorf("retailerProductId = %q", r.URL.Query().Get("retailerProductId"))
		}
		_, _ = w.Write([]byte(`["uuid-a","uuid-b","uuid-c"]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	rel, err := GetRelated(context.Background(), c, "74927")
	if err != nil {
		t.Fatalf("GetRelated: %v", err)
	}
	if len(rel) != 3 || rel[0] != "uuid-a" {
		t.Fatalf("unexpected: %+v", rel)
	}
}

func TestGetProducts_ShapesAndEmpty(t *testing.T) {
	// PUT body is the uuid list; server returns the "products" wrapper shape.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		_, _ = w.Write([]byte(`{"products":[{"productId":"uuid-1","retailerProductId":"111","name":"Iogurt","price":{"currency":"EUR","amount":"1.20"}}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	prods, err := GetProducts(context.Background(), c, []string{"uuid-1"})
	if err != nil {
		t.Fatalf("GetProducts: %v", err)
	}
	if len(prods) != 1 || prods[0].Name != "Iogurt" {
		t.Fatalf("unexpected: %+v", prods)
	}

	// Empty uuid list short-circuits without a request.
	if got, err := GetProducts(context.Background(), c, nil); err != nil || got != nil {
		t.Fatalf("empty input: %v %+v", err, got)
	}
}

func TestGetProducts_EmptyAndUnparseable(t *testing.T) {
	// Empty JSON body → nil, no error.
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer empty.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = empty.URL
	if got, err := GetProducts(context.Background(), c, []string{"u1"}); err != nil || got != nil {
		t.Fatalf("empty: %v %+v", err, got)
	}

	// Unrecognized shape → parse error (exercises truncateRaw).
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"weird":123}`))
	}))
	defer bad.Close()
	c.BaseURL = bad.URL
	if _, err := GetProducts(context.Background(), c, []string{"u1"}); err == nil {
		t.Fatal("expected parse error for unrecognized shape")
	}
}

func TestEndpointsPropagateHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	ctx := context.Background()

	if _, err := SearchProducts(ctx, c, "q", 5); err == nil {
		t.Error("SearchProducts should propagate 500")
	}
	if _, err := GetCategories(ctx, c, 0); err == nil {
		t.Error("GetCategories should propagate 500")
	}
	if _, err := GetRelated(ctx, c, "111"); err == nil {
		t.Error("GetRelated should propagate 500")
	}
	if _, err := GetProducts(ctx, c, []string{"u1"}); err == nil {
		t.Error("GetProducts should propagate 500")
	}
}

func TestParseProducts_Variants(t *testing.T) {
	cases := map[string]string{
		"direct": `[{"productId":"p","name":"A"}]`,
		"group":  `{"productGroups":[{"decoratedProducts":[{"productId":"p","name":"A"}]}]}`,
	}
	for name, body := range cases {
		prods, ok := parseProducts([]byte(body))
		if !ok || len(prods) != 1 || prods[0].Name != "A" {
			t.Errorf("%s: ok=%v prods=%+v", name, ok, prods)
		}
	}
	if _, ok := parseProducts([]byte(`{"unexpected":true}`)); ok {
		t.Error("unrecognized shape should not parse")
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
