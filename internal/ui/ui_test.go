package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
	"github.com/Eion21/mobatui/internal/ugg"
)

// stubIconos sirve un PNG mínimo para cualquier icono de ítem: así el
// test de pre-render no depende de assets reales.
type stubIconos struct {
	staticdata.Provider
}

func (s stubIconos) ItemIcon(int) ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// keyMsg construye pulsaciones para los tests de transición.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errNoNet
}

type noNetErr struct{}

func (noNetErr) Error() string { return "sin red (test)" }

var errNoNet = noNetErr{}

// testLoader construye el loader con Data Dragon sin red (respaldo
// local) y el proveedor de Deadlock desde fixtures.
func testLoader(t *testing.T) Loader {
	t.Helper()
	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	c.Offline = true
	// Transport fallido: ejercita el respaldo local también en el
	// primer fetch de Data Dragon.
	ddc := ddragon.New(c, "../../testdata/ddragon", "es_ES").
		WithHTTPClient(&http.Client{Transport: failTransport{}})
	return Loader{
		UggDir:      "../../testdata/ugg",
		DeadlockDir: "../../testdata/deadlock/builds",
		Providers: map[model.Game]staticdata.Provider{
			model.GameLoL:      ddc,
			model.GameDeadlock: deadlock.New("../../testdata/deadlock"),
		},
	}
}

func loadOffline(t *testing.T, g model.Game) *Session {
	t.Helper()
	s, err := LoadSession(g, testLoader(t))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func findChampion(t *testing.T, s *Session, key string) *ChampEntry {
	t.Helper()
	for _, e := range s.Champions {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("campeón/héroe %q no encontrado", key)
	return nil
}

// stubNombresEnvuelve un proveedor real y aporta el mapa id→nombre
// completo que usan los cruces (matchups/sinergias).
type stubNombres struct {
	staticdata.Provider
	byID map[int]string
}

func (s stubNombres) HeroNamesByID() map[int]string { return s.byID }

// TestSesionResuelveNombresParaCruces: cuando el proveedor trae el
// mapa id→nombre completo, la sesión lo usa para matchups/sinergias
// aunque el roster no liste al héroe (Violet/Baba in_development).
func TestSesionResuelveNombresParaCruces(t *testing.T) {
	l := testLoader(t)
	l.Providers[model.GameDeadlock] = stubNombres{
		Provider: l.Providers[model.GameDeadlock],
		byID:     map[int]string{88: "Baba", 86: "Violet"},
	}
	s, err := LoadSession(model.GameDeadlock, l)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Names.HeroNameByID[88]; got != "Baba" {
		t.Errorf("HeroNameByID[88] = %q, se esperaba Baba", got)
	}
	if got := s.Names.HeroNameByID[86]; got != "Violet" {
		t.Errorf("HeroNameByID[86] = %q, se esperaba Violet", got)
	}
}

// TestItemArtIncluyeWards: el ward recomendado se pre-renderiza junto
// a core y botas; sin esto la tarjeta del ward cae al icono fallback.
func TestItemArtIncluyeWards(t *testing.T) {
	l := testLoader(t)
	l.Providers[model.GameLoL] = stubIconos{Provider: l.Providers[model.GameLoL]}
	l.BuildLoaders = map[model.Game]func() ([]*model.ChampionBuilds, error){
		model.GameLoL: func() ([]*model.ChampionBuilds, error) {
			cbs, err := ugg.LoadDir("../../testdata/ugg")
			if err != nil {
				return nil, err
			}
			for _, cb := range cbs {
				if cb.ChampionKey == "Ahri" {
					cb.Wards = []model.ItemStats{{
						ID:    2055,
						Stats: model.Stats{Games: 20000, Wins: 11000},
					}}
				}
			}
			return cbs, nil
		},
	}
	s, err := LoadSession(model.GameLoL, l)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.ItemArt[2055]; !ok {
		t.Error("el ward 2055 no fue pre-renderizado junto a core y botas")
	}
}

func TestLoadSessionLoL(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	if s.Game != model.GameLoL {
		t.Errorf("game = %s", s.Game)
	}
	if len(s.Builds) != 21 {
		t.Errorf("builds = %d, se esperaban 21", len(s.Builds))
	}
	if len(s.Champions) != 14 {
		t.Errorf("campeones = %d, se esperaban 14", len(s.Champions))
	}
	// Un campeón por entrada, con sus roles en orden canónico.
	ahri := findChampion(t, s, "Ahri")
	if len(ahri.Roles) != 2 || ahri.Roles[0].String() != "MIDDLE" || ahri.Roles[1].String() != "SUPPORT" {
		t.Errorf("roles de Ahri = %v", ahri.Roles)
	}
	// Nombres reales resueltos, no IDs.
	if got := s.ItemName(3089); got != "Sombrero mortal de Rabadon" {
		t.Errorf("ItemName(3089) = %q", got)
	}
	if got := s.RuneName(8112); got != "Electrocutar" {
		t.Errorf("RuneName(8112) = %q", got)
	}
	// Arte pre-renderizado: mini half-block para la cabecera y
	// variantes ASCII grandes para el toggle `a`.
	art, ok := s.ChampionArt["Ahri"]
	if !ok || !strings.Contains(art.Mini, "▀") {
		t.Fatalf("sin mini half-block para Ahri: %+v", art)
	}
	if !strings.Contains(art.ASCII, "\x1b[38;2;") {
		t.Error("sin variante ASCII con color")
	}
	if !strings.Contains(art.Mono, "\x1b[38;2;") {
		t.Error("sin variante ASCII mono")
	}
}

func TestLoadSessionDeadlock(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	if s.Game != model.GameDeadlock {
		t.Fatalf("game = %s", s.Game)
	}
	if len(s.Champions) != 14 {
		t.Errorf("héroes = %d, se esperaban 14", len(s.Champions))
	}
	if len(s.Builds) != 14 {
		t.Errorf("builds = %d, se esperaban 14", len(s.Builds))
	}
	abrams := findChampion(t, s, "Abrams")
	if abrams.Name != "Abrams" || len(abrams.Roles) != 1 || abrams.Roles[0] != model.RoleAll {
		t.Errorf("Abrams mal agrupado: %+v", abrams)
	}
	// Nombres reales de ítems de Deadlock resueltos desde fixtures.
	if got := s.ItemName(101); got != "Salud extra" {
		t.Errorf("ItemName(101) = %q", got)
	}
	// Sin runas: fallback con ID.
	if got := s.RuneName(8112); got != "Runa 8112" {
		t.Errorf("RuneName en Deadlock = %q", got)
	}
	// Arte pre-renderizado para héroes e ítems: mini half-block y
	// variantes ASCII grandes.
	art, ok := s.ChampionArt["Abrams"]
	if !ok || !strings.Contains(art.Mini, "▀") {
		t.Fatalf("sin mini half-block para Abrams: %+v", art)
	}
	if strings.Contains(art.ASCII, "▀") {
		t.Error("la variante ASCII no debía usar half-blocks")
	}
	if _, ok := s.ItemArt[101]; !ok {
		t.Error("sin arte para el ítem 101")
	}
}

func TestDetailViewRenderLoL(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.height = 50
	v := d.View()
	// Cabecera + pestañas + pestaña ítems por defecto.
	for _, want := range []string{
		"Ahri", "MIDDLE", "SUPPORT",
		"1 Ítems", "2 Resumen", "3 Runas", "4 Habilidades",
		"CORE", "max", "WR",
		// La línea de selección muestra el nombre completo del ítem
		// top (las tarjetas truncan). El core son los 3 más comprados,
		// así que el top es el 6653, no Rabadon.
		"Companyera de Luden",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("vista LoL sin %q", want)
		}
	}
	if strings.Contains(v, "█") {
		t.Error("la barra de WR debía haber desaparecido")
	}
	if strings.Contains(v, "Ítem ") {
		t.Error("la vista mostró un ID sin resolver (Ítem NNNN)")
	}
	// Pestaña runas: keystone protagonista y árboles.
	m, _ := d.Update(keyMsg("3"))
	v = m.View()
	for _, want := range []string{"Electrocutar", "Dominación", "Brujería", "fragmentos"} {
		if !strings.Contains(v, want) {
			t.Errorf("pestaña runas sin %q", want)
		}
	}
	// Pestaña habilidades: rejilla con marcas y niveles.
	m, _ = m.Update(keyMsg("4"))
	v = m.View()
	for _, want := range []string{"◆", "max", "18"} {
		if !strings.Contains(v, want) {
			t.Errorf("pestaña habilidades sin %q", want)
		}
	}
	// Pestaña resumen: lo jugable en una pantalla. La fuente de los
	// fixtures (UGG) no trae hechizos ni orden de compra: esas
	// secciones no se dibujan.
	m, _ = m.Update(keyMsg("2"))
	v = m.View()
	for _, want := range []string{"RUNA CLAVE", "BOTAS", "HABILIDADES"} {
		if !strings.Contains(v, want) {
			t.Errorf("pestaña resumen sin %q", want)
		}
	}
	for _, unwanted := range []string{"HECHIZOS", "ORDEN DE COMPRA"} {
		if strings.Contains(v, unwanted) {
			t.Errorf("pestaña resumen mostró %q sin datos de la fuente", unwanted)
		}
	}
}

func TestDetailViewRenderDeadlock(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	d := newDetailView(abrams, s)
	d.height = 50
	v := d.View()
	for _, want := range []string{
		"Abrams",
		"1 Ítems", "2 Resumen", "3 Matchups", "4 Habilidades",
		// Sin cargador de rutas, se muestran estadísticas sin fases inventadas.
		"Ruta no disponible — mostrando estadísticas generales",
		"Compras frecuentes",
		"max",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("vista Deadlock sin %q", want)
		}
	}
	// Sin roles ni runas: esas secciones no deben aparecer.
	for _, unwanted := range []string{"roles", "Runas", "BOTAS", "CORE", "EARLY", "MID", "LATE"} {
		if strings.Contains(v, unwanted) {
			t.Errorf("vista Deadlock mostró %q, que no aplica al juego", unwanted)
		}
	}
	// La trampa (Maldición 3/3, 100% WR) queda fuera del ranking.
	if strings.Contains(v, "Maldición") {
		t.Error("ítem trampa 306 (3 partidas, 100% WR) no debía pasar el filtro")
	}
	// Pestaña matchups con fixtures: sin datos, estado vacío claro.
	m, _ := d.Update(keyMsg("3"))
	if v := m.View(); !strings.Contains(v, "sin datos de matchups") {
		t.Error("la pestaña matchups no mostró estado vacío con fixtures")
	}
	// Pestaña habilidades: rejilla con los slots del héroe.
	m, _ = m.Update(keyMsg("4"))
	if v := m.View(); !strings.Contains(v, "◆") {
		t.Error("la rejilla de habilidades no tiene marcas")
	}
}

func TestDetailViewCambiaRolYNavegaTarjetas(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.height = 50
	if got := d.CurrentBuild().Role.String(); got != "MIDDLE" {
		t.Fatalf("rol inicial = %s, se esperaba MIDDLE", got)
	}
	m, _ := d.Update(keyMsg("]"))
	d = m
	if got := d.CurrentBuild().Role.String(); got != "SUPPORT" {
		t.Fatalf("tras ], rol = %s, se esperaba SUPPORT", got)
	}
	if !strings.Contains(d.View(), "Renovador de piedra lunar") {
		t.Error("el rol SUP no mostró el ítem de support top")
	}
	m, _ = d.Update(keyMsg("["))
	if got := m.CurrentBuild().Role.String(); got != "MIDDLE" {
		t.Errorf("tras [, rol = %s", got)
	}
	// Navegación de tarjetas: la línea de detalle cambia con →.
	before := m.selectionLine()
	m2, _ := m.Update(keyMsg("right"))
	if m2.selectionLine() == before {
		t.Error("→ no movió la selección de tarjeta")
	}
}

func TestDetailViewPopupYVim(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	// Enter abre el popup de la tarjeta activa.
	m, cmd := d.Update(keyMsg("enter"))
	if !m.popupOpen() {
		t.Fatal("enter no abrió el popup")
	}
	for _, want := range []string{"Wilson", "muestra", "esc cierra"} {
		if !strings.Contains(m.View(), want) {
			t.Errorf("popup sin %q", want)
		}
	}
	// Esc lo cierra.
	m2, _ := m.Update(keyMsg("esc"))
	if m2.popupOpen() {
		t.Error("esc no cerró el popup")
	}
	// El toggle de retrato devuelve un comando de toast.
	_, cmd = d.Update(keyMsg("a"))
	if cmd == nil {
		t.Error("el toggle de retrato no notifica")
	}
	// hjkl mueve tarjetas como las flechas.
	if m, _ = d.Update(keyMsg("l")); m.itemIdx != 1 {
		t.Errorf("l: itemIdx = %d", m.itemIdx)
	}
	if m, _ = m.Update(keyMsg("k")); m.itemIdx != 0 { // width 80 → 2 por fila
		t.Errorf("k: itemIdx = %d", m.itemIdx)
	}
}

func TestAyudaOverlayYFooterContextual(t *testing.T) {
	app := NewApp(testLoader(t))
	m, _ := app.Update(keyMsg("?"))
	app = m.(App)
	if !app.showHelp {
		t.Fatal("? no abrió la ayuda")
	}
	if v := app.View(); !strings.Contains(v, "AYUDA") {
		t.Error("overlay sin título")
	}
	// Cualquier tecla cierra.
	m, _ = app.Update(keyMsg("x"))
	if m.(App).showHelp {
		t.Error("la ayuda no se cerró con una tecla cualquiera")
	}
}

// TestSinDesbordeEstrecho: la regresión del corte por la derecha — a
// 50 columnas, ninguna línea visible del detalle supera el ancho.
func TestSinDesbordeEstrecho(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width = 50
	if got := d.cardsPerRow(); got != 1 {
		t.Fatalf("a 50 columnas, tarjetas por fila = %d", got)
	}
	re := regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	for _, line := range strings.Split(d.View(), "\n") {
		visible := len([]rune(re.ReplaceAllString(line, "")))
		if visible > 50 {
			t.Fatalf("línea de %d columnas a ancho 50: %q", visible, line)
		}
	}
}

func TestAppTransicionesYSelectorDeJuego(t *testing.T) {
	app := NewApp(testLoader(t))
	if app.screen != screenPick {
		t.Fatal("debe arrancar en el selector de juego")
	}
	// Elige Deadlock con → y enter.
	m, _ := app.Update(keyMsg("right"))
	app = m.(App)
	if app.pickIdx != 1 {
		t.Fatalf("pickIdx = %d, se esperaba 1 (Deadlock)", app.pickIdx)
	}
	m, _ = app.Update(keyMsg("enter"))
	app = m.(App)
	if app.screen != screenLoading {
		t.Fatalf("tras enter, pantalla = %v", app.screen)
	}
	// La carga (síncrona en test) entrega la sesión.
	s := loadOffline(t, model.GameDeadlock)
	m, _ = app.Update(sessionLoadedMsg{game: model.GameDeadlock, session: s})
	app = m.(App)
	if app.screen != screenList {
		t.Fatalf("tras cargar, pantalla = %v", app.screen)
	}
	if !strings.Contains(app.View(), "Deadlock") {
		t.Error("la lista no muestra el juego activo")
	}
	// Enter → detalle de héroe; esc → lista; esc → selector (g ya no
	// es atajo: cualquier letra escribe en el filtro).
	m, _ = app.Update(keyMsg("enter"))
	app = m.(App)
	if app.screen != screenDetail || app.detail.champ == nil {
		t.Fatalf("tras enter, pantalla = %v", app.screen)
	}
	m, _ = app.Update(keyMsg("esc"))
	app = m.(App)
	if app.screen != screenList {
		t.Fatal("esc debía volver a la lista")
	}
	m, _ = app.Update(keyMsg("esc"))
	app = m.(App)
	if app.screen != screenPick {
		t.Fatal("el segundo esc debía volver al selector de juego")
	}
	// Reentrar al mismo juego es instantáneo: la sesión está en
	// memoria y no pasa por la pantalla de carga.
	m, _ = app.Update(keyMsg("enter"))
	app = m.(App)
	if app.screen != screenList {
		t.Fatalf("reentrada debía ir directa a la lista, fue a %v", app.screen)
	}
	if !strings.Contains(app.View(), "Deadlock") {
		t.Error("la sesión cacheada no se restauró")
	}
}
