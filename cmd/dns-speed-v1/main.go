package main

import (
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"dnsspeed/internal/dnscore"
)

//go:embed web/index.html
var indexHTML []byte

var (
	token    string
	addr     string
	bench    = &dnscore.Bench{}
	lastPing atomic.Int64 // ostatnia aktywnosc interfejsu (ns)
	byeAt    atomic.Int64 // moment zamkniecia karty (ns)
	started  = time.Now().UnixNano()
)

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		dnscore.Fatal("Nie udało się wygenerować klucza sesji: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

// guard dopuszcza tylko zapytania z naszej strony (poprawny host i klucz sesji).
// Dzieki temu inna strona otwarta w przegladarce nie moze sterowac programem.
func guard(touch bool, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Host != addr {
			http.Error(w, "zły host", http.StatusForbidden)
			return
		}
		t := r.Header.Get("X-Token")
		if t == "" {
			t = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(t), []byte(token)) != 1 {
			http.Error(w, "brak dostępu", http.StatusForbidden)
			return
		}
		if touch {
			lastPing.Store(time.Now().UnixNano())
		}
		h(w, r)
	}
}

func main() {
	noBrowser := flag.Bool("nobrowser", false, "nie otwieraj przeglądarki")
	flag.Parse()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		dnscore.Fatal("Nie udało się uruchomić lokalnego serwera: " + err.Error())
	}
	addr = ln.Addr().String()
	token = randHex(16)

	mux := http.NewServeMux()

	mux.HandleFunc("/", guard(true, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(indexHTML)
	}))

	mux.HandleFunc("/api/ping", guard(true, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/servers", guard(true, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, dnscore.BuildServers(nil))
	}))

	mux.HandleFunc("/api/start", guard(true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "tylko POST", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Extra []string `json:"extra"`
		}
		_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body)
		if len(body.Extra) > 20 {
			body.Extra = body.Extra[:20]
		}
		if !bench.Start(dnscore.BuildServers(body.Extra)) {
			http.Error(w, "test już trwa", http.StatusConflict)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("/api/status", guard(true, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, bench.Snapshot())
	}))

	mux.HandleFunc("/api/apply", guard(true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "tylko POST", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			IPs []string `json:"ips"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			http.Error(w, "zły format danych", http.StatusBadRequest)
			return
		}
		if len(body.IPs) < 1 || len(body.IPs) > 2 {
			http.Error(w, "podaj jeden lub dwa adresy", http.StatusBadRequest)
			return
		}
		var clean []string
		for _, ip := range body.IPs {
			if !dnscore.IsIPv4(ip) {
				http.Error(w, "niepoprawny adres IPv4: "+ip, http.StatusBadRequest)
				return
			}
			clean = append(clean, net.ParseIP(strings.TrimSpace(ip)).To4().String())
		}
		now, err := dnscore.ApplyDNS(clean)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "current": now})
	}))

	mux.HandleFunc("/api/reset", guard(true, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "tylko POST", http.StatusMethodNotAllowed)
			return
		}
		now, err := dnscore.ResetDNS()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "current": now})
	}))

	// Karta przegladarki zostala zamknieta (wysylane przez sendBeacon).
	mux.HandleFunc("/api/bye", guard(false, func(w http.ResponseWriter, r *http.Request) {
		byeAt.Store(time.Now().UnixNano())
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/api/quit", guard(false, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() {
			time.Sleep(300 * time.Millisecond)
			os.Exit(0)
		}()
	}))

	// Zamykamy program sam, gdy karta zniknela albo przegladarka nie odpowiada.
	go func() {
		for range time.Tick(time.Second) {
			now := time.Now().UnixNano()
			lp, bye := lastPing.Load(), byeAt.Load()
			switch {
			case bye != 0 && now-bye > int64(6*time.Second) && lp < bye:
				os.Exit(0)
			case lp != 0 && now-lp > int64(3*time.Minute):
				os.Exit(0)
			case lp == 0 && now-started > int64(90*time.Second):
				os.Exit(0)
			}
		}
	}()

	url := fmt.Sprintf("http://%s/?t=%s", addr, token)
	fmt.Println("DNS Speed działa pod adresem:", url)
	if !*noBrowser {
		dnscore.OpenBrowser(url)
	}

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := srv.Serve(ln); err != nil {
		dnscore.Fatal("Serwer przestał działać: " + err.Error())
	}
}
