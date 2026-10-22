package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"time"

	"github.com/miekg/dns"
)

type Config struct {
	DNSListen   []string
	FakeIP      net.IP
	ProxyAddrs  []string
	Upstreams   []string
	DoH         []string
	DomainsFile string
	SplitTLS    bool
	FragDelay   time.Duration
	Verbose     bool
	TargetPort  string
}

type Server struct {
	dnsServers []*dns.Server
	listeners  []net.Listener
}

func StartServer(cfg Config) (*Server, error) {
	domains := append([]string{}, defaultDomains...)
	if cfg.DomainsFile != "" {
		extra, err := loadDomainsFile(cfg.DomainsFile)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			log.Printf("domain dosyası okunamadı (%s): %v", cfg.DomainsFile, err)
		}
		domains = append(domains, extra...)
	}
	matcher := NewMatcher(domains)
	resolver := NewDoHResolver(cfg.DoH)

	s := &Server{}
	proxy := &Proxy{
		matcher:    matcher,
		resolver:   resolver,
		splitTLS:   cfg.SplitTLS,
		fragDelay:  cfg.FragDelay,
		verbose:    cfg.Verbose,
		targetPort: cfg.TargetPort,
	}
	for _, addr := range cfg.ProxyAddrs {
		ln, err := listenTCP(addr)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("proxy %s dinlenemedi: %w", addr, err)
		}
		s.listeners = append(s.listeners, ln)
		go func() {
			if err := proxy.Serve(ln); err != nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("proxy %s durdu: %v", ln.Addr(), err)
			}
		}()
	}

	handler := NewDNSServer(matcher, cfg.FakeIP, cfg.Upstreams, resolver, cfg.Verbose)
	for i, addr := range cfg.DNSListen {
		pc, err := net.ListenPacket("udp", addr)
		if err != nil {
			if i == 0 {
				s.Close()
				return nil, fmt.Errorf("DNS %s (udp) dinlenemedi: %w", addr, err)
			}
			log.Printf("DNS %s atlandı: %v", addr, err)
			continue
		}
		tl, err := net.Listen("tcp", addr)
		if err != nil {
			pc.Close()
			if i == 0 {
				s.Close()
				return nil, fmt.Errorf("DNS %s (tcp) dinlenemedi: %w", addr, err)
			}
			log.Printf("DNS %s atlandı: %v", addr, err)
			continue
		}
		for _, srv := range []*dns.Server{
			{PacketConn: pc, Handler: handler},
			{Listener: tl, Handler: handler},
		} {
			s.dnsServers = append(s.dnsServers, srv)
			go func() {
				if err := srv.ActivateAndServe(); err != nil {
					log.Printf("DNS sunucusu durdu: %v", err)
				}
			}()
		}
	}

	log.Printf("discord-dns çalışıyor: DNS %v, proxy %v, %d domain", cfg.DNSListen, cfg.ProxyAddrs, len(domains))
	return s, nil
}

func (s *Server) Close() {
	for _, d := range s.dnsServers {
		d.Shutdown()
	}
	for _, ln := range s.listeners {
		ln.Close()
	}
}

func listenTCP(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp4", addr)
	if err == nil {
		return ln, nil
	}
	host, _, _ := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() || host == "127.0.0.1" {
		return nil, err
	}
	if aerr := ensureLoopbackAlias(host); aerr != nil {
		log.Printf("loopback alias %s eklenemedi: %v", host, aerr)
		return nil, err
	}
	time.Sleep(200 * time.Millisecond)
	return net.Listen("tcp4", addr)
}

func checkDNS(addr string, fakeIP net.IP) error {
	m := new(dns.Msg)
	m.SetQuestion("discord.com.", dns.TypeA)
	c := &dns.Client{Timeout: 2 * time.Second}
	r, _, err := c.Exchange(m, addr)
	if err != nil {
		return err
	}
	for _, rr := range r.Answer {
		if a, ok := rr.(*dns.A); ok && a.A.Equal(fakeIP) {
			return nil
		}
	}
	return fmt.Errorf("beklenmeyen cevap: %v", r.Answer)
}
