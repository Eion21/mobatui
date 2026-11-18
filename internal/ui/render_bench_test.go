package ui

import (
	"testing"
)

// BenchmarkRenderRuta mide el render repetido de la pestaña Ruta.
func BenchmarkRenderRuta(b *testing.B) {
	d := planDetailView(b, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.routeViewA(plan)
	}
}

// BenchmarkRenderAlt mide lo mismo para Alt.
func BenchmarkRenderAlt(b *testing.B) {
	d := planDetailView(b, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 2
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.alternativesViewB(plan)
	}
}

// BenchmarkCambioPestanaRapido mide ciclar entre pestañas con el arte ya
// caliente: no debe re-renderizar iconos.
func BenchmarkCambioPestanaRapido(b *testing.B) {
	d := planDetailView(b, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	d.ensureTabArt()
	d.tab = 2
	d.ensureTabArt()
	antes := len(d.session.ItemArt)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.tab = 1 + i%2
		d.ensureTabArt()
	}
	b.StopTimer()
	if despues := len(d.session.ItemArt); despues != antes {
		b.Errorf("el arte creció de %d a %d: no debe re-renderizar", antes, despues)
	}
}

// BenchmarkRunasPreparacion mide preparar el arte de una página de runas
// (keystone + primarias + secundarias) desde un proveedor local.
func BenchmarkRunasPreparacion(b *testing.B) {
	d := runeDetailView(b, 118, 60)
	cb := d.CurrentBuild()
	ids := runePageIDs(cb)
	prov := newRuneArtProviderB(b, ids).over(d.session.Provider)
	d.session.Provider = prov
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.session.InvalidateRuneArt()
		d.ensureRuneArt()
	}
	b.StopTimer()
	if prov.calls() == 0 {
		b.Error("el proveedor debía consultarse")
	}
}

// BenchmarkRunasRenderCaliente mide pintar Runas con el arte ya en caché.
func BenchmarkRunasRenderCaliente(b *testing.B) {
	d := runeDetailView(b, 118, 60)
	cb := d.CurrentBuild()
	d.session.Provider = newRuneArtProviderB(b, runePageIDs(cb)).over(d.session.Provider)
	d.ensureRuneArt()
	antes := len(d.session.RuneArt)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.runesView()
	}
	b.StopTimer()
	if len(d.session.RuneArt) != antes {
		b.Errorf("el arte creció: %d → %d", antes, len(d.session.RuneArt))
	}
}

// BenchmarkCambioRunasAOtras mide alternar entre Runas y otra pestaña.
func BenchmarkCambioRunasAOtras(b *testing.B) {
	d := runeDetailView(b, 118, 60)
	cb := d.CurrentBuild()
	d.session.Provider = newRuneArtProviderB(b, runePageIDs(cb)).over(d.session.Provider)
	d.ensureRuneArt()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.tab = 4
		d.ensureTabArt()
		d.tab = 0
		d.ensureTabArt()
	}
}

// BenchmarkCandidatosGeneracion mide derivar los candidatos del plan ya
// calculado (sin reparsear Qwik ni recalcular ranking).
func BenchmarkCandidatosGeneracion(b *testing.B) {
	d := planDetailView(b, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	plan := d.lolPlan()
	if len(plan.CandidatesFor()) == 0 {
		b.Skip("sin candidatos")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = plan.CandidatesFor()
	}
}

// BenchmarkCandidatosRender mide pintar la vista de Ruta con la sección
// de candidatos incluida.
func BenchmarkCandidatosRender(b *testing.B) {
	d := planDetailView(b, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	d.tab = 1
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.routeViewA(plan)
	}
}
