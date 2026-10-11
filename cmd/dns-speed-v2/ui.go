package main

import (
	"math"
	"strings"
)

// Rect to prostokat we wspolrzednych logicznych.
type Rect struct{ X, Y, W, H float64 }

func (r Rect) Right() float64  { return r.X + r.W }
func (r Rect) Bottom() float64 { return r.Y + r.H }

// Layout opisuje, gdzie lezy kazdy element okna. Korzystaja z niego i malowanie,
// i ustawianie prawdziwych kontrolek Windows, wiec nigdy sie nie rozjezdzaja.
type Layout struct {
	W, H                  float64
	Title, Sub            Rect
	Start, Status, Track  Rect
	Verdict, Apply, Hint  Rect
	Legend, Pool          Rect
	FLabel, FEdit, FAdd   Rect
	FCSV, FMsg, FReset    Rect
	Note                  Rect
	HasVerdict, ShowApply bool
}

const (
	minClientW = 820.0
	minClientH = 600.0
	applyW     = 330.0
)

func ComputeLayout(W, H float64, v *Verdict) Layout {
	var L Layout
	L.W, L.H = W, H
	L.Title = Rect{margin, 16, W - 2*margin, 50}
	L.Sub = Rect{margin, 68, W - 2*margin, 24}

	cy := 104.0
	L.Start = Rect{margin, cy, 156, 44}
	sx := L.Start.Right() + 22
	L.Status = Rect{sx, cy + 1, math.Max(0, W-margin-sx), 24}
	L.Track = Rect{sx, cy + 33, math.Max(0, math.Min(W-margin-sx, 640)), 6}

	y := cy + 44 + 20
	if v != nil {
		L.HasVerdict = true
		L.ShowApply = v.CanApply
		vh := verdictHeight(v, W)
		L.Verdict = Rect{margin, y, W - 2*margin, vh}
		L.Apply = Rect{W - margin - applyW, y + 6, applyW, 44}
		L.Hint = Rect{W - margin - applyW, y + 54, applyW, 20}
		y += vh + 16
	}
	L.Legend = Rect{margin, y, W - 2*margin, 20}
	y += 28

	bottom := H - 14
	L.Note = Rect{margin, bottom - 18, W - 2*margin, 18}
	r2 := L.Note.Y - 8 - 34
	r1 := r2 - 8 - 34
	L.FReset = Rect{W - margin - 250, r2, 250, 34}
	L.FMsg = Rect{margin, r2, math.Max(0, L.FReset.X-margin-16), 34}
	L.FCSV = Rect{W - margin - 210, r1, 210, 34}
	L.FLabel = Rect{margin, r1, 150, 34}
	L.FEdit = Rect{margin + 158, r1, 190, 34}
	L.FAdd = Rect{L.FEdit.Right() + 10, r1, 170, 34}
	L.Pool = Rect{margin, y, W - 2*margin, math.Max(0, r1-14-y)}
	return L
}

// verdictHeight dobiera wysokosc panelu do dlugosci zdania porownania:
// w waskim oknie zdanie zawija sie do 2-3 wierszy.
func verdictHeight(v *Verdict, W float64) float64 {
	tw := W - 2*margin - 78
	if v.CanApply {
		tw -= applyW + 24
	}
	lines := 0
	if v.Line2 != "" {
		est := float64(len([]rune(v.Line2))) * 7.6
		lines = int(math.Ceil(est / math.Max(60, tw)))
		lines = int(clampf(float64(lines), 1, 3))
	}
	return math.Max(62+float64(lines)*21+8, 78)
}

func clampf(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

// PaintMain maluje cale okno poza basenem i prawdziwymi kontrolkami.
func PaintMain(c Canvas, m *Model, L Layout) {
	c.Clear(cPaper)
	c.Text("Wyścig DNS", L.Title.X, L.Title.Y, L.Title.W, L.Title.H, FTitle, cInk, AlignL|VMid)
	c.Text("Sprawdź, który serwer DNS odpowiada najszybciej na Twoim łączu.",
		L.Sub.X, L.Sub.Y, L.Sub.W, L.Sub.H, FBody, cMuted, AlignL|VMid|Ellipsis)

	c.Text(m.Status, L.Status.X, L.Status.Y, L.Status.W, L.Status.H, FBody, cInk, AlignL|VMid|Ellipsis)
	c.FillRRect(L.Track.X, L.Track.Y, L.Track.W, L.Track.H, 3, cInk.Alpha(.12))
	if m.ProgShown > .002 {
		c.FillRRect(L.Track.X, L.Track.Y, math.Max(6, L.Track.W*clampf(m.ProgShown, 0, 1)), L.Track.H, 3, cCobalt)
	}

	if m.Verdict != nil {
		paintVerdict(c, m.Verdict, L)
	}
	paintLegend(c, L)

	c.Text("Własny serwer DNS", L.FLabel.X, L.FLabel.Y, L.FLabel.W, L.FLabel.H, FBodyB, cInk, AlignL|VMid)
	col := cMuted
	switch m.MsgKind {
	case 1:
		col = cGreen
	case 2:
		col = cRed
	}
	c.Text(m.Msg, L.FMsg.X, L.FMsg.Y, L.FMsg.W, L.FMsg.H, FBody, col, AlignL|VMid|Ellipsis)
	c.Text("Różnice rzędu kilku milisekund nie są odczuwalne podczas przeglądania. Przy ważnej decyzji powtórz test o innej porze dnia.",
		L.Note.X, L.Note.Y, L.Note.W, L.Note.H, FSmall, cMuted, AlignL|VMid|Ellipsis)
}

func paintVerdict(c Canvas, v *Verdict, L Layout) {
	r := L.Verdict
	cx, cy := r.X+30, r.Y+34
	c.FillCircle(cx, cy, 31, cGold.Alpha(.28))
	c.FillCircle(cx, cy, 26, cGold)
	c.Text("1", cx-26, cy-26, 52, 52, FH2, cGoldInk, AlignC|VMid)

	tx := r.X + 78
	tw := r.Right() - tx
	if L.ShowApply {
		tw = L.Apply.X - 24 - tx
	}
	tw = math.Max(0, tw)
	c.Text(v.Title, tx, r.Y+2, tw, 32, FH2, cInk, AlignL|VMid|Ellipsis)
	c.Text(v.Line1, tx, r.Y+38, tw, 22, FBody, cInk, AlignL|VMid|Ellipsis)
	if v.Line2 != "" {
		c.Text(v.Line2, tx, r.Y+62, tw, r.H-62-4, FBody, cMuted, AlignL|Wrap)
	}
	if L.ShowApply {
		c.Text("Windows poprosi o zgodę administratora.", L.Hint.X, L.Hint.Y, L.Hint.W, L.Hint.H,
			FSmall, cMuted, AlignL|VMid|Ellipsis)
	}
}

func paintLegend(c Canvas, L Layout) {
	r := L.Legend
	const a, b = "Znane strony (serwer ma je w pamięci)", "Nowe nazwy (serwer musi zapytać dalej)"
	c.FillRRect(r.X, r.Y+5, 22, 10, 5, cCobalt)
	c.Text(a, r.X+30, r.Y, r.W, r.H, FSmall, cMuted, AlignL|VMid)
	x2 := r.X + 30 + c.Measure(a, FSmall) + 28
	c.FillRRect(x2, r.Y+5, 22, 10, 5, cInk)
	c.Text(b, x2+30, r.Y, math.Max(0, r.Right()-x2-30), r.H, FSmall, cMuted, AlignL|VMid|Ellipsis)
}

// PaintPool maluje basen (okno z torami) o rozmiarze w x h, przewiniety o scroll.
func PaintPool(c Canvas, m *Model, w, h, scroll float64) {
	c.FillGrad(0, 0, w, h, 0, cWaterA, cWaterB)
	dom := m.Domain()
	for _, lv := range m.Lanes {
		y := lv.Y - scroll
		if y+laneH < 0 || y > h {
			continue
		}
		paintLane(c, m, lv, w, y, dom)
	}
}

type chipSpec struct {
	text   string
	bg, fg Color
	font   Font
}

func paintLane(c Canvas, m *Model, lv *laneView, w, y, dom float64) {
	r := &lv.R
	dim := 1.0
	if r.State == "waiting" {
		dim = .62
	}
	tx := func(col Color) Color {
		if dim < 1 {
			return mix(col, cWaterB, 1-dim)
		}
		return col
	}
	a := func(col Color, al float64) Color { return col.Alpha(al * dim) }

	nameW := clampf(w*.30, 200, 320)
	midY := y + laneH/2

	// Miejsce na podium.
	if m.Finished && r.State == "done" && r.Rank > 0 {
		mx, my := 30.0, midY
		num := itoa(r.Rank)
		if r.Rank == 1 {
			c.FillCircle(mx, my, 17, cGold.Alpha(.35))
			c.FillCircle(mx, my, 14, cGold)
			c.Text(num, mx-14, my-14, 28, 28, FSmallB, cGoldInk, AlignC|VMid)
		} else {
			c.FillCircle(mx, my, 14, cWhite.Alpha(.75))
			c.Text(num, mx-14, my-14, 28, 28, FSmallB, cInk, AlignC|VMid)
		}
	}

	// Nazwa i adres.
	x0 := 56.0
	textW := nameW - x0 - 8
	name := shortName(r.Name)
	isCurrentOnly := strings.HasPrefix(r.Name, sysPrefix)
	if isCurrentOnly {
		name = "Twój obecny DNS"
	}
	nameCol := cInk
	if r.State == "dead" {
		nameCol = RGB(0x4a5a70)
	}
	c.Text(name, x0, y+9, textW, 20, FBodyB, tx(nameCol), AlignL|VMid|Ellipsis)
	ipW := c.Measure(r.IP, FSmall)
	c.Text(r.IP, x0, y+31, ipW+4, 18, FSmall, tx(cLaneText), AlignL|VMid)

	var chips []chipSpec
	if r.State == "dead" {
		chips = append(chips, chipSpec{"nie odpowiada", cWhite.Alpha(.75), cRed, FTiny})
	}
	if r.Hijack {
		chips = append(chips, chipSpec{"podmienia adresy", cAmberBG, cAmberInk, FTiny})
	}
	if r.System && !isCurrentOnly {
		chips = append(chips, chipSpec{"twój obecny", cWhite.Alpha(.75), cInk, FTiny})
	}
	cx := x0 + ipW + 10
	for _, ch := range chips {
		cw := c.Measure(ch.text, ch.font) + 16
		if cx+cw > x0+textW {
			break
		}
		c.FillRRect(cx, y+32, cw, 18, 9, ch.bg)
		c.Text(ch.text, cx, y+32, cw, 18, ch.font, ch.fg, AlignC|VMid)
		cx += cw + 6
	}

	// Paski: znane strony (kobalt) i nowe nazwy (granat).
	valW := 74.0
	barsX := nameW
	barsR := w - 16 - valW - 12
	barsW := math.Max(10, barsR-barsX)
	c.FillRRect(barsX, midY-17, 3, 34, 1.5, cWhite.Alpha(.95*dim))

	track := .38
	if r.State == "testing" {
		track = .30 + .32*(.5+.5*math.Sin(m.Tick*2*math.Pi))
	}
	bars := []struct {
		v   float64 // wyswietlana wartosc
		raw float64 // wartosc docelowa (<0 = brak)
		col Color
		off float64
	}{
		{lv.C, r.Cached, cCobalt, -15},
		{lv.U, r.Uncached, cInk, 3},
	}
	for _, b := range bars {
		by := midY + b.off
		c.FillRRect(barsX+3, by, barsW-3, 12, 6, cWhite.Alpha(track*dim))
		if b.v > .05 {
			fw := math.Min(barsW-3, math.Max(8, b.v/dom*(barsW-3)))
			c.FillRRect(barsX+3, by, fw, 12, 6, a(b.col, 1))
		}
		c.Text(fmtMs(b.raw), w-16-valW, by-2, valW, 16, FNum, tx(cLaneText), AlignR|VMid)
	}

	// Lina miedzy torami.
	for dx := 4.5; dx < w; dx += 9 {
		c.FillCircle(dx, y+laneH-1.5, 1.8, cWhite.Alpha(.9))
	}
}

// ButtonKind to wyglad przycisku.
type ButtonKind int

const (
	BtnPrimary ButtonKind = iota
	BtnGhost
)

// PaintButton maluje przycisk o rozmiarze w x h. Tlo to kolor okna, zeby rogi byly gladkie.
func PaintButton(c Canvas, w, h float64, label string, kind ButtonKind, pressed, focus, disabled bool) {
	c.Clear(cPaper)
	rad := 10.0
	switch kind {
	case BtnPrimary:
		bg := cCobalt
		if pressed {
			bg = cCobaltDk
		}
		fg := cWhite
		if disabled {
			bg = mix(cCobalt, cPaper, .55)
		}
		c.FillRRect(0, 0, w, h, rad, bg)
		c.Text(label, 8, 0, w-16, h, FBodyB, fg, AlignC|VMid|Ellipsis)
		if focus && !disabled {
			c.StrokeRRect(3, 3, w-6, h-6, rad-2, 2, cWhite.Alpha(.9))
		}
	default:
		ink := cInk
		if disabled {
			ink = mix(cInk, cPaper, .6)
		}
		fg := ink
		if pressed && !disabled {
			c.FillRRect(0, 0, w, h, rad, ink)
			fg = cWhite
		} else {
			c.StrokeRRect(0, 0, w, h, rad, 2, ink)
		}
		c.Text(label, 8, 0, w-16, h, FBodyB, fg, AlignC|VMid|Ellipsis)
		if focus && !disabled {
			c.StrokeRRect(4, 4, w-8, h-8, rad-3, 2, cCobalt)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
