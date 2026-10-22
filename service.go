package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kardianos/service"
)

const serviceName = "discord-dns"

type program struct {
	opts *runOptions
	srv  *Server
	stop chan struct{}
	done chan struct{}
}

func (p *program) Start(s service.Service) error {
	srv, err := StartServer(p.opts.cfg)
	if err != nil {
		log.Printf("başlatılamadı: %v", err)
		return err
	}
	p.srv = srv
	if !p.opts.noSystemDNS {
		p.stop = make(chan struct{})
		p.done = make(chan struct{})
		go p.manageSystemDNS()
	}
	return nil
}

func (p *program) manageSystemDNS() {
	defer close(p.done)
	apply := func() {
		if err := applySystemDNS(); err != nil {
			log.Printf("sistem DNS'i ayarlanamadı: %v", err)
		}
	}
	apply()
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			apply()
		}
	}
}

func (p *program) Stop(s service.Service) error {
	if p.stop != nil {
		close(p.stop)
		<-p.done
		if err := restoreSystemDNS(); err != nil {
			log.Printf("sistem DNS'i geri yüklenemedi: %v", err)
		} else {
			log.Printf("sistem DNS'i eski haline getirildi")
		}
	}
	if p.srv != nil {
		p.srv.Close()
	}
	log.Printf("durduruldu")
	return nil
}

func serviceConfig(runArgs []string) *service.Config {
	return &service.Config{
		Name:        serviceName,
		DisplayName: "Discord DNS",
		Description: "Sadece Discord trafiğini DPI engelinden geçiren yerel DNS ve proxy.",
		Executable:  installedExePath(),
		Arguments:   append([]string{"run"}, runArgs...),
		Option: service.KeyValue{

			"RunAtLoad": true,
			"KeepAlive": true,

			"OnFailure":              "restart",
			"OnFailureDelayDuration": "2s",
			"OnFailureResetPeriod":   60,
		},
	}
}

func newService(runArgs []string) (service.Service, error) {
	return service.New(&program{}, serviceConfig(runArgs))
}

func runCmd(args []string) error {
	opts, err := parseRunFlags(args)
	if err != nil {
		return err
	}
	if !service.Interactive() {
		setupFileLog()
	}
	s, err := service.New(&program{opts: opts}, serviceConfig(args))
	if err != nil {
		return err
	}
	return s.Run()
}

func installCmd(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	verbose := fs.Bool("v", false, "ayrıntılı log")
	fs.Bool("pause", false, "bitince Enter bekle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !isAdmin() {
		return elevate(append([]string{"install"}, args...))
	}

	fmt.Println("==> discord-dns kuruluyor")
	var runArgs []string
	if *verbose {
		runArgs = append(runArgs, "-v")
	}
	s, err := newService(runArgs)
	if err != nil {
		return err
	}

	removeLegacy()
	if _, err := s.Status(); !errors.Is(err, service.ErrNotInstalled) {
		fmt.Println("    önceki kurulum durduruluyor")
		s.Stop()
		s.Uninstall()
	}

	fmt.Println("    dosyalar kopyalanıyor:", installedExePath())
	if err := waitFor(15*time.Second, func() error { return copySelf(installedExePath()) }); err != nil {
		return fmt.Errorf("program kopyalanamadı: %w", err)
	}
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(domainsFilePath()); errors.Is(err, os.ErrNotExist) {
		os.WriteFile(domainsFilePath(), []byte(domainsFileTemplate), 0o644)
	}

	fmt.Println("    servis kaydediliyor")
	if err := s.Install(); err != nil {
		return fmt.Errorf("servis kurulamadı: %w", err)
	}
	fmt.Println("    servis başlatılıyor")
	if err := s.Start(); err != nil {
		return fmt.Errorf("servis başlatılamadı: %w", err)
	}

	fakeIP := net.ParseIP(defaultFakeIP)
	if err := waitFor(15*time.Second, func() error { return checkDNS("127.0.0.1:53", fakeIP) }); err != nil {
		printLogTail(15)
		return fmt.Errorf("servis cevap vermiyor (%v); log: %s", err, logFilePath())
	}

	time.Sleep(2 * time.Second)

	fmt.Println()
	fmt.Println("Kurulum tamam. discord-dns arka planda çalışıyor ve açılışta otomatik başlayacak.")
	fmt.Println("Sistem DNS'i:")
	for _, line := range describeSystemDNS() {
		fmt.Println("    " + line)
	}
	fmt.Println()
	fmt.Println("Discord açıksa tamamen kapatıp yeniden aç.")
	fmt.Printf("Durum:     %s status\n", installedExePath())
	fmt.Printf("Kaldırmak: %s uninstall\n", installedExePath())
	pauseIfNeeded(args)
	return nil
}

func uninstallCmd(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.Bool("pause", false, "bitince Enter bekle")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !isAdmin() {
		return elevate(append([]string{"uninstall"}, args...))
	}

	fmt.Println("==> discord-dns kaldırılıyor")
	s, err := newService(nil)
	if err != nil {
		return err
	}
	if _, err := s.Status(); !errors.Is(err, service.ErrNotInstalled) {
		fmt.Println("    servis durduruluyor")
		s.Stop()
	}

	fmt.Println("    DNS ayarları geri yükleniyor")
	if err := restoreSystemDNS(); err != nil {
		fmt.Println("    uyarı:", err)
	}
	if err := s.Uninstall(); err != nil && !errors.Is(err, service.ErrNotInstalled) {
		fmt.Println("    uyarı:", err)
	}
	removeLegacy()
	removeLoopbackAlias(defaultFakeIP)
	flushDNSCache()
	os.RemoveAll(stateDir())
	os.Remove(logFilePath())
	os.Remove(logFilePath() + ".old")
	if err := removeInstalledExe(); err != nil {
		fmt.Println("    uyarı:", err)
	}

	fmt.Println()
	fmt.Println("Kaldırıldı. DNS ayarların kurulum öncesi haline döndü.")
	pauseIfNeeded(args)
	return nil
}

func statusCmd(args []string) error {
	s, err := newService(nil)
	if err != nil {
		return err
	}
	installed := true
	if _, err := os.Stat(installedExePath()); err != nil {
		installed = false
	}
	if _, err := s.Status(); errors.Is(err, service.ErrNotInstalled) {
		installed = false
	}

	fmt.Println("discord-dns", version, "durumu")
	if !installed {
		fmt.Println("  Kurulu değil. Kurmak için programı çalıştır.")
		pauseIfNeeded(args)
		return nil
	}
	if err := checkDNS("127.0.0.1:53", net.ParseIP(defaultFakeIP)); err != nil {
		fmt.Printf("  Servis:  ÇALIŞMIYOR (%v)\n", err)
	} else {
		fmt.Println("  Servis:  çalışıyor")
	}
	fmt.Println("  Sistem DNS'i:")
	for _, line := range describeSystemDNS() {
		fmt.Println("    " + line)
	}
	fmt.Println("  Log:", logFilePath())
	printLogTail(15)
	pauseIfNeeded(args)
	return nil
}

const domainsFileTemplate = `# Ek domain listesi: buradaki domainler de (alt alan adlarıyla birlikte) bypass'tan geçer.
# Discord'un ana domainleri zaten programın içinde tanımlı; buraya sadece ekstra eklemen gerekenleri yaz.
# Her satıra bir domain. Değişiklikten sonra programı tekrar çalıştır (servis yeniden başlar).
`

func domainsFilePath() string { return filepath.Join(stateDir(), "domains.txt") }
func dnsStatePath() string    { return filepath.Join(stateDir(), "dns-state.json") }

func copySelf(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	if r, err := filepath.EvalSymlinks(src); err == nil {
		src = r
	}
	if samePath(src, dst) {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func samePath(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func setupFileLog() {
	path := logFilePath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if fi, err := os.Stat(path); err == nil && fi.Size() > 5<<20 {
		os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}

func printLogTail(n int) {
	f, err := os.Open(logFilePath())
	if err != nil {
		return
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Println("  Son log satırları:")
	for _, l := range lines {
		fmt.Println("    " + l)
	}
}

func pauseIfNeeded(args []string) {
	pause := runtime.GOOS == "windows" && len(os.Args) == 1
	for _, a := range args {
		if a == "-pause" || a == "--pause" {
			pause = true
		}
	}
	if !pause {
		return
	}
	fmt.Print("\nKapatmak için Enter'a bas...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
