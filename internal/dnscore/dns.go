package dnscore

import (
	"crypto/rand"
	"encoding/binary"
	"math/big"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Server opisuje jeden serwer DNS do przetestowania.
type Server struct {
	Name   string `json:"name"`
	IP     string `json:"ip"`
	System bool   `json:"system"`
}

// Result to wynik testu jednego serwera (wysylany do interfejsu).
type Result struct {
	Name     string  `json:"name"`
	IP       string  `json:"ip"`
	System   bool    `json:"system"`
	State    string  `json:"state"` // waiting, testing, done, dead
	Cached   float64 `json:"cached"`
	Uncached float64 `json:"uncached"`
	Score    float64 `json:"score"`
	Fails    int     `json:"fails"`
	Total    int     `json:"total"`
	Hijack   bool    `json:"hijack"`
	Rank     int     `json:"rank"`
}

// Status to to, co interfejs dostaje podczas odpytywania.
type Status struct {
	Running bool      `json:"running"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	Current string    `json:"current"`
	Results []*Result `json:"results"`
}

var builtinServers = []Server{
	{"Cloudflare", "1.1.1.1", false},
	{"Cloudflare (zapasowy)", "1.0.0.1", false},
	{"Google", "8.8.8.8", false},
	{"Google (zapasowy)", "8.8.4.4", false},
	{"Quad9", "9.9.9.9", false},
	{"Quad9 (zapasowy)", "149.112.112.112", false},
	{"OpenDNS", "208.67.222.222", false},
	{"OpenDNS (zapasowy)", "208.67.220.220", false},
	{"AdGuard", "94.140.14.14", false},
	{"AdGuard (zapasowy)", "94.140.15.15", false},
	{"CleanBrowsing", "185.228.168.9", false},
	{"Control D", "76.76.2.0", false},
	{"Mullvad", "194.242.2.2", false},
	{"DNS.WATCH", "84.200.69.80", false},
	{"Comodo", "8.26.56.26", false},
	{"Orange Polska", "194.204.152.34", false},
}

// Domeny do pomiaru z pamiecia podreczna (popularne, czesto odpytywane).
var popularDomains = []string{
	"google.com", "youtube.com", "facebook.com", "wp.pl", "onet.pl",
	"allegro.pl", "interia.pl", "olx.pl", "wikipedia.org", "amazon.com",
}

// Domeny, pod ktorymi tworzymy losowe, nieistniejace nazwy (pomiar bez cache).
var randomParents = []string{
	"google.com", "wikipedia.org", "microsoft.com", "amazon.com",
	"github.com", "mozilla.org", "apple.com", "cloudflare.com",
}

// dnsPort jest zmienna tylko po to, by testy mogly uzyc falszywego serwera.
var dnsPort = "53"

const (
	queryTimeout   = 1500 * time.Millisecond
	cachedRounds   = 2
	uncachedProbes = 8
	failPenaltyMs  = 40.0
)

func randUint16() uint16 {
	n, err := rand.Int(rand.Reader, big.NewInt(65536))
	if err != nil {
		return uint16(time.Now().UnixNano())
	}
	return uint16(n.Int64())
}

func randLabel(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		k, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			b[i] = letters[int(time.Now().UnixNano())%len(letters)]
			continue
		}
		b[i] = letters[k.Int64()]
	}
	return string(b)
}

func buildQuery(name string, id uint16) []byte {
	b := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[2:], 0x0100) // RD: chcemy rekurencji
	binary.BigEndian.PutUint16(b[4:], 1)      // 1 pytanie
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	b = append(b, 0, 0, 1, 0, 1) // typ A, klasa IN
	return b
}

type reply struct {
	rcode   int
	answers int
	rtt     time.Duration
}

// ask wysyla jedno zapytanie A przez UDP i mierzy czas do odpowiedzi.
func ask(server, name string, timeout time.Duration) (reply, error) {
	conn, err := net.Dial("udp", net.JoinHostPort(server, dnsPort))
	if err != nil {
		return reply{}, err
	}
	defer conn.Close()

	id := randUint16()
	q := buildQuery(name, id)
	_ = conn.SetDeadline(time.Now().Add(timeout))

	start := time.Now()
	if _, err := conn.Write(q); err != nil {
		return reply{}, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return reply{}, err
		}
		rtt := time.Since(start)
		if n < 12 || binary.BigEndian.Uint16(buf[0:]) != id {
			continue
		}
		flags := binary.BigEndian.Uint16(buf[2:])
		if flags&0x8000 == 0 { // to nie odpowiedz
			continue
		}
		return reply{
			rcode:   int(flags & 0xF),
			answers: int(binary.BigEndian.Uint16(buf[6:])),
			rtt:     rtt,
		}, nil
	}
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return -1
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// Bench trzyma stan calego testu.
type Bench struct {
	mu      sync.Mutex
	running bool
	done    int
	total   int
	current string
	results []*Result
}

func (b *Bench) Snapshot() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]*Result, len(b.results))
	for i, r := range b.results {
		c := *r
		out[i] = &c
	}
	return Status{Running: b.running, Done: b.done, Total: b.total, Current: b.current, Results: out}
}

func (b *Bench) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

// Start uruchamia test w tle. Zwraca false, jesli test juz trwa.
func (b *Bench) Start(servers []Server) bool {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return false
	}
	b.running = true
	b.done = 0
	b.total = len(servers)
	b.current = ""
	b.results = make([]*Result, len(servers))
	for i, s := range servers {
		b.results[i] = &Result{
			Name: s.Name, IP: s.IP, System: s.System,
			State: "waiting", Cached: -1, Uncached: -1, Score: -1,
		}
	}
	b.mu.Unlock()

	go b.run(servers)
	return true
}

func (b *Bench) update(i int, f func(r *Result)) {
	b.mu.Lock()
	f(b.results[i])
	b.mu.Unlock()
}

func (b *Bench) run(servers []Server) {
	for i, s := range servers {
		b.mu.Lock()
		b.current = s.Name
		b.mu.Unlock()
		b.update(i, func(r *Result) { r.State = "testing" })

		res := measure(s.IP)

		b.update(i, func(r *Result) {
			r.State = res.State
			r.Cached = res.Cached
			r.Uncached = res.Uncached
			r.Score = res.Score
			r.Fails = res.Fails
			r.Total = res.Total
			r.Hijack = res.Hijack
		})
		b.mu.Lock()
		b.done++
		b.mu.Unlock()
	}

	b.mu.Lock()
	rank(b.results)
	b.running = false
	b.current = ""
	b.mu.Unlock()
}

// rank nadaje miejsca: najpierw serwery, ktore odpowiedzialy, wedlug wyniku.
func rank(results []*Result) {
	idx := make([]*Result, len(results))
	copy(idx, results)
	sort.SliceStable(idx, func(a, c int) bool {
		ra, rc := idx[a], idx[c]
		da, dc := ra.State == "dead", rc.State == "dead"
		if da != dc {
			return !da
		}
		// Serwery podmieniajace adresy nie moga wygrac z uczciwymi.
		if ra.Hijack != rc.Hijack {
			return !ra.Hijack
		}
		return ra.Score < rc.Score
	})
	for n, r := range idx {
		r.Rank = n + 1
	}
}

// measure sprawdza jeden serwer: pomiar z cache, bez cache i wykrywanie podmiany adresow.
func measure(ip string) Result {
	out := Result{State: "done", Cached: -1, Uncached: -1, Score: -1}

	// Rozgrzewka: pierwsze zapytania wypelniaja cache serwera.
	// Jesli trzy pierwsze nie dostaja odpowiedzi, serwer uznajemy za martwy.
	warmFails := 0
	for k := 0; k < 3; k++ {
		if _, err := ask(ip, popularDomains[k], queryTimeout); err != nil {
			warmFails++
		}
	}
	if warmFails == 3 {
		out.State = "dead"
		out.Fails = 3
		out.Total = 3
		return out
	}
	for k := 3; k < len(popularDomains); k++ {
		_, _ = ask(ip, popularDomains[k], queryTimeout)
	}

	var cached []float64
	fails, total := 0, 0
	for r := 0; r < cachedRounds; r++ {
		for _, d := range popularDomains {
			total++
			rep, err := ask(ip, d, queryTimeout)
			if err != nil || rep.rcode != 0 {
				fails++
				continue
			}
			cached = append(cached, ms(rep.rtt))
		}
	}

	var uncached []float64
	hijacked := 0
	for k := 0; k < uncachedProbes; k++ {
		total++
		name := randLabel(14) + "." + randomParents[k%len(randomParents)]
		rep, err := ask(ip, name, queryTimeout)
		if err != nil {
			fails++
			continue
		}
		if rep.rcode == 0 && rep.answers > 0 {
			hijacked++ // nieistniejaca nazwa dostala adres: serwer podmienia odpowiedzi
		} else if rep.rcode != 3 && rep.rcode != 0 {
			fails++
			continue
		}
		uncached = append(uncached, ms(rep.rtt))
	}

	out.Fails = fails
	out.Total = total
	out.Hijack = hijacked >= 6
	out.Cached = median(cached)
	out.Uncached = median(uncached)

	if out.Cached < 0 || out.Uncached < 0 {
		out.State = "dead"
		return out
	}
	out.Score = (out.Cached+out.Uncached)/2 + float64(fails)*failPenaltyMs
	return out
}

// IsIPv4 sprawdza, czy tekst jest poprawnym adresem IPv4.
func IsIPv4(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ip.To4() != nil
}

// BuildServers sklada liste: wbudowane serwery, obecny DNS systemu i wlasne adresy.
func BuildServers(extra []string) []Server {
	list := make([]Server, len(builtinServers))
	copy(list, builtinServers)

	have := func(ip string) int {
		for i, s := range list {
			if s.IP == ip {
				return i
			}
		}
		return -1
	}

	var front []Server
	for _, ip := range SystemDNS() {
		if i := have(ip); i >= 0 {
			list[i].System = true
		} else {
			front = append(front, Server{Name: "Obecny DNS (" + ip + ")", IP: ip, System: true})
		}
	}
	list = append(front, list...)

	for _, raw := range extra {
		ip := strings.TrimSpace(raw)
		if !IsIPv4(ip) {
			continue
		}
		ip = net.ParseIP(ip).To4().String()
		if have(ip) >= 0 {
			continue
		}
		list = append(list, Server{Name: "Własny (" + ip + ")", IP: ip})
	}
	return list
}
