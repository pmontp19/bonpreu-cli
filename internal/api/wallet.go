package api

import (
	"context"
	"net/http"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// PaymentMethod is one saved card, flattened from the walletservice response.
type PaymentMethod struct {
	WalletItemID   string `json:"walletItemId"`
	CardType       string `json:"cardType,omitempty"`
	LastFourDigits string `json:"lastFourDigits,omitempty"`
	ExpiryMonth    string `json:"expiryMonth,omitempty"`
	ExpiryYear     string `json:"expiryYear,omitempty"`
	Default        bool   `json:"default"`
	Expired        bool   `json:"expired"`
}

type walletItem struct {
	WalletItemID string `json:"walletItemId"`
	Default      bool   `json:"defaultWalletItem"`
	Expired      bool   `json:"expired"`
	Details      struct {
		LastFourDigits string `json:"lastFourDigits"`
		CardType       string `json:"cardType"`
		ExpiryMonth    string `json:"expiryMonth"`
		ExpiryYear     string `json:"expiryYear"`
	} `json:"details"`
}

// GetWalletItems lists saved payment methods (read-only) for the session.
func GetWalletItems(ctx context.Context, c *client.Client) ([]PaymentMethod, error) {
	var items []walletItem
	if err := c.DoJSON(ctx, http.MethodGet, "/api/walletservice/v3/wallet-items", nil, &items); err != nil {
		return nil, err
	}
	methods := make([]PaymentMethod, 0, len(items))
	for _, it := range items {
		methods = append(methods, PaymentMethod{
			WalletItemID:   it.WalletItemID,
			CardType:       it.Details.CardType,
			LastFourDigits: it.Details.LastFourDigits,
			ExpiryMonth:    it.Details.ExpiryMonth,
			ExpiryYear:     it.Details.ExpiryYear,
			Default:        it.Default,
			Expired:        it.Expired,
		})
	}
	return methods, nil
}
