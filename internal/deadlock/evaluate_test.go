package deadlock

// Pruebas del evaluador estadístico (Fase 3). Fixtures sintéticos:
// reproducibles, sin red.

import (
	"math"
	"testing"
)

// mkRuns construye una muestra donde `items` se compra en la fracción
// indicada de partidas, a los minutos dados.
func mkRuns(total int, items map[int]struct {
	share float64
	min   int
	wins  float64
}) []Run {
	runs := make([]Run, 0, total)
	for i := 0; i < total; i++ {
		var evs []Ev
		win := false
		for id, spec := range items {
			// Determinista: los primeros share*total lo compran.
			n := int(spec.share * float64(total))
			if i < n {
				evs = append(evs, Ev{Item: id, T: spec.min * 60})
			}
		}
		// El win se decide por el ítem de menor id con wins declarado
		// (orden determinista: el mapa no lo es).
		bestID := 0
		for id, spec := range items {
			if spec.wins > 0 && (bestID == 0 || id < bestID) {
				bestID = id
			}
		}
		if bestID != 0 {
			win = float64(i)/float64(total) < items[bestID].wins
		}
		runs = append(runs, Run{Win: win, Evs: evs})
	}
	return runs
}

// TestEvaluatePopularBasicWins: un básico comprado en casi todas las
// partidas debe puntuar por encima de un objeto raro con mejor WR.
// Es la trampa que el diseño prohíbe: "mayor win rate = mejores
// resultados".
func TestEvaluatePopularBasicWins(t *testing.T) {
	runs := mkRuns(200, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.90, min: 5},  // básico popular
		2: {share: 0.15, min: 30}, // raro con mejor WR
	})
	// El raro gana más: forzar WR distinto por ítem requiere otra
	// construcción; aquí se comprueba el efecto de la popularidad con
	// el mismo WR de fondo.
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	if len(cands) < 2 {
		t.Fatalf("candidatos = %d, esperado 2", len(cands))
	}
	if cands[0].Item != 1 {
		t.Errorf("primero = %d, esperado el básico popular (1)", cands[0].Item)
	}
}

// TestEvaluateSmallSampleLowConfidence: una muestra pequeña no puede
// tener confianza alta, por muy bueno que sea su WR.
func TestEvaluateSmallSampleLowConfidence(t *testing.T) {
	// 25 partidas, el objeto aparece en 22 (share alto, N bajo).
	runs := mkRuns(25, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.88, min: 5},
	})
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	if len(cands) != 1 {
		t.Fatalf("candidatos = %d", len(cands))
	}
	if cands[0].Confidence >= 0.70 {
		t.Errorf("confianza %.2f con N=%d: no puede ser alta",
			cands[0].Confidence, cands[0].N)
	}
}

// TestEvaluateLargeSampleHighConfidence: con muestra grande y estable
// la confianza sube.
func TestEvaluateLargeSampleHighConfidence(t *testing.T) {
	runs := mkRuns(400, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.90, min: 5},
	})
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	if len(cands) != 1 {
		t.Fatalf("candidatos = %d", len(cands))
	}
	if cands[0].Confidence < 0.60 {
		t.Errorf("confianza %.2f con N=%d: demasiado baja",
			cands[0].Confidence, cands[0].N)
	}
}

// TestEvaluatePhaseDenominator: una compra tardía se mide contra las
// partidas que llegaron a LATE, no contra todas.
func TestEvaluatePhaseDenominator(t *testing.T) {
	// 100 partidas: 60 terminan pronto (última compra en min 8),
	// 40 llegan a late. El objeto tardío lo compran 20 de esas 40.
	var runs []Run
	for i := 0; i < 100; i++ {
		if i < 60 {
			runs = append(runs, Run{Evs: []Ev{{Item: 1, T: 5 * 60}, {Item: 9, T: 8 * 60}}})
			continue
		}
		evs := []Ev{{Item: 1, T: 5 * 60}, {Item: 9, T: 8 * 60}}
		if i < 80 {
			evs = append(evs, Ev{Item: 2, T: 30 * 60}) // tardío
		} else {
			evs = append(evs, Ev{Item: 9, T: 30 * 60})
		}
		runs = append(runs, Run{Evs: evs})
	}
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	var late *Candidate
	for i := range cands {
		if cands[i].Item == 2 {
			late = &cands[i]
		}
	}
	if late == nil {
		t.Fatal("el objeto tardío no está entre los candidatos")
	}
	// Share global: 20/100 = 0.20. ShareReached: 20/40 = 0.50.
	if math.Abs(late.Share-0.20) > 0.01 {
		t.Errorf("share global = %.2f, esperado 0.20", late.Share)
	}
	if late.ReachedN != 40 {
		t.Errorf("partidas que alcanzaron LATE = %d, esperado 40", late.ReachedN)
	}
	if math.Abs(late.ShareReached-0.50) > 0.01 {
		t.Errorf("share por fase = %.2f, esperado 0.50", late.ShareReached)
	}
}

// TestEvaluateBetaBinomialShrinksSmallSamples: la posterior de una
// muestra pequeña se acerca al prior del héroe más que la de una
// muestra grande con el mismo WR.
func TestEvaluateBetaBinomialShrinksSmallSamples(t *testing.T) {
	// Héroe al 50 %: prior centrado en 0.5.
	small := mkRuns(40, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.90, min: 5, wins: 0.9},
	})
	large := mkRuns(400, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.90, min: 5, wins: 0.9},
	})
	cs := Evaluate(small, EvalOptions{MinN: 20})
	cl := Evaluate(large, EvalOptions{MinN: 20})
	if len(cs) == 0 || len(cl) == 0 {
		t.Fatal("faltan candidatos")
	}
	// La muestra pequeña se encoge hacia 0.5 más que la grande.
	distSmall := math.Abs(cs[0].Posterior - 0.5)
	distLarge := math.Abs(cl[0].Posterior - 0.5)
	if distSmall >= distLarge {
		t.Errorf("la posterior pequeña (%.3f) no se encogió más que la grande (%.3f)",
			cs[0].Posterior, cl[0].Posterior)
	}
}

// TestEvaluatePosteriorNotCausal: un WR alto no produce por sí solo una
// confianza alta ni un score dominante.
func TestEvaluatePosteriorNotCausal(t *testing.T) {
	runs := mkRuns(100, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.30, min: 25, wins: 1.0}, // 100 % WR, muestra pequeña
		2: {share: 0.95, min: 5},             // básico
	})
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	if len(cands) < 2 {
		t.Fatalf("candidatos = %d", len(cands))
	}
	if cands[0].Item != 2 {
		t.Errorf("primero = %d: un WR del 100%% con muestra pequeña no debe ganar", cands[0].Item)
	}
}

// TestEvaluateDeterministic: mismo input, mismo output.
func TestEvaluateDeterministic(t *testing.T) {
	runs := mkRuns(120, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.8, min: 5}, 2: {share: 0.5, min: 15}, 3: {share: 0.3, min: 25},
	})
	a := Evaluate(runs, EvalOptions{MinN: 20})
	b := Evaluate(runs, EvalOptions{MinN: 20})
	if len(a) != len(b) {
		t.Fatalf("longitudes distintas: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Item != b[i].Item || a[i].Score != b[i].Score {
			t.Errorf("paso %d difiere: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// TestEvaluateMinNFilter: los objetos con muestra insuficiente no
// entran como candidatos.
func TestEvaluateMinNFilter(t *testing.T) {
	runs := mkRuns(200, map[int]struct {
		share float64
		min   int
		wins  float64
	}{
		1: {share: 0.9, min: 5},
		2: {share: 0.05, min: 25}, // 10 partidas
	})
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	for _, c := range cands {
		if c.Item == 2 {
			t.Errorf("objeto con N=%d pasó el filtro MinN=20", c.N)
		}
	}
}

// TestEvaluateEmpty: sin muestra no hay candidatos ni pánico.
func TestEvaluateEmpty(t *testing.T) {
	if got := Evaluate(nil, EvalOptions{}); len(got) != 0 {
		t.Errorf("esperado vacío, hay %d", len(got))
	}
}

// TestEvaluateConsistency: un objeto presente en una sola mitad de la
// muestra tiene consistencia baja.
func TestEvaluateConsistency(t *testing.T) {
	var runs []Run
	for i := 0; i < 100; i++ {
		evs := []Ev{{Item: 1, T: 5 * 60}}
		if i >= 50 {
			evs = append(evs, Ev{Item: 2, T: 6 * 60}) // solo en la 2.ª mitad
		}
		runs = append(runs, Run{Evs: evs})
	}
	cands := Evaluate(runs, EvalOptions{MinN: 20})
	for _, c := range cands {
		if c.Item == 2 {
			if c.Consistency > 0.1 {
				t.Errorf("consistencia = %.2f para un objeto de una sola mitad", c.Consistency)
			}
		}
		if c.Item == 1 {
			if c.Consistency < 0.9 {
				t.Errorf("consistencia = %.2f para un objeto en toda la muestra", c.Consistency)
			}
		}
	}
}

// TestConfidenceLabel: los umbrales son los documentados.
func TestConfidenceLabel(t *testing.T) {
	cases := map[float64]string{
		0.9: "alta", 0.7: "alta",
		0.6: "moderada", 0.45: "moderada",
		0.3: "insuficiente", 0: "insuficiente",
	}
	for v, want := range cases {
		if got := ConfidenceLabel(v); got != want {
			t.Errorf("ConfidenceLabel(%.2f) = %q, esperado %q", v, got, want)
		}
	}
}

// TestBetaQuantilesBounded: los cuantiles quedan en [0,1] y el
// intervalo contiene la media.
func TestBetaQuantilesBounded(t *testing.T) {
	for _, tc := range []struct{ a, b float64 }{
		{1, 1}, {10, 10}, {100, 90}, {2, 50}, {500, 500},
	} {
		lo, hi := betaQuantiles(tc.a, tc.b)
		mean := tc.a / (tc.a + tc.b)
		if lo < 0 || hi > 1 {
			t.Errorf("a=%v b=%v: [%v,%v] fuera de [0,1]", tc.a, tc.b, lo, hi)
		}
		if lo > mean || hi < mean {
			t.Errorf("a=%v b=%v: [%v,%v] no contiene la media %v", tc.a, tc.b, lo, hi, mean)
		}
	}
}
