package dnscore

import (
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// fakeDNS uruchamia falszywy serwer UDP: odpowiada po zadanym czasie.
// mode: "ok" (NOERROR dla znanych, NXDOMAIN dla losowych), "hijack" (zawsze adres),
// "silent" (nie odpowiada wcale).
func fakeDNS(t *testing.T, delay time.Duration, mode string) (port string, stop func()) {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if mode == "silent" || n < 12 {
				continue
			}
			q := append([]byte(nil), buf[:n]...)
			go func() {
				time.Sleep(delay)
				// odczytaj nazwe, by rozroznic losowe od znanych
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
				if random && mode == "ok" {
					flags |= 3 // NXDOMAIN
					answers = 0
				}
				binary.BigEndian.PutUint16(resp[2:], flags)
				binary.BigEndian.PutUint16(resp[6:], answers)
				_, _ = pc.WriteTo(resp, from)
			}()
		}
	}()
	_, p, _ := net.SplitHostPort(pc.LocalAddr().String())
	return p, func() { pc.Close() }
}

func TestMeasureFast(t *testing.T) {
	port, stop := fakeDNS(t, 5*time.Millisecond, "ok")
	defer stop()
	dnsPort = port

	r := measure("127.0.0.1")
	if r.State != "done" {
		t.Fatalf("stan %q, oczekiwano done", r.State)
	}
	if r.Cached < 4 || r.Cached > 30 || r.Uncached < 4 || r.Uncached > 30 {
		t.Fatalf("nietypowe czasy: cached=%.1f uncached=%.1f", r.Cached, r.Uncached)
	}
	if r.Hijack || r.Fails != 0 {
		t.Fatalf("hijack=%v fails=%d", r.Hijack, r.Fails)
	}
	t.Logf("szybki: cache=%.1f ms, bez cache=%.1f ms, wynik=%.1f", r.Cached, r.Uncached, r.Score)
}

func TestMeasureHijack(t *testing.T) {
	port, stop := fakeDNS(t, 3*time.Millisecond, "hijack")
	defer stop()
	dnsPort = port
	r := measure("127.0.0.1")
	if !r.Hijack {
		t.Fatalf("nie wykryto podmiany adresow: %+v", r)
	}
}

func TestMeasureDead(t *testing.T) {
	port, stop := fakeDNS(t, 0, "silent")
	defer stop()
	dnsPort = port
	start := time.Now()
	r := measure("127.0.0.1")
	if r.State != "dead" {
		t.Fatalf("stan %q, oczekiwano dead", r.State)
	}
	if time.Since(start) > 8*time.Second {
		t.Fatalf("martwy serwer trwal za dlugo: %v", time.Since(start))
	}
}

func TestBenchRanking(t *testing.T) {
	fast, s1 := fakeDNS(t, 4*time.Millisecond, "ok")
	defer s1()
	dnsPort = fast
	b := &Bench{}
	b.Start([]Server{{Name: "A", IP: "127.0.0.1"}, {Name: "B", IP: "127.0.0.1", System: true}})
	deadline := time.Now().Add(20 * time.Second)
	for b.IsRunning() && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	st := b.Snapshot()
	if st.Running || st.Done != 2 {
		t.Fatalf("test nie zakonczyl sie: %+v", st)
	}
	for _, r := range st.Results {
		if r.Rank < 1 || r.Rank > 2 {
			t.Fatalf("zle miejsce: %+v", r)
		}
	}
}

func TestRankHijackBelowHonest(t *testing.T) {
	rs := []*Result{
		{Name: "podmienia", State: "done", Score: 3, Hijack: true},
		{Name: "uczciwy", State: "done", Score: 10},
		{Name: "martwy", State: "dead", Score: -1},
		{Name: "wolny", State: "done", Score: 50},
	}
	rank(rs)
	want := map[string]int{"uczciwy": 1, "wolny": 2, "podmienia": 3, "martwy": 4}
	for _, r := range rs {
		if r.Rank != want[r.Name] {
			t.Errorf("%s: miejsce %d, oczekiwano %d", r.Name, r.Rank, want[r.Name])
		}
	}
}
