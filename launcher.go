package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strings"
)

var version = "dev"

func isInstalled() bool {
	_, err := os.Stat(installedExePath())
	return err == nil
}

func isRunning() bool {
	return checkDNS("127.0.0.1:53", net.ParseIP(defaultFakeIP)) == nil
}

func launch() error {
	if !isTerminal() {
		return guiLaunch()
	}
	if !isInstalled() {
		return installCmd(nil)
	}
	state := "kurulu ve çalışıyor"
	if !isRunning() {
		state = "kurulu ama ÇALIŞMIYOR"
	}
	fmt.Printf("Discord DNS %s %s.\n\n", version, state)
	fmt.Println("  1) Güncelle / yeniden kur")
	fmt.Println("  2) Kaldır")
	fmt.Println("  3) Durumu göster")
	fmt.Println("  4) Çık")
	fmt.Print("\nSeçim [4]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	switch strings.TrimSpace(line) {
	case "1":
		return installCmd(nil)
	case "2":
		return uninstallCmd(nil)
	case "3":
		return statusCmd(nil)
	}
	return nil
}
