package ui

// Helpers de prueba para las vistas de plan (LoL): construyen una
// sesión con datos de Lolalytics (fixtures de lolbuild) y el catálogo
// enriquecido de Data Dragon, sin red ni caché personal.

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// planFixture es un campeón del fixture de planes.
type planFixture struct {
	ChampionID int           `json:"champion_id"`
	Role       string        `json:"role"`
	Patch      string        `json:"patch"`
	Games      int           `json:"games"`
	WinRate    float64       `json:"win_rate"`
	ItemSlots  [][][]float64 `json:"item_slots"`
	Boots      [][]float64   `json:"boots"`
}

// loadPlanFixture carga el fixture de planes por clave.
func loadPlanFixture(t testing.TB, key string) planFixture {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/lolbuild/plans-16.21.json")
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]planFixture
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	f, ok := all[key]
	if !ok {
		t.Fatalf("falta el fixture %q", key)
	}
	return f
}

// loadItemCatalog carga el catálogo enriquecido desde el fixture.
func loadItemCatalog(t testing.TB) map[int]lolitems.Item {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/ddragon/items-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Data map[string]struct {
			Name string   `json:"name"`
			Tags []string `json:"tags"`
			Into []string `json:"into"`
			From []string `json:"from"`
			Gold struct {
				Total       int  `json:"total"`
				Purchasable bool `json:"purchasable"`
			} `json:"gold"`
			Maps          map[string]bool `json:"maps"`
			InStore       *bool           `json:"inStore"`
			Consumable    bool            `json:"consumable"`
			SpecialRecipe int             `json:"specialRecipe"`
			RequiredAlly  string          `json:"requiredAlly"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	out := make(map[int]lolitems.Item, len(f.Data))
	for key, v := range f.Data {
		var id int
		if json.Unmarshal([]byte(key), &id) != nil {
			continue
		}
		it := lolitems.Item{
			ID: id, Name: v.Name, Tags: v.Tags,
			Into: toIntList(v.Into), From: toIntList(v.From),
			Consumable: v.Consumable, SpecialRecipe: v.SpecialRecipe,
			RequiredAlly: v.RequiredAlly,
			Gold:         lolitems.Gold{Total: v.Gold.Total, Purchasable: v.Gold.Purchasable},
			Maps:         map[int]bool{},
		}
		it.InStore = true
		if v.InStore != nil {
			it.InStore = *v.InStore
		}
		for k, ok := range v.Maps {
			var n int
			if json.Unmarshal([]byte(k), &n) == nil {
				it.Maps[n] = ok
			}
		}
		for _, tag := range v.Tags {
			if tag == "Trinket" {
				it.Trinket = true
			}
		}
		out[id] = it
	}
	return out
}

func toIntList(list []string) []int {
	out := make([]int, 0, len(list))
	for _, s := range list {
		var n int
		if json.Unmarshal([]byte(s), &n) == nil {
			out = append(out, n)
		}
	}
	return out
}

// buildCleanFromPlan convierte el fixture en un CleanBuild con las
// tablas por posición que usa el planificador.
func buildCleanFromPlan(f planFixture) *model.CleanBuild {
	cb := &model.CleanBuild{
		Game:         model.GameLoL,
		ChampionID:   f.ChampionID,
		ChampionKey:  "Test",
		ChampionName: "Test",
		Role:         model.Role(f.Role),
		Patch:        f.Patch,
		Source:       "lolalytics.com",
		Total:        model.Stats{Games: f.Games},
		WinRate:      f.WinRate / 100,
	}
	for _, slot := range f.ItemSlots {
		var rows []model.ItemStats
		for _, r := range slot {
			if len(r) < 4 {
				continue
			}
			games := int(r[3])
			rows = append(rows, model.ItemStats{
				ID:      int(r[0]),
				Stats:   model.Stats{Games: games, Wins: int(float64(games) * r[1] / 100)},
				AvgBuyS: r[4] * 60,
			})
		}
		cb.ItemSlots = append(cb.ItemSlots, rows)
	}
	for _, r := range f.Boots {
		if len(r) < 4 {
			continue
		}
		games := int(r[3])
		cb.Boots = append(cb.Boots, model.RankedItem{
			ItemStats: model.ItemStats{
				ID:    int(r[0]),
				Stats: model.Stats{Games: games, Wins: int(float64(games) * r[1] / 100)},
			},
		})
	}
	return cb
}

// planSession construye una sesión de LoL con el catálogo enriquecido y
// el campeón indicado con datos de Lolalytics.
func planSession(t testing.TB, key, champName string) (*Session, *ChampEntry) {
	t.Helper()
	f := loadPlanFixture(t, key)
	cb := buildCleanFromPlan(f)
	cb.ChampionKey = champName
	cb.ChampionName = champName

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	c.Offline = true
	ddc := ddragon.New(c, "../../testdata/ddragon", "es_ES").
		WithHTTPClient(&http.Client{Transport: failTransport{}})

	s := &Session{
		Game:        model.GameLoL,
		ChampionArt: map[string]champArt{},
		ItemArt:     map[artKey]string{},
		ItemCatalog: loadItemCatalog(t),
		Version:     f.Patch,
		Provider:    ddc,
		Names:       Names{Heroes: map[string]staticdata.HeroInfo{}, Items: map[int]string{}},
	}
	// Nombres de ítem desde el catálogo (los iconos usan ItemArt).
	for id, it := range s.ItemCatalog {
		s.Names.Items[id] = it.Name
	}
	champ := &ChampEntry{
		Key:    champName,
		Name:   champName,
		Roles:  []model.Role{model.Role(f.Role)},
		ByRole: map[model.Role]*model.CleanBuild{model.Role(f.Role): cb},
	}
	s.Champions = []*ChampEntry{champ}
	_ = ddc
	return s, champ
}

// planDetailView construye la vista de detalle del fixture.
func planDetailView(t testing.TB, key, champName string, width, height int) detailView {
	t.Helper()
	s, champ := planSession(t, key, champName)
	d := newDetailView(champ, s)
	d.width, d.height = width, height
	d.vp.Width = max(24, width-4)
	d.vp.Height = height
	d.sync()
	return d
}

// gotoTab cambia a la pestaña con la tecla numérica.
func gotoTab(d detailView, key string) detailView {
	nd, _ := d.Update(keyMsg(key))
	nd.sync()
	return nd
}

// stripANSIStr quita las secuencias de color: las pruebas miden texto.
func stripANSIStr(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// ansiPattern cubre códigos SGR y también los marcadores de zona de
// bubblezone (\x1b[<n>z), que no ocupan columnas en pantalla.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[mz]`)

// toCleanBuildFixture devuelve un CleanBuild nuevo desde el fixture de
// Kai'Sa (para pruebas que mutan datos).
func toCleanBuildFixture(t *testing.T) *model.CleanBuild {
	t.Helper()
	return buildCleanFromPlan(loadPlanFixture(t, "kaisa"))
}

// contrastRatio devuelve el contraste WCAG entre dos colores.
func contrastRatio(a, b lipgloss.Color) float64 {
	la, lb := relLum(string(a)), relLum(string(b))
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// relLum calcula la luminancia relativa de un color #rrggbb.
func relLum(hex string) float64 {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		return 0
	}
	var rgb [3]float64
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseInt(h[i*2:i*2+2], 16, 32)
		if err != nil {
			return 0
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			rgb[i] = c / 12.92
		} else {
			rgb[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*rgb[0] + 0.7152*rgb[1] + 0.0722*rgb[2]
}

// waitZone espera a que bubblezone registre una zona. `Scan` es
// ASÍNCRONO: entrega la información por un canal a un worker en otra
// goroutine, así que una lectura inmediata puede no verla todavía.
func waitZone(id string) bool {
	for i := 0; i < 200; i++ {
		if z := zoneMgr.Get(id); !z.IsZero() {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}
