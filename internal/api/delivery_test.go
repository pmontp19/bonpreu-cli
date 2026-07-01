package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/client"
	"github.com/pmontp19/bonpreu-cli/internal/config"
)

func TestGetDeliveryAddresses_PathAndMethodQuery(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("deliveryMethod")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"deliveryDestinationId":"d1","addressId":"a1","formattedAddress":"C-246a, 08812","name":"Home","deliveryMethod":"HOME_DELIVERY","resolvedRegionId":"r1","postalCode":"08812","isPrimary":true},
			{"deliveryDestinationId":"d2","addressId":"a2","formattedAddress":"Pickup point","name":"Esclat","deliveryMethod":"HOME_DELIVERY","resolvedRegionId":"r1","postalCode":"08800","isPrimary":false}
		]`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{}, nil)
	c.BaseURL = srv.URL

	addrs, err := GetDeliveryAddresses(context.Background(), c, MethodHome)
	if err != nil {
		t.Fatalf("GetDeliveryAddresses: %v", err)
	}
	if gotPath != "/api/ecomdeliverydestinations/v4/delivery-addresses" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "HOME_DELIVERY" {
		t.Errorf("deliveryMethod query = %q, want HOME_DELIVERY", gotQuery)
	}
	if len(addrs) != 2 || addrs[0].DeliveryDestinationID != "d1" || !addrs[0].IsPrimary {
		t.Errorf("addrs = %+v", addrs)
	}
}

func TestGetSlots_FlattensV2Grid(t *testing.T) {
	var gotPath string
	var gotBody slotsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"minimumOrderValue":{"currency":"EUR","amount":"35.00"},
			"carriers":[{"gridSlots":[
				{"day":"2026-07-01","slots":[
					{"slotId":"s1","slotWindow":{"startTime":"2026-07-01T10:00:00Z","endTime":"2026-07-01T11:00:00Z"},"deliveryPrice":{"currency":"EUR","amount":"4.95"},"attributes":["AVAILABLE"]},
					{"slotId":"s2","slotWindow":{"startTime":"2026-07-01T11:00:00Z","endTime":"2026-07-01T12:00:00Z"},"deliveryPrice":{"currency":"EUR","amount":"4.95"},"attributes":["UNAVAILABLE"]}
				]},
				{"day":"2026-07-02","slots":[
					{"slotId":"s3","slotWindow":{"startTime":"2026-07-02T10:00:00Z","endTime":"2026-07-02T11:00:00Z"},"deliveryPrice":{"currency":"EUR","amount":"0.00"},"attributes":["AVAILABLE"]}
				]}
			]}]
		}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{DeliveryDestinationID: "d1", RegionID: "r1"}, nil)
	c.BaseURL = srv.URL

	res, err := GetSlots(context.Background(), c, "home", 7)
	if err != nil {
		t.Fatalf("GetSlots: %v", err)
	}
	if gotPath != "/api/ecomslots/v2/slots" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody.DeliveryDestinationID != "d1" || gotBody.RegionID != "r1" {
		t.Errorf("body dest/region = %+v", gotBody)
	}
	if gotBody.ShippingGroupType != ShipHome || gotBody.NumberOfDays != 7 {
		t.Errorf("body shipping/days = %+v", gotBody)
	}
	if res.MinimumOrderValue == nil || res.MinimumOrderValue.Amount != "35.00" {
		t.Errorf("minimumOrderValue = %+v", res.MinimumOrderValue)
	}
	if len(res.Slots) != 3 {
		t.Fatalf("flattened slots = %d, want 3", len(res.Slots))
	}
	if res.Slots[0].SlotID != "s1" || res.Slots[0].Day != "2026-07-01" || !res.Slots[0].Available {
		t.Errorf("slot0 = %+v", res.Slots[0])
	}
	if res.Slots[1].Available {
		t.Errorf("slot1 (UNAVAILABLE) should not be available: %+v", res.Slots[1])
	}
	if res.Slots[2].Day != "2026-07-02" || res.Slots[2].Price != "0.00" {
		t.Errorf("slot2 = %+v", res.Slots[2])
	}
}

func TestGetSlots_MissingSessionDefaults(t *testing.T) {
	c, _ := client.New(&config.Session{}, nil)
	if _, err := GetSlots(context.Background(), c, "home", 7); err == nil {
		t.Fatal("expected error when session lacks deliveryDestinationId/regionId")
	}
}

func TestGroupParams(t *testing.T) {
	cases := []struct {
		group      string
		wantMethod string
		wantErr    bool
	}{
		{"home", MethodHome, false},
		{"", MethodHome, false},
		{"cc", MethodCC, false},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		method, _, err := GroupParams(tc.group)
		if tc.wantErr {
			if err == nil {
				t.Errorf("group %q: expected error", tc.group)
			}
			continue
		}
		if err != nil || method != tc.wantMethod {
			t.Errorf("group %q: method=%q err=%v", tc.group, method, err)
		}
	}
}

func TestReserveSlot_Body(t *testing.T) {
	var gotPath string
	var gotBody reservationRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"slot":{"slotId":"s1","slotWindow":{"startTime":"2026-07-01T10:00:00Z","endTime":"2026-07-01T11:00:00Z"},"type":"STANDARD","expiryTime":"2026-07-01T09:00:00Z","deliveryMethod":"HOME_DELIVERY"}}`))
	}))
	defer srv.Close()
	c, _ := client.New(&config.Session{DeliveryDestinationID: "d1", RegionID: "r1"}, nil)
	c.BaseURL = srv.URL

	res, err := ReserveSlot(context.Background(), c, "s1")
	if err != nil {
		t.Fatalf("ReserveSlot: %v", err)
	}
	if gotPath != "/api/ecomslots/v1/slots/reservation" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody.SlotID != "s1" || gotBody.DeliveryDestinationID != "d1" || gotBody.RegionID != "r1" {
		t.Errorf("body = %+v", gotBody)
	}
	if res.Slot.SlotID != "s1" || res.Slot.Type != "STANDARD" {
		t.Errorf("reservation = %+v", res.Slot)
	}
}
