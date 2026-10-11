//go:build windows

package dnscore

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const createNoWindow = 0x08000000

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

func runPS(script string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := hidden(exec.CommandContext(ctx, "powershell.exe",
		"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func OpenBrowser(url string) {
	_ = hidden(exec.Command("rundll32", "url.dll,FileProtocolHandler", url)).Start()
}

func Fatal(msg string) {
	mb := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	title, _ := syscall.UTF16PtrFromString("DNS Speed")
	text, _ := syscall.UTF16PtrFromString(msg)
	mb.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
	os.Exit(1)
}

// Fizyczne karty sieciowe, ktore sa teraz polaczone (bez wirtualnych i VPN).
const upAdapters = `Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.HardwareInterface }`

// SystemDNS zwraca adresy IPv4 serwerow DNS ustawione na aktywnych kartach.
func SystemDNS() []string {
	script := upAdapters + ` | ForEach-Object { (Get-DnsClientServerAddress -InterfaceIndex $_.ifIndex -AddressFamily IPv4).ServerAddresses } | Select-Object -Unique`
	out, err := runPS(script, 15*time.Second)
	if err != nil {
		return nil
	}
	var ips []string
	seen := map[string]bool{}
	for _, line := range strings.Fields(out) {
		ip := net.ParseIP(strings.TrimSpace(line))
		if ip == nil || ip.To4() == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		s := ip.To4().String()
		if !seen[s] {
			seen[s] = true
			ips = append(ips, s)
		}
	}
	return ips
}

func encodePS(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(u)*2)
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// runElevated uruchamia skrypt PowerShell z uprawnieniami administratora
// (Windows pokaze okno zgody UAC).
func runElevated(inner string) error {
	enc := encodePS("$ErrorActionPreference = 'Stop'; " + inner)
	outer := `try { $p = Start-Process powershell.exe -Verb RunAs -Wait -PassThru -WindowStyle Hidden ` +
		`-ArgumentList '-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-EncodedCommand','` + enc + `'; ` +
		`exit $p.ExitCode } catch { exit 1223 }`
	out, err := runPS(outer, 3*time.Minute)
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1223 {
			return errors.New("Nie wyrażono zgody administratora, więc DNS nie został zmieniony.")
		}
		_ = out
		return errors.New("Nie udało się zmienić DNS. Spróbuj uruchomić program jako administrator.")
	}
	return nil
}

func ApplyDNS(ips []string) ([]string, error) {
	quoted := make([]string, len(ips))
	for i, ip := range ips {
		quoted[i] = "'" + ip + "'"
	}
	inner := `$ips = @(` + strings.Join(quoted, ",") + `); ` + upAdapters +
		` | ForEach-Object { Set-DnsClientServerAddress -InterfaceIndex $_.ifIndex -ServerAddresses $ips }; Clear-DnsClientCache`
	if err := runElevated(inner); err != nil {
		return nil, err
	}
	return SystemDNS(), nil
}

func ResetDNS() ([]string, error) {
	inner := upAdapters + ` | ForEach-Object { Set-DnsClientServerAddress -InterfaceIndex $_.ifIndex -ResetServerAddresses }; Clear-DnsClientCache`
	if err := runElevated(inner); err != nil {
		return nil, err
	}
	return SystemDNS(), nil
}
