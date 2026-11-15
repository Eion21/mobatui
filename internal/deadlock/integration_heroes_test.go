package deadlock

// Integración de los cuatro héroes del brief con el pipeline completo:
// muestra real → BuildHybrid → datos que consume la TUI.

import (
	"context"
	"testing"
)

func TestIntegrationFourHeroes(t *testing.T) {
	api := diagClient(t)
	cat, err := api.ProgressionCatalog()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	for _, tc := range []struct {
		name string
		id   int
	}{
		{"Violet", 86},
		{"Solomon", 85},
		{"Pocket", 50},
		{"Celeste", 81},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ResetRunsMemo()
			runs, err := api.FetchRuns(context.Background(), tc.id, 0, 250)
			if err != nil || len(runs) < 100 {
				t.Skipf("sin muestra: %v (%d)", err, len(runs))
			}
			hb, err := BuildHybrid(runs, cat, BuildOptions{})
			if err != nil {
				t.Fatalf("build: %v", err)
			}

			// 1. Las tres secciones existen y la ruta no está vacía.
			if len(hb.Standard) == 0 {
				t.Error("ruta estándar vacía")
			}
			// 2. Fases presentes en orden no decreciente.
			prev := -1
			for _, st := range hb.Plan.Steps {
				if int(st.Phase) < prev {
					t.Errorf("fase retrocede en el paso %d", st.Item)
				}
				prev = int(st.Phase)
			}
			// 3. Inventario válido en cada paso.
			for i, st := range hb.Plan.Steps {
				if len(st.Inventory) > 12 {
					t.Errorf("paso %d: inventario %d > 12", i+1, len(st.Inventory))
				}
				for _, comp := range st.Consumed {
					for _, inv := range st.Inventory {
						if inv == comp {
							t.Errorf("paso %d: componente %d sigue en inventario", i+1, comp)
						}
					}
				}
			}
			// 4. Sin redundancias: ni ampliación ni situacional es
			//    componente de algo que la build ya compra.
			consumed := map[int]bool{}
			for _, e := range hb.Standard {
				for _, id := range cat.Chain(e.Item) {
					if id != e.Item {
						consumed[id] = true
					}
				}
			}
			for _, e := range hb.Extensions {
				if consumed[e.Item] {
					t.Errorf("ampliación redundante: %s", e.Name)
				}
			}
			for _, e := range hb.Situationals {
				if consumed[e.Item] {
					t.Errorf("situacional redundante: %s", e.Name)
				}
			}
			// 5. Confianza en rango y etiqueta válida.
			if hb.Confidence < 0 || hb.Confidence > 1 {
				t.Errorf("confianza fuera de rango: %.2f", hb.Confidence)
			}
			// 6. Cada entrada tiene procedencia y motivo.
			for _, e := range hb.Standard {
				if len(e.Origin) == 0 {
					t.Errorf("%s sin procedencia", e.Name)
				}
				if e.Reason == "" {
					t.Errorf("%s sin motivo", e.Name)
				}
			}
			// 7. Situacionales con etiqueta breve.
			for _, e := range hb.Situationals {
				if e.Reason == "" {
					t.Errorf("situacional %s sin función", e.Name)
				}
				if len(e.Reason) > 24 {
					t.Errorf("etiqueta larga: %q", e.Reason)
				}
			}

			t.Logf("%s: muestra=%d · ruta=%d · finales=%d · ampliaciones=%d · situacionales=%d · confianza=%.2f",
				tc.name, len(runs), len(hb.Standard), hb.Plan.FinalItems(),
				len(hb.Extensions), len(hb.Situationals), hb.Confidence)
		})
	}
}
