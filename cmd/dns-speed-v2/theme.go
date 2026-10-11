package main

// Paleta jest ta sama co w v1 i w ikonie: kobalt, zloto zwyciezcy, woda basenu.
var (
	cInk      = RGB(0x0f2038)
	cPaper    = RGB(0xedf3f9)
	cMuted    = RGB(0x4f6078)
	cLaneText = RGB(0x33475f)
	cCobalt   = RGB(0x2457f5)
	cCobaltDk = RGB(0x1a42c4)
	cGold     = RGB(0xf2b705)
	cGoldInk  = RGB(0x4a3500)
	cRed      = RGB(0xb8322a)
	cGreen    = RGB(0x1b6b3a)
	cWaterA   = RGB(0xd9eaf8)
	cWaterB   = RGB(0xbcd8ee)
	cAmberBG  = RGB(0xffe8a3)
	cAmberInk = RGB(0x5c4400)
	cWhite    = RGB(0xffffff)
)

// fontSpec opisuje czcionke. Jesli Windows nie ma pierwszej, uzywamy zapasowej.
type fontSpec struct {
	Face       string
	Weight     int
	FallFace   string
	FallWeight int
	Px         float64
}

var fontSpecs = [nFonts]fontSpec{
	FTitle:  {"Bahnschrift", 700, "Segoe UI", 700, 40},
	FH2:     {"Segoe UI Semibold", 400, "Segoe UI", 700, 24},
	FBody:   {"Segoe UI", 400, "Segoe UI", 400, 15},
	FBodyB:  {"Segoe UI Semibold", 400, "Segoe UI", 700, 15},
	FSmall:  {"Segoe UI", 400, "Segoe UI", 400, 13},
	FSmallB: {"Segoe UI Semibold", 400, "Segoe UI", 700, 13},
	FTiny:   {"Segoe UI", 400, "Segoe UI", 400, 12},
	FNum:    {"Segoe UI", 400, "Segoe UI", 400, 13},
}
