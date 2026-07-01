package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/pmontp19/bonpreu-cli/internal/client"
)

// Delivery method query values and shipping group type strings used by the
// ecomslots/v2 grid and delivery-addresses endpoints.
const (
	MethodHome = "HOME_DELIVERY"
	MethodCC   = "CUSTOMER_COLLECTION"
	ShipHome   = "default home delivery"
	ShipCC     = "default customer collection"
)

type DeliveryAddress struct {
	DeliveryDestinationID string `json:"deliveryDestinationId"`
	AddressID             string `json:"addressId"`
	FormattedAddress      string `json:"formattedAddress"`
	Name                  string `json:"name"`
	DeliveryType          string `json:"deliveryType,omitempty"`
	DeliveryMethod        string `json:"deliveryMethod"`
	ResolvedRegionID      string `json:"resolvedRegionId"`
	PostalCode            string `json:"postalCode"`
	IsPrimary             bool   `json:"isPrimary"`
}

// GetDeliveryAddresses lists saved addresses (MethodHome) or pickup points
// (MethodCC) for the session.
func GetDeliveryAddresses(ctx context.Context, c *client.Client, method string) ([]DeliveryAddress, error) {
	path := "/api/ecomdeliverydestinations/v4/delivery-addresses?deliveryMethod=" + url.QueryEscape(method)
	var addrs []DeliveryAddress
	if err := c.DoJSON(ctx, http.MethodGet, path, nil, &addrs); err != nil {
		return nil, err
	}
	return addrs, nil
}

type SlotWindow struct {
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type gridSlot struct {
	SlotID        string     `json:"slotId"`
	SlotWindow    SlotWindow `json:"slotWindow"`
	DeliveryPrice *Money     `json:"deliveryPrice,omitempty"`
	Attributes    []string   `json:"attributes,omitempty"`
}

type gridDay struct {
	Day   string     `json:"day"`
	Slots []gridSlot `json:"slots"`
}

type carrier struct {
	GridSlots []gridDay `json:"gridSlots"`
}

type slotsResponse struct {
	MinimumOrderValue *Money    `json:"minimumOrderValue,omitempty"`
	Carriers          []carrier `json:"carriers"`
}

type slotsRequest struct {
	DeliveryDestinationID string `json:"deliveryDestinationId"`
	RegionID              string `json:"regionId"`
	DisplayConfiguration  string `json:"displayConfiguration"`
	ShippingGroupType     string `json:"shippingGroupType"`
	NumberOfDays          int    `json:"numberOfDays"`
}

// Slot is one flattened entry of the v2 day grid.
type Slot struct {
	SlotID    string `json:"slotId"`
	Day       string `json:"day,omitempty"`
	StartTime string `json:"startTime,omitempty"`
	EndTime   string `json:"endTime,omitempty"`
	Price     string `json:"price,omitempty"`
	Available bool   `json:"available"`
}

// SlotsResult is the flattened slots view plus the minimum order value.
type SlotsResult struct {
	MinimumOrderValue *Money `json:"minimumOrderValue,omitempty"`
	Slots             []Slot `json:"slots"`
}

// GroupParams maps the user-facing group ("home"|"cc") to the delivery method
// and shipping group type strings the API expects.
func GroupParams(group string) (method, shippingGroupType string, err error) {
	switch group {
	case "home", "":
		return MethodHome, ShipHome, nil
	case "cc":
		return MethodCC, ShipCC, nil
	default:
		return "", "", fmt.Errorf("unknown group %q (want home|cc)", group)
	}
}

// GetSlots posts the ecomslots/v2 grid request for the session's delivery
// destination + region and flattens the day grid into a single list.
func GetSlots(ctx context.Context, c *client.Client, group string, days int) (*SlotsResult, error) {
	_, shippingGroupType, err := GroupParams(group)
	if err != nil {
		return nil, err
	}
	if c.Sess == nil || c.Sess.DeliveryDestinationID == "" || c.Sess.RegionID == "" {
		return nil, fmt.Errorf("no delivery destination/region in session; run import-har")
	}
	if days <= 0 {
		days = 7
	}
	req := slotsRequest{
		DeliveryDestinationID: c.Sess.DeliveryDestinationID,
		RegionID:              c.Sess.RegionID,
		DisplayConfiguration:  "DELIVERY_METHOD",
		ShippingGroupType:     shippingGroupType,
		NumberOfDays:          days,
	}
	var resp slotsResponse
	if err := c.DoJSON(ctx, http.MethodPost, "/api/ecomslots/v2/slots", req, &resp); err != nil {
		return nil, err
	}
	res := &SlotsResult{MinimumOrderValue: resp.MinimumOrderValue}
	for _, ca := range resp.Carriers {
		for _, d := range ca.GridSlots {
			for _, s := range d.Slots {
				res.Slots = append(res.Slots, Slot{
					SlotID:    s.SlotID,
					Day:       d.Day,
					StartTime: s.SlotWindow.StartTime,
					EndTime:   s.SlotWindow.EndTime,
					Price:     priceAmount(s.DeliveryPrice),
					Available: hasAttribute(s.Attributes, "AVAILABLE"),
				})
			}
		}
	}
	return res, nil
}

type reservationRequest struct {
	RegionID              string `json:"regionId"`
	SlotID                string `json:"slotId"`
	DeliveryDestinationID string `json:"deliveryDestinationId"`
}

// Reservation is the slot confirmation returned by the reservation endpoint.
type Reservation struct {
	Slot struct {
		SlotID         string     `json:"slotId"`
		SlotWindow     SlotWindow `json:"slotWindow"`
		Type           string     `json:"type"`
		ExpiryTime     string     `json:"expiryTime"`
		DeliveryMethod string     `json:"deliveryMethod"`
	} `json:"slot"`
}

// ReserveSlot reserves a slot for the session's delivery destination + region.
func ReserveSlot(ctx context.Context, c *client.Client, slotID string) (*Reservation, error) {
	if c.Sess == nil || c.Sess.DeliveryDestinationID == "" || c.Sess.RegionID == "" {
		return nil, fmt.Errorf("no delivery destination/region in session; run import-har")
	}
	req := reservationRequest{
		RegionID:              c.Sess.RegionID,
		SlotID:                slotID,
		DeliveryDestinationID: c.Sess.DeliveryDestinationID,
	}
	var res Reservation
	if err := c.DoJSON(ctx, http.MethodPost, "/api/ecomslots/v1/slots/reservation", req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func priceAmount(m *Money) string {
	if m == nil {
		return ""
	}
	return m.Amount
}

func hasAttribute(attrs []string, want string) bool {
	for _, a := range attrs {
		if a == want {
			return true
		}
	}
	return false
}
