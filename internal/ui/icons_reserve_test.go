package ui

import (
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── ciclo de vida de las reservas ──────────────────────────────────
//
// reserveArt se llama al construir el Cmd; releaseArt, dentro del Cmd.
// Si el Cmd nunca se ejecuta, la reserva queda viva. Estas pruebas fijan
// qué ocurre hoy en cada caso.

// TestCmdNuncaEjecutadoDejaReserva es el escenario de fuga por diseño:
// el Cmd se crea (reserva hecha) pero el runtime no lo ejecuta.
func TestCmdNuncaEjecutadoDejaReserva(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionCon(p)
	k := artKey{Kind: artItem, ID: 777, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	cmd := itemArtCmdFor(s, []int{777}, smallArtCols)
	if cmd == nil {
		t.Fatal("debía crear el Cmd")
	}
	// No se ejecuta: el runtime lo descartó.
	if got := s.pendingFor(k); got != 1 {
		t.Fatalf("reserva pendiente = %d, se esperaba 1 (el Cmd no se ejecutó)", got)
	}

	// Consecuencia: un nuevo intento para la MISMA clave no relanza nada.
	if again := itemArtCmdFor(s, []int{777}, smallArtCols); again != nil {
		t.Error("con la reserva viva no se relanza: el icono queda sin pedir")
	}
	// Un ancho distinto tampoco: comparte recurso y ya está reservado.
	if other := itemArtCmdFor(s, []int{777}, artColsLarge); other != nil {
		t.Error("otro ancho del mismo recurso tampoco relanza")
	}
	t.Log("FUGA CONFIRMADA: reserva viva sin Cmd ejecutado; " +
		"el recurso no vuelve a pedirse en esa sesión")
}

// TestInvalidateArtNoLimpiaReservasDeItem: InvalidateArt limpia
// ItemArt y artFailed, pero NO runePending/pendingKeys, así que una
// reserva de ítem viva sobrevive a la invalidación.
func TestInvalidateArtNoLimpiaReservasDeItem(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionCon(p)
	k := artKey{Kind: artItem, ID: 888, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	_ = itemArtCmdFor(s, []int{888}, smallArtCols) // reserva sin ejecutar

	s.InvalidateArt()

	// ESTADO ACTUAL DOCUMENTADO (defecto abierto, pendiente de decisión):
	// InvalidateArt limpia ItemArt y artFailed pero NO runePending ni
	// pendingKeys, así que la reserva de ítem sobrevive. Consecuencia: el
	// recurso no vuelve a pedirse hasta que cambie la sesión.
	pending := s.pendingFor(k)
	again := itemArtCmdFor(s, []int{888}, smallArtCols)
	if pending != 0 && again == nil {
		t.Logf("DEFECTO CONFIRMADO: InvalidateArt no reinicia reservas de ítem "+
			"(pendiente=%d) y el recurso no vuelve a pedirse", pending)
		return
	}
	if pending != 0 {
		t.Errorf("estado inesperado: pendiente=%d pero el Cmd sí se relanza", pending)
	}
	if again == nil {
		t.Error("sin reserva pendiente el ítem debería poder volver a pedirse")
	}
}

// TestInvalidateRuneArtSiLimpiaReservas: la invalidación de runas sí
// reinicia los mapas de reserva (mismo mapa compartido).
func TestInvalidateRuneArtSiLimpiaReservas(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionCon(p)
	k := artKey{Kind: artItem, ID: 999, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	_ = itemArtCmdFor(s, []int{999}, smallArtCols)
	if s.pendingFor(k) != 1 {
		t.Fatal("debía haber reserva")
	}
	s.InvalidateRuneArt()
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("InvalidateRuneArt debía reiniciar los mapas de reserva, quedan %d", got)
	}
}

// TestReservaVivaTras404NoBloqueaOtroId: una reserva colgada de un id no
// debe afectar a otro recurso distinto.
func TestReservaVivaTras404NoBloqueaOtroId(t *testing.T) {
	p := &cuentaProvider{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionCon(p)
	_ = itemArtCmdFor(s, []int{1000}, smallArtCols) // no se ejecuta

	if cmd := itemArtCmdFor(s, []int{1001}, smallArtCols); cmd == nil {
		t.Error("otro id debe poder pedirse aunque haya una reserva colgada")
	}
}

var _ = model.GameLoL
