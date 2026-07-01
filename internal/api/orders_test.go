package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetOrders_ParsesAndLimits(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orders":[
			{"orderId":"o1","status":"DELIVERED","placedAt":"2025-12-21T10:00:00Z","deliveryDate":"2025-12-22","total":{"currency":"EUR","amount":"158.29"}},
			{"orderId":"o2","status":"CANCELLED","placedAt":"2025-11-01T10:00:00Z","total":{"currency":"EUR","amount":"42.00"}},
			{"orderId":"o3","status":"DELIVERED","placedAt":"2025-10-01T10:00:00Z","total":{"currency":"EUR","amount":"12.00"}}
		]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	orders, err := GetOrders(context.Background(), c, 2)
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if gotPath != "/api/order/v6/orders" {
		t.Errorf("path = %q", gotPath)
	}
	if len(orders) != 2 {
		t.Fatalf("orders = %d, want 2 (limit)", len(orders))
	}
	if orders[0].OrderID != "o1" || orders[0].Status != "DELIVERED" || orders[0].Total == nil || orders[0].Total.Amount != "158.29" {
		t.Errorf("order0 = %+v", orders[0])
	}
}

func TestGetOrders_BareArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"orderId":"o1","status":"DELIVERED","total":{"currency":"EUR","amount":"10.00"}}]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	orders, err := GetOrders(context.Background(), c, 0)
	if err != nil {
		t.Fatalf("GetOrders: %v", err)
	}
	if len(orders) != 1 || orders[0].OrderID != "o1" {
		t.Errorf("orders = %+v", orders)
	}
}

func TestGetOrders_EmptyWrapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"orders":[]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	orders, err := GetOrders(context.Background(), c, 0)
	if err != nil {
		t.Fatalf("GetOrders on empty history should not error: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("orders = %+v, want empty", orders)
	}
}

func TestGetOrder_EscapesOrderID(t *testing.T) {
	var gotEscapedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEscapedPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"entities":{"order":{},"product":{}},"result":[]}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	if _, err := GetOrder(context.Background(), c, "a/b"); err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if gotEscapedPath != "/api/order/v6/orders/a%2Fb/decorated" {
		t.Errorf("escaped path = %q, want the orderID segment escaped", gotEscapedPath)
	}
}

func TestGetOrder_DenormalizesDecorated(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"entities":{
				"order":{"o1":{"orderId":"o1","status":"DELIVERED","total":{"currency":"EUR","amount":"11.00"}}},
				"product":{
					"p1":{"productId":"p1","retailerProductId":"111","name":"Iogurt natural","price":{"currency":"EUR","amount":"3.50"}},
					"p2":{"productId":"p2","retailerProductId":"222","name":"Llet sencera","price":{"currency":"EUR","amount":"1.00"}}
				}
			},
			"result":[
				{"product":"p1","quantity":2,"price":{"currency":"EUR","amount":"7.00"}},
				{"product":"p2","quantity":4,"price":{"currency":"EUR","amount":"4.00"}}
			]
		}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	ord, err := GetOrder(context.Background(), c, "o1")
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if gotPath != "/api/order/v6/orders/o1/decorated" {
		t.Errorf("path = %q", gotPath)
	}
	if ord.OrderID != "o1" || ord.Status != "DELIVERED" || ord.Total == nil || ord.Total.Amount != "11.00" {
		t.Errorf("order meta = %+v", ord)
	}
	if len(ord.Lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(ord.Lines))
	}
	if ord.Lines[0].Product.Name != "Iogurt natural" || ord.Lines[0].Product.RetailerProductID != "111" {
		t.Errorf("line0 product = %+v", ord.Lines[0].Product)
	}
	if ord.Lines[0].Quantity != 2 || ord.Lines[0].Price == nil || ord.Lines[0].Price.Amount != "7.00" {
		t.Errorf("line0 = %+v", ord.Lines[0])
	}
	if ord.Lines[1].Product.Name != "Llet sencera" || ord.Lines[1].Quantity != 4 {
		t.Errorf("line1 = %+v", ord.Lines[1])
	}
}
