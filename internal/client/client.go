package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"

	"github.com/pmontp19/bonpreu-cli/internal/config"
)

const (
	BaseURL   = "https://www.compraonline.bonpreuesclat.cat"
	userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"

	// maxRawBodyBytes caps DoRaw reads (product page HTML) so a pathological
	// or compromised response can't be unmarshaled/walked as unbounded data.
	maxRawBodyBytes = 10 << 20 // 10MB
)

type Client struct {
	HTTP    *http.Client
	BaseURL string
	Sess    *config.Session
	Log     *log.Logger
	dirty   bool
}

func New(sess *config.Session, logger *log.Logger) (*Client, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		HTTP:    &http.Client{Jar: jar, Timeout: 30 * time.Second},
		BaseURL: BaseURL,
		Sess:    sess,
		Log:     logger,
	}
	if sess != nil {
		c.SeedCookies(sess.Cookies)
	}
	return c, nil
}

func (c *Client) SeedCookies(cookies map[string]string) {
	if cookies == nil {
		return
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return
	}
	var httpCookies []*http.Cookie
	for k, v := range cookies {
		httpCookies = append(httpCookies, &http.Cookie{Name: k, Value: v})
	}
	c.HTTP.Jar.SetCookies(u, httpCookies)
}

func (c *Client) NewRequest(ctx context.Context, method, urlPath string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+urlPath, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json; charset=utf-8")
	req.Header.Set("Accept-Language", "ca,es;q=0.9,en;q=0.8")
	if c.Sess != nil {
		if c.Sess.EcomRequestSource != "" {
			req.Header.Set("ecom-request-source", c.Sess.EcomRequestSource)
		}
		if c.Sess.EcomRequestSourceVersion != "" {
			req.Header.Set("ecom-request-source-version", c.Sess.EcomRequestSourceVersion)
		}
		if c.Sess.CSRFToken != "" {
			req.Header.Set("x-csrf-token", c.Sess.CSRFToken)
		}
		if c.Sess.ClientRouteID != "" {
			req.Header.Set("client-route-id", c.Sess.ClientRouteID)
		}
		if c.Sess.PageViewID != "" {
			req.Header.Set("page-view-id", c.Sess.PageViewID)
		}
	}
	return req, nil
}

func (c *Client) DoRaw(ctx context.Context, method, urlPath string) ([]byte, error) {
	data, status, err := c.doWithCSRFRetry(ctx, method, urlPath, nil, false)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, &HTTPError{Status: status, URL: urlPath, Body: truncate(data, 500)}
	}
	return data, nil
}

func (c *Client) DoJSON(ctx context.Context, method, urlPath string, in any, out any) error {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		body = b
	}
	data, status, err := c.doWithCSRFRetry(ctx, method, urlPath, body, in != nil)
	if err != nil {
		return err
	}
	if status >= 400 {
		return &HTTPError{Status: status, URL: urlPath, Body: truncate(data, 500)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// do performs a single request and returns the response body and status.
// Only transport-level failures produce a non-nil error; HTTP error statuses
// are returned so callers (and the CSRF-retry wrapper) can interpret them.
func (c *Client) do(ctx context.Context, method, urlPath string, body []byte, jsonBody bool) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := c.NewRequest(ctx, method, urlPath, r)
	if err != nil {
		return nil, 0, err
	}
	if jsonBody {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	c.captureCSRF(resp)
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRawBodyBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if len(data) > maxRawBodyBytes {
		return nil, resp.StatusCode, fmt.Errorf("response body exceeds %d bytes", maxRawBodyBytes)
	}
	return data, resp.StatusCode, nil
}

// doWithCSRFRetry runs the request once; on a 403 (typically a stale, rotated
// CSRF token — GETs still pass but state-changing POSTs are rejected) it
// refreshes the token from the homepage and retries exactly once. If the
// refresh yields no new token, the original 403 stands so the caller can
// surface the "re-import HAR" message for a genuinely dead session.
func (c *Client) doWithCSRFRetry(ctx context.Context, method, urlPath string, body []byte, jsonBody bool) ([]byte, int, error) {
	data, status, err := c.do(ctx, method, urlPath, body, jsonBody)
	if err != nil || status != http.StatusForbidden {
		return data, status, err
	}
	changed, rerr := c.RefreshCSRF(ctx)
	if rerr != nil {
		c.Log.Printf("csrf refresh failed after 403: %v", rerr)
		return data, status, nil
	}
	if !changed {
		return data, status, nil
	}
	c.Log.Printf("csrf token was stale; refreshed from homepage and retrying %s", urlPath)
	return c.do(ctx, method, urlPath, body, jsonBody)
}

// RefreshCSRF re-fetches the homepage and adopts a rotated CSRF token from
// window.__INITIAL_STATE__.session.csrf.token. It uses do() directly (never
// the retry wrapper) so a 403 here can't recurse. Returns true if the token
// changed. A nil error with changed=false means the token was already current.
// It is also used by the import flows to fill in a CSRF token that was not
// present in the imported request.
func (c *Client) RefreshCSRF(ctx context.Context) (bool, error) {
	if c.Sess == nil {
		return false, nil
	}
	data, status, err := c.do(ctx, http.MethodGet, "/", nil, false)
	if err != nil {
		return false, err
	}
	if status >= 400 {
		return false, &HTTPError{Status: status, URL: "/", Body: truncate(data, 500)}
	}
	js, ok := extractInitialState(string(data))
	if !ok {
		return false, fmt.Errorf("no __INITIAL_STATE__ on homepage")
	}
	var st initialState
	if err := json.Unmarshal([]byte(js), &st); err != nil {
		return false, fmt.Errorf("parse homepage state: %w", err)
	}
	tok := st.Session.CSRF.Token
	if tok == "" || tok == c.Sess.CSRFToken {
		return false, nil
	}
	c.Sess.CSRFToken = tok
	c.dirty = true
	return true, nil
}

// captureCSRF records a rotated CSRF token from a response and marks the
// session dirty so it can be persisted across invocations.
func (c *Client) captureCSRF(resp *http.Response) {
	if c.Sess == nil {
		return
	}
	if token := resp.Header.Get("x-csrf-token"); token != "" && token != c.Sess.CSRFToken {
		c.Sess.CSRFToken = token
		c.dirty = true
	}
}

// SyncSession folds the live cookie jar and any rotated CSRF token back into
// the Session. It returns true if the Session changed and should be persisted.
func (c *Client) SyncSession() bool {
	if c.Sess == nil {
		return false
	}
	changed := c.dirty
	u, err := url.Parse(c.BaseURL)
	if err == nil && c.HTTP != nil && c.HTTP.Jar != nil {
		if c.Sess.Cookies == nil {
			c.Sess.Cookies = map[string]string{}
		}
		for _, ck := range c.HTTP.Jar.Cookies(u) {
			if c.Sess.Cookies[ck.Name] != ck.Value {
				c.Sess.Cookies[ck.Name] = ck.Value
				changed = true
			}
		}
	}
	c.dirty = false
	return changed
}

type HTTPError struct {
	Status int
	URL    string
	Body   string
}

// Expired reports whether the status indicates a stale or rejected session
// (401 unauthorized) or a WAF/authorization block (403). Both are recovered
// by re-importing a fresh HAR.
func (e *HTTPError) Expired() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

func (e *HTTPError) Error() string {
	if e.Expired() {
		// The raw upstream body for auth failures is opaque WAF/JSON noise;
		// surface an actionable instruction instead. The full body is still
		// available via Body for verbose diagnostics.
		return fmt.Sprintf("session expired or unauthorized (HTTP %d at %s) — re-run `bonpreu import-har --file <fresh.har>` to refresh your session",
			e.Status, e.URL)
	}
	return fmt.Sprintf("bonpreu %s: HTTP %d: %s", e.URL, e.Status, e.Body)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
