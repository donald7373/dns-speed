//go:build windows

package main

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"dnsspeed/internal/dnscore"
)

const (
	idStart = 101
	idApply = 102
	idAdd   = 103
	idCSV   = 104
	idReset = 105
	idEdit  = 106

	timerPoll = 1
	timerAnim = 2
	wmQueue   = wmApp + 1

	classMain = "DnsSpeedRaceMain"
	classPool = "DnsSpeedRacePool"

	defaultClientW = 1000.0
	defaultClientH = 780.0
)

// Cala interakcja z systemem dzieje sie w jednym watku (zablokowanym), bo okna
// Windows naleza do watku, ktory je utworzyl.
func init() { runtime.LockOSThread() }

func main() {
	if err := runApp(); err != nil {
		dnscore.Fatal("Nie udało się uruchomić programu: " + err.Error())
	}
}

type App struct {
	hInst, hwnd, hPool                        uintptr
	hStart, hApply, hAdd, hCSV, hReset, hEdit uintptr

	dpi   int
	scale float64
	fonts fontSet

	model *Model
	bench *dnscore.Bench
	L     Layout

	mainBuf, poolBuf *backbuf
	scrollPx         int

	queue     chan func()
	extra     []string
	busy      bool
	polling   bool
	animating bool
	last      time.Time
	panicked  bool
	cue       *uint16
}

var app *App

// lpToPtr zamienia LPARAM na wskaznik (bez ostrzezen go vet).
func lpToPtr(lp uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&lp)) }

func guard(a *App, f func() uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			if !a.panicked {
				a.panicked = true
				st := string(debug.Stack())
				if len(st) > 900 {
					st = st[:900]
				}
				messageBox(fmt.Sprintf("Wystąpił błąd wewnętrzny programu:\n%v\n\n%s", r, st))
			}
			ret = 0
		}
	}()
	return f()
}

func mainProc(hwnd, msg, wp, lp uintptr) uintptr {
	return guard(app, func() uintptr { return app.handleMain(hwnd, msg, wp, lp) })
}

func poolProc(hwnd, msg, wp, lp uintptr) uintptr {
	return guard(app, func() uintptr { return app.handlePool(hwnd, msg, wp, lp) })
}

func defProc(hwnd, msg, wp, lp uintptr) uintptr {
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// --- uruchomienie ---

func registerClass(name string, proc, icon uintptr) error {
	cur, _, _ := pLoadCursorW.Call(0, idcArrow)
	wc := wndClassEx{
		Style: csHRedraw | csVRedraw, WndProc: proc, Instance: app.hInst,
		Icon: icon, Cursor: cur, ClassName: u16(name),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 {
		return fmt.Errorf("RegisterClassEx(%s): %v", name, e)
	}
	return nil
}

func loadIcon(cx, cy int) uintptr {
	// Ikona programu to zasob nr 1 (wbudowany w plik .exe).
	h, _, _ := pLoadImageW.Call(app.hInst, 1, imageIcon, uintptr(cx), uintptr(cy), 0)
	return h
}

func systemDPI() int {
	if pGetDpiForSystem.Find() == nil {
		if d, _, _ := pGetDpiForSystem.Call(); d > 0 {
			return int(d)
		}
	}
	dc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, dc)
	if d, _, _ := pGetDeviceCaps.Call(dc, logPixelsX); d > 0 {
		return int(d)
	}
	return 96
}

func (a *App) adjust(rc *rect) {
	const style = wsOverlapped | wsClipChildren
	if pAdjustWindowRectExForDpi.Find() == nil {
		pAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(rc)), style, 0, 0, uintptr(a.dpi))
		return
	}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(rc)), style, 0, 0)
}

// outerSize zamienia rozmiar obszaru roboczego (px) na rozmiar calego okna.
func (a *App) outerSize(cw, ch float64) (int, int) {
	rc := rect{0, 0, int32(math.Round(cw)), int32(math.Round(ch))}
	a.adjust(&rc)
	return int(rc.Right - rc.Left), int(rc.Bottom - rc.Top)
}

func runApp() error {
	a := &App{
		model: NewModel(),
		bench: &dnscore.Bench{},
		queue: make(chan func(), 64),
	}
	app = a
	h, _, _ := pGetModuleHandleW.Call(0)
	a.hInst = h
	a.dpi = systemDPI()
	a.scale = float64(a.dpi) / 96

	on := int32(1)
	pSystemParametersInfoW.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&on)), 0)
	a.model.Animate = on != 0

	big := loadIcon(32, 32)
	if err := registerClass(classMain, syscall.NewCallback(mainProc), big); err != nil {
		return err
	}
	if err := registerClass(classPool, syscall.NewCallback(poolProc), 0); err != nil {
		return err
	}

	// Rozmiar startowy: 1000 x 780, ale nie wiekszy niz ekran (np. laptopy 1366 x 768).
	var work rect
	pSystemParametersInfoW.Call(0x0030, 0, uintptr(unsafe.Pointer(&work)), 0) // SPI_GETWORKAREA
	frameW, frameH := a.outerSize(0, 0)
	cw := math.Min(defaultClientW*a.scale, float64(int(work.Right-work.Left)-frameW))
	ch := math.Min(defaultClientH*a.scale, float64(int(work.Bottom-work.Top)-frameH))
	cw = math.Max(cw, minClientW*a.scale)
	ch = math.Max(ch, minClientH*a.scale)
	ow, oh := a.outerSize(cw, ch)

	const useDefault = 0x80000000
	hwnd, _, e := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(classMain))), uintptr(unsafe.Pointer(u16("Wyścig DNS"))),
		wsOverlapped|wsClipChildren, useDefault, useDefault, uintptr(ow), uintptr(oh), 0, 0, a.hInst, 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %v", e)
	}
	a.hwnd = hwnd
	if small := loadIcon(int(a.metric(smCxSmIcon)), int(a.metric(smCySmIcon))); small != 0 {
		sendMessage(hwnd, wmSetIcon, 0, small)
	}
	if big != 0 {
		sendMessage(hwnd, wmSetIcon, 1, big)
	}
	pShowWindow.Call(hwnd, swShowNormal)
	pUpdateWindow.Call(hwnd)
	pSetFocus.Call(a.hStart)

	var m msgT
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if m.Message == wmKeyDown && m.WParam == vkReturn && a.onEnter(m.Hwnd) {
			continue
		}
		if ok, _, _ := pIsDialogMessageW.Call(hwnd, uintptr(unsafe.Pointer(&m))); ok != 0 {
			continue
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func (a *App) metric(idx int) uintptr {
	r, _, _ := pGetSystemMetrics.Call(uintptr(idx))
	return r
}

// onEnter: Enter w polu adresu dodaje serwer, a na przycisku go klika.
func (a *App) onEnter(h uintptr) bool {
	switch h {
	case a.hEdit:
		a.onAdd()
		return true
	case a.hStart, a.hApply, a.hAdd, a.hCSV, a.hReset:
		sendMessage(h, bmClick, 0, 0)
		return true
	}
	return false
}

// --- okno glowne ---

func (a *App) handleMain(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmCreate:
		a.hwnd = hwnd
		a.onCreate()
		return 0
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		a.paintMain(hwnd)
		return 0
	case wmSize:
		if wp != 1 && a.hStart != 0 { // 1 = zminimalizowane
			a.applyLayout()
		}
		return 0
	case wmGetMinMaxInfo:
		mm := (*minMaxInfo)(lpToPtr(lp))
		w, h := a.outerSize(minClientW*a.scale, minClientH*a.scale)
		mm.MinTrack = point{int32(w), int32(h)}
		return 0
	case wmDpiChanged:
		a.onDpiChanged(wp, lp)
		return 0
	case wmDrawItem:
		return a.drawItem(lp)
	case wmCommand:
		if hiword(wp) == 0 { // BN_CLICKED
			switch loword(wp) {
			case idStart:
				a.onStart()
			case idApply:
				a.onApply()
			case idAdd:
				a.onAdd()
			case idCSV:
				a.onCSV()
			case idReset:
				a.onReset()
			}
		}
		return 0
	case wmTimer:
		switch wp {
		case timerPoll:
			a.onPoll()
		case timerAnim:
			a.onAnim()
		}
		return 0
	case wmMouseWheel:
		a.onWheel(wp)
		return 0
	case wmQueue:
		a.drain()
		return 0
	case wmDestroy:
		pPostQuitMessage.Call(0)
		return 0
	}
	return defProc(hwnd, msg, wp, lp)
}

func (a *App) mkButton(text string, id int) uintptr {
	h, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16("BUTTON"))), uintptr(unsafe.Pointer(u16(text))),
		wsChild|wsVisible|wsTabStop|bsOwnerDraw, 0, 0, 10, 10, a.hwnd, uintptr(id), a.hInst, 0)
	return h
}

func (a *App) onCreate() {
	dpi := a.dpi
	if pGetDpiForWindow.Find() == nil {
		if d, _, _ := pGetDpiForWindow.Call(a.hwnd); d > 0 {
			dpi = int(d)
		}
	}
	a.dpi = dpi
	a.scale = float64(dpi) / 96
	a.fonts = makeFonts(a.scale)

	a.hStart = a.mkButton("Zacznij test", idStart)
	a.hApply = a.mkButton("", idApply)
	h, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16("EDIT"))), 0,
		wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll, 0, 0, 10, 10, a.hwnd, idEdit, a.hInst, 0)
	a.hEdit = h
	a.hAdd = a.mkButton("Dodaj do wyścigu", idAdd)
	a.hCSV = a.mkButton("Zapisz wyniki (CSV)", idCSV)
	a.hReset = a.mkButton("Przywróć automatyczny DNS", idReset)
	pEnableWindow.Call(a.hCSV, 0)

	a.cue = u16("np. 192.168.0.1")
	sendMessage(a.hEdit, emLimitText, 15, 0)
	sendMessage(a.hEdit, emSetCueBanr, 1, uintptr(unsafe.Pointer(a.cue)))
	sendMessage(a.hEdit, wmSetFont, a.fonts.h[FBody], 1)

	p, _, _ := pCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(classPool))), 0,
		wsChild|wsVisible|wsVScroll|wsClipSibling, 0, 0, 10, 10, a.hwnd, 0, a.hInst, 0)
	a.hPool = p

	a.applyLayout()
	a.startLoading()
}

func (a *App) onDpiChanged(wp, lp uintptr) {
	a.dpi = hiword(wp)
	a.scale = float64(a.dpi) / 96
	if rc := (*rect)(lpToPtr(lp)); rc != nil {
		pSetWindowPos.Call(a.hwnd, 0, uintptr(rc.Left), uintptr(rc.Top),
			uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), swpNoZOrder|swpNoActive)
	}
	old := a.fonts
	a.fonts = makeFonts(a.scale)
	sendMessage(a.hEdit, wmSetFont, a.fonts.h[FBody], 1)
	old.free()
	a.applyLayout()
}

func (a *App) px(v float64) uintptr { return uintptr(int32(math.Round(v * a.scale))) }

func (a *App) place(h uintptr, r Rect) {
	pMoveWindow.Call(h, a.px(r.X), a.px(r.Y), a.px(r.W), a.px(r.H), 1)
}

// applyLayout ustawia prawdziwe kontrolki zgodnie z ComputeLayout i odswieza okno.
func (a *App) applyLayout() {
	rc := clientRect(a.hwnd)
	if rc.Right <= 0 || rc.Bottom <= 0 {
		return
	}
	W, H := float64(rc.Right)/a.scale, float64(rc.Bottom)/a.scale
	a.L = ComputeLayout(W, H, a.model.Verdict)
	L := a.L

	a.place(a.hStart, L.Start)
	label := "Zacznij test"
	if a.model.Finished {
		label = "Powtórz test"
	}
	if windowText(a.hStart) != label {
		setWindowText(a.hStart, label)
	}
	if L.ShowApply {
		a.place(a.hApply, L.Apply)
		if windowText(a.hApply) != a.model.Verdict.ApplyLabel {
			setWindowText(a.hApply, a.model.Verdict.ApplyLabel)
		}
		pShowWindow.Call(a.hApply, swShowNormal)
	} else {
		pShowWindow.Call(a.hApply, swHide)
	}
	// Pole tekstowe ma standardowa wysokosc, wysrodkowane w rzedzie.
	a.place(a.hEdit, Rect{L.FEdit.X, L.FEdit.Y + 3, L.FEdit.W, L.FEdit.H - 6})
	a.place(a.hAdd, L.FAdd)
	a.place(a.hCSV, L.FCSV)
	a.place(a.hReset, L.FReset)
	a.place(a.hPool, L.Pool)

	a.updateScroll()
	a.invalidate()
}

func (a *App) invalidate() {
	pInvalidateRect.Call(a.hwnd, 0, 0)
	pInvalidateRect.Call(a.hPool, 0, 0)
}

func (a *App) invalidateMain() { pInvalidateRect.Call(a.hwnd, 0, 0) }
func (a *App) invalidatePool() { pInvalidateRect.Call(a.hPool, 0, 0) }

func (a *App) paintMain(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	rc := clientRect(hwnd)
	w, h := int(rc.Right), int(rc.Bottom)
	if hdc == 0 || w <= 0 || h <= 0 {
		return
	}
	a.mainBuf = ensureBuf(a.mainBuf, w, h)
	if a.mainBuf == nil {
		return
	}
	PaintMain(newWinCanvas(a.mainBuf, &a.fonts, a.scale), a.model, a.L)
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), a.mainBuf.dc, 0, 0, srcCopy)
}

func (a *App) paintPool(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	defer pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	rc := clientRect(hwnd)
	w, h := int(rc.Right), int(rc.Bottom)
	if hdc == 0 || w <= 0 || h <= 0 {
		return
	}
	a.poolBuf = ensureBuf(a.poolBuf, w, h)
	if a.poolBuf == nil {
		return
	}
	cv := newWinCanvas(a.poolBuf, &a.fonts, a.scale)
	PaintPool(cv, a.model, float64(w)/a.scale, float64(h)/a.scale, float64(a.scrollPx)/a.scale)
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), a.poolBuf.dc, 0, 0, srcCopy)
}

// drawItem rysuje przycisk (BS_OWNERDRAW) naszym rysownikiem.
func (a *App) drawItem(lp uintptr) uintptr {
	dis := (*drawItemStruct)(lpToPtr(lp))
	w, h := int(dis.Rc.Right-dis.Rc.Left), int(dis.Rc.Bottom-dis.Rc.Top)
	buf := newBackbuf(w, h)
	if buf == nil {
		return 1
	}
	defer buf.free()
	kind := BtnGhost
	if dis.CtlID == idStart || dis.CtlID == idApply {
		kind = BtnPrimary
	}
	st := dis.ItemState
	PaintButton(newWinCanvas(buf, &a.fonts, a.scale), float64(w)/a.scale, float64(h)/a.scale,
		windowText(dis.Item), kind, st&odsSelected != 0, st&odsFocus != 0 && st&odsNoFocusRct == 0, st&odsDisabled != 0)
	pBitBlt.Call(dis.HDC, uintptr(dis.Rc.Left), uintptr(dis.Rc.Top), uintptr(w), uintptr(h), buf.dc, 0, 0, srcCopy)
	return 1
}

// --- basen: przewijanie ---

func (a *App) handlePool(hwnd, msg, wp, lp uintptr) uintptr {
	switch msg {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		a.paintPool(hwnd)
		return 0
	case wmVScroll:
		a.onVScroll(wp)
		return 0
	case wmMouseWheel:
		a.onWheel(wp)
		return 0
	}
	return defProc(hwnd, msg, wp, lp)
}

func (a *App) poolHeight() int { return int(clientRect(a.hPool).Bottom) }

func (a *App) contentPx() int { return int(math.Ceil(a.model.ContentH() * a.scale)) }

func (a *App) maxScroll() int {
	m := a.contentPx() - a.poolHeight()
	if m < 0 {
		return 0
	}
	return m
}

func (a *App) updateScroll() {
	ph := a.poolHeight()
	content := a.contentPx()
	if a.scrollPx > a.maxScroll() {
		a.scrollPx = a.maxScroll()
	}
	si := scrollInfo{Mask: sifRange | sifPage | sifPos, Min: 0, Max: int32(content - 1), Page: uint32(ph), Pos: int32(a.scrollPx)}
	if content <= 0 {
		si.Max = 0
	}
	si.Size = uint32(unsafe.Sizeof(si))
	pSetScrollInfo.Call(a.hPool, sbVert, uintptr(unsafe.Pointer(&si)), 1)
}

func (a *App) scrollTo(pos int) {
	if pos > a.maxScroll() {
		pos = a.maxScroll()
	}
	if pos < 0 {
		pos = 0
	}
	if pos == a.scrollPx {
		return
	}
	a.scrollPx = pos
	si := scrollInfo{Mask: sifPos, Pos: int32(pos)}
	si.Size = uint32(unsafe.Sizeof(si))
	pSetScrollInfo.Call(a.hPool, sbVert, uintptr(unsafe.Pointer(&si)), 1)
	a.invalidatePool()
}

func (a *App) onVScroll(wp uintptr) {
	line := int(laneH * a.scale / 2)
	page := a.poolHeight() - line
	pos := a.scrollPx
	switch loword(wp) {
	case sbLineUp:
		pos -= line
	case sbLineDown:
		pos += line
	case sbPageUp:
		pos -= page
	case sbPageDown:
		pos += page
	case sbTop:
		pos = 0
	case sbBottom:
		pos = a.maxScroll()
	case sbThumbTrk, sbThumbPos:
		si := scrollInfo{Mask: sifTrack}
		si.Size = uint32(unsafe.Sizeof(si))
		pGetScrollInfo.Call(a.hPool, sbVert, uintptr(unsafe.Pointer(&si)))
		pos = int(si.TrackPos)
	}
	a.scrollTo(pos)
}

func (a *App) onWheel(wp uintptr) {
	delta := int(int16(uint16(hiword(wp))))
	a.scrollTo(a.scrollPx - int(float64(delta)/120*laneH*a.scale*1.4))
}

// ensureVisible przewija basen tak, by pokazac tor, ktory wlasnie jest testowany.
func (a *App) ensureVisible(lv *laneView) {
	top := int(lv.TY * a.scale)
	bottom := int((lv.TY + laneH) * a.scale)
	switch {
	case top < a.scrollPx:
		a.scrollTo(top)
	case bottom > a.scrollPx+a.poolHeight():
		a.scrollTo(bottom - a.poolHeight())
	}
}

// --- praca w tle ---

// post wykonuje funkcje w watku okna (bezpieczne do wywolania z innych watkow).
func (a *App) post(f func()) {
	select {
	case a.queue <- f:
	default:
		go func() { a.queue <- f }()
	}
	pPostMessageW.Call(a.hwnd, wmQueue, 0, 0)
}

func (a *App) drain() {
	for {
		select {
		case f := <-a.queue:
			f()
		default:
			return
		}
	}
}

func (a *App) setBusy(b bool) {
	a.busy = b
	for _, h := range []uintptr{a.hStart, a.hAdd, a.hEdit, a.hReset, a.hApply} {
		pEnableWindow.Call(h, b2u(!b))
	}
	pEnableWindow.Call(a.hCSV, b2u(!b && a.model.Finished))
}

func (a *App) startLoading() {
	go func() {
		list := dnscore.BuildServers(nil) // wywoluje PowerShell, wiec nie w watku okna
		a.post(func() {
			if a.busy || a.model.Running || a.model.Finished {
				return
			}
			m := a.model
			m.SetServers(list)
			for _, ip := range a.extra {
				m.AddCustom(ip)
			}
			m.Status = "Gotowe do startu. Test trwa około minuty."
			a.applyLayout()
		})
	}()
}

func (a *App) startPolling() {
	if !a.polling {
		a.polling = true
		pSetTimer.Call(a.hwnd, timerPoll, 250, 0)
	}
	a.ensureAnim()
}

func (a *App) ensureAnim() {
	if !a.animating {
		a.animating = true
		a.last = time.Now()
		pSetTimer.Call(a.hwnd, timerAnim, 16, 0)
	}
}

func (a *App) onAnim() {
	now := time.Now()
	dt := now.Sub(a.last).Seconds()
	a.last = now
	if dt > .1 {
		dt = .1
	}
	m := a.model
	moving := m.Step(dt)
	a.invalidatePool()
	if math.Abs(m.ProgShown-m.Progress) > .001 {
		a.invalidateMain()
	}
	if !moving {
		pKillTimer.Call(a.hwnd, timerAnim)
		a.animating = false
		a.invalidateMain()
	}
}

func (a *App) onPoll() {
	st := a.bench.Snapshot()
	a.model.Update(st)
	if st.Running {
		for _, lv := range a.model.Lanes {
			if lv.R.State == "testing" {
				a.ensureVisible(lv)
				break
			}
		}
	} else {
		pKillTimer.Call(a.hwnd, timerPoll)
		a.polling = false
		a.setBusy(false)
		a.applyLayout()
		a.scrollTo(0)
	}
	a.ensureAnim()
	a.invalidateMain()
}

// --- przyciski ---

func (a *App) onStart() {
	if a.busy {
		return
	}
	a.setBusy(true)
	m := a.model
	m.Verdict, m.Finished, m.Running = nil, false, false
	m.Progress, m.ProgShown = 0, 0
	m.Status = "Przygotowuję listę serwerów…"
	m.SetMsg(0, "")
	a.applyLayout()
	extra := append([]string(nil), a.extra...)
	go func() {
		list := dnscore.BuildServers(extra)
		started := a.bench.Start(list)
		a.post(func() {
			if !started {
				m.Status = "Test już trwa."
				a.setBusy(false)
				a.invalidateMain()
				return
			}
			m.SetServers(list)
			m.Running = true
			m.Status = "Uruchamiam test…"
			a.scrollTo(0)
			a.updateScroll()
			a.invalidate()
			a.startPolling()
		})
	}()
}

func (a *App) editText() string { return strings.TrimSpace(windowText(a.hEdit)) }

func (a *App) onAdd() {
	if a.busy {
		return
	}
	ip, msg := a.model.AddCustom(a.editText())
	if msg != "" {
		a.model.SetMsg(2, msg)
		a.invalidateMain()
		pSetFocus.Call(a.hEdit)
		return
	}
	a.extra = append(a.extra, ip)
	setWindowText(a.hEdit, "")
	a.model.SetMsg(1, "Dodano "+ip+". Uruchom test, aby go sprawdzić.")
	a.updateScroll()
	a.scrollTo(a.maxScroll())
	a.invalidate()
}

func (a *App) afterDNSChange(ok string, cur []string, err error) {
	a.setBusy(false)
	if err != nil {
		a.model.SetMsg(2, err.Error())
	} else {
		if len(cur) > 0 {
			ok += " Obecnie: " + strings.Join(cur, ", ") + "."
		}
		a.model.SetMsg(1, ok)
	}
	a.invalidateMain()
}

func (a *App) onApply() {
	v := a.model.Verdict
	if a.busy || v == nil || !v.CanApply {
		return
	}
	ips := append([]string(nil), v.ApplyIPs...)
	a.setBusy(true)
	a.model.SetMsg(0, "Czekam na zgodę administratora w oknie Windows…")
	a.invalidateMain()
	go func() {
		cur, err := dnscore.ApplyDNS(ips)
		a.post(func() { a.afterDNSChange("Gotowe. DNS ustawiony na "+strings.Join(ips, " i ")+".", cur, err) })
	}()
}

func (a *App) onReset() {
	if a.busy {
		return
	}
	a.setBusy(true)
	a.model.SetMsg(0, "Czekam na zgodę administratora w oknie Windows…")
	a.invalidateMain()
	go func() {
		cur, err := dnscore.ResetDNS()
		a.post(func() { a.afterDNSChange("Przywrócono automatyczny DNS.", cur, err) })
	}()
}

// csvPath: obok programu, a gdy tam nie da sie zapisac, w Dokumentach.
func csvPath() string {
	name := "wyniki-dns.csv"
	var dirs []string
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Documents"), home)
	}
	dirs = append(dirs, os.TempDir())
	for _, d := range dirs {
		p := filepath.Join(d, name)
		if f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644); err == nil {
			f.Close()
			return p
		}
	}
	return filepath.Join(os.TempDir(), name)
}

func (a *App) onCSV() {
	if a.busy || !a.model.Finished {
		return
	}
	path := csvPath()
	if err := os.WriteFile(path, []byte(buildCSV(a.model.Lanes)), 0o644); err != nil {
		a.model.SetMsg(2, "Nie udało się zapisać pliku: "+err.Error())
		a.invalidateMain()
		return
	}
	a.model.SetMsg(1, "Zapisano wyniki: "+path)
	a.invalidateMain()
	// Pokaz plik w Eksploratorze (zaznaczony).
	cmd := exec.Command("explorer.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer.exe /select,"` + path + `"`}
	_ = cmd.Start()
}
