// Package lolbuild planifica rutas de compra de League of Legends a
// partir de las tablas de Lolalytics y del catálogo de Data Dragon.
//
// Principio rector: solo se afirma lo que los datos demuestran.
//
//   - Lolalytics publica item1..item5 como la compra número 1, 2, … de
//     la partida (el minuto medio crece de forma monótona: 11, 17, 22,
//     27, 31 en el caso verificado). Eso permite construir una ruta
//     ORDENADA por posición de compra.
//   - builtBootSet3 publica tríos con su WR y partidas conjuntas, pero su
//     SEMÁNTICA NO ESTÁ DEMOSTRADA: publica el mismo conjunto de ids en
//     varias permutaciones con estadísticas distintas, lo que es
//     compatible tanto con "combinación sin orden" como con un orden que
//     la fuente no documenta. Mientras no se pueda demostrar cuál es, se
//     mantiene FUERA del planificador: no se usa para construir pasos ni
//     para inferir secuencias.
//   - itemBootSetN contiene pares de contadores internos de la web cuyo
//     significado no está documentado ni cuadra con las partidas de
//     builtBootSet3. NO se usa: inventar su semántica sería peor que no
//     tenerla.
package lolbuild

import (
	"fmt"
	"sort"

	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
)

// EvidenceLevel declara de dónde sale cada afirmación del plan.
type EvidenceLevel string

const (
	// EvidenceObserved: la fuente publica la posición de compra.
	EvidenceObserved EvidenceLevel = "observado"
	// EvidenceComposed: la ruta se compone de posiciones agregadas
	// (cada paso es la compra N más frecuente, no una partida real).
	EvidenceComposed EvidenceLevel = "compuesto"
	// EvidenceCombination: combinación frecuente, sin orden demostrado.
	EvidenceCombination EvidenceLevel = "combinación"
	// EvidenceAlternative: candidato compatible que no ocupa espacio.
	EvidenceAlternative EvidenceLevel = "alternativa"
	// EvidenceFallback: datos insuficientes para reconstruir la ruta.
	EvidenceFallback EvidenceLevel = "respaldo"
)

// MinGames es el mínimo de partidas para aceptar un paso como evidencia.
// Se deja como constante propia en vez de tocar los umbrales de clean:
// este paquete no rankea, solo describe.
const MinGames = 100

// Step es un paso de compra del plan.
type Step struct {
	Item  int
	Name  string
	Stats model.Stats
	// BuyMinute es el minuto medio de compra que publica la fuente.
	BuyMinute int
	// Slot es la posición de compra (1..5) según la fuente.
	Slot int
	// Evidence dice cómo se sostiene este paso.
	Evidence EvidenceLevel
	// Boots marca que el paso es un par de botas.
	Boots bool
	// Quest marca que el paso es el objeto de misión de SOPORTE: se
	// elige al empezar la partida y se mejora con puntos de misión. NO
	// es una compra cronológica, así que no lleva minuto ni se presenta
	// como tal.
	Quest bool
	// QuestOccupies indica que el objeto, aun siendo de misión, ocupa un
	// espacio del inventario (el de soporte sí; las botas de BOT no).
	QuestOccupies bool
}

// InventoryState es el estado de la misión de rol de temporada.
type InventoryState string

const (
	// MissionUnknown: no hay dato fiable. Nunca se asume completada.
	MissionUnknown InventoryState = "desconocida"
	// MissionPending: aún con inventario normal (botas en un espacio).
	MissionPending InventoryState = "pendiente"
	// MissionComplete: botas en ranura especial, 6 espacios principales.
	MissionComplete InventoryState = "completada"
)

// Inventory describe la restricción de espacios aplicable.
type Inventory struct {
	// MainSlots es el número de espacios principales disponibles.
	MainSlots int
	// BootsSlot indica que las botas ocupan un espacio principal
	// (false solo cuando la misión de BOT las mueve a su ranura).
	BootsSlot bool
	// QuestSlot indica que este rol tiene ranura de misión propia.
	// OJO: no siempre es para botas. En SUPPORT es para guardianes de
	// control; en BOT es para las botas. En el resto no existe.
	QuestSlot bool
	// QuestSlotUse describe para qué sirve la ranura de misión.
	QuestSlotUse string
	// State es el estado de la misión.
	State InventoryState
	// Note explica la restricción en lenguaje llano.
	Note string
}

// TotalSlots devuelve cuántos objetos de build caben: los espacios
// principales más el de botas cuando las botas no van a la ranura de
// misión. Las ranuras especiales (botas de BOT, guardianes de SUPPORT)
// son aparte y no cuentan aquí.
func (inv Inventory) TotalSlots() int {
	n := inv.MainSlots
	if inv.BootsSlot {
		n++
	}
	return n
}

// QuestSlots devuelve cuántas ranuras especiales de misión hay.
func (inv Inventory) QuestSlots() int {
	if inv.QuestSlot {
		return 1
	}
	return 0
}

// Plan es la ruta de compra completa de un campeón en un rol.
type Plan struct {
	ChampionID   int
	ChampionKey  string
	ChampionName string
	Role         model.Role
	Patch        string
	Source       string

	// Steps son los pasos de compra ordenados.
	Steps []Step
	// Core son los objetos CORE ya rankeados por clean (sin cambios).
	Core []model.RankedItem
	// Boots son las botas recomendadas.
	Boots []Step
	// Alternatives son candidatos compatibles AGRUPADOS por función.
	// NO están equipados: no ocupan espacio.
	Alternatives []CategoryGroup
	// Final es el inventario final propuesto (incluye las botas).
	Final []int
	// FinalMain son los objetos que ocupan espacio principal.
	FinalMain []int
	// Inventory describe la restricción aplicada.
	Inventory Inventory

	// Complete indica que la ruta cubre el inventario esperado.
	Complete bool
	// Reason explica por qué el plan está incompleto ("" si completo).
	Reason string
	// Total son las partidas de la muestra.
	Total model.Stats
	// WinRate es el WR del campeón/rol en el parche.
	WinRate float64
	// Evidence resume el nivel de evidencia dominante del plan.
	Evidence EvidenceLevel
	// MissingIDs cuenta los objetos DISTINTOS con muestra suficiente
	// cuyo id no está en el catálogo. Es distinto de un parche
	// incompatible: aquí la fuente sí vio comprar el objeto, pero el
	// catálogo no lo conoce.
	MissingIDs int
	// PatchMismatch indica que NINGÚN id de la fuente está en el
	// catálogo: huele a parche incompatible, no a un objeto aislado
	// ausente.
	PatchMismatch bool
}

// CategoryGroup agrupa alternativas por función. Un mismo objeto puede
// aparecer en varios grupos: sirve para varias cosas a la vez, y
// mostrarlo en cada grupo es más útil que elegir una etiqueta única.
type CategoryGroup struct {
	Category lolitems.Category
	Items    []Alternative
}

// Alternative es un candidato agrupado por función.
type Alternative struct {
	Item       int
	Name       string
	Categories []lolitems.Category
	Stats      model.Stats
	WinRate    float64
	// PickRate en % de las partidas del campeón/rol.
	PickRate float64
	// Evidence siempre EvidenceAlternative: es una opción, no un paso.
	Evidence EvidenceLevel
}

// Options ajusta la planificación.
type Options struct {
	// MinGames es el mínimo de partidas por paso. 0 = MinGames.
	MinGames int
	// MaxSteps limita los pasos de compra. 0 = 5 (item1..item5).
	MaxSteps int
	// MaxAlternatives es el máximo POR CATEGORÍA. 0 = 3.
	MaxAlternatives int
	// MaxAlternativesTotal acota el total de candidatos distintos (un
	// objeto puede estar en varias categorías). 0 = 12.
	MaxAlternativesTotal int
	// Mission permite forzar el estado de la misión. "" = desconocida.
	Mission InventoryState
}

func (o *Options) fill() {
	if o.MinGames == 0 {
		o.MinGames = MinGames
	}
	if o.MaxSteps == 0 {
		o.MaxSteps = 5
	}
	if o.MaxAlternatives == 0 {
		o.MaxAlternatives = 3
	}
	if o.MaxAlternativesTotal == 0 {
		o.MaxAlternativesTotal = 12
	}
	if o.Mission == "" {
		o.Mission = MissionUnknown
	}
}

// BuildPlan construye el plan a partir de una build ya limpia (con
// ItemSlots, ItemCombos y Boots) y del catálogo de objetos.
//
// No modifica ni reordena Core, y no recalcula Wilson.
func BuildPlan(cb *model.CleanBuild, catalog map[int]lolitems.Item, opts Options) Plan {
	opts.fill()
	p := Plan{
		ChampionID:   cb.ChampionID,
		ChampionKey:  cb.ChampionKey,
		ChampionName: cb.ChampionName,
		Role:         cb.Role,
		Patch:        cb.Patch,
		Source:       cb.Source,
		Core:         cb.CoreItems,
		Total:        cb.Total,
		WinRate:      cb.WinRate,
		Inventory:    inventoryFor(cb.Role, opts.Mission),
		Evidence:     EvidenceComposed,
	}

	// Sin tablas por posición no hay ruta reconstruible: se declara.
	if len(cb.ItemSlots) == 0 {
		p.Complete = false
		p.Evidence = EvidenceFallback
		p.Reason = "la fuente no publica tablas por posición (item1..item5): " +
			"no se puede reconstruir la progresión de compra"
		p.Boots = bootsFrom(cb, catalog, opts)
		return p
	}

	// Pasos: para cada posición, la compra más frecuente con muestra
	// suficiente. Esto es una ruta COMPUESTA: cada paso es el objeto más
	// comprado como compra N, no la secuencia de una partida concreta.
	seen := map[int]bool{}
	missing := map[int]bool{}
	for slot := 0; slot < len(cb.ItemSlots) && slot < opts.MaxSteps; slot++ {
		step, ok := pickSlot(cb.ItemSlots[slot], slot+1, catalog, opts.MinGames, seen, missing)
		if !ok {
			continue
		}
		seen[step.Item] = true
		p.Steps = append(p.Steps, step)
	}

	// Parche incompatible: la fuente aporta objetos con muestra pero el
	// catálogo no conoce NINGUNO. Es un problema distinto a que falte un
	// id suelto.
	p.MissingIDs = len(missing)
	p.PatchMismatch = len(p.Steps) == 0 && p.MissingIDs > 0

	p.Boots = bootsFrom(cb, catalog, opts)
	p.Alternatives = alternativesFrom(cb, catalog, opts, seen)
	p.Final, p.FinalMain = finalInventory(p, catalog)
	p.Complete, p.Reason = completeness(p)

	return p
}

// pickSlot elige la compra más frecuente de una tabla de posición.
// Empates: menor id, para que el resultado sea determinista.
func pickSlot(rows []model.ItemStats, slot int, catalog map[int]lolitems.Item, minGames int, seen map[int]bool, missing map[int]bool) (Step, bool) {
	type cand struct {
		it   model.ItemStats
		meta lolitems.Item
	}
	var cands []cand
	for _, r := range rows {
		if r.Games < minGames {
			continue
		}
		if seen[r.ID] {
			continue
		}
		meta, ok := catalog[r.ID]
		if !ok {
			// La fuente vio comprar este id pero el catálogo no lo
			// tiene: puede ser un objeto retirado o un catálogo de otro
			// parche. Se cuenta aparte; no es lo mismo que un parche
			// incompatible (donde no cuadra ninguno).
			missing[r.ID] = true
			continue
		}
		// Los objetos de misión de soporte se eligen al empezar: son un
		// paso real pero no ocupan espacio principal. Se incluyen
		// marcados para no dejar un hueco artificial en la ruta.
		quest := lolitems.IsQuestItem(r.ID)
		if !quest && !lolitems.Eligible(meta) {
			// Las botas son válidas como paso aunque su ranura sea
			// aparte; el resto de no elegibles (componentes,
			// consumibles) no son pasos de build final.
			if !lolitems.IsBoots(meta) {
				continue
			}
		}
		cands = append(cands, cand{r, meta})
	}
	if len(cands) == 0 {
		return Step{}, false
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].it.Games != cands[j].it.Games {
			return cands[i].it.Games > cands[j].it.Games
		}
		return cands[i].it.ID < cands[j].it.ID
	})
	best := cands[0]
	quest := lolitems.IsQuestItem(best.it.ID)
	step := Step{
		Item:     best.it.ID,
		Name:     best.meta.Name,
		Stats:    best.it.Stats,
		Slot:     slot,
		Evidence: EvidenceComposed,
		Boots:    lolitems.IsBoots(best.meta),
		Quest:    quest,
	}
	if quest {
		// El objeto de misión de soporte no se compra en un minuto
		// concreto: se obtiene al empezar y se mejora con puntos. No se
		// inventa un tiempo de compra ni se presenta como cronológico.
		step.BuyMinute = 0
		step.Evidence = EvidenceComposed
		step.QuestOccupies = true
	} else {
		step.BuyMinute = int(best.it.AvgBuyS / 60)
	}
	return step, true
}

// bootsFrom elige las botas más usadas con muestra suficiente.
func bootsFrom(cb *model.CleanBuild, catalog map[int]lolitems.Item, opts Options) []Step {
	var out []Step
	for _, b := range cb.Boots {
		if b.Games < opts.MinGames {
			continue
		}
		meta, ok := catalog[b.ID]
		if !ok {
			continue
		}
		out = append(out, Step{
			Item:      b.ID,
			Name:      meta.Name,
			Stats:     b.Stats,
			BuyMinute: int(b.AvgBuyS / 60),
			Evidence:  EvidenceComposed,
			Boots:     true,
		})
		if len(out) >= 2 {
			break
		}
	}
	return out
}

// alternativesFrom agrupa candidatos por función, cruzando el catálogo
// con la evidencia de uso. Solo aparecen objetos que la fuente vio
// comprar y que son elegibles: pertenecer a una categoría no basta.
func alternativesFrom(cb *model.CleanBuild, catalog map[int]lolitems.Item, opts Options, used map[int]bool) []CategoryGroup {
	// Índice de uso: partidas por objeto agregadas de las tablas por
	// posición (la única evidencia de uso que llega a CleanBuild) más
	// los CORE ya rankeados. Un objeto que la fuente no vio comprar no
	// aparece como alternativa.
	usage := map[int]model.ItemStats{}
	accumulate := func(r model.ItemStats) {
		prev, ok := usage[r.ID]
		if !ok {
			usage[r.ID] = r
			return
		}
		prev.Games += r.Games
		prev.Wins += r.Wins
		usage[r.ID] = prev
	}
	for _, slot := range cb.ItemSlots {
		for _, r := range slot {
			accumulate(r)
		}
	}
	for _, r := range cb.CoreItems {
		accumulate(r.ItemStats)
	}

	type acc struct {
		cats []lolitems.Category
		alts []Alternative
	}
	byCat := map[lolitems.Category]*acc{}

	for id, meta := range catalog {
		u, ok := usage[id]
		if !ok || u.Games < opts.MinGames {
			continue // sin evidencia de uso: no se propone
		}
		if used[id] || !lolitems.Eligible(meta) || lolitems.IsQuestItem(id) {
			continue
		}
		cl := lolitems.Classify(meta)
		wr := 0.0
		if u.Games > 0 {
			wr = float64(u.Wins) / float64(u.Games)
		}
		alt := Alternative{
			Item:       id,
			Name:       meta.Name,
			Categories: cl.Categories(),
			Stats:      u.Stats,
			WinRate:    wr,
			PickRate:   pickRate(u.Games, cb.Total.Games),
			Evidence:   EvidenceAlternative,
		}
		for _, cat := range cl.Categories() {
			a := byCat[cat]
			if a == nil {
				a = &acc{}
				byCat[cat] = a
			}
			a.alts = append(a.alts, alt)
		}
	}

	// Orden determinista: por categoría canónica, y dentro de cada una
	// por partidas (más usada primero) y luego id. El tope se aplica por
	// grupo.
	var out []CategoryGroup
	for _, cat := range lolitems.AllCategories {
		a := byCat[cat]
		if a == nil {
			continue
		}
		sort.SliceStable(a.alts, func(i, j int) bool {
			if a.alts[i].Stats.Games != a.alts[j].Stats.Games {
				return a.alts[i].Stats.Games > a.alts[j].Stats.Games
			}
			return a.alts[i].Item < a.alts[j].Item
		})
		n := opts.MaxAlternatives
		if len(a.alts) < n {
			n = len(a.alts)
		}
		out = append(out, CategoryGroup{Category: cat, Items: a.alts[:n]})
	}

	// Tope TOTAL de candidatos distintos. Un objeto puede aparecer en
	// varias categorías, así que el total se cuenta por id.
	//
	// El reparto es POR RONDAS, no por orden: en cada ronda se toma un
	// candidato de cada categoría antes de tomar el segundo de
	// cualquiera. Con un corte codicioso en orden canónico, las
	// categorías que aparecen al final (resistencia mágica, apoyo…) se
	// quedaban fuera SIEMPRE, que es un sesgo reproducible: un ADC
	// nunca veía opciones defensivas.
	//
	// Empates: se respeta el orden ya calculado dentro de cada grupo
	// (partidas desc, id asc), que es determinista.
	if opts.MaxAlternativesTotal > 0 {
		seen := map[int]bool{}
		total := 0
		// picked[i] = cuántos lleva el grupo i en esta ronda.
		picked := make([]int, len(out))
		kept := make([][]Alternative, len(out))

		for round := 0; total < opts.MaxAlternativesTotal; round++ {
			progressed := false
			for i := range out {
				if total >= opts.MaxAlternativesTotal {
					break
				}
				// Buscar el siguiente candidato no emitido de este grupo.
				for picked[i] < len(out[i].Items) {
					a := out[i].Items[picked[i]]
					picked[i]++
					if seen[a.Item] {
						continue
					}
					seen[a.Item] = true
					total++
					kept[i] = append(kept[i], a)
					progressed = true
					break
				}
			}
			if !progressed {
				break // ningún grupo aporta más candidatos nuevos
			}
		}

		trimmed := out[:0]
		for i := range out {
			if len(kept[i]) > 0 {
				trimmed = append(trimmed, CategoryGroup{Category: out[i].Category, Items: kept[i]})
			}
		}
		out = trimmed
	}
	return out
}

// pickRate devuelve el % de partidas de la muestra en que se compró.
func pickRate(games, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(games) / float64(total) * 100
}

// finalInventory arma el inventario final respetando los espacios.
//
// Reglas:
//   - Un objeto de misión de SOPORTE ocupa un espacio principal (no va a
//     la ranura de control: esa es solo para guardianes).
//   - Las botas ocupan un espacio principal salvo en BOT con la misión
//     completada, donde pasan a la ranura de misión.
//   - Un solo par de botas.
//   - Ni duplicados ni objetos no elegibles.
func finalInventory(p Plan, catalog map[int]lolitems.Item) (all, main []int) {
	var out []int
	seen := map[int]bool{}
	mainUsed := 0

	// add coloca un objeto en un espacio principal si cabe.
	// withBoots=true permite ocupar el espacio extra de botas: los
	// objetos de build se limitan a MainSlots y las botas son el +1.
	add := func(id int, isBoots bool) bool {
		if seen[id] {
			return false
		}
		if meta, ok := catalog[id]; ok && !lolitems.Eligible(meta) && !lolitems.IsQuestItem(id) && !isBoots {
			return false
		}
		// Las botas ocupan su espacio propio (+1); el resto, los
		// principales.
		if isBoots {
			if !p.Inventory.BootsSlot {
				// BOT con misión: van a la ranura de misión, fuera del
				// inventario principal.
				seen[id] = true
				out = append(out, id)
				return true
			}
			if mainUsed >= p.Inventory.TotalSlots() {
				return false
			}
			seen[id] = true
			out = append(out, id)
			return true
		}
		if mainUsed >= p.Inventory.MainSlots {
			return false
		}
		seen[id] = true
		mainUsed++
		out = append(out, id)
		main = append(main, id)
		return true
	}
	addMain := func(id int) bool { return add(id, false) }

	// 1. Objetos de misión de soporte: ocupan espacio principal.
	for _, s := range p.Steps {
		if s.Quest && s.QuestOccupies {
			addMain(s.Item)
		}
	}

	// 2. Pasos de build, saltando los de misión (ya tratados).
	for _, s := range p.Steps {
		if s.Quest {
			continue
		}
		addMain(s.Item)
	}

	// 3. Botas: una sola vez.
	var bootsID int
	for _, b := range p.Boots {
		if !seen[b.Item] {
			bootsID = b.Item
			break
		}
	}
	if bootsID != 0 {
		add(bootsID, true)
	}
	return out, main
}

// completeness decide si el plan cubre el inventario esperado.
func completeness(p Plan) (bool, string) {
	if p.PatchMismatch {
		return false, fmt.Sprintf(
			"parche incompatible: ninguno de los %d objetos con muestra de la fuente "+
				"existe en el catálogo de Data Dragon (%s)",
			p.MissingIDs, p.Patch)
	}
	if len(p.Steps) == 0 {
		if p.MissingIDs > 0 {
			return false, fmt.Sprintf(
				"%d objetos con muestra no están en el catálogo del parche: "+
					"no se puede construir la ruta sin nombres ni categorías",
				p.MissingIDs)
		}
		return false, "sin pasos de compra con muestra suficiente"
	}
	// Los espacios principales se comparan aparte de las botas: con la
	// misión de BOT completada hay 6 principales y las botas NO cuentan.
	need := p.Inventory.MainSlots
	have := len(p.FinalMain)
	if have >= need {
		return true, ""
	}
	faltan := need - have
	// Concordancia: "falta 1 objeto" / "faltan N objetos".
	verbo, plural := "faltan", "objetos"
	if faltan == 1 {
		verbo, plural = "falta", "objeto"
	}
	espacios := "espacios"
	if need == 1 {
		espacios = "espacio"
	}
	return false, fmt.Sprintf(
		"%s %d %s para completar %d %s principales: "+
			"la fuente no ofrece candidatos con muestra suficiente; no se rellena el hueco",
		verbo, faltan, plural, need, espacios)
}

// inventoryFor devuelve la restricción de espacios según el rol y el
// estado de la misión.
//
// Regla verificada: el inventario normal de LoL tiene 6 espacios y las
// botas ocupan uno. La misión de rol de temporada mueve las botas a una
// ranura especial y libera el espacio, pero SOLO en los roles que la
// tienen (bot lane); en el resto no se asume.
func inventoryFor(role model.Role, mission InventoryState) Inventory {
	switch role {
	case model.RoleBottom:
		// BOT: la misión mueve las BOTAS a la ranura especial y libera
		// el espacio. Antes de completarla, las botas ocupan inventario.
		if mission == MissionComplete {
			return Inventory{
				MainSlots: 6, BootsSlot: false,
				QuestSlot: true, QuestSlotUse: "botas",
				State: MissionComplete,
				Note:  "misión de bot completada: botas en la ranura de misión, 6 espacios principales",
			}
		}
		return Inventory{
			MainSlots: 5, BootsSlot: true,
			QuestSlot: false, State: mission,
			Note: "inventario normal: 6 espacios con las botas ocupando uno",
		}

	case model.RoleSupport:
		// SUPPORT: su ranura de misión es para guardianes de control, NO
		// para botas. El objeto de soporte y las botas ocupan espacios
		// principales igual que cualquier otro objeto.
		return Inventory{
			MainSlots: 5, BootsSlot: true,
			QuestSlot: true, QuestSlotUse: "guardianes de control",
			State: mission,
			Note: "inventario normal con ranura de misión aparte para guardianes de control: " +
				"el objeto de soporte y las botas ocupan espacios principales",
		}

	default:
		// TOP, MID, JUNGLE: sin ranura adicional. MID recibe la mejora
		// de botas Tier 3, pero es una mejora del objeto, no un espacio.
		return Inventory{
			MainSlots: 5, BootsSlot: true,
			QuestSlot: false, State: mission,
			Note: "inventario normal: 6 espacios con las botas ocupando uno",
		}
	}
}

// AlternativeItems devuelve las alternativas sin agrupar, una sola vez
// por objeto y en orden determinista (recorriendo los grupos).
func (p Plan) AlternativeItems() []Alternative {
	var out []Alternative
	seen := map[int]bool{}
	for _, g := range p.Alternatives {
		for _, a := range g.Items {
			if seen[a.Item] {
				continue
			}
			seen[a.Item] = true
			out = append(out, a)
		}
	}
	return out
}

// GroupFor devuelve el grupo de una categoría, si existe.
func (p Plan) GroupFor(cat lolitems.Category) (CategoryGroup, bool) {
	for _, g := range p.Alternatives {
		if g.Category == cat {
			return g, true
		}
	}
	return CategoryGroup{}, false
}
