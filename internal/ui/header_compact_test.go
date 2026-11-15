package ui

// Cabecera compacta al desplazarse: se recupera al volver arriba, sin
// saltos de scroll ni cambios de selección.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/model"
)

func headerApp(t *testing.T, w, h int) App {
	t.Helper()
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CoreItems, b.Boots = nil, nil
	s.Names.Items = map[int]string{}
	for i := 0; i < 20; i++ {
		id := 9000 + i
		s.Names.Items[id] = "Objeto prueba"
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: float64(100 + i*100)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	d := newDetailView(champ, s)
	d.width, d.height = w, h
	d.layout(0)
	a := App{screen: screenDetail, session: s, detail: d, width: w, height: h}
	a.detail.sync()
	return a
}

// TestHeaderExpandsAtTop: al abrir, la cabecera es la completa.
func TestHeaderExpandsAtTop(t *testing.T) {
	a := headerApp(t, 120, 40)
	if a.detail.scrolled() {
		t.Fatal("precondición: no debería estar desplazado")
	}
	full := lipgloss.Height(a.detail.headerBlock())
	if full < 2 {
		t.Errorf("cabecera completa de %d líneas: parece ya compacta", full)
	}
}

// TestHeaderCompactsOnScroll: al desplazarse, la cabecera se compacta.
func TestHeaderCompactsOnScroll(t *testing.T) {
	a := headerApp(t, 120, 40)
	full := lipgloss.Height(a.detail.headerBlock())

	for i := 0; i < 4; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	if !a.detail.scrolled() {
		t.Fatal("precondición: no se desplazó")
	}
	compact := lipgloss.Height(a.detail.headerBlock())
	if compact >= full {
		t.Errorf("cabecera %d líneas tras scroll, completa era %d: no se compactó", compact, full)
	}
	if compact != 1 {
		t.Errorf("cabecera compacta de %d líneas, esperado 1", compact)
	}
}

// TestHeaderRestoresAtTop: al volver arriba se recupera la completa.
func TestHeaderRestoresAtTop(t *testing.T) {
	a := headerApp(t, 120, 40)
	full := lipgloss.Height(a.detail.headerBlock())
	for i := 0; i < 4; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	for i := 0; i < 8; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	}
	if a.detail.vp.YOffset != 0 {
		t.Fatalf("no volvió arriba: offset %d", a.detail.vp.YOffset)
	}
	if got := lipgloss.Height(a.detail.headerBlock()); got != full {
		t.Errorf("cabecera %d tras volver arriba, esperado %d", got, full)
	}
}

// TestHeaderScrollNoJump: el cambio de cabecera no reinicia el scroll.
func TestHeaderScrollNoJump(t *testing.T) {
	a := headerApp(t, 120, 40)
	for i := 0; i < 5; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	before := a.detail.vp.YOffset
	// Renderizar (que recalcula el layout) no debe mover el offset.
	_ = a.View()
	if a.detail.vp.YOffset != before {
		t.Errorf("el render movió el scroll: %d → %d", before, a.detail.vp.YOffset)
	}
}

// TestHeaderSelectionStable: la selección no cambia al compactarse.
func TestHeaderSelectionStable(t *testing.T) {
	a := headerApp(t, 120, 40)
	for i := 0; i < 3; i++ {
		a = routeLoadingUpdate(a, keyMsg("right"))
	}
	sel := a.detail.cardList()[a.detail.itemIdx].item.ID
	for i := 0; i < 4; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	_ = a.View()
	if got := a.detail.cardList()[a.detail.itemIdx].item.ID; got != sel {
		t.Errorf("la selección cambió: %d → %d", sel, got)
	}
}

// TestHeaderNarrowTerminalKeepsCompact: en terminal baja siempre compacta.
func TestHeaderNarrowTerminalKeepsCompact(t *testing.T) {
	a := headerApp(t, 80, 16)
	if got := lipgloss.Height(a.detail.headerBlock()); got != 1 {
		t.Errorf("cabecera de %d líneas en 80x16, esperado 1", got)
	}
	// Y el nombre sigue visible.
	plain := collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
	if !strings.Contains(plain, "Abrams") {
		t.Error("la cabecera compacta perdió el nombre del héroe")
	}
}

// TestHeaderResizeStable: redimensionar no rompe el estado.
func TestHeaderResizeStable(t *testing.T) {
	a := headerApp(t, 120, 40)
	for i := 0; i < 3; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	before := a.detail.vp.YOffset
	// El resize se aplica al detalle directamente: el list de la pantalla
	// de lista no está inicializado en este fixture y no debe tocarse.
	a.detail.width, a.detail.height = 100, 30
	a.width, a.height = 100, 30
	a.detail.sync()
	if a.detail.vp.YOffset > before {
		t.Errorf("el resize aumentó el scroll: %d → %d", before, a.detail.vp.YOffset)
	}
	if got := lipgloss.Height(a.View()); got > 30 {
		t.Errorf("alto %d excede 30 tras resize", got)
	}
}
