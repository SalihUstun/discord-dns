package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DoHResolver struct {
	endpoints []string
	client    *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	ips []net.IP
	exp time.Time
}

func NewDoHResolver(endpoints []string) *DoHResolver {
	tr := &http.Transport{
		Proxy:               nil,
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 5 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		MaxIdleConnsPerHost: 4,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &DoHResolver{
		endpoints: endpoints,
		client:    &http.Client{Transport: tr, Timeout: 6 * time.Second},
		cache:     make(map[string]cacheEntry),
	}
}

func (r *DoHResolver) LookupA(ctx context.Context, host string) ([]net.IP, error) {
	name := dns.Fqdn(normalize(host))

	r.mu.Lock()
	if e, ok := r.cache[name]; ok && time.Now().Before(e.exp) {
		r.mu.Unlock()
		return e.ips, nil
	}
	r.mu.Unlock()

	q := new(dns.Msg)
	q.SetQuestion(name, dns.TypeA)
	q.Id = 0
	wire, err := q.Pack()
	if err != nil {
		return nil, err
	}

	if len(r.endpoints) == 0 {
		return nil, errors.New("DoH uç noktası tanımlı değil")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type result struct {
		ips []net.IP
		ttl time.Duration
		err error
	}
	results := make(chan result, len(r.endpoints))
	for _, ep := range r.endpoints {
		go func(ep string) {
			ips, ttl, err := r.query(ctx, ep, wire)
			if err == nil && len(ips) == 0 {
				err = fmt.Errorf("%s için A kaydı yok", name)
			}
			if err != nil {
				err = fmt.Errorf("%s: %w", ep, err)
			}
			results <- result{ips, ttl, err}
		}(ep)
	}

	var lastErr error
	for range r.endpoints {
		res := <-results
		if res.err != nil {
			lastErr = res.err
			continue
		}
		r.mu.Lock()
		r.cache[name] = cacheEntry{ips: res.ips, exp: time.Now().Add(res.ttl)}
		r.mu.Unlock()
		return res.ips, nil
	}
	return nil, lastErr
}

func (r *DoHResolver) Exchange(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	q := req.Copy()
	q.Id = 0
	wire, err := q.Pack()
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, ep := range r.endpoints {
		m, err := r.exchange(ctx, ep, wire)
		if err == nil {
			m.Id = req.Id
			return m, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	return nil, lastErr
}

func (r *DoHResolver) exchange(ctx context.Context, endpoint string, wire []byte) (*dns.Msg, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}

	m := new(dns.Msg)
	if err := m.Unpack(body); err != nil {
		return nil, err
	}
	return m, nil
}

func (r *DoHResolver) query(ctx context.Context, endpoint string, wire []byte) ([]net.IP, time.Duration, error) {
	m, err := r.exchange(ctx, endpoint, wire)
	if err != nil {
		return nil, 0, err
	}
	if m.Rcode != dns.RcodeSuccess {
		return nil, 0, fmt.Errorf("rcode %s", dns.RcodeToString[m.Rcode])
	}

	var ips []net.IP
	minTTL := uint32(3600)
	for _, rr := range m.Answer {
		if a, ok := rr.(*dns.A); ok {
			ips = append(ips, a.A)
			if a.Hdr.Ttl < minTTL {
				minTTL = a.Hdr.Ttl
			}
		}
	}
	if minTTL < 60 {
		minTTL = 60
	}
	return ips, time.Duration(minTTL) * time.Second, nil
}
