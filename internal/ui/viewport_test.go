package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/builds-tui/internal/model"
	"github.com/Eion21/builds-tui/internal/staticdata"
)

// ansiRE quita las secuencias de escape para medir el texto visible.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// visibleLines devuelve las líneas de la vista sin ANSI.
func visibleLines(v string) []string {
	raw := strings.Split(v, "\n")
	out := make([]string, len(raw))
	for i, l := range raw {
		out[i] = ansiRE.ReplaceAllString(l, "")
	}
	return out
}

// TestViewportNoCortaLaCabecera: el bug original. Con una terminal
// baja, la cabecera con el nombre y los datos del campeón debe seguir
// visible; antes Bubble Tea recortaba por arriba y se perdía.
func TestViewportNoCortaLaCabecera(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 20 // terminal baja: el contenido no cabe

	v := d.View()
	if !strings.Contains(v, "Ahri") {
		t.Fatal("la cabecera desapareció en una terminal baja")
	}
	if !strings.Contains(v, "WR") {
		t.Error("la cabecera perdió las métricas")
	}
	// La vista no debe pasarse del alto de la terminal.
	if got := len(strings.Split(v, "\n")); got > d.height {
		t.Errorf("la vista ocupa %d líneas para una terminal de %d", got, d.height)
	}
}

// TestViewportScrollSigueLaSeleccion: navegar con ↓ hasta una tarjeta
// por debajo del pliegue debe desplazar el contenido. Antes el estado
// cambiaba pero no se veía nada.
func TestViewportScrollSigueLaSeleccion(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 22
	d.View() // fija el layout
	if d.vp.Height <= 0 {
		t.Fatalf("viewport sin alto: %d", d.vp.Height)
	}
	top := d.vp.YOffset

	// Baja hasta el final con el cursor.
	for i := 0; i < len(d.cardList())+2; i++ {
		d, _ = d.Update(keyMsg("down"))
		d.View()
	}
	if len(d.cardList()) <= d.cardsPerRow() {
		t.Skip("hacen falta varias filas de tarjetas para probar el scroll")
	}
	if d.vp.YOffset <= top {
		t.Errorf("el viewport no bajó con la selección (YOffset sigue en %d)", d.vp.YOffset)
	}
	// Y la tarjeta seleccionada tiene que estar dentro del viewport.
	_, starts := d.itemsSections(d.cardList())
	line0 := starts[d.itemIdx] + 2 // borde + padding del panel
	if line0 < d.vp.YOffset || line0 >= d.vp.YOffset+d.vp.Height {
		t.Errorf("la tarjeta %d (línea %d) quedó fuera de [%d,%d)",
			d.itemIdx, line0, d.vp.YOffset, d.vp.YOffset+d.vp.Height)
	}

	// Subir hasta arriba devuelve el scroll al principio.
	for i := 0; i < len(d.cardList())+2; i++ {
		d, _ = d.Update(keyMsg("up"))
		d.View()
	}
	if d.vp.YOffset != 0 {
		t.Errorf("volver arriba debía dejar YOffset=0, quedó en %d", d.vp.YOffset)
	}
}

// TestViewportRuedaDelRaton: la rueda debe mover el contenido. Antes
// App solo reenviaba clics izquierdos y el viewport no la veía nunca.
func TestViewportRuedaDelRaton(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	app := NewApp(testLoader(t))
	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	app = m.(App)
	m, _ = app.Update(sessionLoadedMsg{game: model.GameLoL, session: s})
	app = m.(App)
	app.list.SelectChampion("Ahri")
	m, _ = app.Update(keyMsg("enter"))
	app = m.(App)
	if app.screen != screenDetail {
		t.Fatalf("pantalla = %v, se esperaba el detalle", app.screen)
	}
	app.View() // fija el layout del viewport
	if app.detail.vp.Height <= 0 {
		t.Fatalf("viewport sin alto: %d", app.detail.vp.Height)
	}

	m, _ = app.Update(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	})
	app = m.(App)
	if app.detail.vp.YOffset == 0 {
		t.Error("la rueda no desplazó el contenido del detalle")
	}
}

// TestPopupModal: con el popup abierto, tab/r/a no deben cambiar el
// estado de debajo (el popup quedaría mintiendo).
func TestPopupModal(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 40

	d, _ = d.Update(keyMsg("enter"))
	if !d.popupOpen() {
		t.Fatal("enter no abrió el popup")
	}
	role, tab, art := d.roleIdx, d.tab, d.artMode
	for _, k := range []string{"tab", "r", "a", "2", "down"} {
		d, _ = d.Update(keyMsg(k))
	}
	if d.roleIdx != role || d.tab != tab || d.artMode != art {
		t.Errorf("el popup dejó pasar teclas: rol %d→%d, pestaña %d→%d, retrato %d→%d",
			role, d.roleIdx, tab, d.tab, art, d.artMode)
	}
	if !d.popupOpen() {
		t.Error("el popup se cerró con una tecla que no era esc/enter")
	}

	// ←→ sí cambian de ítem dentro del popup.
	before := d.itemIdx
	d, _ = d.Update(keyMsg("right"))
	if d.itemIdx == before && len(d.cardList()) > 1 {
		t.Error("→ no pasó al siguiente ítem dentro del popup")
	}
	// Esc cierra.
	d, _ = d.Update(keyMsg("esc"))
	if d.popupOpen() {
		t.Error("esc no cerró el popup")
	}

	// Cerrado, las teclas vuelven a funcionar.
	d, _ = d.Update(keyMsg("2"))
	if d.tab != 1 {
		t.Error("tras cerrar el popup, '2' debía cambiar de pestaña")
	}
}

// TestPopupCabeEnTerminalBaja: el popup no pasa por el viewport, así
// que su pie ("esc cierra") se ve aunque la terminal sea baja.
func TestPopupCabeEnTerminalBaja(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 30
	d, _ = d.Update(keyMsg("enter"))
	v := d.View()
	if !strings.Contains(v, "esc cierra") {
		t.Error("el popup perdió su pie en una terminal baja")
	}
	for _, want := range []string{"Wilson", "muestra", "pick"} {
		if !strings.Contains(v, want) {
			t.Errorf("popup sin %q", want)
		}
	}
}

// TestPieDeItemsFueraDelScroll: la línea con el nombre completo del
// ítem y la leyenda de WR deben verse sin hacer scroll.
func TestPieDeItemsFueraDelScroll(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 24
	v := d.View()
	for _, want := range []string{"Companyera de Luden", "WR"} {
		if !strings.Contains(v, want) {
			t.Errorf("la vista sin scroll no muestra %q", want)
		}
	}
}

// collapsePlain quita ANSI, bordes de panel y saltos de línea para
// poder buscar frases que el render parte entre líneas.
func collapsePlain(v string) string {
	v = strings.ReplaceAll(v, "│", "")
	return regexp.MustCompile(`\s+`).ReplaceAllString(v, " ")
}

// TestFooterComboYOrden: cuando la build trae el WR de tríos reales
// (builtBootSet3), el pie muestra el combo; el orden de compra medido
// (item1..item5) vive en su bloque propio dentro del panel.
func TestFooterComboYOrden(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.ItemCombos = []model.ItemCombo{
		{IDs: []int{6653, 3020, 3116}, Stats: model.Stats{Games: 2444, Wins: 1386}, PickRate: 17.28},
	}
	b.ItemSlots = [][]model.ItemStats{
		{{ID: 6653, Stats: model.Stats{Games: 6772, Wins: 3595}, AvgBuyS: 12 * 60}},
		{{ID: 3116, Stats: model.Stats{Games: 5765, Wins: 3099}, AvgBuyS: 19 * 60}},
	}
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 30
	v := ansiRE.ReplaceAllString(d.View(), "")
	if want := s.ItemName(6653); !strings.Contains(v, "combo: "+want+" + ") {
		t.Errorf("el pie no muestra el combo con el nombre real de %d (%q)", 6653, want)
	}
	if !strings.Contains(v, "WR 56.7%") {
		t.Errorf("el pie no muestra el WR del combo")
	}
	if !strings.Contains(v, "2.4k partidas") {
		t.Errorf("el pie no muestra la muestra del combo")
	}
	// El orden es un bloque del panel: 1.º X 12' → 2.º Y 19'.
	want := "ORDEN DE COMPRA 1.º " + s.ItemName(6653) + " 12' → 2.º " + s.ItemName(3116) + " 19'"
	if !strings.Contains(collapsePlain(v), want) {
		t.Errorf("el panel no muestra el bloque de orden de compra: %q", want)
	}
}

// TestOrdenSinDuplicados: el mismo ítem top de dos turnos no debe
// repetirse en el orden de compra; el turno cubierto muestra su
// siguiente opción y el turno agotado se salta.
func TestOrdenSinDuplicados(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.ItemSlots = [][]model.ItemStats{
		{{ID: 6653, Stats: model.Stats{Games: 6000, Wins: 3100}, AvgBuyS: 11 * 60}},
		{{ID: 6653, Stats: model.Stats{Games: 5000, Wins: 2600}, AvgBuyS: 19 * 60},
			{ID: 3157, Stats: model.Stats{Games: 4000, Wins: 2080}, AvgBuyS: 20 * 60}},
		{{ID: 3157, Stats: model.Stats{Games: 3000, Wins: 1530}, AvgBuyS: 26 * 60}},
	}
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 30
	v := ansiRE.ReplaceAllString(d.View(), "")
	want := "1.º " + s.ItemName(6653) + " 11' → 2.º " + s.ItemName(3157) + " 20'"
	if !strings.Contains(collapsePlain(v), want) {
		t.Errorf("el bloque de orden no muestra %q", want)
	}
	if strings.Contains(collapsePlain(v), "3.º") {
		t.Errorf("el turno con todo cubierto debía omitirse")
	}
}

// TestBotasPlegadas: una sola bota visible como tarjeta y el resto
// plegado en una línea informativa (casi nadie compra la cuarta
// opción; confunde más de lo que ayuda).
func TestBotasPlegadas(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.Boots = []model.RankedItem{
		{ItemStats: model.ItemStats{ID: 3020, Stats: model.Stats{Games: 20000, Wins: 10400}}, WinRate: 0.52, PickRate: 0.40},
		{ItemStats: model.ItemStats{ID: 3006, Stats: model.Stats{Games: 5000, Wins: 2550}}, WinRate: 0.51, PickRate: 0.10},
		{ItemStats: model.ItemStats{ID: 3158, Stats: model.Stats{Games: 3000, Wins: 1500}}, WinRate: 0.50, PickRate: 0.06},
	}
	d := newDetailView(ahri, s)
	// Alto suficiente para que la sección BOTAS y su plegado queden
	// dentro del viewport (las tarjetas con icono miden ~10 líneas).
	d.width, d.height = 100, 70
	v := ansiRE.ReplaceAllString(d.View(), "")
	if !strings.Contains(v, "otras botas (2): ") {
		t.Errorf("el plegado de botas no aparece en la vista")
	}
	for _, want := range []string{s.ItemName(3006), s.ItemName(3158)} {
		if !strings.Contains(v, want) {
			t.Errorf("el plegado no nombra a %q", want)
		}
	}
}

// TestIndicadorScroll: cuando el contenido no cabe, el detalle debe
// avisar que hay más arriba o abajo; si todo cabe, no hay indicador.
// TestResetScrollAlCambiarPestana: bajar al fondo en Ítems y cambiar a
// otra pestaña (tecla, tab o clic) arranca desde arriba: el contenido
// nuevo no hereda el offset del anterior. Lo mismo al cambiar de rol.
func TestResetScrollAlCambiarPestana(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 30 // contenido que desborda
	// Bajar al fondo del contenido de Ítems.
	for i := 0; i < len(d.cardList())+4; i++ {
		d, _ = d.Update(keyMsg("down"))
		d.View()
	}
	if d.vp.YOffset == 0 {
		t.Fatal("el test necesita haber scrolleado: YOffset = 0")
	}
	// Cambiar a Habilidades por tecla: la vista arranca arriba.
	m, _ := d.Update(keyMsg("4"))
	if m.vp.YOffset != 0 {
		t.Errorf("tras cambiar a pestaña por tecla, YOffset = %d, se esperaba 0", m.vp.YOffset)
	}
	// Cambiar de rol: también es contenido nuevo.
	m, _ = m.Update(keyMsg("["))
	if m.vp.YOffset != 0 {
		t.Errorf("tras cambiar de rol, YOffset = %d, se esperaba 0", m.vp.YOffset)
	}
	// Cambiar a Resumen por tab: mismo criterio.
	m, _ = m.Update(keyMsg("tab"))
	if m.vp.YOffset != 0 {
		t.Errorf("tras tab, YOffset = %d, se esperaba 0", m.vp.YOffset)
	}
}

// TestSeccionWardLoL: el mejor ward va en su propia sección WARD,
// después de BOTAS; sin ward en la build la sección no existe.
func TestSeccionWardLoL(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 90 // todo el contenido visible
	if v := d.View(); strings.Contains(v, "◆ WARD") {
		t.Fatal("sin ward en la build no debía haber sección WARD")
	}
	// Darle un ward a la build activa (el rankeo ya está probado en
	// clean): 2055 con muestra y WR decentes.
	b := d.CurrentBuild()
	b.Wards = []model.RankedItem{{
		ItemStats: model.ItemStats{ID: 2055, Stats: model.Stats{Games: 20000, Wins: 11000}},
		WinRate:   0.55, PickRate: 0.2, Wilson: 0.543,
	}}
	v := d.View()
	for _, want := range []string{"◆ WARD", "Guardián"} {
		if !strings.Contains(v, want) {
			t.Errorf("pestaña ítems sin %q", want)
		}
	}
	// El ward es navegable: la última tarjeta es el ward.
	if n := len(d.cardList()); d.cardList()[n-1].item.ID != 2055 {
		t.Errorf("la última tarjeta = %d, se esperaba el ward 2055", d.cardList()[n-1].item.ID)
	}
}

// TestRutaDeCompraDL: con ruta de consenso cargada, cada ítem es su
// propia tarjeta — las mejoras son un tipo distinto (borde doble, ↑,
// línea `de <base>`, coste neto), la fase se decide por el minuto
// propio y el número de orden cuenta compras y mejoras juntas.
func TestRutaDeCompraDL(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	d := newDetailView(abrams, s)
	d.width, d.height = 100, 90
	if s.ItemMeta == nil {
		s.ItemMeta = map[int]staticdata.ItemInfo{}
	}
	// Ruta: 101 (Salud extra) es compra directa; 201 (Recarga activa)
	// es mejora de 101 en el 81% de sus compras.
	s.Routes = map[string][]RouteStep{"Abrams": {
		{Item: 101, Order: 1, MedT: 300, N: 120},
		{Item: 201, Order: 2, MedT: 693, UpgradeOf: 101, ChainRate: 0.81, N: 97},
	}}
	s.ItemMeta[101] = staticdata.ItemInfo{Name: "Salud extra", Cost: 800, Category: "vitality", Tier: 1}
	s.ItemMeta[201] = staticdata.ItemInfo{Name: "Recarga activa", Cost: 1600, Category: "vitality", Tier: 2}
	b := d.CurrentBuild()
	b.CoreItems = []model.RankedItem{
		{ItemStats: model.ItemStats{ID: 101, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: 300},
			WinRate: 0.53, PickRate: 0.4, Wilson: 0.523},
		{ItemStats: model.ItemStats{ID: 201, Stats: model.Stats{Games: 25000, Wins: 10600}, AvgBuyS: 900},
			WinRate: 0.54, PickRate: 0.3, Wilson: 0.533},
	}
	cards := d.cardList()
	if len(cards) != 2 {
		t.Fatalf("tarjetas = %d, se esperaban 2 (cada ítem la suya)", len(cards))
	}
	// Orden de consenso: compras y mejoras juntas, por minuto propio.
	if cards[0].item.ID != 101 || cards[0].order != 1 || cards[0].kind != kindBuy {
		t.Errorf("tarjeta 0 = %d (orden %d, kind %v), se esperaba compra 101 orden 1",
			cards[0].item.ID, cards[0].order, cards[0].kind)
	}
	if cards[1].item.ID != 201 || cards[1].order != 2 || cards[1].kind != kindUpgrade || cards[1].baseID != 101 {
		t.Errorf("tarjeta 1 = %d (orden %d, kind %v, base %d), se esperaba mejora 201 orden 2 de 101",
			cards[1].item.ID, cards[1].order, cards[1].kind, cards[1].baseID)
	}
	// El minuto de la tarjeta es el de la ruta, no el de item-stats.
	if cards[1].dlMinuteOf() != 693 {
		t.Errorf("minuto de la mejora = %d, se esperaba el mediano de la ruta (693)", cards[1].dlMinuteOf())
	}
	// La vista muestra la ruta, la línea `de <base>`, el coste neto
	// y el minuto propio; y ya no el coste encadenado viejo.
	v := collapsePlain(d.View())
	for _, want := range []string{"RUTA", "de Salud extra", "+800 almas", "~11 min"} {
		if !strings.Contains(v, want) {
			t.Errorf("vista Deadlock sin %q", want)
		}
	}
	if strings.Contains(v, "800 → +800 souls") {
		t.Error("la vista aún muestra el coste encadenado viejo (800 → +800)")
	}
	// Relacionados: con la base seleccionada, la mejora queda marcada y
	// la línea de selección explica la cadena.
	d.itemIdx = 1
	if sel := d.selectionLine(); !strings.Contains(collapsePlain(sel), "Mejora Salud extra") {
		t.Errorf("línea de selección sin la cadena: %s", collapsePlain(sel))
	}
	// The base footer remains concise; chain statistics live in the popup.
	d.itemIdx = 0
	if sel := d.selectionLine(); !strings.Contains(collapsePlain(sel), "Salud extra") {
		t.Errorf("línea de selección sin la mejora de la base: %s", collapsePlain(sel))
	}
	// Toggle `u`: las mejoras se pliegan y quedan solo compras.
	d2, _ := d.Update(keyMsg("u"))
	if cards2 := d2.cardList(); len(cards2) != 1 || cards2[0].item.ID != 101 {
		t.Fatalf("con `u` quedan %d tarjetas, se esperaba solo la compra 101", len(cards2))
	}
}

func TestIndicadorScroll(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")

	d := newDetailView(ahri, s)
	d.width, d.height = 100, 30 // el contenido seguro no cabe
	v := d.View()
	if !strings.Contains(v, "▼") {
		t.Error("terminal baja sin indicar que hay más contenido abajo")
	}

	d = newDetailView(ahri, s)
	d.width, d.height = 100, 90 // acá sobra pantalla
	if v = d.View(); strings.Contains(v, "▼") || strings.Contains(v, "▲") {
		t.Error("con todo el contenido visible no debe haber indicador de scroll")
	}
}

// TestTituloFijoAlScroll: al bajar, la sección activa debe quedar
// indicada arriba aunque su título haya salido de pantalla.
func TestTituloFijoAlScroll(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 34
	// Baja hasta que el título de SITUACIONALES haya salido del pliegue.
	for i := 0; i < len(d.cardList())+2; i++ {
		d, _ = d.Update(keyMsg("down"))
		d.View()
	}
	if d.vp.YOffset <= 2 {
		t.Skipf("el fixture no alcanza a hacer scroll (YOffset=%d)", d.vp.YOffset)
	}
	v := d.View()
	if !strings.Contains(v, "SITUACIONALES") {
		t.Error("scrolleado, la vista perdió el título de la sección activa")
	}
}

// TestTarjetasCompactas: en terminal baja las tarjetas pierden el
// icono (una tarjeta compacta mide pocas líneas) para que las tres
// secciones quepan sin scroll.
func TestTarjetasCompactas(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")

	d := newDetailView(ahri, s)
	d.width, d.height = 100, 24 // baja: modo compacto
	// sync es quien fija el layout real; View opera sobre una copia.
	(&d).sync()
	card := d.itemCard(0, d.cardList()[0])
	if h := lipgloss.Height(card); h > 5 {
		t.Errorf("en terminal baja la tarjeta mide %d líneas, se esperaban ≤5", h)
	}

	d = newDetailView(ahri, s)
	d.width, d.height = 100, 80 // alta: tarjeta completa con icono
	(&d).sync()
	card = d.itemCard(0, d.cardList()[0])
	if h := lipgloss.Height(card); h <= 5 {
		t.Errorf("en terminal alta la tarjeta mide %d líneas: perdió el icono", h)
	}
}

// TestDeltaWRLoL: las tarjetas de LoL muestran la diferencia contra el
// WR del campeón (como Deadlock), no un verde absoluto que premia el
// sesgo de bola de nieve; el WR absoluto queda en la línea de detalle.
func TestDeltaWRLoL(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.WinRate = 0.50
	b.CoreItems[0].WinRate = 0.536
	d := newDetailView(ahri, s)
	(&d).sync()
	card := ansiRE.ReplaceAllString(d.itemCard(0, d.cardList()[0]), "")
	if !strings.Contains(card, "+3.6") {
		t.Errorf("la tarjeta no muestra el delta contra el campeón: %q", card)
	}
	if strings.Contains(card, "53.6%") {
		t.Errorf("la tarjeta sigue mostrando el WR absoluto: %q", card)
	}
}

// TestAvisoSnowball: un ítem con WR muy por encima del campeón (Mejai
// al 79.9% en un héroe al 52.7%) se marca como sesgo de bola de
// nieve, en la tarjeta y en la línea de detalle.
func TestAvisoSnowball(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.WinRate = 0.527
	b.CoreItems[0].WinRate = 0.799
	d := newDetailView(ahri, s)
	(&d).sync()
	if card := ansiRE.ReplaceAllString(d.itemCard(0, d.cardList()[0]), ""); !strings.Contains(card, "⚠") {
		t.Errorf("la tarjeta no marca el sesgo: %q", card)
	}
	v := ansiRE.ReplaceAllString(d.View(), "")
	// El pie envuelve por palabras: colapsar espacios para que la
	// frase partida en dos líneas siga encontrándose.
	collapsed := regexp.MustCompile(`\s+`).ReplaceAllString(v, " ")
	if !strings.Contains(collapsed, "se compra yendo por delante") {
		t.Error("la línea de detalle no avisa el sesgo de bola de nieve")
	}
}

// TestHechizosEnItems: el bloque HECHIZOS muestra el par de hechizos
// más usado con nombres reales de Data Dragon.
func TestHechizosEnItems(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	b := ahri.Build(0)
	b.Spells = []model.SummonerStats{
		{IDs: [2]int{4, 14}, Stats: model.Stats{Games: 26605, Wins: 14207}},
		{IDs: [2]int{4, 11}, Stats: model.Stats{Games: 1200, Wins: 600}},
	}
	d := newDetailView(ahri, s)
	d.width, d.height = 100, 40
	v := ansiRE.ReplaceAllString(d.View(), "")
	want := "HECHIZOS " + s.SpellPairName([2]int{4, 14}) + " · WR 53.4% · " + nThousand(26605) + " partidas"
	if !strings.Contains(collapsePlain(v), want) {
		t.Errorf("sin bloque de hechizos con nombres reales: %q", want)
	}
}

// TestGuardsSinRoles: un campeón sin roles canónicos no debe provocar
// pánico en la lista ni en el detalle.
func TestGuardsSinRoles(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	var raro *ChampEntry
	for _, e := range s.Champions {
		if len(e.Roles) > 0 && e.Key != "Ahri" {
			raro = e
			break
		}
	}
	if raro == nil {
		t.Skip("sin campeones suficientes en los fixtures")
	}
	roto := &ChampEntry{Key: raro.Key, Name: raro.Name, ByRole: raro.ByRole}
	// Sin roles: MainRoleIdx 0, Build nil, nada de pánicos.
	if got := roto.MainRoleIdx(); got != 0 {
		t.Errorf("MainRoleIdx sin roles = %d", got)
	}
	if b := roto.Build(0); b != nil {
		t.Error("Build sin roles debía ser nil")
	}
	_ = mainWR(roto)
	_ = mainGames(roto)
	_ = champItem{champ: roto, session: s}.Description()

	d := newDetailView(roto, s)
	d.width, d.height = 100, 40
	for _, k := range []string{"r", "tab", "down", "right", "enter", "esc", "a"} {
		d, _ = d.Update(keyMsg(k))
	}
	if v := d.View(); v == "" {
		t.Error("la vista de un campeón sin roles quedó vacía")
	}
}

// TestAnchoDelViewport: el viewport debe medir el ancho real de la
// terminal. Con el ancho fijo de 80, a 60 columnas el borde del panel
// se cortaba y a 140 sobraba espacio muerto.
func TestAnchoDelViewport(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	ahri := findChampion(t, s, "Ahri")
	for _, w := range []int{60, 100, 140} {
		d := newDetailView(ahri, s)
		d.width, d.height = w, 40
		lines := visibleLines(d.View())
		if len(lines) == 0 {
			t.Fatal("vista vacía")
		}
		for _, line := range lines {
			if n := len([]rune(line)); n > w {
				t.Fatalf("a %d columnas, línea de %d: %q", w, n, line)
			}
		}
		// El panel debe llenar el ancho disponible, no quedarse corto.
		if got := len([]rune(strings.TrimRight(lines[0], " "))); got < w-8 {
			t.Errorf("a %d columnas el panel solo ocupa %d", w, got)
		}
	}
}

// Route position, not average time or card kind, controls reading order.
func TestDeadlockInterleavedRoute(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	d.width, d.height = 100, 90
	s.ItemMeta = map[int]staticdata.ItemInfo{
		101: {Name: "Base", Cost: 800, Category: "vitality", Tier: 1},
		201: {Name: "Mejora", Cost: 1600, Category: "vitality", Tier: 2},
		301: {Name: "Otra compra", Cost: 800, Category: "spirit", Tier: 1},
		401: {Name: "Alternativa", Cost: 800, Category: "weapon", Tier: 1},
	}
	s.Routes = map[string][]RouteStep{"Abrams": {
		{Item: 101, MedT: 300},
		{Item: 201, MedT: 850, UpgradeOf: 101, ChainRate: .81},
		{Item: 301, MedT: 700},
	}}
	b := d.CurrentBuild()
	b.CoreItems = nil
	for i, id := range []int{301, 401, 201, 101} {
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: float64(100 + i*100)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	cards := d.cardList()
	if len(cards) != 4 {
		t.Fatalf("cards = %d", len(cards))
	}
	for i, id := range []int{101, 201, 301, 401} {
		if cards[i].item.ID != id {
			t.Errorf("position %d: got %d, want %d", i, cards[i].item.ID, id)
		}
	}
	sections, starts := d.itemsSectionsDL(cards)
	view := collapsePlain(strings.Join(sections, "\n"))
	if strings.Contains(view, "MEJORAS") {
		t.Error("upgrades still form a separate group")
	}
	for _, want := range []string{"↑ Mejora de", "~14 min", "SITUACIONALES"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q", want)
		}
	}
	for i := 1; i < len(starts); i++ {
		if starts[i] < starts[i-1] {
			t.Error("navigation disagrees with reading order")
		}
	}
	d.itemIdx = 1
	if line := collapsePlain(d.selectionLine()); !strings.Contains(line, "Mejora "+s.ItemName(101)) {
		t.Errorf("missing predecessor: %s", line)
	}
	d, _ = d.Update(keyMsg("u"))
	hidden := collapsePlain(d.View())
	if strings.Contains(hidden, "Mejora en paso 2") {
		t.Error("base points to a hidden upgrade")
	}
	cards = d.cardList()
	if len(cards) != 3 || cards[0].order != 1 || cards[1].order != 3 {
		t.Error("toggle changed route numbering")
	}
}

// Route medians must be applied before the per-phase selection cap.
func TestDeadlockRoutePhaseSelection(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	b := d.CurrentBuild()
	b.CoreItems = nil
	s.Routes = map[string][]RouteStep{"Abrams": nil}
	for i := 0; i < 9; i++ {
		id := 9000 + i
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000}, AvgBuyS: 300},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
		med := 300
		if i == 0 {
			med = 1300
		}
		s.Routes["Abrams"] = append(s.Routes["Abrams"], RouteStep{Item: id, MedT: med})
	}
	cards := d.cardList()
	if len(cards) != 9 {
		t.Fatalf("route phases should retain 1 late + 8 early cards; got %d", len(cards))
	}
	for i, c := range cards {
		if c.order != i+1 {
			t.Fatalf("phase regrouping changed consensus order: %v", cards)
		}
	}
	sections, starts := d.itemsSectionsDL(cards)
	text := collapsePlain(strings.Join(sections, "\n"))
	// La cabecera de fase nunca retrocede: si la ruta abre en LATE,
	// los pasos posteriores siguen bajo esa banda.
	if strings.Contains(text, "◆ EARLY") {
		t.Error("an earlier band reopened after LATE")
	}
	for i := 1; i < len(starts); i++ {
		if starts[i] < starts[i-1] {
			t.Error("incorrect navigation offsets")
		}
	}
	// Without a route, retain the existing time-based, eight-per-phase fallback.
	s.Routes = nil
	if got := len(d.cardList()); got != 8 {
		t.Errorf("fallback count = %d, want 8", got)
	}
}

func TestDeadlockCardVisualHierarchy(t *testing.T) {
	for _, width := range []int{60, 80, 100, 180} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			s := loadOffline(t, model.GameDeadlock)
			d := newDetailView(findChampion(t, s, "Abrams"), s)
			d.width, d.height = width, 90
			d.vp.Height = 60
			s.Names.Items[101] = "Extra Regen"
			s.Names.Items[201] = "Healing Booster"
			s.ItemMeta = map[int]staticdata.ItemInfo{}
			s.ItemMeta[101] = staticdata.ItemInfo{Name: "Extra Regen", Cost: 800, Category: "vitality", Tier: 1}
			s.ItemMeta[201] = staticdata.ItemInfo{Name: "Healing Booster", Cost: 1600, Category: "vitality", Tier: 2}
			s.Routes = map[string][]RouteStep{"Abrams": {
				{Item: 101, MedT: 300, UpgradedRate: .81},
				{Item: 201, MedT: 650, UpgradeOf: 101, ChainRate: .81},
			}}
			d.CurrentBuild().CoreItems = []model.RankedItem{
				{ItemStats: model.ItemStats{ID: 101, Stats: model.Stats{Games: 20000}, AvgBuyS: 300}, WinRate: .53, PickRate: .4, Wilson: .52},
				{ItemStats: model.ItemStats{ID: 201, Stats: model.Stats{Games: 20000}, AvgBuyS: 650}, WinRate: .55, PickRate: .3, Wilson: .54},
			}
			// Missing art must reserve exactly the same space as a real icon.
			delete(s.ItemArt, 101)
			cards := d.cardList()
			first, second := d.itemCard(0, cards[0]), d.itemCard(1, cards[1])
			if lipgloss.Height(first) != lipgloss.Height(second) {
				t.Error("unequal card heights")
			}
			if lipgloss.Width(first) != lipgloss.Width(second) {
				t.Error("unequal card widths")
			}
			if lipgloss.Width(first)*d.cardsPerRow() > width-8 {
				t.Error("cards overflow panel")
			}
			plain := collapsePlain(second)
			for _, want := range []string{"Healing Booster", "↑ Mejora de Extra Regen", "~10 min · +800 almas"} {
				if !strings.Contains(plain, want) {
					t.Errorf("missing %q: %s", want, plain)
				}
			}
			for _, unwanted := range []string{"Tras paso", "habitual:", "p30.0%", " II ", "╔", "↗"} {
				if strings.Contains(plain, unwanted) {
					t.Errorf("clutter remains: %q", unwanted)
				}
			}
			sections, _ := d.itemsSectionsDL(cards)
			if strings.Count(strings.Join(sections, "\n"), "minutos orientativos") != 1 {
				t.Error("timing disclaimer repeated or missing")
			}
			d.itemIdx = 1
			footer := d.itemsFooter()
			if lipgloss.Height(footer) > 2 {
				t.Errorf("footer height = %d", lipgloss.Height(footer))
			}
			if strings.Contains(collapsePlain(footer), "WR") {
				t.Error("statistics still in footer")
			}
			d, _ = d.Update(keyMsg("enter"))
			popup := collapsePlain(d.itemPopup())
			for _, want := range []string{"WR", "pick", "tier", "81%"} {
				if !strings.Contains(popup, want) {
					t.Errorf("popup lost %q", want)
				}
			}
		})
	}
}

func TestDeadlockCardMissingMetadata(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	d.width, d.height = 80, 90
	s.ItemMeta = nil
	c := cardItem{item: model.RankedItem{ItemStats: model.ItemStats{ID: 99999}}, kind: kindUpgrade, baseName: "Una base con un nombre muy largo para la tarjeta"}
	for _, height := range []int{15, 60} {
		d.vp.Height = height
		rendered := d.itemCard(0, c)
		if !strings.Contains(collapsePlain(rendered), "min — · coste —") {
			t.Error("missing metadata became a fabricated time or price")
		}
		if lipgloss.Width(rendered) != d.cardW()+2 {
			t.Error("long relationship overflowed the card")
		}
	}
}

func TestDeadlockScrollPinsOnlyPhaseTitle(t *testing.T) {
	for _, size := range []struct{ width, height int }{{80, 30}, {180, 65}} {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			s := loadOffline(t, model.GameDeadlock)
			d := newDetailView(findChampion(t, s, "Abrams"), s)
			d.width, d.height = size.width, size.height
			d.CurrentBuild().CoreItems = nil
			s.Routes = map[string][]RouteStep{"Abrams": nil}
			for i := 0; i < 18; i++ {
				id := 9000 + i
				minute := 180 + (i/6)*600
				s.Names.Items[id] = fmt.Sprintf("Scroll item %d", i+1)
				d.CurrentBuild().CoreItems = append(d.CurrentBuild().CoreItems, model.RankedItem{
					ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000}, AvgBuyS: float64(minute)},
					WinRate:   .53, PickRate: .4, Wilson: .52,
				})
				s.Routes["Abrams"] = append(s.Routes["Abrams"], RouteStep{Item: id, MedT: minute})
			}
			d.sync()
			pinnedFrames := 0
			check := func() {
				t.Helper()
				title := collapsePlain(d.activeSection())
				if title != "" {
					pinnedFrames++
					if lipgloss.Height(d.activeSection()) != 1 || !strings.HasPrefix(title, "◆ ") || strings.Contains(title, "Scroll item") {
						t.Fatalf("pinned content is not a phase title: %q", title)
					}
				}
				if h := lipgloss.Height(d.View()); h > d.height {
					t.Fatalf("view height %d exceeds terminal %d", h, d.height)
				}
			}
			// Both selection-driven scrolling and wheel round trips used to pin cards.
			for _, key := range []string{"down", "up"} {
				for i := 0; i < 18; i++ {
					d, _ = d.Update(keyMsg(key))
					check()
				}
			}
			if d.vp.YOffset != 0 {
				t.Error("returning to first card did not restore the top")
			}
			for _, button := range []tea.MouseButton{tea.MouseButtonWheelDown, tea.MouseButtonWheelUp} {
				for i := 0; i < 8; i++ {
					d, _ = d.Update(tea.MouseMsg{Button: button, Action: tea.MouseActionPress})
					check()
				}
			}
			if pinnedFrames == 0 {
				t.Fatal("test did not exercise sticky section titles")
			}
			if d.vp.YOffset != 0 {
				t.Error("wheel round trip did not restore the top")
			}
		})
	}
}

// Una fase que retrocede (un paso de 9 min después de uno de 10) no
// puede abrir otra vez la cabecera EARLY debajo de MID: la ruta manda.
func TestDeadlockPhaseSectionsNeverGoBack(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	d.width, d.height = 100, 90
	b := d.CurrentBuild()
	b.CoreItems = nil
	s.Routes = map[string][]RouteStep{"Abrams": nil}
	times := []int{300, 650, 580, 700}
	for i, med := range times {
		id := 7000 + i
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000}, AvgBuyS: float64(med)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
		s.Routes["Abrams"] = append(s.Routes["Abrams"], RouteStep{Item: id, MedT: med})
	}
	cards := d.cardList()
	sections, _ := d.itemsSectionsDL(cards)
	text := collapsePlain(strings.Join(sections, "\n"))
	if got := strings.Count(text, "◆ EARLY"); got != 1 {
		t.Errorf("EARLY headings = %d, want 1 (a regression reopened the band)", got)
	}
	if strings.Index(text, "◆ EARLY") > strings.Index(text, "◆ MID") {
		t.Error("EARLY shown after MID")
	}
}

// Las alternativas fuera de la ruta se leen en una sola sección al
// final, sin repetir bandas de tiempo ni decir "sin orden de ruta".
func TestDeadlockSituationalsOneSection(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	d.width, d.height = 100, 90
	b := d.CurrentBuild()
	b.CoreItems = nil
	s.Routes = map[string][]RouteStep{"Abrams": nil}
	for i, med := range []int{300, 1300} {
		id := 7100 + i
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000}, AvgBuyS: float64(med)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
		s.Routes["Abrams"] = append(s.Routes["Abrams"], RouteStep{Item: id, MedT: med})
	}
	for i, med := range []int{200, 900, 2000} {
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: 7200 + i, Stats: model.Stats{Games: 20000}, AvgBuyS: float64(med)},
			WinRate:   .53, PickRate: .2, Wilson: .52,
		})
	}
	cards := d.cardList()
	sections, _ := d.itemsSectionsDL(cards)
	text := collapsePlain(strings.Join(sections, "\n"))
	for _, unwanted := range []string{"OTRAS COMPRAS", "sin orden de ruta"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("label still says %q", unwanted)
		}
	}
	if got := strings.Count(text, "SITUACIONALES"); got != 1 {
		t.Errorf("SITUACIONALES headings = %d, want 1", got)
	}
	if strings.Index(text, "SITUACIONALES") < strings.Index(text, "LATE") {
		t.Error("situationals shown before the end of the route")
	}
}

// El corte por fase puede dejar fuera la base de una mejora visible:
// la tarjeta diría "Mejora de X" y X no estaría antes. Se recupera la
// base de la ruta.
func TestDeadlockUpgradeKeepsBase(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	d := newDetailView(findChampion(t, s, "Abrams"), s)
	d.width, d.height = 100, 90
	b := d.CurrentBuild()
	b.CoreItems = nil
	s.Routes = map[string][]RouteStep{"Abrams": nil}
	for i := 0; i < 10; i++ {
		id := 8000 + i
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000}, AvgBuyS: 300},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	base := model.RankedItem{
		ItemStats: model.ItemStats{ID: 8100, Stats: model.Stats{Games: 20000}, AvgBuyS: 400},
		WinRate:   .50, PickRate: .3, Wilson: .50, // score bajo: cae por el tope de fase
	}
	up := model.RankedItem{
		ItemStats: model.ItemStats{ID: 8200, Stats: model.Stats{Games: 20000}, AvgBuyS: 700},
		WinRate:   .55, PickRate: .3, Wilson: .54,
	}
	b.CoreItems = append(b.CoreItems, base, up)
	var route []RouteStep
	for i := 0; i < 10; i++ {
		route = append(route, RouteStep{Item: 8000 + i, MedT: 300})
		if i == 2 {
			route = append(route, RouteStep{Item: 8100, MedT: 400})
		}
	}
	route = append(route, RouteStep{Item: 8200, MedT: 700, UpgradeOf: 8100, ChainRate: .9})
	s.Routes["Abrams"] = route
	cards := d.cardList()
	baseIdx, upIdx := -1, -1
	for i, c := range cards {
		switch c.item.ID {
		case 8100:
			baseIdx = i
		case 8200:
			upIdx = i
		}
	}
	if baseIdx < 0 {
		t.Fatal("base missing while its upgrade is visible")
	}
	if upIdx < 0 {
		t.Fatal("upgrade missing")
	}
	if baseIdx > upIdx {
		t.Error("base shown after its upgrade")
	}
}
