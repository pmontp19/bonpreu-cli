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
// `result` is the root order id (a string), `entities.order[id]` carries the
// order metadata and its line items, and `entities.product` holds the referenced
// products keyed by uuid. Product prices are nested under `price.current`, so
// products decode as raw messages and are unpacked per line.
type decoratedOrder struct {
	Entities struct {
		Order   map[string]orderMeta       `json:"order"`
		Product map[string]json.RawMessage `json:"product"`
	} `json:"entities"`
	Result string `json:"result"`
}

type orderMeta struct {
	OrderID     string `json:"orderId"`
	Status      string `json:"status"`
	OrderTotals struct {
		TotalPrice *Money `json:"totalPrice"`
		FinalPrice *Money `json:"finalPrice"`
	} `json:"orderTotals"`
	Items []orderItemRef `json:"items"`
}

type orderItemRef struct {
	Product  string `json:"product"`
	Quantity int    `json:"quantity"`
}

// GetOrder fetches a single order and denormalizes it via entities.product.
func GetOrder(ctx context.Context, c *client.Client, orderID string) (*OrderDetail, error) {
	var dec decoratedOrder
	path := "/api/order/v6/orders/" + url.PathEscape(orderID) + "/decorated"
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &dec); err != nil {
		return nil, err
	}
	out := &OrderDetail{OrderID: orderID}
	meta, ok := dec.Entities.Order[dec.Result]
	if !ok {
		meta, ok = dec.Entities.Order[orderID]
	}
	if !ok && len(dec.Entities.Order) == 1 {
		// Fallback only when unambiguous: map iteration order is randomized,
		// so picking "the first" of several entries would be non-deterministic.
		for _, m := range dec.Entities.Order {
			meta, ok = m, true
		}
	}
	if ok {
		out.Status = meta.Status
		out.Total = meta.OrderTotals.TotalPrice
		if out.Total == nil {
			out.Total = meta.OrderTotals.FinalPrice
		}
		if meta.OrderID != "" {
			out.OrderID = meta.OrderID
		}
		for _, li := range meta.Items {
			prod, price := decodeOrderProduct(dec.Entities.Product[li.Product])
			out.Lines = append(out.Lines, OrderLine{
				Product:  prod,
				Quantity: li.Quantity,
				Price:    price,
			})
		}
	}
	return out, nil
}

// decodeOrderProduct unpacks a product entity, whose price is nested under
// `price.current` rather than the flat Money shape used elsewhere.
func decodeOrderProduct(raw json.RawMessage) (Product, *Money) {
	var p Product
	if len(raw) == 0 {
		return p, nil
	}
	_ = json.Unmarshal(raw, &p)
	var nested struct {
		Price struct {
			Current *Money `json:"current"`
		} `json:"price"`
	}
	_ = json.Unmarshal(raw, &nested)
	if p.Price == nil {
		p.Price = nested.Price.Current
	}
	return p, nested.Price.Current
}
