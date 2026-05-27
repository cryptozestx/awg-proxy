package dns

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type fakeDynamicBypassRoutes struct {
	mu       sync.Mutex
	prefixes []netip.Prefix
	reasons  []string
	ttls     []time.Duration
}

func (r *fakeDynamicBypassRoutes) AddBypassRoute(ctx context.Context, prefix netip.Prefix, reason string, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prefixes = append(r.prefixes, prefix)
	r.reasons = append(r.reasons, reason)
	r.ttls = append(r.ttls, ttl)
	return nil
}

func (r *fakeDynamicBypassRoutes) Close() error {
	return nil
}

func (r *fakeDynamicBypassRoutes) snapshot() ([]netip.Prefix, []string, []time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefixes := append([]netip.Prefix(nil), r.prefixes...)
	reasons := append([]string(nil), r.reasons...)
	ttls := append([]time.Duration(nil), r.ttls...)
	return prefixes, reasons, ttls
}

func startFakeDNSServer(t *testing.T, name string, ip string) (string, func()) {
	t.Helper()

	return startFakeDNSServerFunc(t, func(w dns.ResponseWriter, req *dns.Msg) {
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
}

func startFakeDNSServerFunc(t *testing.T, handler dns.HandlerFunc) (string, func()) {
	t.Helper()

	mux := dns.NewServeMux()
	mux.HandleFunc(".", handler)

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

func cnameResponseHandler(t *testing.T, queryName string, cnameName string, ip string) dns.HandlerFunc {
	t.Helper()

	return func(w dns.ResponseWriter, req *dns.Msg) {
		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Answer = append(resp.Answer,
			&dns.CNAME{
				Hdr: dns.RR_Header{
					Name:   queryName,
					Rrtype: dns.TypeCNAME,
					Class:  dns.ClassINET,
					Ttl:    20,
				},
				Target: cnameName,
			},
			&dns.A{
				Hdr: dns.RR_Header{
					Name:   cnameName,
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    30,
				},
				A: net.ParseIP(ip).To4(),
			},
		)
		if err := w.WriteMsg(resp); err != nil {
			t.Errorf("write DNS response: %v", err)
		}
	}
}

func TestDomainBypassRuntimeAddsRoutesForMatchingAnswers(t *testing.T) {
	runtime := &DNSDomainBypassRuntime{}
	routes := &fakeDynamicBypassRoutes{}
	rules := []DomainRule{{Pattern: "*.delimobil.*"}}
	answer := DNSAnswer{
		Name: "git.delimobil.ru",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  30 * time.Second,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}

	want := netip.MustParsePrefix("198.51.100.44/32")
	prefixes, reasons, ttls := routes.snapshot()
	if len(prefixes) != 1 || prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", prefixes, want)
	}
	if reasons[0] != "dns:git.delimobil.ru" {
		t.Fatalf("reason = %q, want dns:git.delimobil.ru", reasons[0])
	}
	if ttls[0] != 10*time.Minute {
		t.Fatalf("ttl = %v, want 10m", ttls[0])
	}
}

func TestDomainBypassRuntimeIgnoresNonMatchingAnswers(t *testing.T) {
	runtime := &DNSDomainBypassRuntime{}
	routes := &fakeDynamicBypassRoutes{}
	rules := []DomainRule{{Pattern: "*.delimobil.*"}}
	answer := DNSAnswer{
		Name: "openai.com",
		A:    []netip.Addr{netip.MustParseAddr("198.51.100.44")},
		TTL:  time.Hour,
	}

	if err := runtime.HandleAnswer(context.Background(), rules, answer, routes); err != nil {
		t.Fatalf("HandleAnswer returned error: %v", err)
	}
	prefixes, _, _ := routes.snapshot()
	if len(prefixes) != 0 {
		t.Fatalf("prefixes = %v, want empty", prefixes)
	}
}

func TestDomainBypassRuntimeForwarderHandlesARecord(t *testing.T) {
	upstreamAddr, stopUpstream := startFakeDNSServer(t, "git.delimobil.ru.", "198.51.100.44")
	defer stopUpstream()

	routes := &fakeDynamicBypassRoutes{}
	rules := []DomainRule{{Pattern: "*.delimobil.*"}}
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
	prefixes, _, _ := routes.snapshot()
	if len(prefixes) != 1 || prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", prefixes, want)
	}
}

func TestDomainBypassRuntimeStartThenImmediateClose(t *testing.T) {
	for i := 0; i < 25; i++ {
		runtime := NewDomainBypassRuntime()
		if err := runtime.Start(context.Background(), DomainBypassConfig{
			ListenAddr: "127.0.0.1:0",
			Upstream:   "127.0.0.1:1",
		}); err != nil {
			t.Fatalf("Start returned error on iteration %d: %v", i, err)
		}
		if err := runtime.Close(); err != nil {
			t.Fatalf("Close returned error on iteration %d: %v", i, err)
		}
	}
}

func TestDomainBypassRuntimeStartWithCanceledContextClosesBeforeReady(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runtime := NewDomainBypassRuntime()
	if err := runtime.Start(ctx, DomainBypassConfig{
		ListenAddr: "127.0.0.1:0",
		Upstream:   "127.0.0.1:1",
	}); err != context.Canceled {
		t.Fatalf("Start error = %v, want context.Canceled", err)
	}
	if addr := runtime.Addr(); addr != "" {
		t.Fatalf("Addr = %q, want empty after canceled start", addr)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close after canceled start returned error: %v", err)
	}
}

func TestDomainBypassRuntimeForwarderHandlesCNAMEARecord(t *testing.T) {
	upstreamAddr, stopUpstream := startFakeDNSServerFunc(t, cnameResponseHandler(t, "git.delimobil.ru.", "cdn.example.net.", "198.51.100.44"))
	defer stopUpstream()

	routes := &fakeDynamicBypassRoutes{}
	rules := []DomainRule{{Pattern: "*.delimobil.*"}}
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
	if len(resp.Answer) != 2 {
		t.Fatalf("answers = %d, want 2", len(resp.Answer))
	}

	want := netip.MustParsePrefix("198.51.100.44/32")
	prefixes, reasons, _ := routes.snapshot()
	if len(prefixes) != 1 || prefixes[0] != want {
		t.Fatalf("prefixes = %v, want [%v]", prefixes, want)
	}
	if reasons[0] != "dns:git.delimobil.ru" {
		t.Fatalf("reason = %q, want dns:git.delimobil.ru", reasons[0])
	}
}

func TestCollectDNSAAnswersForQuestionsIgnoresUnrelatedARecord(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("git.delimobil.ru.", dns.TypeA)
	resp := new(dns.Msg)
	resp.SetReply(req)
	resp.Answer = append(resp.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "unrelated.delimobil.ru.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    30,
		},
		A: net.ParseIP("198.51.100.44").To4(),
	})
	rules := []DomainRule{{Pattern: "*.delimobil.*"}}

	answers := collectDNSAAnswersForQuestions(req, resp, rules)
	if len(answers) != 0 {
		t.Fatalf("answers = %v, want empty", answers)
	}
}

func TestDomainBypassRuntimeNilDNSHelpersDoNotPanic(t *testing.T) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("serverFailure(nil) panicked: %v", r)
			}
		}()
		resp := serverFailure(nil)
		if resp == nil || resp.Rcode != dns.RcodeServerFailure {
			t.Fatalf("serverFailure(nil) = %#v, want SERVFAIL response", resp)
		}
	}()

	if answers := collectDNSAAnswersForQuestions(nil, nil, nil); len(answers) != 0 {
		t.Fatalf("answers = %v, want empty", answers)
	}
}
