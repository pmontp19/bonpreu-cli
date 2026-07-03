package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestDoJSONInjectsHeadersAndRoundTrips(t *testing.T) {
	var gotHeaders http.Header
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		if len(b) > 0 {
			_ = json.Unmarshal(b, &gotBody)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-csrf-token", "rotated")
		_, _ = w.Write([]byte(`{"ok":true,"echo":42}`))
	}))
	defer srv.Close()

	sess := &config.Session{
		Cookies:                  map[string]string{"VISITORID": "v"},
		CSRFToken:                "csrf-1",
		ClientRouteID:            "route-1",
		PageViewID:               "pv-1",
		EcomRequestSource:        "web",
		EcomRequestSourceVersion: "2.0.0-x",
	}
	c, err := New(sess, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c.BaseURL = srv.URL
	c.SeedCookies(sess.Cookies)

	var out struct {
		Ok   bool `json:"ok"`
		Echo int  `json:"echo"`
	}
	if err := c.DoJSON(context.Background(), http.MethodPost, "/api/test",
		map[string]int{"n": 1}, &out); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if !out.Ok || out.Echo != 42 {
		t.Fatalf("unexpected response: %+v", out)
	}
	checks := map[string]string{
		"x-csrf-token":                "csrf-1",
		"client-route-id":             "route-1",
		"page-view-id":                "pv-1",
		"ecom-request-source":         "web",
		"ecom-request-source-version": "2.0.0-x",
		"content-type":                "application/json; charset=utf-8",
	}
	for h, want := range checks {
		if got := gotHeaders.Get(h); got != want {
			t.Errorf("header %s = %q, want %q", h, got, want)
		}
	}
	if gotHeaders.Get("Cookie") == "" {
		t.Error("expected Cookie header from jar")
	}
	if gotBody["n"] != float64(1) {
		t.Errorf("body not forwarded: %+v", gotBody)
	}
	if sess.CSRFToken != "rotated" {
		t.Errorf("expected csrf rotated to %q, got %q", "rotated", sess.CSRFToken)
	}
}

func TestDoJSONErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"err":"blocked"}`))
	}))
	defer srv.Close()
	c, _ := New(&config.Session{}, nil)
	c.BaseURL = srv.URL
	err := c.DoJSON(context.Background(), http.MethodGet, "/x", nil, nil)
	he, ok := err.(*HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T %v", err, err)
	}
	if he.Status != http.StatusForbidden {
		t.Errorf("status = %d, want 403", he.Status)
	}
}

func TestHTTPErrorExpiryMessaging(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		he := &HTTPError{Status: status, URL: "/api/order/v6/orders", Body: `{"code":"UNAUTHORIZED"}`}
		if !he.Expired() {
			t.Errorf("status %d should be Expired()", status)
		}
		msg := he.Error()
		if !strings.Contains(msg, "import-har") || !strings.Contains(msg, "session expired") {
			t.Errorf("status %d message = %q, want re-import instruction", status, msg)
		}
		if strings.Contains(msg, "UNAUTHORIZED") {
			t.Errorf("status %d message should not leak raw body: %q", status, msg)
		}
	}
	// A non-auth error keeps the raw body for debugging.
	other := &HTTPError{Status: http.StatusBadRequest, URL: "/x", Body: "boom"}
	if other.Expired() {
		t.Error("400 should not be Expired()")
	}
	if !strings.Contains(other.Error(), "boom") {
		t.Errorf("400 message should include body, got %q", other.Error())
	}
}

func TestDoRaw(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("nope"))
			return
		}
		_, _ = w.Write([]byte("<html>hi</html>"))
	}))
	defer srv.Close()
	c, _ := New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	data, err := c.DoRaw(context.Background(), http.MethodGet, "/page")
	if err != nil {
		t.Fatalf("DoRaw: %v", err)
	}
	if string(data) != "<html>hi</html>" {
		t.Errorf("body = %q", data)
	}

	if _, err := c.DoRaw(context.Background(), http.MethodGet, "/bad"); err == nil {
		t.Fatal("expected error on 401")
	} else if he, ok := err.(*HTTPError); !ok || !he.Expired() {
		t.Errorf("expected expired HTTPError, got %v", err)
	}
}

// TestDoJSONRefreshesStaleCSRFAndRetries exercises the common failure mode:
// a rotated server-side CSRF token makes a mutating POST 403 while the session
// itself is live. The client should fetch `/`, adopt the fresh token from
// __INITIAL_STATE__, and retry once — succeeding without a HAR re-import.
func TestDoJSONRefreshesStaleCSRFAndRetries(t *testing.T) {
	const freshToken = "fresh-csrf-token"
	var postAttempts int
	var lastPostToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			// Homepage carries the live token in the SSR state blob.
			_, _ = w.Write([]byte(`<html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"` + freshToken + `"}}};</script></html>`))
			return
		}
		// The mutating endpoint: reject the stale token, accept the fresh one.
		postAttempts++
		lastPostToken = r.Header.Get("x-csrf-token")
		if lastPostToken != freshToken {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	sess := &config.Session{CSRFToken: "stale-csrf"}
	c, _ := New(sess, nil)
	c.BaseURL = srv.URL

	var out struct {
		Ok bool `json:"ok"`
	}
	if err := c.DoJSON(context.Background(), http.MethodPost, "/api/cart", map[string]int{"q": 1}, &out); err != nil {
		t.Fatalf("DoJSON should succeed after CSRF refresh: %v", err)
	}
	if !out.Ok {
		t.Fatal("expected ok:true after retry")
	}
	if postAttempts != 2 {
		t.Errorf("expected exactly 2 POST attempts (403 then retry), got %d", postAttempts)
	}
	if lastPostToken != freshToken {
		t.Errorf("retry used token %q, want %q", lastPostToken, freshToken)
	}
	if sess.CSRFToken != freshToken {
		t.Errorf("session token = %q, want refreshed %q", sess.CSRFToken, freshToken)
	}
	if !c.SyncSession() {
		t.Error("refreshed CSRF should mark the session dirty for persistence")
	}
}

// TestDoJSONForbiddenSurvivesWhenTokenAlreadyFresh verifies we do not loop or
// mask a genuine 403: if the homepage returns the same token we already hold,
// the original 403 is surfaced (→ "re-import HAR").
func TestDoJSONForbiddenSurvivesWhenTokenAlreadyFresh(t *testing.T) {
	const token = "current-token"
	var postAttempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"` + token + `"}}};</script></html>`))
			return
		}
		postAttempts++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	sess := &config.Session{CSRFToken: token}
	c, _ := New(sess, nil)
	c.BaseURL = srv.URL

	err := c.DoJSON(context.Background(), http.MethodPost, "/api/cart", map[string]int{"q": 1}, nil)
	he, ok := err.(*HTTPError)
	if !ok || !he.Expired() {
		t.Fatalf("expected expired HTTPError, got %v", err)
	}
	if postAttempts != 1 {
		t.Errorf("expected no retry when token unchanged, got %d attempts", postAttempts)
	}
}

func TestSyncSessionFoldsCookiesAndCSRF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-csrf-token", "rotated-2")
		http.SetCookie(w, &http.Cookie{Name: "global_sid", Value: "fresh"})
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	sess := &config.Session{Cookies: map[string]string{"global_sid": "old"}, CSRFToken: "old-csrf"}
	c, _ := New(sess, nil)
	c.BaseURL = srv.URL

	if err := c.DoJSON(context.Background(), http.MethodGet, "/x", nil, nil); err != nil {
		t.Fatalf("DoJSON: %v", err)
	}
	if !c.SyncSession() {
		t.Fatal("SyncSession should report a change")
	}
	if sess.CSRFToken != "rotated-2" {
		t.Errorf("csrf = %q, want rotated-2", sess.CSRFToken)
	}
	if sess.Cookies["global_sid"] != "fresh" {
		t.Errorf("cookie = %q, want fresh", sess.Cookies["global_sid"])
	}
	// A second sync with no further changes reports false.
	if c.SyncSession() {
		t.Error("SyncSession with no changes should report false")
	}
}
