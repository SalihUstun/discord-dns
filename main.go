package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const usage = `discord-dns — sadece Discord trafiğini DPI engelinden geçiren arka plan servisi

Kullanım:
  discord-dns              Kur ve arka planda başlat (kuruluysa menü gösterir)
  discord-dns install      Kur / güncelle
  discord-dns status       Durumu göster
  discord-dns uninstall    Kaldır ve DNS ayarlarını eski haline getir
  discord-dns run          Ön planda çalıştır (servis bu komutla başlar; hata ayıklama için)
  discord-dns version      Sürümü göster
  discord-dns license      Lisans bilgilerini göster

Kurulum seçenekleri:
  -v                       Ayrıntılı log tut
`

const defaultFakeIP = "127.0.0.2"

func main() {
	cmd, args := "install", os.Args[1:]
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}

	var err error
	if len(os.Args) == 1 {
		cmd = "launch"
	}
	switch cmd {
	case "launch":
		err = launch()
	case "license", "lisans":
		err = licenseCmd()
	case "version", "-version", "--version":
		fmt.Println("discord-dns", version)
	case "install", "kur":
		err = installCmd(args)
	case "uninstall", "kaldir", "remove":
		err = uninstallCmd(args)
	case "status", "durum":
		err = statusCmd(args)
	case "run":
		err = runCmd(args)
	case "help", "yardim":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "bilinmeyen komut: %s\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nHATA: %v\n", err)
		pauseIfNeeded(args)
		os.Exit(1)
	}
}

type runOptions struct {
	cfg         Config
	noSystemDNS bool
}

func parseRunFlags(args []string) (*runOptions, error) {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	var (
		dnsListen   = fs.String("dns-listen", "127.0.0.1:53,[::1]:53", "DNS dinleme adresleri (ilki zorunlu)")
		fakeIPStr   = fs.String("fake-ip", defaultFakeIP, "Discord domainleri için döndürülecek yerel IP")
		proxyListen = fs.String("proxy-listen", "", "tek bir proxy dinleme adresi (test için; boşsa <fake-ip>:<proxy-ports>)")
		proxyPorts  = fs.String("proxy-ports", "443,2053,2083,2087,2096,8443", "sahte IP'de dinlenecek HTTPS portları")
		upstreams   = fs.String("upstream", "1.1.1.1:53,1.0.0.1:53,8.8.8.8:53,8.8.4.4:53", "Discord dışı sorgular için DNS sunucuları")
		dohList     = fs.String("doh", "https://1.1.1.1/dns-query,https://8.8.8.8/dns-query", "Discord IP'leri için DoH uç noktaları")
		domainsFile = fs.String("domains", domainsFilePath(), "ek domain listesi dosyası (her satıra bir domain)")
		noTLSSplit  = fs.Bool("no-tls-split", false, "TLS kaydı bölmeyi kapat, sadece TCP seviyesinde böl")
		fragDelay   = fs.Duration("frag-delay", 0, "parçalar arasında bekleme (ör. 5ms)")
		verbose     = fs.Bool("v", false, "ayrıntılı log")
		noSysDNS    = fs.Bool("no-system-dns", false, "sistem DNS ayarına dokunma (test için)")
		targetPort  = fs.String("proxy-target-port", "", "gerçek sunucuda hep bu porta git (test için; boşsa gelen bağlantının portu)")
	)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	fakeIP := net.ParseIP(*fakeIPStr).To4()
	if fakeIP == nil {
		return nil, fmt.Errorf("geçersiz -fake-ip: %s", *fakeIPStr)
	}
	var proxyAddrs []string
	if *proxyListen != "" {
		proxyAddrs = []string{*proxyListen}
	} else {
		for _, port := range splitList(*proxyPorts) {
			proxyAddrs = append(proxyAddrs, net.JoinHostPort(fakeIP.String(), port))
		}
	}
	return &runOptions{
		cfg: Config{
			DNSListen:   splitList(*dnsListen),
			FakeIP:      fakeIP,
			ProxyAddrs:  proxyAddrs,
			Upstreams:   splitList(*upstreams),
			DoH:         splitList(*dohList),
			DomainsFile: *domainsFile,
			SplitTLS:    !*noTLSSplit,
			FragDelay:   *fragDelay,
			Verbose:     *verbose,
			TargetPort:  *targetPort,
		},
		noSystemDNS: *noSysDNS,
	}, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func waitFor(timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := fn()
		if err == nil || time.Now().After(deadline) {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
}
