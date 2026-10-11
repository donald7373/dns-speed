//go:build windows

package main

import (
	"math"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

// backbuf to bitmapa 32-bitowa w pamieci, w ktorej rysujemy cale okno,
// a potem jednym BitBlt wysylamy ja na ekran (bez migotania).
type backbuf struct {
	dc, bmp, old uintptr
	w, h         int
	pix          []uint32
}

func newBackbuf(w, h int) *backbuf {
	if w <= 0 || h <= 0 {
		return nil
	}
	dc, _, _ := pCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil
	}
	bi := bitmapInfo{H: bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}}
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		pDeleteDC.Call(dc)
		return nil
	}
	old, _, _ := pSelectObject.Call(dc, bmp)
	pSetBkMode.Call(dc, transpar)
	return &backbuf{dc: dc, bmp: bmp, old: old, w: w, h: h, pix: unsafe.Slice((*uint32)(bits), w*h)}
}

func (b *backbuf) free() {
	if b == nil {
		return
	}
	pSelectObject.Call(b.dc, b.old)
	pDeleteObject.Call(b.bmp)
	pDeleteDC.Call(b.dc)
}

// ensureBuf zwraca bufor o wymaganym rozmiarze (tworzy nowy, gdy sie zmienil).
func ensureBuf(b *backbuf, w, h int) *backbuf {
	if b != nil && b.w == w && b.h == h {
		return b
	}
	b.free()
	return newBackbuf(w, h)
}

// --- czcionki ---

type fontSet struct{ h [nFonts]uintptr }

func faceOf(dc, font uintptr) string {
	old, _, _ := pSelectObject.Call(dc, font)
	buf := make([]uint16, 64)
	pGetTextFaceW.Call(dc, uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0])))
	pSelectObject.Call(dc, old)
	return syscall.UTF16ToString(buf)
}

func createFont(face string, weight int, px float64) uintptr {
	h := -int(math.Round(px))
	const (
		defaultCharset = 1
		clearType      = 5
	)
	f, _, _ := pCreateFontW.Call(uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0,
		defaultCharset, 0, 0, clearType, 0, uintptr(unsafe.Pointer(u16(face))))
	return f
}

// makeFonts tworzy wszystkie czcionki dla danej skali. Gdy Windows nie ma
// wybranej kroju (np. Bahnschrift w starszych systemach), bierze zapasowy.
func makeFonts(scale float64) fontSet {
	dc, _, _ := pCreateCompatibleDC.Call(0)
	defer pDeleteDC.Call(dc)
	var fs fontSet
	for i, sp := range fontSpecs {
		px := sp.Px * scale
		f := createFont(sp.Face, sp.Weight, px)
		if f == 0 || !strings.EqualFold(faceOf(dc, f), sp.Face) {
			if f != 0 {
				pDeleteObject.Call(f)
			}
			f = createFont(sp.FallFace, sp.FallWeight, px)
		}
		fs.h[i] = f
	}
	return fs
}

func (fs *fontSet) free() {
	for i, f := range fs.h {
		if f != 0 {
			pDeleteObject.Call(f)
			fs.h[i] = 0
		}
	}
}

// --- rysowanie ---

// winCanvas laczy nasz Raster (ksztalty z wygladzaniem) z GDI (tekst ClearType).
type winCanvas struct {
	*Raster
	b  *backbuf
	fs *fontSet
}

func newWinCanvas(b *backbuf, fs *fontSet, scale float64) *winCanvas {
	return &winCanvas{Raster: &Raster{W: b.w, H: b.h, Pix: b.pix, S: scale}, b: b, fs: fs}
}

func (c *winCanvas) devRect(x, y, w, h float64) rect {
	s := c.S
	return rect{
		int32(math.Floor(x * s)), int32(math.Floor(y * s)),
		int32(math.Ceil((x + w) * s)), int32(math.Ceil((y + h) * s)),
	}
}

func toUTF16(s string) []uint16 { return utf16.Encode([]rune(s)) }

func (c *winCanvas) Text(s string, x, y, w, h float64, f Font, col Color, fl TextFlags) {
	if s == "" || w <= 0 || h <= 0 {
		return
	}
	u := append(toUTF16(s), 0)
	rc := c.devRect(x, y, w, h)
	pSelectObject.Call(c.b.dc, c.fs.h[f])
	pSetTextColor.Call(c.b.dc, uintptr(col.R)|uintptr(col.G)<<8|uintptr(col.B)<<16)
	flags := uintptr(dtNoPrefix)
	switch fl & 3 {
	case AlignC:
		flags |= dtCenter
	case AlignR:
		flags |= dtRight
	}
	if fl&Wrap != 0 {
		flags |= dtWordBreak
	} else {
		flags |= dtSingle
		if fl&VMid != 0 {
			flags |= dtVCenter
		}
		if fl&Ellipsis != 0 {
			flags |= dtEndEllips
		}
	}
	pDrawTextW.Call(c.b.dc, uintptr(unsafe.Pointer(&u[0])), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), flags)
	pGdiFlush.Call() // zanim Raster znowu zapisze do tej samej pamieci
}

func (c *winCanvas) Measure(s string, f Font) float64 {
	if s == "" {
		return 0
	}
	u := toUTF16(s)
	pSelectObject.Call(c.b.dc, c.fs.h[f])
	var sz sizeT
	pGetTextExtentPoint32W.Call(c.b.dc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)), uintptr(unsafe.Pointer(&sz)))
	return float64(sz.CX) / c.S
}

func (c *winCanvas) MeasureWrap(s string, w float64, f Font) float64 {
	if s == "" {
		return 0
	}
	u := append(toUTF16(s), 0)
	rc := rect{0, 0, int32(math.Ceil(w * c.S)), 0}
	pSelectObject.Call(c.b.dc, c.fs.h[f])
	pDrawTextW.Call(c.b.dc, uintptr(unsafe.Pointer(&u[0])), ^uintptr(0), uintptr(unsafe.Pointer(&rc)), dtCalcRect|dtWordBreak|dtNoPrefix)
	return float64(rc.Bottom) / c.S
}
