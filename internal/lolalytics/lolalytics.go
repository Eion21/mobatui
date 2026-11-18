// Package lolalytics extrae builds reales de League of Legends desde
// lolalytics.com. u.gg (la fuente original prevista) está tras un
// desafío JS de Cloudflare inaccesible para clientes HTTP, pero
// Lolalytics sirve los mismos datos —ítems, runas y órdenes de
// habilidades con WR y partidas— embebidos en el HTML SSR de sus
// páginas, serializados como estado Qwik (un array plano de objetos
// con referencias numéricas en base36). Este paquete descifra ese
// estado y lo traduce al modelo de dominio.
//
// Todo el conocimiento del formato vive aquí: si Lolalytics cambia su
// serialización, solo se ajusta este archivo.
package lolalytics

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/model"
)

const (
	baseURL    = "https://lolalytics.com"
	userAgent  = "Mozilla/5.0 (X11; Linux x86_64) mobatui/1.0"
	ttlPages   = 6 * time.Hour
	fetchDelay = 150 * time.Millisecond // cortesía con la fuente
	// Umbral de muestra mínima por campeón/carril en el tierlist.
	minLaneGames = 300
	// buildWorkers solapa las latencias de las páginas de build. No
	// sube el ritmo contra la fuente: el limitador de fetchDelay es
	// global, así que el intervalo entre descargas es el mismo.
	buildWorkers = 4
)

// Client descarga y decodifica Lolalytics con caché local.
type Client struct {
	c    *cache.Cache
	http *http.Client
	// base re-dirigible para tests.
	base string
	// delay entre descargas (0 en tests).
	delay time.Duration
	// fellBack se activa cuando la carga terminó en los fixtures.
	fellBack bool
	// gate serializa los fetch reales: el pool de workers solapa
	// latencias sin multiplicar el ritmo contra la fuente (si no,
	// 4 workers bajarían el intervalo efectivo a ~37 ms).
	gate chan struct{}
	// onProgress, si no es nil, recibe (hecho, total, nombre) por
	// cada página resuelta.
	onProgress func(done, total int, name string)
}

// DataState implementa staticdata.FreshnessProvider: origen y
// antigüedad de lo que se está sirviendo.
func (cl *Client) DataState() (string, time.Duration, bool) {
	if cl.fellBack {
		return "fixtures", 0, true
	}
	s, at, ok := cl.c.Last()
	if !ok {
		return "", 0, false
	}
	age := time.Since(at)
	switch s {
	case cache.StateStale:
		return "caducado", age, true
	case cache.StateHit:
		return "caché", age, true
	default:
		return "red", age, true
	}
}

func New(c *cache.Cache) *Client {
	return &Client{
		c:     c,
		http:  &http.Client{Timeout: 20 * time.Second},
		base:  baseURL,
		delay: fetchDelay,
		gate:  make(chan struct{}, 1),
	}
}

// WithHTTPClient y WithBase inyectan dependencias (tests).
func (cl *Client) WithHTTPClient(h *http.Client) *Client { cl.http = h; return cl }
func (cl *Client) WithBase(u string) *Client             { cl.base = u; return cl }
func (cl *Client) WithDelay(d time.Duration) *Client     { cl.delay = d; return cl }

// OnProgress registra el callback de progreso de la carga.
func (cl *Client) OnProgress(fn func(done, total int, name string)) *Client {
	cl.onProgress = fn
	return cl
}

// fetch descarga una URL a través de la caché. El delay de cortesía
// aplica solo cuando hay fetch real —y bajo el gate, para que varios
// workers no se cuelen a la vez—; en acierto de caché no se espera.
func (cl *Client) fetch(url string) ([]byte, error) {
	res, err := cl.c.Get(url, ttlPages, func() ([]byte, error) {
		if cl.delay > 0 {
			cl.gate <- struct{}{}
			defer func() { <-cl.gate }()
			time.Sleep(cl.delay)
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := cl.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("lolalytics: %s → HTTP %d", url, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

// ─── estado Qwik ───────────────────────────────────────────────────

var qwikRe = regexp.MustCompile(`(?s)<script type="qwik/json">(.*?)</script>`)

// qwik es el estado serializado: el array objs con todos los objetos
// y valores, referenciados por índice en base36.
type qwik struct {
	objs []any
	n    int
}

func parseQwik(html []byte) (*qwik, error) {
	m := qwikRe.FindSubmatch(html)
	if m == nil {
		return nil, fmt.Errorf("lolalytics: sin estado qwik/json en la página")
	}
	var payload struct {
		Objs []any `json:"objs"`
	}
	if err := json.Unmarshal(m[1], &payload); err != nil {
		return nil, fmt.Errorf("lolalytics: estado qwik inválido: %w", err)
	}
	if len(payload.Objs) == 0 {
		return nil, fmt.Errorf("lolalytics: estado qwik vacío")
	}
	return &qwik{n: len(payload.Objs), objs: payload.Objs}, nil
}

var refRe = regexp.MustCompile(`^[0-9a-z]{1,4}$`)

// isRef dice si s puede ser una referencia válida al array de objetos
// (índice base36 dentro de rango). Los literales cortos pueden colar:
// por eso solo se resuelve donde el esquema lo espera.
func (q *qwik) isRef(s string) bool {
	if !refRe.MatchString(s) {
		return false
	}
	i, err := strconv.ParseInt(s, 36, 64)
	return err == nil && i >= 0 && int(i) < q.n
}

// deref sigue una cadena de referencias (con tope de profundidad) y
// devuelve el valor final sin resolver más.
func (q *qwik) deref(v any, maxDepth int) any {
	for depth := 0; depth < maxDepth; depth++ {
		s, ok := v.(string)
		if !ok || !q.isRef(s) {
			return v
		}
		i, _ := strconv.ParseInt(s, 36, 64)
		next := q.objs[i]
		if next == nil {
			return v
		}
		v = next
	}
	return v
}

// obj convierte v a map[string]any resolviendo refs.
func (q *qwik) obj(v any) map[string]any {
	m, _ := q.deref(v, 4).(map[string]any)
	return m
}

// list convierte v a []any resolviendo refs.
func (q *qwik) list(v any) []any {
	l, _ := q.deref(v, 4).([]any)
	return l
}

// scalar resuelve un valor hoja: números y textos finales.
func (q *qwik) scalar(v any) any {
	return q.deref(v, 3)
}

// str resuelve v a string ("" si no lo es).
func (q *qwik) str(v any) string {
	s, _ := q.scalar(v).(string)
	return s
}

// num resuelve v a float64 (0 si no lo es).
func (q *qwik) num(v any) float64 {
	switch n := q.scalar(v).(type) {
	case float64:
		return n
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	}
	return 0
}

// int resuelve v a int.
func (q *qwik) int(v any) int { return int(q.num(v)) }

// ─── tierlist: matriz campeón × carril ─────────────────────────────

var lanes = []string{"top", "jungle", "middle", "bottom", "support"}

func laneToRole(lane string) model.Role {
	switch lane {
	case "top":
		return model.RoleTop
	case "jungle":
		return model.RoleJungle
	case "middle":
		return model.RoleMiddle
	case "bottom":
		return model.RoleBottom
	case "support":
		return model.RoleSupport
	}
	return model.RoleAll
}

// laneRow es un campeón jugado en un carril según el tierlist.
type laneRow struct {
	cid     int
	lane    string
	games   int
	pctLane float64
}

// tierlist descifra las filas de un tierlist (filtrado por carril o
// no) y el patch vigente.
func (cl *Client) tierlist(lane string) (rows []laneRow, paths map[int]string, patch string, err error) {
	url := cl.base + "/lol/tierlist/"
	if lane != "" {
		url += "?lane=" + lane
	}
	body, err := cl.fetch(url)
	if err != nil {
		return nil, nil, "", err
	}
	q, err := parseQwik(body)
	if err != nil {
		return nil, nil, "", err
	}
	for _, o := range q.objs {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		if len(m) != 5 {
			continue
		}
		if _, hasCid := m["cid"]; !hasCid {
			continue
		}
		if _, hasRow := m["row"]; !hasRow {
			continue
		}
		if _, hasPh := m["placeholder"]; !hasPh {
			continue
		}
		row := q.obj(m["row"])
		if row == nil {
			continue
		}
		ln := q.str(row["lane"])
		if ln == "" {
			continue
		}
		rows = append(rows, laneRow{
			cid:     q.int(m["cid"]),
			lane:    ln,
			games:   q.int(row["games"]),
			pctLane: q.num(row["pctLane"]),
		})
	}
	// índice global: cid → path de URL ('ahri') y patch vigente.
	paths = map[int]string{}
	for _, o := range q.objs {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		if cp, has := m["champPath"]; has {
			pm := q.obj(cp)
			for cid, path := range pm {
				id, err := strconv.Atoi(cid)
				if err != nil {
					continue
				}
				if s := q.str(path); s != "" {
					paths[id] = s
				}
			}
		}
		if p, has := m["patch"]; has && patch == "" {
			// "16.21" → "16.21"
			if s := q.str(p); s != "" {
				patch = s
			}
		}
	}
	return rows, paths, patch, nil
}

// ─── página de build de un campeón/carril ──────────────────────────

// LoL15 son las reglas de los órdenes de Lolalytics: medidos a nivel
// 15 (los 3 niveles finales aún no se han gastado).
func LoL15() model.SkillRules {
	return model.SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
}

// buildData es lo extraído de una página /build/?lane=.
type buildData struct {
	lane   string
	games  int
	wr     float64
	items  []model.ItemStats
	boots  []model.ItemStats
	slots  [][]model.ItemStats
	combos []model.ItemCombo
	spells []model.SummonerStats
	starts []model.ItemStats
	wards  []model.ItemStats
	enemys []model.HeroRef
	runes  []model.RunePageStats
	skills []model.SkillSequenceStats
}

// mainData localiza el objeto principal de la página (el que tiene
// header, item y skillOrder).
func (q *qwik) mainData() map[string]any {
	for _, o := range q.objs {
		m, ok := o.(map[string]any)
		if !ok {
			continue
		}
		if _, h := m["header"]; !h {
			continue
		}
		if _, it := m["item"]; !it {
			continue
		}
		if _, so := m["skillOrder"]; !so {
			continue
		}
		return m
	}
	return nil
}

// statTable decodifica filas [id, wr%, pick, partidas, …] de items o
// botas.
func (q *qwik) statTable(v any) []model.ItemStats {
	var out []model.ItemStats
	for _, rowRef := range q.list(v) {
		row := q.list(rowRef)
		if len(row) < 4 {
			continue
		}
		id := q.int(row[0])
		wr := q.num(row[1])
		games := q.int(row[3])
		// Los ítems reales viven en 1001-7xxx; el 9999 es un marcador
		// interno de Lolalytics.
		if id <= 0 || id >= 8000 || games <= 0 {
			continue
		}
		// Quinta columna: minuto medio de compra (0 si la tabla no lo
		// trae) → segundos, como el resto del modelo.
		avg := 0.0
		if len(row) >= 5 {
			avg = q.num(row[4]) * 60
		}
		out = append(out, model.ItemStats{
			ID:      id,
			AvgBuyS: avg,
			Stats:   model.Stats{Games: games, Wins: int(wr / 100 * float64(games))},
		})
	}
	return out
}

// basicBoots es el id de las Botas base: la raíz de la que mejoran
// todas las demás, no una recomendación.
const basicBoots = 1001

// dropBasicBoots descarta las botas base de la tabla de botas.
func dropBasicBoots(in []model.ItemStats) []model.ItemStats {
	out := make([]model.ItemStats, 0, len(in))
	for _, b := range in {
		if b.ID != basicBoots {
			out = append(out, b)
		}
	}
	return out
}

// wardIDs son los ítems de visión (tabla item): el Punto de control
// (2055) es el que se compra, los otros dos son amuletos (el gratis y
// el de visión lejana) que compiten igual en la tabla.
var wardIDs = map[int]bool{2055: true, 3340: true, 3363: true}

// wardsOf separa las filas de wards de la tabla item.
func wardsOf(in []model.ItemStats) []model.ItemStats {
	var out []model.ItemStats
	for _, it := range in {
		if wardIDs[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// dropWards quita los wards de la tabla de ítems (respaldo sin
// popularItem) para que el 2055 no gane rankings por comprarse mucho.
func dropWards(in []model.ItemStats) []model.ItemStats {
	out := make([]model.ItemStats, 0, len(in))
	for _, it := range in {
		if !wardIDs[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// comboTable decodifica filas [combo, wr%, pick%, partidas] de
// builtBootSet3: tríos de ítems con WR conjunto (combo real, no tres
// ítems sueltos).
func (q *qwik) comboTable(v any) []model.ItemCombo {
	var out []model.ItemCombo
	for _, rowRef := range q.list(v) {
		row := q.list(rowRef)
		if len(row) < 4 {
			continue
		}
		ids := parseComboIDs(q.str(row[0]))
		if len(ids) == 0 {
			continue
		}
		games := q.int(row[3])
		if games <= 0 {
			continue
		}
		wr := q.num(row[1])
		out = append(out, model.ItemCombo{
			IDs:      ids,
			Stats:    model.Stats{Games: games, Wins: int(wr / 100 * float64(games))},
			PickRate: q.num(row[2]),
		})
	}
	return out
}

// parseComboIDs parte "3116_3047_6653" en ids; un combo son exactamente
// 3 ítems (builtBootSet3) y cualquier celda fuera de rango invalida el
// combo completo.
func parseComboIDs(s string) []int {
	var out []int
	for _, p := range strings.Split(s, "_") {
		id, err := strconv.Atoi(p)
		if err != nil || id <= 0 || id >= 8000 {
			return nil
		}
		out = append(out, id)
	}
	if len(out) != 3 {
		return nil
	}
	return out
}

// spellIDs es el set de hechizos de invocador de la Grieta: los ids
// válidos para partir pares concatenados.
var spellIDs = map[int]bool{
	1: true, 3: true, 4: true, 6: true, 7: true, 11: true,
	12: true, 13: true, 14: true, 21: true, 32: true,
}

// parseSpellPair parte el id concatenado de la tabla spells ("414" =
// Destello + Encender) en los dos ids que lo forman. Lolalytics
// concatena ordenado, así que el primer id es el menor ("114" es 1+14,
// no 11+4); un id suelto del set es una partida con un solo hechizo.
func parseSpellPair(s string) (a, b int, ok bool) {
	for cut := 1; cut <= len(s) && cut <= 2; cut++ {
		a, errA := strconv.Atoi(s[:cut])
		b, errB := strconv.Atoi(s[cut:])
		if errA == nil && errB == nil && spellIDs[a] && spellIDs[b] && a <= b {
			return a, b, true
		}
	}
	if len(s) == 1 {
		if n, err := strconv.Atoi(s); err == nil && spellIDs[n] {
			return n, 0, true
		}
	}
	return 0, 0, false
}

// spellsTable decodifica la tabla spells: pares de hechizos con WR
// conjunto, mismo formato de fila que los ítems.
func (q *qwik) spellsTable(v any) []model.SummonerStats {
	var out []model.SummonerStats
	for _, it := range q.statTable(v) {
		a, b, ok := parseSpellPair(strconv.Itoa(it.ID))
		if !ok {
			continue
		}
		out = append(out, model.SummonerStats{IDs: [2]int{a, b}, Stats: it.Stats})
	}
	return out
}

// enemyTable decodifica la tabla enemy de un carril: filas
// [idEnemigo, wr%, pick%, deltaWR, %partidas, partidas]. La col5 son
// las partidas contra ese enemigo (verificado contra caché real:
// 2105/37840 = 5.56% de la col4).
func (q *qwik) enemyTable(v any, lane string) []model.HeroRef {
	e := q.obj(v)
	laneRows := q.list(e[lane])
	var out []model.HeroRef
	for _, rowRef := range laneRows {
		row := q.list(rowRef)
		if len(row) < 6 {
			continue
		}
		id := q.num(row[0])
		games := q.num(row[5])
		wr := q.num(row[1])
		if id <= 0 || games < 1 {
			continue
		}
		out = append(out, model.HeroRef{
			HeroID: int(id),
			Games:  int(games),
			Wins:   int(wr / 100 * games),
		})
	}
	return out
}

// quickbuild extrae un bloque quickbuild (pick o win): runas y orden
// de habilidades con sus muestras.
func (q *qwik) quickbuild(v any, games int) (runes *model.RunePageStats, skill *model.SkillSequenceStats) {
	qb := q.obj(v)
	if qb == nil {
		return nil, nil
	}
	// Runas: página (árboles) + set (ids) + wr/n del bloque.
	if r := q.obj(qb["runes"]); r != nil {
		page := q.obj(r["page"])
		set := q.obj(r["set"])
		if page != nil && set != nil {
			pri := q.list(set["pri"])
			sec := q.list(set["sec"])
			mod := q.list(set["mod"])
			if len(pri) >= 1 {
				rp := &model.RunePageStats{
					PrimaryTree:   treeFromIndex(q.int(page["pri"])),
					SecondaryTree: treeFromIndex(q.int(page["sec"])),
					KeystoneID:    q.int(pri[0]),
				}
				for _, id := range pri[1:] {
					rp.PrimaryIDs = append(rp.PrimaryIDs, q.int(id))
				}
				for _, id := range sec {
					rp.SecondaryIDs = append(rp.SecondaryIDs, q.int(id))
				}
				for i, id := range mod {
					if i < len(rp.Shards) {
						rp.Shards[i] = q.int(id)
					}
				}
				rp.Games = q.int(r["n"])
				rp.Wins = int(q.num(r["wr"]) / 100 * float64(rp.Games))
				if rp.Games == 0 && games > 0 {
					rp.Games, rp.Wins = games, int(q.num(r["wr"])/100*float64(games))
				}
				runes = rp
			}
		}
	}
	// Habilidades. Lolalytics publica DOS representaciones:
	//   · skillorder.id     → secuencia por nivel (15 dígitos, 1..4 = QWER)
	//   · skillpriority.id  → orden de maximización ("QEW" = Q > E > W)
	//
	// La prioridad es independiente y válida aunque la secuencia no lo
	// sea, así que se conserva siempre. La secuencia se valida aquí: si
	// sus 15 dígitos no son legales con las reglas del juego, NO se
	// propaga (no se relaja la validación para aceptarla).
	games, wins := 0, 0
	var seq string
	if so := q.obj(q.obj(qb["skillorder"])); so != nil {
		games, wins = q.int(so["n"]), int(q.num(so["wr"])/100*float64(q.int(so["n"])))
		switch v := q.scalar(so["id"]).(type) {
		case string:
			seq, _ = digitsToQWER(v)
		case float64:
			seq, _ = digitsToQWER(strconv.FormatFloat(v, 'f', 0, 64))
		}
	}
	var prio string
	if sp := q.obj(q.obj(qb["skillpriority"])); sp != nil {
		p, _ := q.scalar(sp["id"]).(string)
		if isSkillPriority(p) {
			prio = p
			// La prioridad la comparte todo el campeón, pero su muestra
			// es mayor: si la secuencia no aporta, se usa la suya.
			if games == 0 {
				games, wins = q.int(sp["n"]), int(q.num(sp["wr"])/100*float64(q.int(sp["n"])))
			}
		}
	}
	if seq != "" || prio != "" {
		skill = &model.SkillSequenceStats{
			Sequence: seq,
			Priority: prio,
			Stats:    model.Stats{Games: games, Wins: wins},
		}
	}
	return runes, skill
}

// treeFromIndex mapea el índice de árbol de Lolalytics (0-4) al id de
// estilo de Data Dragon. Los valores que ya vienen como id se pasan.
func treeFromIndex(i int) int {
	trees := []int{8000, 8100, 8200, 8300, 8400}
	if i >= 0 && i < len(trees) {
		return trees[i]
	}
	return i
}

// digitsToQWER convierte "213114121242233" a "WQEWQQ..." (1=Q 2=W 3=E
// 4=R).
func digitsToQWER(s string) (string, bool) {
	if len(s) != 15 {
		return "", false
	}
	var b strings.Builder
	for _, d := range s {
		switch d {
		case '1':
			b.WriteRune('Q')
		case '2':
			b.WriteRune('W')
		case '3':
			b.WriteRune('E')
		case '4':
			b.WriteRune('R')
		default:
			return "", false
		}
	}
	return b.String(), true
}

// buildPage descarga y decodifica la página de build de un campeón en
// un carril.
func (cl *Client) buildPage(path, lane string) (*buildData, error) {
	url := fmt.Sprintf("%s/lol/%s/build/", cl.base, path)
	if lane != "" {
		url += "?lane=" + lane
	}
	body, err := cl.fetch(url)
	if err != nil {
		return nil, err
	}
	q, err := parseQwik(body)
	if err != nil {
		return nil, err
	}
	main := q.mainData()
	if main == nil {
		return nil, fmt.Errorf("lolalytics: %s: sin objeto de datos", path)
	}
	header := q.obj(main["header"])
	// popularItem ya viene filtrado por Lolalytics: solo ítems
	// completados. La tabla item incluye componentes, consumibles y
	// wards —se compran en casi todas las partidas y ganan cualquier
	// ranking por WR—, así que solo se usa como respaldo; de ella salen
	// los wards, que van por separado.
	itemTable := q.statTable(main["item"])
	items := q.statTable(main["popularItem"])
	if len(items) == 0 {
		items = dropWards(itemTable)
	}
	bd := &buildData{
		lane:  lane,
		games: q.int(main["n"]),
		wr:    q.num(header["wr"]),
		items: items,
		wards: wardsOf(itemTable),
		boots: dropBasicBoots(q.statTable(main["boots"])),
		// Tablas por turno de compra (item1..item5) y tríos con WR
		// conjunto (builtBootSet3): el "orden de la build" sin inferir.
		slots: [][]model.ItemStats{
			q.statTable(main["item1"]),
			q.statTable(main["item2"]),
			q.statTable(main["item3"]),
			q.statTable(main["item4"]),
			q.statTable(main["item5"]),
		},
		combos: q.comboTable(main["builtBootSet3"]),
		spells: q.spellsTable(main["spells"]),
		starts: q.statTable(main["startItem"]),
		enemys: q.enemyTable(main["enemy"], lane),
	}
	// quickbuilds: summary.pick (más jugado) y summary.win (mejor WR).
	summary := q.obj(main["summary"])
	if summary != nil {
		for _, k := range []string{"pick", "win"} {
			rb, sk := q.quickbuild(summary[k], bd.games)
			if rb != nil {
				bd.runes = append(bd.runes, *rb)
			}
			if sk != nil {
				bd.skills = append(bd.skills, *sk)
			}
		}
	}
	return bd, nil
}

// ─── carga completa ────────────────────────────────────────────────

// LoadBuilds compone los datos crudos de todos los campeones/carriles
// con muestra. champsByID resuelve el id numérico de campeón a la
// clave de Data Dragon (para nombres e iconos). Sin red ni caché,
// delega en los fixtures de u.gg.
func (cl *Client) LoadBuilds(champsByID map[int]string, fallback func() ([]*model.ChampionBuilds, error)) ([]*model.ChampionBuilds, error) {
	if fallback == nil {
		fallback = func() ([]*model.ChampionBuilds, error) {
			return nil, fmt.Errorf("lolalytics: sin fuente alternativa")
		}
	}
	// 1) matriz campeón × carril desde los 5 tierlists.
	type key struct {
		cid  int
		lane string
	}
	matrix := map[key]laneRow{}
	paths := map[int]string{}
	patch := ""
	for _, lane := range lanes {
		rows, p, pt, err := cl.tierlist(lane)
		if err != nil {
			cl.fellBack = true
			return fallback()
		}
		for k, v := range p {
			if _, ok := paths[k]; !ok {
				paths[k] = v
			}
		}
		if patch == "" {
			patch = pt
		}
		for _, r := range rows {
			if r.games < minLaneGames {
				continue
			}
			matrix[key{r.cid, r.lane}] = r
		}
	}
	if len(matrix) == 0 {
		cl.fellBack = true
		return fallback()
	}
	// Orden estable de proceso.
	pairs := make([]key, 0, len(matrix))
	for kk := range matrix {
		pairs = append(pairs, kk)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].cid != pairs[j].cid {
			return pairs[i].cid < pairs[j].cid
		}
		return pairs[i].lane < pairs[j].lane
	})

	// 2) página de build por pareja campeón × carril, con un pool
	// pequeño: lo caro es la latencia de ~200 páginas, no la CPU.
	type job struct {
		key   key
		path  string
		name  string // clave del campeón ("Ahri")
		label string // para el progreso: "Ahri · middle"
	}
	var jobs []job
	for _, kk := range pairs {
		keyStr, ok := champsByID[kk.cid]
		if !ok {
			continue
		}
		path, ok := paths[kk.cid]
		if !ok {
			continue
		}
		jobs = append(jobs, job{key: kk, path: path, name: keyStr, label: keyStr + " · " + kk.lane})
	}

	sem := make(chan struct{}, max(1, buildWorkers))
	var (
		mu    sync.Mutex
		out   []*model.ChampionBuilds
		done  int
		total = len(jobs)
		wg    sync.WaitGroup
	)
	for _, j := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			bd, err := cl.buildPage(j.path, j.key.lane)

			mu.Lock()
			done++
			n, tl := done, total
			if err == nil {
				out = append(out, buildFrom(j.key.cid, j.name, j.key.lane, patch, bd))
			}
			if cl.onProgress != nil {
				cl.onProgress(n, tl, j.label)
			}
			mu.Unlock()
		}(j)
	}
	wg.Wait()

	// Orden estable: el pool termina en cualquier orden y la lista de
	// la UI se ordena por WR, pero el orden de llegada hacía que dos
	// cargas idénticas dieran listas distintas.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChampionID != out[j].ChampionID {
			return out[i].ChampionID < out[j].ChampionID
		}
		return out[i].Role < out[j].Role
	})
	if len(out) == 0 {
		cl.fellBack = true
		return fallback()
	}
	cl.fellBack = false
	return out, nil
}

// buildFrom arma el ChampionBuilds crudo con lo que trae la página.
func buildFrom(cid int, keyStr, lane, patch string, bd *buildData) *model.ChampionBuilds {
	return &model.ChampionBuilds{
		Game:        model.GameLoL,
		ChampionID:  cid,
		ChampionKey: keyStr,
		Role:        laneToRole(lane),
		Patch:       patch,
		Source:      "lolalytics.com",
		Total:       model.Stats{Games: bd.games, Wins: int(bd.wr / 100 * float64(bd.games))},
		Items:       bd.items,
		Boots:       bd.boots,
		Wards:       bd.wards,
		ItemSlots:   bd.slots,
		ItemCombos:  bd.combos,
		Spells:      bd.spells,
		StartItems:  bd.starts,
		Matchups:    bd.enemys,
		RunePages:   bd.runes,
		Skills:      bd.skills,
		Rules:       LoL15(),
	}
}

// isSkillPriority dice si s es una prioridad de habilidades bien formada:
// una permutación de QWER (3 letras en LoL, hasta 4 en otros juegos), sin
// repeticiones.
func isSkillPriority(s string) bool {
	if len(s) < 2 || len(s) > 4 {
		return false
	}
	seen := map[rune]bool{}
	for _, r := range s {
		if !strings.ContainsRune("QWER", r) || seen[r] {
			return false
		}
		seen[r] = true
	}
	return true
}
