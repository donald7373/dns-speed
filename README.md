# Wyścig DNS

Darmowy program do Windows, który sprawdza, który serwer DNS odpowiada najszybciej na Twoim łączu.
Każdy serwer biegnie na własnym torze, a wyniki widać na żywo.

Ikona: `assets/icon.ico` (podgląd: `assets/icon-1024.png`).

## Dwie wersje

| | Wersja | Interfejs |
|---|---|---|
| `cmd/dns-speed-v1` | v1 | Otwiera się w przeglądarce (lokalny serwer na `127.0.0.1`, chroniony kluczem sesji) |
| `cmd/dns-speed-v2` | v2 | Własne okno Windows, bez przeglądarki (czyste Win32, bez zewnętrznych bibliotek) |

Obie korzystają z tego samego silnika testu (`internal/dnscore`).

## Co robi

- Testuje ok. 16 serwerów (Cloudflare, Google, Quad9, OpenDNS, AdGuard i inne) oraz DNS-y ustawione w systemie.
- Mierzy dwie rzeczy: czas odpowiedzi dla popularnych stron (z pamięci serwera) i dla losowych nowych nazw (serwer musi zapytać dalej).
- Wykrywa serwery, które nie odpowiadają albo podmieniają nieistniejące adresy. Takie nie mogą wygrać.
- Może ustawić dwa najszybsze serwery w Windows (okno zgody administratora) i cofnąć zmianę.
- Pozwala dodać własny serwer i zapisać wyniki do CSV.

## Budowanie

Wymagane: Go 1.24+ i Python 3 (tylko do zasobów Windows). Budowanie działa też na Linuksie i macOS.

```sh
# 1. Ikona i manifest wbudowane w plik .exe (pliki .syso są w repozytorium, to tylko ich odtworzenie)
python3 tools/mksyso.py assets/icon.ico cmd/dns-speed-v1/rsrc_windows_amd64.syso assets/app.manifest
python3 tools/mksyso.py assets/icon.ico cmd/dns-speed-v2/rsrc_windows_amd64.syso assets/app.manifest

# 2. Kompilacja
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui -s -w" -o dist/DNS-Speed-v1.exe ./cmd/dns-speed-v1
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-H=windowsgui -s -w" -o dist/DNS-Speed-v2.exe ./cmd/dns-speed-v2
```

## Testy

```sh
go test ./...
```

Testy sprawdzają silnik na lokalnych, fałszywych serwerach DNS, logikę okna v2 i to, że każde wywołanie Win32
jest przypisane do właściwej biblioteki DLL. Same wywołania systemowe da się wykonać tylko w Windows.

`PREVIEW_DIR=./podglad go test -run TestPreview ./cmd/dns-speed-v2` zapisuje obrazy okna v2, a
`python3 tools/overlay.py podglad/<plik>.png` nakłada na nie tekst (do oglądania układu bez Windows).

## Uwagi

- Pliki .exe nie są podpisane, więc SmartScreen może ostrzec o „nierozpoznanej aplikacji".
- Różnice rzędu kilku milisekund nie są odczuwalne podczas przeglądania. Wynik zależy od łącza i pory dnia.
- Zmiana DNS wywołuje PowerShell (`Set-DnsClientServerAddress`) z prośbą o uprawnienia administratora.
