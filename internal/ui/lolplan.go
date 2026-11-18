package ui

// Integración del planificador de LoL (internal/lolbuild) con la TUI.
//
// El plan NO se recalcula en cada render de Bubble Tea: se calcula una
// vez por (campeón, rol, parche y estado de misión) y se guarda en la
// sesión. Así el scroll y la navegación no pagan el coste y la caché de
// Data Dragon se reutiliza sin peticiones nuevas.

import (
	"bytes"
	"fmt"
	"image/png"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/imgrender"
	"github.com/Eion21/mobatui/internal/staticdata"

	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
)

// lolPlanKey identifica un plan cacheado.
type lolPlanKey struct {
	Champ   string
	Role    model.Role
	Patch   string
	Mission lolbuild.InventoryState
	// Source distingue lolalytics de u.gg para el mismo campeón/rol.
	Source string
	// Stats es una huella del CONTENIDO estadístico (stats por posición,
	// core y botas), no solo del número de partidas. Sin ella, una
	// recarga con el mismo total de partidas pero objetos distintos
	// reutilizaba el plan viejo.
	Stats uint64
	// Catalog identifica el catálogo usado. Si cambia (parche nuevo,
	// recarga de Data Dragon), el plan anterior ya no vale.
	Catalog uint64
}

// LolPlan devuelve el plan del campeón/rol actual, calculándolo solo si
// cambió la clave. Devuelve nil si no hay datos suficientes para
// intentarlo (sin build, sin catálogo).
func (s *Session) LolPlan(cb *model.CleanBuild, champKey string, role model.Role, mission lolbuild.InventoryState) *lolbuild.Plan {
	if cb == nil || s.ItemCatalog == nil {
		return nil
	}
	key := lolPlanKey{
		Champ:   champKey,
		Role:    role,
		Patch:   cb.Patch,
		Mission: mission,
		Source:  cb.Source,
		Stats:   statsFingerprint(cb),
		Catalog: catalogFingerprint(s.ItemCatalog),
	}
	if s.lolPlans == nil {
		s.lolPlans = map[lolPlanKey]*lolbuild.Plan{}
	}
	if p, ok := s.lolPlans[key]; ok {
		return p
	}
	plan := lolbuild.BuildPlan(cb, s.ItemCatalog, lolbuild.Options{
		Mission:              mission,
		MaxAlternativesTotal: maxAlternativesShown,
	})
	s.lolPlans[key] = &plan
	return &plan
}

// maxAlternativesShown es cuántos candidatos distintos como máximo se
// muestran en la vista de alternativas. El planificador acota además
// por categoría para que ninguna sección crezca sin límite.
const maxAlternativesShown = 12

// MissionStateFor decide qué estado de misión usar para un rol.
//
// Solo BOT tiene la ranura de botas que cambia el inventario, así que
// solo ahí tiene sentido el selector antes/después. En el resto se deja
// como desconocida: no se inventa un estado que no aplica.
func (s *Session) MissionStateFor(role model.Role) lolbuild.InventoryState {
	if role != model.RoleBottom {
		return lolbuild.MissionUnknown
	}
	if s.botMissionDone {
		return lolbuild.MissionComplete
	}
	return lolbuild.MissionPending
}

// ToggleBotMission alterna el estado de misión de BOT. Devuelve true si
// el rol lo admite.
func (s *Session) ToggleBotMission(role model.Role) bool {
	if role != model.RoleBottom {
		return false
	}
	s.botMissionDone = !s.botMissionDone
	return true
}

// ─── Vista A: ruta de compra ───────────────────────────────────────

// wrapLines envuelve las líneas al ancho útil de la vista. Se aplica al
// final: así cada línea puede construirse con formato y el texto largo
// (notas, motivos) rompe por palabras en vez de cortarse a media
// palabra, que es lo que hace el viewport al recortar.
func (d detailView) wrapLines(lines []string) []string {
	inner := max(24, d.width-10)
	style := lipgloss.NewStyle().Width(inner)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			out = append(out, l)
			continue
		}
		out = append(out, style.Render(l))
	}
	return out
}

// routeViewA renderiza la ruta sugerida y el inventario final.
func (d detailView) routeViewA(plan *lolbuild.Plan) []string {
	if plan == nil {
		return nil
	}
	var out []string

	// Título con la etiqueta de evidencia exigida: los pasos item1..item5
	// son posiciones observadas por separado, la ruta es una
	// construcción estadística.
	out = append(out, stSection.Render("◆ RUTA SUGERIDA · basada en compras observadas"))
	if note := routeEvidenceNote(plan); note != "" {
		out = append(out, stDim.Render("  "+note))
	}

	// Pasos.
	if len(plan.Steps) == 0 {
		out = append(out, stDim.Render("  sin pasos de compra con muestra suficiente"))
	}
	// Cada paso lleva su zona para poder seleccionarlo y abrir su
	// popup con el mouse, igual que las tarjetas de Ítems.
	for i, st := range plan.Steps {
		out = append(out, zoneMgr.Mark(fmt.Sprintf("card:%d", i), d.routeStepLine(st)))
	}

	// Botas: se muestran aparte, sin inventar una posición exacta.
	if len(plan.Boots) > 0 {
		b := plan.Boots[0]
		var line string
		if b.BuyMinute > 0 {
			line = fmt.Sprintf("  botas: %s · ~%d min", stKeyName.Render(b.Name), b.BuyMinute)
		} else {
			line = fmt.Sprintf("  botas: %s", stKeyName.Render(b.Name))
		}
		// La etiqueta de muestra va DENTRO del mismo Render: dos
		// llamadas a Render por línea añadían una allocación por paso
		// sin cambiar el resultado visible.
		line += stDim.Render(fmt.Sprintf(" · %s partidas · %s%s",
			nThousand(b.Stats.Games), pct(model.Stats.WinRate(b.Stats)),
			sampleSuffix(b.Stats.Games)))
		// Las botas son la última fila de tabCardList: misma zona que
		// las compras para que el clic y el teclado la alcancen.
		out = append(out, zoneMgr.Mark(fmt.Sprintf("card:%d", len(plan.Steps)), line))
	} else {
		out = append(out, stDim.Render("  botas: sin recomendación con muestra suficiente"))
	}

	// Inventario final y estado de misión.
	out = append(out, d.finalInventoryLines(plan)...)
	return out
}

// smallArtCols es el ancho del icono compacto en Ruta y Alt.
//
// Tres columnas producen exactamente UNA fila de medio bloque. Con 4
// el icono ocupa dos filas y rompería el formato de un paso por línea.
const smallArtCols = 3

// itemIconInline devuelve el icono compacto de un ítem con un espacio
// final, o "" si no hay arte disponible.
//
// Degradación: sin PNG, sin proveedor o con terminal estrecho se
// devuelve vacío y la línea queda exactamente como antes. No se hace
// ninguna petición HTTP aquí: el arte se prerenderiza en la carga o al
// abrir el detalle.
func (d detailView) itemIconInline(id int) string {
	if id <= 0 {
		return ""
	}
	// En terminales muy estrechas el icono desplazaría el nombre.
	if d.width > 0 && d.width < 60 {
		return ""
	}
	icon := d.session.ItemArtFor(id, smallArtCols)
	if icon == "" {
		return ""
	}
	return icon + " "
}

// routeStepLine formatea un paso de compra.
func (d detailView) routeStepLine(st lolbuild.Step) string {
	if st.Quest {
		// Objeto de misión: se obtiene/mejora por misión, no se compra
		// en un minuto concreto. Nunca se muestra "0 min".
		return fmt.Sprintf("  %s %s%s",
			stDim.Render(fmt.Sprintf("%d.", st.Slot)),
			stKeyName.Render(st.Name),
			stWinrateGood.Render(" · por misión"))
	}
	pos := fmt.Sprintf("%d.", st.Slot)
	// Icono solo para compras reales: un objeto de misión no es una
	// compra y ya lleva su propia etiqueta.
	icon := d.itemIconInline(st.Item)
	line := fmt.Sprintf("  %s %s%s", stDim.Render(pos), icon, stKeyName.Render(st.Name))
	if st.BuyMinute > 0 {
		line += stDim.Render(fmt.Sprintf(" · ~%d min", st.BuyMinute))
	}
	line += stDim.Render(fmt.Sprintf(" · %s partidas · %s%s",
		nThousand(st.Stats.Games), pct(model.Stats.WinRate(st.Stats)),
		sampleSuffix(st.Stats.Games)))
	if st.Boots {
		line += stDim.Render(" · botas")
	}
	return line
}

// routeEvidenceNote explica la procedencia de la ruta.
func routeEvidenceNote(plan *lolbuild.Plan) string {
	switch plan.Evidence {
	case lolbuild.EvidenceFallback:
		return "la fuente no publica posiciones: ruta no reconstruible"
	case lolbuild.EvidenceComposed:
		return "cada paso es la compra N más frecuente; no es la secuencia de una partida real"
	default:
		return ""
	}
}

// finalInventoryLines describe el inventario final y la misión.
func (d detailView) finalInventoryLines(plan *lolbuild.Plan) []string {
	var out []string
	inv := plan.Inventory

	state := string(inv.State)
	if inv.State == lolbuild.MissionUnknown {
		state = "desconocida"
	}
	slotInfo := fmt.Sprintf("%d principales", inv.MainSlots)
	if inv.BootsSlot {
		slotInfo += " + botas"
	}
	if inv.QuestSlot {
		slotInfo += fmt.Sprintf(" + ranura de misión (%s)", inv.QuestSlotUse)
	}
	out = append(out, stSection.Render("◆ INVENTARIO FINAL"))
	out = append(out, stDim.Render(fmt.Sprintf("  misión: %s · %s", state, slotInfo)))

	// Objetos equipados, distinguiendo ranura.
	for _, id := range plan.Final {
		meta, hasMeta := d.session.ItemCatalog[id]
		name := d.session.ItemName(id)
		slot := "principal"
		badge := ""
		if hasMeta {
			if lolitems.IsBoots(meta) {
				if inv.BootsSlot {
					slot = "principal · botas"
				} else {
					slot = "ranura de misión"
				}
			}
			if lolitems.IsQuestItem(id) {
				badge = " · misión"
			}
		}
		out = append(out, fmt.Sprintf("  · %s%s %s",
			stKeyName.Render(name), stWinrateGood.Render(badge), stDim.Render("("+slot+")")))
	}

	// Completitud: nunca presentar un plan incompleto como completo.
	if plan.Complete {
		out = append(out, stWinrateGood.Render("  inventario completo"))
		return out
	}
	out = append(out, stWinrateBad.Render("  inventario incompleto: "+plan.Reason))
	out = append(out, d.candidateLines(plan)...)
	return out
}

// candidateLines muestra los objetos que PODRÍAN completar el hueco.
//
// Etiqueta explícita: "sin orden observado". No son una sexta compra
// observada ni aparecen dentro de item1..item5: la fuente no publica
// inventarios finales ni coocurrencia con esta build, así que solo se
// ofrece la estadística del objeto en este campeón/rol/parche.
func (d detailView) candidateLines(plan *lolbuild.Plan) []string {
	cands := plan.CandidatesFor()
	if len(cands) == 0 {
		// Sin datos suficientes: se mantiene el aviso de incompleto y no
		// se inventa nada.
		return nil
	}
	out := []string{
		"",
		stSection.Render("◆ CANDIDATOS PARA COMPLETAR · sin orden observado"),
	}
	if d.width >= 60 {
		out = append(out, stDim.Render(
			"  no son una sexta compra observada: la fuente no publica el inventario final"))
	}
	for _, c := range cands {
		line := fmt.Sprintf("  · %s%s", d.itemIconInline(c.Item), stKeyName.Render(c.Name))
		line += stDim.Render(fmt.Sprintf(" · %s partidas", nThousand(c.Stats.Games)))
		if c.PickRate > 0 {
			line += stDim.Render(fmt.Sprintf(" · pick %.0f%%", c.PickRate))
		}
		if label := sampleSuffix(c.Stats.Games); label != "" {
			line += stDim.Render(fmt.Sprintf(" · WR %s%s", pct(c.WinRate), label))
		} else if c.WinRate > 0 {
			line += stDim.Render(fmt.Sprintf(" · WR %s", pct(c.WinRate)))
		}
		out = append(out, line)
	}
	return out
}

// ─── Vista B: alternativas por función ──────────────────────────────

// altCategoryLabel traduce una categoría interna a un nombre legible.
func altCategoryLabel(c lolitems.Category) string {
	switch c {
	case lolitems.CatPhysicalDamage:
		return "DAÑO FÍSICO"
	case lolitems.CatMagicDamage:
		return "DAÑO MÁGICO"
	case lolitems.CatArmorPen:
		return "PENETRACIÓN FÍSICA"
	case lolitems.CatMagicPen:
		return "PENETRACIÓN MÁGICA"
	case lolitems.CatAttackSpeed:
		return "VELOCIDAD DE ATAQUE"
	case lolitems.CatBoots:
		return "BOTAS"
	case lolitems.CatMovement:
		return "MOVIMIENTO"
	case lolitems.CatArmor:
		return "ARMADURA"
	case lolitems.CatHealth:
		return "VIDA"
	case lolitems.CatMagicResist:
		return "RESISTENCIA MÁGICA"
	case lolitems.CatAntiHeal:
		return "ANTICURACIÓN"
	case lolitems.CatUtility:
		return "UTILIDAD"
	case lolitems.CatSupport:
		return "APOYO"
	default:
		return string(c)
	}
}

// alternativesViewB renderiza las alternativas agrupadas por función.
//
// Un objeto puede pertenecer a varias categorías; para no repetirlo en
// cada bloque se muestra completo solo en su PRIMERA categoría y en el
// resto aparece como referencia compacta ("ya listado arriba").
func (d detailView) alternativesViewB(plan *lolbuild.Plan) []string {
	if plan == nil {
		return nil
	}
	var out []string
	alts := plan.AlternativeItems()
	out = append(out, stSection.Render("◆ ALTERNATIVAS POR FUNCIÓN"))
	if len(alts) == 0 {
		out = append(out, stDim.Render("  sin alternativas con muestra suficiente para este campeón"))
		return out
	}
	out = append(out, stDim.Render(fmt.Sprintf(
		// La vista muestra WR ABSOLUTO de cada objeto, no la diferencia
		// frente al campeón: el texto no puede prometer un delta.
		"  %d candidatos con evidencia de uso · el WR es descriptivo, no causal", len(alts))))

	// Primera categoría de cada objeto: donde se muestra completo.
	primary := map[int]lolitems.Category{}
	for _, g := range plan.Alternatives {
		for _, a := range g.Items {
			if _, ok := primary[a.Item]; !ok {
				primary[a.Item] = g.Category
			}
		}
	}

	for _, g := range plan.Alternatives {
		if len(g.Items) == 0 {
			continue
		}
		out = append(out, stSection.Render("◆ "+altCategoryLabel(g.Category)))
		for _, a := range g.Items {
			if primary[a.Item] != g.Category {
				// Ya se mostró arriba: referencia compacta.
				out = append(out, stDim.Render(fmt.Sprintf("  ↑ %s (ya listado)", a.Name)))
				continue
			}
			// El índice de zona es el de tabCardList (una por candidato
			// mostrado), para que el clic abra el popup correcto.
			out = append(out, zoneMgr.Mark(fmt.Sprintf("card:%d", altZoneIdx(plan, a.Item)), d.alternativeLine(a)))
		}
	}
	return out
}

// alternativeLine formatea una alternativa con sus datos reales.
func (d detailView) alternativeLine(a lolbuild.Alternative) string {
	line := fmt.Sprintf("  %s%s", d.itemIconInline(a.Item), stKeyName.Render(a.Name))
	line += stDim.Render(fmt.Sprintf(" · %s partidas", nThousand(a.Stats.Games)))
	line += stDim.Render(fmt.Sprintf(" · pick %.0f%%", a.PickRate))
	if a.WinRate > 0 {
		line += stWinrateGood.Render(fmt.Sprintf(" · WR %s%s", pct(a.WinRate),
			sampleSuffix(a.Stats.Games)))
	} else {
		line += stDim.Render(sampleSuffix(a.Stats.Games))
	}
	return line
}

// ─── helpers de sesión ─────────────────────────────────────────────

// catalogFingerprint resume el catálogo para invalidar el plan cuando
// cambia. Se apoya en el tamaño y en una suma de ids y nombres: es
// barato (una vez por consulta cacheada, no por frame) y suficiente para
// detectar recargas de Data Dragon.
func catalogFingerprint(cat map[int]lolitems.Item) uint64 {
	if len(cat) == 0 {
		return 0
	}
	// FNV-1a sobre tamaño + id + nombre. La suma es conmutativa, así que
	// el orden de iteración del mapa no altera el resultado.
	h := uint64(1469598103934665603)
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	mix(uint64(len(cat)))
	var sum uint64
	for id, it := range cat {
		var n uint64
		for _, b := range []byte(it.Name) {
			n = n*31 + uint64(b)
		}
		sum += uint64(id)*1000003 ^ n
	}
	mix(sum)
	return h
}

// statsFingerprint resume los datos estadísticos que alimentan el plan:
// estadísticas totales, las tablas por posición, el core y las botas.
// Detecta recargas que cambian recomendaciones SIN cambiar el total de
// partidas, que es justo lo que `Games` no distingue.
//
// Es barato (una vez por consulta, no por frame) y determinista: el
// orden de las filas ya es fijo en el CleanBuild.
func statsFingerprint(cb *model.CleanBuild) uint64 {
	if cb == nil {
		return 0
	}
	h := uint64(1469598103934665603)
	mix := func(v uint64) {
		h ^= v
		h *= 1099511628211
	}
	mixU := func(items ...uint64) {
		for _, v := range items {
			mix(v)
		}
	}

	mixU(uint64(cb.Total.Games), uint64(cb.Total.Wins))

	item := func(it model.ItemStats) {
		mixU(uint64(it.ID), uint64(it.Games), uint64(it.Wins), uint64(it.AvgBuyS))
	}
	for _, slot := range cb.ItemSlots {
		mix(uint64(len(slot)) + 1)
		for _, it := range slot {
			item(it)
		}
	}
	mix(uint64(len(cb.CoreItems)) + 2)
	for _, it := range cb.CoreItems {
		item(it.ItemStats)
	}
	mix(uint64(len(cb.Boots)) + 3)
	for _, bt := range cb.Boots {
		item(bt.ItemStats)
	}
	return h
}

// ─── arte bajo demanda de Ruta y Alt. ─────────────────────────────

// ensureTabArt prerenderiza el arte de los ítems que la pestaña actual
// va a mostrar, solo si esa pestaña los usa.
//
// Es bajo demanda —al abrir Ruta o Alt.— y no una precarga global: no
// toca Ítems, Runas ni ninguna otra pestaña, y no descarga nada de red
// que no se fuera a descargar igualmente.
func (d *detailView) ensureTabArt() {
	if d.session == nil || d.session.Game != model.GameLoL {
		return
	}
	// Runas: el arte se carga en un Cmd asíncrono (ver runeArtCmdFor);
	// aquí no se descarga nada.
	if d.tab == 4 {
		return
	}
	if d.tab != 1 && d.tab != 2 { // 1 = Ruta, 2 = Alt.
		return
	}
	// El arte de ítems se descarga en un Cmd asíncrono (itemArtCmdFor):
	// descargarlo aquí bloqueaba el bucle de Bubble Tea, igual que pasaba
	// con las runas antes de la fase 6.
}

// itemArtCmdFor devuelve un comando que descarga y renderiza el arte de
// los ítems que la pestaña Ruta o Alt. va a mostrar.
//
// Se ejecuta FUERA del bucle de Bubble Tea: con el CDN lento, abrir Ruta
// congelaba la interfaz.
func itemArtCmdFor(s *Session, ids []int, cols int) tea.Cmd {
	if s == nil || len(ids) == 0 || cols <= 0 {
		return nil
	}
	p := s.Provider
	if p == nil {
		return nil
	}
	type job struct {
		key artKey
		id  int
	}
	var jobs []job
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		key := artKey{Kind: artItem, ID: id, Cols: cols, Mode: artHalfBlock, Patch: s.Version}
		if s.hasArt(key) {
			continue
		}
		if !s.reserveArt(key) {
			continue
		}
		jobs = append(jobs, job{key: key, id: id})
	}
	if len(jobs) == 0 {
		return nil
	}
	return func() tea.Msg {
		out := make(map[artKey]string, len(jobs))
		defer func() {
			for _, j := range jobs {
				s.releaseArt(j.key)
			}
		}()
		for _, j := range jobs {
			raw, err := p.ItemIcon(j.id)
			if err != nil || len(raw) == 0 {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			out[j.key] = imgrender.RenderAspect(img, cols, panelBg)
		}
		return artLoadedMsg{Art: out}
	}
}

// tabItemIDs devuelve los ids de ítem que muestra una pestaña de ruta o
// alternativas, en orden y sin repetir.
func tabItemIDs(tab int, plan *lolbuild.Plan) []int {
	if plan == nil {
		return nil
	}
	seen := map[int]bool{}
	var out []int
	add := func(id int) {
		if id <= 0 || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	if tab == 1 {
		for _, st := range plan.Steps {
			// Los objetos de misión no son compras: sin icono.
			if !st.Quest {
				add(st.Item)
			}
		}
		return out
	}
	for _, g := range plan.Alternatives {
		for _, a := range g.Items {
			add(a.Item)
		}
	}
	return out
}

// tabCardList devuelve las tarjetas ENFOCABLES de las pestañas Ruta (1)
// y Alt. (2), en el mismo orden en que se dibujan.
//
// No es una lista nueva de datos: envuelve los ítems que el plan ya
// calculó para que la selección y el popup de Ítems funcionen igual.
// No altera el orden, las estadísticas ni las categorías.
func (d detailView) tabCardList() []cardItem {
	plan := d.lolPlan()
	if plan == nil {
		return nil
	}
	var out []cardItem
	if d.tab == 1 {
		// Ruta: un elemento por paso mostrado (incluidas las misiones,
		// para que la selección no salte líneas), y las botas al final.
		for _, st := range plan.Steps {
			out = append(out, cardItem{
				item: model.RankedItem{
					ItemStats: model.ItemStats{ID: st.Item, Stats: st.Stats},
					WinRate:   model.Stats.WinRate(st.Stats),
				},
				kind:  kindBuy,
				order: st.Slot,
				step:  st.Slot,
			})
		}
		if len(plan.Boots) > 0 {
			b := plan.Boots[0]
			out = append(out, cardItem{
				item: model.RankedItem{
					ItemStats: model.ItemStats{ID: b.Item, Stats: b.Stats},
					WinRate:   model.Stats.WinRate(b.Stats),
				},
				boot:  true,
				order: b.Slot,
				step:  b.Slot,
			})
		}
		return out
	}
	// Alt.: en el mismo orden en que alternativesViewB las pinta, con
	// un elemento por candidato mostrado (no por categoría) para que
	// flechas y clic recorran lo que se ve.
	for _, g := range plan.Alternatives {
		for _, a := range g.Items {
			out = append(out, cardItem{
				item: model.RankedItem{
					ItemStats: model.ItemStats{ID: a.Item, Stats: a.Stats},
					WinRate:   a.WinRate,
					PickRate:  a.PickRate,
				},
			})
		}
	}
	return out
}

// altZoneIdx devuelve el índice de tabCardList correspondiente a un
// ítem de las alternativas. Los candidatos se recorren en el mismo
// orden en ambas funciones, así que basta contar hasta encontrarlo.
func altZoneIdx(plan *lolbuild.Plan, item int) int {
	i := 0
	for _, g := range plan.Alternatives {
		for _, a := range g.Items {
			if a.Item == item {
				return i
			}
			i++
		}
	}
	return 0
}

// runeArtCols es el ancho del icono de runa. Tres columnas dan una fila,
// igual que en Ruta y Alt.
const runeArtCols = 3

// ensureRuneArt prepara el arte de la página de runas que se va a
// mostrar: keystone, primarias y secundarias.
//
// Bajo demanda y bajo tolerancia: si no hay proveedor de iconos de runa,
// no hay recurso o el PNG está corrupto, la vista sigue mostrando los
// nombres. Nunca se llama desde View().
func (d *detailView) ensureRuneArt() {
	// Ya NO descarga: el arte se pide con runeArtCmdFor desde Update, en
	// un Cmd asíncrono. Se conserva por compatibilidad de las pruebas.
	if d.session == nil || d.session.Game != model.GameLoL {
		return
	}
}

// runePageIDs devuelve los ids de runa de la página activa, en el orden
// visual: keystone, primarias, secundarias.
func runePageIDs(cb *model.CleanBuild) []int {
	if cb == nil || len(cb.RunePages) == 0 {
		return nil
	}
	p := cb.RunePages[0]
	seen := map[int]bool{}
	var out []int
	add := func(id int) {
		if id <= 0 || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
	}
	add(p.KeystoneID)
	for _, id := range p.PrimaryIDs {
		add(id)
	}
	for _, id := range p.SecondaryIDs {
		add(id)
	}
	return out
}

// runeArtCmdFor devuelve un comando que descarga y renderiza el arte de
// la página de runas ACTIVA fuera del bucle de Bubble Tea.
//
// Antes se hacía dentro de Update(): con el CDN lento, pulsar "5"
// congelaba la interfaz hasta que respondiera la red. Ahora Update solo
// lanza el comando y sigue atendiendo teclas.
//
// Devuelve nil si no hay nada que cargar (sin proveedor, arte ya
// presente o descarga ya en curso para la misma clave).
func runeArtCmdFor(s *Session, ids []int) tea.Cmd {
	if s == nil || len(ids) == 0 {
		return nil
	}
	rp, ok := s.Provider.(staticdata.RuneIconProvider)
	if !ok {
		return nil
	}
	// Se reservan solo las claves que faltan y no están en curso.
	type job struct {
		key artKey
		id  int
	}
	var jobs []job
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		key := artKey{Kind: artRune, ID: id, Cols: runeArtCols, Mode: artHalfBlock, Patch: s.Version}
		if s.hasArt(key) {
			continue
		}
		if !s.reserveArt(key) {
			continue // ya hay una descarga en curso para esta clave
		}
		jobs = append(jobs, job{key: key, id: id})
	}
	if len(jobs) == 0 {
		return nil
	}
	// Copia de los valores: el Cmd corre en otra goroutine.
	patch := s.Version
	_ = patch
	return func() tea.Msg {
		out := make(map[artKey]string, len(jobs))
		defer func() {
			for _, j := range jobs {
				s.releaseArt(j.key)
			}
		}()
		for _, j := range jobs {
			raw, err := rp.RuneIcon(j.id)
			if err != nil || len(raw) == 0 {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			out[j.key] = imgrender.RenderAspect(img, runeArtCols, panelBg)
		}
		return artLoadedMsg{Art: out}
	}
}

// artLoadedMsg entrega arte descargado al bucle. Sirve tanto para ítems
// como para runas: la clave incluye qué es.
type artLoadedMsg struct {
	Art map[artKey]string
}

// artCmd lanza la carga asíncrona del arte de la pestaña actual.
//
// Cubre las tres pestañas con iconos: Ruta (1), Alt. (2) y Runas (4).
// Devuelve nil en el resto.
func (d detailView) artCmd() tea.Cmd {
	if d.session == nil || d.session.Game != model.GameLoL {
		return nil
	}
	switch d.tab {
	case 1, 2:
		return itemArtCmdFor(d.session, tabItemIDs(d.tab, d.lolPlan()), smallArtCols)
	case 4:
		return runeArtCmdFor(d.session, runePageIDs(d.CurrentBuild()))
	}
	return nil
}
