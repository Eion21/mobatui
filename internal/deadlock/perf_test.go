package deadlock

// Medición y regresión de Fase 5: trabajo repetido al consultar un
// héroe. Solo caché local, sin red.

import (
	"context"
	"testing"
	"time"
)

// TestPerfBreakdown desglosa los tiempos por etapa. Es informativo:
// imprime, no falla por lentitud (las máquinas varían).
func TestPerfBreakdown(t *testing.T) {
	api := diagClient(t)
	ctx := context.Background()
	ResetRunsMemo()

	start := time.Now()
	runs, err := api.FetchRuns(ctx, 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}
	t.Logf("FetchRuns (frío):        %v · %d runs", time.Since(start), len(runs))

	start = time.Now()
	_, _ = api.FetchRuns(ctx, 86, 0, 250)
	t.Logf("FetchRuns (2.ª vez):     %v", time.Since(start))

	start = time.Now()
	steps := BuildRoute(runs, 0.30)
	t.Logf("BuildRoute (anterior):   %v · %d pasos", time.Since(start), len(steps))

	start = time.Now()
	cands := Evaluate(runs, EvalOptions{MinShare: 0.30})
	t.Logf("Evaluate (nuevo):        %v · %d candidatos", time.Since(start), len(cands))

	cat, _, err := buildTestCatalog(api)
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	start = time.Now()
	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("híbrida: %v", err)
	}
	t.Logf("BuildHybrid (nuevo):     %v · %d entradas", time.Since(start), len(hb.Standard))
}

// TestFetchRunsSecondCallIsCheap: volver a consultar el mismo héroe no
// debe repetir la decodificación del catálogo ni de la muestra.
func TestFetchRunsSecondCallIsCheap(t *testing.T) {
	api := diagClient(t)
	ctx := context.Background()
	ResetRunsMemo()

	start := time.Now()
	if _, err := api.FetchRuns(ctx, 86, 0, 250); err != nil {
		t.Skipf("sin runs: %v", err)
	}
	cold := time.Since(start)

	// Promedio de varias llamadas calientes para reducir ruido.
	var warm time.Duration
	const n = 5
	for i := 0; i < n; i++ {
		start = time.Now()
		if _, err := api.FetchRuns(ctx, 86, 0, 250); err != nil {
			t.Fatalf("runs: %v", err)
		}
		warm += time.Since(start)
	}
	warm /= n
	t.Logf("frío %v · caliente %v (×%.1f)", cold, warm, float64(cold)/float64(warm))

	// La llamada caliente debe ser al menos 5× más rápida. El margen es
	// amplio a propósito: mide orden de magnitud, no microbenchmark.
	if warm*5 > cold {
		t.Errorf("la segunda consulta no reutiliza trabajo: frío %v, caliente %v", cold, warm)
	}
}

// TestFetchRunsSameResult: la reutilización no cambia el resultado.
func TestFetchRunsSameResult(t *testing.T) {
	api := diagClient(t)
	ctx := context.Background()
	ResetRunsMemo()

	a, err := api.FetchRuns(ctx, 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}
	b, err := api.FetchRuns(ctx, 86, 0, 250)
	if err != nil {
		t.Fatalf("runs: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("longitudes distintas: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Win != b[i].Win || len(a[i].Evs) != len(b[i].Evs) {
			t.Fatalf("run %d difiere entre llamadas", i)
		}
	}
}

// TestRunsMemoInvalidatedByNewData: si la fecha de los datos cambia, la
// entrada memorizada no se usa (no se sirven datos viejos).
func TestRunsMemoInvalidatedByNewData(t *testing.T) {
	ResetRunsMemo()
	base := time.Unix(1000, 0)
	k1 := memoKey(86, 0, 250, base)
	k2 := memoKey(86, 0, 250, base.Add(time.Hour))

	memoPut(k1, base, []Run{{Win: true}})
	if _, ok := memoGet(k1); !ok {
		t.Fatal("la entrada esperada no está")
	}
	if _, ok := memoGet(k2); ok {
		t.Error("una fecha de datos distinta devolvió la entrada vieja")
	}
}

// TestRunsMemoBounded: la memoización no crece sin límite.
func TestRunsMemoBounded(t *testing.T) {
	ResetRunsMemo()
	base := time.Unix(1000, 0)
	for i := 0; i < runsMemoMax*3; i++ {
		memoPut(memoKey(i, 0, 250, base.Add(time.Duration(i)*time.Minute)), base, []Run{{Win: true}})
	}
	runsMemoMu.Lock()
	n := len(runsMemo)
	runsMemoMu.Unlock()
	if n > runsMemoMax {
		t.Errorf("memoización con %d entradas, tope %d", n, runsMemoMax)
	}
}

// TestRunsMemoDistinctQueries: consultas con parámetros distintos no se
// mezclan.
func TestRunsMemoDistinctQueries(t *testing.T) {
	ResetRunsMemo()
	at := time.Unix(1000, 0)
	a := memoKey(86, 0, 250, at)
	b := memoKey(86, 100, 250, at) // minBadge distinto
	c := memoKey(85, 0, 250, at)   // héroe distinto
	if a == b || a == c || b == c {
		t.Error("claves distintas colisionan")
	}
}
