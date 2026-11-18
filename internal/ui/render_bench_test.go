package ui

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"testing"

	"github.com/Eion21/mobatui/internal/imgrender"
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

// BenchmarkEtiquetaIncertidumbre mide calcular la etiqueta de muestra y
// la banda de Wilson. Es aritmética pura sobre un entero ya disponible:
// no debe añadir una allocación por frame ni tocar la red.
func BenchmarkEtiquetaIncertidumbre(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sampleSuffix(148)
	}
}

// BenchmarkBandaWilson mide el cálculo del intervalo, que solo se hace
// al abrir el popup (nunca por paso de la ruta).
func BenchmarkBandaWilson(b *testing.B) {
	st := fmtStats{Games: 148, Wins: 102}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = wilsonBand(st)
	}
}

// BenchmarkRenderRutaConEtiquetas mide la Ruta completa de Briar, que
// incluye pasos con muestra limitada (a diferencia de Kai'Sa).
func BenchmarkRenderRutaConEtiquetas(b *testing.B) {
	d := planDetailView(b, "briar", "Briar", 118, 60)
	d.tab = 1
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.routeViewA(plan)
	}
}

// BenchmarkIconoCompactoPorAncho mide la generación de UN icono nuevo
// (decodificación PNG + render) para cada ancho de la escala adaptativa.
// Es lo que cuesta la primera vez que se abre Ruta en ese tamaño.
func BenchmarkIconoCompactoPorAncho(b *testing.B) {
	for _, cols := range []int{smallArtCols, artColsMedium, artColsLarge} {
		b.Run(fmt.Sprintf("cols=%d", cols), func(b *testing.B) {
			raw, err := os.ReadFile("../../testdata/ddragon/3026.png")
			if err != nil {
				b.Skip(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				img, err := png.Decode(bytes.NewReader(raw))
				if err != nil {
					b.Fatal(err)
				}
				_ = imgrender.Render(img, cols, 1, panelBg)
			}
		})
	}
}

// BenchmarkRenderRuta118Icono6 mide pintar Ruta a 118 columnas, donde el
// icono es de 6 columnas (el caso más caro).
func BenchmarkRenderRuta118Icono6(b *testing.B) {
	d := planDetailView(b, "briar", "Briar", 118, 60)
	d.tab = 1
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.routeViewA(plan)
	}
}

// BenchmarkRenderAlt118Icono6 lo mismo para Alternativas.
func BenchmarkRenderAlt118Icono6(b *testing.B) {
	d := planDetailView(b, "briar", "Briar", 118, 60)
	d.tab = 2
	d.ensureTabArt()
	plan := d.lolPlan()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.alternativesViewB(plan)
	}
}

// BenchmarkIconColsFor mide la elección de ancho: debe ser trivial y sin
// asignaciones (se llama por icono y por frame).
func BenchmarkIconColsFor(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = iconColsFor(118)
	}
}
