//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// Cienka warstwa nad Win32. Bez zewnetrznych bibliotek: tylko syscall.

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	pRegisterClassExW      = user32.NewProc("RegisterClassExW")
	pCreateWindowExW       = user32.NewProc("CreateWindowExW")
	pDefWindowProcW        = user32.NewProc("DefWindowProcW")
	pDestroyWindow         = user32.NewProc("DestroyWindow")
	pShowWindow            = user32.NewProc("ShowWindow")
	pUpdateWindow          = user32.NewProc("UpdateWindow")
	pGetMessageW           = user32.NewProc("GetMessageW")
	pTranslateMessage      = user32.NewProc("TranslateMessage")
	pDispatchMessageW      = user32.NewProc("DispatchMessageW")
	pIsDialogMessageW      = user32.NewProc("IsDialogMessageW")
	pPostQuitMessage       = user32.NewProc("PostQuitMessage")
	pPostMessageW          = user32.NewProc("PostMessageW")
	pSendMessageW          = user32.NewProc("SendMessageW")
	pGetClientRect         = user32.NewProc("GetClientRect")
	pMoveWindow            = user32.NewProc("MoveWindow")
	pSetWindowPos          = user32.NewProc("SetWindowPos")
	pInvalidateRect        = user32.NewProc("InvalidateRect")
	pBeginPaint            = user32.NewProc("BeginPaint")
	pEndPaint              = user32.NewProc("EndPaint")
	pSetTimer              = user32.NewProc("SetTimer")
	pKillTimer             = user32.NewProc("KillTimer")
	pLoadCursorW           = user32.NewProc("LoadCursorW")
	pLoadImageW            = user32.NewProc("LoadImageW")
	pGetSystemMetrics      = user32.NewProc("GetSystemMetrics")
	pSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	pGetWindowTextW        = user32.NewProc("GetWindowTextW")
	pSetWindowTextW        = user32.NewProc("SetWindowTextW")
	pEnableWindow          = user32.NewProc("EnableWindow")
	pSetFocus              = user32.NewProc("SetFocus")
	pSetScrollInfo         = user32.NewProc("SetScrollInfo")
	pGetScrollInfo         = user32.NewProc("GetScrollInfo")
	pAdjustWindowRectEx    = user32.NewProc("AdjustWindowRectEx")
	pMessageBoxW           = user32.NewProc("MessageBoxW")
	pGetDC                 = user32.NewProc("GetDC")
	pReleaseDC             = user32.NewProc("ReleaseDC")
	pDrawTextW             = user32.NewProc("DrawTextW") // uwaga: DrawText jest w user32, nie w gdi32

	// Dostepne dopiero od Windows 10 (1607). Sprawdzamy przez Find().
	pAdjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")
	pGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	pGetDpiForSystem          = user32.NewProc("GetDpiForSystem")

	pCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	pDeleteDC              = gdi32.NewProc("DeleteDC")
	pDeleteObject          = gdi32.NewProc("DeleteObject")
	pSelectObject          = gdi32.NewProc("SelectObject")
	pCreateDIBSection      = gdi32.NewProc("CreateDIBSection")
	pBitBlt                = gdi32.NewProc("BitBlt")
	pCreateFontW           = gdi32.NewProc("CreateFontW")
	pSetBkMode             = gdi32.NewProc("SetBkMode")
	pSetTextColor          = gdi32.NewProc("SetTextColor")
	pGetTextExtentPoint32W = gdi32.NewProc("GetTextExtentPoint32W")
	pGetTextFaceW          = gdi32.NewProc("GetTextFaceW")
	pGdiFlush              = gdi32.NewProc("GdiFlush")
	pGetDeviceCaps         = gdi32.NewProc("GetDeviceCaps")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

// Komunikaty.
const (
	wmCreate        = 0x0001
	wmDestroy       = 0x0002
	wmSize          = 0x0005
	wmPaint         = 0x000F
	wmEraseBkgnd    = 0x0014
	wmGetMinMaxInfo = 0x0024
	wmSetFont       = 0x0030
	wmSetIcon       = 0x0080
	wmDrawItem      = 0x002B
	wmCommand       = 0x0111
	wmTimer         = 0x0113
	wmVScroll       = 0x0115
	wmKeyDown       = 0x0100
	wmMouseWheel    = 0x020A
	wmDpiChanged    = 0x02E0
	wmApp           = 0x8000

	bmClick       = 0x00F5
	emLimitText   = 0x00C5
	emSetCueBanr  = 0x1501
	vkReturn      = 0x0D
	odsSelected   = 0x0001
	odsDisabled   = 0x0004
	odsFocus      = 0x0010
	odsNoFocusRct = 0x0200
)

// Style okien.
const (
	wsChild        = 0x40000000
	wsVisible      = 0x10000000
	wsTabStop      = 0x00010000
	wsBorder       = 0x00800000
	wsVScroll      = 0x00200000
	wsOverlapped   = 0x00CF0000 // WS_OVERLAPPEDWINDOW
	wsClipChildren = 0x02000000
	wsClipSibling  = 0x04000000
	bsOwnerDraw    = 0x0000000B
	esAutoHScroll  = 0x0080

	csHRedraw = 0x0002
	csVRedraw = 0x0001

	swHide       = 0
	swShowNormal = 1
	swpNoZOrder  = 0x0004
	swpNoActive  = 0x0010

	sbVert     = 1
	sifRange   = 0x0001
	sifPage    = 0x0002
	sifPos     = 0x0004
	sifTrack   = 0x0010
	sbLineUp   = 0
	sbLineDown = 1
	sbPageUp   = 2
	sbPageDown = 3
	sbThumbPos = 4
	sbThumbTrk = 5
	sbTop      = 6
	sbBottom   = 7

	idcArrow                  = 32512
	imageIcon                 = 1
	smCxSmIcon                = 49
	smCySmIcon                = 50
	spiGetClientAreaAnimation = 0x1042

	srcCopy    = 0x00CC0020
	transpar   = 1
	logPixelsX = 88

	dtLeft      = 0x0000
	dtCenter    = 0x0001
	dtRight     = 0x0002
	dtVCenter   = 0x0004
	dtWordBreak = 0x0010
	dtSingle    = 0x0020
	dtCalcRect  = 0x0400
	dtNoPrefix  = 0x0800
	dtEndEllips = 0x8000

	mbIconError = 0x00000010
)

type rect struct{ Left, Top, Right, Bottom int32 }
type point struct{ X, Y int32 }
type sizeT struct{ CX, CY int32 }

type msgT struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     rect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type scrollInfo struct {
	Size, Mask uint32
	Min, Max   int32
	Page       uint32
	Pos        int32
	TrackPos   int32
}

type drawItemStruct struct {
	CtlType, CtlID, ItemID, ItemAction, ItemState uint32
	Item                                          uintptr
	HDC                                           uintptr
	Rc                                            rect
	ItemData                                      uintptr
}

type minMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrack, MaxTrack point
}

type bitmapInfoHeader struct {
	Size         uint32
	Width        int32
	Height       int32
	Planes       uint16
	BitCount     uint16
	Compression  uint32
	SizeImage    uint32
	XPels, YPels int32
	ClrUsed      uint32
	ClrImportant uint32
}

type bitmapInfo struct {
	H      bitmapInfoHeader
	Colors [4]byte
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("?")
	}
	return p
}

func loword(v uintptr) int { return int(v & 0xFFFF) }
func hiword(v uintptr) int { return int((v >> 16) & 0xFFFF) }

func b2u(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

func sendMessage(h uintptr, msg uint32, wp, lp uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(h, uintptr(msg), wp, lp)
	return r
}

func windowText(h uintptr) string {
	buf := make([]uint16, 256)
	n, _, _ := pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

func setWindowText(h uintptr, s string) {
	pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s))))
}

func clientRect(h uintptr) rect {
	var rc rect
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&rc)))
	return rc
}

func messageBox(text string) {
	pMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16("Wyścig DNS"))), mbIconError)
}
