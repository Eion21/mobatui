package ui

// Integración del planificador de LoL (internal/lolbuild) con la TUI.
//
// El plan NO se recalcula en cada render de Bubble Tea: se calcula una
// vez por (campeón, rol, parche y estado de misión) y se guarda en la
// sesión. Así el scroll y la navegación no pagan el coste y la caché de
// Data Dragon se reutiliza sin peticiones nuevas.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

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
	for _, st := range plan.Steps {
		out = append(out, d.routeStepLine(st))
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
		line += stDim.Render(fmt.Sprintf(" · %s partidas · %s",
			nThousand(b.Stats.Games), pct(model.Stats.WinRate(b.Stats))))
		out = append(out, line)
	} else {
		out = append(out, stDim.Render("  botas: sin recomendación con muestra suficiente"))
	}

	// Inventario final y estado de misión.
	out = append(out, d.finalInventoryLines(plan)...)
	return out
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
	line := fmt.Sprintf("  %s %s", stDim.Render(pos), stKeyName.Render(st.Name))
	if st.BuyMinute > 0 {
		line += stDim.Render(fmt.Sprintf(" · ~%d min", st.BuyMinute))
	}
	line += stDim.Render(fmt.Sprintf(" · %s partidas · %s",
		nThousand(st.Stats.Games), pct(model.Stats.WinRate(st.Stats))))
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
	} else {
		out = append(out, stWinrateBad.Render("  inventario incompleto: "+plan.Reason))
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
			out = append(out, d.alternativeLine(a))
		}
	}
	return out
}

// alternativeLine formatea una alternativa con sus datos reales.
func (d detailView) alternativeLine(a lolbuild.Alternative) string {
	line := fmt.Sprintf("  %s", stKeyName.Render(a.Name))
	line += stDim.Render(fmt.Sprintf(" · %s partidas", nThousand(a.Stats.Games)))
	line += stDim.Render(fmt.Sprintf(" · pick %.0f%%", a.PickRate))
	if a.WinRate > 0 {
		line += stWinrateGood.Render(fmt.Sprintf(" · WR %s", pct(a.WinRate)))
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
