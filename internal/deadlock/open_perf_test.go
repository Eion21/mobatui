package deadlock

// Medición del tiempo de APERTURA completo de la pantalla de un héroe:
// no solo FetchRuns, sino todo lo que ocurre entre pulsar el héroe y
// tener la build lista para pintar.

import (
	"context"
	"testing"
	"time"
)

func TestOpenScreenTiming(t *testing.T) {
	api := diagClient(t)
	ctx := context.Background()

	// Apertura en frío: catálogo + runs + build híbrida.
	ResetRunsMemo()
	start := time.Now()
	cat, err := api.ProgressionCatalog()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	catDur := time.Since(start)
	start = time.Now()
	runs, err := api.FetchRuns(ctx, 86, 0, 250)
	if err != nil {
		t.Skipf("runs: %v", err)
	}
	runsDur := time.Since(start)
	start = time.Now()
	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("híbrida: %v", err)
	}
	buildDur := time.Since(start)
	cold := catDur + runsDur + buildDur
	t.Logf("APERTURA FRÍA: %v  (catálogo %v + runs %v + build %v)",
		cold, catDur, runsDur, buildDur)
	t.Logf("  → %d entradas, %d objetos finales", len(hb.Standard), hb.Plan.FinalItems())

	// Segunda apertura del MISMO héroe: todo debe reutilizarse.
	start = time.Now()
	cat2, _ := api.ProgressionCatalog()
	cat2Dur := time.Since(start)
	start = time.Now()
	runs2, _ := api.FetchRuns(ctx, 86, 0, 250)
	runs2Dur := time.Since(start)
	start = time.Now()
	hb2, _ := BuildHybrid(runs2, cat2, BuildOptions{})
	build2Dur := time.Since(start)
	warm := cat2Dur + runs2Dur + build2Dur
	t.Logf("APERTURA CALIENTE: %v  (catálogo %v + runs %v + build %v)",
		warm, cat2Dur, runs2Dur, build2Dur)

	if warm*5 > cold {
		t.Errorf("la reapertura no reutiliza trabajo: frío %v, caliente %v", cold, warm)
	}
	if len(hb2.Standard) != len(hb.Standard) {
		t.Errorf("la reapertura dio otra build: %d vs %d", len(hb2.Standard), len(hb.Standard))
	}

	// Apertura de otro héroe: reutiliza catálogo, no runs.
	start = time.Now()
	cat3, _ := api.ProgressionCatalog()
	cat3Dur := time.Since(start)
	start = time.Now()
	runs3, err := api.FetchRuns(ctx, 85, 0, 250) // Solomon
	runs3Dur := time.Since(start)
	if err != nil {
		t.Logf("Solomon sin muestra: %v", err)
		return
	}
	start = time.Now()
	hb3, err := BuildHybrid(runs3, cat3, BuildOptions{})
	build3Dur := time.Since(start)
	if err != nil {
		t.Logf("Solomon sin build: %v", err)
		return
	}
	t.Logf("OTRO HÉROE (Solomon): %v  (catálogo %v + runs %v + build %v)",
		cat3Dur+runs3Dur+build3Dur, cat3Dur, runs3Dur, build3Dur)
	t.Logf("  → %d entradas, %d objetos finales", len(hb3.Standard), hb3.Plan.FinalItems())
}
