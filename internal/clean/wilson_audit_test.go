package clean

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// ─── auditoría: WilsonInterval vs implementación original ───────────
//
// En la Fase 1 se refactorizó clean.WilsonLower para que derivara de
// WilsonInterval (una sola fórmula en el proyecto). El ranking del
// planificador depende POR COMPLETO de ese valor, así que hay que
// demostrar —no asumir— que el límite inferior es idéntico bit a bit.

// z95Oraculo es el valor histórico del commit f8de376, escrito como
// LITERAL y no tomado del const z95.
//
// Deliberado: si el oráculo leyera el mismo const que la implementación,
// subir o bajar z95 movería ambos lados y la auditoría pasaría en verde
// sin detectar nada. Se comprobó por mutación: con z95=1.95 la versión
// anterior de este test NO fallaba. Ese acoplamiento era un punto ciego.
const z95Oraculo = 1.96

// wilsonLowerOriginal es la implementación del commit f8de376, copiada
// VERBATIM salvo el literal de z (ver z95Oraculo). Es el oráculo:
// cualquier divergencia de la versión nueva contra esta función es una
// regresión del ranking.
func wilsonLowerOriginal(wins, games int) float64 {
	if games <= 0 {
		return 0
	}
	n := float64(games)
	p := float64(wins) / n
	z2 := z95Oraculo * z95Oraculo
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	margin := z95Oraculo * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (centre - margin) / denom
}

// wilsonIntervalOriginal es el oráculo del intervalo completo, derivado
// de la misma expresión original (no de WilsonInterval).
func wilsonIntervalOriginal(wins, games int) (lo, hi float64) {
	if games <= 0 {
		return 0, 0
	}
	n := float64(games)
	p := float64(wins) / n
	z2 := z95Oraculo * z95Oraculo
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	margin := z95Oraculo * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (centre - margin) / denom, (centre + margin) / denom
}

// Desglose EXACTO de la matriz. Los totales se verifican en
// TestConteoDeLaMatriz, así que estos números no pueden derivar de la
// realidad sin que el test falle.
//
//	(a) exhaustivo games 0..400, wins 0..games ........ 80.601
//	(b) wins > games, games 0..60, extra 1..5 .........    305
//	(c) negativos .....................................      4
//	(d) muestras grandes 18 games × 15 ratios ..........    270
//	(e) límites duros .................................      9
//	                                        TOTAL ...... 81.189
//
// Nota sobre (b): son 305 casos, de los cuales 300 dan NaN (games>0 y
// wins>games) y 5 caen en el guard temprano por games==0. El bloque NO
// son 300: ese es solo el subconjunto con NaN. Confundirlos daba
// 81.184, que es aritmética incorrecta, no un fallo de la fórmula.
const (
	combosExhaustivo = 80_601
	combosWinsMayor  = 305
	combosNegativos  = 4
	combosGrandes    = 270
	combosLimites    = 9
	combosTotal      = combosExhaustivo + combosWinsMayor + combosNegativos +
		combosGrandes + combosLimites // 81.189
)

// combosWilson genera la matriz determinista de casos: exhaustiva para
// muestras pequeñas, logarítmica para grandes, y los límites.
func combosWilson() [][2]int {
	var out [][2]int

	// (a) Exhaustivo 0..400: 80.601 combinaciones, incluidos wins=0 y
	// wins=games. (wins>games NO entra aquí: wins nunca supera games en
	// este bucle; eso es el bloque (b).)
	for games := 0; games <= 400; games++ {
		for wins := 0; wins <= games; wins++ {
			out = append(out, [2]int{wins, games})
		}
	}
	// (b) wins > games para muestras pequeñas: 61 games × 5 extras = 305.
	// No debe desbordar ni dar NaN; los datos de origen podrían traerlo,
	// así que la función debe ser total.
	for games := 0; games <= 60; games++ {
		for extra := 1; extra <= 5; extra++ {
			out = append(out, [2]int{games + extra, games})
		}
	}
	// (c) Negativos: games < 0 y wins < 0 caen en el guard temprano.
	out = append(out, [2]int{0, -1}, [2]int{5, -1}, [2]int{-1, -1}, [2]int{-100, -100})
	// (d) Muestras grandes, en escala logarítmica y potencias de 10.
	for _, games := range []int{
		401, 499, 500, 501, 999, 1000, 1001, 2048, 4096,
		10_000, 12_345, 65_535, 65_536, 100_000,
		1_000_000, 1_000_001, 10_000_000, 100_000_000,
	} {
		for _, ratio := range []float64{
			0, 0.0001, 0.01, 0.1, 0.25, 0.3333, 0.49, 0.5,
			0.51, 0.6667, 0.75, 0.9, 0.99, 0.9999, 1,
		} {
			wins := int(ratio * float64(games))
			if wins > games {
				wins = games
			}
			out = append(out, [2]int{wins, games})
		}
	}
	// (e) Casos límite duros: 1/1, 1/2, 39/40, 399/400, 1/10000.
	out = append(out,
		[2]int{1, 1}, [2]int{0, 1}, [2]int{1, 2}, [2]int{1, 3},
		[2]int{39, 40}, [2]int{399, 400}, [2]int{1, 10_000},
		[2]int{9_999, 10_000}, [2]int{5_000, 10_000},
	)
	return out
}

// TestConteoDeLaMatriz ancla el desglose documentado contra la matriz
// real, bloque por bloque. Evita que los comentarios (y el informe)
// vuelvan a citar un total que no cuadra.
func TestConteoDeLaMatriz(t *testing.T) {
	var exhaustivo, winsMayor, negativos, grandes, limites int
	for games := 0; games <= 400; games++ {
		for wins := 0; wins <= games; wins++ {
			exhaustivo++
		}
	}
	for games := 0; games <= 60; games++ {
		for extra := 1; extra <= 5; extra++ {
			winsMayor++
		}
	}
	negativos = 4
	grandes = 18 * 15
	limites = 9

	if exhaustivo != combosExhaustivo {
		t.Errorf("bloque (a) = %d, documentado %d", exhaustivo, combosExhaustivo)
	}
	if winsMayor != combosWinsMayor {
		t.Errorf("bloque (b) = %d, documentado %d", winsMayor, combosWinsMayor)
	}
	if negativos != combosNegativos {
		t.Errorf("bloque (c) = %d, documentado %d", negativos, combosNegativos)
	}
	if grandes != combosGrandes {
		t.Errorf("bloque (d) = %d, documentado %d", grandes, combosGrandes)
	}
	if limites != combosLimites {
		t.Errorf("bloque (e) = %d, documentado %d", limites, combosLimites)
	}
	suma := exhaustivo + winsMayor + negativos + grandes + limites
	if suma != combosTotal {
		t.Errorf("suma de bloques = %d, constante = %d", suma, combosTotal)
	}
	if real := len(combosWilson()); real != combosTotal {
		t.Errorf("combosWilson() = %d, constante = %d", real, combosTotal)
	}
	t.Logf("desglose verificado: %d+%d+%d+%d+%d = %d",
		exhaustivo, winsMayor, negativos, grandes, limites, suma)

	// Subconjunto NaN del bloque (b): 300, no 305. Se deja constancia
	// para que nadie vuelva a confundir el total del bloque con sus NaN.
	nan := 0
	for games := 1; games <= 60; games++ {
		for extra := 1; extra <= 5; extra++ {
			if math.IsNaN(wilsonLowerOriginal(games+extra, games)) {
				nan++
			}
		}
	}
	if nan != 300 {
		t.Errorf("NaN en (b) = %d, se esperaban 300", nan)
	}
}

// TestZ95EsElValorDocumentado: el const compartido debe seguir siendo
// 1.96 (IC del 95 %). Si alguien lo cambiara, el ranking se movería en
// silencio; este test lo ancla contra el literal histórico.
func TestZ95EsElValorDocumentado(t *testing.T) {
	if z95 != z95Oraculo {
		t.Fatalf("z95 = %v, se esperaba %v (IC del 95%%); "+
			"cambiarlo altera el ranking del planificador", z95, z95Oraculo)
	}
}

// TestWilsonLowerIdenticoBitABit es la auditoría principal: el límite
// inferior nuevo y el original deben coincidir EXACTAMENTE (mismos bits
// IEEE-754) en todas las combinaciones.
func TestWilsonLowerIdenticoBitABit(t *testing.T) {
	combos := combosWilson()
	if len(combos) != combosTotal {
		t.Fatalf("la matriz tiene %d combinaciones, se documentaron %d",
			len(combos), combosTotal)
	}
	if len(combos) < 50_000 {
		t.Fatalf("la matriz debe cubrir >=50.000 combinaciones, hay %d", len(combos))
	}

	var distintos, nanos int
	for _, c := range combos {
		wins, games := c[0], c[1]
		want := wilsonLowerOriginal(wins, games)
		got := WilsonLower(wins, games)

		// NaN se compara aparte: NaN != NaN.
		if math.IsNaN(want) || math.IsNaN(got) {
			nanos++
			if math.IsNaN(want) != math.IsNaN(got) {
				distintos++
				if distintos <= 10 {
					t.Errorf("wins=%d games=%d: NaN original=%v nuevo=%v",
						wins, games, math.IsNaN(want), math.IsNaN(got))
				}
			}
			continue
		}
		if math.Float64bits(want) != math.Float64bits(got) {
			distintos++
			// Solo se imprimen los primeros: si diverge, divergen miles.
			if distintos <= 10 {
				t.Errorf("wins=%d games=%d: original=%.17g nuevo=%.17g (bits %x vs %x)",
					wins, games, want, got,
					math.Float64bits(want), math.Float64bits(got))
			}
		}
	}
	if nanos > 0 {
		t.Logf("casos con NaN (esperado solo en wins>games inexistente): %d", nanos)
	}
	if distintos > 0 {
		t.Fatalf("%d de %d combinaciones difieren del original", distintos, len(combos))
	}
	t.Logf("verificadas %d combinaciones, 0 diferencias bit a bit", len(combos))
}

// TestWilsonIntervalCoherenteConOriginal verifica el límite SUPERIOR
// (nuevo, no existía antes) contra la expresión original, y que el
// límite inferior del intervalo es el mismo valor que WilsonLower.
func TestWilsonIntervalCoherenteConOriginal(t *testing.T) {
	for _, c := range combosWilson() {
		wins, games := c[0], c[1]
		wantLo, wantHi := wilsonIntervalOriginal(wins, games)
		gotLo, gotHi := WilsonInterval(wins, games)

		if !igualFloat(wantLo, gotLo) {
			t.Fatalf("lo wins=%d games=%d: %.17g != %.17g", wins, games, wantLo, gotLo)
		}
		if !igualFloat(wantHi, gotHi) {
			t.Fatalf("hi wins=%d games=%d: %.17g != %.17g", wins, games, wantHi, gotHi)
		}
		// WilsonLower debe ser EXACTAMENTE el límite inferior.
		if lower := WilsonLower(wins, games); !igualFloat(lower, gotLo) {
			t.Fatalf("WilsonLower != lo en wins=%d games=%d", wins, games)
		}
		// El intervalo no puede invertirse.
		if !math.IsNaN(gotLo) && gotLo > gotHi {
			t.Fatalf("intervalo invertido wins=%d games=%d: %v > %v",
				wins, games, gotLo, gotHi)
		}
		// LÍMITE MATEMÁTICO [0,1] con tolerancia de redondeo.
		//
		// Hallazgo de la auditoría: con wins=0 la fórmula produce un
		// límite inferior negativo del orden de 1e-17 (-2.2e-17 en
		// games=15 por cancelación catastrófica: centre-margin resta dos
		// valores casi iguales). NO es una regresión: el original da
		// exactamente el mismo valor (verificado bit a bit), ocurre solo
		// con wins=0 y nunca con wins>0. Se tolera el epsilon en vez de
		// recortar el valor, porque recortarlo cambiaría los bits y la
		// auditoría exige identidad exacta con el original.
		const eps = 1e-12
		if gotLo < -eps || gotHi > 1+eps {
			t.Fatalf("intervalo fuera de [0,1] más allá del redondeo wins=%d games=%d: [%v, %v]",
				wins, games, gotLo, gotHi)
		}
	}
}

// igualFloat compara flotantes tratando NaN como igual a NaN.
func igualFloat(a, b float64) bool {
	if math.IsNaN(a) || math.IsNaN(b) {
		return math.IsNaN(a) == math.IsNaN(b)
	}
	return math.Float64bits(a) == math.Float64bits(b)
}

// itemWilson es un candidato con su puntuación, para probar el orden.
type itemWilson struct {
	id    int
	wins  int
	games int
	// nombre solo para los mensajes de fallo.
	nombre string
}

// TestOrdenDeRankingIdentico replica el ordenamiento REAL del pipeline
// (sort.SliceStable por Wilson descendente, como en rankItems/coreItems)
// usando ambas implementaciones y comprueba que la permutación es la
// misma. Esto es lo que de verdad importa: no el valor aislado, sino qué
// objeto queda por delante de cuál.
func TestOrdenDeRankingIdentico(t *testing.T) {
	// Muestras realistas del dominio: mezcla de tamaños y WR, con
	// muchos empates de partidas (que es donde un establo importa).
	var items []itemWilson
	id := 1000
	for _, games := range []int{
		1, 2, 3, 5, 10, 20, 50, 99, 100, 148, 200, 351, 400,
		464, 500, 557, 870, 888, 1000, 1057, 1234, 5000, 10_000,
		50_000, 100_000, 335_558,
	} {
		for _, wr := range []float64{
			0.30, 0.35, 0.40, 0.45, 0.48, 0.50, 0.5175,
			0.53, 0.55, 0.58, 0.60, 0.63, 0.689, 0.75, 0.80,
		} {
			wins := int(wr * float64(games))
			items = append(items, itemWilson{
				id: id, wins: wins, games: games,
				nombre: fmt.Sprintf("item%d", id),
			})
			id++
		}
	}
	// Muchos empates EXACTOS de (wins, games) con distinto id: el orden
	// relativo debe preservarse por estabilidad en ambas versiones.
	for i := 0; i < 50; i++ {
		items = append(items, itemWilson{id: id, wins: 102, games: 148,
			nombre: fmt.Sprintf("empate%d", i)})
		id++
	}
	if len(items) < 400 {
		t.Fatalf("hacen falta más candidatos, hay %d", len(items))
	}

	// Orden con la implementación ORIGINAL.
	orig := append([]itemWilson(nil), items...)
	sort.SliceStable(orig, func(i, j int) bool {
		return wilsonLowerOriginal(orig[i].wins, orig[i].games) >
			wilsonLowerOriginal(orig[j].wins, orig[j].games)
	})

	// Orden con la implementación NUEVA.
	nuevo := append([]itemWilson(nil), items...)
	sort.SliceStable(nuevo, func(i, j int) bool {
		return WilsonLower(nuevo[i].wins, nuevo[i].games) >
			WilsonLower(nuevo[j].wins, nuevo[j].games)
	})

	for i := range orig {
		if orig[i].id != nuevo[i].id {
			t.Fatalf("posición %d: original=%s nuevo=%s", i, orig[i].nombre, nuevo[i].nombre)
		}
	}

	// Y se comprueba que el orden es realmente no trivial (hay variedad).
	visto := map[int]bool{}
	for _, it := range nuevo[:min(20, len(nuevo))] {
		visto[it.games] = true
	}
	if len(visto) < 5 {
		t.Errorf("el orden de prueba es degenerado: %d tamaños distintos en el top", len(visto))
	}
	t.Logf("orden idéntico sobre %d candidatos (%d empates exactos incluidos)",
		len(items), 50)
}

// TestEmpatesExactosNoSeReordenan localiza pares con puntuación
// EXACTAMENTE igual y confirma que la versión nueva los ve igual de
// empatados (si uno cambiara de valor, el desempate por WR del
// planificador podría cambiar de ganador).
func TestEmpatesExactosNoSeReordenan(t *testing.T) {
	// Pares que producen el mismo WilsonLower por construcción: misma
	// proporción y mismo tamaño.
	pares := [][2][2]int{
		{{102, 148}, {102, 148}},
		{{53, 100}, {53, 100}},
		{{1, 3}, {1, 3}},
		{{0, 20}, {0, 20}},
		{{20, 20}, {20, 20}},
		{{256561, 495000}, {256561, 495000}},
	}
	for _, par := range pares {
		a, b := par[0], par[1]
		origA := wilsonLowerOriginal(a[0], a[1])
		origB := wilsonLowerOriginal(b[0], b[1])
		newA := WilsonLower(a[0], a[1])
		newB := WilsonLower(b[0], b[1])

		// Si el original los veía empatados, el nuevo también.
		if origA == origB && newA != newB {
			t.Errorf("%v vs %v: el original empata y el nuevo no (%.17g vs %.17g)",
				a, b, newA, newB)
		}
		if !igualFloat(origA, newA) || !igualFloat(origB, newB) {
			t.Errorf("%v/%v: el valor cambió", a, b)
		}
	}

	// Barrido: ningún par de la matriz cambia su relación de orden
	// (menor/igual/mayor) entre versiones.
	combos := combosWilson()
	// Submuestra determinista para no ser O(n²) sobre 80k.
	paso := max(1, len(combos)/700)
	var prev [2]int
	prevOK := false
	for i := 0; i < len(combos); i += paso {
		c := combos[i]
		if prevOK {
			o1 := wilsonLowerOriginal(prev[0], prev[1])
			o2 := wilsonLowerOriginal(c[0], c[1])
			n1 := WilsonLower(prev[0], prev[1])
			n2 := WilsonLower(c[0], c[1])
			if signo(o1-o2) != signo(n1-n2) {
				t.Fatalf("relación de orden cambiada entre %v y %v", prev, c)
			}
		}
		prev, prevOK = c, true
	}
}

// signo devuelve -1, 0 o 1; NaN cuenta como 0.
func signo(x float64) int {
	switch {
	case math.IsNaN(x), x == 0:
		return 0
	case x > 0:
		return 1
	default:
		return -1
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
