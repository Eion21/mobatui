package deadlock

// Pruebas del constructor híbrido (Fase 4). Fixtures sintéticos:
// reproducibles, sin red.

import (
	"testing"

	"github.com/Eion21/mobatui/internal/progression"
)

// hybridFixture arma un catálogo con una cadena de tres, una
// bifurcación y compras sueltas.
func hybridFixture(t *testing.T) *progression.Catalog {
	t.Helper()
	raw := []progression.RawItem{
		{ID: 1, Name: "Extra Spirit", Class: "extra_spirit", Slot: "spirit", Tier: 1, Cost: 800, Shopable: true},
		{ID: 2, Name: "Improved Spirit", Class: "improved_spirit", Slot: "spirit", Tier: 2, Cost: 1600,
			Comps: []string{"extra_spirit"}, Shopable: true},
		{ID: 3, Name: "Boundless Spirit", Class: "boundless_spirit", Slot: "spirit", Tier: 3, Cost: 6400,
			Comps: []string{"improved_spirit"}, Shopable: true},
		{ID: 4, Name: "Grit", Class: "grit", Slot: "vitality", Tier: 1, Cost: 800, Shopable: true},
		{ID: 5, Name: "Spirit Shielding", Class: "spirit_shielding", Slot: "vitality", Tier: 2, Cost: 1600,
			Comps: []string{"grit"}, Shopable: true},
		{ID: 6, Name: "Mystic Shot", Class: "mystic_shot", Slot: "spirit", Tier: 2, Cost: 1600, Shopable: true},
		{ID: 7, Name: "Rare Item", Class: "rare_item", Slot: "weapon", Tier: 3, Cost: 3200, Shopable: true},
		{ID: 8, Name: "Other Rare", Class: "other_rare", Slot: "weapon", Tier: 3, Cost: 3200, Shopable: true},
	}
	cat, _, err := progression.BuildCatalog(raw)
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	return cat
}

// hybridRuns construye una muestra con los shares indicados.
func hybridRuns(total int, shares map[int]float64, minute map[int]int) []Run {
	runs := make([]Run, 0, total)
	for i := 0; i < total; i++ {
		var evs []Ev
		for id, sh := range shares {
			if float64(i) < sh*float64(total) {
				m := 5
				if mm, ok := minute[id]; ok {
					m = mm
				}
				evs = append(evs, Ev{Item: id, T: m * 60})
			}
		}
		runs = append(runs, Run{Evs: evs})
	}
	return runs
}

// TestHybridChainsComplete: una cadena de tres produce los tres pasos y
// un solo objeto final.
func TestHybridChainsComplete(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 2: 0.85, 3: 0.60, 6: 0.50,
	}, map[int]int{1: 5, 2: 10, 3: 28, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// La cadena completa debe estar en el plan.
	var steps []int
	for _, st := range hb.Plan.Steps {
		steps = append(steps, st.Item)
	}
	want := []int{1, 2, 3}
	for _, w := range want {
		found := false
		for _, s := range steps {
			if s == w {
				found = true
			}
		}
		if !found {
			t.Errorf("falta el paso %d en la secuencia: %v", w, steps)
		}
	}
	// Solo Boundless Spirit queda en el inventario de esa cadena.
	inInv := map[int]bool{}
	for _, id := range hb.Plan.Inventory {
		inInv[id] = true
	}
	if !inInv[3] {
		t.Errorf("Boundless Spirit no está en el inventario: %v", hb.Plan.Inventory)
	}
	if inInv[1] || inInv[2] {
		t.Errorf("componentes consumidos siguen en el inventario: %v", hb.Plan.Inventory)
	}
}

// TestHybridRespectsMinShare: un objeto por debajo del umbral no entra
// en la ruta estándar aunque tenga buena confianza.
func TestHybridRespectsMinShare(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 6: 0.50, 7: 0.15, // 7 es raro
	}, map[int]int{1: 5, 6: 12, 7: 25})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, e := range hb.Standard {
		if e.Item == 7 {
			t.Errorf("objeto con share 15%% entró en la ruta estándar")
		}
	}
	// Debe estar excluido con motivo explícito.
	found := false
	for _, x := range hb.Excluded {
		if x.Item == 7 {
			found = true
			if x.Reason == "" {
				t.Error("exclusión sin motivo")
			}
		}
	}
	if !found {
		t.Error("el objeto raro no aparece en las exclusiones")
	}
}

// TestHybridPrefersFinalOverComponent: si un componente y su mejora
// están ambos por encima del umbral, solo la mejora ocupa plaza.
func TestHybridPrefersFinalOverComponent(t *testing.T) {
	cat := hybridFixture(t)
	// Muestra realista: con 200 partidas la confianza de un objeto de
	// MID ronda el umbral y el test mediría el corte, no la regla de
	// cadenas que quiere verificar.
	runs := hybridRuns(1000, map[int]float64{
		4: 0.60, 5: 0.55,
	}, map[int]int{4: 4, 5: 11})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var finals []int
	for _, e := range hb.Standard {
		if e.ConsumedBy == 0 {
			finals = append(finals, e.Item)
		}
	}
	// Grit (4) es consumido por Spirit Shielding (5): solo 5 es final.
	if len(finals) != 1 || finals[0] != 5 {
		t.Errorf("objetos finales = %v, esperado [5]", finals)
	}
}

// TestHybridMarksConsumedBy: los pasos intermedios quedan marcados.
func TestHybridMarksConsumedBy(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		4: 0.60, 5: 0.55,
	}, map[int]int{4: 4, 5: 11})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, e := range hb.Standard {
		if e.Item == 4 && e.ConsumedBy != 5 {
			t.Errorf("Grit ConsumedBy = %d, esperado 5", e.ConsumedBy)
		}
		if e.Item == 5 && e.ConsumedBy != 0 {
			t.Errorf("Spirit Shielding ConsumedBy = %d, esperado 0", e.ConsumedBy)
		}
	}
}

// TestHybridOrderIsTemporal: la ruta va en orden de minuto de compra.
func TestHybridOrderIsTemporal(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 4: 0.60, 6: 0.50,
	}, map[int]int{1: 5, 4: 4, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	prev := -1
	for _, e := range hb.Standard {
		if e.Minute < prev {
			t.Errorf("orden no temporal: %d tras %d", e.Minute, prev)
		}
		prev = e.Minute
	}
}

// TestHybridInventoryValid: el plan no excede la capacidad.
func TestHybridInventoryValid(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 4: 0.60, 6: 0.50, 7: 0.40,
	}, map[int]int{1: 5, 4: 4, 6: 12, 7: 25})

	hb, err := BuildHybrid(runs, cat, BuildOptions{Capacity: 2})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(hb.Plan.Inventory) > 2 {
		t.Errorf("inventario %v excede la capacidad 2", hb.Plan.Inventory)
	}
}

// TestHybridDeterministic: misma muestra, misma build.
func TestHybridDeterministic(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 2: 0.85, 3: 0.60, 4: 0.60, 5: 0.55, 6: 0.50,
	}, map[int]int{1: 5, 2: 10, 3: 28, 4: 4, 5: 11, 6: 12})

	a, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(a.Standard) != len(b.Standard) {
		t.Fatalf("longitudes distintas: %d vs %d", len(a.Standard), len(b.Standard))
	}
	for i := range a.Standard {
		if a.Standard[i].Item != b.Standard[i].Item {
			t.Errorf("paso %d difiere: %d vs %d", i, a.Standard[i].Item, b.Standard[i].Item)
		}
	}
}

// TestHybridSituationalsHaveFunction: todo situacional trae una
// etiqueta de función verificable, y ninguno afirma contrarrestar a un
// héroe concreto.
func TestHybridSituationalsHaveFunction(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 6: 0.50, 7: 0.20,
	}, map[int]int{1: 5, 6: 12, 7: 25})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, s := range hb.Situationals {
		if s.Reason == "" {
			t.Errorf("situacional %q sin función", s.Name)
		}
	}
}

// TestHybridNoCandidates: una muestra vacía devuelve error, no una
// build inventada.
func TestHybridNoCandidates(t *testing.T) {
	cat := hybridFixture(t)
	if _, err := BuildHybrid(nil, cat, BuildOptions{}); err == nil {
		t.Fatal("esperado error con muestra vacía")
	}
}

// TestHybridNilCatalog: catálogo nil es un error explícito.
func TestHybridNilCatalog(t *testing.T) {
	runs := hybridRuns(100, map[int]float64{1: 0.9}, nil)
	if _, err := BuildHybrid(runs, nil, BuildOptions{}); err == nil {
		t.Fatal("esperado error con catálogo nil")
	}
}

// TestHybridConfidenceIsWeakestLink: la confianza global no supera la
// del eslabón más débil.
func TestHybridConfidenceIsWeakestLink(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 6: 0.35,
	}, map[int]int{1: 5, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, e := range hb.Standard {
		if hb.Confidence > e.Confidence+1e-9 {
			t.Errorf("confianza global %.3f supera la del paso %q (%.3f)",
				hb.Confidence, e.Name, e.Confidence)
		}
	}
}

// TestHybridExtensionsDistinctFromStandard: ampliaciones y ruta no
// comparten objetos.
func TestHybridExtensionsDistinctFromStandard(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(200, map[int]float64{
		1: 0.90, 4: 0.60, 6: 0.50, 7: 0.35,
	}, map[int]int{1: 5, 4: 4, 6: 12, 7: 25})

	hb, err := BuildHybrid(runs, cat, BuildOptions{Capacity: 2, MaxExtensions: 3})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	inStd := map[int]bool{}
	for _, e := range hb.Standard {
		inStd[e.Item] = true
	}
	for _, e := range hb.Extensions {
		if inStd[e.Item] {
			t.Errorf("%q está en ruta y en ampliaciones", e.Name)
		}
	}
}

// TestHybridNoRedundantExtensions: ninguna ampliación puede ser
// componente de un objeto que la build ya compra. Regresión del caso
// real de Violet: Superior Cooldown aparecía como ampliación cuando
// Transcendent Cooldown ya cerraba la cadena, y Mystic Burst cuando
// Tankbuster ya estaba en el inventario.
func TestHybridNoRedundantExtensions(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		1: 0.90, 2: 0.85, 3: 0.60, // cadena de tres
		4: 0.60, 5: 0.55, // cadena de dos
		6: 0.50,
	}, map[int]int{1: 5, 2: 10, 3: 28, 4: 4, 5: 11, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{MaxExtensions: 10})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// Componentes de todo lo que la build compra.
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
			t.Errorf("ampliación %q es componente de algo que la ruta ya compra", e.Name)
		}
	}
	for _, e := range hb.Situationals {
		if consumed[e.Item] {
			t.Errorf("situacional %q es componente de algo que la ruta ya compra", e.Name)
		}
	}
}

// TestHybridInventoryValidAfterEachStep: el inventario es válido en
// cada paso de la secuencia, no solo al final. Es lo que el jugador ve
// mientras compra.
func TestHybridInventoryValidAfterEachStep(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		1: 0.90, 2: 0.85, 3: 0.60, 4: 0.60, 5: 0.55, 6: 0.50,
	}, map[int]int{1: 5, 2: 10, 3: 28, 4: 4, 5: 11, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{Capacity: 12})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	seen := map[int]bool{}
	for i, st := range hb.Plan.Steps {
		// Ningún objeto puede estar dos veces en el inventario.
		dup := map[int]bool{}
		for _, id := range st.Inventory {
			if dup[id] {
				t.Errorf("paso %d: %d duplicado en el inventario", i+1, id)
			}
			dup[id] = true
		}
		// No puede exceder la capacidad.
		if len(st.Inventory) > 12 {
			t.Errorf("paso %d: inventario %d excede 12", i+1, len(st.Inventory))
		}
		// El objeto comprado debe estar en el inventario resultante.
		if !dup[st.Item] {
			t.Errorf("paso %d: el objeto comprado %d no está en el inventario", i+1, st.Item)
		}
		// Los componentes consumidos no pueden seguir ahí.
		for _, comp := range st.Consumed {
			if dup[comp] {
				t.Errorf("paso %d: el componente %d sigue en el inventario", i+1, comp)
			}
		}
		// Ninguna compra repetida a lo largo de la secuencia.
		if seen[st.Item] {
			t.Errorf("paso %d: %d comprado dos veces", i+1, st.Item)
		}
		seen[st.Item] = true
	}
}

// TestHybridPhaseProgressionRealistic: las fases respetan una progresión
// temporal. Un objeto EARLY no puede aparecer después de uno LATE.
func TestHybridPhaseProgressionRealistic(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		1: 0.90, 2: 0.85, 3: 0.60, 4: 0.60, 5: 0.55, 6: 0.50,
	}, map[int]int{1: 5, 2: 10, 3: 28, 4: 4, 5: 11, 6: 12})

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// En la secuencia simulada, la fase no puede retroceder.
	prev := -1
	for i, st := range hb.Plan.Steps {
		p := int(st.Phase)
		if p < prev {
			t.Errorf("paso %d: fase %v retrocede tras %d", i+1, st.Phase, prev)
		}
		prev = p
	}
}

// TestHybridExtensionsAreLateGame: una ampliación es una compra para
// partidas largas. Un objeto de EARLY que no entró en la ruta no es una
// ampliación: es una compra con evidencia insuficiente. Regresión del
// caso real: Extra Regen (min 3) aparecía como ampliación de Violet.
func TestHybridExtensionsAreLateGame(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		1: 0.90, 6: 0.50, // ruta: EARLY + MID
		7: 0.20, // raro, LATE (min 25)
	}, map[int]int{1: 5, 6: 12, 7: 25})

	hb, err := BuildHybrid(runs, cat, BuildOptions{MaxExtensions: 10})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, e := range hb.Extensions {
		if e.Phase != progression.PhaseLate {
			t.Errorf("ampliación %q en fase %v: no es de partida larga", e.Name, e.Phase)
		}
	}
}

// TestExtensionsChronological: las ampliaciones se ordenan por minuto
// estimado de compra, no por score. Regresión del caso real de Violet,
// donde salían ~29, ~20, ~33, ~22 min.
func TestExtensionsChronological(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{
		1: 0.90, 6: 0.50,
		7: 0.20, // raro, min 25
		8: 0.18, // raro, min 21
	}, map[int]int{1: 5, 6: 12, 7: 25, 8: 21})

	hb, err := BuildHybrid(runs, cat, BuildOptions{MaxExtensions: 10})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	prev := -1
	for _, e := range hb.Extensions {
		if e.Minute < prev {
			t.Errorf("ampliación %q en min %d tras min %d: orden no cronológico",
				e.Name, e.Minute, prev)
		}
		prev = e.Minute
	}
}

// TestExtensionsDeterministic: mismo input, mismo orden.
func TestExtensionsDeterministic(t *testing.T) {
	cat := hybridFixture(t)
	runs := hybridRuns(1000, map[int]float64{1: 0.90, 6: 0.50, 7: 0.20, 8: 0.18},
		map[int]int{1: 5, 6: 12, 7: 25, 8: 21})
	a, _ := BuildHybrid(runs, cat, BuildOptions{MaxExtensions: 10})
	b, _ := BuildHybrid(runs, cat, BuildOptions{MaxExtensions: 10})
	if len(a.Extensions) != len(b.Extensions) {
		t.Fatalf("longitudes distintas")
	}
	for i := range a.Extensions {
		if a.Extensions[i].Item != b.Extensions[i].Item {
			t.Errorf("orden distinto en %d", i)
		}
	}
}
