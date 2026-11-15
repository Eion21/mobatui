package progression

import (
	"fmt"
	"testing"
)

// fixture arma un catálogo de prueba con cadenas reales de Violet:
// Extra Spirit → Improved Spirit → Boundless Spirit, y una bifurcación.
func fixture(t *testing.T) *Catalog {
	t.Helper()
	raw := []RawItem{
		{ID: 1, Name: "Extra Spirit", Class: "upgrade_extra_spirit", Slot: "spirit", Tier: 1, Cost: 800, Shopable: true},
		{ID: 2, Name: "Improved Spirit", Class: "upgrade_improved_spirit", Slot: "spirit", Tier: 2, Cost: 1600,
			Comps: []string{"upgrade_extra_spirit"}, Shopable: true},
		{ID: 3, Name: "Boundless Spirit", Class: "upgrade_boundless_spirit", Slot: "spirit", Tier: 3, Cost: 3200,
			Comps: []string{"upgrade_improved_spirit"}, Shopable: true},
		// Bifurcación: dos objetos mejoran a Improved Spirit.
		{ID: 4, Name: "Spirit Burn", Class: "upgrade_spirit_burn", Slot: "spirit", Tier: 3, Cost: 3200,
			Comps: []string{"upgrade_improved_spirit"}, Shopable: true},
		// Compra sin mejora.
		{ID: 5, Name: "Mystic Shot", Class: "upgrade_mystic_shot", Slot: "spirit", Tier: 2, Cost: 1600, Shopable: true},
		// Objeto con dos componentes.
		{ID: 6, Name: "Sharpshooter", Class: "upgrade_sharpshooter", Slot: "weapon", Tier: 3, Cost: 3200,
			Comps: []string{"upgrade_extra_spirit", "upgrade_mystic_shot"}, Shopable: true},
		// Deshabilitado.
		{ID: 7, Name: "Retirado", Class: "upgrade_retired", Slot: "weapon", Tier: 1, Cost: 500, Shopable: false},
	}
	cat, warns, err := BuildCatalog(raw)
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("advertencias inesperadas: %v", warns)
	}
	return cat
}

// TestCatalogChains: la cadena lineal se reconstruye en orden de compra.
func TestCatalogChains(t *testing.T) {
	cat := fixture(t)
	got := cat.Chain(3)
	want := []int{1, 2, 3}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("cadena = %v, esperado %v", got, want)
	}
}

// TestCatalogBranches: una bifurcación produce varias cadenas válidas.
func TestCatalogBranches(t *testing.T) {
	cat := fixture(t)
	paths := cat.ChainPaths(2)
	if len(paths) != 1 {
		t.Fatalf("caminos a Improved Spirit = %d, esperado 1", len(paths))
	}
	ups := cat.Upgrades(2)
	if fmt.Sprint(ups) != fmt.Sprint([]int{3, 4}) {
		t.Errorf("mejoras de Improved Spirit = %v, esperado [3 4]", ups)
	}
}

// TestCatalogRejectsUnknownComponent: no se inventan ids.
func TestCatalogRejectsUnknownComponent(t *testing.T) {
	raw := []RawItem{
		{ID: 1, Name: "A", Class: "a", Shopable: true},
		{ID: 2, Name: "B", Class: "b", Comps: []string{"no_existe"}, Shopable: true},
	}
	_, warns, err := BuildCatalog(raw)
	if err != nil {
		t.Fatalf("no debía fallar: %v", err)
	}
	if len(warns) != 1 {
		t.Errorf("esperada 1 advertencia, hay %d: %v", len(warns), warns)
	}
}

// TestCatalogRejectsCycles: una cadena cíclica es un error, no un bucle.
func TestCatalogRejectsCycles(t *testing.T) {
	raw := []RawItem{
		{ID: 1, Name: "A", Class: "a", Comps: []string{"b"}, Shopable: true},
		{ID: 2, Name: "B", Class: "b", Comps: []string{"a"}, Shopable: true},
	}
	if _, _, err := BuildCatalog(raw); err == nil {
		t.Fatal("esperado error de ciclo")
	}
}

// TestCatalogRejectsSelfComponent.
func TestCatalogRejectsSelfComponent(t *testing.T) {
	raw := []RawItem{{ID: 1, Name: "A", Class: "a", Comps: []string{"a"}, Shopable: true}}
	if _, _, err := BuildCatalog(raw); err == nil {
		t.Fatal("esperado error de auto-componente")
	}
}

// TestIncrementalCost: el coste incremental es la diferencia real.
func TestIncrementalCost(t *testing.T) {
	cat := fixture(t)
	inc, ok := cat.IncrementalCost(2, 1)
	if !ok || inc != 800 {
		t.Errorf("coste incremental = %d (ok=%v), esperado 800", inc, ok)
	}
	// Sin costes conocidos no se inventa un número.
	if _, ok := cat.IncrementalCost(7, 1); ok {
		t.Error("no debía haber incremental con objeto deshabilitado sin coste válido")
	}
}

// TestSimulateConsumesComponents: mejorar consume el componente y libera
// su hueco.
func TestSimulateConsumesComponents(t *testing.T) {
	cat := fixture(t)
	plan, rejects := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 2, Minute: 10},
		{Item: 3, Minute: 28},
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos inesperados: %+v", rejects)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("pasos = %d, esperado 3", len(plan.Steps))
	}
	if got := plan.Steps[1].Consumed; fmt.Sprint(got) != fmt.Sprint([]int{1}) {
		t.Errorf("consumidos en el paso 2 = %v, esperado [1]", got)
	}
	if got := plan.Steps[2].Consumed; fmt.Sprint(got) != fmt.Sprint([]int{2}) {
		t.Errorf("consumidos en el paso 3 = %v, esperado [2]", got)
	}
	// Un solo objeto final: la cadena completa.
	if plan.FinalItems() != 1 {
		t.Errorf("objetos finales = %d, esperado 1 (%v)", plan.FinalItems(), plan.Inventory)
	}
	if plan.Inventory[0] != 3 {
		t.Errorf("inventario = %v, esperado [3]", plan.Inventory)
	}
}

// TestSimulateInventoryAfterEachStep: el inventario se puede leer tras
// cada paso.
func TestSimulateInventoryAfterEachStep(t *testing.T) {
	cat := fixture(t)
	plan, _ := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 6},
		{Item: 2, Minute: 10},
	})
	want := [][]int{{1}, {1, 5}, {2, 5}}
	for i, w := range want {
		if fmt.Sprint(plan.Steps[i].Inventory) != fmt.Sprint(w) {
			t.Errorf("paso %d inventario = %v, esperado %v", i+1, plan.Steps[i].Inventory, w)
		}
	}
}

// TestSimulateRespectsCapacity: sin espacio se rechaza, no se desborda.
func TestSimulateRespectsCapacity(t *testing.T) {
	cat := fixture(t)
	// Capacidad 2: la tercera compra no cabe.
	plan, rejects := Simulate(cat, 2, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 6},
		{Item: 4, Minute: 20}, // Spirit Burn: consume Improved Spirit, que no está
	})
	if len(rejects) != 1 {
		t.Fatalf("rechazos = %d, esperado 1: %+v", len(rejects), rejects)
	}
	if len(plan.Inventory) > 2 {
		t.Errorf("inventario excede la capacidad: %v", plan.Inventory)
	}
}

// TestSimulateCapacityFreedByUpgrade: consumir un componente libera el
// hueco que necesita la mejora.
func TestSimulateCapacityFreedByUpgrade(t *testing.T) {
	cat := fixture(t)
	// Con capacidad 2: Extra Spirit + Mystic Shot llenan; Improved
	// Spirit consume Extra Spirit y entra sin problema.
	plan, rejects := Simulate(cat, 2, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 6},
		{Item: 2, Minute: 10},
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos inesperados: %+v", rejects)
	}
	if fmt.Sprint(plan.Inventory) != fmt.Sprint([]int{2, 5}) {
		t.Errorf("inventario = %v, esperado [2 5]", plan.Inventory)
	}
}

// TestSimulateRejectsUnknownItem: nunca se compra algo inexistente.
func TestSimulateRejectsUnknownItem(t *testing.T) {
	cat := fixture(t)
	_, rejects := Simulate(cat, 12, []Purchase{{Item: 999, Minute: 5}})
	if len(rejects) != 1 {
		t.Fatalf("rechazos = %d, esperado 1", len(rejects))
	}
}

// TestSimulateRejectsDisabledItem.
func TestSimulateRejectsDisabledItem(t *testing.T) {
	cat := fixture(t)
	_, rejects := Simulate(cat, 12, []Purchase{{Item: 7, Minute: 5}})
	if len(rejects) != 1 {
		t.Fatalf("rechazos = %d, esperado 1", len(rejects))
	}
}

// TestSimulateRejectsDuplicate: no se compra dos veces el mismo objeto.
func TestSimulateRejectsDuplicate(t *testing.T) {
	cat := fixture(t)
	_, rejects := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 1, Minute: 6},
	})
	if len(rejects) != 1 {
		t.Fatalf("rechazos = %d, esperado 1", len(rejects))
	}
}

// TestSimulateMultiComponent: un objeto con dos componentes consume los
// dos y libera dos huecos.
func TestSimulateMultiComponent(t *testing.T) {
	cat := fixture(t)
	plan, rejects := Simulate(cat, 2, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 6},
		{Item: 6, Minute: 20}, // Sharpshooter consume ambos
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos inesperados: %+v", rejects)
	}
	last := plan.Steps[len(plan.Steps)-1]
	if len(last.Consumed) != 2 {
		t.Errorf("consumidos = %v, esperado 2", last.Consumed)
	}
	if fmt.Sprint(plan.Inventory) != fmt.Sprint([]int{6}) {
		t.Errorf("inventario = %v, esperado [6]", plan.Inventory)
	}
}

// TestSimulatePhases: la fase sale del minuto, con los cortes de la TUI.
func TestSimulatePhases(t *testing.T) {
	cat := fixture(t)
	plan, _ := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 15},
		{Item: 2, Minute: 25},
	})
	want := []Phase{PhaseEarly, PhaseMid, PhaseLate}
	for i, w := range want {
		if plan.Steps[i].Phase != w {
			t.Errorf("paso %d fase = %v, esperado %v", i+1, plan.Steps[i].Phase, w)
		}
	}
}

// TestSimulateDeterministic: mismo catálogo y secuencia, mismo plan.
func TestSimulateDeterministic(t *testing.T) {
	cat := fixture(t)
	seq := []Purchase{{Item: 1, Minute: 5}, {Item: 5, Minute: 6}, {Item: 2, Minute: 10}}
	a, _ := Simulate(cat, 12, seq)
	b, _ := Simulate(cat, 12, seq)
	if fmt.Sprint(a) != fmt.Sprint(b) {
		t.Error("la simulación no es determinista")
	}
}

// TestSimulateUnknownCost: un objeto sin coste no suma al gasto y se
// contabiliza aparte (no se inventa un precio).
func TestSimulateUnknownCost(t *testing.T) {
	raw := []RawItem{
		{ID: 1, Name: "Sin coste", Class: "a", Shopable: true},
		{ID: 2, Name: "Con coste", Class: "b", Cost: 800, Shopable: true},
	}
	cat, _, err := BuildCatalog(raw)
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	plan, _ := Simulate(cat, 12, []Purchase{{Item: 1, Minute: 5}, {Item: 2, Minute: 6}})
	if plan.Spent != 800 {
		t.Errorf("gasto = %d, esperado 800", plan.Spent)
	}
	if plan.Unknown != 1 {
		t.Errorf("costes desconocidos = %d, esperado 1", plan.Unknown)
	}
}

// TestValidate: una secuencia inválida devuelve el primer problema.
func TestValidate(t *testing.T) {
	cat := fixture(t)
	if err := Validate(cat, 12, []Purchase{{Item: 1, Minute: 5}}); err != nil {
		t.Errorf("secuencia válida rechazada: %v", err)
	}
	if err := Validate(cat, 12, []Purchase{{Item: 999, Minute: 5}}); err == nil {
		t.Error("secuencia inválida aceptada")
	}
}

// TestPurchaseOrigins: las procedencias se conservan y se consultan.
func TestPurchaseOrigins(t *testing.T) {
	p := Purchase{Item: 1, Origin: []Origin{OriginObserved, OriginStatistical}}
	if !p.HasOrigin(OriginObserved) || !p.HasOrigin(OriginStatistical) {
		t.Error("procedencias perdidas")
	}
	if p.HasOrigin(OriginCommunity) {
		t.Error("procedencia inventada")
	}
}

// TestByPhase: agrupa conservando el orden.
func TestByPhase(t *testing.T) {
	cat := fixture(t)
	plan, _ := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5}, {Item: 5, Minute: 15}, {Item: 2, Minute: 25},
	})
	by := plan.ByPhase()
	if len(by[PhaseEarly]) != 1 || len(by[PhaseMid]) != 1 || len(by[PhaseLate]) != 1 {
		t.Errorf("agrupación por fase incorrecta: %+v", by)
	}
}

// TestSpentUsesIncrementalCost: el gasto acumulado refleja el oro real.
// Comprar el componente y luego mejorarlo cuesta lo mismo que comprar
// el objeto final directamente; sumar precios de catálogo lo contaba dos
// veces.
func TestSpentUsesIncrementalCost(t *testing.T) {
	cat := fixture(t)
	// 1 (800) → 2 (1600) → 3 (3200).
	plan, rejects := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 2, Minute: 10},
		{Item: 3, Minute: 28},
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos: %+v", rejects)
	}
	// El oro real es el precio del objeto final: 3200.
	if plan.Spent != 3200 {
		t.Errorf("gasto = %d, esperado 3200 (precio del objeto final)", plan.Spent)
	}
	// Los incrementales suman exactamente eso.
	var sum int
	for _, st := range plan.Steps {
		if st.HasIncrement {
			sum += st.Incremental
		} else {
			sum += st.Cost
		}
	}
	if sum != plan.Spent {
		t.Errorf("suma de pagos %d != gasto %d", sum, plan.Spent)
	}
}

// TestSpentDirectPurchasePaysFullPrice: sin el componente en el
// inventario se paga el precio completo del objeto.
func TestSpentDirectPurchasePaysFullPrice(t *testing.T) {
	cat := fixture(t)
	plan, rejects := Simulate(cat, 12, []Purchase{
		{Item: 3, Minute: 28}, // Boundless Spirit sin Improved Spirit
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos: %+v", rejects)
	}
	if plan.Spent != 3200 {
		t.Errorf("gasto = %d, esperado 3200 (precio completo)", plan.Spent)
	}
	if plan.Steps[0].HasIncrement {
		t.Error("sin componente no hay coste incremental")
	}
}

// TestSpentComponentPresentVsAbsent: la misma compra cuesta lo mismo
// por las dos vías. Es la propiedad que hace coherente la etiqueta
// "Mejora de": el descuento existe porque el componente ya se pagó.
func TestSpentComponentPresentVsAbsent(t *testing.T) {
	cat := fixture(t)

	withComp, _ := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 2, Minute: 10},
	})
	direct, _ := Simulate(cat, 12, []Purchase{
		{Item: 2, Minute: 10},
	})
	if withComp.Spent != direct.Spent {
		t.Errorf("con componente %d != compra directa %d",
			withComp.Spent, direct.Spent)
	}
	// Y el inventario final es el mismo objeto.
	if fmt.Sprint(withComp.Inventory) != fmt.Sprint(direct.Inventory) {
		t.Errorf("inventario %v != %v", withComp.Inventory, direct.Inventory)
	}
}

// TestSpentChainOfThree: una cadena de tres pagos suma el precio del
// objeto final, no la suma de los tres precios.
func TestSpentChainOfThree(t *testing.T) {
	cat := fixture(t)
	plan, _ := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 2, Minute: 10},
		{Item: 3, Minute: 28},
	})
	// 800 + 800 + 1600 = 3200 = precio de Boundless Spirit.
	if plan.Spent != 3200 {
		t.Errorf("gasto = %d, esperado 3200", plan.Spent)
	}
}

// TestIncrementalMultiComponent: con dos componentes el incremental es
// la diferencia contra la suma de ambos.
func TestIncrementalMultiComponent(t *testing.T) {
	cat := fixture(t)
	// Sharpshooter (3200) = Extra Spirit (800) + Mystic Shot (1600).
	plan, rejects := Simulate(cat, 12, []Purchase{
		{Item: 1, Minute: 5},
		{Item: 5, Minute: 6},
		{Item: 6, Minute: 20},
	})
	if len(rejects) != 0 {
		t.Fatalf("rechazos: %+v", rejects)
	}
	last := plan.Steps[len(plan.Steps)-1]
	if !last.HasIncrement || last.Incremental != 800 {
		t.Errorf("incremental = %d (ok=%v), esperado 800", last.Incremental, last.HasIncrement)
	}
	if plan.Spent != 3200 {
		t.Errorf("gasto = %d, esperado 3200", plan.Spent)
	}
}
