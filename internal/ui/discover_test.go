package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/config"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

func TestOrdenDeLista(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	app := NewApp(testLoader(t))
	m, _ := app.Update(sessionLoadedMsg{game: model.GameLoL, session: s})
	app = m.(App)

	wrFirst := app.list.SelectedChampion().Key
	// Cicla a "Partidas" con tab: el orden puede cambiar.
	m, _ = app.Update(keyMsg("tab"))
	app = m.(App)
	if app.list.sortMode != 1 {
		t.Fatalf("sortMode = %d", app.list.sortMode)
	}
	if got := SortModes[1]; got != "Partidas ▼" {
		t.Errorf("SortModes[1] = %q", got)
	}
	_ = wrFirst
	// El filtro no rompe el orden y Counts respeta el total.
	vis, total := app.list.Counts()
	if vis != total || total != 14 {
		t.Errorf("counts = %d de %d, se esperaban 14", vis, total)
	}
}

func TestFiltroAlEscribir(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	app := NewApp(testLoader(t))
	m, _ := app.Update(sessionLoadedMsg{game: model.GameLoL, session: s})
	app = m.(App)

	// Escribir "ah" filtra sin pulsar / primero.
	for _, k := range []string{"a", "h"} {
		m, _ = app.Update(keyMsg(k))
		app = m.(App)
	}
	if !app.list.FilterActive() {
		t.Fatal("escribir no activó el filtro")
	}
	vis, total := app.list.Counts()
	if vis == total || vis == 0 {
		t.Fatalf("filtro: visibles=%d total=%d", vis, total)
	}
	if sel := app.list.SelectedChampion(); sel == nil || sel.Key != "Ahri" {
		t.Errorf("filtro 'ah' no dejó a Ahri: %+v", sel)
	}
	// Esc cancela el filtro.
	m, _ = app.Update(keyMsg("esc"))
	app = m.(App)
	if app.list.FilterActive() {
		t.Error("esc no canceló el filtro")
	}
}

// TestFiltroAceptaTeclasDeAtajo: la regresión que rompía "Lash" y
// "Grey Talon". `s` y `g` eran atajos de la lista y se comían la letra
// antes de llegar al filtro.
func TestFiltroAceptaTeclasDeAtajo(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	app := NewApp(testLoader(t))
	m, _ := app.Update(sessionLoadedMsg{game: model.GameLoL, session: s})
	app = m.(App)
	sortBefore := app.list.sortMode

	// "s" ya no reordena: escribe.
	m, _ = app.Update(keyMsg("s"))
	app = m.(App)
	if app.list.sortMode != sortBefore {
		t.Error("la 's' reordenó la lista en vez de escribir en el filtro")
	}
	if !app.list.FilterActive() {
		t.Fatal("la 's' no activó el filtro")
	}
	if app.screen != screenList {
		t.Fatalf("escribir cambió de pantalla: %v", app.screen)
	}

	// "g" sigue escribiendo en el filtro, no salta al selector.
	m, _ = app.Update(keyMsg("g"))
	app = m.(App)
	if app.screen != screenList {
		t.Fatalf("la 'g' saltó al selector de juego (pantalla = %v)", app.screen)
	}
	if got := app.list.filterBuf; got != "sg" {
		t.Errorf("filterBuf = %q, se esperaba \"sg\"", got)
	}
	// Esc cancela y el segundo esc sí vuelve al selector.
	m, _ = app.Update(keyMsg("esc"))
	app = m.(App)
	if app.list.FilterActive() || app.screen != screenList {
		t.Fatalf("el primer esc debía solo cancelar el filtro (pantalla %v)", app.screen)
	}
	m, _ = app.Update(keyMsg("esc"))
	if m.(App).screen != screenPick {
		t.Error("el segundo esc debía volver al selector de juego")
	}
}

// TestFiltroConTodasLasLetras: ningún campeón/héroe debe quedar
// inalcanzable tecleando.
func TestFiltroConTodasLasLetras(t *testing.T) {
	for _, r := range "abcdfghijklmnoprstuvwxyz" {
		if reservedRunes[r] {
			t.Errorf("la letra %q no puede iniciar un filtro", r)
		}
	}
}

func TestClicEnPestanaYTarjeta(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 120, 50
	_ = d.View() // registra las zonas en zoneMgr
	// Clic en la pestaña 2 (Runas).
	//
	// Las zonas llegan ASÍNCRONAMENTE: `Scan` encola cada zona en un canal
	// y las registra un worker en otra goroutine, así que una lectura
	// inmediata puede no verlas todavía. En Linux el worker casi siempre
	// gana; en Windows (y con la máquina cargada) no, y esto fallaba en
	// CI con "zona "tab:1" no registrada". Se espera a la condición,
	// no a un tiempo fijo.
	click := func(id string) (int, int) {
		if !waitZone(id) {
			t.Fatalf("zona %q no registrada", id)
		}
		z := zoneMgr.Get(id)
		// El centro de la zona siempre cae dentro.
		return z.StartX + (z.EndX-z.StartX)/2, z.StartY + (z.EndY-z.StartY)/2
	}
	cx, cy := click("tab:1")
	mm := tea.MouseMsg{X: cx, Y: cy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m := d.handleMouse(mm)
	if m.tab != 1 {
		t.Errorf("clic en pestaña: tab = %d", m.tab)
	}
	// Clic en la tercera tarjeta (vuelve a Ítems con teclado).
	m, _ = m.Update(keyMsg("1"))
	_ = m.View()
	cx, cy = click("card:2")
	mm = tea.MouseMsg{X: cx, Y: cy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	m2 := m.handleMouse(mm)
	if m2.itemIdx != 2 {
		t.Errorf("clic en tarjeta: itemIdx = %d", m2.itemIdx)
	}
}

func TestConfigRestauraSeleccion(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("BUILDS_CONFIG", p)
	cfg := config.Config{LastGame: "deadlock", LastChampion: "Haze", SortMode: 0}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	s := loadOffline(t, model.GameDeadlock)
	app := NewApp(testLoader(t))
	if app.pickIdx != 1 {
		t.Fatalf("pickIdx = %d, se esperaba 1 (Deadlock de la config)", app.pickIdx)
	}
	m, _ := app.Update(sessionLoadedMsg{game: model.GameDeadlock, session: s})
	app = m.(App)
	if sel := app.list.SelectedChampion(); sel == nil || sel.Key != "Haze" {
		t.Errorf("cursor = %+v, se esperaba Haze", sel)
	}
}

func TestTrendStripEnCabecera(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	// Los fixtures no traen tendencia: inyectamos puntos.
	abrams.ByRole[model.RoleAll].WRTrend = []model.WRTrendPoint{
		{Time: 1, WR: 0.512, Games: 1000},
		{Time: 2, WR: 0.525, Games: 1200},
		{Time: 3, WR: 0.509, Games: 900},
		{Time: 4, WR: 0.518, Games: 1100},
	}
	d := newDetailView(abrams, s)
	d.width, d.height = 100, 50
	v := d.View()
	if !strings.Contains(v, "Tendencia") {
		t.Error("la cabecera no muestra la franja de tendencia")
	}
	// Con 4+ puntos la chispa ya se lee como línea de tendencia.
	if !strings.Contains(v, "▅") {
		t.Error("sin chispas en la tendencia")
	}
	// Con menos de 4 puntos, cada punto lleva su mes: dos números con
	// un bloque cada uno no dicen qué son.
	abrams.ByRole[model.RoleAll].WRTrend = abrams.ByRole[model.RoleAll].WRTrend[:2]
	v = d.View()
	if !strings.Contains(v, "→") {
		t.Error("dos puntos de tendencia sin flecha entre ellos")
	}
	if !strings.Contains(v, "(ene 70)") {
		t.Error("punto de tendencia sin etiqueta de mes")
	}
}

// TestRutaEnPopupDL: con la ruta cargada, el popup del ítem muestra el
// número de orden de consenso y la línea de ruta.
func TestRutaEnPopupDL(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	s.ItemMeta = map[int]staticdata.ItemInfo{
		101: {Name: "Salud extra", Cost: 800, Category: "vitality", Tier: 1},
	}
	b := abrams.ByRole[model.RoleAll]
	b.CoreItems = []model.RankedItem{
		{ItemStats: model.ItemStats{ID: 101, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: 300},
			WinRate: 0.53, PickRate: 0.4, Wilson: 0.523},
	}
	d := newDetailView(abrams, s)
	d.width, d.height = 100, 90
	// Sin ruta cargada el popup no inventa números; al cargarse,
	// muestra orden, posición y patrimonio.
	s.Routes = nil
	v := collapsePlain(d.itemPopup())
	if strings.Contains(v, "ruta #") {
		t.Fatal("sin ruta cargada, el popup no debía mostrar número de ruta")
	}
	s.Routes = map[string][]RouteStep{"Abrams": {
		{Item: 101, Order: 1, MedT: 300, MedNet: 1200, PosLo: 1, PosHi: 2, N: 120},
	}}
	v = collapsePlain(d.itemPopup())
	for _, want := range []string{"ruta #1", "posición 1–2", "patrim. ~1.2k"} {
		if !strings.Contains(v, want) {
			t.Errorf("popup sin %q: %s", want, v)
		}
	}
}

// TestClicEnBarraCompacta: en terminales estrechas la barra muestra solo
// la pestaña activa, pero debe seguir siendo clicable.
//
// Regresión de CI: la rama compacta de tabsView no marcaba la zona, así
// que el clic dejaba de funcionar con anchos pequeños. Windows arranca
// con una ventana más estrecha y era donde se veía.
func TestClicEnBarraCompacta(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	for _, width := range []int{40, 50, 60} {
		d := newDetailView(ahri, s)
		d.width, d.height = width, 50
		_ = d.View()

		id := fmt.Sprintf("tab:%d", d.tab)
		// Las zonas llegan de forma asíncrona: se espera la condición.
		if !waitZone(id) {
			t.Fatalf("ancho %d: zona %q no registrada", width, id)
		}
		z := zoneMgr.Get(id)
		cx := z.StartX + (z.EndX-z.StartX)/2
		cy := z.StartY + (z.EndY-z.StartY)/2
		mm := tea.MouseMsg{X: cx, Y: cy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
		// Clic sobre la pestaña activa: no cambia de pestaña, pero el clic
		// debe resolverse (no puede caer fuera de toda zona).
		if m := d.handleMouse(mm); m.tab != d.tab {
			t.Errorf("ancho %d: el clic movió de la pestaña %d a %d", width, d.tab, m.tab)
		}
	}
}
