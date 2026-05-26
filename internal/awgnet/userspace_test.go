package awgnet

import (
	"net/netip"
	"testing"
)

func TestParseAddressesReturnsErrorForMalformedAddress(t *testing.T) {
	if _, err := parseAddresses([]string{"not-an-ip"}); err == nil {
		t.Fatal("expected malformed address to return an error")
	}
}

func TestParseAddressesStripsCIDRAndParsesPlainAddress(t *testing.T) {
	got, err := parseAddresses([]string{"10.8.0.2/32", "1.1.1.1"})
	if err != nil {
		t.Fatalf("parseAddresses returned error: %v", err)
	}

	want := []netip.Addr{
		netip.MustParseAddr("10.8.0.2"),
		netip.MustParseAddr("1.1.1.1"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d addresses, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("address %d = %v, want %v", i, got[i], want[i])
		}
	}
}
