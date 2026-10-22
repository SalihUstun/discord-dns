package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func appleString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func osascript(script string) (string, error) {
	cmd := exec.Command("osascript")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func appIcon() string {
	exe, err := os.Executable()
	if err != nil {
		return "note"
	}
	p := filepath.Clean(filepath.Join(filepath.Dir(exe), "..", "Resources", "AppIcon.icns"))
	if _, err := os.Stat(p); err != nil {
		return "note"
	}
	return "(POSIX file " + appleString(p) + ")"
}

func dialog(msg string, buttons []string, def string, icon string) string {
	if icon == "note" {
		icon = appIcon()
	}
	quoted := make([]string, len(buttons))
	for i, b := range buttons {
		quoted[i] = appleString(b)
	}
	script := fmt.Sprintf("activate\nset r to display dialog %s with title \"Discord DNS\" buttons {%s} default button %s with icon %s\nreturn button returned of r\n",
		appleString(msg), strings.Join(quoted, ", "), appleString(def), icon)
	out, err := osascript(script)
	if err != nil {
		return ""
	}
	return out
}

func guiRunAdmin(exe, cmd, okMsg string) error {
	script := fmt.Sprintf("do shell script (quoted form of %s) & \" %s 2>&1\" with prompt %s with administrator privileges\n",
		appleString(exe), cmd, appleString("Discord DNS sistem DNS ayarlarını değiştirmek için izin istiyor."))
	out, err := osascript(script)
	if err != nil {
		if strings.Contains(out, "-128") {
			return nil
		}
		lines := strings.Split(out, "\n")
		if len(lines) > 12 {
			lines = lines[len(lines)-12:]
		}
		dialog("Bir sorun oluştu:\n\n"+strings.Join(lines, "\n"), []string{"Tamam"}, "Tamam", "stop")
		return fmt.Errorf("%s başarısız: %v", cmd, err)
	}
	dialog(okMsg, []string{"Tamam"}, "Tamam", "note")
	return nil
}

func guiLaunch() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	installMsg := "Kurulum tamam.\n\nDiscord DNS arka planda çalışıyor ve bilgisayar her açıldığında kendiliğinden başlayacak.\n\nDiscord açıksa tamamen kapatıp (Cmd+Q) yeniden aç."

	if isInstalled() && isRunning() {
		switch dialog("Discord DNS "+version+" çalışıyor.\n\nDiscord trafiği arka planda engelden geçiriliyor, diğer trafiğe dokunulmuyor.",
			[]string{"Kaldır", "Yeniden Kur", "Tamam"}, "Tamam", "note") {
		case "Kaldır":
			if dialog("Discord DNS kaldırılsın mı?\n\nDNS ayarların kurulum öncesi haline döner.", []string{"Vazgeç", "Kaldır"}, "Vazgeç", "caution") != "Kaldır" {
				return nil
			}
			return guiRunAdmin(exe, "uninstall", "Discord DNS kaldırıldı. DNS ayarların eski haline döndü.")
		case "Yeniden Kur":
			return guiRunAdmin(exe, "install", installMsg)
		}
		return nil
	}

	msg := "Discord DNS kurulsun mu?\n\nSadece Discord trafiği engelden geçirilir, diğer internet trafiğine dokunulmaz. Program arka planda çalışır ve bilgisayar her açıldığında kendiliğinden başlar."
	if isInstalled() {
		msg = "Discord DNS kurulu ama çalışmıyor.\n\nYeniden kurulsun mu?"
	}
	if dialog(msg, []string{"İptal", "Kur"}, "Kur", "note") != "Kur" {
		return nil
	}
	return guiRunAdmin(exe, "install", installMsg)
}
