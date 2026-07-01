package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/api"
	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestEnrichNames_FillsMissingViaBatchedCall(t *testing.T) {
	var productCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/products") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		productCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"productId":"u1","name":"Iogurt natural"},
			{"productId":"u2","name":"Llet sencera"}
		]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	rt := runtime{client: c}

	lines := []api.CartItem{
		{ProductID: "u1", Quantity: 1},
		{ProductID: "u2", Quantity: 2, Name: "already has a name"},
	}
	got := enrichNames(context.Background(), rt, lines)
	if productCalls != 1 {
		t.Fatalf("expected 1 batched products call, got %d", productCalls)
	}
	if got[0].Name != "Iogurt natural" {
		t.Errorf("line 0 name = %q, want enriched name", got[0].Name)
	}
	if got[1].Name != "already has a name" {
		t.Errorf("line 1 name = %q, want the pre-existing name preserved", got[1].Name)
	}
}

func TestEnrichNames_NoCallWhenAllNamesPresent(t *testing.T) {
	var productCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		productCalls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	rt := runtime{client: c}

	lines := []api.CartItem{{ProductID: "u1", Quantity: 1, Name: "Iogurt"}}
	got := enrichNames(context.Background(), rt, lines)
	if productCalls != 0 {
		t.Fatalf("expected no products call when every line already has a name, got %d", productCalls)
	}
	if got[0].Name != "Iogurt" {
		t.Errorf("name = %q, want unchanged", got[0].Name)
	}
}

func TestEnrichNames_FallsBackToIDsOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	rt := runtime{client: c}

	lines := []api.CartItem{{ProductID: "u1", Quantity: 1}}
	got := enrichNames(context.Background(), rt, lines)
	if got[0].Name != "" {
		t.Errorf("name = %q, want empty (caller falls back to printing the ID)", got[0].Name)
	}
	if got[0].ProductID != "u1" {
		t.Errorf("ProductID = %q, want unchanged", got[0].ProductID)
	}
}

func TestPrintCart_JSONIncludesEnrichedNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/products") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"productId":"u1","name":"Iogurt natural"}]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	rt := runtime{client: c, json: true}

	cart := &api.Cart{Items: []api.CartItem{{ProductID: "u1", Quantity: 1}}}
	out := captureStdout(t, func() {
		if err := printCart(context.Background(), rt, cart); err != nil {
			t.Fatalf("printCart: %v", err)
		}
	})
	if !strings.Contains(out, "Iogurt natural") {
		t.Fatalf("json output = %q, want it to contain the enriched name", out)
	}
}
