package clean

import (
	"math"
	"slices"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/ugg"
)

func TestWilsonLower(t *testing.T) {
	// 100% con 3 partidas debe puntuar por debajo de ~53% con 40.000.
	trap := WilsonLower(3, 3)
	big := WilsonLower(21200, 40000)
	if trap >= big {
		t.Errorf("Wilson(3/3)=%f debía ser menor que Wilson(21200/40000)=%f", trap, big)
	}
	if WilsonLower(0, 0) != 0 {
		t.Error("games=0 debía devolver 0")
	}
	// Sanity: el límite inferior nunca supera la proporción observada.
	if w := WilsonLower(70, 100); w >= 0.70 {
		t.Errorf("Wilson(70/100)=%f debía ser < 0.70", w)
	}
}

// TestCorePorPopularidadYOrdenDeCompra: en LoL el core son los ítems
// más comprados (no los de mejor Wilson, sesgados a los que se compran
// siempre), y se presentan en orden de compra. Deadlock conserva Wilson.
func TestCorePorPopularidadYOrdenDeCompra(t *testing.T) {
	cb := &model.ChampionBuilds{
		Game:  model.GameLoL,
		Total: model.Stats{Games: 1000, Wins: 500},
		Items: []model.ItemStats{
			{ID: 1, Stats: model.Stats{Games: 600, Wins: 300}, AvgBuyS: 900},
			{ID: 2, Stats: model.Stats{Games: 200, Wins: 110}, AvgBuyS: 600},
			// Trampa: 96% WR sobre 50 partidas; Wilson la pondría primera.
			{ID: 3, Stats: model.Stats{Games: 50, Wins: 48}, AvgBuyS: 1200},
			{ID: 4, Stats: model.Stats{Games: 100, Wins: 51}, AvgBuyS: 1500},
		},
	}
	b := Build(cb, Options{})
	got := make([]int, len(b.CoreItems))
	for i, it := range b.CoreItems {
		got[i] = it.ID
	}
	// Top-3 por pick rate (1, 2, 4) en orden de compra; el resto por Wilson.
	want := []int{2, 1, 4, 3}
	if len(got) != len(want) {
		t.Fatalf("core = %v, se esperaba %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("core = %v, se esperaba %v", got, want)
		}
	}

	cb.Game = model.GameDeadlock
	d := Build(cb, Options{})
	if len(d.CoreItems) == 0 || d.CoreItems[0].ID != 3 {
		t.Errorf("Deadlock debía conservar el orden Wilson (3 primero): %+v", d.CoreItems)
	}
}

// TestPropagacionSlotsYCombos: los slots (item1..item5) atraviesan el
// pipeline intactos y los combos quedan limitados a los 3 con más
// presencia.
func TestPropagacionSlotsYCombos(t *testing.T) {
	cb := &model.ChampionBuilds{
		Game:  model.GameLoL,
		Total: model.Stats{Games: 1000, Wins: 500},
		ItemSlots: [][]model.ItemStats{
			{{ID: 7, Stats: model.Stats{Games: 300, Wins: 160}, AvgBuyS: 720}},
		},
		ItemCombos: []model.ItemCombo{
			{IDs: []int{1, 2, 3}, Stats: model.Stats{Games: 200, Wins: 110}, PickRate: 20},
			{IDs: []int{2, 3, 4}, Stats: model.Stats{Games: 180, Wins: 95}, PickRate: 18},
			{IDs: []int{3, 4, 5}, Stats: model.Stats{Games: 150, Wins: 80}, PickRate: 15},
			{IDs: []int{4, 5, 6}, Stats: model.Stats{Games: 120, Wins: 60}, PickRate: 12},
		},
	}
	b := Build(cb, Options{})
	if len(b.ItemSlots) != 1 || b.ItemSlots[0][0].ID != 7 {
		t.Errorf("ItemSlots = %+v", b.ItemSlots)
	}
	if len(b.ItemCombos) != 3 || b.ItemCombos[0].IDs[0] != 1 {
		t.Errorf("ItemCombos = %+v (debían quedar los 3 primeros)", b.ItemCombos)
	}
}

// TestSituacionalesSinRarezas: un ítem que casi nadie compra (pick
// ínfimo con muestra absoluta grande) no es una recomendación; el
// filtro de partidas solos no lo atrapa. Deadlock conserva su lista.
func TestSituacionalesSinRarezas(t *testing.T) {
	cb := &model.ChampionBuilds{
		Game:  model.GameLoL,
		Total: model.Stats{Games: 100000, Wins: 50000},
		Items: []model.ItemStats{
			{ID: 1, Stats: model.Stats{Games: 50000, Wins: 26000}, AvgBuyS: 600},
			{ID: 2, Stats: model.Stats{Games: 40000, Wins: 20800}, AvgBuyS: 900},
			{ID: 3, Stats: model.Stats{Games: 30000, Wins: 15600}, AvgBuyS: 1200},
			{ID: 4, Stats: model.Stats{Games: 2500, Wins: 1275}, AvgBuyS: 1800}, // 2.5% ✓
			// Rareza: 0.8% de pick; pasa el filtro de partidas solas.
			{ID: 5, Stats: model.Stats{Games: 800, Wins: 440}, AvgBuyS: 2400},
		},
	}
	b := Build(cb, Options{})
	got := make([]int, len(b.CoreItems))
	for i, it := range b.CoreItems {
		got[i] = it.ID
	}
	if !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("core+situacionales = %v, se esperaba [1 2 3 4]", got)
	}
	// Deadlock no comparte el filtro: su dato ya viene curado.
	cb.Game = model.GameDeadlock
	d := Build(cb, Options{})
	if len(d.CoreItems) != 5 {
		t.Errorf("Deadlock debía conservar los 5: %+v", d.CoreItems)
	}
}

// TestWardRankeado: los wards se rankean aparte por Wilson y sobrevive
// el mejor; una muestra insuficiente deja la lista vacía.
func TestWardRankeado(t *testing.T) {
	cb := &model.ChampionBuilds{
		Game:  model.GameLoL,
		Total: model.Stats{Games: 100000, Wins: 50000},
		Wards: []model.ItemStats{
			{ID: 2055, Stats: model.Stats{Games: 20000, Wins: 11000}}, // 55% ✓
			{ID: 3340, Stats: model.Stats{Games: 95000, Wins: 47500}}, // 50%, más muestra
			// Muestra insuficiente: 10 partidas, 0.01% pick.
			{ID: 3363, Stats: model.Stats{Games: 10, Wins: 9}},
		},
	}
	b := Build(cb, Options{})
	if len(b.Wards) != 2 {
		t.Fatalf("wards = %+v, se esperaban 2 con muestra", b.Wards)
	}
	// Wilson premia al 2055 (55% con muestra grande) sobre el 3340.
	if b.Wards[0].ID != 2055 {
		t.Errorf("mejor ward = %d, se esperaba 2055", b.Wards[0].ID)
	}
	cb.Wards[1].Games = 50 // el 3340 cae bajo el filtro de muestra
	b = Build(cb, Options{})
	if len(b.Wards) != 1 || b.Wards[0].ID != 2055 {
		t.Errorf("tras recortar muestra: %+v, se esperaba solo 2055", b.Wards)
	}
}

// TestPipelineTrampas valida los datos trampa de los fixtures reales:
// win rates altos con muestras mínimas no deben colarse en el ranking.
func TestPipelineTrampas(t *testing.T) {
	cbs, err := ugg.LoadDir("../../testdata/ugg")
	if err != nil {
		t.Fatal(err)
	}
	for _, cb := range cbs {
		if cb.ChampionKey != "Ahri" || cb.Role != "MIDDLE" {
			continue
		}
		b := Build(cb, Options{})
		for _, it := range b.CoreItems {
			switch it.ID {
			case 3157: // Zhonya's 3/3 (100% WR): excluido por el filtro
				t.Error("ítem trampa 3157 (3 partidas, 100% WR) no debía pasar el filtro")
			case 3135: // Void Staff 120/48213 (pick 0.25%): rareza, fuera
				t.Error("3135 (pick 0.25% sobre 48k partidas) debía caer por rareza")
			}
		}
		// El core son los 3 más comprados (6653, 4645, 3089): el orden
		// por Wilson premiaba muestras grandes aunque el ítem fuese un
		// componente, y el 3089 quedaba primero solo por WR. El resto
		// sigue por Wilson: la trampa 3157 cae por muestra corta y la
		// 3135 por rareza.
		wantCore := []int{6653, 4645, 3089}
		for i, id := range wantCore {
			if i >= len(b.CoreItems) || b.CoreItems[i].ID != id {
				t.Fatalf("core = %+v, se esperaba que empiece por %v", b.CoreItems, wantCore)
			}
		}
		if len(b.CoreItems) != 5 {
			t.Errorf("core = %d ítems, se esperaban 5", len(b.CoreItems))
		}
		// Página de runas trampa: 34/50 (68% WR) → excluida.
		for _, p := range b.RunePages {
			if p.KeystoneID == 8437 {
				t.Error("página de runas trampa (50 partidas, 68% WR) no debía pasar el filtro")
			}
		}
		// Electrocutar (8112): 53.25% sobre 9.840 partidas supera el
		// límite de Wilson de Cometa (8229): 52.54% sobre 21.020.
		if len(b.RunePages) != 2 || b.RunePages[0].KeystoneID != 8112 {
			t.Errorf("la mejor página de Ahri mid debía ser Electrocutar (8112), hay %+v", b.RunePages)
		}
	}
}

// TestPipelinePoolPequeño cubre Darius mid: pool de 890 partidas donde
// conviven muestras que pasan por pick rate y otras que no.
func TestPipelinePoolPequeño(t *testing.T) {
	cbs, err := ugg.LoadDir("../../testdata/ugg")
	if err != nil {
		t.Fatal(err)
	}
	for _, cb := range cbs {
		if cb.ChampionKey != "Darius" || cb.Role != "MIDDLE" {
			continue
		}
		b := Build(cb, Options{})
		ids := map[int]bool{}
		for _, it := range b.CoreItems {
			ids[it.ID] = true
		}
		// 140 y 120 partidas pasan (≥100). 40 (4.5%) y 30 (3.4%) quedan
		// fuera por no llegar ni a 100 partidas ni al 5% de pick rate.
		// La trampa 3026 (3/3) obviamente fuera.
		for _, id := range []int{6631, 3071} {
			if !ids[id] {
				t.Errorf("ítem %d con muestra suficiente debía conservarse", id)
			}
		}
		for _, id := range []int{3053, 3068, 3026} {
			if ids[id] {
				t.Errorf("ítem %d debía quedar filtrado", id)
			}
		}
		// La segunda secuencia (95 partidas, 10.7% pick) se conserva.
		if b.SkillOrder.Games != 410 {
			t.Errorf("skill order games = %d, se esperaba 410", b.SkillOrder.Games)
		}
	}
}

func TestBestSkillOrderPrioridad(t *testing.T) {
	cbs, err := ugg.LoadDir("../../testdata/ugg")
	if err != nil {
		t.Fatal(err)
	}
	for _, cb := range cbs {
		if cb.ChampionKey != "LeeSin" || cb.Role != "JUNGLE" {
			continue
		}
		b := Build(cb, Options{})
		want := "QEW"
		got := string(b.SkillOrder.Priority)
		if got != want {
			t.Errorf("prioridad Lee Sin jg = %q, se esperaba %q", got, want)
		}
	}
}

func TestBuildAllOrdenEstable(t *testing.T) {
	cbs, err := ugg.LoadDir("../../testdata/ugg")
	if err != nil {
		t.Fatal(err)
	}
	all := BuildAll(cbs, Options{})
	if len(all) != len(cbs) {
		t.Fatalf("BuildAll devolvió %d de %d", len(all), len(cbs))
	}
	for i, b := range all {
		if math.IsNaN(b.WinRate) {
			t.Errorf("build %d con WinRate NaN", i)
		}
	}
}
