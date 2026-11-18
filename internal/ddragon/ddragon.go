// Package ddragon es un cliente de Data Dragon (los datos estáticos de
// Riot: campeones, ítems, runas) orientado a offline-first: cada
// petición pasa por la caché local con TTL; si no hay red y no hay
// caché, se sirve la copia de respaldo de testdata/ddragon. Así la app
// funciona sin conexión y no golpea las APIs más de lo necesario.
package ddragon

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/staticdata"
)

const (
	defaultBaseURL = "https://ddragon.leagueoflegends.com"
	// TTLs por tipo de recurso.
	ttlVersion = time.Hour
	ttlStatic  = 24 * time.Hour
	ttlIcon    = 720 * time.Hour // las URL llevan la versión: nunca cambian
)

// Client implementa staticdata.Provider para LoL.
var _ staticdata.Provider = (*Client)(nil)

// Shards nombres de los fragmentos de estadísticas (no vienen en
// runesReforged.json, se resuelven estáticamente).
var Shards = map[int]string{
	5001: "Vida",
	5002: "Armadura",
	5003: "Resistencia mágica",
	5005: "Velocidad de ataque",
	5007: "Aceleración de habilidades",
	5008: "Fuerza adaptativa",
}

// Client carga datos estáticos con caché y respaldo local.
type Client struct {
	c          *cache.Cache
	http       *http.Client
	fixtureDir string
	locale     string
	version    string
	baseURL    string // campo para apuntar a un servidor de pruebas

	// runeIcons cachea id → ruta relativa del icono de runa, resuelta
	// una sola vez desde runesReforged.json.
	runeIconsMu sync.Mutex
	runeIcons   map[int]string
}

func New(c *cache.Cache, fixtureDir, locale string) *Client {
	return &Client{
		c:          c,
		http:       &http.Client{Timeout: 15 * time.Second},
		fixtureDir: fixtureDir,
		locale:     locale,
		baseURL:    defaultBaseURL,
	}
}

// WithHTTPClient sustituye el cliente HTTP (inyección en tests).
func (cl *Client) WithHTTPClient(h *http.Client) *Client {
	cl.http = h
	return cl
}

// HTTPError es un fallo de Data Dragon con código de estado.
//
// Existe para que quien consume un icono pueda distinguir un recurso
// DEFINITIVAMENTE ausente (404/410: ese PNG no existe para este parche)
// de un fallo TRANSITORIO (5xx, timeout, DNS). Sin esta distinción, la
// UI no puede decidir entre "no volver a pedirlo" y "reintentar cuando
// vuelva la red", y acaba tratando igual ambos casos.
type HTTPError struct {
	URL        string
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("ddragon: %s → HTTP %d", e.URL, e.StatusCode)
}

// NotFound indica que el recurso no existe en el CDN para esta versión.
// Es un fallo permanente: reintentarlo daría el mismo 404.
func (e *HTTPError) NotFound() bool {
	return e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusGone
}

// IsNotFound informa de si el error (posiblemente envuelto) es un 404/410.
func IsNotFound(err error) bool {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.NotFound()
	}
	return false
}

// getURL resuelve una URL con caché → HTTP → fixture.
func (cl *Client) getURL(url string, ttl time.Duration) ([]byte, cache.State, error) {
	res, err := cl.c.Get(url, ttl, func() ([]byte, error) {
		resp, err := cl.http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, &HTTPError{URL: url, StatusCode: resp.StatusCode}
		}
		return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	})
	if err == nil {
		return res.Body, res.State, nil
	}
	// Respaldo: archivo local con el mismo nombre base que la URL.
	if data, ferr := os.ReadFile(filepath.Join(cl.fixtureDir, filepath.Base(url))); ferr == nil {
		return data, cache.StateFixed, nil
	}
	return nil, "", err
}

// Version devuelve la versión vigente de Data Dragon.
func (cl *Client) Version() (string, error) {
	if cl.version != "" {
		return cl.version, nil
	}
	body, _, err := cl.getURL(cl.baseURL+"/api/versions.json", ttlVersion)
	if err != nil {
		return "", fmt.Errorf("ddragon: versiones: %w", err)
	}
	var versions []string
	if err := json.Unmarshal(body, &versions); err != nil {
		return "", fmt.Errorf("ddragon: versions.json inválido: %w", err)
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("ddragon: lista de versiones vacía")
	}
	cl.version = versions[0]
	return cl.version, nil
}

// Champion es un campeón de Data Dragon.
type Champion struct {
	ID    string `json:"id"`
	Key   string `json:"key"`
	Name  string `json:"name"`
	Title string `json:"title"`
}

type championsFile struct {
	Data map[string]Champion `json:"data"`
}

// Champions devuelve los campeones indexados por clave ("Ahri").
func (cl *Client) Champions() (map[string]Champion, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/champion.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, fmt.Errorf("ddragon: campeones: %w", err)
	}
	var f championsFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("ddragon: champion.json inválido: %w", err)
	}
	return f.Data, nil
}

// Heroes adapta Champions a la interfaz staticdata.Provider.
func (cl *Client) Heroes() (map[string]staticdata.HeroInfo, error) {
	champs, err := cl.Champions()
	if err != nil {
		return nil, err
	}
	out := make(map[string]staticdata.HeroInfo, len(champs))
	for key, c := range champs {
		id, _ := strconv.Atoi(c.Key)
		out[key] = staticdata.HeroInfo{ID: id, Name: c.Name, Title: c.Title}
	}
	return out, nil
}

type itemsFile struct {
	Data map[string]struct {
		Name string `json:"name"`
	} `json:"data"`
}

// Items devuelve id numérico → nombre de ítem.
func (cl *Client) Items() (map[int]string, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/item.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, fmt.Errorf("ddragon: ítems: %w", err)
	}
	var f itemsFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("ddragon: item.json inválido: %w", err)
	}
	out := make(map[int]string, len(f.Data))
	for id, it := range f.Data {
		n, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		out[n] = it.Name
	}
	return out, nil
}

type runesFile []struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Icon es la ruta RELATIVA del PNG del árbol, p.ej.
	// "perk-images/Styles/7200_Domination.png".
	Icon  string `json:"icon"`
	Slots []struct {
		Runes []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			// Icon es la ruta RELATIVA del PNG de la runa, p.ej.
			// "perk-images/Styles/Domination/Electrocute/Electrocute.png".
			// NO sigue el esquema /img/rune/<id>.png.
			Icon string `json:"icon"`
		} `json:"runes"`
	} `json:"slots"`
}

// SummonerSpells devuelve id numérico → nombre de hechizo de
// invocador (summoner.json).
func (cl *Client) SummonerSpells() (map[int]string, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/summoner.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, fmt.Errorf("ddragon: hechizos de invocador: %w", err)
	}
	var f summonerFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("ddragon: summoner.json inválido: %w", err)
	}
	out := make(map[int]string, len(f.Data))
	for _, sp := range f.Data {
		id, err := strconv.Atoi(sp.Key)
		if err != nil {
			continue
		}
		out[id] = sp.Name
	}
	return out, nil
}

type summonerFile struct {
	Data map[string]struct {
		Key  string `json:"key"`
		Name string `json:"name"`
	} `json:"data"`
}

// Runes devuelve id → nombre de runa (aplanado, incluye keystones) y
// id → nombre de árbol.
func (cl *Client) Runes() (runes map[int]string, trees map[int]string, err error) {
	runes, trees, _, err = cl.RunesWithIcons()
	return runes, trees, err
}

// RunesWithIcons devuelve además el mapa id → ruta relativa del icono.
//
// El campo `icon` de runesReforged.json es una ruta relativa a
// /cdn/img/ (SIN versión): los iconos de runa no están versionados en
// el CDN, a diferencia de ítems y campeones.
func (cl *Client) RunesWithIcons() (runes map[int]string, trees map[int]string, icons map[int]string, err error) {
	v, err := cl.Version()
	if err != nil {
		return nil, nil, nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/runesReforged.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ddragon: runas: %w", err)
	}
	var f runesFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, nil, nil, fmt.Errorf("ddragon: runesReforged.json inválido: %w", err)
	}
	runes = map[int]string{}
	trees = map[int]string{}
	icons = map[int]string{}
	for _, tree := range f {
		trees[tree.ID] = tree.Name
		if tree.Icon != "" {
			icons[tree.ID] = tree.Icon
		}
		for _, slot := range tree.Slots {
			for _, r := range slot.Runes {
				runes[r.ID] = r.Name
				if r.Icon != "" {
					icons[r.ID] = r.Icon
				}
			}
		}
	}
	return runes, trees, icons, nil
}

// ChampionIcon descarga (con caché) el icono cuadrado de un campeón.
func (cl *Client) ChampionIcon(key string) ([]byte, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/img/champion/%s.png", cl.baseURL, v, key)
	body, _, err := cl.getURL(url, ttlIcon)
	return body, err
}

// RuneIcon descarga (con caché) el icono de una runa. Data Dragon
// publica los iconos de runa en /cdn/<ver>/img/rune/<id>.png; si el id
// no existe, devuelve error y la UI cae al nombre textual.
func (cl *Client) RuneIcon(id int) ([]byte, error) {
	if id <= 0 {
		return nil, fmt.Errorf("ddragon: id de runa inválido: %d", id)
	}
	// La ruta sale del campo `icon` de runesReforged.json. NO es
	// /img/rune/<id>.png (esa ruta devuelve 403) ni lleva versión: los
	// iconos de runa no están versionados en el CDN.
	icon, err := cl.runeIconPath(id)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/img/%s", cl.baseURL, icon)
	body, _, err := cl.getURL(url, ttlIcon)
	return body, err
}

// runeIconPath resuelve id → ruta relativa del icono, validándola.
//
// Validar la ruta evita construir URLs con "..", esquemas o barras
// iniciales si el catálogo viniera manipulado.
func (cl *Client) runeIconPath(id int) (string, error) {
	cl.runeIconsMu.Lock()
	icons := cl.runeIcons
	cl.runeIconsMu.Unlock()
	if icons == nil {
		_, _, fetched, err := cl.RunesWithIcons()
		if err != nil {
			return "", err
		}
		cl.runeIconsMu.Lock()
		cl.runeIcons = fetched
		icons = fetched
		cl.runeIconsMu.Unlock()
	}
	icon, ok := icons[id]
	if !ok || icon == "" {
		return "", fmt.Errorf("ddragon: runa %d sin icono en el catálogo", id)
	}
	if err := validateRelPath(icon); err != nil {
		return "", fmt.Errorf("ddragon: ruta de icono inválida para la runa %d: %w", id, err)
	}
	return icon, nil
}

// validateRelPath comprueba que la ruta sea relativa y segura.
func validateRelPath(p string) error {
	if p == "" {
		return fmt.Errorf("ruta vacía")
	}
	if strings.HasPrefix(p, "/") || strings.Contains(p, "://") {
		return fmt.Errorf("debe ser relativa")
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("no puede contener ..")
	}
	if !strings.HasSuffix(p, ".png") {
		return fmt.Errorf("debe terminar en .png")
	}
	return nil
}

// ItemIcon descarga (con caché) el icono de un ítem.
func (cl *Client) ItemIcon(id int) ([]byte, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/img/item/%d.png", cl.baseURL, v, id)
	body, _, err := cl.getURL(url, ttlIcon)
	return body, err
}

// ─── catálogo enriquecido (fase 3) ─────────────────────────────────
//
// Items() sigue devolviendo id → nombre para no romper a sus
// consumidores. ItemCatalog() expone los metadatos completos que
// necesita la clasificación por función, reutilizando EXACTAMENTE la
// misma URL, caché y respaldo local: no hay peticiones nuevas.

// itemJSON es la forma de un objeto en item.json. Solo se decodifican
// los campos que usamos; el resto se ignora.
type itemJSON struct {
	Name          string   `json:"name"`
	Plaintext     string   `json:"plaintext"`
	Tags          []string `json:"tags"`
	Into          []string `json:"into"`
	From          []string `json:"from"`
	InStore       *bool    `json:"inStore"`
	Depth         int      `json:"depth"`
	Consumable    bool     `json:"consumable"`
	SpecialRecipe int      `json:"specialRecipe"`
	RequiredAlly  string   `json:"requiredAlly"`
	Gold          struct {
		Base        int  `json:"base"`
		Total       int  `json:"total"`
		Sell        int  `json:"sell"`
		Purchasable bool `json:"purchasable"`
	} `json:"gold"`
	Maps  map[string]bool `json:"maps"`
	Stats struct {
		FlatArmor          float64 `json:"FlatArmorMod"`
		FlatSpellBlock     float64 `json:"FlatSpellBlockMod"`
		FlatHealth         float64 `json:"FlatHPPoolMod"`
		FlatMagicDamage    float64 `json:"FlatMagicDamageMod"`
		FlatPhysicalDamage float64 `json:"FlatPhysicalDamageMod"`
		FlatAttackSpeed    float64 `json:"FlatAttackSpeedMod"`
		FlatMoveSpeed      float64 `json:"FlatMovementSpeedMod"`
		FlatMP             float64 `json:"FlatMPPoolMod"`
		FlatCritChance     float64 `json:"FlatCritChanceMod"`
		PercentAttackSpeed float64 `json:"PercentAttackSpeedMod"`
		PercentMoveSpeed   float64 `json:"PercentMovementSpeedMod"`
		FlatMPRegen        float64 `json:"FlatMPRegenMod"`
		FlatHPRegen        float64 `json:"FlatHPRegenMod"`
		FlatMagicPen       float64 `json:"FlatMagicPenetrationMod"`
		FlatArmorPen       float64 `json:"FlatArmorPenetrationMod"`
		PercentArmorPen    float64 `json:"PercentArmorPenetrationMod"`
		PercentMagicPen    float64 `json:"PercentMagicPenetrationMod"`
		FlatLifeSteal      float64 `json:"FlatLifeStealMod"`
		FlatSpellVamp      float64 `json:"FlatSpellVampMod"`
		FlatCooldownRed    float64 `json:"FlatCooldownMod"`
		FlatAbilityPower   float64 `json:"FlatAbilityPowerMod"`
	} `json:"stats"`
}

// catalogFile envuelve item.json.
type catalogFile struct {
	Data map[string]itemJSON `json:"data"`
}

// ItemCatalog devuelve el catálogo completo de objetos con metadatos.
//
// Usa la misma URL que Items(), así que comparte caché y respaldo: si
// Items() ya la descargó, esto no hace ninguna petición nueva.
func (cl *Client) ItemCatalog() (map[int]lolitems.Item, error) {
	v, err := cl.Version()
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/item.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, fmt.Errorf("ddragon: catálogo de ítems: %w", err)
	}
	var f catalogFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, fmt.Errorf("ddragon: item.json inválido: %w", err)
	}
	out := make(map[int]lolitems.Item, len(f.Data))
	for key, raw := range f.Data {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		it := lolitems.Item{
			ID:            id,
			Name:          raw.Name,
			Plain:         stripTags(raw.Plaintext),
			Tags:          raw.Tags,
			Into:          parseIDs(raw.Into),
			From:          parseIDs(raw.From),
			Depth:         raw.Depth,
			Consumable:    raw.Consumable,
			Trinket:       containsTag(raw.Tags, "Trinket"),
			SpecialRecipe: raw.SpecialRecipe,
			RequiredAlly:  raw.RequiredAlly,
			Gold: lolitems.Gold{
				Base:        raw.Gold.Base,
				Total:       raw.Gold.Total,
				Sell:        raw.Gold.Sell,
				Purchasable: raw.Gold.Purchasable,
			},
			Maps: map[int]bool{},
		}
		// inStore: si el parche no lo trae, se asume true (objetos
		// anteriores al campo). Un false explícito manda.
		it.InStore = true
		if raw.InStore != nil {
			it.InStore = *raw.InStore
		}
		for k, ok := range raw.Maps {
			if n, err := strconv.Atoi(k); err == nil {
				it.Maps[n] = ok
			}
		}
		it.Stats = lolitems.Stats{
			FlatArmor:          raw.Stats.FlatArmor,
			FlatSpellBlock:     raw.Stats.FlatSpellBlock,
			FlatHealth:         raw.Stats.FlatHealth,
			FlatMagicDamage:    raw.Stats.FlatMagicDamage,
			FlatPhysicalDamage: raw.Stats.FlatPhysicalDamage,
			FlatAttackSpeed:    raw.Stats.FlatAttackSpeed,
			FlatMoveSpeed:      raw.Stats.FlatMoveSpeed,
			FlatMP:             raw.Stats.FlatMP,
			FlatCritChance:     raw.Stats.FlatCritChance,
			PercentAttackSpeed: raw.Stats.PercentAttackSpeed,
			PercentMoveSpeed:   raw.Stats.PercentMoveSpeed,
			FlatMPRegen:        raw.Stats.FlatMPRegen,
			FlatHPRegen:        raw.Stats.FlatHPRegen,
			FlatMagicPen:       raw.Stats.FlatMagicPen,
			FlatArmorPen:       raw.Stats.FlatArmorPen,
			PercentArmorPen:    raw.Stats.PercentArmorPen,
			PercentMagicPen:    raw.Stats.PercentMagicPen,
			FlatLifeSteal:      raw.Stats.FlatLifeSteal,
			FlatSpellVamp:      raw.Stats.FlatSpellVamp,
			FlatCooldownRed:    raw.Stats.FlatCooldownRed,
			FlatAbilityPower:   raw.Stats.FlatAbilityPower,
		}
		out[id] = it
	}
	return out, nil
}

// parseIDs convierte las claves de into/from a números.
func parseIDs(list []string) []int {
	out := make([]int, 0, len(list))
	for _, s := range list {
		if n, err := strconv.Atoi(s); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// stripTags quita el marcado simple de las descripciones de Data Dragon.
func stripTags(s string) string {
	var b []byte
	inTag := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b = append(b, s[i])
			}
		}
	}
	return string(b)
}
