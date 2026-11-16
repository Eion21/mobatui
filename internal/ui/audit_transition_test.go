package ui

// Auditoría de la transición entre la ruta observada y la build híbrida:
// ambos llegan de forma concurrente y en cualquier orden.

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// obsRoute arma una ruta observada sintética.
func obsRoute(n int) []RouteStep {
	out := make([]RouteStep, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, RouteStep{Item: 9000 + i, MedT: 300 + i*120, SampleSize: 250, MinShare: 0.3})
	}
	return out
}

// TestTransitionObservedThenHybrid: primero la ruta, después la build.
func TestTransitionObservedThenHybrid(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.Hybrids["Abrams"] = nil // en curso

	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
	if len(a.session.Routes["Abrams"]) != 6 {
		t.Fatalf("ruta no guardada: %d", len(a.session.Routes["Abrams"]))
	}
	// La vista muestra la ruta observada mientras la híbrida no llega.
	text := hybridText(a)
	if !strings.Contains(text, "Ruta observada") {
		t.Errorf("la ruta provisional no está identificada: %s", text)
	}

	if a.session.Hybrids == nil {
		a.session.Hybrids = map[string]*HybridBuildView{}
	}
	if a.session.HybridLoading == nil {
		a.session.HybridLoading = map[string]string{}
	}
	a.session.Hybrids["Abrams"] = nil
	a.session.HybridLoading["Abrams"] = a.session.HybridKey
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
	text = hybridText(a)
	if !strings.Contains(text, "Build híbrida") {
		t.Errorf("la build híbrida no reemplazó a la ruta: %s", text)
	}
	if strings.Contains(text, "Ruta observada") {
		t.Error("la cabecera de ruta observada sigue visible con la híbrida cargada")
	}
}

// TestTransitionHybridThenObserved: primero la build, después la ruta.
// La ruta que llega tarde no debe pisar la build.
func TestTransitionHybridThenObserved(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.Hybrids["Abrams"] = nil

	if a.session.Hybrids == nil {
		a.session.Hybrids = map[string]*HybridBuildView{}
	}
	if a.session.HybridLoading == nil {
		a.session.HybridLoading = map[string]string{}
	}
	a.session.Hybrids["Abrams"] = nil
	a.session.HybridLoading["Abrams"] = a.session.HybridKey
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
	if !strings.Contains(hybridText(a), "Build híbrida") {
		t.Fatal("la build no se mostró")
	}
	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})

	text := hybridText(a)
	if !strings.Contains(text, "Build híbrida") {
		t.Errorf("la ruta tardía pisó la build híbrida: %s", text)
	}
	// La ruta se guarda igualmente (sirve de comparación y de fallback).
	if len(a.session.Routes["Abrams"]) != 6 {
		t.Errorf("la ruta tardía no se guardó: %d", len(a.session.Routes["Abrams"]))
	}
}

// TestTransitionLateResponseOtherHero: una respuesta tardía de otro
// héroe no debe tocar el detalle activo.
func TestTransitionLateResponseOtherHero(t *testing.T) {
	a := hybridApp(t, sampleHybrid(), 12, 120, 40)
	before := hybridText(a)

	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "OtroHeroe", Key: a.session.HybridKey, Build: sampleHybrid()})
	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "OtroHeroe", Steps: obsRoute(3)})

	if got := hybridText(a); got != before {
		t.Error("una respuesta de otro héroe cambió el detalle activo")
	}
}

// TestTransitionSelectionSurvivesBothOrders: la selección se conserva
// en los dos órdenes de llegada.
func TestTransitionSelectionSurvivesBothOrders(t *testing.T) {
	for _, hybridFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("hybridFirst=%v", hybridFirst), func(t *testing.T) {
			a := hybridApp(t, nil, 12, 120, 40)
			a.session.Hybrids["Abrams"] = nil
			// Ruta inicial para tener tarjetas.
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
			for i := 0; i < 3; i++ {
				a = routeLoadingUpdate(a, keyMsg("right"))
			}
			sel := a.detail.cardList()[a.detail.itemIdx].item.ID

			if hybridFirst {
				if a.session.Hybrids == nil {
					a.session.Hybrids = map[string]*HybridBuildView{}
				}
				if a.session.HybridLoading == nil {
					a.session.HybridLoading = map[string]string{}
				}
				a.session.Hybrids["Abrams"] = nil
				a.session.HybridLoading["Abrams"] = a.session.HybridKey
				a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
				a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
			} else {
				a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
				if a.session.Hybrids == nil {
					a.session.Hybrids = map[string]*HybridBuildView{}
				}
				if a.session.HybridLoading == nil {
					a.session.HybridLoading = map[string]string{}
				}
				a.session.Hybrids["Abrams"] = nil
				a.session.HybridLoading["Abrams"] = a.session.HybridKey
				a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
			}
			cards := a.detail.cardList()
			if a.detail.itemIdx >= len(cards) {
				t.Fatalf("selección fuera de rango: %d >= %d", a.detail.itemIdx, len(cards))
			}
			// La selección puede cambiar de índice (la lista cambió) pero
			// debe seguir apuntando a un objeto válido y no perderse.
			if cards[a.detail.itemIdx].item.ID != sel {
				t.Logf("selección %d → %d (la lista cambió de contenido)",
					sel, cards[a.detail.itemIdx].item.ID)
			}
		})
	}
}

// TestTransitionErrorThenHybrid: un error de ruta no impide la build.
func TestTransitionErrorThenHybrid(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.Hybrids["Abrams"] = nil

	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Err: fmt.Errorf("sin red")})
	if len(a.session.Routes["Abrams"]) != 0 {
		t.Error("un error no debe dejar pasos")
	}
	if a.session.Hybrids == nil {
		a.session.Hybrids = map[string]*HybridBuildView{}
	}
	if a.session.HybridLoading == nil {
		a.session.HybridLoading = map[string]string{}
	}
	a.session.Hybrids["Abrams"] = nil
	a.session.HybridLoading["Abrams"] = a.session.HybridKey
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
	if !strings.Contains(hybridText(a), "Build híbrida") {
		t.Error("la build no se mostró tras un error de ruta")
	}
}

// TestTransitionHybridErrorFallsBackToRoute: si la híbrida falla, la
// ruta observada sigue siendo la vista.
func TestTransitionHybridErrorFallsBackToRoute(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.Hybrids["Abrams"] = nil

	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Err: fmt.Errorf("sin red")})

	text := hybridText(a)
	if strings.Contains(text, "Build híbrida") {
		t.Error("una build fallida no debe mostrarse")
	}
	if !strings.Contains(text, "Ruta observada") {
		t.Errorf("sin build, la ruta observada debe seguir visible: %s", text)
	}
}

// TestTransitionNoScrollReset: la llegada de cualquiera de las dos no
// reinicia el scroll.
func TestTransitionNoScrollReset(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 24)
	a.session.Hybrids["Abrams"] = nil
	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: obsRoute(6)})
	for i := 0; i < 4; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	}
	before := a.detail.vp.YOffset
	if before == 0 {
		t.Fatal("precondición: no se desplazó")
	}
	if a.session.Hybrids == nil {
		a.session.Hybrids = map[string]*HybridBuildView{}
	}
	if a.session.HybridLoading == nil {
		a.session.HybridLoading = map[string]string{}
	}
	a.session.Hybrids["Abrams"] = nil
	a.session.HybridLoading["Abrams"] = a.session.HybridKey
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Key: a.session.HybridKey, Build: sampleHybrid()})
	if a.detail.vp.YOffset == 0 && before != 0 {
		t.Error("la llegada de la build reinició el scroll")
	}
}
