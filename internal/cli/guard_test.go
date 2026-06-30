package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func tmpFile(t *testing.T, content string) *os.File {
	t.Helper()
	f, err := os.CreateTemp("", "bp-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close(); _ = os.Remove(f.Name()) })
	return f
}

func newGuardClient(t *testing.T, cartTotal string, productPrice string) (*client.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/carts/active"):
			tot := cartTotal
			if tot == "" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"items":[]}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"totals":{"itemPriceAfterPromos":{"currency":"EUR","amount":"` + tot + `"}},"items":[]}`))
		case strings.HasSuffix(r.URL.Path, "/products"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"productId":"u1","price":{"currency":"EUR","amount":"` + productPrice + `"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	return c, srv.Close
}

func TestEnforceAdd_Disabled(t *testing.T) {
	g := guard{max: 0}
	if err := g.enforceAdd(context.Background(), runtime{}, nil); err != nil {
		t.Fatalf("disabled guard should pass without network: %v", err)
	}
}

func TestEnforceAdd_WithinLimit(t *testing.T) {
	c, stop := newGuardClient(t, "8.00", "1.90")
	defer stop()
	rt := runtime{client: c}
	g := guard{max: 10}
	err := g.enforceAdd(context.Background(), rt, []api.CartItemInput{{ProductID: "u1", Quantity: 1}})
	if err != nil {
		t.Fatalf("8.00 + 1.90 <= 10 should pass: %v", err)
	}
}

func TestEnforceAdd_OverLimit(t *testing.T) {
	c, stop := newGuardClient(t, "8.00", "1.90")
	defer stop()
	rt := runtime{client: c}
	g := guard{max: 10}
	err := g.enforceAdd(context.Background(), rt, []api.CartItemInput{{ProductID: "u1", Quantity: 2}})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("8.00 + 3.80 > 10 should be refused, got: %v", err)
	}
}

func TestEnforceAdd_UnreadableTotalFailsClosed(t *testing.T) {
	c, stop := newGuardClient(t, "", "1.90")
	defer stop()
	rt := runtime{client: c}
	g := guard{max: 10}
	err := g.enforceAdd(context.Background(), rt, []api.CartItemInput{{ProductID: "u1", Quantity: 1}})
	if err == nil || !strings.Contains(err.Error(), "fail-closed") {
		t.Fatalf("unreadable total must fail closed, got: %v", err)
	}
}

func TestGuardResolution(t *testing.T) {
	t.Run("flag wins", func(t *testing.T) {
		t.Setenv("BONPREU_HOME", t.TempDir())
		t.Setenv("BONPREU_MAX_EUR", "5")
		rt := runtime{flags: &Flags{Max: 12.5}}
		g, err := rt.guard()
		if err != nil || g.max != 12.5 {
			t.Fatalf("flag should win: %v max=%v", err, g.max)
		}
	})
	t.Run("env over config", func(t *testing.T) {
		t.Setenv("BONPREU_HOME", t.TempDir())
		t.Setenv("BONPREU_MAX_EUR", "7.5")
		rt := runtime{flags: &Flags{}}
		g, err := rt.guard()
		if err != nil || g.max != 7.5 {
			t.Fatalf("env should apply: %v max=%v", err, g.max)
		}
	})
	t.Run("bad env errors", func(t *testing.T) {
		t.Setenv("BONPREU_HOME", t.TempDir())
		t.Setenv("BONPREU_MAX_EUR", "notnum")
		rt := runtime{flags: &Flags{}}
		if _, err := rt.guard(); err == nil {
			t.Fatal("malformed BONPREU_MAX_EUR must error, not silently disable")
		}
	})
	t.Run("config fallback", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("BONPREU_HOME", dir)
		os.Unsetenv("BONPREU_MAX_EUR")
		if err := os.WriteFile(dir+"/config.json", []byte(`{"default_max_eur":3.25}`), 0o600); err != nil {
			t.Fatal(err)
		}
		rt := runtime{flags: &Flags{}}
		g, err := rt.guard()
		if err != nil || g.max != 3.25 {
			t.Fatalf("config default should apply: %v max=%v", err, g.max)
		}
	})
}

func TestReadItemsJSONLines(t *testing.T) {
	c, stop := newGuardClient(t, "0", "1.90")
	defer stop()
	rt := runtime{client: c, flags: &Flags{}}
	cache := &config.IDCache{RetailerToProduct: map[string]string{"111": "u1"}}

	in := tmpFile(t, `{"id":"111","qty":2}
{"id":"22222222-2222-2222-2222-222222222222","qty":3}

{"id":"111"}`)
	items, err := readItemsJSONLines(in, context.Background(), rt, cache)
	if err != nil {
		t.Fatalf("readItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d (%+v)", len(items), items)
	}
	if items[0].ProductID != "u1" || items[0].Quantity != 2 {
		t.Errorf("item0 = %+v", items[0])
	}
	if items[1].ProductID != "22222222-2222-2222-2222-222222222222" {
		t.Errorf("uuid passthrough item = %+v", items[1])
	}
	if items[2].Quantity != 1 {
		t.Errorf("default qty should be 1, got %d", items[2].Quantity)
	}
}
