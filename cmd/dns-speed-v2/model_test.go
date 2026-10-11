package main

import (
	"math"
	"strings"
	"testing"

	"dnsspeed/internal/dnscore"
)

func inside(r Rect, W, H float64) bool {
	return r.X >= -.001 && r.Y >= -.001 && r.Right() <= W+.001 && r.Bottom() <= H+.001
}

func overlap(a, b Rect) bool {
	return a.X < b.Right()-.001 && b.X < a.Right()-.001 && a.Y < b.Bottom()-.001 && b.Y < a.Bottom()-.001
}

func TestLayoutNoOverlap(t *testing.T) {
	v := &Verdict{CanApply: true}
	for _, withV := range []*Verdict{nil, v} {
		for _, sz := range [][2]float64{{820, 680}, {1000, 780}, {1280, 900}, {1600, 1000}, {2200, 1300}} {
			W, H := sz[0], sz[1]
			L := ComputeLayout(W, H, withV)
			named := map[string]Rect{
				"Start": L.Start, "Pool": L.Pool, "FLabel": L.FLabel, "FEdit": L.FEdit,
				"FAdd": L.FAdd, "FCSV": L.FCSV, "FMsg": L.FMsg, "FReset": L.FReset,
				"Note": L.Note, "Legend": L.Legend, "Sub": L.Sub,
			}
			if L.ShowApply {
				named["Apply"] = L.Apply
				named["Hint"] = L.Hint
			}
			var keys []string
			for k, r := range named {
				keys = append(keys, k)
				if !inside(r, W, H) {
					t.Errorf("%vx%v verdict=%v: %s poza oknem: %+v", W, H, withV != nil, k, r)
				}
				if r.W < 0 || r.H < 0 {
					t.Errorf("%s ma ujemny rozmiar: %+v", k, r)
				}
			}
			for i := range keys {
				for j := i + 1; j < len(keys); j++ {
					if overlap(named[keys[i]], named[keys[j]]) {
						t.Errorf("%vx%v verdict=%v: %s nachodzi na %s: %+v / %+v", W, H, withV != nil,
							keys[i], keys[j], named[keys[i]], named[keys[j]])
					}
				}
			}
			if W == minClientW && H == minClientH && L.Pool.H < laneH*2.5 {
				t.Errorf("basen za niski w najmniejszym oknie: %.0f px", L.Pool.H)
			}
		}
	}
}

func TestVerdictBestIsHonest(t *testing.T) {
	res := []*dnscore.Result{
		{Name: "Podmienia", IP: "1.2.3.4", State: "done", Score: 3, Hijack: true, Rank: 3},
		{Name: "Dobry", IP: "1.1.1.1", State: "done", Score: 10, Rank: 1},
		{Name: "Obecny DNS (192.168.0.1)", IP: "192.168.0.1", System: true, State: "done", Score: 40, Rank: 2},
		{Name: "Martwy", IP: "9.9.9.9", State: "dead", Rank: 4},
	}
	v := computeVerdict(res)
	if v.Title != "Wygrywa Dobry" || !v.CanApply {
		t.Fatalf("zly werdykt: %+v", v)
	}
	if !strings.Contains(v.Line2, "192.168.0.1") || !strings.Contains(v.Line2, "30 ms") {
		t.Errorf("porownanie z obecnym DNS: %q", v.Line2)
	}
	if v.ApplyLabel != "Ustaw 1.1.1.1 i 192.168.0.1" {
		t.Errorf("etykieta: %q", v.ApplyLabel)
	}
}

func TestVerdictCurrentWins(t *testing.T) {
	res := []*dnscore.Result{
		{Name: "Obecny DNS (192.168.0.1)", IP: "192.168.0.1", System: true, State: "done", Score: 5, Rank: 1},
		{Name: "Dobry", IP: "1.1.1.1", State: "done", Score: 10, Rank: 2},
	}
	v := computeVerdict(res)
	if v.CanApply || v.Title != "Wygrywa 192.168.0.1" {
		t.Fatalf("zly werdykt: %+v", v)
	}
}

func TestVerdictNobody(t *testing.T) {
	v := computeVerdict([]*dnscore.Result{{Name: "A", IP: "1.1.1.1", State: "dead", Rank: 1}})
	if v.CanApply || !strings.Contains(v.Title, "Żaden") {
		t.Fatalf("zly werdykt: %+v", v)
	}
}

func TestPolishPlural(t *testing.T) {
	want := map[int]string{1: "serwer", 2: "serwery", 4: "serwery", 5: "serwerów", 11: "serwerów",
		12: "serwerów", 14: "serwerów", 16: "serwerów", 22: "serwery", 25: "serwerów"}
	for n, w := range want {
		if got := plServers(n); got != w {
			t.Errorf("%d -> %q, oczekiwano %q", n, got, w)
		}
	}
}

func TestFmtMs(t *testing.T) {
	cases := map[float64]string{-1: "–", 0: "0,0 ms", 5.44: "5,4 ms", 9.96: "10,0 ms", 10: "10 ms", 137.8: "138 ms"}
	for in, want := range cases {
		got := fmtMs(in)
		// 9.96 zaokragla sie do "10,0" przez %.1f, co jest akceptowalne
		if got != want {
			t.Errorf("fmtMs(%v) = %q, oczekiwano %q", in, got, want)
		}
	}
}

func TestModelFlowAndAnimation(t *testing.T) {
	m := scenario(0, false)
	if m.Finished || !m.Running {
		t.Fatal("test powinien trwac")
	}
	m = scenario(len(sample), true)
	if !m.Finished || m.Verdict == nil || m.Verdict.Title != "Wygrywa Cloudflare" {
		t.Fatalf("po zakonczeniu: %+v", m.Verdict)
	}
	// kolejnosc po zakonczeniu zgodna z miejscami
	for i, lv := range m.Lanes {
		if lv.R.Rank != i+1 {
			t.Errorf("tor %d ma miejsce %d", i, lv.R.Rank)
		}
		if math.Abs(lv.Y-lv.TY) > .01 {
			t.Errorf("animacja nie doszla do celu: %v vs %v", lv.Y, lv.TY)
		}
	}
	// podmieniajacy adresy nie moze byc przed uczciwymi
	var hijackRank, lastHonest int
	for _, lv := range m.Lanes {
		if lv.R.Hijack {
			hijackRank = lv.R.Rank
		} else if lv.R.State == "done" && lv.R.Rank > lastHonest {
			lastHonest = lv.R.Rank
		}
	}
	if hijackRank < lastHonest {
		t.Errorf("podmieniajacy adresy (%d) przed uczciwym (%d)", hijackRank, lastHonest)
	}
	// Step zatrzymuje sie, gdy nic sie nie rusza i test nie trwa
	if m.Step(.016) {
		t.Error("Step powinien zwrocic false po zakonczeniu animacji")
	}
}

func TestAddCustom(t *testing.T) {
	m := NewModel()
	m.SetServers([]dnscore.Server{{Name: "A", IP: "1.1.1.1"}})
	if ip, msg := m.AddCustom(" 192.168.0.1 "); ip != "192.168.0.1" || msg != "" {
		t.Fatalf("poprawny adres: %q %q", ip, msg)
	}
	for _, bad := range []string{"", "abc", "999.1.1.1", "1.2.3", "::1"} {
		if _, msg := m.AddCustom(bad); msg == "" {
			t.Errorf("%q powinno byc odrzucone", bad)
		}
	}
	if _, msg := m.AddCustom("1.1.1.1"); msg != "Ten adres jest już na liście." {
		t.Errorf("duplikat: %q", msg)
	}
	if len(m.Lanes) != 2 {
		t.Errorf("tory: %d", len(m.Lanes))
	}
}

func TestBuildCSV(t *testing.T) {
	lanes := []*laneView{
		{R: dnscore.Result{Name: `Serwer "A"`, IP: "1.1.1.1", State: "done", Cached: 5.44, Uncached: 12.3, Score: 8.9, Rank: 1}},
		{R: dnscore.Result{Name: "B", IP: "2.2.2.2", State: "dead", Cached: -1, Uncached: -1, Score: -1, Fails: 3, Hijack: true}},
	}
	out := buildCSV(lanes)
	if !strings.HasPrefix(out, "\uFEFFMiejsce;Serwer;") {
		t.Fatalf("brak BOM lub naglowka: %q", out[:30])
	}
	for _, want := range []string{`1;"Serwer ""A""";"1.1.1.1";5,4;12,3;8,9;0;nie`, `;"B";"2.2.2.2";;;;3;tak`} {
		if !strings.Contains(out, want) {
			t.Errorf("brak %q w:\n%s", want, out)
		}
	}
}
