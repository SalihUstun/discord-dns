package main

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type DNSServer struct {
	matcher   *Matcher
	fakeIP    net.IP
	upstreams []string
	doh       *DoHResolver
	udp       *dns.Client
	tcp       *dns.Client
	verbose   bool

	mu    sync.Mutex
	cache map[string]cachedMsg
}

type cachedMsg struct {
	msg *dns.Msg
	exp time.Time
}

const (
	forwardTimeout = 4 * time.Second
	maxCacheTTL    = 300 * time.Second
	maxCacheSize   = 10000
)

func NewDNSServer(matcher *Matcher, fakeIP net.IP, upstreams []string, doh *DoHResolver, verbose bool) *DNSServer {
	return &DNSServer{
		matcher:   matcher,
		fakeIP:    fakeIP,
		upstreams: upstreams,
		doh:       doh,
		udp:       &dns.Client{Net: "udp", Timeout: forwardTimeout, UDPSize: dns.DefaultMsgSize},
		tcp:       &dns.Client{Net: "tcp", Timeout: forwardTimeout},
		verbose:   verbose,
		cache:     make(map[string]cachedMsg),
	}
}

func (s *DNSServer) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 1 && s.matcher.Match(req.Question[0].Name) {
		s.answerFake(w, req)
		return
	}
	s.forward(w, req)
}

func (s *DNSServer) answerFake(w dns.ResponseWriter, req *dns.Msg) {
	q := req.Question[0]
	m := new(dns.Msg)
	m.SetReply(req)
	m.RecursionAvailable = true
	answer := "boş"
	if q.Qtype == dns.TypeA && q.Qclass == dns.ClassINET {
		m.Answer = append(m.Answer, &dns.A{
			Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
			A:   s.fakeIP,
		})
		answer = s.fakeIP.String()
	}
	if s.verbose {
		log.Printf("[dns] discord  %s %s -> %s", dns.TypeToString[q.Qtype], q.Name, answer)
	}
	w.WriteMsg(m)
}

func (s *DNSServer) forward(w dns.ResponseWriter, req *dns.Msg) {
	_, isTCP := w.RemoteAddr().(*net.TCPAddr)

	key := cacheKey(req)
	if key != "" {
		if m := s.cacheGet(key); m != nil {
			m.Id = req.Id
			s.write(w, req, m, isTCP)
			return
		}
	}

	resp := s.race(req, isTCP)
	if resp == nil {
		m := new(dns.Msg)
		m.SetRcode(req, dns.RcodeServerFailure)
		w.WriteMsg(m)
		return
	}
	resp.Id = req.Id
	if key != "" && !resp.Truncated {
		s.cachePut(key, resp)
	}
	s.write(w, req, resp, isTCP)
}

func (s *DNSServer) race(req *dns.Msg, isTCP bool) *dns.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), forwardTimeout)
	defer cancel()

	client := s.udp
	if isTCP {
		client = s.tcp
	}

	n := len(s.upstreams)
	if s.doh != nil {
		n++
	}
	results := make(chan *dns.Msg, n)

	for _, up := range s.upstreams {
		go func(up string) {
			r, _, err := client.ExchangeContext(ctx, req.Copy(), up)
			if err != nil {
				if s.verbose && ctx.Err() == nil {
					log.Printf("[dns] upstream %s hata: %v", up, err)
				}
				r = nil
			}
			results <- usable(r)
		}(up)
	}
	if s.doh != nil {
		go func() {
			r, err := s.doh.Exchange(ctx, req)
			if err != nil {
				if s.verbose && ctx.Err() == nil {
					log.Printf("[dns] DoH hata: %v", err)
				}
				r = nil
			}
			results <- usable(r)
		}()
	}

	for i := 0; i < n; i++ {
		select {
		case r := <-results:
			if r != nil {
				return r
			}
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func usable(r *dns.Msg) *dns.Msg {
	if r == nil {
		return nil
	}
	if r.Rcode == dns.RcodeSuccess || r.Rcode == dns.RcodeNameError {
		return r
	}
	return nil
}

func (s *DNSServer) write(w dns.ResponseWriter, req, resp *dns.Msg, isTCP bool) {
	if !isTCP {

		size := dns.MinMsgSize
		if opt := req.IsEdns0(); opt != nil {
			size = int(opt.UDPSize())
		}
		resp.Truncate(size)
	}
	w.WriteMsg(resp)
}

func cacheKey(req *dns.Msg) string {
	if len(req.Question) != 1 {
		return ""
	}
	q := req.Question[0]
	do := ""
	if opt := req.IsEdns0(); opt != nil && opt.Do() {
		do = "|do"
	}
	return strings.ToLower(q.Name) + "|" + dns.TypeToString[q.Qtype] + "|" + dns.ClassToString[q.Qclass] + do
}

func (s *DNSServer) cacheGet(key string) *dns.Msg {
	s.mu.Lock()
	e, ok := s.cache[key]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	remaining := time.Until(e.exp)
	if remaining <= 0 {
		return nil
	}
	m := e.msg.Copy()
	ttl := uint32(remaining / time.Second)
	if ttl == 0 {
		ttl = 1
	}
	for _, sec := range [][]dns.RR{m.Answer, m.Ns, m.Extra} {
		for _, rr := range sec {
			if rr.Header().Rrtype != dns.TypeOPT && rr.Header().Ttl > ttl {
				rr.Header().Ttl = ttl
			}
		}
	}
	return m
}

func (s *DNSServer) cachePut(key string, m *dns.Msg) {
	ttl := maxCacheTTL
	found := false
	for _, sec := range [][]dns.RR{m.Answer, m.Ns} {
		for _, rr := range sec {
			t := time.Duration(rr.Header().Ttl) * time.Second
			if soa, ok := rr.(*dns.SOA); ok && time.Duration(soa.Minttl)*time.Second < t {
				t = time.Duration(soa.Minttl) * time.Second
			}
			if t < ttl {
				ttl = t
			}
			found = true
		}
	}
	if !found {
		ttl = 30 * time.Second
	}
	if ttl < 5*time.Second {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= maxCacheSize {
		now := time.Now()
		for k, e := range s.cache {
			if now.After(e.exp) {
				delete(s.cache, k)
			}
		}
		if len(s.cache) >= maxCacheSize {
			s.cache = make(map[string]cachedMsg)
		}
	}
	s.cache[key] = cachedMsg{msg: m.Copy(), exp: time.Now().Add(ttl)}
}
