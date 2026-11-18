package ui

import (
	"errors"
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// ─── deduplicación de descargas en vuelo ───────────────────────────
//
// El recurso de un icono NO depende del ancho (ver artResource), así que
// lanzar 3, 4 y 6 columnas sin resolver debe producir UNA sola descarga,
// y esa descarga debe alimentar las tres representaciones.

// cuentaProvider cuenta descargas y permite controlar la respuesta.
type cuentaProvider struct {
	staticdata.Provider
	n   int
	err error
	raw []byte
}

func (c *cuentaProvider) ItemIcon(int) ([]byte, error) {
	c.n++
	return c.raw, c.err
}

func sesionCon(p staticdata.Provider) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    p,
	}
}

// TestUnaSolaSolicitudConTresAnchosEnVuelo es la prueba principal del
// punto 4: 3 Cmd (3/4/6) lanzados sin resolver contra un servidor que
// responde 404 ⇒ exactamente 1 solicitud.
func TestUnaSolaSolicitudConTresAnchosEnVuelo(t *testing.T) {
	p := &cuentaProvider{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionCon(p)

	cmd3 := itemArtCmdFor(s, []int{9999}, smallArtCols)
	cmd4 := itemArtCmdFor(s, []int{9999}, artColsMedium)
	cmd6 := itemArtCmdFor(s, []int{9999}, artColsLarge)

	// Solo el primero que reserva devuelve Cmd; los otros ya esperan esa
	// misma descarga, así que devuelven nil (no hay nada que lanzar).
	if cmd3 == nil {
		t.Fatal("el primer Cmd debía existir")
	}
	for name, c := range map[string]bool{"4": cmd4 != nil, "6": cmd6 != nil} {
		if c {
			t.Errorf("el Cmd de %s columnas no debía lanzarse: el recurso ya está en vuelo", name)
		}
	}
	_ = cmd3()
	if p.n != 1 {
		t.Errorf("solicitudes = %d, se esperaba 1 (el recurso no depende del ancho)", p.n)
	}
}

// TestUnaDescargaAlimentaTresClaves: la única respuesta debe producir
// arte para las tres claves, cada una con su representación.
func TestUnaDescargaAlimentaTresClaves(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionCon(p)

	cmd3 := itemArtCmdFor(s, []int{4242}, smallArtCols)
	cmd4 := itemArtCmdFor(s, []int{4242}, artColsMedium)
	cmd6 := itemArtCmdFor(s, []int{4242}, artColsLarge)
	if cmd3 == nil {
		t.Fatal("el primer Cmd debía existir")
	}
	// 4 y 6 se suman como esperadores del MISMO recurso: nil es correcto.
	_ = cmd4
	_ = cmd6

	// Solo se ejecuta el primero: es el único que descarga.
	msg, ok := cmd3().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado")
	}
	if p.n != 1 {
		t.Fatalf("descargas = %d, se esperaba 1", p.n)
	}
	for k, a := range msg.Art {
		s.putArt(k, a)
	}

	// Las tres claves deben tener arte, y con su ancho correcto.
	for _, cols := range []int{smallArtCols, artColsMedium, artColsLarge} {
		art := s.ItemArtFor(4242, cols)
		if art == "" {
			t.Errorf("la clave de %d columnas quedó sin arte tras una sola descarga", cols)
		}
	}
	if p.n != 1 {
		t.Errorf("descargas = %d; se esperaba 1", p.n)
	}
}

// TestReservaLiberadaTrasExito: al terminar no queda reserva pendiente.
func TestReservaLiberadaTrasExito(t *testing.T) {
	p := &cuentaProvider{raw: pngValido(t)}
	s := sesionCon(p)
	cmd := itemArtCmdFor(s, []int{111}, smallArtCols)
	if cmd == nil {
		t.Fatal("sin Cmd")
	}
	// El Cmd devuelve el arte; publicarlo es del bucle (putArt).
	if msg, ok := cmd().(artLoadedMsg); ok {
		for k, a := range msg.Art {
			s.putArt(k, a)
		}
	}
	k := artKey{Kind: artItem, ID: 111, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva pendiente = %d, se esperaba 0", got)
	}
	// Un nuevo intento no debe relanzar: ya hay arte.
	if again := itemArtCmdFor(s, []int{111}, smallArtCols); again != nil {
		t.Error("con arte en caché no debe relanzarse")
	}
}

// TestReservaLiberadaTrasErrorTransitorio: tras un fallo de red, un
// nuevo intento DEBE volver a solicitar (no queda bloqueado).
func TestReservaLiberadaTrasErrorTransitorio(t *testing.T) {
	p := &cuentaProvider{err: errors.New("dial tcp: connection refused")}
	s := sesionCon(p)

	cmd := itemArtCmdFor(s, []int{222}, smallArtCols)
	if cmd == nil {
		t.Fatal("sin Cmd")
	}
	_ = cmd()
	k := artKey{Kind: artItem, ID: 222, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva pendiente = %d tras error transitorio", got)
	}
	again := itemArtCmdFor(s, []int{222}, smallArtCols)
	if again == nil {
		t.Fatal("un fallo transitorio no debe bloquear el reintento")
	}
	_ = again()
	if p.n != 2 {
		t.Errorf("solicitudes = %d, se esperaban 2", p.n)
	}
}

// TestReservaLiberadaTras404: tras un 404 tampoco queda reserva (aunque
// la clave sí queda marcada como ausente).
func TestReservaLiberadaTras404(t *testing.T) {
	p := &cuentaProvider{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionCon(p)
	cmd := itemArtCmdFor(s, []int{333}, smallArtCols)
	_ = cmd()
	k := artKey{Kind: artItem, ID: 333, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva pendiente = %d tras 404", got)
	}
	// Y no se vuelve a pedir: el 404 es definitivo.
	if again := itemArtCmdFor(s, []int{333}, smallArtCols); again != nil {
		t.Error("un 404 conocido no debe volver a solicitarse")
	}
}

// TestReservaLiberadaSiElCmdNoDevuelveArte: un Cmd que no produce arte
// (proveedor sin datos) tampoco debe dejar la reserva colgada.
func TestReservaLiberadaSiElCmdNoDevuelveArte(t *testing.T) {
	p := &cuentaProvider{raw: nil}
	s := sesionCon(p)
	cmd := itemArtCmdFor(s, []int{444}, smallArtCols)
	if cmd == nil {
		t.Fatal("sin Cmd")
	}
	_ = cmd()
	k := artKey{Kind: artItem, ID: 444, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	if got := s.pendingFor(k); got != 0 {
		t.Errorf("reserva pendiente = %d sin arte", got)
	}
	if s.ItemArtFor(444, smallArtCols) != "" {
		t.Error("no debe haber arte")
	}
}
