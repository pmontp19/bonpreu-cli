package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

type harFile struct {
	Log struct {
		Entries []harEntry `json:"entries"`
	} `json:"log"`
}

type harEntry struct {
	Request struct {
		Method  string   `json:"method"`
		URL     string   `json:"url"`
		Headers []harNVP `json:"headers"`
	} `json:"request"`
	Response struct {
		Status  int      `json:"status"`
		Headers []harNVP `json:"headers"`
		Content struct {
			MimeType string `json:"mimeType"`
			Text     string `json:"text"`
		} `json:"content"`
	} `json:"response"`
}

type harNVP struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type initialState struct {
	Session struct {
		CSRF struct {
			Token string `json:"token"`
		} `json:"csrf"`
	} `json:"session"`
}

const bonpreuHost = "compraonline.bonpreuesclat.cat"

func ParseSession(r io.Reader) (*config.Session, error) {
	var har harFile
	if err := json.NewDecoder(r).Decode(&har); err != nil {
		return nil, fmt.Errorf("parse har json: %w", err)
	}
	s := &config.Session{Cookies: map[string]string{}, EcomRequestSource: config.SourceWeb}
	var addressJSON []byte

	for _, e := range har.Log.Entries {
		u, err := url.Parse(e.Request.URL)
		if err != nil {
			continue
		}
		isBP := strings.Contains(u.Host, bonpreuHost)
		if isBP {
			for _, h := range e.Request.Headers {
				switch strings.ToLower(h.Name) {
				case "cookie":
					mergeCookieHeader(s.Cookies, h.Value)
				case "ecom-request-source-version":
					if s.EcomRequestSourceVersion == "" {
						s.EcomRequestSourceVersion = h.Value
					}
				case "client-route-id":
					if s.ClientRouteID == "" && h.Value != "" && h.Value != "undefined" {
						s.ClientRouteID = h.Value
					}
				case "page-view-id":
					if s.PageViewID == "" && h.Value != "" && h.Value != "undefined" {
						s.PageViewID = h.Value
					}
				}
			}
		}
		for _, h := range e.Response.Headers {
			switch strings.ToLower(h.Name) {
			case "ecom-request-source-version":
				if s.EcomRequestSourceVersion == "" {
					s.EcomRequestSourceVersion = h.Value
				}
			case "set-cookie":
				if isBP {
					mergeSetCookie(s.Cookies, h.Value)
				}
			}
		}
		if isHomepage(u) && strings.Contains(e.Response.Content.MimeType, "html") {
			if js, ok := extractInitialState(e.Response.Content.Text); ok {
				var st initialState
				if json.Unmarshal([]byte(js), &st) == nil && st.Session.CSRF.Token != "" {
					s.CSRFToken = st.Session.CSRF.Token
				}
			}
		}
		if strings.Contains(u.Path, "delivery-addresses") {
			// Only a detail URL (.../delivery-addresses/<uuid>) carries the
			// destination id; the list URL (.../delivery-addresses?...) does
			// not, so guard against storing the literal path segment.
			if s.DeliveryDestinationID == "" {
				if seg := lastPathSegment(u.Path); isUUID(seg) {
					s.DeliveryDestinationID = seg
				}
			}
			if e.Response.Content.Text != "" {
				addressJSON = []byte(e.Response.Content.Text)
			}
		}
	}

	if s.CSRFToken == "" {
		return nil, fmt.Errorf("csrf token not found in HAR (no homepage entry with __INITIAL_STATE__.session.csrf.token)")
	}
	if len(s.Cookies) == 0 {
		return nil, fmt.Errorf("no session cookies found in HAR")
	}
	if addressJSON != nil {
		applyAddress(s, addressJSON)
	}
	return s, nil
}

func applyAddress(s *config.Session, b []byte) {
	setFrom := func(m map[string]any) {
		if id, ok := m["resolvedRegionId"].(string); ok && s.RegionID == "" {
			s.RegionID = id
		}
		if id, ok := m["deliveryDestinationId"].(string); ok && s.DeliveryDestinationID == "" {
			s.DeliveryDestinationID = id
		}
	}
	var single map[string]any
	if json.Unmarshal(b, &single) == nil && single != nil {
		setFrom(single)
		return
	}
	var arr []map[string]any
	if json.Unmarshal(b, &arr) == nil && len(arr) > 0 {
		// Prefer the primary address; fall back to the first entry. Order in
		// the response is not guaranteed, so never let a non-primary entry win.
		best := arr[0]
		for _, m := range arr {
			if primary, _ := m["isPrimary"].(bool); primary {
				best = m
				break
			}
		}
		setFrom(best)
	}
}

func mergeCookieHeader(cookies map[string]string, header string) {
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		cookies[k] = strings.TrimSpace(v)
	}
}

func mergeSetCookie(cookies map[string]string, header string) {
	parts := strings.Split(header, ";")
	if len(parts) == 0 {
		return
	}
	k, v, ok := strings.Cut(strings.TrimSpace(parts[0]), "=")
	if !ok {
		return
	}
	k = strings.TrimSpace(k)
	if k == "" || v == "" {
		return
	}
	cookies[k] = strings.TrimSpace(v)
}

func isHomepage(u *url.URL) bool {
	return u.Host != "" && strings.Contains(u.Host, bonpreuHost) && (u.Path == "" || u.Path == "/")
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func lastPathSegment(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func ExtractInitialState(html string) (string, bool) {
	return extractInitialState(html)
}

func extractInitialState(html string) (string, bool) {
	marker := "window.__INITIAL_STATE__="
	idx := strings.Index(html, marker)
	if idx < 0 {
		return "", false
	}
	rel := strings.IndexByte(html[idx:], '{')
	if rel < 0 {
		return "", false
	}
	start := idx + rel
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(html); i++ {
		c := html[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return html[start : i+1], true
			}
		}
	}
	return "", false
}
