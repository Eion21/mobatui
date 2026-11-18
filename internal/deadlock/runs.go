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

// runsMemo evita repetir la decodificación de la muestra de partidas al
// volver a un héroe ya consultado. NO es una caché de datos: la clave
// incluye la fecha de los datos servidos por la caché de disco, así que
// un refresco real invalida la entrada. Se acota a un número pequeño de
// héroes para no crecer sin límite durante una sesión larga.
type runsMemoEntry struct {
	fetchedAt time.Time
	runs      []Run
}

var (
	runsMemoMu sync.Mutex
	runsMemo   = map[string]runsMemoEntry{}
)

// runsMemoMax es el número de héroes cuyas runs se retienen. Con 250
// partidas por héroe son unos pocos MB como mucho.
const runsMemoMax = 8

// RunOrigin describe de dónde salió la muestra de partidas de un héroe
// y cuándo se obtuvieron esos datos. Es por héroe: la antigüedad
// pertenece a la MUESTRA, no a cada objeto de la ruta.
type RunOrigin struct {
	// State es el estado de caché: hit, miss, stale o fixed.
	State string
	// FetchedAt es la fecha de los datos. Zero = desconocida; en ese
	// caso no se inventa una antigüedad.
	FetchedAt time.Time
}

// Age devuelve cuánto tiempo tienen los datos, o false si se desconoce.
func (o RunOrigin) Age(now time.Time) (time.Duration, bool) {
	if o.FetchedAt.IsZero() {
		return 0, false
	}
	return now.Sub(o.FetchedAt), true
}

// Stale indica si los datos se sirvieron de una entrada caducada, o si
// proceden de un respaldo local: en ambos casos no son la descarga
// fresca de esta consulta.
func (o RunOrigin) Stale() bool {
	return o.State == "stale" || o.State == "fixed"
}

// memoKey identifica una consulta de runs de forma inequívoca.
func memoKey(hero, minBadge, limit int, fetchedAt time.Time) string {
	return fmt.Sprintf("%d|%d|%d|%d", hero, minBadge, limit, fetchedAt.UnixNano())
}

// memoGet devuelve las runs memorizadas para esa clave exacta.
func memoGet(key string) ([]Run, bool) {
	runsMemoMu.Lock()
	defer runsMemoMu.Unlock()
	e, ok := runsMemo[key]
	if !ok {
		return nil, false
	}
	return e.runs, true
}

// memoPut guarda las runs bajo la clave. Si se supera el tope, se
// descarta la entrada más antigua (mapa pequeño: barrido lineal).
func memoPut(key string, fetchedAt time.Time, runs []Run) {
	runsMemoMu.Lock()
	defer runsMemoMu.Unlock()
	if len(runsMemo) >= runsMemoMax {
		var oldestKey string
		var oldest time.Time
		for k, v := range runsMemo {
			if oldestKey == "" || v.fetchedAt.Before(oldest) {
				oldestKey, oldest = k, v.fetchedAt
			}
		}
		delete(runsMemo, oldestKey)
	}
	runsMemo[key] = runsMemoEntry{fetchedAt: fetchedAt, runs: runs}
}

// perfHook permite instrumentar los tiempos internos en pruebas.
var perfHook func(stage string, d time.Duration)

// SetPerfHook instala (o quita, con nil) el hook de instrumentación.
func SetPerfHook(fn func(stage string, d time.Duration)) { perfHook = fn }

// ResetRunsMemo vacía la memoización (tests y cambio de fuente).
func ResetRunsMemo() {
	runsMemoMu.Lock()
	defer runsMemoMu.Unlock()
	runsMemo = map[string]runsMemoEntry{}
}

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

// runsCatalog resuelve el catálogo de tienda y sus componentes una sola
// vez por cliente. El catálogo es estático dentro de una sesión: la
// política de actualización la sigue marcando la caché de disco de
// assets (ttlAssets), no esta reutilización.
func (a *APIClient) runsCatalog() (map[int]apiItem, map[int][]int, error) {
	a.runsCatMu.Lock()
	defer a.runsCatMu.Unlock()
	if a.runsCat != nil {
		return a.runsCat, a.runsComps, nil
	}
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, nil, err
	}
	a.runsCat, a.runsComps = shop, compsOf(shop)
	return a.runsCat, a.runsComps, nil
}

// FetchRuns descarga (con caché diaria) las secuencias de compra del
// héroe. El array JSON se decodifica en streaming: la respuesta cruda
// (decenas de KB por partida) nunca se retiene ni se cachea; lo que
// queda en caché es el resultado agregado (las Run).
func (a *APIClient) FetchRuns(ctx context.Context, hero, minBadge, limit int) ([]Run, error) {
	// Primero el catálogo: sin él no se puede ni construir la URL de forma
	// útil (y su error no debe provocar reintentos de red de la muestra).
	shop, comps, err := a.runsCatalog()
	if err != nil {
		return nil, err
	}
	path := a.runsPath(hero, minBadge, limit)

	// Deduplicación: la UI lanza Ruta y build híbrida a la vez (tea.Batch)
	// y ambas piden LA MISMA muestra. La clave es la URL completa, así que
	// héroes distintos no se bloquean entre sí.
	runs, origin, err := joinRuns(ctx, a.apiBase+path, func() ([]Run, RunOrigin, error) {
		return a.fetchRunsOnce(ctx, path, hero, shop, comps)
	})
	if err != nil {
		return nil, err
	}
	// La procedencia se publica por héroe con la fecha DEL RESULTADO. Si
	// otra petición del mismo héroe termina después, su procedencia es la
	// de SU respuesta: no se pisa la de esta.
	a.setRunOrigin(hero, origin)
	return runs, nil
}

// runsPath construye la ruta de la muestra. Es la identidad de la
// solicitud: héroe, filtros y límite.
func (a *APIClient) runsPath(hero, minBadge, limit int) string {
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
	return "/v1/matches/metadata?" + q.Encode()
}

// fetchRunsOnce resuelve la muestra de un héroe: caché, descarga con
// reintento acotado, memo y decodificación.
func (a *APIClient) fetchRunsOnce(ctx context.Context, path string, hero int, shop map[int]apiItem, comps map[int][]int) ([]Run, RunOrigin, error) {
	shopSet := make(map[int]bool, len(shop))
	for id := range shop {
		shopSet[id] = true
	}

	// Se consulta la caché sin fetch para saber si ya hay una entrada
	// utilizable (aunque esté caducada). Con entrada, un fallo de fuente
	// se resuelve con stale-if-error y NO se reintenta: pedir dos veces
	// no aporta nada.
	staleExists := a.c.HasEntry(a.apiBase + path)

	fetch := func() ([]byte, error) {
		return a.downloadRuns(ctx, path, hero, shopSet, comps, staleExists)
	}
	tGet := time.Now()
	res, err := a.c.Get(a.apiBase+path, metaTTL, fetch)
	if perfHook != nil {
		perfHook("cache.Get", time.Since(tGet))
	}
	if err != nil {
		return nil, RunOrigin{}, err
	}
	// La fecha de los datos servidos forma parte de la clave: si la
	// caché de disco se refresca, la entrada memorizada deja de
	// coincidir y se vuelve a decodificar.
	//
	// La fecha sale del RESULTADO de esta petición, no de Cache.Last():
	// Last() es estado global y con consultas concurrentes —cambio de
	// héroe rápido, o UI y precarga a la vez— podía devolver la fecha de
	// otra respuesta, reutilizando una muestra que no correspondía.
	// La clave incluye la URL (héroe, filtros y límite) y la fecha del
	// resultado, así que dos peticiones con parámetros distintos o con
	// muestras distintas nunca comparten entrada.
	key := res.FetchedAt.Format("20060102150405.999999999") + "|" + path
	origin := RunOrigin{State: string(res.State), FetchedAt: res.FetchedAt}
	if runs, ok := memoGet(key); ok {
		return runs, origin, nil
	}
	var runs []Run
	tDec := time.Now()
	if err := json.Unmarshal(res.Body, &runs); err != nil {
		return nil, origin, fmt.Errorf("deadlock-api: runs inválidas: %w", err)
	}
	if perfHook != nil {
		perfHook("json.Unmarshal", time.Since(tDec))
	}
	memoPut(key, res.FetchedAt, runs)
	return runs, origin, nil
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

// setRunOrigin guarda la procedencia de la muestra de un héroe.
func (a *APIClient) setRunOrigin(hero int, o RunOrigin) {
	a.runsOriginMu.Lock()
	if a.runsOrigin == nil {
		a.runsOrigin = map[int]RunOrigin{}
	}
	a.runsOrigin[hero] = o
	a.runsOriginMu.Unlock()
}

// RunOriginFor devuelve la procedencia de la muestra de un héroe.
// ok=false si aún no se ha consultado.
func (a *APIClient) RunOriginFor(hero int) (RunOrigin, bool) {
	a.runsOriginMu.Lock()
	defer a.runsOriginMu.Unlock()
	o, ok := a.runsOrigin[hero]
	return o, ok
}
