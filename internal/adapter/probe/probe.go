// Package probe measures each network layer without privileges: ICMP echo
// through unprivileged ping sockets (IcmpSendEcho on Windows), and a DNS
// query through the system resolver (ADR 0004).
package probe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

// Config holds the probe targets and timings.
type Config struct {
	InternetTargets []string      // IPv4 addresses; the internet is up if any answers
	DNSZone         string        // zone queried with a random label to bypass resolver caches
	DNSEvery        time.Duration // the DNS layer is re-measured at most this often
	Timeout         time.Duration // per-layer timeout
}

// DefaultConfig is the default target set.
func DefaultConfig() Config {
	return Config{
		InternetTargets: []string{"1.1.1.1", "8.8.8.8"},
		DNSZone:         "example.com",
		DNSEvery:        10 * time.Second,
		Timeout:         time.Second,
	}
}

// Prober implements app.Prober.
type Prober struct {
	cfg      Config
	resolver *net.Resolver
	route    func() (route, error)

	lastDNS   bool
	lastDNSAt time.Time
}

// New checks that unprivileged ICMP is available and returns a prober.
func New(cfg Config) (*Prober, error) {
	if err := checkICMP(); err != nil {
		return nil, fmt.Errorf("unprivileged ICMP is not available: %w", err)
	}
	return &Prober{
		cfg:      cfg,
		resolver: &net.Resolver{PreferGo: true},
		route:    defaultRoute,
		lastDNS:  true,
	}, nil
}

// Probe measures every layer. A layer is only measured when the closer ones
// answer, since a failing link makes every farther result meaningless.
func (p *Prober) Probe(ctx context.Context) domain.Sample {
	s := domain.Sample{At: time.Now()}

	r, err := p.route()
	if err != nil || !r.linkUp() {
		return s
	}
	s.Link = true

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.Router = ping(ctx, r.gateway.String(), p.cfg.Timeout) }()
	go func() { defer wg.Done(); s.Internet = p.internetUp(ctx) }()
	wg.Wait()

	s.DNS = p.dnsUp(ctx, s)
	return s
}

func (p *Prober) internetUp(ctx context.Context) bool {
	results := make(chan bool, len(p.cfg.InternetTargets))
	for _, target := range p.cfg.InternetTargets {
		go func() { results <- ping(ctx, target, p.cfg.Timeout) }()
	}
	up := false
	for range p.cfg.InternetTargets {
		if <-results {
			up = true
		}
	}
	return up
}

// dnsUp re-measures DNS at most every DNSEvery and only while the internet
// answers; otherwise it reports the previous result.
func (p *Prober) dnsUp(ctx context.Context, s domain.Sample) bool {
	if !s.Internet || s.At.Sub(p.lastDNSAt) < p.cfg.DNSEvery {
		return p.lastDNS
	}
	p.lastDNSAt = s.At
	p.lastDNS = p.resolves(ctx)
	return p.lastDNS
}

// resolves queries a random, never cached label: the resolver must forward it,
// and NXDOMAIN proves it answered.
func (p *Prober) resolves(ctx context.Context) bool {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, 2*p.cfg.Timeout)
	defer cancel()

	_, err := p.resolver.LookupHost(ctx, "netmon-"+hex.EncodeToString(b[:])+"."+p.cfg.DNSZone)
	var dnsErr *net.DNSError
	return err == nil || (errors.As(err, &dnsErr) && dnsErr.IsNotFound)
}
