package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetFavorites_EnrichesViaBatchLookup(t *testing.T) {
	// Real favorite entries past the page's decoration budget arrive as bare
	// {"productId":...} — GetFavorites must always re-enrich via GetProducts
	// rather than trust partial decoration on the page itself.
	var gotPUTPath string
	var gotPUTBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"productGroups":[
				{"type":"regular","products":[{"productId":"r1","product":{"productId":"r1","name":"Patates"}}]},
				{"type":"favorite","products":[{"productId":"f1"},{"productId":"f2","product":{"productId":"f2","name":"decorated but ignored"}}]}
			]}`))
		case http.MethodPut:
			gotPUTPath = r.URL.Path
			b, _ := io.ReadAll(r.Body)
			gotPUTBody = string(b)
			_, _ = w.Write([]byte(`{"products":[
				{"productId":"f1","name":"Cervesa sense alcohol","brand":"FREE DAMM","price":{"currency":"EUR","amount":"3.89"}},
				{"productId":"f2","name":"Cervesa Moritz","price":{"currency":"EUR","amount":"0.87"}}
			]}`))
		}
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	items, err := GetFavorites(context.Background(), c)
	if err != nil {
		t.Fatalf("GetFavorites: %v", err)
	}
	if gotPUTPath != "/api/webproductpagews/v6/products" {
		t.Errorf("PUT path = %q", gotPUTPath)
	}
	if !strings.Contains(gotPUTBody, "f1") || !strings.Contains(gotPUTBody, "f2") || strings.Contains(gotPUTBody, "r1") {
		t.Errorf("PUT body = %q, want only favorite ids (not the regular group)", gotPUTBody)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[0].ProductID != "f1" || items[0].Name != "Cervesa sense alcohol" || items[0].Brand != "FREE DAMM" || items[0].Price.Amount != "3.89" {
		t.Errorf("items[0] = %+v", items[0])
	}
}

func TestGetFavorites_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"productGroups":[{"type":"regular","products":[]}]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	items, err := GetFavorites(context.Background(), c)
	if err != nil || len(items) != 0 {
		t.Fatalf("GetFavorites: %v %+v", err, items)
	}
}

func TestGetFavorites_PropagatesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	if _, err := GetFavorites(context.Background(), c); err == nil {
		t.Fatal("expected error to propagate")
	}
}
