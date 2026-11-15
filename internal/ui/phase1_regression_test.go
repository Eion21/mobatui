package ui

// Pruebas de regresión de Fase 1: numeración continua de pasos y
// orden visual de habilidades. No dependen de red ni de la caché real:
// usan fixtures sintéticos para ser reproducibles en CI.

import "testing"

// TestNumberStepsContinuous: los números de paso visibles son 1..m sin
// huecos aunque la ruta de consenso tenga pasos filtrados.
func TestNumberStepsContinuous(t *testing.T) {
	cards := []cardItem{
		{order: 1}, {order: 2}, {order: 3},
		{order: 5},             // el 4 se filtró por fase
		{order: 6}, {order: 9}, // el 7 y el 8 también
		{order: 0}, // fuera de la ruta
		{order: 12},
	}
	got := numberSteps(cards)
	want := []int{1, 2, 3, 4, 5, 6, 0, 7}
	for i, w := range want {
		if got[i].step != w {
			t.Errorf("tarjeta %d: step = %d, esperado %d", i, got[i].step, w)
		}
	}
	// `order` se conserva: el detalle sigue pudiendo explicar la ruta.
	if got[3].order != 5 || got[7].order != 12 {
		t.Errorf("order no debe alterarse: %+v", got)
	}
}

// TestNumberStepsEmpty: sin tarjetas no hay pánico ni números.
func TestNumberStepsEmpty(t *testing.T) {
	if got := numberSteps(nil); len(got) != 0 {
		t.Errorf("esperado vacío, hay %d", len(got))
	}
}

// TestNumberStepsOnlyOutsideRoute: si todo está fuera de la ruta no se
// numera nada (no se inventan pasos).
func TestNumberStepsOnlyOutsideRoute(t *testing.T) {
	got := numberSteps([]cardItem{{order: 0}, {order: 0}})
	for i, c := range got {
		if c.step != 0 {
			t.Errorf("tarjeta %d fuera de ruta con step = %d", i, c.step)
		}
	}
}

// TestNumberStepsAfterUpgradeFilter: plegar mejoras no deja huecos en
// la numeración visible.
func TestNumberStepsAfterUpgradeFilter(t *testing.T) {
	d := detailView{showUpgrades: false}
	cards := []cardItem{
		{order: 1, kind: kindBuy},
		{order: 2, kind: kindUpgrade},
		{order: 3, kind: kindBuy},
		{order: 4, kind: kindUpgrade},
		{order: 5, kind: kindBuy},
	}
	got := numberSteps(d.filterUpgrades(cards))
	if len(got) != 3 {
		t.Fatalf("esperadas 3 tarjetas, hay %d", len(got))
	}
	for i, c := range got {
		if c.step != i+1 {
			t.Errorf("tarjeta %d: step = %d, esperado %d", i, c.step, i+1)
		}
	}
}
