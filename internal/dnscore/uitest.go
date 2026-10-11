//go:build uitest

package dnscore

import (
	"encoding/binary"
	"net"
	"strings"
	"time"
)

// Wersja tylko do podgladu interfejsu: zamiast prawdziwych serwerow
// uruchamia kilka falszywych na adresach 127.0.0.x o roznych opoznieniach.

type fake struct {
	name  string
	ip    string
	delay time.Duration
	mode  string
}

func serveFake(f fake, port string) {
	pc, err := net.ListenPacket("udp", net.JoinHostPort(f.ip, port))
	if err != nil {
		panic(err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if f.mode == "silent" || n < 12 {
				continue
			}
			q := append([]byte(nil), buf[:n]...)
			go func() {
				jitter := time.Duration(int(q[1])%5) * time.Millisecond / 2
				time.Sleep(f.delay + jitter)
				name, i := "", 12
				for i < len(q) && q[i] != 0 {
					l := int(q[i])
					name += string(q[i+1:i+1+l]) + "."
					i += l + 1
				}
				random := len(strings.Split(name, ".")[0]) == 14
				resp := append([]byte(nil), q...)
				flags := uint16(0x8180)
				answers := uint16(1)
				extra := time.Duration(0)
				if random {
					extra = f.delay * 2 // bez cache wolniej
					if f.mode == "ok" {
						flags |= 3
						answers = 0
					}
				}
				time.Sleep(extra)
				binary.BigEndian.PutUint16(resp[2:], flags)
				binary.BigEndian.PutUint16(resp[6:], answers)
				_, _ = pc.WriteTo(resp, from)
			}()
		}
	}()
}

func init() {
	dnsPort = "5353"
	fakes := []fake{
		{"Cloudflare", "127.0.0.11", 4 * time.Millisecond, "ok"},
		{"Google", "127.0.0.12", 7 * time.Millisecond, "ok"},
		{"Quad9", "127.0.0.13", 11 * time.Millisecond, "ok"},
		{"OpenDNS", "127.0.0.14", 16 * time.Millisecond, "ok"},
		{"AdGuard", "127.0.0.15", 24 * time.Millisecond, "ok"},
		{"Orange Polska", "127.0.0.16", 30 * time.Millisecond, "ok"},
		{"DNS.WATCH", "127.0.0.17", 45 * time.Millisecond, "ok"},
		{"Szybki ale podmienia", "127.0.0.18", 3 * time.Millisecond, "hijack"},
		{"Martwy serwer", "127.0.0.19", 0, "silent"},
	}
	builtinServers = nil
	for _, f := range fakes {
		serveFake(f, dnsPort)
		builtinServers = append(builtinServers, Server{Name: f.name, IP: f.ip})
	}
}
