package ui

// Pruebas de integración de la build híbrida con la TUI (Fase 5).
// Usan fixtures sintéticos: reproducibles, sin red.

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/model"
)

// hybridStub implementa RunsLoader y BuildLoader con datos fijos.
type hybridStub struct {
	build *HybridBuildView
	err   error
}

func (h hybridStub) LoadRuns(string) ([]RouteStep, error) { return nil, nil }
func (h hybridStub) LoadHybrid(string) (*HybridBuildView, error) {
	return h.build, h.err
}

// hybridApp arma un detalle de Deadlock con `count` ítems y la build
// híbrida ya cargada.
func hybridApp(t *testing.T, build *HybridBuildView, count, width, height int) App {
	t.Helper()
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CoreItems, b.Boots = nil, nil
	s.Names.Items = map[int]string{}
	for i := 0; i < count; i++ {
		id := 9000 + i
		s.Names.Items[id] = fmt.Sprintf("Objeto prueba %02d", i)
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: float64(100 + i*100)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	s.HybridLoader = hybridStub{build: build}
	s.HybridKey = "test|"
	if build != nil {
		build.Key = s.HybridKey // como hace hybridLoadedMsg al llegar
	}
	s.Hybrids = map[string]*HybridBuildView{"Abrams": build}
	d := newDetailView(champ, s)
	d.width, d.height = width, height
	d.layout(0) // fija el viewport al tamaño real de la terminal
	a := App{screen: screenDetail, session: s, detail: d, width: width, height: height}
	a.detail.sync()
	return a
}

// sampleHybrid arma una build con las cinco secciones.
func sampleHybrid() *HybridBuildView {
	return &HybridBuildView{
		Confidence: 0.62,
		FinalItems: 4,
		Spent:      12800,
		Standard: []HybridStep{
			{Item: 9000, Name: "Objeto prueba 00", Phase: 0, Minute: 4, Confidence: 0.80, Share: 0.90, Origin: []string{"observed", "statistical"}},
			{Item: 9001, Name: "Objeto prueba 01", Phase: 0, Minute: 6, Confidence: 0.78, Share: 0.85, Origin: []string{"observed"}},
			{Item: 9002, Name: "Objeto prueba 02", Phase: 1, Minute: 12, Confidence: 0.70, Share: 0.55, Chain: []int{9001, 9002}, Origin: []string{"statistical"}},
			{Item: 9003, Name: "Objeto prueba 03", Phase: 2, Minute: 25, Confidence: 0.60, Share: 0.40, Origin: []string{"statistical"}},
		},
		Extensions: []HybridStep{
			{Item: 9004, Name: "Objeto prueba 04", Phase: 2, Minute: 30, Confidence: 0.55, Share: 0.35},
		},
		Situationals: []HybridStep{
			{Item: 9005, Name: "Objeto prueba 05", Phase: 1, Minute: 15, Confidence: 0.50, Share: 0.20, Reason: "Anticuración"},
			{Item: 9006, Name: "Objeto prueba 06", Phase: 2, Minute: 22, Confidence: 0.48, Share: 0.18, Reason: "Defensa contra balas"},
		},
	}
}

func hybridText(a App) string {
	return collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
}

// TestHybridSectionsVisible: las cinco secciones aparecen en orden.
func TestHybridSectionsVisible(t *testing.T) {
	// El contenido excede el viewport (como en una partida real), así
	// que se recorre con la rueda recogiendo lo visible: el orden de
	// las secciones es lo que se verifica.
	a := hybridApp(t, sampleHybrid(), 10, 120, 24)
	var seen strings.Builder
	seen.WriteString(hybridText(a))
	for i := 0; i < 20; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		seen.WriteString("\n" + hybridText(a))
	}
	text := seen.String()
	assertInOrder(t, text, "EARLY", "MID", "LATE", "AMPLIACIONES", "SITUACIONALES")
}

// TestHybridStepNumbersContinuous: la numeración de la ruta es continua
// y las ampliaciones/situacionales no llevan número.
func TestHybridStepNumbersContinuous(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 120, 40)
	cards := a.detail.cardList()
	var steps []int
	for _, c := range cards {
		if c.hybridSection == SectionStandard {
			steps = append(steps, c.step)
		}
	}
	want := []int{1, 2, 3, 4}
	if fmt.Sprint(steps) != fmt.Sprint(want) {
		t.Errorf("pasos = %v, esperado %v", steps, want)
	}
	for _, c := range cards {
		if c.hybridSection != SectionStandard && c.step != 0 {
			t.Errorf("%d fuera de la ruta con paso %d", c.item.ID, c.step)
		}
	}
}

// TestHybridCardsComeFromBuild: las tarjetas reflejan la build, no la
// ruta observada.
func TestHybridCardsComeFromBuild(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 120, 40)
	cards := a.detail.cardList()
	if len(cards) != 7 { // 4 ruta + 1 ampliación + 2 situacionales
		t.Fatalf("tarjetas = %d, esperado 7", len(cards))
	}
	if !cards[0].hybrid {
		t.Error("la primera tarjeta no está marcada como híbrida")
	}
	if cards[0].item.ID != 9000 {
		t.Errorf("primera tarjeta = %d, esperado 9000", cards[0].item.ID)
	}
}

// TestHybridFallsBackWithoutBuild: sin build híbrida se usa la ruta
// observada, sin romper nada.
func TestHybridFallsBackWithoutBuild(t *testing.T) {
	a := hybridApp(t, nil, 10, 120, 40)
	a.session.Hybrids["Abrams"] = nil
	cards := a.detail.cardList()
	for _, c := range cards {
		if c.hybrid {
			t.Error("tarjeta híbrida sin build calculada")
		}
	}
}

// TestHybridConfidenceNotShownAsProbability: la confianza se presenta
// como palabra, nunca como porcentaje de victoria.
func TestHybridConfidenceNotShownAsProbability(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 120, 40)
	text := hybridText(a)
	if strings.Contains(text, "62%") || strings.Contains(text, "0.62") {
		t.Error("la confianza se muestra como número crudo")
	}
	if !strings.Contains(text, "moderada") {
		t.Error("falta la etiqueta cualitativa de confianza")
	}
	for _, bad := range []string{"probabilidad", "victoria", "winrate", "win rate"} {
		if strings.Contains(strings.ToLower(text), bad) {
			t.Errorf("la UI sugiere causalidad: %q", bad)
		}
	}
}

// TestHybridSituationalLabelsShort: las etiquetas situacionales son
// breves y no afirman contrarrestar héroes.
func TestHybridSituationalLabelsShort(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 120, 40)
	for _, c := range a.detail.cardList() {
		if c.hybridSection != SectionSituationals {
			continue
		}
		if len(c.hybridReason) > 24 {
			t.Errorf("etiqueta demasiado larga: %q", c.hybridReason)
		}
		// "contra balas" / "contra espíritu" nombran un tipo de daño,
		// no un héroe. Lo prohibido es afirmar que contrarresta a un
		// héroe concreto.
		lower := strings.ToLower(c.hybridReason)
		for _, bad := range []string{" vs ", "counter", "contrarresta a", "contra el héroe"} {
			if strings.Contains(lower, bad) {
				t.Errorf("etiqueta sugiere contrarrestar un héroe: %q", c.hybridReason)
			}
		}
	}
}

// TestHybridPopupShowsProvenance: el detalle (Enter) muestra la
// procedencia.
func TestHybridPopupShowsProvenance(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 10, 120, 40)
	a = routeLoadingUpdate(a, keyMsg("enter"))
	text := hybridText(a)
	if !strings.Contains(text, "procedencia") {
		t.Errorf("el popup no muestra la procedencia: %s", text)
	}
	if !strings.Contains(text, "observed") {
		t.Errorf("el popup no lista las procedencias reales: %s", text)
	}
}

// TestHybridSelectionAndScrollPreserved: la llegada de la build no
// mueve la selección ni el scroll.
func TestHybridSelectionAndScrollPreserved(t *testing.T) {
	a := hybridApp(t, nil, 14, 120, 24)
	// Mover a la tarjeta 5 y desplazar.
	for i := 0; i < 5; i++ {
		a = routeLoadingUpdate(a, keyMsg("right"))
	}
	for i := 0; i < 3; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	before := a.detail.vp.YOffset
	selected := a.detail.cardList()[a.detail.itemIdx].item.ID

	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Build: sampleHybrid()})
	if a.detail.vp.YOffset != before {
		t.Errorf("la build movió el scroll: %d → %d", before, a.detail.vp.YOffset)
	}
	got := a.detail.cardList()[a.detail.itemIdx].item.ID
	if got != selected {
		t.Errorf("la build cambió la selección: %d → %d", selected, got)
	}
}

// TestHybridNarrowTerminal: en una terminal estrecha la vista no
// desborda y conserva las secciones.
func TestHybridNarrowTerminal(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{80, 24}, {120, 40}, {60, 20}, {40, 16},
	} {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			a := hybridApp(t, sampleHybrid(), 10, size.w, size.h)
			view := a.View()
			// El ancho es el requisito real de "terminal angosta": ninguna
			// línea puede desbordar.
			for _, line := range strings.Split(view, "\n") {
				if lipgloss.Width(line) > size.w {
					t.Errorf("línea excede el ancho %d: %q", size.w, line)
				}
			}
			// El alto debe caber en todos los tamaños: la cabecera
			// compacta de terminales bajas lo garantiza.
			if got := lipgloss.Height(view); got > size.h {
				t.Errorf("alto %d excede %d", got, size.h)
			}
			text := hybridText(a)
			if !strings.Contains(text, "EARLY") {
				t.Errorf("sin EARLY en %dx%d", size.w, size.h)
			}
		})
	}
}

// TestHybridLoadingState: mientras se calcula, se dice; no se muestra
// una build vacía como si fuera definitiva.
func TestHybridLoadingState(t *testing.T) {
	a := hybridApp(t, nil, 10, 120, 40)
	a.session.Hybrids["Abrams"] = nil // en curso
	if got := a.detail.routeStatus(); !strings.Contains(got, "Calculando") {
		t.Errorf("estado en curso = %q", got)
	}
}

// TestHybridErrorKeepsRoute: un error de la build híbrida no rompe la
// vista; se cae a la ruta observada.
func TestHybridErrorKeepsRoute(t *testing.T) {
	a := hybridApp(t, nil, 10, 120, 40)
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Err: fmt.Errorf("sin red")})
	if a.session.Hybrids["Abrams"] != nil {
		t.Error("un error no debe dejar una build parcial")
	}
	// La vista sigue renderizando.
	if hybridText(a) == "" {
		t.Error("la vista quedó vacía tras el error")
	}
}

// TestHybridDoesNotAffectLoL: en LoL las tarjetas no llevan marca
// híbrida aunque la sesión tenga un loader.
func TestHybridDoesNotAffectLoL(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	champ := s.Champions[0]
	d := newDetailView(champ, s)
	d.width, d.height = 120, 40
	cards := d.cardList()
	for _, c := range cards {
		if c.hybrid {
			t.Error("tarjeta híbrida en LoL")
		}
	}
}

// TestConfidenceWord: los umbrales son los documentados.
func TestConfidenceWord(t *testing.T) {
	cases := map[float64]string{
		0.9: "alta", 0.70: "alta",
		0.6: "moderada", 0.45: "moderada",
		0.3: "baja", 0: "baja",
	}
	for v, want := range cases {
		if got := ConfidenceWord(v); got != want {
			t.Errorf("ConfidenceWord(%.2f) = %q, esperado %q", v, got, want)
		}
	}
}
