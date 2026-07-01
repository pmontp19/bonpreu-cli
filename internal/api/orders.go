package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

func isJSONObject(raw json.RawMessage) bool {
	return strings.HasPrefix(strings.TrimSpace(string(raw)), "{")
}

// OrderSummary is one entry of the read-only order history.
type OrderSummary struct {
	OrderID      string `json:"orderId"`
	Status       string `json:"status,omitempty"`
	PlacedAt     string `json:"placedAt,omitempty"`
	DeliveryDate string `json:"deliveryDate,omitempty"`
	Total        *Money `json:"total,omitempty"`
}

// OrderLine is a denormalized order line: the product plus quantity and price.
type OrderLine struct {
	Product  Product `json:"product"`
	Quantity int     `json:"quantity"`
	Price    *Money  `json:"price,omitempty"`
}

// OrderDetail is a denormalized single order (see decorated endpoint).
type OrderDetail struct {
	OrderID string      `json:"orderId"`
	Status  string      `json:"status,omitempty"`
	Total   *Money      `json:"total,omitempty"`
	Lines   []OrderLine `json:"lines"`
}

// GetOrders lists prior orders (read-only). limit ≤ 0 returns all.
func GetOrders(ctx context.Context, c *client.Client, limit int) ([]OrderSummary, error) {
	var raw json.RawMessage
	if err := c.DoJSON(ctx, http.MethodGet, "/api/order/v6/orders", nil, &raw); err != nil {
		return nil, err
	}
	orders, ok := parseOrders(raw)
	if !ok {
		return nil, fmt.Errorf("could not parse orders response: %s", truncateRaw(raw))
	}
	if limit > 0 && len(orders) > limit {
		orders = orders[:limit]
	}
	return orders, nil
}

func parseOrders(raw json.RawMessage) ([]OrderSummary, bool) {
	var bare []OrderSummary
	if err := json.Unmarshal(raw, &bare); err == nil && len(bare) > 0 {
		return bare, true
	}
	var wrapped struct {
		Orders  []OrderSummary `json:"orders"`
		Content []OrderSummary `json:"content"`
	}
	// A successful object unmarshal is itself proof the shape matched, so an
	// empty history ({"orders":[]}) is a valid empty result, not a parse error.
	if err := json.Unmarshal(raw, &wrapped); err == nil && isJSONObject(raw) {
		if len(wrapped.Content) > 0 {
			return wrapped.Content, true
		}
		return wrapped.Orders, true
	}
	if isEmptyJSON(raw) {
		return []OrderSummary{}, true
	}
	return nil, false
}

// decoratedOrder mirrors the normalizr shape returned by the decorated endpoint:
// entities keyed by id + a result list of line items referencing product ids.
type decoratedOrder struct {
	Entities struct {
		Order   map[string]orderMeta `json:"order"`
		Product map[string]Product   `json:"product"`
	} `json:"entities"`
	Result []struct {
		Product  string `json:"product"`
		Quantity int    `json:"quantity"`
		Price    *Money `json:"price,omitempty"`
	} `json:"result"`
}

type orderMeta struct {
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
	Total   *Money `json:"total"`
}

// GetOrder fetches a single order and denormalizes it via entities.product.
func GetOrder(ctx context.Context, c *client.Client, orderID string) (*OrderDetail, error) {
	var dec decoratedOrder
	path := "/api/order/v6/orders/" + url.PathEscape(orderID) + "/decorated"
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &dec); err != nil {
		return nil, err
	}
	out := &OrderDetail{OrderID: orderID}
	if meta, ok := dec.Entities.Order[orderID]; ok {
		out.Status = meta.Status
		out.Total = meta.Total
		if meta.OrderID != "" {
			out.OrderID = meta.OrderID
		}
	} else if len(dec.Entities.Order) == 1 {
		// Fallback only when unambiguous: map iteration order is randomized,
		// so picking "the first" of several entries would be non-deterministic.
		for _, meta := range dec.Entities.Order {
			out.Status = meta.Status
			out.Total = meta.Total
			if meta.OrderID != "" {
				out.OrderID = meta.OrderID
			}
		}
	}
	for _, li := range dec.Result {
		out.Lines = append(out.Lines, OrderLine{
			Product:  dec.Entities.Product[li.Product],
			Quantity: li.Quantity,
			Price:    li.Price,
		})
	}
	return out, nil
}
