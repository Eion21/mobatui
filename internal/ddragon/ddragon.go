// Package ddragon es un cliente de Data Dragon (los datos estáticos de
// Riot: campeones, ítems, runas) orientado a offline-first: cada
// petición pasa por la caché local con TTL; si no hay red y no hay
// caché, se sirve la copia de respaldo de testdata/ddragon. Así la app
// funciona sin conexión y no golpea las APIs más de lo necesario.
package ddragon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
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

// getURL resuelve una URL con caché → HTTP → fixture.
func (cl *Client) getURL(url string, ttl time.Duration) ([]byte, cache.State, error) {
	res, err := cl.c.Get(url, ttl, func() ([]byte, error) {
		resp, err := cl.http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ddragon: %s → HTTP %d", url, resp.StatusCode)
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
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Slots []struct {
		Runes []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
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
	v, err := cl.Version()
	if err != nil {
		return nil, nil, err
	}
	url := fmt.Sprintf("%s/cdn/%s/data/%s/runesReforged.json", cl.baseURL, v, cl.locale)
	body, _, err := cl.getURL(url, ttlStatic)
	if err != nil {
		return nil, nil, fmt.Errorf("ddragon: runas: %w", err)
	}
	var f runesFile
	if err := json.Unmarshal(body, &f); err != nil {
		return nil, nil, fmt.Errorf("ddragon: runesReforged.json inválido: %w", err)
	}
	runes = map[int]string{}
	trees = map[int]string{}
	for _, tree := range f {
		trees[tree.ID] = tree.Name
		for _, slot := range tree.Slots {
			for _, r := range slot.Runes {
				runes[r.ID] = r.Name
			}
		}
	}
	return runes, trees, nil
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
