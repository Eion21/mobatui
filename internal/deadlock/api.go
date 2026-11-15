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
	"sync"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/progression"
	"github.com/Eion21/mobatui/internal/staticdata"
)

const (
	defaultAPIBase = "https://api.deadlock-api.com"
	userAgent      = "mobatui/1.0 (+https://github.com/Eion21/mobatui)"
	// TTLs por tipo de recurso.
	ttlVersions = time.Hour
	ttlAssets   = 24 * time.Hour
	ttlStats    = 6 * time.Hour
	ttlIcon     = 720 * time.Hour
	// minMatches recorta las respuestas de analíticas y complementa el
	// filtro de Wilson del pipeline.
	minMatches = 100
)

type staticSnapshot struct {
	mu           sync.Mutex
	heroes       []apiHero
	heroesErr    error
	heroesLoaded bool
	shop         map[int]apiItem
	abilityNames map[int]string
	abilityClass map[string]int
	shopErr      error
	shopLoaded   bool
}

// APIClient es el cliente de deadlock-api.com; implementa
// staticdata.Provider y carga builds reales.
type APIClient struct {
	c            *cache.Cache
	http         *http.Client
	apiBase      string
	fixtureDir   string // respaldo estático (testdata/deadlock)
	fixtureBuild string // fixtures de builds (testdata/deadlock/builds)
	snapshot     *staticSnapshot
	// onProgress avisa cuántos héroes van cargados, como el pool de
	// Lolalytics; nil = a nadie le importa el progreso.
	onProgress func(done, total int, name string)
	// fellBack se activa cuando la carga terminó sirviendo fixtures.
	fellBack bool
	// runsCat/runsComps memorizan el catálogo de tienda y sus cadenas
	// para no re-decodificar 782 ítems en cada consulta de ruta.
	runsCatMu sync.Mutex
	runsCat   map[int]apiItem
	runsComps map[int][]int
	// progCat memoiza el catálogo de progresión (cadenas validadas).
	progCatMu sync.Mutex
	progCat   *progression.Catalog
	// allItemsCache memoiza el catálogo completo de ítems (incluye
	// habilidades y sus dependent_abilities).
	allItemsMu    sync.Mutex
	allItemsCache []apiItem
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
	// Items trae los class_name del kit: signature1..4 son las cuatro
	// habilidades en el orden canónico del héroe (el que usa el juego
	// para las teclas 1-4). Es la única fuente fiable del orden: la
	// frecuencia de uso NO lo es (en Solomon la 3 es la más subida).
	Items map[string]string `json:"items"`
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
	// DependentAbilities declara sub-habilidades: una habilidad que
	// cambia de forma (Silver) expone la variante aquí con el flag
	// DisplayAsSubAbility. Es la relación canónica del juego, no una
	// heurística por nombre.
	DependentAbilities map[string]apiDependent `json:"dependent_abilities"`
	Description        struct {
		Desc string `json:"desc"`
	} `json:"description"`
}

// apiDependent describe una habilidad dependiente y cómo se relaciona
// con su principal.
type apiDependent struct {
	Flags []string `json:"flags"`
}

// IsSubAbility indica que la dependiente ocupa el mismo espacio que su
// principal (cambio de forma), no una tecla propia.
func (d apiDependent) IsSubAbility() bool {
	for _, f := range d.Flags {
		if f == "DisplayAsSubAbility" {
			return true
		}
	}
	return false
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
	if a.snapshot != nil {
		a.snapshot.mu.Lock()
		if a.snapshot.heroesLoaded {
			hs, err := a.snapshot.heroes, a.snapshot.heroesErr
			a.snapshot.mu.Unlock()
			return hs, err
		}
		a.snapshot.mu.Unlock()
	}

	var hs []apiHero
	err := getJSON(a, "/v1/assets/heroes", "heroes.json", ttlAssets, &hs)

	if a.snapshot != nil {
		a.snapshot.mu.Lock()
		a.snapshot.heroes = hs
		a.snapshot.heroesErr = err
		a.snapshot.heroesLoaded = true
		a.snapshot.mu.Unlock()
	}
	return hs, err
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
	shop, abilityNames, _, err = a.shopItemsFull()
	return shop, abilityNames, err
}

// abilityClassIndex resuelve class_name → id de habilidad del catálogo.
func (a *APIClient) abilityClassIndex() map[string]int {
	_, _, class, _ := a.shopItemsFull()
	return class
}

// shopItemsFull es shopItems más el índice class_name → id de habilidad,
// necesario para reconstruir el orden canónico del kit (signature1..4).
func (a *APIClient) shopItemsFull() (shop map[int]apiItem, abilityNames map[int]string, abilityClass map[string]int, err error) {
	if a.snapshot != nil {
		a.snapshot.mu.Lock()
		if a.snapshot.shopLoaded {
			s, an, ac, e := a.snapshot.shop, a.snapshot.abilityNames, a.snapshot.abilityClass, a.snapshot.shopErr
			a.snapshot.mu.Unlock()
			return s, an, ac, e
		}
		a.snapshot.mu.Unlock()
	}

	var items []apiItem
	err = getJSON(a, "/v1/assets/items", "items.json", ttlAssets, &items)
	if err == nil {
		shop = map[int]apiItem{}
		abilityNames = map[int]string{}
		abilityClass = map[string]int{}
		for _, it := range items {
			switch it.Type {
			case "upgrade", "weapon":
				shop[it.ID] = it
			case "ability":
				if it.Name != "" {
					abilityNames[it.ID] = it.Name
				}
				if it.ClassName != "" {
					abilityClass[it.ClassName] = it.ID
				}
			}
		}
	}

	if a.snapshot != nil {
		a.snapshot.mu.Lock()
		a.snapshot.shop = shop
		a.snapshot.abilityNames = abilityNames
		a.snapshot.abilityClass = abilityClass
		a.snapshot.shopErr = err
		a.snapshot.shopLoaded = true
		a.snapshot.mu.Unlock()
	}
	return shop, abilityNames, abilityClass, err
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
	a.snapshot = &staticSnapshot{}
	defer func() { a.snapshot = nil }()

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
	heroByID := make(map[int]apiHero, len(apiHeroes))
	for _, h := range apiHeroes {
		nameOf[h.ID] = h.Name
		heroByID[h.ID] = h
	}
	// class_name → id de habilidad: resuelve signature1..4 del kit.
	//
	// OJO: hay que usar el índice del catálogo COMPLETO. `shop` solo
	// contiene ítems comprables (upgrade/weapon); las habilidades se
	// filtran al construirlo, así que derivar el índice de `shop`
	// devuelve un mapa vacío y canonicalAbilityOrder cae siempre al
	// fallback por frecuencia. Ese era el fallo de raíz: los slots se
	// asignaban por popularidad en TODOS los héroes.
	abilityByClass := a.abilityClassIndex()
	// class_name → ítem completo: hace falta para leer
	// dependent_abilities (sub-habilidades de cambio de forma).
	itemsByClass := a.abilityItemsByClass()
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
		cb, err := a.loadHeroBuilds(hs, name, shop, abilityNames, patch, heroByID[hs.HeroID], abilityByClass, itemsByClass)
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
func (a *APIClient) loadHeroBuilds(hs apiHeroStat, name string, shop map[int]apiItem, abilityNames map[int]string, patch string, hero apiHero, abilityByClass map[string]int, itemsByClass map[string]apiItem) (*model.ChampionBuilds, error) {
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

	// Habilidades: mapear ids reales → espacios canónicos (signature1..4,
	// agrupando sub-habilidades de cambio de forma) y derivar topes por
	// espacio. Sin mapeo completo se cae al orden por uso, declarado.
	slots, primaries := abilitySlotMap(hero, abilityByClass, itemsByClass)
	seqs, rules, legend, err := buildSkillSequences(abilStats, abilityNames, slots, primaries)
	if err != nil {
		return nil, fmt.Errorf("héroe %d: %w", hs.HeroID, err)
	}
	cb.Skills = seqs
	cb.Rules = rules
	cb.AbilityNames = legend
	return cb, nil
}

// canonicalAbilityOrder devuelve los ids de habilidad del héroe en el
// orden canónico del juego (signature1..4), resolviendo cada class_name
// contra el catálogo de ítems. Devuelve nil si el dato no está
// disponible: el llamador cae entonces al orden por uso.
func canonicalAbilityOrder(hero apiHero, abilityByClass map[string]int) []int64 {
	var out []int64
	for i := 1; i <= 4; i++ {
		class := hero.Items[fmt.Sprintf("signature%d", i)]
		if class == "" {
			return nil // kit incompleto: no se inventa un orden
		}
		id, ok := abilityByClass[class]
		if !ok || id == 0 {
			return nil
		}
		out = append(out, int64(id))
	}
	return out
}

// abilitySlotMap construye el mapa id de habilidad → espacio canónico
// (1..4). Incluye las sub-habilidades: una habilidad que cambia de
// forma (Silver) comparte espacio con su principal, así que sus eventos
// de mejora se atribuyen a la misma tecla.
//
// La relación sale de los metadatos del juego (`dependent_abilities` +
// flag DisplayAsSubAbility), nunca de similitud de nombres ni de la
// frecuencia de uso.
func abilitySlotMap(hero apiHero, abilityByClass map[string]int, itemsByClass map[string]apiItem) (map[int64]int, map[int]int64) {
	out := map[int64]int{}
	primaries := map[int]int64{}
	for i := 1; i <= 4; i++ {
		class := hero.Items[fmt.Sprintf("signature%d", i)]
		id, ok := abilityByClass[class]
		if !ok || id == 0 {
			continue
		}
		out[int64(id)] = i
		primaries[i] = int64(id)
		// Sub-habilidades declaradas por el juego.
		it, ok := itemsByClass[class]
		if !ok {
			continue
		}
		for subClass, dep := range it.DependentAbilities {
			if !dep.IsSubAbility() {
				continue
			}
			subID, ok := abilityByClass[subClass]
			if !ok || subID == 0 {
				continue
			}
			out[int64(subID)] = i
		}
	}
	return out, primaries
}

// canonicalSlots coloca las habilidades observadas en el orden canónico
// del héroe. Devuelve nil si el orden canónico no cubre exactamente el
// conjunto observado (kit distinto, habilidad nueva o dato ausente): en
// ese caso el llamador usa el orden por uso y no se inventa nada.
func canonicalSlots(observed []int64, canonical []int64, use map[int64]int) []int64 {
	if len(canonical) == 0 || len(canonical) != len(observed) {
		return nil
	}
	seen := make(map[int64]bool, len(observed))
	for _, id := range observed {
		seen[id] = true
	}
	if len(seen) != len(observed) {
		return nil // ids duplicados en la observación: dato raro
	}
	out := make([]int64, 0, len(canonical))
	for _, id := range canonical {
		if !seen[id] {
			return nil // el canónico menciona algo que no se observó
		}
		out = append(out, id)
	}
	return out
}

// buildSkillSequences convierte las secuencias de ids de habilidad al
// alfabeto de slots y deriva los topes reales (máximo observado por
// slot).
//
// El slot de cada habilidad sale del orden canónico del héroe cuando se
// conoce (canonical); si no, se cae al orden por uso total descendente,
// que es el comportamiento histórico. El orden canónico es el correcto
// para la posición visual: en Solomon la habilidad más subida es la 3,
// y ordenar por uso invertía los slots (1↔4, 2↔3).
func buildSkillSequences(stats []apiAbilityStat, abilityNames map[int]string, slots map[int64]int, primaries map[int]int64) ([]model.SkillSequenceStats, model.SkillRules, map[rune]string, error) {
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

	// Camino canónico: todos los ids observados tienen espacio conocido
	// por los metadatos del juego. Las sub-habilidades (cambio de forma)
	// comparten espacio con su principal, así que no crean teclas
	// ficticias.
	slotOf := map[int64]rune{}
	legend := map[rune]string{}
	caps := map[rune]int{}
	canonicalOrder := len(slots) > 0
	var alphabet []rune
	if canonicalOrder {
		for _, id := range ids {
			if _, ok := slots[id]; !ok {
				canonicalOrder = false
				break
			}
		}
	}
	if canonicalOrder {
		// Nombres por espacio: manda la habilidad de la FIRMA canónica
		// (primaries), no la sub-habilidad. La sub-habilidad es la
		// forma transformada del mismo espacio y no debe robar el
		// nombre de la tecla.
		primary := primaries
		for slot := 1; slot <= 4; slot++ {
			r := rune('0' + slot)
			alphabet = append(alphabet, r)
			caps[r] = 0
			if id, ok := primary[slot]; ok {
				if n, ok := abilityNames[int(id)]; ok {
					legend[r] = n
				}
			}
		}
		for id, slot := range slots {
			slotOf[id] = rune('0' + slot)
		}
	} else {
		// Fallback declarado: orden por uso. NO es el orden del juego.
		sort.Slice(ids, func(i, j int) bool {
			if use[ids[i]] != use[ids[j]] {
				return use[ids[i]] > use[ids[j]] // más jugada primero
			}
			return ids[i] < ids[j] // desempate estable
		})
		for i, id := range ids {
			slot := rune('1' + i)
			slotOf[id] = slot
			if n, ok := abilityNames[int(id)]; ok {
				legend[slot] = n
			}
			caps[slot] = 0
			alphabet = append(alphabet, slot)
		}
	}

	rules := model.SkillRules{
		Alphabet:       alphabet,
		Length:         length,
		MaxPoints:      caps,
		CanonicalOrder: canonicalOrder,
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
		// Topes = máximo observado. Con sub-habilidades, dos ids
		// distintos comparten espacio: el tope es el máximo del espacio,
		// no la suma (no se cuenta dos veces el mismo evento).
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

// abilityItemsByClass devuelve class_name → ítem de habilidad completo,
// con sus dependent_abilities. Se construye desde el catálogo completo
// (no desde shop, que solo trae objetos comprables).
func (a *APIClient) abilityItemsByClass() map[string]apiItem {
	out := map[string]apiItem{}
	for _, it := range a.allItems() {
		if it.Type == "ability" && it.ClassName != "" {
			out[it.ClassName] = it
		}
	}
	return out
}

// allItems devuelve el catálogo completo de /v1/assets/items, memoizado
// por cliente. Es la fuente de las habilidades y de sus relaciones
// (dependent_abilities); shopItems solo conserva lo comprable.
func (a *APIClient) allItems() []apiItem {
	a.allItemsMu.Lock()
	defer a.allItemsMu.Unlock()
	if a.allItemsCache != nil {
		return a.allItemsCache
	}
	var items []apiItem
	if err := getJSON(a, "/v1/assets/items", "items.json", ttlAssets, &items); err != nil {
		return nil
	}
	a.allItemsCache = items
	return items
}
