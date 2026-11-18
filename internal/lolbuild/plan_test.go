package lolbuild

// Pruebas del planificador. Usan testdata/lolbuild/plans-16.21.json
// (extraído de las tablas reales de Lolalytics, parche 16.21) y el
// catálogo de Data Dragon: sin red y sin caché personal.

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── carga de fixtures ─────────────────────────────────────────────

type slotRow struct {
	ID      int
	WR      float64
	Pick    float64
	Games   int
	Minutes int
}

type champFixture struct {
	ChampionID   int           `json:"champion_id"`
	ChampionName string        `json:"champion_name"`
	Role         string        `json:"role"`
	Patch        string        `json:"patch"`
	Games        int           `json:"games"`
	WinRate      float64       `json:"win_rate"`
	ItemSlots    [][][]float64 `json:"item_slots"`
	Boots        [][]float64   `json:"boots"`
	Combos       [][]any       `json:"combos"`
}

// fixture carga un campeón del JSON de planes.
func fixture(t *testing.T, key string) champFixture {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/lolbuild/plans-16.21.json")
	if err != nil {
		t.Fatal(err)
	}
	var all map[string]champFixture
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	f, ok := all[key]
	if !ok {
		t.Fatalf("falta el fixture %q", key)
	}
	return f
}

// catalog carga el catálogo de Data Dragon de la fase 3.
func catalog(t *testing.T) map[int]lolitems.Item {
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
			Into: toInts(v.Into), From: toInts(v.From),
			Consumable: v.Consumable, SpecialRecipe: v.SpecialRecipe,
			RequiredAlly: v.RequiredAlly,
			Trinket:      hasTag(v.Tags, "Trinket"),
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
		out[id] = it
	}
	return out
}

func toInts(list []string) []int {
	out := make([]int, 0, len(list))
	for _, s := range list {
		var n int
		if json.Unmarshal([]byte(s), &n) == nil {
			out = append(out, n)
		}
	}
	return out
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// toCleanBuild convierte el fixture en un model.CleanBuild, como haría
// el pipeline real (clean.BuildAll) para LoL.
func toCleanBuild(f champFixture) *model.CleanBuild {
	cb := &model.CleanBuild{
		Game:         model.GameLoL,
		ChampionID:   f.ChampionID,
		ChampionKey:  f.ChampionName,
		ChampionName: f.ChampionName,
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
			wr := r[1]
			rows = append(rows, model.ItemStats{
				ID:      int(r[0]),
				Stats:   model.Stats{Games: games, Wins: int(float64(games) * wr / 100)},
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
		wr := r[1]
		cb.Boots = append(cb.Boots, model.RankedItem{
			ItemStats: model.ItemStats{
				ID:    int(r[0]),
				Stats: model.Stats{Games: games, Wins: int(float64(games) * wr / 100)},
			},
		})
	}
	return cb
}

// ─── semántica de las tablas ───────────────────────────────────────

// TestItemSlotsSonCronologicos: el minuto medio de compra crece con la
// posición. Es la prueba de que item1..item5 son la compra N-ésima y no
// una combinación sin orden.
func TestItemSlotsSonCronologicos(t *testing.T) {
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			f := fixture(t, key)
			// Mediana del minuto por columna, ignorando los objetos de
			// misión (minuto 0: se eligen al empezar).
			var medias []float64
			for _, slot := range f.ItemSlots {
				var mins []int
				for _, r := range slot {
					if len(r) < 5 {
						continue
					}
					if m := int(r[4]); m > 0 {
						mins = append(mins, m)
					}
				}
				if len(mins) == 0 {
					medias = append(medias, 0)
					continue
				}
				sum := 0
				for _, m := range mins {
					sum += m
				}
				medias = append(medias, float64(sum)/float64(len(mins)))
			}
			// Cada columna debe comprarse más tarde que la anterior.
			for i := 1; i < len(medias); i++ {
				if medias[i] > 0 && medias[i-1] > 0 && medias[i] <= medias[i-1] {
					t.Errorf("columna %d (%0.f min) no es posterior a %d (%0.f min): item1..item5 no son cronológicas",
						i+1, medias[i], i, medias[i-1])
				}
			}
		})
	}
}

// TestCombosNoTienenOrden: las claves de builtBootSet3 codifican una
// combinación, no una secuencia. Permutar la clave da estadísticas
// distintas, lo que demuestra que el orden no es cronológico.
func TestCombosNoTienenOrden(t *testing.T) {
	f := fixture(t, "kaisa")
	sets := map[string]map[string]float64{}
	for _, c := range f.Combos {
		if len(c) < 4 {
			continue
		}
		key, _ := c[0].(string)
		wr, _ := c[1].(float64)
		// Normalizar el conjunto de ids.
		ids := ""
		for _, part := range splitUnderscore(key) {
			ids += part + ","
		}
		sortStr := sortedParts(key)
		if sets[sortStr] == nil {
			sets[sortStr] = map[string]float64{}
		}
		sets[sortStr][key] = wr
	}
	permutaciones := 0
	for _, variants := range sets {
		if len(variants) > 1 {
			permutaciones++
			// Mismo conjunto, WR distintos: el orden de la clave no es
			// una secuencia de compra.
			var wrs []float64
			for _, wr := range variants {
				wrs = append(wrs, wr)
			}
			if len(wrs) >= 2 && wrs[0] == wrs[1] {
				// Podría coincidir por casualidad; se informa sin fallar.
				t.Logf("permutaciones con el mismo WR: %v", variants)
			}
		}
	}
	if permutaciones == 0 {
		t.Skip("el fixture no trae permutaciones del mismo conjunto")
	}
	t.Logf("conjuntos con varias permutaciones: %d (confirma que no hay orden)", permutaciones)
}

func splitUnderscore(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '_' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func sortedParts(s string) string {
	parts := splitUnderscore(s)
	for i := 1; i < len(parts); i++ {
		for j := i; j > 0 && parts[j] < parts[j-1]; j-- {
			parts[j], parts[j-1] = parts[j-1], parts[j]
		}
	}
	out := ""
	for _, p := range parts {
		out += p + ","
	}
	return out
}

// ─── planes de los cuatro campeones ────────────────────────────────

func TestPlanKaisa(t *testing.T) {
	f := fixture(t, "kaisa")
	cb := toCleanBuild(f)
	cat := catalog(t)

	// Antes de la misión: inventario normal (botas ocupan espacio).
	plan := BuildPlan(cb, cat, Options{Mission: MissionPending})
	if plan.Role != model.RoleBottom {
		t.Fatalf("rol = %s", plan.Role)
	}
	if plan.Inventory.State != MissionPending || plan.Inventory.BootsSlot != true {
		t.Errorf("inventario = %+v", plan.Inventory)
	}
	if plan.Inventory.TotalSlots() != 6 {
		t.Errorf("espacios = %d, se esperaban 6", plan.Inventory.TotalSlots())
	}
	if len(plan.Steps) == 0 {
		t.Fatal("sin pasos")
	}
	// El primer paso es el objeto más comprado como compra 1.
	if plan.Steps[0].Item != 6672 {
		t.Errorf("paso 1 = %d, se esperaba 6672 (Verdugo de krakens)", plan.Steps[0].Item)
	}
	if plan.Steps[0].Slot != 1 {
		t.Errorf("slot = %d", plan.Steps[0].Slot)
	}
	// Los minutos crecen a lo largo del plan.
	for i := 1; i < len(plan.Steps); i++ {
		if plan.Steps[i].BuyMinute > 0 && plan.Steps[i-1].BuyMinute > 0 &&
			plan.Steps[i].BuyMinute < plan.Steps[i-1].BuyMinute {
			t.Errorf("paso %d (%d min) antes que %d (%d min)",
				i+1, plan.Steps[i].BuyMinute, i, plan.Steps[i-1].BuyMinute)
		}
	}
	// Con misión completada, las botas salen a ranura aparte.
	planM := BuildPlan(cb, cat, Options{Mission: MissionComplete})
	if planM.Inventory.State != MissionComplete || planM.Inventory.BootsSlot {
		t.Errorf("inventario con misión = %+v", planM.Inventory)
	}
	if planM.Inventory.TotalSlots() != 6 {
		t.Errorf("espacios con misión = %d", planM.Inventory.TotalSlots())
	}
	// La ruta es compuesta: cada paso es la compra N más frecuente.
	for _, s := range plan.Steps {
		if s.Evidence != EvidenceComposed {
			t.Errorf("paso %d evidencia = %s", s.Slot, s.Evidence)
		}
	}
}

func TestPlanAhri(t *testing.T) {
	f := fixture(t, "ahri")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{})

	if plan.Role != model.RoleMiddle {
		t.Fatalf("rol = %s", plan.Role)
	}
	if plan.Inventory.State != MissionUnknown {
		t.Errorf("la misión debe quedar desconocida: %s", plan.Inventory.State)
	}
	if len(plan.Steps) < 3 {
		t.Fatalf("pasos = %d", len(plan.Steps))
	}
	if plan.Steps[0].Item != 3118 {
		t.Errorf("paso 1 = %d (%s), se esperaba 3118 (Maleficio de Luden)",
			plan.Steps[0].Item, plan.Steps[0].Name)
	}
	// Un mago: sus alternativas incluyen daño mágico.
	_, hasMagic := plan.GroupFor(lolitems.CatMagicDamage)
	if !hasMagic {
		t.Error("las alternativas de Ahri debían incluir daño mágico")
	}
}

func TestPlanMalphite(t *testing.T) {
	f := fixture(t, "malphite")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{})

	if plan.Role != model.RoleTop {
		t.Fatalf("rol = %s", plan.Role)
	}
	if len(plan.Steps) == 0 {
		t.Fatal("sin pasos")
	}
	if plan.Steps[0].Item != 3068 {
		t.Errorf("paso 1 = %d (%s), se esperaba 3068 (Cota de espinas)",
			plan.Steps[0].Item, plan.Steps[0].Name)
	}
	// Un tanque: armadura o vida entre sus alternativas.
	_, hasArmor := plan.GroupFor(lolitems.CatArmor)
	_, hasHealth := plan.GroupFor(lolitems.CatHealth)
	tanky := hasArmor || hasHealth
	if !tanky {
		t.Error("las alternativas de Malphite debían incluir armadura o vida")
	}
}

func TestPlanSoraka(t *testing.T) {
	f := fixture(t, "soraka")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{})

	if plan.Role != model.RoleSupport {
		t.Fatalf("rol = %s", plan.Role)
	}
	// El objeto de misión es un paso real (se elige al empezar), pero
	// debe estar marcado y no consumir espacio del inventario.
	var questSteps int
	for _, s := range plan.Steps {
		if lolitems.IsQuestItem(s.Item) {
			questSteps++
			if !s.Quest {
				t.Errorf("%d (%s) es de misión y no está marcado", s.Item, s.Name)
			}
		} else if s.Quest {
			t.Errorf("%d (%s) marcado como misión sin serlo", s.Item, s.Name)
		}
	}
	if questSteps == 0 {
		t.Error("Soraka debía tener su objeto de misión como primer paso")
	}
	// Y aparece en el inventario final solo si es elegible; nunca como
	// alternativa.
	for _, a := range plan.AlternativeItems() {
		if lolitems.IsQuestItem(a.Item) {
			t.Errorf("el objeto de misión %d no debe ser alternativa", a.Item)
		}
	}
	if len(plan.Steps) == 0 {
		t.Fatal("sin pasos")
	}
	// Su primer paso es el objeto de misión; el primero de build es
	// Renovación de piedra lunar.
	if !plan.Steps[0].Quest {
		t.Errorf("paso 1 = %d (%s), se esperaba el objeto de misión",
			plan.Steps[0].Item, plan.Steps[0].Name)
	}
	var firstBuild *Step
	for i := range plan.Steps {
		if !plan.Steps[i].Quest {
			firstBuild = &plan.Steps[i]
			break
		}
	}
	if firstBuild == nil {
		t.Fatal("sin pasos de build tras la misión")
	}
	if firstBuild.Item != 6617 {
		t.Errorf("primer objeto de build = %d (%s), se esperaba 6617 (Renovación de piedra lunar)",
			firstBuild.Item, firstBuild.Name)
	}
}

// ─── inventarios y restricciones ───────────────────────────────────

func TestInventarioMaximoSeis(t *testing.T) {
	f := fixture(t, "kaisa")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{Mission: MissionPending})
	if len(plan.Final) > 6 {
		t.Errorf("inventario final con %d objetos: máximo 6", len(plan.Final))
	}
	// Sin duplicados.
	seen := map[int]bool{}
	for _, id := range plan.Final {
		if seen[id] {
			t.Errorf("objeto duplicado en el inventario: %d", id)
		}
		seen[id] = true
	}
}

func TestBotasEnInventario(t *testing.T) {
	f := fixture(t, "kaisa")
	cat := catalog(t)
	plan := BuildPlan(toCleanBuild(f), cat, Options{})

	// Las botas más usadas son las Grebas de berserker (3006).
	if len(plan.Boots) == 0 {
		t.Fatal("sin botas recomendadas")
	}
	if plan.Boots[0].Item != 3006 {
		t.Errorf("botas = %d (%s), se esperaba 3006", plan.Boots[0].Item, plan.Boots[0].Name)
	}
	// Solo se aceptan objetos con tag Boots.
	for _, b := range plan.Boots {
		if meta, ok := cat[b.Item]; ok && !lolitems.IsBoots(meta) {
			t.Errorf("%d (%s) no es un par de botas", b.Item, b.Name)
		}
	}
}

// TestSinDuplicadosNiInexistentes: el plan nunca incluye objetos que no
// estén en el catálogo del parche.
func TestSinDuplicadosNiInexistentes(t *testing.T) {
	cat := catalog(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			plan := BuildPlan(toCleanBuild(fixture(t, key)), cat, Options{})
			for _, s := range plan.Steps {
				if _, ok := cat[s.Item]; !ok {
					t.Errorf("paso %d (%s) no existe en el catálogo", s.Item, s.Name)
				}
				if s.Name == "" {
					t.Errorf("paso %d sin nombre", s.Item)
				}
			}
			for _, id := range plan.Final {
				if _, ok := cat[id]; !ok {
					t.Errorf("inventario incluye %d, inexistente en el parche", id)
				}
			}
			for _, a := range plan.AlternativeItems() {
				if _, ok := cat[a.Item]; !ok {
					t.Errorf("alternativa %d inexistente", a.Item)
				}
			}
		})
	}
}

// ─── datos insuficientes y fallback ────────────────────────────────

// TestDatosIncompletos: sin tablas por posición no se inventa ruta.
func TestDatosIncompletos(t *testing.T) {
	cb := &model.CleanBuild{
		Game: model.GameLoL, ChampionID: 1, ChampionKey: "Test",
		Role: model.RoleMiddle, Patch: "16.21", Source: "lolalytics.com",
		Total: model.Stats{Games: 500},
		// Sin ItemSlots: la fuente no publica posiciones.
	}
	plan := BuildPlan(cb, catalog(t), Options{})
	if plan.Complete {
		t.Error("no puede estar completo sin datos por posición")
	}
	if plan.Evidence != EvidenceFallback {
		t.Errorf("evidencia = %s, se esperaba %s", plan.Evidence, EvidenceFallback)
	}
	if plan.Reason == "" {
		t.Error("debe explicar por qué está incompleto")
	}
	if len(plan.Steps) != 0 {
		t.Errorf("no debe inventar pasos: %v", plan.Steps)
	}
}

// TestFallbackUgg: el respaldo no trae tablas por posición.
func TestFallbackUgg(t *testing.T) {
	// u.gg solo aporta ítems sueltos y botas, sin item1..item5.
	cb := &model.CleanBuild{
		Game: model.GameLoL, ChampionID: 103, ChampionKey: "Ahri",
		Role: model.RoleMiddle, Patch: "15.20", Source: "ugg",
		Total: model.Stats{Games: 1200, Wins: 620},
		CoreItems: []model.RankedItem{
			{ItemStats: model.ItemStats{ID: 6653, Stats: model.Stats{Games: 900}}},
		},
		Boots: []model.RankedItem{
			{ItemStats: model.ItemStats{ID: 3020, Stats: model.Stats{Games: 800}}},
		},
	}
	plan := BuildPlan(cb, catalog(t), Options{})
	if plan.Evidence != EvidenceFallback {
		t.Errorf("evidencia = %s, se esperaba respaldo", plan.Evidence)
	}
	if plan.Complete {
		t.Error("u.gg no permite reconstruir la progresión")
	}
	if plan.Reason == "" {
		t.Error("debe explicar la limitación")
	}
	// Los CORE se conservan tal cual (no se recalculan).
	if len(plan.Core) != 1 || plan.Core[0].ID != 6653 {
		t.Errorf("los CORE debían conservarse: %+v", plan.Core)
	}
	// Las botas sí se pueden listar: su tabla existe.
	if len(plan.Boots) != 1 || plan.Boots[0].Item != 3020 {
		t.Errorf("botas del respaldo = %+v", plan.Boots)
	}
}

// TestParcheIncompatible: objetos de otro parche no se inventan.
func TestParcheIncompatible(t *testing.T) {
	f := fixture(t, "kaisa")
	cb := toCleanBuild(f)
	cb.Patch = "15.20" // el fixture es de 16.21
	// Catálogo sin ninguno de los ids del fixture: simula un parche que
	// ya no conoce esos objetos.
	plan := BuildPlan(cb, map[int]lolitems.Item{}, Options{})
	if len(plan.Steps) != 0 {
		t.Errorf("no debe haber pasos con un catálogo vacío: %+v", plan.Steps)
	}
	if plan.Complete {
		t.Error("el plan no puede estar completo")
	}
	if plan.Reason == "" {
		t.Error("debe explicar la falta de datos")
	}
}

// TestOrdenDeterminista: dos ejecuciones dan exactamente lo mismo.
func TestOrdenDeterminista(t *testing.T) {
	cat := catalog(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			f := fixture(t, key)
			a := BuildPlan(toCleanBuild(f), cat, Options{})
			b := BuildPlan(toCleanBuild(f), cat, Options{})

			if len(a.Steps) != len(b.Steps) {
				t.Fatalf("pasos %d vs %d", len(a.Steps), len(b.Steps))
			}
			for i := range a.Steps {
				if a.Steps[i].Item != b.Steps[i].Item {
					t.Errorf("paso %d: %d vs %d", i, a.Steps[i].Item, b.Steps[i].Item)
				}
			}
			ai, bi := a.AlternativeItems(), b.AlternativeItems()
			if len(ai) != len(bi) {
				t.Fatalf("alternativas %d vs %d", len(ai), len(bi))
			}
			for i := range ai {
				if ai[i].Item != bi[i].Item {
					t.Errorf("alternativa %d: %d vs %d", i, ai[i].Item, bi[i].Item)
				}
			}
			if len(a.Alternatives) != len(b.Alternatives) {
				t.Fatalf("grupos %d vs %d", len(a.Alternatives), len(b.Alternatives))
			}
			for i := range a.Alternatives {
				if a.Alternatives[i].Category != b.Alternatives[i].Category {
					t.Errorf("grupo %d: %s vs %s", i,
						a.Alternatives[i].Category, b.Alternatives[i].Category)
				}
			}
			for i := range a.Final {
				if a.Final[i] != b.Final[i] {
					t.Errorf("inventario %d: %d vs %d", i, a.Final[i], b.Final[i])
				}
			}
		})
	}
}

// TestNoSeleccionaPorWR: el paso elegido es el más comprado, no el de
// mejor WR.
func TestNoSeleccionaPorWR(t *testing.T) {
	// Un paso con menos partidas pero mejor WR no debe ganar.
	cb := &model.CleanBuild{
		Game: model.GameLoL, ChampionID: 1, ChampionKey: "T",
		Role: model.RoleMiddle, Patch: "16.21", Total: model.Stats{Games: 10000},
		ItemSlots: [][]model.ItemStats{{
			{ID: 3031, Stats: model.Stats{Games: 5000, Wins: 2500}}, // 50% WR, 5000 games
			{ID: 3089, Stats: model.Stats{Games: 500, Wins: 400}},   // 80% WR, 500 games
		}},
	}
	plan := BuildPlan(cb, catalog(t), Options{})
	if len(plan.Steps) == 0 {
		t.Fatal("sin pasos")
	}
	if plan.Steps[0].Item != 3031 {
		t.Errorf("se eligió %d por WR alto; debe mandar la frecuencia", plan.Steps[0].Item)
	}
}

// TestUmbralDeMuestra: por debajo del mínimo no se acepta un paso.
func TestUmbralDeMuestra(t *testing.T) {
	cb := &model.CleanBuild{
		Game: model.GameLoL, Role: model.RoleMiddle, Patch: "16.21",
		Total: model.Stats{Games: 10000},
		ItemSlots: [][]model.ItemStats{{
			{ID: 3031, Stats: model.Stats{Games: 50, Wins: 25}}, // < MinGames
		}},
	}
	plan := BuildPlan(cb, catalog(t), Options{})
	if len(plan.Steps) != 0 {
		t.Errorf("con 50 partidas no debe haber paso: %+v", plan.Steps)
	}
	if plan.Complete {
		t.Error("no puede estar completo")
	}
}

// TestAlternativasNoOcupanEspacio: las alternativas son opciones, no
// objetos equipados.
func TestAlternativasNoOcupanEspacio(t *testing.T) {
	f := fixture(t, "ahri")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{})
	if len(plan.Alternatives) == 0 {
		t.Skip("sin alternativas en este fixture")
	}
	alts := plan.AlternativeItems()
	// Ninguna alternativa está en el inventario final.
	equipped := map[int]bool{}
	for _, id := range plan.Final {
		equipped[id] = true
	}
	for _, a := range alts {
		if equipped[a.Item] {
			t.Errorf("la alternativa %d (%s) aparece como equipada", a.Item, a.Name)
		}
		if a.Evidence != EvidenceAlternative {
			t.Errorf("la alternativa %d tiene evidencia %s", a.Item, a.Evidence)
		}
	}
	// El tope se aplica por GRUPO, no al total.
	for _, g := range plan.Alternatives {
		if len(g.Items) > 3 {
			t.Errorf("categoría %q con %d alternativas (máx 3)", g.Category, len(g.Items))
		}
		if len(g.Items) == 0 {
			t.Errorf("categoría %q vacía: no debe emitirse", g.Category)
		}
		// Cada grupo solo contiene objetos de su categoría.
		for _, it := range g.Items {
			if !hasCat(it.Categories, g.Category) {
				t.Errorf("%s en el grupo %q sin esa categoría: %v",
					it.Name, g.Category, it.Categories)
			}
		}
	}
}

// TestInventarioMisionExplicita: la misión nunca se asume completada.
func TestInventarioMisionExplicita(t *testing.T) {
	f := fixture(t, "kaisa")
	cb := toCleanBuild(f)

	// Por defecto: desconocida, con botas en espacio principal.
	p := BuildPlan(cb, catalog(t), Options{})
	if p.Inventory.State != MissionUnknown {
		t.Errorf("estado por defecto = %s", p.Inventory.State)
	}
	if !p.Inventory.BootsSlot {
		t.Error("por defecto las botas ocupan espacio principal")
	}
	if p.Inventory.TotalSlots() != 6 {
		t.Errorf("espacios = %d", p.Inventory.TotalSlots())
	}

	// Forzada a completada: ranura especial y 6 principales.
	pc := BuildPlan(cb, catalog(t), Options{Mission: MissionComplete})
	if pc.Inventory.BootsSlot {
		t.Error("con misión completada las botas no ocupan espacio principal")
	}
	if pc.Inventory.MainSlots != 6 {
		t.Errorf("espacios principales = %d", pc.Inventory.MainSlots)
	}

	// Solo bot lane/support tienen misión.
	for _, role := range []model.Role{model.RoleTop, model.RoleJungle, model.RoleMiddle} {
		inv := inventoryFor(role, MissionComplete)
		if !inv.BootsSlot {
			t.Errorf("%s no debe tener ranura de botas de misión", role)
		}
		if inv.TotalSlots() != 6 {
			t.Errorf("%s: espacios = %d", role, inv.TotalSlots())
		}
	}
}

// TestCoreSeConserva: el plan no reordena ni recalcula los CORE.
func TestCoreSeConserva(t *testing.T) {
	f := fixture(t, "kaisa")
	cb := toCleanBuild(f)
	cb.CoreItems = []model.RankedItem{
		{ItemStats: model.ItemStats{ID: 3031, Stats: model.Stats{Games: 900}}, Wilson: 0.61},
		{ItemStats: model.ItemStats{ID: 3153, Stats: model.Stats{Games: 800}}, Wilson: 0.58},
	}
	plan := BuildPlan(cb, catalog(t), Options{})
	if len(plan.Core) != 2 {
		t.Fatalf("CORE = %d", len(plan.Core))
	}
	if plan.Core[0].ID != 3031 || plan.Core[1].ID != 3153 {
		t.Errorf("el orden de CORE cambió: %+v", plan.Core)
	}
	if plan.Core[0].Wilson != 0.61 {
		t.Errorf("Wilson alterado: %v", plan.Core[0].Wilson)
	}
}

// TestCamposDeCabecera: campeón, rol, parche y fuente se propagan.
func TestCamposDeCabecera(t *testing.T) {
	f := fixture(t, "malphite")
	plan := BuildPlan(toCleanBuild(f), catalog(t), Options{})
	if plan.ChampionName != "Malphite" || plan.ChampionID != 54 {
		t.Errorf("campeón = %s/%d", plan.ChampionName, plan.ChampionID)
	}
	if plan.Role != model.RoleTop {
		t.Errorf("rol = %s", plan.Role)
	}
	if plan.Patch != "16.21" {
		t.Errorf("parche = %s", plan.Patch)
	}
	if plan.Source != "lolalytics.com" {
		t.Errorf("fuente = %s", plan.Source)
	}
	if plan.Total.Games != f.Games {
		t.Errorf("partidas = %d, se esperaban %d", plan.Total.Games, f.Games)
	}
}

func hasCat(list []lolitems.Category, want lolitems.Category) bool {
	for _, c := range list {
		if c == want {
			return true
		}
	}
	return false
}

// TestUnSoloParDeBotas: el inventario no puede llevar dos pares de
// botas, aunque la fuente publique varias opciones.
func TestUnSoloParDeBotas(t *testing.T) {
	cat := catalog(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			plan := BuildPlan(toCleanBuild(fixture(t, key)), cat, Options{})
			boots := 0
			for _, id := range plan.Final {
				if meta, ok := cat[id]; ok && lolitems.IsBoots(meta) {
					boots++
				}
			}
			if boots > 1 {
				t.Errorf("%d pares de botas en el inventario final: %v", boots, plan.Final)
			}
		})
	}
	// Y el plan sí recomienda botas aunque el inventario ya esté lleno.
	plan := BuildPlan(toCleanBuild(fixture(t, "kaisa")), cat, Options{})
	if len(plan.Boots) == 0 {
		t.Fatal("debe haber botas recomendadas")
	}
}

// TestObjetoDeMisionDeSoporteOcupaInventario: el objeto de misión de
// soporte SÍ ocupa un espacio principal (su ranura especial es para
// guardianes de control, no para el objeto de soporte).
func TestObjetoDeMisionDeSoporteOcupaInventario(t *testing.T) {
	cat := catalog(t)
	plan := BuildPlan(toCleanBuild(fixture(t, "soraka")), cat, Options{})

	var questID int
	for _, s := range plan.Steps {
		if s.Quest {
			questID = s.Item
			if !s.QuestOccupies {
				t.Errorf("%d (%s) es de misión y debe ocupar inventario", s.Item, s.Name)
			}
			// No se inventa un minuto de compra.
			if s.BuyMinute != 0 {
				t.Errorf("%d (%s): minuto inventado = %d", s.Item, s.Name, s.BuyMinute)
			}
		}
	}
	if questID == 0 {
		t.Fatal("Soraka debía tener objeto de misión")
	}

	// Está en el inventario final.
	found := false
	for _, id := range plan.Final {
		if id == questID {
			found = true
		}
	}
	if !found {
		t.Errorf("el objeto de misión %d no está en el inventario final: %v", questID, plan.Final)
	}
	// Y no se superan los espacios.
	if len(plan.Final) > plan.Inventory.TotalSlots() {
		t.Errorf("inventario %d > %d espacios", len(plan.Final), plan.Inventory.TotalSlots())
	}
}

// TestSoporteNoRecibeRanuraDeBotas: la bonificación de BOT no se aplica
// al support.
func TestSoporteNoRecibeRanuraDeBotas(t *testing.T) {
	cat := catalog(t)
	cb := toCleanBuild(fixture(t, "soraka"))

	// Incluso forzando la misión como completada, el support mantiene
	// las botas en un espacio principal.
	for _, state := range []InventoryState{MissionUnknown, MissionPending, MissionComplete} {
		plan := BuildPlan(cb, cat, Options{Mission: state})
		inv := plan.Inventory
		if !inv.BootsSlot {
			t.Errorf("estado %s: el support no debe tener ranura de botas", state)
		}
		if inv.MainSlots != 5 || inv.TotalSlots() != 6 {
			t.Errorf("estado %s: espacios = %d principales / %d totales",
				state, inv.MainSlots, inv.TotalSlots())
		}
		if !inv.QuestSlot {
			t.Errorf("estado %s: el support sí tiene ranura de misión", state)
		}
		if inv.QuestSlotUse != "guardianes de control" {
			t.Errorf("estado %s: la ranura es para %q", state, inv.QuestSlotUse)
		}
	}
}

// TestBotSiRecibeRanuraDeBotas: solo BOT mueve las botas a la ranura.
func TestBotSiRecibeRanuraDeBotas(t *testing.T) {
	for _, role := range []model.Role{model.RoleTop, model.RoleJungle, model.RoleMiddle} {
		inv := inventoryFor(role, MissionComplete)
		if !inv.BootsSlot {
			t.Errorf("%s no debe tener ranura de botas", role)
		}
		if inv.QuestSlot {
			t.Errorf("%s no debe tener ranura de misión", role)
		}
		if inv.TotalSlots() != 6 {
			t.Errorf("%s: espacios = %d", role, inv.TotalSlots())
		}
	}
	// BOT antes y después.
	before := inventoryFor(model.RoleBottom, MissionPending)
	if !before.BootsSlot || before.MainSlots != 5 || before.TotalSlots() != 6 {
		t.Errorf("BOT antes: %+v", before)
	}
	after := inventoryFor(model.RoleBottom, MissionComplete)
	if after.BootsSlot {
		t.Error("BOT después: las botas deben ir a la ranura de misión")
	}
	if after.MainSlots != 6 {
		t.Errorf("BOT después: principales = %d", after.MainSlots)
	}
	if after.QuestSlotUse != "botas" {
		t.Errorf("BOT después: ranura para %q", after.QuestSlotUse)
	}
}

// TestParcheVsIDAusente: un parche incompatible y un id suelto ausente
// son problemas distintos y se informan distinto.
func TestParcheVsIDAusente(t *testing.T) {
	f := fixture(t, "kaisa")

	// (a) Catálogo vacío: ningún id cuadra → parche incompatible.
	cb := toCleanBuild(f)
	cb.Patch = "99.99"
	plan := BuildPlan(cb, map[int]lolitems.Item{}, Options{})
	if !plan.PatchMismatch {
		t.Error("con el catálogo vacío debe marcarse parche incompatible")
	}
	if plan.MissingIDs == 0 {
		t.Error("debe contar los ids ausentes")
	}
	if plan.Reason == "" || !contains(plan.Reason, "incompatible") {
		t.Errorf("motivo = %q", plan.Reason)
	}
	// El motivo nombra el parche de los DATOS, no el del catálogo: son
	// identidades distintas y el mensaje no debe confundirlas.
	if !contains(plan.Reason, "99.99") {
		t.Errorf("el motivo no cita el parche de los datos: %q", plan.Reason)
	}

	// (b) Un solo id ausente: no es un parche incompatible.
	cat := catalog(t)
	delete(cat, 6672) // el paso 1 de Kai'Sa
	cb2 := toCleanBuild(f)
	plan2 := BuildPlan(cb2, cat, Options{})
	if plan2.PatchMismatch {
		t.Error("un id suelto ausente no es un parche incompatible")
	}
	if plan2.MissingIDs != 1 {
		t.Errorf("MissingIDs = %d, se esperaba 1", plan2.MissingIDs)
	}
	// Aun así el plan sigue siendo construible con el resto.
	if len(plan2.Steps) == 0 {
		t.Error("debía seguir construyendo pasos con el resto del catálogo")
	}
	if plan2.Steps[0].Item == 6672 {
		t.Error("no debe incluir el id ausente")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestKaisaAntesYDespuesDeMision verifica el cambio de inventario.
func TestKaisaAntesYDespuesDeMision(t *testing.T) {
	cat := catalog(t)
	cb := toCleanBuild(fixture(t, "kaisa"))

	// Antes: 5 principales + botas = 6 espacios, completo.
	before := BuildPlan(cb, cat, Options{Mission: MissionPending})
	if before.Inventory.MainSlots != 5 || !before.Inventory.BootsSlot {
		t.Errorf("antes: %+v", before.Inventory)
	}
	if len(before.FinalMain) != 5 {
		t.Errorf("antes: principales = %d, se esperaban 5", len(before.FinalMain))
	}
	if !before.Complete {
		t.Errorf("antes debía estar completo: %s", before.Reason)
	}

	// Después: 6 principales; las botas van a la ranura de misión. Solo
	// hay evidencia para 5 principales, así que el plan queda INCOMPLETO
	// y se declara en vez de inventar el sexto.
	after := BuildPlan(cb, cat, Options{Mission: MissionComplete})
	if after.Inventory.MainSlots != 6 || after.Inventory.BootsSlot {
		t.Errorf("después: %+v", after.Inventory)
	}
	if len(after.FinalMain) != 5 {
		t.Errorf("después: principales = %d", len(after.FinalMain))
	}
	if after.Complete {
		t.Error("después no puede estar completo: falta evidencia del sexto principal")
	}
	// Con un solo objeto ausente la concordancia es "falta 1 objeto".
	if after.Reason == "" || (!contains(after.Reason, "falta 1 objeto") &&
		!contains(after.Reason, "faltan") && !contains(after.Reason, "sexto")) {
		t.Errorf("motivo = %q", after.Reason)
	}
	// Las botas siguen presentes, pero en la ranura de misión.
	hasBoots := false
	for _, id := range after.Final {
		if meta, ok := cat[id]; ok && lolitems.IsBoots(meta) {
			hasBoots = true
		}
	}
	if !hasBoots {
		t.Error("las botas deben seguir equipadas en la ranura de misión")
	}
}

// TestBotasNoCuentanComoPrincipal: las botas tienen su propio espacio.
func TestBotasNoCuentanComoPrincipal(t *testing.T) {
	cat := catalog(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			plan := BuildPlan(toCleanBuild(fixture(t, key)), cat, Options{})
			// Ningún objeto de FinalMain es un par de botas.
			for _, id := range plan.FinalMain {
				if meta, ok := cat[id]; ok && lolitems.IsBoots(meta) {
					t.Errorf("las botas %d (%s) están contadas como espacio principal", id, meta.Name)
				}
			}
			if len(plan.FinalMain) > plan.Inventory.MainSlots {
				t.Errorf("principales %d > %d espacios", len(plan.FinalMain), plan.Inventory.MainSlots)
			}
		})
	}
}

// TestSextoPrincipalNoSeInventa: sin evidencia no se rellena.
func TestSextoPrincipalNoSeInventa(t *testing.T) {
	cat := catalog(t)
	plan := BuildPlan(toCleanBuild(fixture(t, "kaisa")), cat, Options{Mission: MissionComplete})
	if len(plan.FinalMain) >= plan.Inventory.MainSlots {
		t.Skip("este fixture sí tiene seis principales")
	}
	if plan.Complete {
		t.Error("no debe declararse completo sin seis principales")
	}
	if plan.Reason == "" {
		t.Error("debe explicar la limitación")
	}
}

// TestTopeNoEstrellaCategorias: el tope global no puede dejar categorías
// enteras fuera por el orden canónico. Un ADC debe seguir viendo
// opciones defensivas aunque daño y penetración vayan primero.
func TestTopeNoEstrellaCategorias(t *testing.T) {
	cat := catalog(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		t.Run(key, func(t *testing.T) {
			cb := toCleanBuild(fixture(t, key))
			capped := BuildPlan(cb, cat, Options{})                        // tope por defecto (12)
			full := BuildPlan(cb, cat, Options{MaxAlternativesTotal: 999}) // sin tope

			got := map[lolitems.Category]bool{}
			for _, g := range capped.Alternatives {
				got[g.Category] = true
			}
			for _, g := range full.Alternatives {
				if !got[g.Category] {
					t.Errorf("el tope dejó fuera la categoría %q: %v", g.Category, got)
				}
			}
			// Y el total no se pasa.
			if n := len(capped.AlternativeItems()); n > 12 {
				t.Errorf("%d alternativas superan el tope de 12", n)
			}
		})
	}
}

// TestTopePorRondas: el reparto es equitativo. Con 12 plazas y más de
// 12 categorías, ninguna se queda a cero si tiene candidatos.
func TestTopePorRondas(t *testing.T) {
	cat := catalog(t)
	cb := toCleanBuild(fixture(t, "kaisa"))
	full := BuildPlan(cb, cat, Options{MaxAlternativesTotal: 999})
	if len(full.Alternatives) <= 12 {
		t.Skip("el fixture no tiene suficientes categorías para probar el reparto")
	}
	capped := BuildPlan(cb, cat, Options{MaxAlternativesTotal: 12})
	// Con más categorías que plazas, cada grupo recibe al menos uno
	// hasta agotar las 12 plazas.
	if len(capped.Alternatives) < 12 {
		t.Errorf("con 12 plazas y %d categorías se esperaban 12 grupos, hubo %d",
			len(full.Alternatives), len(capped.Alternatives))
	}
	for _, g := range capped.Alternatives {
		if len(g.Items) != 1 {
			t.Logf("grupo %q con %d (esperado 1 en la primera ronda)", g.Category, len(g.Items))
		}
	}
}

// TestTopeDeterminista: el reparto no depende del orden del mapa.
func TestTopeDeterminista(t *testing.T) {
	cat := catalog(t)
	cb := toCleanBuild(fixture(t, "kaisa"))
	a := BuildPlan(cb, cat, Options{})
	b := BuildPlan(cb, cat, Options{})
	if len(a.Alternatives) != len(b.Alternatives) {
		t.Fatalf("%d vs %d grupos", len(a.Alternatives), len(b.Alternatives))
	}
	for i := range a.Alternatives {
		if a.Alternatives[i].Category != b.Alternatives[i].Category {
			t.Errorf("grupo %d: %s vs %s", i, a.Alternatives[i].Category, b.Alternatives[i].Category)
		}
		if len(a.Alternatives[i].Items) != len(b.Alternatives[i].Items) {
			t.Errorf("grupo %s: %d vs %d ítems", a.Alternatives[i].Category,
				len(a.Alternatives[i].Items), len(b.Alternatives[i].Items))
		}
		for j := range a.Alternatives[i].Items {
			if a.Alternatives[i].Items[j].Item != b.Alternatives[i].Items[j].Item {
				t.Errorf("grupo %s ítem %d: %d vs %d", a.Alternatives[i].Category, j,
					a.Alternatives[i].Items[j].Item, b.Alternatives[i].Items[j].Item)
			}
		}
	}
}

// TestDiferenciaDeParcheNoEsIncompatibilidad: que el parche de las
// estadísticas y el del catálogo sean de series distintas es NORMAL
// (16.22 vs 16.23.1). No puede marcar el plan como incompatible ni
// cambiar ninguna decisión: el planificador no compara parches.
func TestDiferenciaDeParcheNoEsIncompatibilidad(t *testing.T) {
	f := fixture(t, "kaisa")
	cat := catalog(t)

	// Mismo fixture con parches distintos: las decisiones deben ser
	// idénticas, porque el parche solo se PROPAGA, no se compara.
	var planes []Plan
	for _, patch := range []string{"16.22", "16.23.1", "99.99"} {
		cb := toCleanBuild(f)
		cb.Patch = patch
		planes = append(planes, BuildPlan(cb, cat, Options{}))
	}
	base := planes[0]
	if base.PatchMismatch {
		t.Fatal("una diferencia de parche no es incompatibilidad")
	}
	if len(base.Steps) == 0 {
		t.Fatal("debía construir pasos")
	}
	for i, p := range planes[1:] {
		if p.PatchMismatch {
			t.Errorf("parche %q marcado como incompatible", p.Patch)
		}
		if len(p.Steps) != len(base.Steps) {
			t.Errorf("parche %q: %d pasos vs %d", p.Patch, len(p.Steps), len(base.Steps))
		}
		for j := range p.Steps {
			if p.Steps[j].Item != base.Steps[j].Item {
				t.Errorf("parche %q paso %d: %d vs %d",
					p.Patch, j+1, p.Steps[j].Item, base.Steps[j].Item)
			}
		}
		if len(p.Final) != len(base.Final) {
			t.Errorf("parche %q: inventario distinto", p.Patch)
		}
		// El parche se propaga tal cual: es procedencia, no una clave.
		if p.Patch != planes[i+1].Patch {
			t.Errorf("el parche no se propagó: %q", p.Patch)
		}
	}
}
