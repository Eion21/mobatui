package ui

import (
	"sync"
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── ciclo de vida de las reservas ─────────────────────────────────
//
// reserveArt se llama al construir el Cmd; releaseArt, dentro del Cmd.
// Estas pruebas afirman el comportamiento correcto: invalidar reinicia
// las reservas y una liberación tardía no deja contadores negativos.

func sesionReservas(p *cuentaProvider) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    p,
	}
}

// TestInvalidateArtReiniciaReservas es la prueba del punto 3.3: tras
// invalidar con una reserva viva, el recurso vuelve a pedirse.
func TestInvalidateArtReiniciaReservas(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artItem, ID: 888, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	_ = itemArtCmdFor(s, []int{888}, smallArtCols) // reserva sin ejecutar
	if s.pendingFor(k) != 1 {
		t.Fatalf("debía haber reserva, hay %d", s.pendingFor(k))
	}

	s.InvalidateArt()

	if got := s.pendingFor(k); got != 0 {
		t.Errorf("InvalidateArt debía reiniciar las reservas, quedan %d", got)
	}
	if again := itemArtCmdFor(s, []int{888}, smallArtCols); again == nil {
		t.Error("tras InvalidateArt el ítem debe poder volver a pedirse")
	}
}

// TestInvalidateArtDuranteDescargaEnVuelo: una descarga en vuelo que
// termina DESPUÉS de invalidar no debe provocar pánico, ni contadores
// negativos, ni dejar reserva residual.
//
// El resultado tardío se guarda bajo la clave que lo pidió (j.key), no
// bajo el ancho vigente: eso es exactamente la garantía de "respuestas
// tardías en su clave".
func TestInvalidateArtDuranteDescargaEnVuelo(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	const id = 1234
	k := artKey{Kind: artItem, ID: id, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	cmd := itemArtCmdFor(s, []int{id}, smallArtCols)
	if cmd == nil {
		t.Fatal("debía crear el Cmd")
	}

	// Se invalida mientras la descarga está en vuelo.
	s.InvalidateArt()

	// Y ahora termina la descarga.
	msg := cmd().(artLoadedMsg)
	if msg.Art == nil {
		t.Fatal("mensaje inesperado")
	}

	// Sin pánico y sin contador negativo.
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva residual = %d, se esperaba 0", got)
	}
	if got := s.runePending[s.resourceForLocked(k)]; got < 0 {
		t.Errorf("contador negativo = %d", got)
	}

	// DÓNDE QUEDA EL RESULTADO TARDÍO: en ninguna clave. InvalidateArt
	// es una orden explícita del usuario de descartar el arte, y el Cmd
	// resuelve sus claves con pendingKeysFor en el momento de publicar,
	// que ya está vacío. El trabajo se pierde, que es lo que pidió la
	// invalidación.
	//
	// Esto NO contradice la garantía "respuestas tardías en su clave":
	// esa garantía cubre cambios de representación (tamaño, pestaña,
	// campeón), donde el Cmd conserva su j.key y publica ahí. La cubre
	// TestResultadoTardioNoPisaElTamanoVigente, que sigue verde.
	if len(msg.Art) != 0 {
		t.Errorf("tras invalidar no debe publicarse arte, se publicó %d claves", len(msg.Art))
	}
	// Y el recurso sigue pedible (la invalidación surtió efecto).
	if again := itemArtCmdFor(s, []int{id}, smallArtCols); again == nil {
		t.Error("tras invalidar el recurso debe poder volver a pedirse")
	}
}

// TestLiberacionTardiaNoBajaDeCero: llamar a releaseArt después de una
// invalidación no debe dejar el contador negativo.
func TestLiberacionTardiaNoBajaDeCero(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artItem, ID: 99, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	if !s.reserveArt(k) {
		t.Fatal("debía reservar")
	}
	s.InvalidateArt()  // reinicia los mapas
	s.releaseArt(k, 0) // liberación tardía de la generación vieja

	if got := s.runePending[s.resourceForLocked(k)]; got != 0 {
		t.Errorf("contador = %d, se esperaba 0 (no negativo)", got)
	}
}

// TestInvalidateArtSigueLimpiandoArteYFallos: la invalidación conserva
// su contrato anterior.
func TestInvalidateArtSigueLimpiandoArteYFallos(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artItem, ID: 5, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	s.putArt(k, "arte")
	s.markArtFailed(k, artFailMissing)
	if !s.hasArt(k) || !s.artDidFail(k) {
		t.Fatal("debía haber arte y fallo")
	}

	s.InvalidateArt()

	if s.hasArt(k) {
		t.Error("InvalidateArt debe limpiar el arte")
	}
	if s.artDidFail(k) {
		t.Error("InvalidateArt debe limpiar los fallos")
	}
}

// TestReservaConcurrenteNoNiegaElContador: reservas y liberaciones en
// paralelo no dejan el contador negativo.
func TestReservaConcurrenteNoNiegaElContador(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artItem, ID: 77, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.reserveArt(k) {
				s.releaseArt(k, s.artGenOf(k))
			}
		}()
	}
	wg.Wait()

	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva residual = %d", got)
	}
	if got := s.runePending[s.resourceForLocked(k)]; got < 0 {
		t.Errorf("contador negativo = %d", got)
	}
}

// TestInvalidateArtConReservaDeRuna: cubre también el camino de runas,
// que comparte los mismos mapas.
func TestInvalidateArtConReservaDeRuna(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artRune, ID: 8100, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.21"}

	if !s.reserveArt(k) {
		t.Fatal("debía reservar")
	}
	s.InvalidateArt()
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva de runa residual = %d", got)
	}
}

var _ = ddragon.IsNotFound

// TestLiberacionTardiaNoAnulaReservaNueva: el caso realmente dañino de
// una liberación tardía. Si el Cmd viejo libera DESPUÉS de que ya hay una
// reserva nueva, sin la comprobación el contador baja a 0 y borra la
// entrada: la reserva nueva desaparece y el recurso se descarga dos veces.
func TestLiberacionTardiaNoAnulaReservaNueva(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionReservas(p)
	k := artKey{Kind: artItem, ID: 99, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}

	if !s.reserveArt(k) {
		t.Fatal("debía reservar (Cmd viejo)")
	}
	s.InvalidateArt()
	if !s.reserveArt(k) {
		t.Fatal("debía reservar de nuevo (Cmd nuevo)")
	}

	// El Cmd viejo termina ahora y libera CON SU generación (la 0).
	s.releaseArt(k, 0)

	// La reserva del Cmd nuevo debe SEGUIR viva.
	if got := s.pendingFor(k); got != 1 {
		t.Errorf("la reserva nueva debía sobrevivir; pendiente = %d", got)
	}
	// Consecuencia observable: no se lanza una segunda descarga.
	if s.reserveArt(k) {
		t.Error("no debe poder reservarse otra vez: ya hay una descarga en curso")
	}
}
