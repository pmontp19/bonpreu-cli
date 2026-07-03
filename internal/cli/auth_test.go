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

// newWhoamiClient serves the homepage (with an authenticated customerSession)
// and the active-cart endpoint, so whoami's account-auth check passes and it
// reports the cart summary. Pass authenticated=false to simulate an
// anonymous/expired account session (customerSession.data == null).
func newWhoamiClient(t *testing.T, authenticated bool) (*client.Client, func()) {
	t.Helper()
	loggedIn := "false"
	if authenticated {
		loggedIn = "true"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"t"},"isLoggedIn":` +
				loggedIn + `}}};</script></html>`))
			return
		}
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
	c, stop := newWhoamiClient(t, true)
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
	if !strings.Contains(out, "authenticated") {
		t.Fatalf("plain-text output = %q, want it to report authenticated status", out)
	}
}

// An anonymous/expired account session (guest cart works, customerSession null)
// must fail with the re-import instruction rather than a false "session OK".
func TestWhoami_AnonymousSessionFails(t *testing.T) {
	c, stop := newWhoamiClient(t, false)
	defer stop()
	rt := runtime{client: c, json: false}
	ctx := ctxWithRuntime(context.Background(), rt)
	cmd := newWhoamiCmd()
	cmd.SetContext(ctx)

	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("whoami should fail for an anonymous session, got nil error")
	}
	if !strings.Contains(err.Error(), "import-har") {
		t.Fatalf("error = %q, want it to instruct re-import-har", err)
	}
}

func TestWhoami_JSONReportsProductsAndArticles(t *testing.T) {
	c, stop := newWhoamiClient(t, true)
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
