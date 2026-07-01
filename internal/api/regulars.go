package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// RegularItem is one frequently-bought product, as shown on the "Productes
// recurrents" tab and used by InstantShop ("Compra ràpida") to auto-fill the
// cart from purchase history.
type RegularItem struct {
	ProductID string `json:"productId"`
	Name      string `json:"name,omitempty"`
	Brand     string `json:"brand,omitempty"`
	Price     *Money `json:"price,omitempty"`
	Quantity  int    `json:"quantity"`
	Frequency string `json:"frequency"`
}

// productGroupsResponse is the shared shape of
// /api/webproductpagews/v5/product-pages/regulars: a single call returns
// multiple productGroups distinguished by `type` — "regular" (frequently
// bought, backs InstantShop) and "favorite" (starred products, see
// GetFavorites). Only entries within the response's `limit` budget carry the
// nested `product` decoration; the rest are bare `{"productId":...}` and must
// be enriched separately (GetProducts).
type productGroupsResponse struct {
	ProductGroups []struct {
		Type     string `json:"type"`
		Products []struct {
			ProductID string `json:"productId"`
			Product   struct {
				ProductID string `json:"productId"`
				Name      string `json:"name"`
				Brand     string `json:"brand"`
				Price     *Money `json:"price"`
				Regular   struct {
					Quantity  int    `json:"quantity"`
					Frequency string `json:"frequency"`
				} `json:"regular"`
			} `json:"product"`
		} `json:"products"`
	} `json:"productGroups"`
}

// getProductGroupsPage fetches the shared regulars/favorites page. limit
// controls how many entries across all groups get full product decoration
// (see productGroupsResponse); entries beyond it come back as bare ids.
func getProductGroupsPage(ctx context.Context, c *client.Client, limit int) (*productGroupsResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	path := fmt.Sprintf("/api/webproductpagews/v5/product-pages/regulars?limit=%d&tag=web&tag=regulars", limit)
	var resp productGroupsResponse
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetRegulars lists the account's frequently-bought products, decorated with
// name/brand/price (unlike the bare /api/regulars/v1/regulars, which returns
// only productId/quantity/frequency).
func GetRegulars(ctx context.Context, c *client.Client) ([]RegularItem, error) {
	resp, err := getProductGroupsPage(ctx, c, 50)
	if err != nil {
		return nil, err
	}
	var out []RegularItem
	for _, g := range resp.ProductGroups {
		if g.Type != "regular" {
			continue
		}
		for _, p := range g.Products {
			out = append(out, RegularItem{
				ProductID: p.Product.ProductID,
				Name:      p.Product.Name,
				Brand:     p.Product.Brand,
				Price:     p.Product.Price,
				Quantity:  p.Product.Regular.Quantity,
				Frequency: p.Product.Regular.Frequency,
			})
		}
	}
	return out, nil
}

// InstantShopResult is the outcome of InstantShop: what got added, and the
// cart's new total.
type InstantShopResult struct {
	AddedProducts []CartItemInput `json:"addedProducts"`
	Total         string          `json:"total,omitempty"`
}

type instantShopResponse struct {
	AddedProducts      []CartItemInput `json:"addedProducts"`
	BasketUpdateResult struct {
		Totals Totals `json:"totals"`
	} `json:"basketUpdateResult"`
}

// InstantShop triggers "Compra ràpida": the server picks products from
// purchase history (regulars) and adds them to the active cart in one call.
// There is no way to preview what will be added before calling it.
func InstantShop(ctx context.Context, c *client.Client) (*InstantShopResult, error) {
	var resp instantShopResponse
	if err := c.DoJSON(ctx, http.MethodPost, "/api/cart/v2/instant-shop", struct{}{}, &resp); err != nil {
		return nil, err
	}
	res := &InstantShopResult{AddedProducts: resp.AddedProducts}
	if resp.BasketUpdateResult.Totals.ItemPriceAfterPromos != nil {
		res.Total = resp.BasketUpdateResult.Totals.ItemPriceAfterPromos.Amount
	}
	return res, nil
}
