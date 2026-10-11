package main

import "math"

// Color to kolor z przezroczystoscia (A: 0..1).
type Color struct {
	R, G, B uint8
	A       float64
}

func RGB(hex uint32) Color { return Color{uint8(hex >> 16), uint8(hex >> 8), uint8(hex), 1} }

func (c Color) Alpha(a float64) Color { c.A *= a; return c }

func mix(a, b Color, t float64) Color {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + .5) }
	return Color{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 1}
}

// Font to numer czcionki uzywanej w interfejsie.
type Font int

const (
	FTitle Font = iota
	FH2
	FBody
	FBodyB
	FSmall
	FSmallB
	FTiny
	FNum
	nFonts
)

// TextFlags steruja ustawieniem tekstu w prostokacie.
type TextFlags int

const (
	AlignL   TextFlags = 0
	AlignC   TextFlags = 1
	AlignR   TextFlags = 2
	VMid     TextFlags = 4
	Wrap     TextFlags = 8
	Ellipsis TextFlags = 16
)

// Canvas to wszystko, czego interfejs potrzebuje do rysowania.
// Wspolrzedne sa "logiczne" (przy 100% skali ekranu = piksele).
type Canvas interface {
	Clear(c Color)
	FillRRect(x, y, w, h, r float64, c Color)
	FillGrad(x, y, w, h, r float64, top, bot Color)
	FillCircle(cx, cy, r float64, c Color)
	StrokeRRect(x, y, w, h, r, t float64, c Color)
	PushClip(x, y, w, h float64)
	PopClip()
	Text(s string, x, y, w, h float64, f Font, c Color, fl TextFlags)
	Measure(s string, f Font) float64
	MeasureWrap(s string, w float64, f Font) float64
}

type irect struct{ x0, y0, x1, y1 int }

// Raster rysuje ksztalty programowo, z wygladzonymi krawedziami.
// Piksele maja uklad 0x00RRGGBB, czyli taki, jaki ma 32-bitowa bitmapa Windows.
type Raster struct {
	W, H  int
	Pix   []uint32
	S     float64
	clips []irect
}

func NewRaster(w, h int, s float64) *Raster {
	return &Raster{W: w, H: h, Pix: make([]uint32, w*h), S: s}
}

func (r *Raster) cur() irect {
	if n := len(r.clips); n > 0 {
		return r.clips[n-1]
	}
	return irect{0, 0, r.W, r.H}
}

func (r *Raster) PushClip(x, y, w, h float64) {
	s := r.S
	n := irect{int(math.Floor(x * s)), int(math.Floor(y * s)), int(math.Ceil((x + w) * s)), int(math.Ceil((y + h) * s))}
	c := r.cur()
	if n.x0 < c.x0 {
		n.x0 = c.x0
	}
	if n.y0 < c.y0 {
		n.y0 = c.y0
	}
	if n.x1 > c.x1 {
		n.x1 = c.x1
	}
	if n.y1 > c.y1 {
		n.y1 = c.y1
	}
	if n.x1 < n.x0 {
		n.x1 = n.x0
	}
	if n.y1 < n.y0 {
		n.y1 = n.y0
	}
	r.clips = append(r.clips, n)
}

func (r *Raster) PopClip() {
	if n := len(r.clips); n > 0 {
		r.clips = r.clips[:n-1]
	}
}

func pack(c Color) uint32 { return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B) }

func (r *Raster) Clear(c Color) {
	v := pack(c)
	for i := range r.Pix {
		r.Pix[i] = v
	}
}

func (r *Raster) blend(i int, c Color, cov float64) {
	a := c.A * cov
	if a <= 0.002 {
		return
	}
	if a >= 0.998 {
		r.Pix[i] = pack(c)
		return
	}
	d := r.Pix[i]
	dr, dg, db := float64(d>>16&255), float64(d>>8&255), float64(d&255)
	nr := dr + (float64(c.R)-dr)*a
	ng := dg + (float64(c.G)-dg)*a
	nb := db + (float64(c.B)-db)*a
	r.Pix[i] = uint32(nr+.5)<<16 | uint32(ng+.5)<<8 | uint32(nb+.5)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// rrect rysuje zaokraglony prostokat (wypelniony albo sam obrys) metoda pola odleglosci.
func (r *Raster) rrect(x, y, w, h, rad, stroke float64, top, bot Color, grad bool) {
	s := r.S
	x, y, w, h, rad, stroke = x*s, y*s, w*s, h*s, rad*s, stroke*s
	if w <= 0 || h <= 0 {
		return
	}
	if rad > w/2 {
		rad = w / 2
	}
	if rad > h/2 {
		rad = h / 2
	}
	cl := r.cur()
	x0, x1 := int(math.Floor(x)), int(math.Ceil(x+w))
	y0, y1 := int(math.Floor(y)), int(math.Ceil(y+h))
	if x0 < cl.x0 {
		x0 = cl.x0
	}
	if y0 < cl.y0 {
		y0 = cl.y0
	}
	if x1 > cl.x1 {
		x1 = cl.x1
	}
	if y1 > cl.y1 {
		y1 = cl.y1
	}
	cx, cy := x+w/2, y+h/2
	hx, hy := w/2-rad, h/2-rad
	// Piksele, ktore na pewno sa w calosci wewnatrz (pokrycie 100%), wypelniamy
	// odcinkami bez liczenia odleglosci. Liczymy tylko krawedzie.
	t := math.Min(0, rad-.5)
	for py := y0; py < y1; py++ {
		fy := float64(py) + .5
		qy := math.Abs(fy-cy) - hy
		col := top
		if grad {
			col = mix(top, bot, clamp01((fy-y)/h))
		}
		row := py * r.W
		fs0, fs1 := x1, x1
		if stroke == 0 && qy <= t && hx+t >= 0 {
			fs0 = int(math.Ceil(cx - (hx + t) - .5))
			fs1 = int(math.Floor(cx+(hx+t)-.5)) + 1
			if fs0 < x0 {
				fs0 = x0
			}
			if fs1 > x1 {
				fs1 = x1
			}
			if fs1 <= fs0 {
				fs0, fs1 = x1, x1
			}
		}
		for px := x0; px < x1; px++ {
			if px == fs0 {
				if col.A >= .998 {
					v := pack(col)
					for k := fs0; k < fs1; k++ {
						r.Pix[row+k] = v
					}
				} else {
					for k := fs0; k < fs1; k++ {
						r.blend(row+k, col, 1)
					}
				}
				px = fs1 - 1
				continue
			}
			qx := math.Abs(float64(px)+.5-cx) - hx
			var d float64
			if qx > 0 && qy > 0 {
				d = math.Hypot(qx, qy) - rad
			} else {
				d = math.Max(qx, qy) - rad
			}
			cov := clamp01(.5 - d)
			if stroke > 0 {
				cov *= clamp01(d + stroke + .5)
			}
			if cov > 0 {
				r.blend(row+px, col, cov)
			}
		}
	}
}

func (r *Raster) FillRRect(x, y, w, h, rad float64, c Color) {
	r.rrect(x, y, w, h, rad, 0, c, c, false)
}
func (r *Raster) FillGrad(x, y, w, h, rad float64, top, bot Color) {
	r.rrect(x, y, w, h, rad, 0, top, bot, true)
}
func (r *Raster) FillCircle(cx, cy, rad float64, c Color) {
	r.rrect(cx-rad, cy-rad, 2*rad, 2*rad, rad, 0, c, c, false)
}
func (r *Raster) StrokeRRect(x, y, w, h, rad, t float64, c Color) {
	r.rrect(x, y, w, h, rad, t, c, c, false)
}
