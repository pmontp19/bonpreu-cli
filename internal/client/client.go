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
	req, err := c.NewRequest(ctx, method, urlPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	c.captureCSRF(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{Status: resp.StatusCode, URL: urlPath, Body: truncate(data, 500)}
	}
	return data, nil
}

func (c *Client) DoJSON(ctx context.Context, method, urlPath string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := c.NewRequest(ctx, method, urlPath, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	c.captureCSRF(resp)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return &HTTPError{Status: resp.StatusCode, URL: urlPath, Body: truncate(data, 500)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
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

func (e *HTTPError) Error() string {
	return fmt.Sprintf("bonpreu %s: HTTP %d: %s", e.URL, e.Status, e.Body)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
