package ui

// Auditoría de la vigencia de Session.Hybrids: reutilizar lo válido,
// descartar lo obsoleto. Invalidar runsMemo NO basta.

import (
	"testing"
)

// TestHybridReusedWhenKeyMatches: con la misma clave, no se recalcula.
func TestHybridReusedWhenKeyMatches(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = "patch-1|"
	b := sampleHybrid()
	b.Key = "patch-1|"
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": b}

	if cmd := hybridCmdFor(a); cmd != nil {
		t.Error("con la clave vigente no debe recalcularse")
	}
}

// TestHybridInvalidatedOnPatchChange: un parche nuevo descarta la build.
func TestHybridInvalidatedOnPatchChange(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = "patch-1|"
	b := sampleHybrid()
	b.Key = "patch-1|"
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": b}

	// Cambia el parche: la clave vigente cambia.
	a.session.HybridKey = "patch-2|"
	if cmd := hybridCmdFor(a); cmd == nil {
		t.Fatal("un parche nuevo debe forzar el recálculo")
	}
}

// TestHybridInvalidatedOnFilterChange: un filtro que afecte a la
// generación también invalida.
func TestHybridInvalidatedOnFilterChange(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = hybridKeyFor("patch-1", "")
	b := sampleHybrid()
	b.Key = a.session.HybridKey
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": b}

	a.session.HybridKey = hybridKeyFor("patch-1", "minShare=0.4")
	if cmd := hybridCmdFor(a); cmd == nil {
		t.Fatal("un cambio de filtros debe forzar el recálculo")
	}
}

// TestHybridInFlightNotDuplicated: una carga en curso no se relanza.
func TestHybridInFlightNotDuplicated(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = "patch-1|"
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": nil} // en curso

	if cmd := hybridCmdFor(a); cmd != nil {
		t.Error("no debe relanzarse una carga en curso")
	}
}

// TestHybridKeyStampedOnArrival: la build que llega se marca con la
// clave vigente en ese momento.
func TestHybridKeyStampedOnArrival(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = "patch-9|"
	a = routeLoadingUpdate(a, hybridLoadedMsg{ChampKey: "Abrams", Build: sampleHybrid()})

	got := a.session.Hybrids["Abrams"]
	if got == nil {
		t.Fatal("la build no se guardó")
	}
	if got.Key != "patch-9|" {
		t.Errorf("clave = %q, esperado %q", got.Key, "patch-9|")
	}
}

// TestHybridStaleResultDiscarded: una build vieja guardada no se sirve
// como si fuera actual.
func TestHybridStaleResultDiscarded(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	old := sampleHybrid()
	old.Key = "patch-viejo|"
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": old}
	a.session.HybridKey = "patch-nuevo|"

	// La vista no debe usar la build obsoleta.
	if cards, ok := a.detail.hybridCards(); ok {
		_ = cards
		t.Error("se sirvió una build obsoleta")
	}
}

// TestHybridKeyDiffersByPatchAndFilters: la clave distingue ambos.
func TestHybridKeyDiffersByPatchAndFilters(t *testing.T) {
	a := hybridKeyFor("p1", "")
	b := hybridKeyFor("p2", "")
	c := hybridKeyFor("p1", "f")
	if a == b || a == c || b == c {
		t.Error("claves distintas colisionan")
	}
}
