package api

import (
	"context"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// FavoriteItem is one product starred as a favorite ("Preferits").
type FavoriteItem struct {
	ProductID string `json:"productId"`
	Name      string `json:"name,omitempty"`
	Brand     string `json:"brand,omitempty"`
	Price     *Money `json:"price,omitempty"`
}

// GetFavorites lists the account's favorited products ("Preferits" tab).
// The regulars/favorites page only fully decorates products within its
// `limit` budget (shared with the "regular" group on the same call), so past
// that this always re-enriches every id via a single batched GetProducts
// call rather than relying on the page's partial decoration.
func GetFavorites(ctx context.Context, c *client.Client) ([]FavoriteItem, error) {
	resp, err := getProductGroupsPage(ctx, c, 50)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, g := range resp.ProductGroups {
		if g.Type != "favorite" {
			continue
		}
		for _, p := range g.Products {
			if p.ProductID != "" {
				ids = append(ids, p.ProductID)
			}
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	prods, err := GetProducts(ctx, c, ids)
	if err != nil {
		return nil, err
	}
	out := make([]FavoriteItem, 0, len(prods))
	for _, p := range prods {
		out = append(out, FavoriteItem{ProductID: p.ProductID, Name: p.Name, Brand: p.Brand, Price: p.Price})
	}
	return out, nil
}
