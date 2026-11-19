package ui

import (
	"errors"
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// ─── guard de fallos en runas (paridad con ítems) ──────────────────
//
// runeArtCmdFor debe consultar artDidFail y propagar el fallo
// definitivo, igual que itemArtCmdFor. Sin eso, un 404 de runa se
// vuelve a pedir en cada evento.

// runeProvider cuenta solicitudes y devuelve el error configurado.
type proveedorRunas struct {
	staticdata.Provider
	solicitudes int
	err         error
	raw         []byte
}

func (p *proveedorRunas) RuneIcon(int) ([]byte, error) {
	p.solicitudes++
	return p.raw, p.err
}

func sesionRunas2(p staticdata.Provider) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    p,
	}
}

// TestRuna404UnaSolaSolicitudPorAncho: 404 de una runa y cambio de
// ancho ⇒ EXACTAMENTE 1 solicitud. Como runeArtColsPorDefecto es fijo, se llama a
// runeArtCmdFor directamente con claves de distinto Cols.
func TestRuna404UnaSolaSolicitudPorAncho(t *testing.T) {
	p := &proveedorRunas{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionRunas2(p)
	const id = 8100

	// Primer intento: descarga y 404.
	cmd := runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)
	if cmd == nil {
		t.Fatal("debía pedir la runa")
	}
	_ = cmd()

	// La clave quedó marcada como ausente.
	k3 := artKey{Kind: artRune, ID: id, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.21"}
	if !s.artDidFail(k3) {
		t.Fatal("el 404 de la runa debía memorizarse")
	}

	// Cambio de ancho (Fase 3): otra clave del MISMO recurso.
	artRuneColsGuardado := runeArtColsPorDefecto
	_ = artRuneColsGuardado
	otra := artKey{Kind: artRune, ID: id, Cols: runeArtColsPorDefecto + 3, Mode: artHalfBlock, Patch: "16.21"}
	if !s.artDidFail(otra) {
		t.Error("un 404 de runa debe aplicar a todos los anchos del recurso")
	}

	// Y no debe volver a pedirse.
	if cmd2 := runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto); cmd2 != nil {
		_ = cmd2()
	}
	if p.solicitudes != 1 {
		t.Errorf("solicitudes = %d, se esperaba 1 (el recurso no depende del ancho)", p.solicitudes)
	}
}

// TestRuna404NoBloqueaItemMismoID: la runa y el ítem con el mismo ID son
// recursos distintos (garantía de artResource.Kind).
func TestRuna404NoBloqueaItemMismoID(t *testing.T) {
	p := &proveedorRunas{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionRunas2(p)
	const id = 8200

	_ = runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)

	kItem := artKey{Kind: artItem, ID: id, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	if s.artDidFail(kItem) {
		t.Error("un 404 de RUNA no debe bloquear el ÍTEM con el mismo id")
	}
	// El ítem debe poder pedirse.
	if cmd := itemArtCmdFor(s, []int{id}, smallArtCols); cmd == nil {
		t.Error("el ítem debía poder pedirse")
	}
}

// TestRunaFallosTransitoriosNoSeMemorizan: red, timeout, 429 y 5xx no
// deben impedir reintentar.
func TestRunaFallosTransitoriosNoSeMemorizan(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
	}{
		{"red", errors.New("dial tcp: connection refused")},
		{"timeout", errors.New("context deadline exceeded")},
		{"429", &ddragon.HTTPError{URL: "u", StatusCode: 429}},
		{"500", &ddragon.HTTPError{URL: "u", StatusCode: 500}},
		{"503", &ddragon.HTTPError{URL: "u", StatusCode: 503}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p := &proveedorRunas{err: c.err}
			s := sesionRunas2(p)
			const id = 8300
			_ = runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)()
			k := artKey{Kind: artRune, ID: id, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.21"}
			if s.artDidFail(k) {
				t.Errorf("%s no debe memorizarse como definitivo", c.nombre)
			}
			again := runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)
			if again == nil {
				t.Fatalf("%s debe permitir reintentar", c.nombre)
			}
			_ = again()
		})
	}
}

// TestRunaRespuestaTardiaEnSuClave: una respuesta que llega tarde se
// guarda bajo la clave que la pidió, no bajo el ancho vigente.
func TestRunaRespuestaTardiaEnSuClave(t *testing.T) {
	p := &proveedorRunas{raw: pngValido(t)}
	s := sesionRunas2(p)
	const id = 8400

	cmd := runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)
	if cmd == nil {
		t.Fatal("debía pedir")
	}
	msg := cmd().(artLoadedMsg)

	// La clave pedida debe estar en el mensaje.
	k := artKey{Kind: artRune, ID: id, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.21"}
	if _, ok := msg.Art[k]; !ok {
		t.Errorf("la respuesta no está bajo su clave %+v", k)
	}
}

// TestRunaTresCmdSinResolverUnaSolicitud: el primer interesado lanza la
// descarga; los otros dos anchos se suman como esperadores del MISMO
// recurso y no duplican. Contra 404 ⇒ 1 solicitud.
func TestRunaTresCmdSinResolverUnaSolicitud(t *testing.T) {
	p := &proveedorRunas{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionRunas2(p)
	const id = 8500

	// 1.er ancho: es quien reserva y recibe el Cmd.
	cmd := runeArtCmdFor(s, []int{id}, runeArtColsPorDefecto)
	if cmd == nil {
		t.Fatal("el primer ancho debía lanzar la descarga")
	}

	// 2.º y 3.er ancho del mismo recurso: esperan, no lanzan.
	base := artKey{Kind: artRune, ID: id, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.21"}
	for _, extra := range []int{1, 2} {
		k := base
		k.Cols = runeArtColsPorDefecto + extra
		if s.reserveArt(k) {
			t.Errorf("el ancho +%d no debía lanzar otra descarga", extra)
		}
	}

	// Una sola ejecución ⇒ una sola solicitud.
	_ = cmd()
	if p.solicitudes != 1 {
		t.Errorf("solicitudes = %d, se esperaba 1", p.solicitudes)
	}
}
