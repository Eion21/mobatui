// api.go conecta la API comunitaria deadlock-api.com (v1): datos
// estáticos (héroes, ítems, habilidades) y analíticas de builds (ítems
// y órdenes de habilidades con partidas/victorias). Todo pasa por la
// caché local con TTL; sin red, los estáticos caen al respaldo local y
// las builds a los fixtures.
package deadlock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

const (
	defaultAPIBase = "https://api.deadlock-api.com"
	userAgent      = "builds-tui/1.0 (+https://github.com/jervis/builds)"
	// TTLs por tipo de recurso.
	ttlVersions = time.Hour
	ttlAssets   = 24 * time.Hour
	ttlStats    = 6 * time.Hour
	ttlIcon     = 720 * time.Hour
	// minMatches recorta las respuestas de analíticas y complementa el
	// filtro de Wilson del pipeline.
	minMatches = 100
)

// APIClient es el cliente de deadlock-api.com; implementa
// staticdata.Provider y carga builds reales.
type APIClient struct {
	c            *cache.Cache
	http         *http.Client
	apiBase      string
	fixtureDir   string // respaldo estático (testdata/deadlock)
	fixtureBuild string // fixtures de builds (testdata/deadlock/builds)
	// onProgress avisa cuántos héroes van cargados, como el pool de
	// Lolalytics; nil = a nadie le importa el progreso.
	onProgress func(done, total int, name string)
	// fellBack se activa cuando la carga terminó sirviendo fixtures.
	fellBack bool
}

var _ staticdata.Provider = (*APIClient)(nil)

// OnProgress registra el callback de progreso de la carga: igual que
// el pool de Lolalytics, avisa héroe por héroe.
func (a *APIClient) OnProgress(fn func(done, total int, name string)) *APIClient {
	a.onProgress = fn
	return a
}

func NewAPI(c *cache.Cache, fixtureDir string) *APIClient {
	return &APIClient{
		c:            c,
		http:         &http.Client{Timeout: 20 * time.Second},
		apiBase:      defaultAPIBase,
		fixtureDir:   fixtureDir,
		fixtureBuild: filepath.Join(fixtureDir, "builds"),
	}
}

// WithHTTPClient sustituye el cliente HTTP (inyección en tests).
func (a *APIClient) WithHTTPClient(h *http.Client) *APIClient {
	a.http = h
	return a
}

// WithAPIBase apunta a otro servidor (tests).
func (a *APIClient) WithAPIBase(u string) *APIClient {
	a.apiBase = u
	return a
}

// get resuelve una URL de la API con caché → HTTP → respaldo local.
func (a *APIClient) get(path, fixtureName string, ttl time.Duration) ([]byte, error) {
	url := a.apiBase + path
	res, err := a.c.Get(url, ttl, func() ([]byte, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("deadlock-api: %s → HTTP %d", path, resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	})
	if err == nil {
		return res.Body, nil
	}
	if fixtureName != "" {
		if data, ferr := os.ReadFile(filepath.Join(a.fixtureDir, fixtureName)); ferr == nil {
			return data, nil
		}
	}
	return nil, err
}

// getJSON combina get con la decodificación.
func getJSON[T any](a *APIClient, path, fixture string, ttl time.Duration, out *T) error {
	body, err := a.get(path, fixture, ttl)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("deadlock-api: %s inválido: %w", path, err)
	}
	return nil
}

// ─── formas de la API ──────────────────────────────────────────────

type apiImages struct {
	IconImageSmall string `json:"icon_image_small"`
}

type apiHero struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	PlayerSelectable bool      `json:"player_selectable"`
	InDevelopment    bool      `json:"in_development"`
	Disabled         bool      `json:"disabled"`
	Images           apiImages `json:"images"`
	Description      struct {
		Role string `json:"role"`
	} `json:"description"`
}

type apiItem struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`       // upgrade | weapon | ability
	Image        string `json:"image"`      // icono interno del mod (incompleto)
	ShopImage    string `json:"shop_image"` // icono de la tienda (200x200, el correcto)
	Cost         int    `json:"cost"`
	Hero         int    `json:"hero"`
	ItemSlotType string `json:"item_slot_type"` // weapon | vitality | spirit
	ItemTier     int    `json:"item_tier"`
	// ClassName y ComponentItems modelan la cadena de mejoras: un ítem
	// que lista a otro en component_items es su mejora directa.
	ClassName      string   `json:"class_name"`
	ComponentItems []string `json:"component_items"`
	Description    struct {
		Desc string `json:"desc"`
	} `json:"description"`
}

type apiHeroStat struct {
	HeroID  int `json:"hero_id"`
	Wins    int `json:"wins"`
	Matches int `json:"matches"`
}

type apiItemStat struct {
	ItemID     int     `json:"item_id"`
	Wins       int     `json:"wins"`
	Matches    int     `json:"matches"`
	AvgBuyTime float64 `json:"avg_buy_time_s"`
}

type apiAbilityStat struct {
	Abilities []int64 `json:"abilities"`
	Wins      int     `json:"wins"`
	Matches   int     `json:"matches"`
}

type apiCounterStat struct {
	HeroID        int `json:"hero_id"`
	EnemyHeroID   int `json:"enemy_hero_id"`
	Wins          int `json:"wins"`
	MatchesPlayed int `json:"matches_played"`
}

type apiHeroStatBucketed struct {
	HeroID  int   `json:"hero_id"`
	Bucket  int64 `json:"bucket"`
	Wins    int   `json:"wins"`
	Matches int   `json:"matches"`
}

type apiSynergyStat struct {
	HeroID1       int `json:"hero_id1"`
	HeroID2       int `json:"hero_id2"`
	Wins          int `json:"wins"`
	MatchesPlayed int `json:"matches_played"`
}

// ─── staticdata.Provider ───────────────────────────────────────────

// Version devuelve la versión de cliente más reciente conocida.
func (a *APIClient) Version() (string, error) {
	var versions []int
	if err := getJSON(a, "/v1/assets/client-versions", "", ttlVersions, &versions); err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("deadlock-api: sin versiones")
	}
	latest := versions[0]
	for _, v := range versions {
		if v > latest {
			latest = v
		}
	}
	return fmt.Sprintf("client %d", latest), nil
}

func (a *APIClient) heroes() ([]apiHero, error) {
	var hs []apiHero
	if err := getJSON(a, "/v1/assets/heroes", "heroes.json", ttlAssets, &hs); err != nil {
		return nil, err
	}
	return hs, nil
}

// Heroes indexados por nombre, solo los seleccionables y publicados.
func (a *APIClient) Heroes() (map[string]staticdata.HeroInfo, error) {
	hs, err := a.heroes()
	if err != nil {
		return nil, err
	}
	out := make(map[string]staticdata.HeroInfo, len(hs))
	for _, h := range hs {
		if !h.PlayerSelectable || h.InDevelopment || h.Disabled {
			continue
		}
		out[h.Name] = staticdata.HeroInfo{ID: h.ID, Name: h.Name, Title: h.Description.Role}
	}
	return out, nil
}

// HeroNamesByID resuelve id→nombre con todos los héroes publicados,
// incluidos los jugables marcados in_development: los endpoints de
// cruces (matchups, sinergias) los traen con muestras reales aunque el
// roster no los liste (Violet, Baba).
func (a *APIClient) HeroNamesByID() map[int]string {
	hs, err := a.heroes()
	if err != nil {
		return nil
	}
	out := make(map[int]string, len(hs))
	for _, h := range hs {
		if h.ID != 0 && h.Name != "" {
			out[h.ID] = h.Name
		}
	}
	return out
}

// shopItems devuelve los ítems de tienda (upgrade+weapon) y, aparte,
// los nombres de las habilidades por id.
func (a *APIClient) shopItems() (shop map[int]apiItem, abilityNames map[int]string, err error) {
	var items []apiItem
	if err := getJSON(a, "/v1/assets/items", "items.json", ttlAssets, &items); err != nil {
		return nil, nil, err
	}
	shop = map[int]apiItem{}
	abilityNames = map[int]string{}
	for _, it := range items {
		switch it.Type {
		case "upgrade", "weapon":
			shop[it.ID] = it
		case "ability":
			if it.Name != "" {
				abilityNames[it.ID] = it.Name
			}
		}
	}
	return shop, abilityNames, nil
}

// Items id → nombre de los ítems de tienda.
func (a *APIClient) Items() (map[int]string, error) {
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(shop))
	for id, it := range shop {
		out[id] = it.Name
	}
	return out, nil
}

// Runes: Deadlock no tiene runas.
func (a *APIClient) Runes() (map[int]string, map[int]string, error) {
	return map[int]string{}, map[int]string{}, nil
}

// ItemDetails expone tier, categoría de tienda, coste y descripción
// de los ítems comprables (weapon/vitality/spirit).
func (a *APIClient) ItemDetails() (map[int]staticdata.ItemInfo, error) {
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	out := make(map[int]staticdata.ItemInfo, len(shop))
	for id, it := range shop {
		if it.ItemSlotType == "" {
			continue
		}
		out[id] = staticdata.ItemInfo{
			Name:        it.Name,
			Tier:        it.ItemTier,
			Category:    it.ItemSlotType,
			Cost:        it.Cost,
			Description: stripHTML(it.Description.Desc),
		}
	}
	return out, nil
}

// ItemUpgrades arma el mapa base → mejoras directas: un ítem que
// lista a otro en component_items (vía class_name) es su mejora.
func (a *APIClient) ItemUpgrades() (map[int][]int, error) {
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	byClass := make(map[string]int, len(shop))
	for id, it := range shop {
		if it.ClassName != "" {
			byClass[it.ClassName] = id
		}
	}
	out := map[int][]int{}
	for _, it := range shop {
		for _, comp := range it.ComponentItems {
			if base, ok := byClass[comp]; ok {
				out[base] = append(out[base], it.ID)
				break // una entrada por componente; la primera mejora directa
			}
		}
	}
	return out, nil
}

// stripHTML limpia el HTML simple de las descripciones de la API.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// ChampionIcon descarga (con caché) el icono del héroe desde el CDN.
func (a *APIClient) ChampionIcon(name string) ([]byte, error) {
	hs, err := a.heroes()
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		if h.Name != name {
			continue
		}
		if h.Images.IconImageSmall == "" {
			break
		}
		res, err := a.c.Get(h.Images.IconImageSmall, ttlIcon, a.fetchURL(h.Images.IconImageSmall))
		if err == nil {
			return res.Body, nil
		}
		break
	}
	// Respaldo local.
	return os.ReadFile(filepath.Join(a.fixtureDir, name+".png"))
}

// ItemIcon descarga (con caché) el icono del ítem desde el CDN.
// Prefiere shop_image (el icono cuadrado de la tienda): el campo image
// es el icono interno del mod, con huecos (404) y otro estilo.
func (a *APIClient) ItemIcon(id int) ([]byte, error) {
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	if it, ok := shop[id]; ok {
		for _, url := range []string{it.ShopImage, it.Image} {
			if url == "" {
				continue
			}
			res, err := a.c.Get(url, ttlIcon, a.fetchURL(url))
			if err == nil {
				return res.Body, nil
			}
		}
	}
	return os.ReadFile(filepath.Join(a.fixtureDir, fmt.Sprintf("%d.png", id)))
}

func (a *APIClient) fetchURL(url string) func() ([]byte, error) {
	return func() ([]byte, error) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("deadlock-api: CDN → HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	}
}

// ─── carga de builds ───────────────────────────────────────────────

// LoadBuilds compone los datos crudos de todos los héroes con muestra:
// overview global, ranking de ítems por héroe y órdenes de
// habilidades con sus topes reales. Si la API está inaccesible y no
// hay caché, cae a los fixtures locales.
func (a *APIClient) LoadBuilds() ([]*model.ChampionBuilds, error) {
	patch, _ := a.Version()
	var heroStats []apiHeroStat
	err := getJSON(a, "/v1/analytics/hero-stats", "", ttlStats, &heroStats)
	if err != nil {
		// Sin red ni caché: fixtures.
		a.fellBack = true
		return LoadDir(a.fixtureBuild)
	}
	shop, abilityNames, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	// id → nombre de héroe (clave del proveedor estático).
	apiHeroes, err := a.heroes()
	if err != nil {
		return nil, err
	}
	nameOf := make(map[int]string, len(apiHeroes))
	for _, h := range apiHeroes {
		nameOf[h.ID] = h.Name
	}
	// Matchups, sinergias y tendencia mensual (tolerantes a fallo: son
	// informativas).
	matchups, synergies := a.loadCrossStats(nameOf)
	trend := a.loadTrend()
	omitted := 0

	var out []*model.ChampionBuilds
	for i, hs := range heroStats {
		name := nameOf[hs.HeroID]
		if name == "" {
			omitted++
			continue // héroe sin asset público: sin nombre ni iconos, fuera
		}
		cb, err := a.loadHeroBuilds(hs, name, shop, abilityNames, patch)
		if a.onProgress != nil {
			a.onProgress(i+1, len(heroStats), name)
		}
		if err != nil {
			// Un héroe con datos corruptos no tumba toda la carga.
			omitted++
			continue
		}
		if cb != nil {
			cb.Matchups = matchups[hs.HeroID]
			cb.Synergies = synergies[hs.HeroID]
			cb.WRTrend = trend[hs.HeroID]
			out = append(out, cb)
		}
	}
	if len(out) == 0 {
		a.fellBack = true
		return LoadDir(a.fixtureBuild)
	}
	a.fellBack = false
	omittedTotal = omitted
	return out, nil
}

// Omitted devuelve el número de héroes descartados por la última
// carga (sin asset público o con datos corruptos).
func (a *APIClient) Omitted() int { return omittedTotal }

// DataState implementa staticdata.FreshnessProvider: dice si lo que
// se está sirviendo viene de red, de caché fresca, de caché caducada
// o de los fixtures locales.
func (a *APIClient) DataState() (string, time.Duration, bool) {
	if a.fellBack {
		return "fixtures", 0, true
	}
	s, at, ok := a.c.Last()
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

var omittedTotal int

// loadTrend agrega el WR mensual de cada héroe (últimas 6 ventanas).
func (a *APIClient) loadTrend() map[int][]model.WRTrendPoint {
	var rows []apiHeroStatBucketed
	if err := getJSON(a, "/v1/analytics/hero-stats?bucket=start_time_month", "", ttlStats, &rows); err != nil {
		return nil
	}
	byHero := map[int][]model.WRTrendPoint{}
	for _, r := range rows {
		if r.Matches <= 0 {
			continue
		}
		byHero[r.HeroID] = append(byHero[r.HeroID], model.WRTrendPoint{
			Time:  r.Bucket,
			WR:    float64(r.Wins) / float64(r.Matches),
			Games: r.Matches,
		})
	}
	for h, pts := range byHero {
		sort.Slice(pts, func(i, j int) bool { return pts[i].Time < pts[j].Time })
		if len(pts) > 6 {
			pts = pts[len(pts)-6:]
		}
		byHero[h] = pts
	}
	return byHero
}

// loadCrossStats agrega counters y sinergias por héroe. Los
// endpoints devuelven TODOS los pares de una vez.
func (a *APIClient) loadCrossStats(nameOf map[int]string) (matchups, synergies map[int][]model.HeroRef) {
	matchups, synergies = map[int][]model.HeroRef{}, map[int][]model.HeroRef{}
	var counters []apiCounterStat
	if err := getJSON(a, "/v1/analytics/hero-counter-stats", "", ttlStats, &counters); err == nil {
		for _, c := range counters {
			if _, ok := nameOf[c.EnemyHeroID]; !ok {
				continue
			}
			matchups[c.HeroID] = append(matchups[c.HeroID], model.HeroRef{
				HeroID: c.EnemyHeroID, Games: c.MatchesPlayed, Wins: c.Wins,
			})
		}
	}
	var syns []apiSynergyStat
	if err := getJSON(a, "/v1/analytics/hero-synergy-stats", "", ttlStats, &syns); err == nil {
		for _, s := range syns {
			partner := s.HeroID2
			if _, ok := nameOf[partner]; !ok {
				continue
			}
			synergies[s.HeroID1] = append(synergies[s.HeroID1], model.HeroRef{
				HeroID: partner, Games: s.MatchesPlayed, Wins: s.Wins,
			})
		}
	}
	return matchups, synergies
}

// loadHeroBuilds carga ítems y órdenes de habilidades de un héroe.
// Devuelve nil si no hay muestra suficiente.
func (a *APIClient) loadHeroBuilds(hs apiHeroStat, name string, shop map[int]apiItem, abilityNames map[int]string, patch string) (*model.ChampionBuilds, error) {
	// Ítems comprados en partidas de este héroe.
	var itemStats []apiItemStat
	path := fmt.Sprintf("/v1/analytics/item-stats?hero_id=%d&min_matches=%d", hs.HeroID, minMatches)
	if err := getJSON(a, path, "", ttlStats, &itemStats); err != nil {
		return nil, err
	}

	// Órdenes de habilidades.
	var abilStats []apiAbilityStat
	path = fmt.Sprintf("/v1/analytics/ability-order-stats?hero_id=%d&min_matches=%d", hs.HeroID, minMatches)
	if err := getJSON(a, path, "", ttlStats, &abilStats); err != nil {
		return nil, err
	}
	if len(itemStats) == 0 || len(abilStats) == 0 {
		return nil, nil
	}

	cb := &model.ChampionBuilds{
		Game:        model.GameDeadlock,
		ChampionID:  hs.HeroID,
		ChampionKey: name,
		Role:        model.RoleAll,
		Patch:       patch,
		Source:      "deadlock-api.com",
		Total:       model.Stats{Games: hs.Matches, Wins: hs.Wins},
	}

	// Ítems: solo los de tienda (weapon/upgrade ya vienen solos en las
	// stats, pero nos aseguramos con el join). El tiempo medio de
	// compra alimenta la agrupación por fase.
	for _, s := range itemStats {
		if _, ok := shop[s.ItemID]; !ok {
			continue
		}
		cb.Items = append(cb.Items, model.ItemStats{
			ID:      s.ItemID,
			Stats:   model.Stats{Games: s.Matches, Wins: s.Wins},
			AvgBuyS: s.AvgBuyTime,
		})
	}

	// Habilidades: mapear ids reales → slots 1..N por uso total
	// (slot 1 = la habilidad más jugada) y derivar topes por slot.
	seqs, rules, legend, err := buildSkillSequences(abilStats, abilityNames)
	if err != nil {
		return nil, fmt.Errorf("héroe %d: %w", hs.HeroID, err)
	}
	cb.Skills = seqs
	cb.Rules = rules
	cb.AbilityNames = legend
	return cb, nil
}

// buildSkillSequences convierte las secuencias de ids de habilidad al
// alfabeto de slots, asigna slots por uso total descendente y deriva
// los topes reales (máximo observado por slot).
func buildSkillSequences(stats []apiAbilityStat, abilityNames map[int]string) ([]model.SkillSequenceStats, model.SkillRules, map[rune]string, error) {
	if len(stats) == 0 {
		return nil, model.SkillRules{}, nil, fmt.Errorf("sin secuencias")
	}
	length := len(stats[0].Abilities)
	use := map[int64]int{} // id → partidas totales
	for _, s := range stats {
		if len(s.Abilities) != length {
			continue // filas con otra longitud (datos a medio subir): fuera
		}
		for _, id := range s.Abilities {
			use[id] += s.Matches
		}
	}
	ids := make([]int64, 0, len(use))
	for id := range use {
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > 9 {
		return nil, model.SkillRules{}, nil, fmt.Errorf("%d habilidades distintas (inesperado)", len(ids))
	}
	sort.Slice(ids, func(i, j int) bool {
		if use[ids[i]] != use[ids[j]] {
			return use[ids[i]] > use[ids[j]] // más jugada primero
		}
		return ids[i] < ids[j] // desempate estable
	})
	slotOf := map[int64]rune{}
	legend := map[rune]string{}
	caps := map[rune]int{}
	for i, id := range ids {
		slot := rune('1' + i)
		slotOf[id] = slot
		if n, ok := abilityNames[int(id)]; ok {
			legend[slot] = n
		}
		caps[slot] = 0
	}
	rules := model.SkillRules{
		Alphabet:  make([]rune, len(ids)),
		Length:    length,
		MaxPoints: caps,
	}
	for i := range ids {
		rules.Alphabet[i] = rune('1' + i)
	}

	var seqs []model.SkillSequenceStats
	for _, s := range stats {
		if len(s.Abilities) != length {
			continue
		}
		var b []byte
		counts := map[rune]int{}
		ok := true
		for _, id := range s.Abilities {
			slot, found := slotOf[id]
			if !found {
				ok = false
				break
			}
			b = append(b, byte(slot))
			counts[slot]++
		}
		if !ok {
			continue
		}
		// Topes = máximo observado.
		for slot, n := range counts {
			if n > rules.MaxPoints[slot] {
				rules.MaxPoints[slot] = n
			}
		}
		seqs = append(seqs, model.SkillSequenceStats{
			Sequence: string(b),
			Stats:    model.Stats{Games: s.Matches, Wins: s.Wins},
		})
	}
	if len(seqs) == 0 {
		return nil, model.SkillRules{}, nil, fmt.Errorf("sin secuencias válidas")
	}
	return seqs, rules, legend, nil
}
