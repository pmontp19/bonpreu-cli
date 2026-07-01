package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func newWhoamiClient(t *testing.T) (*client.Client, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totals":{"itemPriceAfterPromos":{"currency":"EUR","amount":"5.70"}},"items":[
			{"productId":"u1","quantity":3,"totalPrice":{"currency":"EUR","amount":"3.80"}},
			{"productId":"u2","quantity":2,"totalPrice":{"currency":"EUR","amount":"1.90"}}
		]}`))
	}))
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	return c, srv.Close
}

func TestWhoami_PlainTextReportsProductsAndArticles(t *testing.T) {
	c, stop := newWhoamiClient(t)
	defer stop()
	rt := runtime{client: c, json: false}
	ctx := ctxWithRuntime(context.Background(), rt)
	cmd := newWhoamiCmd()
	cmd.SetContext(ctx)

	out := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("whoami: %v", err)
		}
	})
	// 2 distinct product lines, 3+2=5 total articles.
	if !strings.Contains(out, "2 products") || !strings.Contains(out, "5 articles") {
		t.Fatalf("plain-text output = %q, want it to report 2 products / 5 articles", out)
	}
}

func TestWhoami_JSONReportsProductsAndArticles(t *testing.T) {
	c, stop := newWhoamiClient(t)
	defer stop()
	rt := runtime{client: c, json: true}
	ctx := ctxWithRuntime(context.Background(), rt)
	cmd := newWhoamiCmd()
	cmd.SetContext(ctx)

	out := captureStdout(t, func() {
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Fatalf("whoami --json: %v", err)
		}
	})
	var got struct {
		Products int    `json:"products"`
		Articles int    `json:"articles"`
		Total    string `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal whoami --json output %q: %v", out, err)
	}
	if got.Products != 2 || got.Articles != 5 {
		t.Fatalf("whoami --json = %+v, want products=2 articles=5", got)
	}
}
