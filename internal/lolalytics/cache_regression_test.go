package lolalytics

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Eion21/builds-tui/internal/clean"
)

// maxCachePages acota el barrido: cada página son ~30 ms de decodificado.
const maxCachePages = 60

// itemMeta es lo mínimo de Data Dragon para decidir si un ítem es final.
type itemMeta struct {
	name string
	into []string
	gold int
	tags []string
}

// loadItemMeta lee la copia cacheada de item.json de Data Dragon (la
// caché no se versiona: sin ella el test se salta).
func loadItemMeta(cacheDir string) map[int]itemMeta {
	entries, _ := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	for _, e := range entries {
		var ent struct {
			URL string `json:"url"`
		}
		raw, err := os.ReadFile(e)
		if err != nil || json.Unmarshal(raw, &ent) != nil {
			continue
		}
		if !strings.HasSuffix(ent.URL, "/item.json") {
			continue
		}
		body, err := readGzip(e + ".body.gz")
		if err != nil {
			continue
		}
		var payload struct {
			Data map[string]struct {
				Name string   `json:"name"`
				Into []string `json:"into"`
				Gold struct {
					Total int `json:"total"`
				} `json:"gold"`
				Tags []string `json:"tags"`
			} `json:"data"`
		}
		if json.Unmarshal(body, &payload) != nil {
			continue
		}
		out := make(map[int]itemMeta, len(payload.Data))
		for id, it := range payload.Data {
			n, err := strconv.Atoi(id)
			if err != nil {
				continue
			}
			out[n] = itemMeta{name: it.Name, into: it.Into, gold: it.Gold.Total, tags: it.Tags}
		}
		return out
	}
	return nil
}

func readGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// TestCacheRealCoreSinComponentes verifica sobre páginas reales de
// Lolalytics que ningún ítem del core es componente, consumible, ward
// ni barato. Es la regresión del sesgo: popularItem ya viene filtrado,
// y el pipeline no debe reintroducir lo que la fuente descartó.
func TestCacheRealCoreSinComponentes(t *testing.T) {
	cacheDir := "../../data/cache"
	meta := loadItemMeta(cacheDir)
	if meta == nil {
		t.Skip("sin item.json de Data Dragon en la caché")
	}
	entries, err := filepath.Glob(filepath.Join(cacheDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, e := range entries {
		if pages >= maxCachePages {
			break
		}
		var ent struct {
			URL string `json:"url"`
		}
		raw, err := os.ReadFile(e)
		if err != nil || json.Unmarshal(raw, &ent) != nil {
			continue
		}
		if !strings.Contains(ent.URL, "/build/") {
			continue
		}
		body, err := readGzip(e + ".body.gz")
		if err != nil {
			continue
		}
		q, err := parseQwik(body)
		if err != nil {
			continue
		}
		main := q.mainData()
		if main == nil {
			continue
		}
		items := q.statTable(main["popularItem"])
		if len(items) == 0 {
			continue
		}
		pages++
		cb := buildFrom(0, "cache", "top", "", &buildData{
			games: q.int(main["n"]),
			items: items,
		})
		b := clean.Build(cb, clean.Options{})
		for _, it := range b.CoreItems {
			m, ok := meta[it.ID]
			if !ok {
				continue // ítem fuera de Data Dragon: sin criterio
			}
			if len(m.into) > 0 {
				t.Errorf("%s: %d (%s) es un componente", ent.URL, it.ID, m.name)
			}
			goldPer := false
			for _, tag := range m.tags {
				switch tag {
				case "Consumable", "Trinket":
					t.Errorf("%s: %d (%s) tiene la etiqueta %s", ent.URL, it.ID, m.name, tag)
				case "GoldPer":
					goldPer = true
				}
			}
			// Los finales de soporte (Tejesueños, Trineo del solsticio…)
			// cuestan 400 de oro por diseño: GoldPer los distingue de un
			// objeto de inicio como el Anillo de Doran. Ojo: "Vision" no
			// sirve como señal de ward, porque esos finales también la
			// llevan.
			if m.gold > 0 && m.gold < 1500 && !goldPer {
				t.Errorf("%s: %d (%s) cuesta %d de oro", ent.URL, it.ID, m.name, m.gold)
			}
		}
	}
	if pages == 0 {
		t.Skip("sin páginas de build en la caché")
	}
	t.Logf("páginas verificadas: %d", pages)
}
