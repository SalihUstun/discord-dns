package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func programFiles() string {
	if p := os.Getenv("ProgramFiles"); p != "" {
		return p
	}
	return `C:\Program Files`
}

func programData() string {
	if p := os.Getenv("ProgramData"); p != "" {
		return p
	}
	return `C:\ProgramData`
}

func installedExePath() string {
	return filepath.Join(programFiles(), "discord-dns", "discord-dns.exe")
}
func stateDir() string    { return filepath.Join(programData(), "discord-dns") }
func logFilePath() string { return filepath.Join(stateDir(), "discord-dns.log") }

func isAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

func elevate(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args = append(args, "-pause")
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = windows.EscapeArg(a)
	}
	cwd, _ := os.Getwd()

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	dir, _ := windows.UTF16PtrFromString(cwd)
	fmt.Println("Yönetici izni gerekiyor; açılan pencerede \"Evet\"e bas.")
	if err := windows.ShellExecute(0, verb, file, params, dir, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("yönetici izni alınamadı: %w", err)
	}
	os.Exit(0)
	return nil
}

func ensureLoopbackAlias(ip string) error {
	return errors.New("Windows'ta loopback alias desteklenmiyor")
}

func removeLoopbackAlias(ip string) {}

func removeLegacy() {}

const uninstallRegKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\discord-dns`

func removeInstalledExe() error {
	registry.DeleteKey(registry.LOCAL_MACHINE, uninstallRegKey)
	dir := filepath.Dir(installedExePath())
	self, _ := os.Executable()
	if samePath(self, installedExePath()) {
		cmd := exec.Command("cmd")
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow: true,
			CmdLine:    `cmd /C "ping 127.0.0.1 -n 3 > nul & rmdir /S /Q "` + dir + `""`,
		}
		return cmd.Start()
	}
	err := os.RemoveAll(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func flushDNSCache() {
	cmd := exec.Command("ipconfig", "/flushdns")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Run()
}

var sysDNSMu sync.Mutex

type winIface struct {
	Index int    `json:"Index"`
	Name  string `json:"Name"`
	Guid  string `json:"Guid"`
	V4    string `json:"V4"`
	V6    string `json:"V6"`
}

type dnsState map[string]winIface

func powershell(script string) (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command",
		"[Console]::OutputEncoding=[Text.Encoding]::UTF8; "+script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("powershell: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

const listIfacesPS = `$ErrorActionPreference='SilentlyContinue'
$r=@()
foreach($a in Get-NetAdapter){
  $g=$a.InterfaceGuid
  $v4=(Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\$g").NameServer
  $v6=(Get-ItemProperty "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip6\Parameters\Interfaces\$g").NameServer
  $r+=[pscustomobject]@{Index=[int]$a.ifIndex;Name=[string]$a.Name;Guid=[string]$g;V4=[string]$v4;V6=[string]$v6}
}
ConvertTo-Json -InputObject $r -Compress`

func listIfaces() ([]winIface, error) {
	out, err := powershell(listIfacesPS)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if i := strings.IndexByte(out, '['); i > 0 {
		out = out[i:]
	}
	var ifs []winIface
	if err := json.Unmarshal([]byte(out), &ifs); err != nil {
		return nil, fmt.Errorf("adaptör listesi okunamadı: %v (%q)", err, out)
	}
	return ifs, nil
}

func splitServers(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
}

func isOurs(i winIface) bool {
	v4 := splitServers(i.V4)
	return len(v4) == 1 && v4[0] == "127.0.0.1"
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func loadDNSState() dnsState {
	st := dnsState{}
	if data, err := os.ReadFile(dnsStatePath()); err == nil {
		json.Unmarshal(data, &st)
	}
	return st
}

func saveDNSState(st dnsState) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(dnsStatePath(), data, 0o644)
}

func applySystemDNS() error {
	sysDNSMu.Lock()
	defer sysDNSMu.Unlock()

	ifs, err := listIfaces()
	if err != nil {
		return err
	}
	st := loadDNSState()
	var script strings.Builder
	for _, i := range ifs {
		if isOurs(i) {
			continue
		}
		st[i.Guid] = i

		fmt.Fprintf(&script,
			"try { Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses ('127.0.0.1','::1') -ErrorAction Stop } catch { Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses ('127.0.0.1') }\n",
			i.Index, i.Index)
	}
	if script.Len() == 0 {
		return nil
	}
	if err := saveDNSState(st); err != nil {
		return err
	}
	_, err = powershell(script.String())
	flushDNSCache()
	return err
}

func restoreSystemDNS() error {
	sysDNSMu.Lock()
	defer sysDNSMu.Unlock()

	ifs, err := listIfaces()
	if err != nil {
		return err
	}
	st := loadDNSState()
	var script strings.Builder
	for _, i := range ifs {
		if !isOurs(i) {
			continue
		}
		fmt.Fprintf(&script, "Set-DnsClientServerAddress -InterfaceIndex %d -ResetServerAddresses\n", i.Index)
		prev := st[i.Guid]
		servers := append(splitServers(prev.V4), splitServers(prev.V6)...)
		if len(servers) > 0 {
			q := make([]string, len(servers))
			for k, s := range servers {
				q[k] = psQuote(s)
			}
			fmt.Fprintf(&script, "Set-DnsClientServerAddress -InterfaceIndex %d -ServerAddresses (%s)\n", i.Index, strings.Join(q, ","))
		}
	}
	if script.Len() == 0 {
		return nil
	}
	_, err = powershell(script.String())
	flushDNSCache()
	return err
}

func describeSystemDNS() []string {
	ifs, err := listIfaces()
	if err != nil {
		return []string{"okunamadı: " + err.Error()}
	}
	var out []string
	for _, i := range ifs {
		s := "otomatik"
		if v := strings.Join(append(splitServers(i.V4), splitServers(i.V6)...), " "); v != "" {
			s = v
		}
		out = append(out, fmt.Sprintf("%s: %s", i.Name, s))
	}
	return out
}
