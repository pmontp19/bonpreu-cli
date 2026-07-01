package cli

import (
	"testing"

	"github.com/pmontp19/bonpreu-cli/internal/api"
)

func TestFilterByPostal(t *testing.T) {
	addrs := []api.DeliveryAddress{
		{DeliveryDestinationID: "d1", PostalCode: "08812"},
		{DeliveryDestinationID: "d2", PostalCode: "08800"},
		{DeliveryDestinationID: "d3", PostalCode: "17001"},
	}

	if got := filterByPostal(addrs, ""); len(got) != 3 {
		t.Errorf("empty prefix: got %d addrs, want 3 (unfiltered)", len(got))
	}
	if got := filterByPostal(addrs, "088"); len(got) != 2 || got[0].DeliveryDestinationID != "d1" || got[1].DeliveryDestinationID != "d2" {
		t.Errorf("prefix 088: got %+v", got)
	}
	if got := filterByPostal(addrs, "08812"); len(got) != 1 || got[0].DeliveryDestinationID != "d1" {
		t.Errorf("exact prefix: got %+v", got)
	}
	if got := filterByPostal(addrs, "99999"); len(got) != 0 {
		t.Errorf("no match: got %+v, want empty", got)
	}
}
