//go:build !darwin && !windows

package main

import (
	"errors"
	"os"
)

var errUnsupported = errors.New("bu işletim sistemi desteklenmiyor (sadece macOS ve Windows)")

func installedExePath() string { return "/usr/local/bin/discord-dns" }
func stateDir() string         { return "/etc/discord-dns" }
func logFilePath() string      { return "/var/log/discord-dns.log" }

func isAdmin() bool                       { return os.Geteuid() == 0 }
func elevate(args []string) error         { return errUnsupported }
func ensureLoopbackAlias(ip string) error { return errUnsupported }
func removeLoopbackAlias(ip string)       {}
func removeLegacy()                       {}
func removeInstalledExe() error           { return errUnsupported }
func flushDNSCache()                      {}
func applySystemDNS() error               { return errUnsupported }
func restoreSystemDNS() error             { return errUnsupported }
func describeSystemDNS() []string         { return nil }
