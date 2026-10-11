package main

import (
	"fmt"
	"math"
	"net"
	"sort"
	"strings"

	"dnsspeed/internal/dnscore"
)

const (
	laneH  = 58.0 // wysokosc jednego toru (px logiczne)
	margin = 24.0
)

// laneView to jeden tor w basenie: wynik serwera i jego animowany stan.
type laneView struct {
	R     dnscore.Result
	Y, TY float64 // pozycja pionowa w basenie: biezaca i docelowa
	C, U  float64 // wyswietlane czasy [ms] (animowane): znane strony, nowe nazwy
}

// Verdict to podsumowanie po zakonczeniu testu.
type Verdict struct {
	Title, Line1, Line2 string
	CanApply            bool
	ApplyLabel          string
	ApplyIPs            []string
}

// Model to caly stan interfejsu. Nie zalezy od systemu, wiec da sie go testowac wszedzie.
type Model struct {
	Lanes     []*laneView
	byIP      map[string]*laneView
	Running   bool
	Finished  bool
	Status    string
	Progress  float64 // cel
	ProgShown float64 // wyswietlany (animowany)
	Verdict   *Verdict
	Msg       string
	MsgKind   int // 0 zwykly, 1 powodzenie, 2 blad
	Tick      float64
	Animate   bool
}

func NewModel() *Model {
	return &Model{byIP: map[string]*laneView{}, Animate: true, Status: "Wczytuję listę serwerów…"}
}

func waitingResult(name, ip string, system bool) dnscore.Result {
	return dnscore.Result{Name: name, IP: ip, System: system, State: "waiting", Cached: -1, Uncached: -1, Score: -1}
}

func (m *Model) addLane(r dnscore.Result) *laneView {
	lv := &laneView{R: r}
	lv.TY = float64(len(m.Lanes)) * laneH
	lv.Y = lv.TY
	m.Lanes = append(m.Lanes, lv)
	m.byIP[r.IP] = lv
	return lv
}

// SetServers ustawia puste tory dla listy serwerow (przed startem testu).
func (m *Model) SetServers(list []dnscore.Server) {
	m.Lanes = nil
	m.byIP = map[string]*laneView{}
	for _, s := range list {
		m.addLane(waitingResult(s.Name, s.IP, s.System))
	}
	m.Verdict = nil
	m.Finished = false
}

// AddCustom dodaje wlasny serwer. Zwraca komunikat bledu albo "".
func (m *Model) AddCustom(raw string) (string, string) {
	s := strings.TrimSpace(raw)
	if !dnscore.IsIPv4(s) {
		return "", "Wpisz adres IPv4 w formacie 192.168.0.1."
	}
	ip := net.ParseIP(s).To4().String()
	if _, dup := m.byIP[ip]; dup {
		return "", "Ten adres jest już na liście."
	}
	m.addLane(waitingResult("Własny ("+ip+")", ip, false))
	return ip, ""
}

func (m *Model) sameServers(res []*dnscore.Result) bool {
	if len(res) != len(m.Lanes) {
		return false
	}
	for _, r := range res {
		if _, ok := m.byIP[r.IP]; !ok {
			return false
		}
	}
	return true
}

func (m *Model) rebuild(res []*dnscore.Result) {
	m.Lanes = nil
	m.byIP = map[string]*laneView{}
	for _, r := range res {
		m.addLane(*r)
	}
}

// Update wczytuje aktualny stan testu z silnika.
func (m *Model) Update(st dnscore.Status) {
	if !m.sameServers(st.Results) {
		m.rebuild(st.Results)
	}
	for _, r := range st.Results {
		if lv := m.byIP[r.IP]; lv != nil {
			lv.R = *r
		}
	}
	m.Running = st.Running
	if st.Running {
		n := st.Done + 1
		if n > st.Total {
			n = st.Total
		}
		m.Status = fmt.Sprintf("Testuję %s (%d z %d)", shortName(st.Current), n, st.Total)
		if st.Total > 0 {
			m.Progress = float64(st.Done) / float64(st.Total)
		}
		return
	}
	if st.Total > 0 && !m.Finished {
		m.finish(st)
	}
}

func (m *Model) finish(st dnscore.Status) {
	m.Finished = true
	m.Progress = 1
	sort.SliceStable(m.Lanes, func(i, j int) bool { return m.Lanes[i].R.Rank < m.Lanes[j].R.Rank })
	for i, lv := range m.Lanes {
		lv.TY = float64(i) * laneH
	}
	m.Status = fmt.Sprintf("Gotowe. Sprawdzono %d %s.", st.Total, plServers(st.Total))
	m.Verdict = computeVerdict(st.Results)
}

// Domain zwraca zakres osi czasu pasków (ms): od 60 do 400.
func (m *Model) Domain() float64 {
	top := 0.0
	for _, lv := range m.Lanes {
		top = math.Max(top, math.Max(lv.R.Cached, lv.R.Uncached))
	}
	d := math.Ceil(top*1.15/10) * 10
	return math.Min(400, math.Max(60, d))
}

// ContentH to wysokosc wszystkich torow.
func (m *Model) ContentH() float64 { return float64(len(m.Lanes)) * laneH }

func approach(v *float64, target, k float64) bool {
	d := target - *v
	if math.Abs(d) < .05 {
		*v = target
		return false
	}
	*v += d * k
	return true
}

// Step przesuwa animacje o dt sekund. Zwraca true, dopoki cos sie rusza
// (albo trwa test, bo wtedy tory "plyna").
func (m *Model) Step(dt float64) bool {
	m.Tick += dt
	k := 1 - math.Exp(-dt*9)
	if !m.Animate {
		k = 1
	}
	moving := false
	for _, lv := range m.Lanes {
		moving = approach(&lv.Y, lv.TY, k) || moving
		moving = approach(&lv.C, math.Max(lv.R.Cached, 0), k) || moving
		moving = approach(&lv.U, math.Max(lv.R.Uncached, 0), k) || moving
	}
	p := m.ProgShown
	moving = approach(&p, m.Progress, k) || moving
	m.ProgShown = p
	return moving || m.Running
}

func (m *Model) SetMsg(kind int, text string) { m.MsgKind, m.Msg = kind, text }

// --- teksty ---

func fmtMs(v float64) string {
	if v < 0 {
		return "–"
	}
	if v < 10 {
		return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1) + " ms"
	}
	return fmt.Sprintf("%d ms", int(math.Round(v)))
}

func plServers(n int) string {
	switch {
	case n == 1:
		return "serwer"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return "serwery"
	}
	return "serwerów"
}

const sysPrefix = "Obecny DNS ("

// shortName pokazuje adres zamiast "Obecny DNS (1.2.3.4)".
func shortName(name string) string {
	if strings.HasPrefix(name, sysPrefix) && strings.HasSuffix(name, ")") {
		return strings.TrimSuffix(strings.TrimPrefix(name, sysPrefix), ")")
	}
	return name
}

func displayName(r *dnscore.Result) string { return shortName(r.Name) }

func computeVerdict(results []*dnscore.Result) *Verdict {
	var ok []*dnscore.Result
	for _, r := range results {
		if r.State == "done" && !r.Hijack {
			ok = append(ok, r)
		}
	}
	sort.SliceStable(ok, func(i, j int) bool { return ok[i].Rank < ok[j].Rank })
	if len(ok) == 0 {
		return &Verdict{
			Title: "Żaden serwer nie odpowiedział",
			Line1: "Sprawdź połączenie z internetem i ustawienia zapory sieciowej, a potem uruchom test ponownie.",
		}
	}
	best := ok[0]
	var cur *dnscore.Result
	for _, r := range results {
		if r.System && r.State == "done" && (cur == nil || r.Score < cur.Score) {
			cur = r
		}
	}
	v := &Verdict{
		Title: "Wygrywa " + displayName(best),
		Line1: fmt.Sprintf("%s odpowiada średnio w %s.", best.IP, fmtMs(best.Score)),
	}
	switch {
	case best.System:
		v.Line2 = "To już jest Twój obecny DNS, więc zmiana nie jest potrzebna."
	case cur != nil && cur.Hijack:
		v.Line2 = fmt.Sprintf("Twój obecny DNS (%s) podmienia adresy nieistniejących stron. Warto go zmienić.", displayName(cur))
	case cur != nil:
		if d := cur.Score - best.Score; d >= 5 {
			v.Line2 = fmt.Sprintf("Twój obecny DNS (%s) jest wolniejszy o około %d ms na zapytanie.", displayName(cur), int(math.Round(d)))
		} else {
			v.Line2 = fmt.Sprintf("Twój obecny DNS (%s) wypada podobnie, więc zmiana raczej niczego nie przyspieszy.", displayName(cur))
		}
	}
	seen := map[string]bool{}
	for _, r := range ok {
		if len(v.ApplyIPs) == 2 {
			break
		}
		if !seen[r.IP] {
			seen[r.IP] = true
			v.ApplyIPs = append(v.ApplyIPs, r.IP)
		}
	}
	v.ApplyLabel = "Ustaw " + strings.Join(v.ApplyIPs, " i ")
	v.CanApply = !best.System
	return v
}

// buildCSV tworzy tekst pliku CSV (UTF-8 z BOM, srednik jako separator, przecinek dziesietny),
// dzieki czemu polski Excel otwiera go poprawnie.
func buildCSV(lanes []*laneView) string {
	num := func(v float64) string {
		if v < 0 {
			return ""
		}
		return strings.Replace(fmt.Sprintf("%.1f", v), ".", ",", 1)
	}
	q := func(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
	var b strings.Builder
	b.WriteString("\uFEFF")
	b.WriteString("Miejsce;Serwer;IP;Znane strony [ms];Nowe nazwy [ms];Wynik [ms];Błędy;Podmienia adresy\r\n")
	for _, lv := range lanes {
		r := lv.R
		hij := "nie"
		if r.Hijack {
			hij = "tak"
		}
		rank := ""
		if r.Rank > 0 {
			rank = itoa(r.Rank)
		}
		b.WriteString(strings.Join([]string{rank, q(r.Name), q(r.IP), num(r.Cached), num(r.Uncached), num(r.Score), itoa(r.Fails), hij}, ";"))
		b.WriteString("\r\n")
	}
	return b.String()
}
