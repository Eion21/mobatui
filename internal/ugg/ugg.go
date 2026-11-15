// Package ugg adapta el JSON crudo de u.gg (o de sus fixtures en
// testdata/ugg) al modelo de dominio normalizado. Todo el conocimiento
// del formato de u.gg vive en este archivo: cuando se conecte la API
// real, cualquier ajuste de índices o campos se hace aquí y el resto
// de la aplicación no cambia.
package ugg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Eion21/mobatui/internal/model"
)

// fixtureFile es la forma en crudo de un resumen de campeón/posición de
// u.gg. Modela su estilo compacto: pares de stats [partidas, victorias]
// y posiciones indexadas 1=TOP..5=SUPPORT.
type fixtureFile struct {
	ChampionID   int    `json:"champion_id"`
	ChampionKey  string `json:"champion_key"`
	Patch        string `json:"patch"`
	Position     int    `json:"position"`
	Overview     [2]int `json:"overview"`
	Itemizations struct {
		Individual [][3]int `json:"individual"` // [id, partidas, victorias]
		Boots      [][3]int `json:"boots"`
	} `json:"itemizations"`
	Runes []struct {
		PrimaryTree   int    `json:"primary_tree"`
		SecondaryTree int    `json:"secondary_tree"`
		Keystone      int    `json:"keystone"`
		Primary       []int  `json:"primary"`
		Secondary     []int  `json:"secondary"`
		Shards        [3]int `json:"shards"`
		Stats         [2]int `json:"stats"`
	} `json:"runes"`
	Skills []any `json:"skills"` // cada fila: [secuencia, partidas, victorias]
}

// Parse decodifica el JSON crudo de u.gg a model.ChampionBuilds.
// Descarta filas con cero partidas y secuencias de habilidades
// matemáticamente inválidas (validación en model).
func Parse(data []byte) (*model.ChampionBuilds, error) {
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("ugg: json inválido: %w", err)
	}
	role, err := roleFromUgg(f.Position)
	if err != nil {
		return nil, err
	}
	cb := &model.ChampionBuilds{
		Game:        model.GameLoL,
		ChampionID:  f.ChampionID,
		ChampionKey: f.ChampionKey,
		Role:        role,
		Patch:       f.Patch,
		Source:      "ugg",
		Total:       model.Stats{Games: f.Overview[0], Wins: f.Overview[1]},
	}
	for _, it := range f.Itemizations.Individual {
		if it[1] <= 0 {
			continue
		}
		cb.Items = append(cb.Items, model.ItemStats{
			ID:    it[0],
			Stats: model.Stats{Games: it[1], Wins: it[2]},
		})
	}
	for _, b := range f.Itemizations.Boots {
		if b[1] <= 0 {
			continue
		}
		cb.Boots = append(cb.Boots, model.ItemStats{
			ID:    b[0],
			Stats: model.Stats{Games: b[1], Wins: b[2]},
		})
	}
	for _, r := range f.Runes {
		if r.Stats[0] <= 0 {
			continue
		}
		cb.RunePages = append(cb.RunePages, model.RunePageStats{
			PrimaryTree:   r.PrimaryTree,
			SecondaryTree: r.SecondaryTree,
			KeystoneID:    r.Keystone,
			PrimaryIDs:    r.Primary,
			SecondaryIDs:  r.Secondary,
			Shards:        r.Shards,
			Stats:         model.Stats{Games: r.Stats[0], Wins: r.Stats[1]},
		})
	}
	for i, raw := range f.Skills {
		row, ok := raw.([]any)
		if !ok || len(row) != 3 {
			return nil, fmt.Errorf("ugg: skills[%d]: fila con formato inesperado", i)
		}
		seqStr, ok := row[0].(string)
		if !ok {
			return nil, fmt.Errorf("ugg: skills[%d]: la secuencia no es texto", i)
		}
		games, err := statToInt(row[1])
		if err != nil {
			return nil, fmt.Errorf("ugg: skills[%d]: %w", i, err)
		}
		wins, err := statToInt(row[2])
		if err != nil {
			return nil, fmt.Errorf("ugg: skills[%d]: %w", i, err)
		}
		seq, err := model.ParseSequence(strings.ToUpper(seqStr), model.GameLoL.SkillRules())
		if err != nil {
			continue // fila corrupta: se descarta sin abortar el parseo
		}
		cb.Skills = append(cb.Skills, model.SkillSequenceStats{
			Sequence: seq,
			Stats:    model.Stats{Games: games, Wins: wins},
		})
	}
	if cb.ChampionKey == "" {
		return nil, fmt.Errorf("ugg: champion_key vacío")
	}
	return cb, nil
}

// roleFromUgg traduce el índice de posición de u.gg (1=TOP..5=SUPPORT).
func roleFromUgg(i int) (model.Role, error) {
	if i < 1 || i > len(model.AllRoles) {
		return "", fmt.Errorf("ugg: posición inválida %d", i)
	}
	return model.AllRoles[i-1], nil
}

// statToInt acepta el número tal cual venga en el JSON (float64 al
// decodificar en any) y lo trunca a entero.
func statToInt(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case string:
		i, err := strconv.Atoi(n)
		if err != nil {
			return 0, fmt.Errorf("stat no numérico: %q", n)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("stat de tipo inesperado %T", v)
	}
}

// LoadDir carga todos los fixtures {championId}-{posición}.json de un
// directorio (testdata/ugg para desarrollo; en un futuro, dumps de la
// API real guardados en data/raw).
func LoadDir(dir string) ([]*model.ChampionBuilds, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("ugg: leer directorio %s: %w", dir, err)
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
			return nil, fmt.Errorf("ugg: %s: %w", e.Name(), err)
		}
		out = append(out, cb)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("ugg: sin fixtures en %s", dir)
	}
	return out, nil
}
