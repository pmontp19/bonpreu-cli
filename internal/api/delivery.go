package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"

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

// resolveDestination picks the delivery destination + region to query for a
// group. "home" defaults to the session's address (captured once at
// import-har time); "cc" defaults to the account's primary customer-
// collection pickup point, since a pickup point's slots are scoped to its
// own destination/region, not the session's home address. Pass destination
// to target a specific address/pickup point instead (see
// `delivery addresses --method home|cc` for the list of ids).
func resolveDestination(ctx context.Context, c *client.Client, group, destination string) (destID, regionID string, err error) {
	if group != "cc" && destination == "" {
		if c.Sess == nil {
			return "", "", fmt.Errorf("no delivery destination/region in session; run import-har")
		}
		return c.Sess.DeliveryDestinationID, c.Sess.RegionID, nil
	}
	method := MethodHome
	kind := "home"
	if group == "cc" {
		method, kind = MethodCC, "customer-collection"
	}
	addrs, err := GetDeliveryAddresses(ctx, c, method)
	if err != nil {
		return "", "", err
	}
	if destination != "" {
		for _, a := range addrs {
			if a.DeliveryDestinationID == destination {
				return a.DeliveryDestinationID, a.ResolvedRegionID, nil
			}
		}
		return "", "", fmt.Errorf("destination %q not found among %s addresses; run `delivery addresses --method %s`", destination, kind, group)
	}
	if len(addrs) == 0 {
		return "", "", fmt.Errorf("no %s pickup points on the account", kind)
	}
	best := addrs[0]
	for _, a := range addrs {
		if a.IsPrimary {
			best = a
			break
		}
	}
	return best.DeliveryDestinationID, best.ResolvedRegionID, nil
}

// GetSlots posts the ecomslots/v2 grid request for the group's delivery
// destination + region and flattens the day grid into a single list, sorted
// by day and start time.
func GetSlots(ctx context.Context, c *client.Client, group, destination string, days int) (*SlotsResult, error) {
	_, shippingGroupType, err := GroupParams(group)
	if err != nil {
		return nil, err
	}
	destID, regionID, err := resolveDestination(ctx, c, group, destination)
	if err != nil {
		return nil, err
	}
	if destID == "" || regionID == "" {
		return nil, fmt.Errorf("no delivery destination/region in session; run import-har")
	}
	if days <= 0 {
		days = 7
	}
	req := slotsRequest{
		DeliveryDestinationID: destID,
		RegionID:              regionID,
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
	sort.Slice(res.Slots, func(i, j int) bool {
		if res.Slots[i].Day != res.Slots[j].Day {
			return res.Slots[i].Day < res.Slots[j].Day
		}
		return res.Slots[i].StartTime < res.Slots[j].StartTime
	})
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

// ReserveSlot reserves a slot for the group's delivery destination + region.
// group and destination must match whatever the slot was fetched with (see
// GetSlots).
func ReserveSlot(ctx context.Context, c *client.Client, group, destination, slotID string) (*Reservation, error) {
	destID, regionID, err := resolveDestination(ctx, c, group, destination)
	if err != nil {
		return nil, err
	}
	if destID == "" || regionID == "" {
		return nil, fmt.Errorf("no delivery destination/region in session; run import-har")
	}
	req := reservationRequest{
		RegionID:              regionID,
		SlotID:                slotID,
		DeliveryDestinationID: destID,
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
