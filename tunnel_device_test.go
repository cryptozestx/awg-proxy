package main

import (
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"

	"github.com/amnezia-vpn/amneziawg-go/tun"
)

func TestBuildResolvedTunnelUAPI(t *testing.T) {
	cfg := validTunnelConfig()
	resolved := netip.MustParseAddrPort("203.0.113.10:51820")

	uapi, err := BuildResolvedTunnelUAPI(cfg, resolved)
	if err != nil {
		t.Fatalf("BuildResolvedTunnelUAPI returned error: %v", err)
	}

	if !strings.Contains(uapi, "endpoint=203.0.113.10:51820") {
		t.Fatalf("UAPI = %q, want resolved endpoint", uapi)
	}
	if strings.Contains(uapi, "endpoint=vpn.example.test:51820") {
		t.Fatalf("UAPI = %q, contains unresolved endpoint", uapi)
	}
}

func TestAWGTunnelDeviceFactoryClosesTUNOnNameFailure(t *testing.T) {
	nameErr := errors.New("name failed")
	tunDev := &fakeTUN{nameErr: nameErr}
	newDeviceCalled := false

	origCreateTUN := createTUN
	origNewAWGDevice := newAWGDevice
	createTUN = func(name string, mtu int) (tun.Device, error) {
		if name != "utun-test" {
			t.Fatalf("name = %q, want utun-test", name)
		}
		if mtu != 1420 {
			t.Fatalf("mtu = %d, want 1420", mtu)
		}
		return tunDev, nil
	}
	newAWGDevice = func(tun.Device, int) awgDevice {
		newDeviceCalled = true
		return &fakeAWGDevice{}
	}
	t.Cleanup(func() {
		createTUN = origCreateTUN
		newAWGDevice = origNewAWGDevice
	})

	_, err := (AWGTunnelDeviceFactory{}).Create("utun-test", 1420, false)
	if !errors.Is(err, nameErr) {
		t.Fatalf("Create error = %v, want name failure", err)
	}
	if tunDev.closeCount != 1 {
		t.Fatalf("TUN close count = %d, want 1", tunDev.closeCount)
	}
	if newDeviceCalled {
		t.Fatalf("newAWGDevice was called after name failure")
	}
}

func TestAWGTunnelDeviceUpAppliesUAPIBeforeStart(t *testing.T) {
	dev := &fakeAWGDevice{}
	tunnel := &AWGTunnelDevice{dev: dev}

	err := tunnel.Up("private_key=test\n")
	if err != nil {
		t.Fatalf("Up returned error: %v", err)
	}

	want := []string{"ipc:private_key=test\n", "up"}
	if strings.Join(dev.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("calls = %#v, want %#v", dev.calls, want)
	}
}

func TestAWGTunnelDeviceCloseDelegatesToDeviceOnly(t *testing.T) {
	dev := &fakeAWGDevice{}
	tunDev := &fakeTUN{}
	tunnel := &AWGTunnelDevice{tun: tunDev, dev: dev}

	err := tunnel.Close()
	if err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if dev.closeCount != 1 {
		t.Fatalf("device close count = %d, want 1", dev.closeCount)
	}
	if tunDev.closeCount != 0 {
		t.Fatalf("TUN close count = %d, want 0", tunDev.closeCount)
	}
}

type fakeAWGDevice struct {
	calls      []string
	closeCount int
}

func (d *fakeAWGDevice) IpcSet(uapi string) error {
	d.calls = append(d.calls, "ipc:"+uapi)
	return nil
}

func (d *fakeAWGDevice) Up() error {
	d.calls = append(d.calls, "up")
	return nil
}

func (d *fakeAWGDevice) Close() {
	d.closeCount++
}

type fakeTUN struct {
	name       string
	nameErr    error
	closeCount int
	events     chan tun.Event
}

func (t *fakeTUN) File() *os.File {
	return nil
}

func (t *fakeTUN) Read([][]byte, []int, int) (int, error) {
	return 0, nil
}

func (t *fakeTUN) Write([][]byte, int) (int, error) {
	return 0, nil
}

func (t *fakeTUN) MTU() (int, error) {
	return 1420, nil
}

func (t *fakeTUN) Name() (string, error) {
	if t.nameErr != nil {
		return "", t.nameErr
	}
	return t.name, nil
}

func (t *fakeTUN) Events() <-chan tun.Event {
	if t.events == nil {
		t.events = make(chan tun.Event)
	}
	return t.events
}

func (t *fakeTUN) Close() error {
	t.closeCount++
	return nil
}

func (t *fakeTUN) BatchSize() int {
	return 1
}
