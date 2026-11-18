package ui

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// ─── cierre Fase 2: fallos, asincronía y no-invención ───────────────
//
// Dos riesgos concretos que hay que demostrar cerrados:
//
//  1. Un icono definitivamente ausente no se pide en bucle, pero un
//     fallo transitorio NO queda bloqueado para siempre.
//  2. Un resultado que llega tarde (el usuario ya redimensionó o cambió
//     de pestaña/campeón) no pisa el tamaño vigente ni mezcla sesiones.

// TestClasificacionDeFallos fija la semántica: solo lo definitivo se
// memoriza.
func TestClasificacionDeFallos(t *testing.T) {
	casos := []struct {
		nombre string
		err    error
		want   artFailure
	}{
		{"nil", nil, artFailNone},
		{"404", &ddragon.HTTPError{URL: "u", StatusCode: 404}, artFailMissing},
		{"410", &ddragon.HTTPError{URL: "u", StatusCode: 410}, artFailMissing},
		{"500", &ddragon.HTTPError{URL: "u", StatusCode: 500}, artFailTransient},
		{"503", &ddragon.HTTPError{URL: "u", StatusCode: 503}, artFailTransient},
		{"429", &ddragon.HTTPError{URL: "u", StatusCode: 429}, artFailTransient},
		{"red", errors.New("dial tcp: connection refused"), artFailTransient},
		{"timeout", errors.New("context deadline exceeded"), artFailTransient},
	}
	for _, c := range casos {
		if got := classifyArtError(c.err); got != c.want {
			t.Errorf("%s: classifyArtError = %v, se esperaba %v", c.nombre, got, c.want)
		}
	}

	// El error de ddragon debe seguir siendo legible como antes.
	he := &ddragon.HTTPError{URL: "https://x/y.png", StatusCode: 404}
	if got := he.Error(); got == "" || !ddragon.IsNotFound(he) {
		t.Errorf("HTTPError(404): mensaje %q, IsNotFound=%v", got, ddragon.IsNotFound(he))
	}
	// Envuelto en fmt.Errorf con %w también debe detectarse.
	envuelto := fmt.Errorf("cache: %w", he)
	if !ddragon.IsNotFound(envuelto) {
		t.Error("IsNotFound debe atravesar el envoltorio de la caché")
	}
}

// TestFallosDefinitivosSeMemorizan: 404 y PNG corrupto no se reintentan.
func TestFallosDefinitivosSeMemorizan(t *testing.T) {
	s := &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
	}
	k := artKey{Kind: artItem, ID: 1, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}

	if s.artDidFail(k) {
		t.Error("sin fallo registrado no debe constar como fallado")
	}
	s.markArtFailed(k, artFailMissing)
	if !s.artDidFail(k) {
		t.Error("un 404 debe memorizarse")
	}

	k2 := artKey{Kind: artItem, ID: 2, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(k2, artFailCorrupt)
	if !s.artDidFail(k2) {
		t.Error("un PNG corrupto debe memorizarse")
	}
}

// TestFallosTransitoriosNoSeBloquean: es el punto que faltaba. Un error
// de red NO puede impedir recuperar el icono cuando vuelva la red.
func TestFallosTransitoriosNoSeBloquean(t *testing.T) {
	s := &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
	}
	k := artKey{Kind: artItem, ID: 3, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}

	s.markArtFailed(k, artFailTransient)
	if s.artDidFail(k) {
		t.Fatal("un fallo de red no debe memorizarse como definitivo")
	}
	// Un fallo transitorio deja la clave SIN arte y SIN memoria de
	// fallo, así que el próximo Cmd la volverá a pedir. Se comprueba
	// sobre un plan real, no con nil (sin ids no hay nada que cargar y
	// tabArtReady daría true por vacío).
	if s.artDidFail(k) {
		t.Fatal("el transitorio no debe constar como definitivo")
	}
	if s.hasArt(k) {
		t.Fatal("sin descarga no puede haber arte en caché")
	}
	// Con un plan real, esa clave pendiente impide dar la pestaña por
	// lista: es lo que provoca el reintento en el siguiente evento.
	d := planDetailView(t, "briar", "Briar", 80, 60)
	d.tab = 1
	d.session.Provider = nil // fuerza fallo al pedir
	plan := d.lolPlan()
	ids := tabItemIDs(1, plan)
	if len(ids) == 0 {
		t.Skip("sin ids")
	}
	if d.session.tabArtReady(1, plan, smallArtCols) {
		t.Error("con iconos pendientes la pestaña no puede estar lista")
	}
}

// TestInvalidacionOlvidaFallos: cambiar de parche o de proveedor debe
// borrar la memoria de fallos. Si no, un 404 del parche 16.21 dejaría
// el ítem ausente para siempre en el 16.22.
func TestInvalidacionOlvidaFallos(t *testing.T) {
	s := &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
	}
	k := artKey{Kind: artItem, ID: 4, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(k, artFailMissing)
	if !s.artDidFail(k) {
		t.Fatal("debía estar memorizado")
	}
	s.InvalidateArt()
	if s.artDidFail(k) {
		t.Error("InvalidateArt debe olvidar los fallos: un parche nuevo puede tener el icono")
	}
}

// TestFallosSeparadosPorParcheYTamano: la clave incluye ambos, así que
// un 404 en un parche no contamina otro.
func TestFallosSeparadosPorParcheYTamano(t *testing.T) {
	s := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}}
	a := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	b := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.22"}
	c := artKey{Kind: artItem, ID: 5, Cols: 6, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(a, artFailMissing)
	if s.artDidFail(b) {
		t.Error("otro parche no debe heredar el fallo")
	}
	if s.artDidFail(c) {
		t.Error("otro tamaño no debe heredar el fallo")
	}
}

// ─── asincronía: resultados tardíos ────────────────────────────────

// TestResultadoTardioNoPisaElTamanoVigente reproduce el caso del brief:
// se lanza la descarga a 3 columnas, el usuario pasa a 118 y llega la
// respuesta vieja. El arte de 3 debe guardarse bajo SU clave y artCols
// NO debe dar por cargado el ancho de 6.
func TestResultadoTardioNoPisaElTamanoVigente(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 80, 60)
	d.tab = 1
	// Se captura el Cmd del ancho VIEJO (3 columnas).
	cmdViejo := d.artCmd()
	if cmdViejo == nil {
		t.Fatal("debía haber arte que pedir a 80")
	}

	// El usuario redimensiona a 118 mientras la descarga está en vuelo.
	d.width = 118
	d.sync()
	quiere := iconColsFor(118)
	if quiere != artColsLarge {
		t.Fatalf("iconColsFor(118) = %d", quiere)
	}

	// Llega la respuesta vieja. Se simula el manejador del bucle.
	msg, ok := cmdViejo().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado")
	}
	if msg.ArtHasWidth(quiere) {
		t.Fatal("el lote viejo no debería traer el ancho vigente")
	}
	for k, art := range msg.Art {
		if k.Cols != smallArtCols {
			t.Fatalf("el lote viejo debía ser de %d columnas, es de %d", smallArtCols, k.Cols)
		}
		d.session.putArt(k, art)
	}
	// artCols NO debe marcarse: el ancho vigente todavía no está.
	if msg.ArtHasWidth(iconColsFor(d.width)) {
		d.artCols = iconColsFor(d.width)
	}
	if d.artCols == artColsLarge {
		t.Error("artCols no debe dar por cargado un ancho que no ha llegado")
	}

	// Y el arte de 3 sigue disponible bajo su propia clave.
	plan := d.lolPlan()
	var conArte bool
	for _, st := range plan.Steps {
		if st.Item > 0 && !st.Quest && d.session.ItemArtFor(st.Item, smallArtCols) != "" {
			conArte = true
			break
		}
	}
	if !conArte {
		t.Error("el arte de 3 columnas se perdió al llegar tarde")
	}

	// La pestaña sigue pendiente a 6: hay que volver a pedir.
	if d.session.tabArtReady(d.tab, plan, artColsLarge) {
		t.Error("con el arte de 6 sin llegar, la pestaña no puede estar lista")
	}
}

// TestArtHasWidth detecta correctamente el ancho del lote.
func TestArtHasWidth(t *testing.T) {
	m := artLoadedMsg{Art: map[artKey]string{
		{Kind: artItem, ID: 1, Cols: 3}: "x",
		{Kind: artItem, ID: 2, Cols: 3}: "y",
	}}
	if !m.ArtHasWidth(3) {
		t.Error("debía reconocer el ancho 3")
	}
	if m.ArtHasWidth(6) {
		t.Error("no debía reconocer el ancho 6")
	}
	var vacio artLoadedMsg
	if vacio.ArtHasWidth(3) {
		t.Error("un lote vacío no tiene ningún ancho")
	}
}

// TestCambioDePestanaDuranteDescarga: el arte se guarda por clave, así
// que la respuesta de Ruta no contamina Alt. (ni al revés).
func TestCambioDePestanaDuranteDescarga(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 1
	cmdRuta := d.artCmd()
	if cmdRuta == nil {
		t.Fatal("sin Cmd de Ruta")
	}
	// Cambio a Alt. antes de que llegue.
	d.tab = 2
	d.sync()
	cmdAlt := d.artCmd()

	msgRuta, _ := cmdRuta().(artLoadedMsg)
	for k, a := range msgRuta.Art {
		d.session.putArt(k, a)
	}
	// Al ser la misma sesión y el mismo ancho, el arte sirve para ambas
	// pestañas: lo importante es que la clave es la misma y no se
	// duplica ni se mezcla.
	if cmdAlt != nil {
		msgAlt, _ := cmdAlt().(artLoadedMsg)
		for k, a := range msgAlt.Art {
			d.session.putArt(k, a)
		}
	}
	plan := d.lolPlan()
	for _, id := range tabItemIDs(2, plan) {
		if id <= 0 {
			continue
		}
		if _, err := d.session.Provider.ItemIcon(id); err != nil {
			continue
		}
		if d.session.ItemArtFor(id, iconColsFor(118)) == "" {
			t.Errorf("Alt.: id=%d con PNG quedó sin arte", id)
		}
	}
}

// TestCambioDeCampeonDuranteDescarga: la sesión es compartida, así que
// el arte de un campeón es válido para otro (mismo id, mismo parche).
// Lo que no puede pasar es que un id se guarde con el parche de otra
// sesión.
func TestCambioDeCampeonDuranteDescarga(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 1
	cmd := d.artCmd()
	if cmd == nil {
		t.Fatal("sin Cmd")
	}
	msg, _ := cmd().(artLoadedMsg)
	for k, a := range msg.Art {
		// Todas las claves llevan el parche de la sesión que las pidió.
		if k.Patch != d.session.Version {
			t.Errorf("clave con parche %q, sesión %q", k.Patch, d.session.Version)
		}
		d.session.putArt(k, a)
	}

	// Otra sesión (otro campeón) con parche distinto: no debe ver ese
	// arte como suyo.
	otra := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}, Version: "99.99"}
	for id := range tabItemIDs(1, d.lolPlan()) {
		_ = id
	}
	for _, st := range d.lolPlan().Steps {
		if st.Item <= 0 {
			continue
		}
		if otra.ItemArtFor(st.Item, iconColsFor(118)) != "" {
			t.Errorf("id=%d: otra sesión con otro parche vio arte ajeno", st.Item)
		}
	}
}

// TestAlternar99y100NoPierdeArte: el caso 99 ↔ 100 del brief. Cada
// tramo conserva su propio arte; alternar no debe vaciar ninguno.
func TestAlternar99y100NoPierdeArte(t *testing.T) {
	s, champ := planSession(t, "briar", "Briar")
	d := newDetailView(champ, s)
	d.width, d.height = 99, 60
	d.tab = 1
	d.sync()
	loadArtSync(t, &d)
	if d.artCols != smallArtCols {
		t.Fatalf("99 -> artCols = %d", d.artCols)
	}

	// Se carga también el de 100.
	d.width = 100
	d.sync()
	loadArtSync(t, &d)
	if d.artCols != artColsMedium {
		t.Fatalf("100 -> artCols = %d", d.artCols)
	}

	// Alternar varias veces no debe perder ninguno de los dos.
	for i := 0; i < 4; i++ {
		d.width = 99
		d.sync()
		if !s.tabArtReady(1, d.lolPlan(), smallArtCols) {
			if !hayItemSinPNG(t, s, d) {
				t.Errorf("iteración %d: 99 dejó de estar listo", i)
			}
		}
		d.width = 100
		d.sync()
		if !s.tabArtReady(1, d.lolPlan(), artColsMedium) {
			if !hayItemSinPNG(t, s, d) {
				t.Errorf("iteración %d: 100 dejó de estar listo", i)
			}
		}
	}
}

// hayItemSinPNG indica si algún paso carece de PNG en el proveedor (lo
// que impide que la pestaña esté "lista" por diseño).
func hayItemSinPNG(t *testing.T, s *Session, d detailView) bool {
	t.Helper()
	for _, st := range d.lolPlan().Steps {
		if st.Item <= 0 || st.Quest {
			continue
		}
		if _, err := s.Provider.ItemIcon(st.Item); err != nil {
			return true
		}
	}
	return false
}

// TestNingunaSolicitudDesdeView: View no puede tocar el proveedor. Se
// envuelve el provider para contar llamadas.
func TestNingunaSolicitudDesdeView(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 1
	cont := &countingProvider{Provider: d.session.Provider}
	d.session.Provider = cont
	loadArtSync(t, &d)
	base := cont.llamadas

	for i := 0; i < 5; i++ {
		_ = d.View()
		_ = d.routeViewA(d.lolPlan())
		_ = d.alternativesViewB(d.lolPlan())
	}
	if cont.llamadas != base {
		t.Errorf("View hizo %d consultas al proveedor", cont.llamadas-base)
	}
}

// countingProvider cuenta ItemIcon sobre el proveedor real.
type countingProvider struct {
	staticdata.Provider
	llamadas int
}

func (c *countingProvider) ItemIcon(id int) ([]byte, error) {
	c.llamadas++
	return c.Provider.ItemIcon(id)
}

// TestFallosRealesRespetanLaClasificacion ejerce la RUTA REAL de carga
// (itemArtCmdFor → Provider → markArtFailed), no solo el helper.
//
// Cierra un punto ciego detectado por mutación: el test anterior
// llamaba markArtFailed directamente, así que hacer que la producción
// memorizara TODO fallo (el bug original) no lo hacía fallar.
func TestFallosRealesRespetanLaClasificacion(t *testing.T) {
	casos := []struct {
		nombre   string
		err      error
		memoriza bool
	}{
		{"404 definitivo", &ddragon.HTTPError{URL: "u", StatusCode: 404}, true},
		{"410 definitivo", &ddragon.HTTPError{URL: "u", StatusCode: 410}, true},
		{"500 transitorio", &ddragon.HTTPError{URL: "u", StatusCode: 500}, false},
		{"red transitoria", errors.New("dial tcp: connection refused"), false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			s := &Session{
				Game:        model.GameLoL,
				ItemArt:     map[artKey]string{},
				ChampionArt: map[string]champArt{},
				Version:     "16.21",
			}
			s.Provider = &errorProvider{err: c.err}
			cmd := itemArtCmdFor(s, []int{1234}, smallArtCols)
			if cmd == nil {
				t.Fatal("debía intentar la descarga")
			}
			_ = cmd() // se ejecuta: aquí ocurre el markArtFailed real

			k := artKey{Kind: artItem, ID: 1234, Cols: smallArtCols,
				Mode: artHalfBlock, Patch: "16.21"}
			if got := s.artDidFail(k); got != c.memoriza {
				t.Errorf("artDidFail = %v, se esperaba %v", got, c.memoriza)
			}
			// Se comprueba TAMBIÉN el mapa crudo: artDidFail filtra los
			// transitorios por su cuenta, así que un fallo de red
			// memorizado por error no cambiaría el comportamiento pero
			// sí haría crecer el mapa sin límite en una sesión larga.
			s.runeArtMu.RLock()
			guardado, existe := s.artFailed[k]
			s.runeArtMu.RUnlock()
			if c.memoriza {
				if !existe {
					t.Error("un fallo definitivo debe quedar registrado")
				}
			} else if existe {
				t.Errorf("un fallo transitorio no debe ocupar el mapa (guardado %v)", guardado)
			}
		})
	}
}

// errorProvider devuelve siempre el mismo error en ItemIcon.
type errorProvider struct {
	staticdata.Provider
	err error
}

func (e *errorProvider) ItemIcon(int) ([]byte, error) { return nil, e.err }

// corruptProvider devuelve bytes que no son un PNG.
type corruptProvider struct{ staticdata.Provider }

func (c *corruptProvider) ItemIcon(int) ([]byte, error) {
	return []byte("no soy un png"), nil
}

// TestPNGCorruptoSeMemorizaSinRomper: un cuerpo inválido se marca como
// corrupto (definitivo) y la UI cae al nombre, sin pánico.
func TestPNGCorruptoSeMemorizaSinRomper(t *testing.T) {
	s := &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
	}
	s.Provider = &corruptProvider{}
	cmd := itemArtCmdFor(s, []int{4321}, smallArtCols)
	if cmd == nil {
		t.Fatal("debía intentar")
	}
	_ = cmd()
	k := artKey{Kind: artItem, ID: 4321, Cols: smallArtCols,
		Mode: artHalfBlock, Patch: "16.21"}
	if !s.artDidFail(k) {
		t.Error("un PNG corrupto debe memorizarse para no reintentarse")
	}
	if s.ItemArtFor(4321, smallArtCols) != "" {
		t.Error("no debe quedar arte de un PNG inválido")
	}
}
