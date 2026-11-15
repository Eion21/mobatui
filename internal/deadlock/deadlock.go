// Package deadlock adapta los datos de builds de Deadlock (modelados
// sobre el formato de los trackers comunitarios, con fixtures en
// testdata/deadlock) al modelo de dominio, y provee los datos
// estáticos del juego (héroes, ítems, iconos) desde copia local,
// lista para apuntar a un CDN comunitario cuando haya fuente estable.
//
// Todo el conocimiento del formato vive aquí: al conectar una fuente
// real solo se ajusta este archivo.
package deadlock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// buildFile es la forma en cruda de un resumen de héroe.
type buildFile struct {
	HeroID   int      `json:"hero_id"`
	HeroKey  string   `json:"hero_key"`
	Patch    string   `json:"patch"`
	Overview [2]int   `json:"overview"`
	Items    [][3]int `json:"items"` // [id, partidas, victorias]
	Skills   [][3]any `json:"skills"`
}

// skillsRow decodifica una fila de skills [secuencia, partidas, victorias].
func skillsRow(row [3]any) (seq string, st model.Stats, err error) {
	s, ok := row[0].(string)
	if !ok {
		return "", st, fmt.Errorf("la secuencia no es texto")
	}
	num := func(v any, what string) (int, error) {
		switch n := v.(type) {
		case float64:
			return int(n), nil
		default:
			return 0, fmt.Errorf("%s de tipo inesperado %T", what, v)
		}
	}
	games, err := num(row[1], "partidas")
	if err != nil {
		return "", st, err
	}
	wins, err := num(row[2], "victorias")
	if err != nil {
		return "", st, err
	}
	return s, model.Stats{Games: games, Wins: wins}, nil
}

// Parse decodifica el JSON crudo de un héroe a model.ChampionBuilds.
// Las secuencias inválidas se descartan; las filas sin partidas, igual.
func Parse(data []byte) (*model.ChampionBuilds, error) {
	var f buildFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("deadlock: json inválido: %w", err)
	}
	if f.HeroKey == "" {
		return nil, fmt.Errorf("deadlock: hero_key vacío")
	}
	cb := &model.ChampionBuilds{
		Game:        model.GameDeadlock,
		ChampionID:  f.HeroID,
		ChampionKey: f.HeroKey,
		Role:        model.RoleAll,
		Patch:       f.Patch,
		Source:      "tracklock (fixtures)",
		Total:       model.Stats{Games: f.Overview[0], Wins: f.Overview[1]},
	}
	for _, it := range f.Items {
		if it[1] <= 0 {
			continue
		}
		cb.Items = append(cb.Items, model.ItemStats{
			ID:    it[0],
			Stats: model.Stats{Games: it[1], Wins: it[2]},
		})
	}
	rules := model.GameDeadlock.SkillRules()
	for i, raw := range f.Skills {
		seqStr, st, err := skillsRow(raw)
		if err != nil {
			return nil, fmt.Errorf("deadlock: skills[%d]: %w", i, err)
		}
		seq, err := model.ParseSequence(strings.TrimSpace(seqStr), rules)
		if err != nil {
			continue // fila corrupta: se descarta sin abortar el parseo
		}
		cb.Skills = append(cb.Skills, model.SkillSequenceStats{Sequence: seq, Stats: st})
	}
	return cb, nil
}

// LoadDir carga todos los fixtures {heroId}.json de un directorio
// (testdata/deadlock/builds para desarrollo; dumps reales en el
// futuro).
func LoadDir(dir string) ([]*model.ChampionBuilds, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("deadlock: leer directorio %s: %w", dir, err)
	}
	var out []*model.ChampionBuilds
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		cb, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("deadlock: %s: %w", e.Name(), err)
		}
		out = append(out, cb)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("deadlock: sin fixtures en %s", dir)
	}
	return out, nil
}

// ─── proveedor de datos estáticos ──────────────────────────────────

// Client sirve los datos estáticos de Deadlock desde un directorio
// local (heroes.json, items.json, version.json e iconos PNG).
type Client struct {
	dir string
}

func New(dir string) *Client { return &Client{dir: dir} }

type heroesFile struct {
	Data map[string]struct {
		Name  string `json:"name"`
		Title string `json:"title"`
	} `json:"data"`
}

type itemsFile struct {
	Data map[string]struct {
		Name string `json:"name"`
	} `json:"data"`
}

type versionFile struct {
	Patch string `json:"patch"`
}

func (c *Client) readJSON(name string, v any) error {
	data, err := os.ReadFile(filepath.Join(c.dir, name))
	if err != nil {
		return fmt.Errorf("deadlock: leer %s: %w", name, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("deadlock: %s inválido: %w", name, err)
	}
	return nil
}

// Version devuelve el patch de version.json.
func (c *Client) Version() (string, error) {
	var v versionFile
	if err := c.readJSON("version.json", &v); err != nil {
		return "", err
	}
	return v.Patch, nil
}

// Heroes indexados por clave ("Abrams").
func (c *Client) Heroes() (map[string]staticdata.HeroInfo, error) {
	var f heroesFile
	if err := c.readJSON("heroes.json", &f); err != nil {
		return nil, err
	}
	out := make(map[string]staticdata.HeroInfo, len(f.Data))
	for key, h := range f.Data {
		out[key] = staticdata.HeroInfo{Name: h.Name, Title: h.Title}
	}
	return out, nil
}

// Items id → nombre.
func (c *Client) Items() (map[int]string, error) {
	var f itemsFile
	if err := c.readJSON("items.json", &f); err != nil {
		return nil, err
	}
	out := make(map[int]string, len(f.Data))
	for id, it := range f.Data {
		var n int
		if _, err := fmt.Sscanf(id, "%d", &n); err != nil {
			continue
		}
		out[n] = it.Name
	}
	return out, nil
}

// Runes: Deadlock no tiene runas.
func (c *Client) Runes() (map[int]string, map[int]string, error) {
	return map[int]string{}, map[int]string{}, nil
}

// ChampionIcon lee el PNG del héroe ({clave}.png).
func (c *Client) ChampionIcon(key string) ([]byte, error) {
	return os.ReadFile(filepath.Join(c.dir, key+".png"))
}

// ItemIcon lee el PNG del ítem ({id}.png).
func (c *Client) ItemIcon(id int) ([]byte, error) {
	return os.ReadFile(filepath.Join(c.dir, fmt.Sprintf("%d.png", id)))
}
