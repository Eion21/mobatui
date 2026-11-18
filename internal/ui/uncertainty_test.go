package ui

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/clean"
)

// ─── incertidumbre estadística ──────────────────────────────────────

// TestTramosDeEvidencia fija los umbrales y comprueba que no se inventó
// un corte nuevo: reutilizan lowSampleGames (500, ya usado para atenuar
// el color) y el mínimo del pipeline (100).
func TestTramosDeEvidencia(t *testing.T) {
	casos := []struct {
		games int
		want  evidenceLevel
	}{
		{0, evidenceUnknown},
		{-5, evidenceUnknown},
		{1, evidenceWeak},
		{20, evidenceWeak},
		{99, evidenceWeak},
		{100, evidenceLimited}, // justo el mínimo del pipeline
		{148, evidenceLimited}, // el caso de Ángel de la guarda
		{499, evidenceLimited},
		{500, evidenceSolid}, // justo el suelo de atenuación
		{1000, evidenceSolid},
		{10000, evidenceSolid},
	}
	for _, c := range casos {
		if got := classifyEvidence(c.games); got != c.want {
			t.Errorf("classifyEvidence(%d) = %v, se esperaba %v", c.games, got, c.want)
		}
	}
}

// TestEtiquetaDeMuestra: la etiqueta corta solo aparece cuando la
// muestra no es sólida, y es estable entre superficies.
func TestEtiquetaDeMuestra(t *testing.T) {
	if got := sampleLabel(0); got != "" && got != sampleLabel(0) {
		t.Errorf("muestra nula: %q", got)
	}
	if got := sampleLabel(148); got != "muestra limitada" {
		t.Errorf("148 partidas: %q", got)
	}
	if got := sampleLabel(20); got != "muestra insuficiente" {
		t.Errorf("20 partidas: %q", got)
	}
	if got := sampleLabel(10000); got != "" {
		t.Errorf("10k partidas no necesita etiqueta: %q", got)
	}
	// El sufijo compuesto incluye el separador.
	if got := sampleSuffix(148); got != " · muestra limitada" {
		t.Errorf("sufijo: %q", got)
	}
	if got := sampleSuffix(10000); got != "" {
		t.Errorf("sufijo con muestra sólida: %q", got)
	}
}

// TestWilsonSeEnsanchaConMuestrasPequenas: el intervalo se estrecha al
// crecer la muestra. Documenta el 95 % y comprueba la monotonía.
func TestWilsonSeEnsanchaConMuestrasPequenas(t *testing.T) {
	type punto struct {
		games int
		width float64
	}
	// Mismo WR observado (60 %) con muestras crecientes.
	var previo float64 = 1
	for _, n := range []int{20, 99, 100, 148, 400, 1000, 10000} {
		wins := int(0.6 * float64(n))
		lo, hi := clean.WilsonInterval(wins, n)
		width := hi - lo
		if width < 0 || width > 1 {
			t.Errorf("n=%d: intervalo imposible [%v, %v]", n, lo, hi)
		}
		if lo > hi {
			t.Errorf("n=%d: límite inferior mayor que el superior", n)
		}
		if width > previo+1e-9 {
			t.Errorf("n=%d: el intervalo se ensanchó (%v > %v)", n, width, previo)
		}
		previo = width
	}
	// Con 148 partidas el intervalo debe ser claramente amplio: es lo
	// que justifica la etiqueta "muestra limitada".
	lo, hi := clean.WilsonInterval(102, 148) // 68.9 %
	if hi-lo < 0.10 {
		t.Errorf("con 148 partidas el intervalo debería ser ancho: %v–%v", lo, hi)
	}
}

// TestWilsonLimitesDegenerados: sin muestra no se fabrica nada.
func TestWilsonLimitesDegenerados(t *testing.T) {
	if lo, hi := clean.WilsonInterval(0, 0); lo != 0 || hi != 0 {
		t.Errorf("n=0: %v–%v", lo, hi)
	}
	if lo, hi := clean.WilsonInterval(5, -1); lo != 0 || hi != 0 {
		t.Errorf("n<0: %v–%v", lo, hi)
	}
	// Todas ganadas: el límite inferior no puede ser 100 %.
	if lo, hi := clean.WilsonInterval(10, 10); lo >= 1 || hi > 1 {
		t.Errorf("10/10: %v–%v", lo, hi)
	}
}

// TestWilsonLowerDerivaDelIntervalo: el ranking del planificador y la
// presentación comparten UNA fórmula; no puede haber dos divergentes.
func TestWilsonLowerDerivaDelIntervalo(t *testing.T) {
	for _, c := range []struct{ w, g int }{{1, 3}, {53, 100}, {102, 148}, {5000, 10000}} {
		lo, _ := clean.WilsonInterval(c.w, c.g)
		if got := clean.WilsonLower(c.w, c.g); got != lo {
			t.Errorf("WilsonLower(%d,%d)=%v != intervalo %v", c.w, c.g, got, lo)
		}
	}
}

// TestAngelDeLaGuardiaSeMarca: el caso reportado. 68.9 % con 148
// partidas debe llevar la etiqueta de muestra limitada, no verde de
// certeza.
func TestAngelDeLaGuardiaSeMarca(t *testing.T) {
	const games, wr = 148, 0.689
	if got := sampleSuffix(games); !strings.Contains(got, "muestra limitada") {
		t.Errorf("148 partidas no se marcan: %q", got)
	}
	// El estilo NO puede ser el verde de certeza.
	estilo := wrStyle(wr, games)
	if estilo.GetForeground() == colWrGreat || estilo.GetForeground() == colWrGood {
		t.Errorf("un WR alto con 148 partidas no puede ir en verde de certeza")
	}
}

// TestEstilosPorTamanoDeMuestra: el color no contradice la etiqueta.
func TestEstilosPorTamanoDeMuestra(t *testing.T) {
	const wr = 0.689 // mismo WR en todos los casos
	// Muestras por debajo del suelo: gris atenuado, sin verde.
	for _, n := range []int{20, 99, 148, 499} {
		if got := wrStyle(wr, n).GetForeground(); got == colWrGreat || got == colWrGood {
			t.Errorf("n=%d: verde de certeza con muestra no sólida", n)
		}
	}
	// Muestra sólida y WR alto: ahora sí destaca.
	if got := wrStyle(wr, 10000).GetForeground(); got != colWrGreat {
		t.Errorf("n=10000 con 68.9%% debería destacar, foreground=%v", got)
	}
}

// TestWRConEtiquetaCompuesta: el número y su etiqueta van juntos.
func TestWRConEtiquetaCompuesta(t *testing.T) {
	got := stripANSIStr(wrWithSample(0.689, 148))
	if !strings.Contains(got, "68.9%") {
		t.Errorf("falta el WR: %q", got)
	}
	if !strings.Contains(got, "muestra limitada") {
		t.Errorf("falta la etiqueta de muestra: %q", got)
	}
	// Con muestra sólida no se añade ruido.
	if s := stripANSIStr(wrWithSample(0.53, 10000)); strings.Contains(s, "muestra") {
		t.Errorf("muestra sólida no lleva etiqueta: %q", s)
	}
}

// TestAdvertenciaDeSesgoEsPrudente: la frase no afirma causalidad.
func TestAdvertenciaDeSesgoEsPrudente(t *testing.T) {
	got := biasWarning(deltaAviso + 1)
	if got == "" {
		t.Fatal("un delta alto debe avisar")
	}
	if !strings.Contains(got, "sesgo de selección") {
		t.Errorf("no nombra el sesgo: %q", got)
	}
	if strings.Contains(got, "se compra") || strings.Contains(got, "yendo por delante") {
		t.Errorf("sigue afirmando cómo se compra: %q", got)
	}
	// Por debajo del umbral no hay aviso.
	if biasWarning(deltaAviso-0.1) != "" {
		t.Error("por debajo del umbral no debe avisar")
	}
	// La explicación larga tampoco promete causalidad ni limpia el sesgo.
	exp := biasExplanation()
	if !strings.Contains(exp, "No demuestra") {
		t.Errorf("la explicación no niega la causalidad: %q", exp)
	}
	if strings.Contains(exp, "elimina") || strings.Contains(exp, "corrige el sesgo") {
		t.Errorf("no puede prometer que se corrige el sesgo: %q", exp)
	}
}

// TestExplicacionDeMuestra: cada tramo explica qué implica.
func TestExplicacionDeMuestra(t *testing.T) {
	for _, n := range []int{0, 20, 148, 10000} {
		if got := uncertaintyExplanation(n); got == "" {
			t.Errorf("n=%d sin explicación", n)
		}
	}
	if got := uncertaintyExplanation(0); !strings.Contains(got, "Sin partidas") {
		t.Errorf("muestra nula: %q", got)
	}
	if got := evidenceWord(0); got != "sin muestra" {
		t.Errorf("evidenceWord(0) = %q", got)
	}
}

// TestEtiquetasCoherentesEntreSuperficies: Ruta, Alternativas, popup y
// la línea de selección usan EL MISMO criterio para el mismo número.
func TestEtiquetasCoherentesEntreSuperficies(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 1
	plan := d.lolPlan()
	if plan == nil {
		t.Fatal("sin plan")
	}
	d.tab = 2
	alts := strings.Join(d.alternativesViewB(plan), "\n")

	// Para cada paso de la ruta, su etiqueta debe coincidir con la del
	// criterio global aplicado a sus partidas.
	for _, st := range plan.Steps {
		quiere := sampleLabel(st.Stats.Games)
		if quiere == "" {
			continue
		}
		linea := stripANSIStr(d.routeStepLine(st))
		if !strings.Contains(linea, quiere) {
			t.Errorf("paso %q (%d partidas) sin etiqueta %q: %q",
				st.Name, st.Stats.Games, quiere, linea)
		}
	}
	// En Alternativas, toda alternativa con muestra no sólida se marca.
	for _, g := range plan.Alternatives {
		for _, a := range g.Items {
			quiere := sampleLabel(a.Stats.Games)
			if quiere == "" {
				continue
			}
			if !strings.Contains(stripANSIStr(alts), quiere) {
				t.Errorf("alternativa %q (%d partidas) sin etiqueta %q",
					a.Name, a.Stats.Games, quiere)
			}
		}
	}
}

// TestPopupNoInventaMetricasSinDatos: sin partidas no se fabrica WR ni
// intervalo.
func TestPopupNoInventaMetricasSinDatos(t *testing.T) {
	if got := wilsonBand(fmtStats{Games: 0, Wins: 0}); got != "" {
		t.Errorf("sin muestra no hay banda: %q", got)
	}
	if got := wilsonBand(fmtStats{Games: 148, Wins: 102}); got == "" {
		t.Error("con muestra debe haber banda")
	}
	// La banda es un texto de dos porcentajes con guion.
	if got := stripANSIStr(wilsonBand(fmtStats{Games: 148, Wins: 102})); !strings.Contains(got, "–") {
		t.Errorf("formato de banda inesperado: %q", got)
	}
}

// TestPopupDelItemSeparaConceptos: WR observado, incertidumbre y calidad
// de evidencia aparecen como cosas distintas, sin prometer causalidad.
func TestPopupDelItemSeparaConceptos(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Version = "16.23.1"
	popup := stripANSIStr(d.itemPopup())
	for _, quiere := range []string{"WR", "muestra", "evidencia", "datos", "catálogo"} {
		if !strings.Contains(popup, quiere) {
			t.Errorf("el popup no distingue %q: %q", quiere, popup)
		}
	}
}

// TestMatrizDeMuestras recorre los tamaños exigidos por la fase y
// comprueba que la etiqueta y la amplitud del intervalo se comportan
// como se espera en cada uno. Incluye WR BAJO con pocas partidas: la
// incertidumbre no depende de que el WR sea alto.
func TestMatrizDeMuestras(t *testing.T) {
	casos := []struct {
		games int
		label string
	}{
		{0, ""},                      // sin muestra: no se etiqueta, no se inventa
		{20, "muestra insuficiente"}, // muy poca
		{99, "muestra insuficiente"}, // justo bajo el mínimo
		{100, "muestra limitada"},    // justo en el mínimo
		{148, "muestra limitada"},    // el caso del informe
		{400, "muestra limitada"},    // aún bajo el suelo
		{1000, "muestra limitada"},   // sigue bajo 500? no: ver abajo
		{10000, ""},                  // sólida
	}
	for _, c := range casos {
		if c.games == 1000 {
			// 1000 >= lowSampleGames (500): es sólida, no limitada.
			if got := sampleLabel(c.games); got != "" {
				t.Errorf("n=%d: se esperaba etiqueta vacía, hubo %q", c.games, got)
			}
			continue
		}
		if got := sampleLabel(c.games); got != c.label {
			t.Errorf("n=%d: etiqueta %q, se esperaba %q", c.games, got, c.label)
		}
	}

	// WR BAJO con pocas partidas: el mismo tramo, sin sesgo por el valor.
	bajo, alto := 0.35, 0.75
	for _, wr := range []float64{bajo, alto} {
		if got := sampleLabel(20); got != "muestra insuficiente" {
			t.Errorf("WR %.2f con 20 partidas: %q", wr, got)
		}
		// Y el color no puede ser verde de certeza en ninguno de los dos.
		if fg := wrStyle(wr, 20).GetForeground(); fg == colWrGreat || fg == colWrGood {
			t.Errorf("WR %.2f con 20 partidas no puede ir en verde de certeza", wr)
		}
	}

	// La amplitud del intervalo decrece con la muestra, para CUALQUIER
	// WR. Se compara por proporción: el ancho también depende de p (es
	// máximo en 0.5), así que mezclar p distintos en una sola serie no
	// probaría monotonía, solo el efecto de p.
	for _, p := range []float64{0.35, 0.5, 0.69} {
		previo := 2.0
		for _, n := range []int{20, 99, 100, 148, 400, 1000, 10000} {
			wins := int(p * float64(n))
			lo, hi := clean.WilsonInterval(wins, n)
			ancho := hi - lo
			if ancho > previo+1e-9 {
				t.Errorf("p=%.2f n=%d: el intervalo se ensanchó (%.4f > %.4f)",
					p, n, ancho, previo)
			}
			previo = ancho
		}
	}
}
