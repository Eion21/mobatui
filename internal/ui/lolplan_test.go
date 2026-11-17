package ui

// Pruebas de las dos vistas nuevas de LoL (Ruta y Alternativas) y del
// estado de misión. Usan fixtures versionados: sin red ni caché.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── Vista A: ruta ─────────────────────────────────────────────────

// TestRutaMuestraOrdenDeCompras: los pasos salen en orden y con su
// minuto cuando existe.
func TestRutaMuestraOrdenDeCompras(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	view := gotoTab(d, "2").tabView()

	for _, want := range []string{
		"RUTA SUGERIDA", "basada en compras observadas",
		"Verdugo de krakens", "Hoja de furia de Guinsoo", "El final",
		"INVENTARIO FINAL",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("la ruta no muestra %q", want)
		}
	}
	// Los pasos aparecen en orden: la posición del 1º < la del 5º.
	first := strings.Index(view, "Verdugo de krakens")
	last := strings.Index(view, "Sombrero mortal de Rabadon")
	if first < 0 || last < 0 || first > last {
		t.Errorf("los pasos no están en orden (1º=%d, 5º=%d)", first, last)
	}
	// Con minuto medio disponible, se muestra.
	if !strings.Contains(view, "min") {
		t.Error("faltan los minutos medios de compra")
	}
}

// TestRutaEtiquetaEvidencia: la ruta se rotula como construcción
// estadística, nunca como secuencia observada.
func TestRutaEtiquetaEvidencia(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	view := gotoTab(d, "2").tabView()

	if strings.Contains(view, "secuencia observada") {
		t.Error("no debe rotularse como secuencia observada")
	}
	if !strings.Contains(view, "compra N más frecuente") &&
		!strings.Contains(view, "no es la secuencia") {
		t.Error("debe advertir que es una composición estadística")
	}
}

// TestRutaInventarioPorRol comprueba las restricciones de espacios.
func TestRutaInventarioPorRol(t *testing.T) {
	casos := []struct {
		key, name, role string
		mission         lolbuild.InventoryState
		wantSlots       string
	}{
		{"kaisa", "Kai'Sa", "BOTTOM", lolbuild.MissionPending, "5 principales + botas"},
		{"ahri", "Ahri", "MIDDLE", lolbuild.MissionUnknown, "5 principales + botas"},
		{"malphite", "Malphite", "TOP", lolbuild.MissionUnknown, "5 principales + botas"},
		{"soraka", "Soraka", "SUPPORT", lolbuild.MissionUnknown, "5 principales + botas + ranura de misión"},
	}
	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			d := planDetailView(t, c.key, c.name, 120, 60)
			view := gotoTab(d, "2").tabView()
			if !strings.Contains(view, c.wantSlots) {
				t.Errorf("no muestra %q:\n%s", c.wantSlots, view)
			}
		})
	}
}

// TestRutaNoMuestraMinutoCero: los objetos de misión no se pintan como
// compra al minuto 0.
func TestRutaNoMuestraMinutoCero(t *testing.T) {
	d := planDetailView(t, "soraka", "Soraka", 120, 60)
	view := gotoTab(d, "2").tabView()

	// "~0 min" sería un minuto inventado; "37 min" es dato real.
	if strings.Contains(view, "~0 min") {
		t.Errorf("la vista muestra un minuto 0 inventado:\n%s", view)
	}
	// Y el paso de misión no lleva marca de minutos.
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Tejesueños") && strings.Contains(line, "min") {
			t.Errorf("el objeto de misión muestra un minuto: %q", line)
		}
	}
	// El objeto de misión se identifica como tal.
	if !strings.Contains(view, "por misión") {
		t.Errorf("el objeto de misión no se identifica:\n%s", view)
	}
}

// TestRutaIncompletaSeDeclara: un plan sin sexto objeto no se presenta
// como completo.
func TestRutaIncompletaSeDeclara(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	// Simular "después de misión": 6 principales pero solo 5 con
	// evidencia → incompleto declarado.
	d.session.botMissionDone = true
	view := d.routeTabView()

	if !strings.Contains(view, "inventario incompleto") {
		t.Errorf("debía declararse incompleto:\n%s", view)
	}
	// Concordancia correcta con un solo objeto ausente.
	if !strings.Contains(view, "falta 1 objeto") {
		t.Errorf("debía explicar cuántos faltan:\n%s", view)
	}
	if strings.Contains(view, "inventario completo") {
		t.Error("no puede decir 'completo' si falta un objeto")
	}
}

// ─── Vista B: alternativas ─────────────────────────────────────────

// TestAlternativasAgrupadas: las categorías se muestran con nombres
// legibles y cada grupo trae sus objetos.
func TestAlternativasAgrupadas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 70)
	view := gotoTab(d, "3").tabView()

	if !strings.Contains(view, "ALTERNATIVAS POR FUNCIÓN") {
		t.Fatal("falta el título de la vista")
	}
	// Al menos daño físico y daño mágico aparecen.
	for _, want := range []string{"DAÑO FÍSICO", "DAÑO MÁGICO"} {
		if !strings.Contains(view, want) {
			t.Errorf("falta la categoría %q", want)
		}
	}
	// Cada alternativa lleva sus datos reales.
	for _, want := range []string{"partidas", "pick", "WR"} {
		if !strings.Contains(view, want) {
			t.Errorf("las alternativas no muestran %q", want)
		}
	}
}

// TestAlternativasLimiteAmpliado: hasta 12 candidatos distintos.
func TestAlternativasLimiteAmpliado(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 70)
	plan := d.lolPlan()
	if plan == nil {
		t.Fatal("sin plan")
	}
	alts := plan.AlternativeItems()
	if len(alts) > maxAlternativesShown {
		t.Errorf("%d alternativas: el máximo es %d", len(alts), maxAlternativesShown)
	}
	if len(alts) < 5 {
		t.Errorf("solo %d alternativas: la vista sería inútil", len(alts))
	}
	// Sin duplicados.
	seen := map[int]bool{}
	for _, a := range alts {
		if seen[a.Item] {
			t.Errorf("alternativa duplicada: %d", a.Item)
		}
		seen[a.Item] = true
	}
}

// TestAlternativasSinRepetirVisualmente: un objeto en varias categorías
// se muestra completo una vez y como referencia después.
func TestAlternativasSinRepetirVisualmente(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 70)
	plan := d.lolPlan()
	view := gotoTab(d, "3").tabView()

	// Contar cuántas veces aparece completo cada nombre de alternativa.
	for _, a := range plan.AlternativeItems() {
		// Línea completa = nombre + " · " + partidas.
		full := strings.Count(view, a.Name+" · ")
		if full > 1 {
			t.Errorf("%q se muestra completo %d veces (debe ser 1)", a.Name, full)
		}
	}
	// Y las referencias compactas existen si hay objetos multicategoría.
	multi := 0
	for _, g := range plan.Alternatives {
		if len(g.Items) > 0 {
			multi++
		}
	}
	if multi > len(plan.AlternativeItems()) {
		if !strings.Contains(view, "ya listado") {
			t.Error("faltan las referencias compactas para objetos repetidos")
		}
	}
}

// TestAlternativasExcluyeEquipados: nada del inventario aparece como
// alternativa.
func TestAlternativasExcluyeEquipados(t *testing.T) {
	for _, c := range []struct{ key, name string }{
		{"kaisa", "Kai'Sa"}, {"ahri", "Ahri"},
		{"malphite", "Malphite"}, {"soraka", "Soraka"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := planDetailView(t, c.key, c.name, 120, 70)
			plan := d.lolPlan()
			equipped := map[int]bool{}
			for _, id := range plan.Final {
				equipped[id] = true
			}
			for _, a := range plan.AlternativeItems() {
				if equipped[a.Item] {
					t.Errorf("%s está equipado y también como alternativa", a.Name)
				}
			}
		})
	}
}

// TestAlternativasCategoriasValidas: cada grupo solo contiene objetos de
// su categoría y usa etiquetas conocidas.
func TestAlternativasCategoriasValidas(t *testing.T) {
	d := planDetailView(t, "ahri", "Ahri", 120, 70)
	plan := d.lolPlan()
	for _, g := range plan.Alternatives {
		if len(g.Items) == 0 {
			t.Errorf("grupo vacío para %q", g.Category)
		}
		label := altCategoryLabel(g.Category)
		if label == "" {
			t.Errorf("sin etiqueta para %q", g.Category)
		}
		for _, a := range g.Items {
			has := false
			for _, c := range a.Categories {
				if c == g.Category {
					has = true
				}
			}
			if !has {
				t.Errorf("%s en el grupo %q sin esa categoría", a.Name, g.Category)
			}
		}
	}
}

// ─── estado de misión ──────────────────────────────────────────────

// TestMisionAntesDespues: la tecla M cambia el inventario de BOT.
func TestMisionAntesDespues(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)

	antes := d.routeTabView()
	if !strings.Contains(antes, "antes de la misión") {
		t.Errorf("estado inicial inesperado:\n%s", antes)
	}
	if !strings.Contains(antes, "principal · botas") {
		t.Error("antes de la misión las botas ocupan espacio principal")
	}

	// M alterna a "después".
	d2, cmd := d.Update(keyMsg("M"))
	if cmd == nil {
		t.Error("M debía avisar con un toast")
	}
	despues := d2.routeTabView()
	if !strings.Contains(despues, "después de la misión") {
		t.Errorf("M no cambió el estado:\n%s", despues)
	}
	if !strings.Contains(despues, "ranura de misión") {
		t.Error("después de la misión las botas van a la ranura")
	}
	// Y vuelve atrás.
	d3, _ := d2.Update(keyMsg("M"))
	if !strings.Contains(d3.routeTabView(), "antes de la misión") {
		t.Error("M debía volver a 'antes'")
	}
}

// TestMisionSoloAplicaABot: en otros roles la tecla no hace nada.
func TestMisionSoloAplicaABot(t *testing.T) {
	for _, c := range []struct{ key, name string }{
		{"ahri", "Ahri"}, {"malphite", "Malphite"}, {"soraka", "Soraka"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := planDetailView(t, c.key, c.name, 120, 60)
			before := d.session.botMissionDone
			d2, _ := d.Update(keyMsg("M"))
			if d2.session.botMissionDone != before {
				t.Errorf("%s no debe tener estado de misión de botas", c.name)
			}
			if strings.Contains(d2.routeTabView(), "simula después") {
				t.Errorf("%s no debe ofrecer el conmutador", c.name)
			}
		})
	}
}

// ─── estados especiales ────────────────────────────────────────────

// TestFallbackSinPosiciones: u.gg no trae item1..item5 → se dice.
func TestFallbackSinPosiciones(t *testing.T) {
	s := loadOffline(t, model.GameLoL) // fixtures de u.gg
	ahri := findChampion(t, s, "Ahri")
	d := newDetailView(ahri, s)
	d.width, d.height = 120, 50
	d.sync()

	view := d.routeTabView()
	if !strings.Contains(view, "no se puede reconstruir") &&
		!strings.Contains(view, "no publica tablas por posición") {
		t.Errorf("debía explicar la falta de datos:\n%s", view)
	}
}

// TestSinCatalogo: sin catálogo no se inventan planes.
func TestSinCatalogo(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 50)
	d.session.ItemCatalog = nil
	if p := d.lolPlan(); p != nil {
		t.Errorf("sin catálogo no debe haber plan: %+v", p)
	}
	if v := d.routeTabView(); !strings.Contains(v, "sin datos suficientes") {
		t.Errorf("debía indicar la falta de datos:\n%s", v)
	}
}

// TestCatalogoIncompleto: con pocos objetos se muestra lo disponible y
// se declara incompleto.
func TestCatalogoIncompleto(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	// Dejar solo tres objetos del catálogo.
	full := d.session.ItemCatalog
	reduced := map[int]lolitems.Item{}
	n := 0
	for id, it := range full {
		if n >= 3 {
			break
		}
		reduced[id] = it
		n++
	}
	d.session.ItemCatalog = reduced
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan con catálogo reducido")
	}
	if plan.Complete {
		t.Error("con un catálogo incompleto no puede estar completo")
	}
	if plan.Reason == "" {
		t.Error("debe explicar por qué falta")
	}
}

// TestPlanCacheadoNoSeRecalcula: pedir el plan dos veces devuelve el
// mismo puntero (no recalcula en el render).
func TestPlanCacheadoNoSeRecalcula(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	p1 := d.lolPlan()
	p2 := d.lolPlan()
	if p1 != p2 {
		t.Error("el plan se recalculó: debe cachearse por (campeón, rol, parche, misión)")
	}
	// Cambiar el estado de misión invalida la clave.
	d.session.botMissionDone = true
	p3 := d.lolPlan()
	if p3 == p1 {
		t.Error("cambiar la misión debe producir otro plan")
	}
}

// ─── navegación y terminales ───────────────────────────────────────

// TestTeclasDePestanaa: las pestañas nuevas son accesibles por número y
// el resto no se rompió.
func TestTeclasDePestanas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 60)
	tabs := d.tabs()
	if len(tabs) != 7 {
		t.Fatalf("pestañas = %v", tabs)
	}
	if tabs[1] != "Ruta" || tabs[2] != "Alt." {
		t.Errorf("orden inesperado: %v", tabs)
	}
	for i, name := range tabs {
		d2 := gotoTab(d, string(rune('1'+i)))
		if d2.tabs()[d2.tab] != name {
			t.Errorf("tecla %d llevó a %q, se esperaba %q", i+1, d2.tabs()[d2.tab], name)
		}
	}
	// tab recorre y vuelve.
	d2 := gotoTab(d, "7")
	d3, _ := d2.Update(keyMsg("tab"))
	if d3.tab != 0 {
		t.Errorf("tab debía volver al inicio, quedó en %d", d3.tab)
	}
}

// TestTerminalEstrecha: la barra de pestañas no desborda.
func TestTerminalEstrecha(t *testing.T) {
	for _, width := range []int{40, 50, 60, 80} {
		t.Run(string(rune('0'+width/10))+string(rune('0'+width%10)), func(t *testing.T) {
			d := planDetailView(t, "kaisa", "Kai'Sa", width, 30)
			bar := d.tabsView()
			if w := len([]rune(stripANSIStr(bar))); w > width {
				t.Errorf("barra de %d columnas en terminal de %d", w, width)
			}
			// Y la vista completa no lanza pánico ni queda vacía.
			if v := gotoTab(d, "2").tabView(); v == "" {
				t.Error("la ruta quedó vacía en terminal estrecha")
			}
		})
	}
}

// TestNoInterceptaTeclasAjenas: M no pisa u, v ni V.
func TestNoInterceptaTeclasAjenas(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	d := newDetailView(abrams, s)
	d.width, d.height = 120, 40
	before := d.showUpgrades
	d2, _ := d.Update(keyMsg("u"))
	if d2.showUpgrades == before {
		t.Error("u debía seguir plegando mejoras en Deadlock")
	}
	// M en Deadlock no cambia el estado de misión de LoL.
	d3, _ := d.Update(keyMsg("M"))
	if d3.session.botMissionDone {
		t.Error("M no debe tener efecto fuera de LoL/BOT")
	}
}

// ─── regresiones ───────────────────────────────────────────────────

// TestDeadlockSinPlan: las pestañas nuevas no aparecen en Deadlock.
func TestDeadlockSinPlan(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	d := newDetailView(abrams, s)
	for _, name := range d.tabs() {
		if name == "Ruta" || name == "Alternativas" {
			t.Errorf("Deadlock no debe tener la pestaña %q", name)
		}
	}
	if p := d.lolPlan(); p != nil {
		t.Error("no debe haber plan de LoL en Deadlock")
	}
}

// TestInvalidacionDeCache cubre las cinco dimensiones que deben
// invalidar un plan ya calculado.
func TestInvalidacionDeCache(t *testing.T) {
	base := func() detailView { return planDetailView(t, "kaisa", "Kai'Sa", 118, 60) }

	// (1) Mismo estado → se reutiliza.
	d := base()
	if d.lolPlan() != d.lolPlan() {
		t.Error("sin cambios debe reutilizar el plan")
	}

	// (2) Catálogo distinto → invalida.
	d = base()
	p := d.lolPlan()
	d.session.ItemCatalog = map[int]lolitems.Item{}
	if d.lolPlan() == p {
		t.Error("cambiar el catálogo debe invalidar el plan")
	}

	// (3) Fuente distinta → invalida.
	d = base()
	p = d.lolPlan()
	d.CurrentBuild().Source = "ugg"
	if d.lolPlan() == p {
		t.Error("cambiar la fuente debe invalidar el plan")
	}

	// (4) Muestra distinta (recarga del mismo parche) → invalida.
	d = base()
	p = d.lolPlan()
	d.CurrentBuild().Total.Games += 1000
	if d.lolPlan() == p {
		t.Error("una recarga con más partidas debe invalidar el plan")
	}

	// (5) Parche distinto → invalida.
	d = base()
	p = d.lolPlan()
	d.CurrentBuild().Patch = "16.22"
	if d.lolPlan() == p {
		t.Error("cambiar el parche debe invalidar el plan")
	}

	// (6) Misión distinta → invalida (ya cubierto, se reafirma).
	d = base()
	p = d.lolPlan()
	d.session.botMissionDone = true
	if d.lolPlan() == p {
		t.Error("cambiar la misión debe invalidar el plan")
	}

	// (7) Rol distinto → invalida.
	d = base()
	p = d.lolPlan()
	other := *d.CurrentBuild()
	other.Role = model.RoleSupport
	d.champ.ByRole[model.RoleSupport] = &other
	d.champ.Roles = append(d.champ.Roles, model.RoleSupport)
	d.roleIdx = len(d.champ.Roles) - 1
	if d.lolPlan() == p {
		t.Error("cambiar el rol debe invalidar el plan")
	}
}

// TestHuellaDeCatalogo: la huella cambia con el contenido y es estable.
func TestHuellaDeCatalogo(t *testing.T) {
	cat := loadItemCatalog(t)
	h1 := catalogFingerprint(cat)
	h2 := catalogFingerprint(cat)
	if h1 != h2 {
		t.Error("la huella debe ser estable para el mismo catálogo")
	}
	if h1 == 0 {
		t.Error("un catálogo no vacío no debe dar huella 0")
	}
	// Cambiar un nombre cambia la huella.
	mod := make(map[int]lolitems.Item, len(cat))
	for id, it := range cat {
		mod[id] = it
	}
	for id, it := range mod {
		it.Name += " x"
		mod[id] = it
		break
	}
	if catalogFingerprint(mod) == h1 {
		t.Error("cambiar un nombre debe cambiar la huella")
	}
	if catalogFingerprint(map[int]lolitems.Item{}) != 0 {
		t.Error("un catálogo vacío debe dar huella 0")
	}
}

// TestCacheInvalidaConGamesConstante es el caso que `Games` no
// distingue: recarga del mismo parche y fuente con el MISMO total de
// partidas pero objetos y estadísticas distintas. Debe producir un plan
// nuevo, no reutilizar el viejo.
func TestCacheInvalidaConGamesConstante(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	p1 := d.lolPlan()
	before := p1.Steps[0].Item
	total := d.CurrentBuild().Total.Games

	// Cambiar el objeto recomendado del primer slot, conservando
	// exactamente el mismo número total de partidas.
	cb := d.CurrentBuild()
	cb.ItemSlots[0][0].ID = 3031
	cb.ItemSlots[0][0].Stats.Wins = cb.ItemSlots[0][0].Stats.Games
	cb.ItemSlots[0][0].AvgBuyS += 120

	if cb.Total.Games != total {
		t.Fatalf("Games debía seguir constante: %d vs %d", cb.Total.Games, total)
	}

	p2 := d.lolPlan()
	if p1 == p2 {
		t.Fatal("caché obsoleta: mismo plan con datos distintos y Games constante")
	}
	if p2.Steps[0].Item == before {
		t.Errorf("el plan reutilizó el objeto %d en vez de recalcular", before)
	}
	if p2.Steps[0].Item != 3031 {
		t.Errorf("paso 1 = %d, se esperaba 3031", p2.Steps[0].Item)
	}
}

// TestStatsFingerprint: la huella cambia con cualquier dato relevante y
// es estable cuando no cambia nada.
func TestStatsFingerprint(t *testing.T) {
	base := func() *model.CleanBuild {
		return toCleanBuildFixture(t)
	}
	h1 := statsFingerprint(base())
	if h1 != statsFingerprint(base()) {
		t.Error("la huella debe ser determinista")
	}

	cambios := map[string]func(*model.CleanBuild){
		"wins totales": func(cb *model.CleanBuild) { cb.Total.Wins++ },
		"id de slot":   func(cb *model.CleanBuild) { cb.ItemSlots[0][0].ID = 3031 },
		"games slot":   func(cb *model.CleanBuild) { cb.ItemSlots[0][0].Games++ },
		"wins slot":    func(cb *model.CleanBuild) { cb.ItemSlots[0][0].Wins++ },
		"minuto medio": func(cb *model.CleanBuild) { cb.ItemSlots[0][0].AvgBuyS += 1 },
		"botas":        func(cb *model.CleanBuild) { cb.Boots[0].ID = 3111 },
		// El core del fixture no viene en item_slots: se puebla a mano
		// para probar que también entra en la huella.
		"core": func(cb *model.CleanBuild) {
			cb.CoreItems = []model.RankedItem{{ItemStats: model.ItemStats{ID: 3089}}}
		},
	}
	for name, mut := range cambios {
		cb := base()
		mut(cb)
		if statsFingerprint(cb) == h1 {
			t.Errorf("la huella no cambió al mutar %s", name)
		}
	}
	if statsFingerprint(nil) != 0 {
		t.Error("nil debe dar huella 0")
	}
}

// ─── despacho de pestañas ──────────────────────────────────────────

// TestCadaPestanaTieneSuVista: las siete pestañas de LoL despachan a
// vistas distintas. Regresión: al abreviar `Matchups` a `Match.` el
// switch por etiqueta dejaba las dos últimas en el caso por defecto y
// ambas mostraban la rejilla de habilidades.
func TestCadaPestanaTieneSuVista(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	if len(d.tabs()) != 7 {
		t.Fatalf("pestañas = %v", d.tabs())
	}
	// Firma de cada vista: un texto que solo aparece en ella.
	firma := []string{
		"ORDEN DE COMPRA",          // Ítems
		"RUTA SUGERIDA",            // Ruta
		"ALTERNATIVAS POR FUNCIÓN", // Alt.
		"ORDEN DE COMPRA",          // Resumen (comparte resumen jugable)
		"runas",                    // Runas
		"",                         // Habil. (vacío en el fixture: se comprueba aparte)
		"",                         // Match. (idem)
	}
	for i := range d.tabs() {
		d2 := gotoTab(d, string(rune('1'+i)))
		if got := d2.tabs()[d2.tab]; got != d.tabs()[i] {
			t.Errorf("tecla %d llevó a %q", i+1, got)
		}
		if firma[i] == "" {
			continue
		}
		if v := d2.tabView(); !strings.Contains(v, firma[i]) {
			t.Errorf("pestaña %q no muestra su vista (falta %q)", d.tabs()[i], firma[i])
		}
	}

	// Habilidades y Matchups NO comparten vista: una pide una secuencia
	// y la otra enfrentamientos.
	hab := gotoTab(d, "6").tabView()
	mat := gotoTab(d, "7").tabView()
	if hab == mat {
		t.Error("Habilidades y Matchups muestran lo mismo")
	}
	if !strings.Contains(hab, "secuencia") {
		t.Errorf("la pestaña de habilidades no habla de secuencia: %q", hab)
	}
	if !strings.Contains(mat, "enfrentamientos") {
		t.Errorf("la pestaña de matchups no habla de enfrentamientos: %q", mat)
	}
}

// TestMensajesVaciosDistintos: el texto de cada pestaña vacía es
// específico, no un genérico "sin muestra suficiente".
func TestMensajesVaciosDistintos(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	hab := gotoTab(d, "6").tabView()
	if strings.Contains(hab, "sin secuencia con muestra suficiente") {
		t.Errorf("habilidades usa el mensaje genérico: %q", hab)
	}
	mat := gotoTab(d, "7").tabView()
	if strings.Contains(mat, "sin datos de matchups para este héroe") {
		t.Errorf("matchups en LoL habla de 'héroe': %q", mat)
	}
	// Deadlock conserva su vocabulario.
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	dd := newDetailView(abrams, s)
	dd.width, dd.height = 118, 40
	dd.sync()
	if v := gotoTab(dd, "3").tabView(); !strings.Contains(v, "héroe") {
		t.Errorf("Deadlock debe decir 'héroe': %q", v)
	}
}

// TestCabeceraSinSeparadorColgando: sin prioridad de habilidades no
// queda un "·" suelto al final de las métricas.
func TestCabeceraSinSeparadorColgando(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	head := stripANSIStr(d.headerBlock())
	for _, line := range strings.Split(head, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, "·") || strings.HasSuffix(trimmed, "· ") {
			t.Errorf("separador colgando en la cabecera: %q", trimmed)
		}
	}
}

// TestTextosDeAlt: el resumen no promete un delta que no se muestra.
func TestTextosDeAlt(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 70)
	view := gotoTab(d, "3").tabView()
	if strings.Contains(view, "delta") {
		t.Error("Alt. no muestra el delta de WR: no debe mencionarlo")
	}
	if !strings.Contains(view, "no causal") {
		t.Error("debe conservar la advertencia de que el WR no es causal")
	}
}

// TestConcordanciaSingularPlural: "falta 1 objeto" vs "faltan N".
func TestConcordanciaSingularPlural(t *testing.T) {
	// Kai'Sa con la misión completada tiene 6 principales pero solo 5
	// candidatos: falta exactamente 1.
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.session.botMissionDone = true
	plan := d.lolPlan()
	if plan.Complete {
		t.Skip("el fixture tiene seis principales")
	}
	if !strings.Contains(plan.Reason, "falta 1 objeto") {
		t.Errorf("con un objeto ausente: %q", plan.Reason)
	}
	if strings.Contains(plan.Reason, "faltan 1") {
		t.Errorf("concordancia incorrecta: %q", plan.Reason)
	}
	// Y en pantalla se lee igual.
	if v := d.routeTabView(); !strings.Contains(v, "falta 1 objeto") {
		t.Errorf("la vista no refleja la concordancia: %q", v)
	}
}

// TestApoyoNoIncluyeLuden: regresión del sesgo de clasificación.
func TestApoyoNoIncluyeLuden(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 70)
	view := gotoTab(d, "3").tabView()
	if strings.Contains(view, "APOYO") {
		// Si hay sección de apoyo, Eco de Luden no puede estar dentro.
		idx := strings.Index(view, "APOYO")
		if rest := view[idx:]; strings.Contains(rest, "Eco de Luden") {
			t.Error("Eco de Luden aparece bajo APOYO")
		}
	}
}

// TestAyudaDePestanasPorJuego: LoL anuncia 1-7 y Deadlock 1-4; antes
// ambos decían "1-3" con siete pestañas disponibles.
func TestAyudaDePestanasPorJuego(t *testing.T) {
	casos := []struct {
		game  model.Game
		champ string
		want  string
		avoid string
	}{
		{model.GameLoL, "Ahri", "tab/1-7", "tab/1-3"},
		{model.GameDeadlock, "Abrams", "tab/1-4", "tab/1-7"},
	}
	for _, c := range casos {
		s := loadOffline(t, c.game)
		ch := findChampion(t, s, c.champ)
		a := NewApp(Loader{})
		a.screen = screenDetail
		a.detail = newDetailView(ch, s)
		footer := stripANSIStr(a.footer())
		if !strings.Contains(footer, c.want) {
			t.Errorf("%s: la ayuda no dice %q: %s", c.game, c.want, footer)
		}
		if strings.Contains(footer, c.avoid) {
			t.Errorf("%s: la ayuda no debe decir %q: %s", c.game, c.avoid, footer)
		}
	}
}

// TestAyudaDeMisionSoloEnBot: la tecla M aparece en la ayuda solo donde
// tiene efecto.
func TestAyudaDeMisionSoloEnBot(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	a := NewApp(Loader{})
	a.screen = screenDetail
	a.detail = d
	if f := stripANSIStr(a.footer()); !strings.Contains(f, "misión") {
		t.Errorf("BOT debe anunciar la tecla de misión: %s", f)
	}
	// Un rol sin ranura de botas no la anuncia.
	d2 := planDetailView(t, "ahri", "Ahri", 118, 60)
	a.detail = d2
	if f := stripANSIStr(a.footer()); strings.Contains(f, "misión") {
		t.Errorf("MID no debe anunciar la tecla de misión: %s", f)
	}
}

// ─── diagnóstico de rutas de Deadlock ──────────────────────────────

// TestRazonDeRutaAusenteDistingueCausas: la TUI no usa el mismo texto
// para un fallo de fuente y para una muestra insuficiente.
func TestRazonDeRutaAusenteDistingueCausas(t *testing.T) {
	casos := []struct {
		why  routeReason
		want string
	}{
		{routeReasonNoSample, "sin muestra de compras suficiente"},
		{routeReasonSource, "fuente no disponible"},
		{routeReasonUnknown, "héroe fuera del catálogo"},
		{routeReasonNone, ""},
	}
	for _, c := range casos {
		if got := c.why.short(); got != c.want {
			t.Errorf("routeReason(%d).short() = %q, se esperaba %q", c.why, got, c.want)
		}
	}
}

// TestClassifyRunsError: los errores del loader se clasifican bien.
func TestClassifyRunsError(t *testing.T) {
	if got := classifyRunsError(nil); got != routeReasonNone {
		t.Errorf("nil = %v", got)
	}
	if got := classifyRunsError(errors.New("HTTP 504")); got != routeReasonSource {
		t.Errorf("error de red = %v, se esperaba fuente", got)
	}
	if got := classifyRunsError(errors.New("héroe desconocido: Nurse")); got != routeReasonUnknown {
		t.Errorf("héroe desconocido = %v", got)
	}
	// Un timeout también es fuente.
	if got := classifyRunsError(context.DeadlineExceeded); got != routeReasonSource {
		t.Errorf("timeout = %v", got)
	}
}

// TestRutaInexistenteReintentaSoloSiFallóLaFuente: un fallo de fuente se
// reintenta al reabrir; una muestra insuficiente no.
func TestRutaInexistenteReintentaSoloSiFallóLaFuente(t *testing.T) {
	base := func(why routeReason) App {
		s := &Session{
			Game:     model.GameDeadlock,
			Routes:   map[string][]RouteStep{"Abrams": {}},
			RouteWhy: map[string]routeReason{"Abrams": why},
			Runs:     staticRuns{},
		}
		a := App{session: s}
		a.detail.champ = &ChampEntry{Key: "Abrams", Name: "Abrams"}
		return a
	}
	// Fuente caída: se reintenta (la entrada se limpia y vuelve a pedir).
	a := base(routeReasonSource)
	if cmd := runsCmdFor(a); cmd == nil {
		t.Error("un fallo de fuente debe reintentar la ruta")
	}
	// Muestra insuficiente: no se reintenta.
	a = base(routeReasonNoSample)
	if cmd := runsCmdFor(a); cmd != nil {
		t.Error("una muestra insuficiente no debe reintentarse")
	}
	// Ruta ya cargada: no se repite.
	s := &Session{
		Game:   model.GameDeadlock,
		Routes: map[string][]RouteStep{"Abrams": {{Item: 1}}},
		Runs:   staticRuns{},
	}
	a = App{session: s}
	a.detail.champ = &ChampEntry{Key: "Abrams", Name: "Abrams"}
	if cmd := runsCmdFor(a); cmd != nil {
		t.Error("una ruta cargada no debe pedirse otra vez")
	}
}

// staticRuns es un loader vacío para las pruebas de decisión.
type staticRuns struct{}

func (staticRuns) LoadRuns(string) ([]RouteStep, error) { return nil, nil }

// TestContrasteDeDeadlock: tras subir colDim en la fase 7.1, el texto
// secundario de Deadlock debe seguir siendo legible.
func TestContrasteDeDeadlock(t *testing.T) {
	if r := contrastRatio(colDim, colPanel); r < 4.5 {
		t.Errorf("colDim sobre el panel tiene ratio %.2f: por debajo de 4.5", r)
	}
	// Y los colores de datos siguen por encima.
	for name, c := range map[string]lipgloss.Color{
		"gold": colGold, "goldLight": colGoldLight, "teal": colTeal,
	} {
		if r := contrastRatio(c, colPanel); r < 4.5 {
			t.Errorf("%s sobre el panel tiene ratio %.2f", name, r)
		}
	}
}

// TestHabilidadesSoloPrioridad: cuando la fuente no da una secuencia por
// nivel válida pero sí una prioridad, se muestra la prioridad
// declarándola como tal, no una rejilla inventada.
func TestHabilidadesSoloPrioridad(t *testing.T) {
	s, champ := planSession(t, "kaisa", "Kai'Sa")
	cb := champ.ByRole[champ.Roles[0]]
	// Simular lo que deja el pipeline real para Kai'Sa: sin secuencia,
	// con la prioridad que publica Lolalytics.
	cb.SkillOrder.Sequence = ""
	cb.SkillOrder.Priority = []rune("QEW")
	cb.SkillOrder.Stats = model.Stats{Games: 55453, Wins: 31724}

	d := newDetailView(champ, s)
	d.width, d.height = 118, 40
	d.sync()

	view := gotoTab(d, "6").tabView()
	if !strings.Contains(view, "prioridad") {
		t.Errorf("debe mostrar la prioridad: %q", view)
	}
	for _, want := range []string{"Q", "E", "W"} {
		if !strings.Contains(view, want) {
			t.Errorf("falta la habilidad %q en la prioridad: %q", want, view)
		}
	}
	if !strings.Contains(view, "no publica una secuencia por nivel válida") {
		t.Errorf("debe declarar que no hay secuencia: %q", view)
	}
	// Y no finge una rejilla nivel a nivel.
	if strings.Contains(view, "Nv ") || strings.Contains(view, "nivel 1 ") {
		t.Errorf("no debe dibujar una rejilla inventada: %q", view)
	}
}

// TestHabilidadesConSecuenciaValida: cuando sí hay secuencia, se muestra
// la rejilla y NO el aviso de prioridad sola.
func TestHabilidadesConSecuenciaValida(t *testing.T) {
	s, champ := planSession(t, "kaisa", "Kai'Sa")
	cb := champ.ByRole[champ.Roles[0]]
	cb.SkillOrder.Sequence = "WQEQQRQWQWRWWEE"
	cb.SkillOrder.Priority = []rune("QWE")
	cb.SkillOrder.Stats = model.Stats{Games: 3133, Wins: 1600}

	d := newDetailView(champ, s)
	d.width, d.height = 118, 40
	d.sync()

	view := gotoTab(d, "6").tabView()
	if strings.Contains(view, "no publica una secuencia por nivel válida") {
		t.Errorf("con secuencia válida no debe avisar: %q", view)
	}
	// La rejilla de LoL se reconoce por el eje de niveles y la fila de
	// habilidades, no por el marcador ◆ de Deadlock.
	if !strings.Contains(view, "1  2  3") {
		t.Errorf("debe dibujar la rejilla de niveles: %q", view)
	}
}

// TestBarraCompactaRegistraZona: en terminales estrechas la barra de
// pestañas se compacta a solo la activa. Debe seguir siendo clicable.
//
// Regresión del CI de Windows: la rama compacta retornaba el texto sin
// `zoneMgr.Mark`, así que no se registraba NINGUNA zona y el clic en
// pestañas dejaba de funcionar. `TestClicEnPestanaYTarjeta` fallaba con
// "zona tab:1 no registrada" cuando el terminal era más estrecho.
func TestBarraCompactaRegistraZona(t *testing.T) {
	for _, width := range []int{40, 50, 60} {
		d := planDetailView(t, "kaisa", "Kai'Sa", width, 40)
		// Las zonas se registran al renderizar (View), no al sincronizar.
		_ = d.View()
		// La pestaña activa siempre debe ser clicable.
		active := "tab:" + string(rune('0'+d.tab))
		if !waitZone(active) {
			t.Errorf("a %d columnas no se registró la zona %q", width, active)
		}
	}
}

// TestBarraCompletaRegistraTodas: con ancho suficiente las siete
// pestañas son clicables.
func TestBarraCompletaRegistraTodas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 120, 45)
	// Las zonas se registran al renderizar (View), no al sincronizar.
	v := d.View()
	if v == "" {
		t.Fatal("la vista quedó vacía")
	}
	for i := range d.tabs() {
		id := "tab:" + string(rune('0'+i))
		if !waitZone(id) {
			t.Errorf("no se registró la zona %q", id)
		}
	}
}
