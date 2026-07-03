package client

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

// ParseCurl builds a Session from a "Copy as cURL" command copied out of the
// browser devtools Network panel. Unlike a HAR export — which recent Chrome
// sanitizes, stripping the Cookie and x-csrf-token headers — "Copy as cURL"
// carries the request verbatim, so it is the reliable one-click way to capture
// an authenticated session.
//
// Only the cookie header is strictly required; the CSRF token, request-source
// version and route/page ids are captured when present (the caller can derive
// a missing CSRF token from the homepage). Region/destination are filled in
// when the copied request URL is a delivery-addresses detail URL, otherwise
// left empty for the caller to resolve.
func ParseCurl(r io.Reader) (*config.Session, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read curl input: %w", err)
	}
	tokens := tokenizeCurl(string(raw))
	s := &config.Session{Cookies: map[string]string{}, EcomRequestSource: config.SourceWeb}

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch {
		case t == "-H" || t == "--header":
			if i+1 < len(tokens) {
				i++
				applyCurlHeader(s, tokens[i])
			}
		case t == "-b" || t == "--cookie":
			if i+1 < len(tokens) {
				i++
				mergeCookieHeader(s.Cookies, tokens[i])
			}
		case strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://"):
			if u, err := url.Parse(t); err == nil && strings.Contains(u.Path, "delivery-addresses") {
				if seg := lastPathSegment(u.Path); IsUUID(seg) {
					s.DeliveryDestinationID = seg
				}
			}
		}
	}

	if len(s.Cookies) == 0 {
		return nil, fmt.Errorf("no cookies found in the curl command (copy an authenticated request via devtools → Copy → Copy as cURL)")
	}
	return s, nil
}

// applyCurlHeader maps a single "name: value" curl header onto the session.
func applyCurlHeader(s *config.Session, header string) {
	name, value, ok := strings.Cut(header, ":")
	if !ok {
		return
	}
	name = strings.ToLower(strings.TrimSpace(name))
	value = strings.TrimSpace(value)
	switch name {
	case "cookie":
		mergeCookieHeader(s.Cookies, value)
	case "x-csrf-token":
		if value != "" && value != "undefined" {
			s.CSRFToken = value
		}
	case "ecom-request-source":
		if value != "" {
			s.EcomRequestSource = value
		}
	case "ecom-request-source-version":
		s.EcomRequestSourceVersion = value
	case "client-route-id":
		if value != "" && value != "undefined" {
			s.ClientRouteID = value
		}
	case "page-view-id":
		if value != "" && value != "undefined" {
			s.PageViewID = value
		}
	}
}

// tokenizeCurl splits a curl command into shell-style tokens, honouring single
// quotes (Chrome/Firefox bash copies), double quotes, backslash escapes and
// backslash-newline line continuations. It is intentionally minimal — enough
// for a pasted "Copy as cURL", not a full shell parser.
func tokenizeCurl(s string) []string {
	var tokens []string
	var cur strings.Builder
	inTok := false
	var quote byte // 0, '\'' or '"'

	flush := func() {
		if inTok {
			tokens = append(tokens, cur.String())
			cur.Reset()
			inTok = false
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
				continue
			}
			// Inside double quotes a backslash escapes the next char; inside
			// single quotes it is literal (shell semantics).
			if quote == '"' && c == '\\' && i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
				continue
			}
			cur.WriteByte(c)
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
			inTok = true
		case '\\':
			// Line continuation: swallow the backslash and the newline.
			if i+1 < len(s) && (s[i+1] == '\n' || s[i+1] == '\r') {
				continue
			}
			if i+1 < len(s) {
				i++
				cur.WriteByte(s[i])
				inTok = true
			}
		case ' ', '\t', '\n', '\r':
			flush()
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	flush()
	return tokens
}
