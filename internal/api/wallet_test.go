package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetWalletItems_FlattensDetails(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"walletItemId":"w1","defaultWalletItem":true,"expired":false,"details":{"lastFourDigits":"1111","cardType":"MasterCard","expiryMonth":"06","expiryYear":"2028"}},
			{"walletItemId":"w2","defaultWalletItem":false,"expired":true,"details":{"lastFourDigits":"2222","cardType":"Visa","expiryMonth":"01","expiryYear":"2027"}}
		]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	methods, err := GetWalletItems(context.Background(), c)
	if err != nil {
		t.Fatalf("GetWalletItems: %v", err)
	}
	if gotPath != "/api/walletservice/v3/wallet-items" {
		t.Errorf("path = %q", gotPath)
	}
	if len(methods) != 2 {
		t.Fatalf("methods = %d, want 2", len(methods))
	}
	if methods[0].WalletItemID != "w1" || methods[0].LastFourDigits != "1111" || methods[0].CardType != "MasterCard" || !methods[0].Default || methods[0].Expired {
		t.Errorf("methods[0] = %+v", methods[0])
	}
	if methods[1].WalletItemID != "w2" || methods[1].Default || !methods[1].Expired {
		t.Errorf("methods[1] = %+v", methods[1])
	}
}

func TestGetWalletItems_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	methods, err := GetWalletItems(context.Background(), c)
	if err != nil {
		t.Fatalf("GetWalletItems: %v", err)
	}
	if len(methods) != 0 {
		t.Errorf("methods = %+v, want empty", methods)
	}
}
