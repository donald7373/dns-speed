package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Tego bledu nie da sie zlapac kompilatorem ani poza Windowsem: gdy funkcja jest
// przypisana do zlej biblioteki DLL, program wywraca sie dopiero przy pierwszym
// jej uzyciu. Ten test porownuje kazde wywolanie z lista, w ktorej DLL naprawde
// sie ono znajduje (wg dokumentacji Microsoftu).
var procDLL = map[string]string{
	// user32.dll
	"RegisterClassExW": "user32", "CreateWindowExW": "user32", "DefWindowProcW": "user32",
	"DestroyWindow": "user32", "ShowWindow": "user32", "UpdateWindow": "user32",
	"GetMessageW": "user32", "TranslateMessage": "user32", "DispatchMessageW": "user32",
	"IsDialogMessageW": "user32", "PostQuitMessage": "user32", "PostMessageW": "user32",
	"SendMessageW": "user32", "GetClientRect": "user32", "MoveWindow": "user32",
	"SetWindowPos": "user32", "InvalidateRect": "user32", "BeginPaint": "user32",
	"EndPaint": "user32", "SetTimer": "user32", "KillTimer": "user32",
	"LoadCursorW": "user32", "LoadImageW": "user32", "GetSystemMetrics": "user32",
	"SystemParametersInfoW": "user32", "GetWindowTextW": "user32", "SetWindowTextW": "user32",
	"EnableWindow": "user32", "SetFocus": "user32", "SetScrollInfo": "user32",
	"GetScrollInfo": "user32", "AdjustWindowRectEx": "user32", "MessageBoxW": "user32",
	"GetDC": "user32", "ReleaseDC": "user32", "DrawTextW": "user32",
	"AdjustWindowRectExForDpi": "user32", "GetDpiForWindow": "user32", "GetDpiForSystem": "user32",
	// gdi32.dll
	"CreateCompatibleDC": "gdi32", "DeleteDC": "gdi32", "DeleteObject": "gdi32",
	"SelectObject": "gdi32", "CreateDIBSection": "gdi32", "BitBlt": "gdi32",
	"CreateFontW": "gdi32", "SetBkMode": "gdi32", "SetTextColor": "gdi32",
	"GetTextExtentPoint32W": "gdi32", "GetTextFaceW": "gdi32", "GdiFlush": "gdi32",
	"GetDeviceCaps": "gdi32",
	// kernel32.dll
	"GetModuleHandleW": "kernel32",
}

func TestWin32ProcsInRightDLL(t *testing.T) {
	src, err := os.ReadFile("win32_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(\w+)\.NewProc\("(\w+)"\)`)
	found := re.FindAllStringSubmatch(string(src), -1)
	if len(found) < 50 {
		t.Fatalf("znaleziono tylko %d wywolan, parser sie rozjechal", len(found))
	}
	seen := map[string]bool{}
	for _, m := range found {
		dll, proc := m[1], m[2]
		want, known := procDLL[proc]
		if !known {
			t.Errorf("%s: brak na liscie znanych funkcji. Dodaj ja po sprawdzeniu, w ktorym DLL jest", proc)
			continue
		}
		if dll != want {
			t.Errorf("%s: ladowana z %s.dll, a naprawde jest w %s.dll", proc, dll, want)
		}
		seen[proc] = true
	}
	for p := range procDLL {
		if !seen[p] && !strings.HasPrefix(p, "_") {
			t.Logf("uwaga: %s jest na liscie, ale nie jest uzywana", p)
		}
	}
}
