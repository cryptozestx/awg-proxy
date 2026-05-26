package main

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const minDomainBypassTTL = 10 * time.Minute

type DomainBypassConfig struct {
	ListenAddr string
	Upstream   string
	Rules      TunnelRules
	Routes     DynamicBypassRoutes
}

type DNSAnswer struct {
	Name string
	A    []netip.Addr
	TTL  time.Duration
}

type DomainBypassRuntime struct {
	mu     sync.Mutex
	server *dns.Server
	addr   string
	config DomainBypassConfig
	done   chan error
	ctx    context.Context
	cancel context.CancelFunc
}

func NewDomainBypassRuntime() *DomainBypassRuntime {
	return &DomainBypassRuntime{}
}

func (r *DomainBypassRuntime) Start(ctx context.Context, config DomainBypassConfig) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if config.ListenAddr == "" {
		config.ListenAddr = "127.0.0.1:0"
	}
	if config.Upstream == "" {
		return fmt.Errorf("domain bypass upstream is empty")
	}

	packetConn, err := net.ListenPacket("udp", config.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen domain bypass DNS on %s: %w", config.ListenAddr, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.server != nil {
		if err := packetConn.Close(); err != nil {
			return fmt.Errorf("close unused domain bypass listener: %w", err)
		}
		return fmt.Errorf("domain bypass runtime already started")
	}

	runCtx, cancel := context.WithCancel(ctx)
	r.config = config
	r.ctx = runCtx
	r.cancel = cancel
	r.addr = packetConn.LocalAddr().String()
	r.done = make(chan error, 1)
	r.server = &dns.Server{PacketConn: packetConn, Handler: dns.HandlerFunc(r.handleDNS)}

	go func(server *dns.Server, done chan<- error) {
		done <- server.ActivateAndServe()
	}(r.server, r.done)

	go func() {
		<-runCtx.Done()
		_ = r.Close()
	}()

	return nil
}

func (r *DomainBypassRuntime) Addr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addr
}

func (r *DomainBypassRuntime) Close() error {
	r.mu.Lock()
	server := r.server
	done := r.done
	cancel := r.cancel
	r.server = nil
	r.done = nil
	r.cancel = nil
	r.addr = ""
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if server == nil {
		return nil
	}
	if err := server.Shutdown(); err != nil {
		return fmt.Errorf("shutdown domain bypass DNS: %w", err)
	}
	if done == nil {
		return nil
	}
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("domain bypass DNS server failed: %w", err)
		}
	case <-time.After(time.Second):
		return fmt.Errorf("domain bypass DNS server did not stop")
	}
	return nil
}

func (r *DomainBypassRuntime) HandleAnswer(ctx context.Context, rules TunnelRules, answer DNSAnswer, routes DynamicBypassRoutes) error {
	if routes == nil {
		return nil
	}
	if !domainRulesMatch(rules.DomainRules, answer.Name) {
		return nil
	}
	ttl := answer.TTL
	if ttl < minDomainBypassTTL {
		ttl = minDomainBypassTTL
	}
	for _, addr := range answer.A {
		if !addr.Is4() {
			continue
		}
		if err := routes.AddBypassRoute(ctx, netip.PrefixFrom(addr, 32), "dns:"+normalizeDomainPattern(answer.Name), ttl); err != nil {
			return err
		}
	}
	return nil
}

func domainRulesMatch(rules []DomainRule, host string) bool {
	for _, rule := range rules {
		if rule.Matches(host) {
			return true
		}
	}
	return false
}

func (r *DomainBypassRuntime) handleDNS(w dns.ResponseWriter, req *dns.Msg) {
	r.mu.Lock()
	config := r.config
	ctx := r.ctx
	r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	client := &dns.Client{Net: "udp"}
	resp, _, err := client.Exchange(req, config.Upstream)
	if err != nil {
		_ = w.WriteMsg(serverFailure(req))
		return
	}

	answers := collectDNSAAnswers(resp)
	for _, answer := range answers {
		if err := r.HandleAnswer(ctx, config.Rules, answer, config.Routes); err != nil {
			_ = w.WriteMsg(serverFailure(req))
			return
		}
	}

	_ = w.WriteMsg(resp)
}

func collectDNSAAnswers(resp *dns.Msg) []DNSAnswer {
	if resp == nil {
		return nil
	}

	type collected struct {
		addrs []netip.Addr
		ttl   uint32
	}
	byName := make(map[string]collected)
	order := make([]string, 0)
	for _, rr := range resp.Answer {
		a, ok := rr.(*dns.A)
		if !ok {
			continue
		}
		addr, ok := netip.AddrFromSlice(a.A.To4())
		if !ok {
			continue
		}
		name := normalizeDomainPattern(a.Hdr.Name)
		entry, exists := byName[name]
		if !exists {
			entry.ttl = math.MaxUint32
			order = append(order, name)
		}
		entry.addrs = append(entry.addrs, addr)
		if a.Hdr.Ttl < entry.ttl {
			entry.ttl = a.Hdr.Ttl
		}
		byName[name] = entry
	}

	answers := make([]DNSAnswer, 0, len(order))
	for _, name := range order {
		entry := byName[name]
		answers = append(answers, DNSAnswer{
			Name: name,
			A:    entry.addrs,
			TTL:  time.Duration(entry.ttl) * time.Second,
		})
	}
	return answers
}

func serverFailure(req *dns.Msg) *dns.Msg {
	resp := new(dns.Msg)
	resp.SetRcode(req, dns.RcodeServerFailure)
	return resp
}
