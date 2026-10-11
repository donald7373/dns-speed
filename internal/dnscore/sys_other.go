//go:build !windows

package dnscore

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
)

// Ten plik istnieje tylko po to, by dalo sie uruchomic i przetestowac program
// poza Windowsem. Wlasciwa wersja jest w sys_windows.go.

func OpenBrowser(url string) {
	_ = exec.Command("xdg-open", url).Start()
}

func Fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}

func SystemDNS() []string {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	defer f.Close()
	var ips []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && fields[0] == "nameserver" {
			ip := net.ParseIP(fields[1])
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
				ips = append(ips, ip.To4().String())
			}
		}
	}
	return ips
}

func ApplyDNS(ips []string) ([]string, error) {
	return nil, errors.New("zmiana DNS działa tylko w Windows")
}

func ResetDNS() ([]string, error) {
	return nil, errors.New("zmiana DNS działa tylko w Windows")
}
