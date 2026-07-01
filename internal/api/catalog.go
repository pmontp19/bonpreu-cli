package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

type Product struct {
	ProductID         string `json:"productId"`
	RetailerProductID string `json:"retailerProductId"`
	Name              string `json:"name"`
	Brand             string `json:"brand,omitempty"`
	Unit              string `json:"unit,omitempty"`
	Price             *Money `json:"price,omitempty"`
}

type productPageResponse struct {
	ProductGroups []struct {
		DecoratedProducts []Product `json:"decoratedProducts"`
	} `json:"productGroups"`
}

func SearchProducts(ctx context.Context, c *client.Client, query string, limit int) ([]Product, error) {
	if limit <= 0 {
		limit = 30
	}
	path := fmt.Sprintf(
		"/api/webproductpagews/v6/product-pages/search?includeAdditionalPageInfo=true&maxPageSize=300&maxProductsToDecorate=%d&q=%s&tag=web",
		limit, url.QueryEscape(query),
	)
	var resp productPageResponse
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	var out []Product
	for _, g := range resp.ProductGroups {
		out = append(out, g.DecoratedProducts...)
	}
	return out, nil
}

type Category struct {
	CategoryID         string     `json:"categoryId"`
	RetailerCategoryID string     `json:"retailerCategoryId"`
	Name               string     `json:"name"`
	ProductCount       int        `json:"productCount"`
	ChildCategories    []Category `json:"childCategories"`
}

func GetCategories(ctx context.Context, c *client.Client, depth int) ([]Category, error) {
	if depth <= 0 {
		depth = 4
	}
	path := fmt.Sprintf("/api/webproductpagews/v1/categories?decoration=false&categoryDepth=%d", depth)
	var out []Category
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func GetRelated(ctx context.Context, c *client.Client, retailerID string) ([]string, error) {
	path := "/api/webproductpagews/v5/products/related?retailerProductId=" + url.QueryEscape(retailerID)
	var out []string
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func GetProducts(ctx context.Context, c *client.Client, uuids []string) ([]Product, error) {
	if len(uuids) == 0 {
		return nil, nil
	}
	var raw json.RawMessage
	if err := c.DoJSON(ctx, http.MethodPut, "/api/webproductpagews/v6/products", uuids, &raw); err != nil {
		return nil, err
	}
	if prods, ok := parseProducts(raw); ok {
		if len(prods) == 0 {
			return nil, nil
		}
		return prods, nil
	}
	if isEmptyJSON(raw) {
		return nil, nil
	}
	return nil, fmt.Errorf("could not parse products response: %s", truncateRaw(raw))
}

func isEmptyJSON(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed == "" || trimmed == "[]" || trimmed == "{}" || trimmed == "null"
}

func truncateRaw(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// parseProducts recognizes the three known top-level shapes for a products
// response and returns ok=true even when the recognized shape is legitimately
// empty (e.g. {"products":[]}) — only a shape with none of these keys is
// treated as unparseable.
func parseProducts(raw json.RawMessage) ([]Product, bool) {
	var direct []Product
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, true
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false
	}
	if v, ok := obj["products"]; ok {
		var prods []Product
		if err := json.Unmarshal(v, &prods); err == nil {
			return prods, true
		}
	}
	if v, ok := obj["productGroups"]; ok {
		var groups []struct {
			DecoratedProducts []Product `json:"decoratedProducts"`
		}
		if err := json.Unmarshal(v, &groups); err == nil {
			var out []Product
			for _, g := range groups {
				out = append(out, g.DecoratedProducts...)
			}
			return out, true
		}
	}
	return nil, false
}

// IsUUID reports whether id is a canonical UUID; the check itself lives in
// internal/client since client.ParseSession needs it too.
func IsUUID(id string) bool {
	return client.IsUUID(id)
}

func ResolveProductID(ctx context.Context, c *client.Client, id string, cache *config.IDCache) (string, error) {
	if IsUUID(id) {
		return id, nil
	}
	if cache != nil {
		if pid, ok := cache.RetailerToProduct[id]; ok {
			return pid, nil
		}
	}
	pid, err := scrapeProductID(ctx, c, id)
	if err != nil {
		return "", fmt.Errorf("resolve %q: not in cache and page scrape failed — run `search` first or pass the product uuid: %w", id, err)
	}
	if cache != nil {
		if cache.RetailerToProduct == nil {
			cache.RetailerToProduct = map[string]string{}
		}
		cache.RetailerToProduct[id] = pid
		_ = config.SaveCache(cache)
	}
	return pid, nil
}

func scrapeProductID(ctx context.Context, c *client.Client, retailerID string) (string, error) {
	b, err := c.DoRaw(ctx, http.MethodGet, "/products/_/"+url.PathEscape(retailerID))
	if err != nil {
		return "", err
	}
	js, ok := client.ExtractInitialState(string(b))
	if !ok {
		return "", fmt.Errorf("no __QUERY_INITIAL_STATE__ (or __INITIAL_STATE__) on product page (slug may be required)")
	}
	var raw any
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return "", fmt.Errorf("parse state: %w", err)
	}
	if pid := findProductID(raw, retailerID); pid != "" {
		return pid, nil
	}
	return "", fmt.Errorf("retailerProductId %s not present in page state", retailerID)
}

func findProductID(node any, retailerID string) string {
	switch v := node.(type) {
	case map[string]any:
		if rp, _ := v["retailerProductId"].(string); rp == retailerID {
			if pid, _ := v["productId"].(string); pid != "" {
				return pid
			}
		}
		for _, child := range v {
			if pid := findProductID(child, retailerID); pid != "" {
				return pid
			}
		}
	case []any:
		for _, child := range v {
			if pid := findProductID(child, retailerID); pid != "" {
				return pid
			}
		}
	}
	return ""
}
