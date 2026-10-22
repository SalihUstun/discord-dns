package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func installedExePath() string { return "/usr/local/bin/discord-dns" }
func stateDir() string         { return "/usr/local/etc/discord-dns" }
func logFilePath() string      { return "/var/log/discord-dns.log" }

func isAdmin() bool { return os.Geteuid() == 0 }

func elevate(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	fmt.Println("Yönetici yetkisi gerekiyor, Mac şifreni gir:")
	cmd := exec.Command("sudo", append([]string{exe}, args...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}

func ensureLoopbackAlias(ip string) error {
	out, err := exec.Command("/sbin/ifconfig", "lo0", "alias", ip, "up").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func removeLoopbackAlias(ip string) {
	exec.Command("/sbin/ifconfig", "lo0", "-alias", ip).Run()
}

func removeInstalledExe() error {
	err := os.Remove(installedExePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func flushDNSCache() {
	exec.Command("dscacheutil", "-flushcache").Run()
	exec.Command("killall", "-HUP", "mDNSResponder").Run()
}

const legacyPlist = "/Library/LaunchDaemons/com.discorddns.plist"

func removeLegacy() {
	if _, err := os.Stat(legacyPlist); err != nil {
		return
	}
	fmt.Println("    eski kurulum (install.sh) kaldırılıyor")
	exec.Command("launchctl", "bootout", "system", legacyPlist).Run()
	os.Remove(legacyPlist)
}

var sysDNSMu sync.Mutex

type dnsState map[string][]string

func networkServices() ([]string, error) {
	out, err := exec.Command("networksetup", "-listallnetworkservices").Output()
	if err != nil {
		return nil, err
	}
	var svcs []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			continue
		}
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		svcs = append(svcs, line)
	}
	return svcs, nil
}

func getServiceDNS(svc string) []string {
	out, err := exec.Command("networksetup", "-getdnsservers", svc).Output()
	if err != nil || strings.Contains(string(out), "aren't any") {
		return nil
	}
	return strings.Fields(string(out))
}

func setServiceDNS(svc string, servers []string) error {
	if len(servers) == 0 {
		servers = []string{"Empty"}
	}
	out, err := exec.Command("networksetup", append([]string{"-setdnsservers", svc}, servers...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v %s", svc, err, out)
	}
	return nil
}

func isOurs(servers []string) bool {
	return len(servers) == 1 && servers[0] == "127.0.0.1"
}

func loadDNSState() dnsState {
	st := dnsState{}
	if data, err := os.ReadFile(dnsStatePath()); err == nil {
		json.Unmarshal(data, &st)
		return st
	}

	legacy := filepath.Join(stateDir(), "prev-dns.txt")
	if data, err := os.ReadFile(legacy); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			svc, servers, ok := strings.Cut(line, "\t")
			if !ok || svc == "" {
				continue
			}
			f := strings.Fields(servers)
			if len(f) == 1 && f[0] == "Empty" {
				f = nil
			}
			st[svc] = f
		}
	}
	return st
}

func saveDNSState(st dnsState) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(dnsStatePath(), data, 0o644); err != nil {
		return err
	}
	os.Remove(filepath.Join(stateDir(), "prev-dns.txt"))
	return nil
}

func applySystemDNS() error {
	sysDNSMu.Lock()
	defer sysDNSMu.Unlock()

	svcs, err := networkServices()
	if err != nil {
		return err
	}
	st := loadDNSState()
	var errs []error
	changed := false
	for _, svc := range svcs {
		cur := getServiceDNS(svc)
		if isOurs(cur) {
			continue
		}
		st[svc] = cur
		if err := setServiceDNS(svc, []string{"127.0.0.1"}); err != nil {
			errs = append(errs, err)
			continue
		}
		changed = true
	}
	if err := saveDNSState(st); err != nil {
		errs = append(errs, err)
	}
	if changed {
		flushDNSCache()
	}
	return errors.Join(errs...)
}

func restoreSystemDNS() error {
	sysDNSMu.Lock()
	defer sysDNSMu.Unlock()

	svcs, err := networkServices()
	if err != nil {
		return err
	}
	st := loadDNSState()
	var errs []error
	for _, svc := range svcs {
		if !isOurs(getServiceDNS(svc)) {
			continue
		}
		if err := setServiceDNS(svc, st[svc]); err != nil {
			errs = append(errs, err)
		}
	}
	flushDNSCache()
	return errors.Join(errs...)
}

func describeSystemDNS() []string {
	svcs, err := networkServices()
	if err != nil {
		return []string{"okunamadı: " + err.Error()}
	}
	var out []string
	for _, svc := range svcs {
		cur := getServiceDNS(svc)
		s := "otomatik"
		if len(cur) > 0 {
			s = strings.Join(cur, " ")
		}
		out = append(out, fmt.Sprintf("%s: %s", svc, s))
	}
	return out
}
