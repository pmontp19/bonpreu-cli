package api

import (
	"context"
	"net/http"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// VoucherResult is the outcome of applying one discount/voucher code to the
// active cart.
type VoucherResult struct {
	VoucherID           string `json:"voucherId"`
	Valid               bool   `json:"valid"`
	NewlyAdded          bool   `json:"newlyAdded"`
	InBasket            bool   `json:"inBasket"`
	ValidationErrorCode string `json:"validationErrorCode,omitempty"`
}

type applyVouchersResponse struct {
	VouchersAddResult []VoucherResult `json:"vouchersAddResult"`
}

// ApplyVouchers applies one or more discount codes to the active cart. The
// API accepts and reports on each code independently — a mix of valid and
// invalid codes in one call returns one result per code, not an error.
func ApplyVouchers(ctx context.Context, c *client.Client, codes []string) ([]VoucherResult, error) {
	var resp applyVouchersResponse
	if err := c.DoJSON(ctx, http.MethodPost, "/api/cart/v1/carts/active/vouchers", codes, &resp); err != nil {
		return nil, err
	}
	return resp.VouchersAddResult, nil
}
