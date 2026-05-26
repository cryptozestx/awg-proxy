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

type dnsARecord struct {
	addr netip.Addr
	ttl  uint32
}

type dnsCNAMERecord struct {
	target string
	ttl    uint32
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
	ready := make(chan struct{})
	r.config = config
	r.ctx = runCtx
	r.cancel = cancel
	r.addr = packetConn.LocalAddr().String()
	r.done = make(chan error, 1)
	r.server = &dns.Server{
		PacketConn: packetConn,
		Handler:    dns.HandlerFunc(r.handleDNS),
		NotifyStartedFunc: func() {
			close(ready)
		},
	}
	done := r.done

	go func(server *dns.Server, done chan<- error) {
		done <- server.ActivateAndServe()
	}(r.server, done)

	go func() {
		<-runCtx.Done()
		_ = r.Close()
	}()

	select {
	case <-ready:
		return nil
	case err := <-done:
		r.server = nil
		r.done = nil
		r.cancel = nil
		r.addr = ""
		cancel()
		if err != nil {
			return fmt.Errorf("start domain bypass DNS server: %w", err)
		}
		return fmt.Errorf("domain bypass DNS server stopped before startup")
	case <-ctx.Done():
		r.mu.Unlock()
		err := r.Close()
		r.mu.Lock()
		if err != nil {
			return err
		}
		return ctx.Err()
	}
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
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	if err := server.ShutdownContext(shutdownCtx); err != nil {
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
	if req == nil {
		if w != nil {
			_ = w.WriteMsg(serverFailure(nil))
		}
		return
	}

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

	answers := collectDNSAAnswersForQuestions(req, resp, config.Rules)
	for _, answer := range answers {
		if err := r.HandleAnswer(ctx, config.Rules, answer, config.Routes); err != nil {
			_ = w.WriteMsg(serverFailure(req))
			return
		}
	}

	_ = w.WriteMsg(resp)
}

func collectDNSAAnswersForQuestions(req *dns.Msg, resp *dns.Msg, rules TunnelRules) []DNSAnswer {
	if req == nil || resp == nil {
		return nil
	}

	aRecords := make(map[string][]dnsARecord)
	cnameRecords := make(map[string][]dnsCNAMERecord)
	for _, rr := range resp.Answer {
		switch rr := rr.(type) {
		case *dns.A:
			addr, ok := netip.AddrFromSlice(rr.A.To4())
			if !ok {
				continue
			}
			name := normalizeDomainPattern(rr.Hdr.Name)
			aRecords[name] = append(aRecords[name], dnsARecord{addr: addr, ttl: rr.Hdr.Ttl})
		case *dns.CNAME:
			name := normalizeDomainPattern(rr.Hdr.Name)
			target := normalizeDomainPattern(rr.Target)
			if name == "" || target == "" {
				continue
			}
			cnameRecords[name] = append(cnameRecords[name], dnsCNAMERecord{target: target, ttl: rr.Hdr.Ttl})
		}
	}

	answers := make([]DNSAnswer, 0)
	seenQuestions := make(map[string]bool)
	for _, question := range req.Question {
		queryName := normalizeDomainPattern(question.Name)
		if queryName == "" || seenQuestions[queryName] || !domainRulesMatch(rules.DomainRules, queryName) {
			continue
		}
		seenQuestions[queryName] = true

		addrs, ttl, ok := collectLinkedARecords(queryName, aRecords, cnameRecords)
		if !ok {
			continue
		}
		answers = append(answers, DNSAnswer{
			Name: queryName,
			A:    addrs,
			TTL:  time.Duration(ttl) * time.Second,
		})
	}
	return answers
}

func collectLinkedARecords(queryName string, aRecords map[string][]dnsARecord, cnameRecords map[string][]dnsCNAMERecord) ([]netip.Addr, uint32, bool) {
	current := queryName
	minTTL := uint32(math.MaxUint32)
	addrs := make([]netip.Addr, 0)
	seenNames := make(map[string]bool)
	for current != "" && !seenNames[current] {
		seenNames[current] = true

		for _, record := range aRecords[current] {
			addrs = append(addrs, record.addr)
			if record.ttl < minTTL {
				minTTL = record.ttl
			}
		}

		cnames := cnameRecords[current]
		if len(cnames) == 0 {
			break
		}
		if cnames[0].ttl < minTTL {
			minTTL = cnames[0].ttl
		}
		current = cnames[0].target
	}
	if len(addrs) == 0 {
		return nil, 0, false
	}
	return addrs, minTTL, true
}

func serverFailure(req *dns.Msg) *dns.Msg {
	resp := new(dns.Msg)
	if req == nil {
		resp.Rcode = dns.RcodeServerFailure
		return resp
	}
	resp.SetRcode(req, dns.RcodeServerFailure)
	return resp
}
