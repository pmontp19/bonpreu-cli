package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

type Money struct {
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
}

type ItemTotals struct {
	RegularPrice *Money `json:"regularPrice,omitempty"`
	FinalPrice   *Money `json:"finalPrice,omitempty"`
}

type CartItem struct {
	ProductID          string      `json:"productId"`
	Quantity           int         `json:"quantity"`
	Name               string      `json:"name,omitempty"`
	Price              *Money      `json:"price,omitempty"`
	TotalPrices        *ItemTotals `json:"totalPrices,omitempty"`
	MaxQuantityReached bool        `json:"maxQuantityReached,omitempty"`
}

type CartItemGroup struct {
	Items []CartItem `json:"items,omitempty"`
}

type Totals struct {
	ItemPriceAfterPromos *Money `json:"itemPriceAfterPromos,omitempty"`
}

type Cart struct {
	CartID   string     `json:"cartId"`
	Items    []CartItem `json:"items,omitempty"`
	Totals   Totals     `json:"totals"`
	Checkout struct {
		BasketAboveThreshold bool `json:"basketAboveThreshold"`
	} `json:"checkout"`
	BasketUpdateResult struct {
		Checkout struct {
			BasketAboveThreshold bool `json:"basketAboveThreshold"`
		} `json:"checkout"`
		ItemGroups []CartItemGroup `json:"itemGroups"`
	} `json:"basketUpdateResult"`
}

func (c *Cart) Lines() []CartItem {
	seen := map[string]bool{}
	var out []CartItem
	add := func(it CartItem) {
		if it.ProductID == "" || seen[it.ProductID] {
			return
		}
		seen[it.ProductID] = true
		out = append(out, it)
	}
	for _, it := range c.Items {
		add(it)
	}
	for _, g := range c.BasketUpdateResult.ItemGroups {
		for _, it := range g.Items {
			add(it)
		}
	}
	return out
}

func (c *Cart) QtyOf(productID string) int {
	for _, it := range c.Lines() {
		if it.ProductID == productID {
			return it.Quantity
		}
	}
	return 0
}

// TotalUnits sums line quantities — the site's "articles" count. It differs
// from len(Lines()), which counts distinct products.
func (c *Cart) TotalUnits() int {
	n := 0
	for _, it := range c.Lines() {
		n += it.Quantity
	}
	return n
}

func (c *Cart) TotalAmount() string {
	if c.Totals.ItemPriceAfterPromos != nil && c.Totals.ItemPriceAfterPromos.Amount != "" {
		return c.Totals.ItemPriceAfterPromos.Amount
	}
	sum := 0.0
	for _, it := range c.Lines() {
		if it.TotalPrices != nil && it.TotalPrices.FinalPrice != nil {
			if v, err := strconv.ParseFloat(it.TotalPrices.FinalPrice.Amount, 64); err == nil {
				sum += v
			}
		}
	}
	if sum > 0 {
		return strconv.FormatFloat(sum, 'f', 2, 64)
	}
	return ""
}

func GetActiveCart(ctx context.Context, c *client.Client) (*Cart, error) {
	var cart Cart
	if err := c.DoJSON(ctx, http.MethodGet, "/api/cart/v1/carts/active", nil, &cart); err != nil {
		return nil, err
	}
	return &cart, nil
}

type CartItemInput struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

func ApplyQuantity(ctx context.Context, c *client.Client, items []CartItemInput) (*Cart, error) {
	var cart Cart
	if err := c.DoJSON(ctx, http.MethodPost,
		"/api/cart/v1/carts/active/apply-quantity?cartProductSorting=CATEGORIES", items, &cart); err != nil {
		return nil, err
	}
	return &cart, nil
}
