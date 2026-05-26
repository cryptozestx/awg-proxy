package main

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type fakeDynamicBypassRoutes struct {
	prefixes []netip.Prefix
	reasons  []string
	ttls     []time.Duration
}

func (r *fakeDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.prefixes = append(r.prefixes, prefix)
	r.reasons = append(r.reasons, reason)
	r.ttls = append(r.ttls, ttl)
	return nil
}

func (r *fakeDynamicBypassRoutes) Close() error {
	return nil
}

func startFakeDNSServer(t *testing.T, name string, ip string) (string, func()) {
	t.Helper()

	mux := dns.NewServeMux()
	mux.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		if len(req.Question) > 0 && req.Question[0].Qtype == dns.TypeA && req.Question[0].Name == name {
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   name,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    30,
				},
				A: net.ParseIP(ip).To4(),
			})
		}
		if err := w.WriteMsg(resp); err != nil {
			t.Errorf("write DNS response: %v", err)
		}
	})

	packetConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake DNS: %v", err)
	}
	server := &dns.Server{PacketConn: packetConn, Handler: mux}
	errs := make(chan error, 1)
	go func() {
		errs <- server.ActivateAndServe()
	}()

	stop := func() {
		if err := server.Shutdown(); err != nil {
			t.Fatalf("shutdown fake DNS: %v", err)
		}
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("fake DNS server failed: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("fake DNS server did not stop")
		}
	}

	return packetConn.LocalAddr().String(), stop
}

func TestDomainBypassRuntimeAddsRoutesForMatchingAnswers(t *testing.T) {
	runtime := NewDomainBypassRuntime()
	routes := &fakeDynamicBypassRoutes{}
	rules := TunnelRules{DomainRules: []DomainRule{{Pattern: "*.delimobil.*"}}}
	answer := DNSAnswer{
		Name: "git.delimobil.ru",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  30 * time.Second,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}

	want := netip.MustParsePrefix("198.51.100.44/32")
	if len(routes.prefixes) != 1 || routes.prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", routes.prefixes, want)
	}
	if routes.reasons[0] != "dns:git.delimobil.ru" {
		t.Fatalf("reason = %q, want dns:git.delimobil.ru", routes.reasons[0])
	}
	if routes.ttls[0] != 10*time.Minute {
		t.Fatalf("ttl = %v, want 10m", routes.ttls[0])
	}
}

func TestDomainBypassRuntimeIgnoresNonMatchingAnswers(t *testing.T) {
	runtime := NewDomainBypassRuntime()
	routes := &fakeDynamicBypassRoutes{}
	rules := TunnelRules{DomainRules: []DomainRule{{Pattern: "*.delimobil.*"}}}
	answer := DNSAnswer{
		Name: "openai.com",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  time.Hour,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}
	if len(routes.prefixes) != 0 {
		t.Fatalf("prefixes = %v, want empty", routes.prefixes)
	}
}

func TestDomainBypassRuntimeForwarderHandlesARecord(t *testing.T) {
	upstreamAddr, stopUpstream := startFakeDNSServer(t, "git.delimobil.ru.", "198.51.100.44")
	defer stopUpstream()

	routes := &fakeDynamicBypassRoutes{}
	rules := TunnelRules{DomainRules: []DomainRule{{Pattern: "*.delimobil.*"}}}
	runtime := NewDomainBypassRuntime()
	if err := runtime.Start(context.Background(), DomainBypassConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   upstreamAddr,
		Rules:      rules,
		Routes:     routes,
	}); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer runtime.Close()

	client := &dns.Client{}
	msg := new(dns.Msg)
	msg.SetQuestion("git.delimobil.ru.", dns.TypeA)
	resp, _, err := client.Exchange(msg, runtime.Addr())
	if err != nil {
		t.Fatalf("DNS exchange failed: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}

	want := netip.MustParsePrefix("198.51.100.44/32")
	if len(routes.prefixes) != 1 || routes.prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", routes.prefixes, want)
	}
}
