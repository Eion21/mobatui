package ui

// Situacionales agrupados por función: categorías escaneables, sin
// repetir objetos y sin inventar categorías.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// situationalHybrid arma una build con situacionales de varias funciones.
func situationalHybrid() *HybridBuildView {
	return &HybridBuildView{
		Confidence: 0.62,
		FinalItems: 2,
		Standard: []HybridStep{
			{Item: 9000, Name: "Objeto prueba 00", Phase: 0, Minute: 4, Confidence: 0.80, Share: 0.90},
		},
		Situationals: []HybridStep{
			{Item: 9001, Name: "Objeto prueba 01", Phase: 2, Minute: 25, Confidence: 0.55, Reason: "Anticuración"},
			{Item: 9002, Name: "Objeto prueba 02", Phase: 1, Minute: 15, Confidence: 0.50, Reason: "Supervivencia"},
			{Item: 9003, Name: "Objeto prueba 03", Phase: 2, Minute: 22, Confidence: 0.52, Reason: "Daño adicional"},
			{Item: 9004, Name: "Objeto prueba 04", Phase: 2, Minute: 20, Confidence: 0.51, Reason: "Supervivencia"},
			{Item: 9005, Name: "Objeto prueba 05", Phase: 1, Minute: 13, Confidence: 0.53, Reason: "Defensa contra balas"},
		},
	}
}

// TestSituationalsGroupedByFunction: cada función tiene su encabezado.
func TestSituationalsGroupedByFunction(t *testing.T) {
	// Se comprueban los títulos de sección directamente: recorrer el
	// viewport depende del alto y haría la prueba frágil.
	a := hybridApp(t, situationalHybrid(), 12, 140, 40)
	secs, _ := a.detail.itemsSectionsDL(a.detail.cardList())
	var titles []string
	for i := 0; i+1 < len(secs); i += 2 {
		titles = append(titles, collapsePlain(ansiRE.ReplaceAllString(secs[i], "")))
	}
	joined := strings.Join(titles, "\n")
	for _, fn := range []string{"Anticuración", "Supervivencia", "Daño adicional", "Defensa contra balas"} {
		if !strings.Contains(joined, "SITUACIONALES · "+fn) {
			t.Errorf("falta el grupo %q en:\n%s", fn, joined)
		}
	}
}

// TestSituationalsNoDuplicates: ningún objeto aparece dos veces.
func TestSituationalsNoDuplicates(t *testing.T) {
	a := hybridApp(t, situationalHybrid(), 12, 140, 40)
	cards := a.detail.cardList()
	seen := map[int]int{}
	for _, c := range cards {
		seen[c.item.ID]++
	}
	for id, n := range seen {
		if n > 1 {
			t.Errorf("objeto %d aparece %d veces", id, n)
		}
	}
}

// TestSituationalsGroupOrderStable: el orden de grupos es fijo.
func TestSituationalsGroupOrderStable(t *testing.T) {
	a := hybridApp(t, situationalHybrid(), 12, 140, 40)
	cards := a.detail.cardList()
	secs, _ := a.detail.itemsSectionsDL(cards)
	var titles []string
	for i := 0; i+1 < len(secs); i += 2 {
		titles = append(titles, collapsePlain(ansiRE.ReplaceAllString(secs[i], "")))
	}
	// Los grupos deben aparecer en el orden canónico declarado.
	pos := map[string]int{}
	for i, t := range titles {
		for _, fn := range situationalOrder {
			if strings.Contains(t, "SITUACIONALES · "+fn) {
				pos[fn] = i
			}
		}
	}
	last := -1
	for _, fn := range situationalOrder {
		if p, ok := pos[fn]; ok {
			if p < last {
				t.Errorf("grupo %q fuera de orden", fn)
			}
			last = p
		}
	}
}

// TestSituationalsSelectionPreserved: la selección sigue siendo válida.
func TestSituationalsSelectionPreserved(t *testing.T) {
	a := hybridApp(t, situationalHybrid(), 12, 140, 40)
	for i := 0; i < 4; i++ {
		a = routeLoadingUpdate(a, keyMsg("right"))
	}
	idx := a.detail.itemIdx
	cards := a.detail.cardList()
	if idx >= len(cards) {
		t.Fatalf("selección fuera de rango: %d >= %d", idx, len(cards))
	}
	_ = a.View()
	if a.detail.itemIdx != idx {
		t.Errorf("el render movió la selección: %d → %d", idx, a.detail.itemIdx)
	}
}

// TestSituationalsNarrowTerminal: en terminal angosta no desborda.
func TestSituationalsNarrowTerminal(t *testing.T) {
	for _, w := range []int{120, 80, 60, 40} {
		a := hybridApp(t, situationalHybrid(), 12, w, 24)
		for _, line := range strings.Split(a.View(), "\n") {
			if width(line) > w {
				t.Errorf("w=%d: línea excede el ancho: %q", w, line)
			}
		}
	}
}

// wheelDown devuelve un evento de rueda hacia abajo.
func wheelDown() tea.MouseMsg {
	return tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
}

// width mide el ancho visible de una línea.
func width(s string) int { return lipgloss.Width(s) }
