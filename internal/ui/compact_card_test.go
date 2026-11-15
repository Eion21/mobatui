package ui

// Tarjeta compacta: menos altura sin perder nombre, paso, mejora,
// coste, minuto ni confianza. La navegación y los popups no cambian.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/model"
)

// cardApp arma un detalle con N ítems a la altura dada.
func cardApp(t *testing.T, h int) App {
	t.Helper()
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CoreItems, b.Boots = nil, nil
	s.Names.Items = map[int]string{}
	for i := 0; i < 12; i++ {
		id := 9000 + i
		s.Names.Items[id] = "Objeto prueba"
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: float64(100 + i*100)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	d := newDetailView(champ, s)
	d.width, d.height = 120, h
	d.layout(0)
	a := App{screen: screenDetail, session: s, detail: d, width: 120, height: h}
	a.detail.sync()
	return a
}

// TestCompactCardShorter: la tarjeta compacta ocupa menos alto.
func TestCompactCardShorter(t *testing.T) {
	// El modo depende del alto del viewport, no del de la terminal: se
	// fuerza cada uno para comparar las dos presentaciones.
	a := cardApp(t, 40)
	cards := a.detail.cardList()
	if len(cards) == 0 {
		t.Fatal("sin tarjetas")
	}

	a.detail.vp.Height = 40 // normal
	if a.detail.compactMode() {
		t.Fatal("precondición: 40 filas debería ser normal")
	}
	hTall := lipgloss.Height(a.detail.itemCard(0, cards[0]))

	a.detail.vp.Height = 10 // compacta
	if !a.detail.compactMode() {
		t.Fatal("precondición: 10 filas debería ser compacta")
	}
	hShort := lipgloss.Height(a.detail.itemCard(0, cards[0]))

	if hShort >= hTall {
		t.Errorf("compacta %d líneas, normal %d: no se redujo", hShort, hTall)
	}
	t.Logf("alto tarjeta: normal=%d compacta=%d", hTall, hShort)
}

// TestCompactCardKeepsData: conserva nombre, paso, coste y minuto.
func TestCompactCardKeepsData(t *testing.T) {
	a := cardApp(t, 20)
	if !a.detail.compactMode() {
		t.Skip("no está en modo compacto")
	}
	cards := a.detail.cardList()
	plain := collapsePlain(ansiRE.ReplaceAllString(a.detail.itemCard(0, cards[0]), ""))
	for _, want := range []string{"Objeto prueba", "1", "min"} {
		if !strings.Contains(plain, want) {
			t.Errorf("la tarjeta compacta perdió %q: %s", want, plain)
		}
	}
}

// TestCompactCardKeepsRelation: la mejora se sigue leyendo.
func TestCompactCardKeepsRelation(t *testing.T) {
	a := cardApp(t, 20)
	if !a.detail.compactMode() {
		t.Skip("no está en modo compacto")
	}
	cards := a.detail.cardList()
	// Forzar una tarjeta de mejora.
	c := cards[0]
	c.kind = kindUpgrade
	c.baseName = "Base"
	plain := collapsePlain(ansiRE.ReplaceAllString(a.detail.itemCard(0, c), ""))
	if !strings.Contains(plain, "Base") {
		t.Errorf("la tarjeta compacta perdió la relación de mejora: %s", plain)
	}
}

// TestCompactCardNavigation: selección y popup siguen funcionando.
func TestCompactCardNavigation(t *testing.T) {
	a := cardApp(t, 20)
	for i := 0; i < 3; i++ {
		a = routeLoadingUpdate(a, keyMsg("right"))
	}
	if a.detail.itemIdx != 3 {
		t.Fatalf("selección = %d, esperado 3", a.detail.itemIdx)
	}
	a = routeLoadingUpdate(a, keyMsg("enter"))
	if !strings.Contains(hybridText(a), "Objeto prueba") {
		t.Error("el popup no se abrió en modo compacto")
	}
	a = routeLoadingUpdate(a, keyMsg("esc"))
	if a.detail.showItem {
		t.Error("el popup no se cerró")
	}
}

// TestCompactCardNoOverflow: en terminal baja no desborda.
func TestCompactCardNoOverflow(t *testing.T) {
	for _, size := range []struct{ w, h int }{{120, 20}, {80, 20}, {60, 20}, {40, 16}} {
		a := cardApp(t, size.h)
		a.detail.width, a.width = size.w, size.w
		a.detail.sync()
		if got := lipgloss.Height(a.View()); got > size.h {
			t.Errorf("%dx%d: alto %d excede", size.w, size.h, got)
		}
	}
}

// TestCompactCardDeterministic: mismo estado, misma tarjeta.
func TestCompactCardDeterministic(t *testing.T) {
	a := cardApp(t, 20)
	cards := a.detail.cardList()
	x := a.detail.itemCard(0, cards[0])
	y := a.detail.itemCard(0, cards[0])
	if x != y {
		t.Error("la tarjeta compacta no es determinista")
	}
}
