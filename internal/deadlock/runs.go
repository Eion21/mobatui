// Ruta de compra: la secuencia real de compras viene de partidas
// individuales (/v1/matches/metadata con include_player_items). Los
// WR y pick oficiales siguen saliendo de item-stats (población
// completa); la muestra de partidas aporta orden, posición,
// patrimonio y tasas de cadena.
package deadlock

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

// metaTTL es el refresco de la muestra de partidas: una vez al día.
const metaTTL = 24 * time.Hour

// rawItem es una compra (o venta) del replay del jugador. En items[]
// también vienen los puntos de habilidad: se filtran con shop.
type rawItem struct {
	ItemID        int `json:"item_id"`
	GameTimeS     int `json:"game_time_s"`
	SoldTimeS     int `json:"sold_time_s"`
	NetWorthAtBuy int `json:"net_worth_at_buy"`
}

// rawPlayer/rawMatch: solo lo que hace falta; el array JSON se
// recorre en streaming y los 12 jugadores completos nunca se retienen.
type rawPlayer struct {
	HeroID  int       `json:"hero_id"`
	Items   []rawItem `json:"items"`
	Outcome string    `json:"player_match_outcome"` // Win | Loss
	Team    string    `json:"team"`                 // Team0 | Team1
}

type rawMatch struct {
	WinningTeam string      `json:"winning_team"` // Team0 | Team1
	Players     []rawPlayer `json:"players"`
}

// Ev es un evento de compra de la ruta. From = id de la base que
// reemplaza cuando la compra es una mejora en el lugar (0 = directa).
type Ev struct{ Item, T, Net, From int }

// Run es la secuencia de compras de un jugador en una partida.
type Run struct {
	Win bool
	Evs []Ev // ordenados por T
}

// buildEvents clasifica las compras del jugador: la base mejorada en
// el lugar se conserva (fue una decisión real, con su coste en la
// curva de oro) y la venta por cambio se descarta como regret.
func buildEvents(items []rawItem, shop map[int]bool, comps map[int][]int) (evs []Ev, regrets []int) {
	var its []rawItem
	for _, it := range items {
		// Fuera habilidades (no están en shop) y marcas de tiempo 0.
		if shop[it.ItemID] && it.GameTimeS > 0 {
			its = append(its, it)
		}
	}
	abs := func(x int) int {
		if x < 0 {
			return -x
		}
		return x
	}
	upgraded := make(map[int]bool, len(its)) // índice de base mejorada
	from := make(map[int]int, len(its))      // índice de compra → base
	for i, up := range its {
		for j, b := range its {
			if i != j && !upgraded[j] && b.SoldTimeS > 0 &&
				abs(b.SoldTimeS-up.GameTimeS) <= 1 &&
				slices.Contains(comps[up.ItemID], b.ItemID) {
				upgraded[j], from[i] = true, b.ItemID
				break
			}
		}
	}
	for i, it := range its {
		if it.SoldTimeS > 0 && !upgraded[i] {
			// Venta por cambio: no es parte de la ruta.
			regrets = append(regrets, it.ItemID)
			continue
		}
		evs = append(evs, Ev{Item: it.ItemID, T: it.GameTimeS, Net: it.NetWorthAtBuy, From: from[i]})
	}
	sort.SliceStable(evs, func(a, b int) bool { return evs[a].T < evs[b].T })
	return evs, regrets
}

// consensusOrder ordena los ítems frecuentes por precedencia: para
// cada par comparable (≥30 co-ocurrencias), la fracción de veces que
// uno se compra antes que el otro. El puntaje es el promedio sobre los
// pares, para que un ítem con pocas comparaciones no quede penalizado;
// empates por tiempo mediano de compra.
func consensusOrder(runs []Run, top []int, medT map[int]int) []int {
	idx := make(map[int]int, len(top))
	for i, id := range top {
		idx[id] = i
	}
	n := len(top)
	before := make([][]int, n)
	for i := range before {
		before[i] = make([]int, n)
	}
	for _, r := range runs {
		first := map[int]int{} // primera compra de cada ítem en la run
		for i, e := range r.Evs {
			if _, ok := idx[e.Item]; ok {
				if _, seen := first[e.Item]; !seen {
					first[e.Item] = i
				}
			}
		}
		for a, pa := range first {
			for b, pb := range first {
				if a != b && pa < pb {
					before[idx[a]][idx[b]]++
				}
			}
		}
	}
	score := make(map[int]float64, n)
	for i, a := range top {
		var sum float64
		var cnt int
		for j := range top {
			if i == j {
				continue
			}
			if tot := before[i][j] + before[j][i]; tot >= 30 {
				sum += float64(before[i][j]) / float64(tot)
				cnt++
			}
		}
		if cnt > 0 {
			score[a] = sum / float64(cnt)
		} else {
			score[a] = 0.5
		}
	}
	out := slices.Clone(top)
	sort.SliceStable(out, func(x, y int) bool {
		if d := score[out[x]] - score[out[y]]; d > 0.02 || d < -0.02 {
			return d > 0
		}
		return medT[out[x]] < medT[out[y]]
	})
	return out
}

// Step es un paso de la ruta de compra: el ítem, cuánta gente lo
// compra, cuándo (mediana y posición) y de qué cadena viene.
type Step struct {
	Item         int
	N            int
	Share        float64 // fracción de partidas que lo compran
	MedT         int     // segundo mediano de compra
	MedNet       int     // patrimonio mediano al comprarlo (acumulado, no almas en mano)
	PosLo        int     // posición de compra, P25
	PosHi        int     // posición de compra, P75
	UpgradeOf    int     // base más habitual (0 = casi siempre directo)
	ChainRate    float64 // de quienes lo compran, fracción que viene de esa base
	UpgradedRate float64 // de la base: fracción de compradores que luego la mejoran
	WR           float64 // de la muestra; el WR oficial sigue saliendo de item-stats
}

// pctDev es el cuantil q de xs (copia interna, no muta el slice).
func pctDev(xs []int, q float64) int {
	s := slices.Clone(xs)
	sort.Ints(s)
	return s[int(q*float64(len(s)-1))]
}

// BuildRoute arma la ruta: los ítems con share suficiente, ordenados
// por precedencia, con estadísticos por paso.
func BuildRoute(runs []Run, minShare float64) []Step {
	type acc struct {
		t, net, pos []int
		wins        int
		upRuns      int // partidas donde algún evento posterior mejora esta base
		from        map[int]int
	}
	by := map[int]*acc{}
	for _, r := range runs {
		seen := map[int]bool{}
		ups := map[int]bool{} // bases mejoradas en esta partida
		for i, e := range r.Evs {
			if e.From != 0 {
				ups[e.From] = true
			}
			if seen[e.Item] {
				continue
			}
			seen[e.Item] = true
			a := by[e.Item]
			if a == nil {
				a = &acc{from: map[int]int{}}
				by[e.Item] = a
			}
			a.t = append(a.t, e.T)
			a.net = append(a.net, e.Net)
			a.pos = append(a.pos, i+1)
			if r.Win {
				a.wins++
			}
			a.from[e.From]++
		}
		for base := range ups {
			if a := by[base]; a != nil {
				a.upRuns++
			}
		}
	}
	if len(runs) == 0 {
		return nil
	}
	var top []int
	medT := map[int]int{}
	for id, a := range by {
		if float64(len(a.t))/float64(len(runs)) >= minShare {
			top = append(top, id)
			medT[id] = pctDev(a.t, 0.5)
		}
	}
	var steps []Step
	for _, id := range consensusOrder(runs, top, medT) {
		a := by[id]
		s := Step{
			Item: id, N: len(a.t),
			Share:  float64(len(a.t)) / float64(len(runs)),
			MedT:   pctDev(a.t, 0.5),
			MedNet: pctDev(a.net, 0.5),
			PosLo:  pctDev(a.pos, 0.25), PosHi: pctDev(a.pos, 0.75),
			UpgradedRate: float64(a.upRuns) / float64(len(a.t)),
			WR:           float64(a.wins) / float64(len(a.t)),
		}
		best := 0
		for base, c := range a.from {
			if base != 0 && c > a.from[best] {
				best = base
			}
		}
		if best != 0 {
			s.UpgradeOf, s.ChainRate = best, float64(a.from[best])/float64(len(a.t))
		}
		steps = append(steps, s)
	}
	return steps
}

// ─── descarga ──────────────────────────────────────────────────────

// metaMu serializa el acceso al limitador de matches/metadata: la API
// permite 5 req/s por IP; un mutex con espera basta para 1 héroe por vez.
var (
	metaMu   sync.Mutex
	metaNext time.Time
)

func waitMetaSlot(ctx context.Context) error {
	metaMu.Lock()
	defer metaMu.Unlock()
	if wait := time.Until(metaNext); wait > 0 {
		metaMu.Unlock()
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
			metaMu.Lock()
			return ctx.Err()
		}
		metaMu.Lock()
	}
	metaNext = time.Now().Add(250 * time.Millisecond) // 4 req/s, margen bajo el tope
	return nil
}

// HeroID resuelve el id numérico del héroe por nombre (0 si no está).
func (a *APIClient) HeroID(name string) int {
	for id, n := range a.HeroNamesByID() {
		if strings.EqualFold(n, name) {
			return id
		}
	}
	return 0
}

// compsOf arma el mapa base → componentes comprables a partir de los
// metadatos de tienda (class_name ↔ component_items).
func compsOf(shop map[int]apiItem) map[int][]int {
	byClass := make(map[string]int, len(shop))
	for id, it := range shop {
		if it.ClassName != "" {
			byClass[it.ClassName] = id
		}
	}
	out := map[int][]int{}
	for id, it := range shop {
		for _, c := range it.ComponentItems {
			if base, ok := byClass[c]; ok {
				out[id] = append(out[id], base)
			}
		}
	}
	return out
}

// FetchRuns descarga (con caché diaria) las secuencias de compra del
// héroe. El array JSON se decodifica en streaming: la respuesta cruda
// (decenas de KB por partida) nunca se retiene ni se cachea; lo que
// queda en caché es el resultado agregado (las Run).
func (a *APIClient) FetchRuns(ctx context.Context, hero, minBadge, limit int) ([]Run, error) {
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	shopSet := make(map[int]bool, len(shop))
	for id := range shop {
		shopSet[id] = true
	}
	comps := compsOf(shop)

	q := url.Values{
		"hero_ids":             {fmt.Sprint(hero)},
		"game_mode":            {"normal"},
		"include_info":         {"true"},
		"include_player_items": {"true"},
		"order_by":             {"start_time"},
		"order_direction":      {"desc"},
		"limit":                {fmt.Sprint(limit)},
	}
	if minBadge > 0 {
		q.Set("min_average_badge", fmt.Sprint(minBadge))
	}
	path := "/v1/matches/metadata?" + q.Encode()

	fetch := func() ([]byte, error) {
		if err := waitMetaSlot(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.apiBase+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return []byte("null"), nil // sin partidas: ruta vacía, no error
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("deadlock-api: %s → HTTP %d", path, resp.StatusCode)
		}
		runs, err := decodeRuns(bufio.NewReaderSize(resp.Body, 1<<20), hero, shopSet, comps)
		if err != nil {
			return nil, err
		}
		return json.Marshal(runs)
	}
	res, err := a.c.Get(a.apiBase+path, metaTTL, fetch)
	if err != nil {
		return nil, err
	}
	var runs []Run
	if err := json.Unmarshal(res.Body, &runs); err != nil {
		return nil, fmt.Errorf("deadlock-api: runs inválidas: %w", err)
	}
	return runs, nil
}

// decodeRuns recorre el array de partidas sin retenerlo entero.
func decodeRuns(r *bufio.Reader, hero int, shop map[int]bool, comps map[int][]int) ([]Run, error) {
	dec := json.NewDecoder(r)
	if b, err := dec.Token(); err != nil {
		return nil, err
	} else if d, ok := b.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("deadlock-api: metadata sin array JSON")
	}
	var runs []Run
	for dec.More() {
		var m rawMatch
		if err := dec.Decode(&m); err != nil {
			return runs, err
		}
		for _, p := range m.Players {
			if p.HeroID != hero {
				continue
			}
			evs, _ := buildEvents(p.Items, shop, comps)
			if len(evs) > 0 {
				runs = append(runs, Run{Win: p.Outcome == "Win" || teamIdx(p.Team, m.WinningTeam), Evs: evs})
			}
		}
	}
	return runs, nil
}

// teamIdx es el respaldo cuando player_match_outcome no viene: el
// equipo del jugador contra el ganador ("Team0"/"Team1").
func teamIdx(team, winning string) bool {
	return strings.TrimPrefix(team, "Team") == strings.TrimPrefix(winning, "Team")
}
