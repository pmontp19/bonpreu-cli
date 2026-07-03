package client

import (
	"strings"
	"testing"
)

// A representative Chrome "Copy as cURL (bash)" paste: multi-line with
// backslash continuations, single-quoted -H headers, cookie + csrf present.
const chromeCurl = `curl 'https://www.compraonline.bonpreuesclat.cat/api/cart/v1/carts/active?cartProductSorting=CATEGORIES' \
  -H 'accept: application/json; charset=utf-8' \
  -H 'accept-language: ca,es;q=0.9' \
  -H 'cookie: VISITORID=abc123; global_sid=SESSIONVALUE; language=ca' \
  -H 'ecom-request-source: web' \
  -H 'ecom-request-source-version: 2.0.0-20260701-deadbeef' \
  -H 'x-csrf-token: 420f5351-9ef2-43b5-8c94-ee46a151c61f' \
  -H 'client-route-id: route-uuid' \
  -H 'page-view-id: page-uuid' \
  --compressed`

func TestParseCurl_Chrome(t *testing.T) {
	s, err := ParseCurl(strings.NewReader(chromeCurl))
	if err != nil {
		t.Fatalf("ParseCurl: %v", err)
	}
	if s.Cookies["VISITORID"] != "abc123" || s.Cookies["global_sid"] != "SESSIONVALUE" || s.Cookies["language"] != "ca" {
		t.Errorf("cookies = %+v", s.Cookies)
	}
	if s.CSRFToken != "420f5351-9ef2-43b5-8c94-ee46a151c61f" {
		t.Errorf("csrf = %q", s.CSRFToken)
	}
	if s.EcomRequestSource != "web" || s.EcomRequestSourceVersion != "2.0.0-20260701-deadbeef" {
		t.Errorf("source = %q / %q", s.EcomRequestSource, s.EcomRequestSourceVersion)
	}
	if s.ClientRouteID != "route-uuid" || s.PageViewID != "page-uuid" {
		t.Errorf("ids = %q / %q", s.ClientRouteID, s.PageViewID)
	}
}

// The -b/--cookie flag form (curl's dedicated cookie flag) and a URL that
// carries a delivery-destination UUID.
func TestParseCurl_CookieFlagAndDestFromURL(t *testing.T) {
	const dest = "722f7dc5-4f88-4df0-a8cb-7b6a7d9887ad"
	cmd := `curl "https://www.compraonline.bonpreuesclat.cat/api/ecomdeliverydestinations/v4/delivery-addresses/` + dest + `" ` +
		`-b "VISITORID=v; global_sid=g" ` +
		`-H "x-csrf-token: tok"`
	s, err := ParseCurl(strings.NewReader(cmd))
	if err != nil {
		t.Fatalf("ParseCurl: %v", err)
	}
	if s.Cookies["global_sid"] != "g" {
		t.Errorf("cookies = %+v", s.Cookies)
	}
	if s.DeliveryDestinationID != dest {
		t.Errorf("dest = %q, want %q", s.DeliveryDestinationID, dest)
	}
}

func TestParseCurl_NoCookiesErrors(t *testing.T) {
	_, err := ParseCurl(strings.NewReader(`curl 'https://x/api' -H 'accept: application/json'`))
	if err == nil {
		t.Fatal("expected error when no cookies present")
	}
}

func TestTokenizeCurl_QuotesAndContinuations(t *testing.T) {
	got := tokenizeCurl("curl 'a b' \\\n  -H \"c: d\"")
	want := []string{"curl", "a b", "-H", "c: d"}
	if len(got) != len(want) {
		t.Fatalf("tokens = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
