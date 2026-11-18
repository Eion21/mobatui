package ui

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
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

	// Un PNG ilegible NO se memoriza: la caché de Data Dragon guarda el
	// cuerpo con HTTP 200 y TTL largo, así que un truncamiento no se
	// distingue de una corrupción permanente. Bloquearlo dejaría el
	// icono muerto para siempre; se deja reintentable.
	k2 := artKey{Kind: artItem, ID: 2, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(k2, artFailCorrupt)
	if s.artDidFail(k2) {
		t.Error("un PNG ilegible no debe bloquearse de forma permanente")
	}
	// Se comprueba el mapa CRUDO: artDidFail filtra por su cuenta, así
	// que memorizar el corrupto por error no cambiaría el
	// comportamiento pero sí haría crecer el mapa sin límite y, sobre
	// todo, ocultaría que la clasificación dejó de respetarse.
	s.runeArtMu.RLock()
	_, existeCorrupto := s.artFailed[s.resourceForLocked(k2)]
	s.runeArtMu.RUnlock()
	if existeCorrupto {
		t.Error("un PNG ilegible no debe ocupar el mapa de fallos definitivos")
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

// TestFallosCompartidosEntreTamanos: el recurso descargado NO depende
// de las columnas (ver artResource), así que un 404 a 3 columnas tiene
// que servir también para 4 y 6. Si no, cambiar de ancho redescarga el
// mismo 404.
//
// Antes existía TestFallosSeparadosPorParcheYTamano exigiendo aislar el
// 404 por tamaño. Era incorrecto: la URL es la misma para todos los
// anchos.
func TestFallosCompartidosEntreTamanos(t *testing.T) {
	s := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}, Version: "16.21"}
	a := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	b := artKey{Kind: artItem, ID: 5, Cols: 4, Mode: artHalfBlock, Patch: "16.21"}
	c := artKey{Kind: artItem, ID: 5, Cols: 6, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(a, artFailMissing)

	if !s.artDidFail(a) {
		t.Fatal("el 404 debe constar para el ancho que lo descubrió")
	}
	if !s.artDidFail(b) || !s.artDidFail(c) {
		t.Error("un 404 debe compartirse entre 3, 4 y 6 columnas: es el mismo PNG")
	}
	// El modo tampoco cambia el recurso.
	if !s.artDidFail(artKey{Kind: artItem, ID: 5, Cols: 3,
		Mode: artPlain, Patch: "16.21"}) {
		t.Error("el modo de render no cambia el recurso descargado")
	}
}

// TestFallosSeparadosPorParcheYProveedor: lo que SÍ cambia el recurso
// (parche, proveedor, id) debe permitir una nueva solicitud.
func TestFallosSeparadosPorParcheYProveedor(t *testing.T) {
	s := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}, Version: "16.21"}
	a := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	s.markArtFailed(a, artFailMissing)
	if !s.artDidFail(a) {
		t.Fatal("debía constar")
	}

	// Otro parche: otro recurso, otra solicitud permitida.
	otroParche := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.22"}
	if s.artDidFail(otroParche) {
		t.Error("otro parche no debe heredar el 404")
	}
	// Otro id: otro recurso.
	if s.artDidFail(artKey{Kind: artItem, ID: 6, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}) {
		t.Error("otro id no debe heredar el 404")
	}
	// Otro proveedor: otro recurso (misma id/parche).
	s.Provider = &errorProvider{err: errors.New("x")}
	if s.artDidFail(a) {
		t.Error("un cambio de proveedor debe permitir volver a pedir")
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
			guardado, existe := s.artFailed[s.resourceForLocked(k)]
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

// TestPNGCorruptoNoRompeYEsRecuperable: un cuerpo inválido no pinta
// nada (la UI cae al nombre, sin pánico) y NO se bloquea para siempre:
// si el mismo recurso vuelve válido, un intento explícito lo recupera.
func TestPNGCorruptoNoRompeYEsRecuperable(t *testing.T) {
	s := &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
	}
	// Primero sirve bytes truncados; luego un PNG válido.
	p := &secuenciaProvider{respuestas: [][]byte{[]byte("\x89PNG truncado")}}
	s.Provider = p

	if cmd := itemArtCmdFor(s, []int{4321}, smallArtCols); cmd != nil {
		_ = cmd()
	}
	if got := s.ItemArtFor(4321, smallArtCols); got != "" {
		t.Errorf("no debe quedar arte de un PNG inválido: %q", got)
	}
	k := artKey{Kind: artItem, ID: 4321, Cols: smallArtCols,
		Mode: artHalfBlock, Patch: "16.21"}
	if s.artDidFail(k) {
		t.Fatal("un PNG ilegible no debe bloquearse permanentemente")
	}

	// Segundo intento explícito con bytes válidos: debe recuperarse.
	// El Cmd devuelve el arte; publicarlo es responsabilidad del bucle
	// (putArt), igual que hace artLoadedMsg en producción.
	p.respuestas = [][]byte{pngValido(t)}
	cmd := itemArtCmdFor(s, []int{4321}, smallArtCols)
	if cmd == nil {
		t.Fatal("debía reintentar: el fallo es recuperable")
	}
	msg, ok := cmd().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado")
	}
	if len(msg.Art) == 0 {
		t.Fatal("el segundo intento no produjo arte")
	}
	for k, a := range msg.Art {
		s.putArt(k, a)
	}
	if s.ItemArtFor(4321, smallArtCols) == "" {
		t.Error("un intento explícito posterior debe recuperar el icono válido")
	}
}

// pngValido genera un PNG real de 64x64 para las pruebas.
func pngValido(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{200, 40, 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// secuenciaProvider devuelve respuestas en orden y repite la última.
type secuenciaProvider struct {
	staticdata.Provider
	respuestas [][]byte
	i          int
}

func (p *secuenciaProvider) ItemIcon(int) ([]byte, error) {
	if len(p.respuestas) == 0 {
		return nil, errors.New("sin respuestas")
	}
	if p.i >= len(p.respuestas) {
		p.i = len(p.respuestas) - 1
	}
	r := p.respuestas[p.i]
	if p.i < len(p.respuestas)-1 {
		p.i++
	}
	return r, nil
}

// Test404NoSeRedescargaAlCambiarDeAncho es la prueba de impacto del
// defecto encontrado: antes, un 404 a 3 columnas volvía a descargarse al
// pasar a 4 o 6 (2 descargas del mismo recurso). El recurso no depende
// del ancho, así que debe ser UNA sola descarga.
func Test404NoSeRedescargaAlCambiarDeAncho(t *testing.T) {
	s := &Session{
		Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}, Version: "16.21",
	}
	p := &contadorProvider{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s.Provider = p

	// Recorrido completo de la escala: 3 -> 4 -> 6 columnas.
	for _, cols := range []int{smallArtCols, artColsMedium, artColsLarge} {
		if cmd := itemArtCmdFor(s, []int{9999}, cols); cmd != nil {
			_ = cmd()
		}
	}
	if p.n != 1 {
		t.Errorf("el mismo 404 se descargó %d veces; debe ser 1 (el recurso no depende del ancho)", p.n)
	}

	// Y sigue sin pedirse al volver a un ancho ya probado.
	if cmd := itemArtCmdFor(s, []int{9999}, smallArtCols); cmd != nil {
		t.Error("volver a 3 columnas no debe relanzar la descarga de un 404 conocido")
	}
	if p.n != 1 {
		t.Errorf("descargas = %d, se esperaba 1", p.n)
	}
}

// contadorProvider cuenta ItemIcon y devuelve siempre el mismo error.
type contadorProvider struct {
	staticdata.Provider
	err error
	n   int
}

func (c *contadorProvider) ItemIcon(int) ([]byte, error) {
	c.n++
	return nil, c.err
}

// ─── identidad del recurso: tipo de arte y origen ───────────────────

// Test404DeItemNoBloqueaLaRuna: artFailed es un mapa único para ítems y
// runas, y sus IDs viven en espacios distintos pero pueden coincidir
// numéricamente (p.ej. 8000). Sin Kind en la identidad, un 404 de ítem
// dejaba sin icono a la runa del mismo id.
func Test404DeItemNoBloqueaLaRuna(t *testing.T) {
	s := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		RuneArt: map[artKey]string{}, ChampionArt: map[string]champArt{},
		Version: "16.21"}

	item := artKey{Kind: artItem, ID: 8000, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}
	runa := artKey{Kind: artRune, ID: 8000, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}

	s.markArtFailed(item, artFailMissing)
	if !s.artDidFail(item) {
		t.Fatal("el 404 del ítem debe constar")
	}
	if s.artDidFail(runa) {
		t.Error("la runa con el mismo id no debe heredar el fallo del ítem")
	}

	// Y al revés.
	s2 := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		RuneArt: map[artKey]string{}, ChampionArt: map[string]champArt{},
		Version: "16.21"}
	s2.markArtFailed(runa, artFailMissing)
	if s2.artDidFail(item) {
		t.Error("el ítem no debe heredar el fallo de la runa")
	}
}

// TestOrigenDistingueInstanciasDelMismoTipo: dos clientes ddragon con
// distinto baseURL son orígenes distintos. Indexar por tipo (%T) hacía
// que un 404 contra un CDN dejara sin icono el mismo id servido por otro.
func TestOrigenDistingueInstanciasDelMismoTipo(t *testing.T) {
	nuevo := func() *ddragon.Client {
		c, err := cache.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return ddragon.New(c, t.TempDir(), "es_ES")
	}
	p1, p2 := nuevo(), nuevo()
	if fmt.Sprintf("%T", p1) != fmt.Sprintf("%T", p2) {
		t.Skip("el test necesita dos instancias del mismo tipo")
	}

	s := &Session{Game: model.GameLoL, ItemArt: map[artKey]string{},
		ChampionArt: map[string]champArt{}, Version: "16.21"}
	k := artKey{Kind: artItem, ID: 5, Cols: 3, Mode: artHalfBlock, Patch: "16.21"}

	s.Provider = p1
	s.markArtFailed(k, artFailMissing)
	if !s.artDidFail(k) {
		t.Fatal("debía constar con p1")
	}
	s.Provider = p2
	if s.artDidFail(k) {
		t.Error("otra instancia del mismo tipo es otro origen: no debe compartir el 404")
	}
}
