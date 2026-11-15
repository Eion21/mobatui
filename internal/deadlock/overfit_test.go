package deadlock

// Pruebas de sobreajuste: el constructor no debe funcionar solo para
// Violet. Pocket (50) y Celeste (81) son casos de control con la misma
// muestra real en caché.

import (
	"context"
	"testing"
)

func TestOverfitOtherHeroes(t *testing.T) {
	api := diagClient(t)
	cat, _, err := buildTestCatalog(api)
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	for _, tc := range []struct {
		name string
		id   int
	}{
		{"Violet", 86},
		{"Pocket", 50},
		{"Celeste", 81},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetRunsMemo()
			runs, err := api.FetchRuns(context.Background(), tc.id, 0, 250)
			if err != nil || len(runs) < 100 {
				t.Skipf("sin muestra suficiente: %v (%d runs)", err, len(runs))
			}
			hb, err := BuildHybrid(runs, cat, BuildOptions{})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			t.Logf("%s: muestra=%d · ruta=%d entradas · finales=%d · ampliaciones=%d · situacionales=%d · confianza=%.2f",
				tc.name, len(runs), len(hb.Standard), hb.Plan.FinalItems(),
				len(hb.Extensions), len(hb.Situationals), hb.Confidence)

			// Invariantes que deben cumplirse para CUALQUIER héroe.
			if len(hb.Standard) == 0 {
				t.Error("ruta estándar vacía")
			}
			if hb.Plan.FinalItems() == 0 {
				t.Error("sin objetos finales")
			}
			if len(hb.Plan.Inventory) > 12 {
				t.Errorf("inventario %d excede 12", len(hb.Plan.Inventory))
			}
			if len(hb.Rejects) > 0 {
				t.Errorf("rechazos en una ruta construida: %+v", hb.Rejects)
			}
			// Ninguna entrada puede quedar sin justificación.
			for _, e := range hb.Standard {
				if e.Reason == "" {
					t.Errorf("%q sin razón", e.Name)
				}
			}
			// Cada paso de la ruta debe estar en el plan simulado.
			inPlan := map[int]bool{}
			for _, st := range hb.Plan.Steps {
				inPlan[st.Item] = true
			}
			for _, e := range hb.Standard {
				if !inPlan[e.Item] {
					t.Errorf("%q está en la ruta pero no en el plan", e.Name)
				}
			}
		})
	}
}
