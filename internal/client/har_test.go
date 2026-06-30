package client

import (
	"os"
	"strings"
	"testing"
)

func TestParseSession_RealHAR(t *testing.T) {
	f, err := os.Open("../../testdata/login.har")
	if err != nil {
		t.Skipf("login.har not available: %v", err)
	}
	defer f.Close()

	s, err := ParseSession(f)
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if s.CSRFToken != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("csrf = %q, want the token from __INITIAL_STATE__", s.CSRFToken)
	}
	if s.EcomRequestSourceVersion != "2.0.0-2026-06-30-12h34m48s-35c69f8f" {
		t.Errorf("ecom-version = %q", s.EcomRequestSourceVersion)
	}
	if s.RegionID != "00000000-0000-0000-0000-000000000002" {
		t.Errorf("regionId = %q", s.RegionID)
	}
	if s.DeliveryDestinationID != "00000000-0000-0000-0000-000000000003" {
		t.Errorf("deliveryDestinationId = %q", s.DeliveryDestinationID)
	}
	for _, k := range []string{"VISITORID", "global_sid", "AWSALB", "aws-waf-token"} {
		if s.Cookies[k] == "" {
			t.Errorf("missing session cookie %q", k)
		}
	}
	if s.EcomRequestSource != "web" {
		t.Errorf("source = %q, want web", s.EcomRequestSource)
	}
}

func TestExtractInitialState_BraceBalancing(t *testing.T) {
	html := `<script>x</script><script>window.__INITIAL_STATE__={"session":{"csrf":{"token":"abc"}},"a":{"b":"}"}};</script><script>z</script>`
	got, ok := extractInitialState(html)
	if !ok {
		t.Fatal("expected extraction")
	}
	if got != `{"session":{"csrf":{"token":"abc"}},"a":{"b":"}"}}` {
		t.Errorf("extracted = %q", got)
	}
}

func TestParseSession_MissingCSRFErrors(t *testing.T) {
	har := `{"log":{"entries":[{"request":{"url":"https://www.compraonline.bonpreuesclat.cat/api/x","headers":[]},"response":{"headers":[],"content":{}}}]}}`
	_, err := ParseSession(strings.NewReader(har))
	if err == nil {
		t.Fatal("expected error for missing csrf")
	}
}

func TestParseSession_ListURLDoesNotPolluteDestAndPrefersPrimary(t *testing.T) {
	har := `{"log":{"entries":[
	{"request":{"method":"GET","url":"https://www.compraonline.bonpreuesclat.cat/","headers":[{"name":"Cookie","value":"VISITORID=v1"}]},"response":{"status":200,"headers":[],"content":{"mimeType":"text/html","text":"<script>window.__INITIAL_STATE__={\"session\":{\"csrf\":{\"token\":\"tok-123\"}}}</script>"}}},
	{"request":{"method":"GET","url":"https://www.compraonline.bonpreuesclat.cat/api/ecomdeliverydestinations/v4/delivery-addresses?deliveryMethod=HOME_DELIVERY","headers":[]},"response":{"status":200,"headers":[],"content":{"mimeType":"application/json","text":"[{\"isPrimary\":false,\"resolvedRegionId\":\"region-A\",\"deliveryDestinationId\":\"dest-A\"},{\"isPrimary\":true,\"resolvedRegionId\":\"region-B\",\"deliveryDestinationId\":\"dest-B\"}]"}}}
	]}}`
	s, err := ParseSession(strings.NewReader(har))
	if err != nil {
		t.Fatalf("ParseSession: %v", err)
	}
	if s.DeliveryDestinationID == "delivery-addresses" {
		t.Fatal("list-endpoint path segment leaked into deliveryDestinationId")
	}
	if s.DeliveryDestinationID != "dest-B" || s.RegionID != "region-B" {
		t.Fatalf("expected primary address (region-B/dest-B), got region=%q dest=%q", s.RegionID, s.DeliveryDestinationID)
	}
}

func TestMergeCookieHeader(t *testing.T) {
	m := map[string]string{}
	mergeCookieHeader(m, "a=1; b=2; c=3")
	for _, kv := range [][2]string{{"a", "1"}, {"b", "2"}, {"c", "3"}} {
		if m[kv[0]] != kv[1] {
			t.Errorf("%s = %q, want %q", kv[0], m[kv[0]], kv[1])
		}
	}
}

func TestMergeSetCookie_IgnoresAttributes(t *testing.T) {
	m := map[string]string{}
	mergeSetCookie(m, "VISITORID=xyz; Max-Age=7884000; Path=/; HttpOnly")
	if m["VISITORID"] != "xyz" {
		t.Errorf("got %q", m["VISITORID"])
	}
}
