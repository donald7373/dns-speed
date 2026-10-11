package main

import (
	"encoding/json"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"dnsspeed/internal/dnscore"
)

// Podglad okna bez Windowsa: ksztalty rysuje nasz Raster, tekst jest zapisywany
// do pliku JSON i nakladany przez tools/overlay.py.

type textOp struct {
	S  string   `json:"s"`
	X  float64  `json:"x"`
	Y  float64  `json:"y"`
	W  float64  `json:"w"`
	H  float64  `json:"h"`
	F  int      `json:"f"`
	C  [3]uint8 `json:"c"`
	Fl int      `json:"fl"`
}

type previewCanvas struct {
	*Raster
	ox, oy float64
	ops    *[]textOp
}

func (p previewCanvas) Text(s string, x, y, w, h float64, f Font, c Color, fl TextFlags) {
	*p.ops = append(*p.ops, textOp{s, x + p.ox, y + p.oy, w, h, int(f), [3]uint8{c.R, c.G, c.B}, int(fl)})
}

// Szerokosc z grubsza: Inter jest szerszy od Segoe UI, wiec to wersja pesymistyczna.
func (p previewCanvas) Measure(s string, f Font) float64 {
	k := .53
	if f == FBodyB || f == FSmallB || f == FH2 || f == FTitle {
		k = .57
	}
	return float64(len([]rune(s))) * fontSpecs[f].Px * k
}

func (p previewCanvas) MeasureWrap(s string, w float64, f Font) float64 {
	lines := math.Ceil(p.Measure(s, f) / math.Max(1, w))
	return lines * fontSpecs[f].Px * 1.35
}

func blit(dst, src *Raster, dx, dy int) {
	for y := 0; y < src.H; y++ {
		ty := y + dy
		if ty < 0 || ty >= dst.H {
			continue
		}
		for x := 0; x < src.W; x++ {
			tx := x + dx
			if tx < 0 || tx >= dst.W {
				continue
			}
			dst.Pix[ty*dst.W+tx] = src.Pix[y*src.W+x]
		}
	}
}

func savePNG(path string, r *Raster) error {
	img := image.NewRGBA(image.Rect(0, 0, r.W, r.H))
	for i, p := range r.Pix {
		img.Pix[i*4] = uint8(p >> 16)
		img.Pix[i*4+1] = uint8(p >> 8)
		img.Pix[i*4+2] = uint8(p)
		img.Pix[i*4+3] = 255
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

type srv struct {
	name, ip string
	c, u     float64
	system   bool
	hijack   bool
	dead     bool
}

var sample = []srv{
	{"Cloudflare", "1.1.1.1", 6.1, 14.8, false, false, false},
	{"Cloudflare (zapasowy)", "1.0.0.1", 6.4, 15.2, false, false, false},
	{"Google", "8.8.8.8", 8.2, 22.9, false, false, false},
	{"Google (zapasowy)", "8.8.4.4", 8.6, 24.1, false, false, false},
	{"Quad9", "9.9.9.9", 12.5, 35.7, false, false, false},
	{"Obecny DNS (192.168.100.1)", "192.168.100.1", 17.4, 50.7, true, false, false},
	{"OpenDNS", "208.67.222.222", 18.1, 52.0, false, false, false},
	{"AdGuard", "94.140.14.14", 26.4, 73.8, false, false, false},
	{"Orange Polska", "194.204.152.34", 32.4, 92.6, false, false, false},
	{"Mullvad", "194.242.2.2", 46.4, 137.8, false, false, false},
	{"Comodo", "8.26.56.26", 5.0, 11.6, false, true, false},
	{"DNS.WATCH", "84.200.69.80", 0, 0, false, false, true},
	{"Własny (192.168.0.77)", "192.168.0.77", 0, 0, false, false, true},
}

// synth buduje stan testu: first elementow zakonczonych, reszta czeka.
func synth(done int, finished bool) dnscore.Status {
	var res []*dnscore.Result
	for i, s := range sample {
		r := &dnscore.Result{Name: s.name, IP: s.ip, System: s.system, State: "waiting", Cached: -1, Uncached: -1, Score: -1}
		switch {
		case i < done && s.dead:
			r.State = "dead"
		case i < done:
			r.State, r.Cached, r.Uncached, r.Hijack = "done", s.c, s.u, s.hijack
			r.Score = (s.c + s.u) / 2
		case i == done && !finished:
			r.State = "testing"
		}
		res = append(res, r)
	}
	if finished {
		idx := append([]*dnscore.Result(nil), res...)
		sort.SliceStable(idx, func(a, b int) bool {
			ra, rb := idx[a], idx[b]
			if (ra.State == "dead") != (rb.State == "dead") {
				return ra.State != "dead"
			}
			if ra.Hijack != rb.Hijack {
				return !ra.Hijack
			}
			return ra.Score < rb.Score
		})
		for n, r := range idx {
			r.Rank = n + 1
		}
	}
	cur := ""
	if done < len(sample) {
		cur = sample[done].name
	}
	return dnscore.Status{Running: !finished, Done: done, Total: len(sample), Current: cur, Results: res}
}

func render(t *testing.T, name string, W, H, scale float64, m *Model, scroll float64) {
	dir := os.Getenv("PREVIEW_DIR")
	if dir == "" {
		t.Skip("ustaw PREVIEW_DIR, aby zapisac podglad")
	}
	L := ComputeLayout(W, H, m.Verdict)
	var ops []textOp
	main := NewRaster(int(W*scale), int(H*scale), scale)
	PaintMain(previewCanvas{Raster: main, ops: &ops}, m, L)

	pool := NewRaster(int(L.Pool.W*scale), int(L.Pool.H*scale), scale)
	PaintPool(previewCanvas{Raster: pool, ox: L.Pool.X, oy: L.Pool.Y, ops: &ops}, m, L.Pool.W, L.Pool.H, scroll)
	blit(main, pool, int(L.Pool.X*scale), int(L.Pool.Y*scale))

	btn := func(r Rect, label string, kind ButtonKind, disabled bool) {
		b := NewRaster(int(r.W*scale), int(r.H*scale), scale)
		PaintButton(previewCanvas{Raster: b, ox: r.X, oy: r.Y, ops: &ops}, r.W, r.H, label, kind, false, false, disabled)
		blit(main, b, int(r.X*scale), int(r.Y*scale))
	}
	startLabel := "Zacznij test"
	if m.Finished {
		startLabel = "Powtórz test"
	}
	btn(L.Start, startLabel, BtnPrimary, m.Running)
	if L.ShowApply {
		btn(L.Apply, m.Verdict.ApplyLabel, BtnPrimary, false)
	}
	btn(L.FAdd, "Dodaj do wyścigu", BtnGhost, m.Running)
	btn(L.FCSV, "Zapisz wyniki (CSV)", BtnGhost, !m.Finished)
	btn(L.FReset, "Przywróć automatyczny DNS", BtnGhost, m.Running)
	// pole tekstowe (w Windows to prawdziwa kontrolka)
	main.FillRRect(L.FEdit.X, L.FEdit.Y+3, L.FEdit.W, L.FEdit.H-6, 0, RGB(0x8a97a8))
	main.FillRRect(L.FEdit.X+1, L.FEdit.Y+4, L.FEdit.W-2, L.FEdit.H-8, 0, cWhite)
	ops = append(ops, textOp{"np. 192.168.0.1", L.FEdit.X + 8, L.FEdit.Y, L.FEdit.W - 16, L.FEdit.H, int(FBody), [3]uint8{0x77, 0x82, 0x92}, int(AlignL | VMid)})

	png := filepath.Join(dir, name+".png")
	if err := savePNG(png, main); err != nil {
		t.Fatal(err)
	}
	js, _ := json.Marshal(map[string]any{"scale": scale, "ops": ops})
	if err := os.WriteFile(png[:len(png)-4]+".json", js, 0o644); err != nil {
		t.Fatal(err)
	}
}

func scenario(done int, finished bool) *Model {
	m := NewModel()
	m.Update(synth(done, finished))
	for i := 0; i < 5; i++ {
		m.Step(10)
	}
	return m
}

func TestPreview(t *testing.T) {
	idle := NewModel()
	var list []dnscore.Server
	for _, s := range sample[:11] {
		list = append(list, dnscore.Server{Name: s.name, IP: s.ip, System: s.system})
	}
	idle.SetServers(list)
	idle.Status = "Gotowe do startu. Test trwa około minuty."
	render(t, "prev-idle", 1000, 780, 1, idle, 0)

	run := scenario(6, false)
	render(t, "prev-running", 1000, 780, 1, run, 0)

	done := scenario(len(sample), true)
	render(t, "prev-done", 1000, 780, 1, done, 0)
	render(t, "prev-narrow", 820, 680, 1, done, 0)
	render(t, "prev-scrolled", 1000, 780, 1, done, 140)
	render(t, "prev-hidpi", 1000, 780, 2, done, 0)
}
