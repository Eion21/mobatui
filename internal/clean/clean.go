// Package clean implementa el pipeline que convierte los datos crudos
// normalizados (model.ChampionBuilds) en builds depuradas listas para
// la UI: filtra muestras irrelevantes, rankea por score de Wilson y
// extrae ítems, runas y orden de habilidades definitivos.
package clean

import (
	"sort"

	"github.com/Eion21/mobatui/internal/model"
)

// Options parametrizan el pipeline. Los ceros activan los valores por
// defecto.
type Options struct {
	MinGames      int     // partidas mínimas absolutas (def: 100)
	MinGamesSmall int     // suelo de partidas para el caso pickRate alto (def: 20)
	MinPickRate   float64 // pick rate mínimo del caso small (def: 0.05)
	MaxItems      int     // nº de ítems core a conservar (def: 6)
	MaxBoots      int     // nº de botas a conservar (def: 2)
	MaxRunePages  int     // nº de páginas de runas (def: 2)
}

func (o *Options) fill() {
	if o.MinGames == 0 {
		o.MinGames = 100
	}
	if o.MinGamesSmall == 0 {
		o.MinGamesSmall = 20
	}
	if o.MinPickRate == 0 {
		o.MinPickRate = 0.05
	}
	if o.MaxItems == 0 {
		o.MaxItems = 6
	}
	if o.MaxBoots == 0 {
		o.MaxBoots = 2
	}
	if o.MaxRunePages == 0 {
		o.MaxRunePages = 2
	}
}

// PassesFilter decide si una muestra es estadísticamente usable:
// muchas partidas en términos absolutos, o un pick rate alto con un
// suelo mínimo de partidas para pools pequeños.
func (o Options) PassesFilter(games, totalGames int) bool {
	if games >= o.MinGames {
		return true
	}
	if totalGames <= 0 {
		return false
	}
	pick := float64(games) / float64(totalGames)
	return games >= o.MinGamesSmall && pick >= o.MinPickRate
}

// Build ejecuta el pipeline completo sobre los datos crudos de un
// campeón/rol y devuelve la build depurada.
func Build(cb *model.ChampionBuilds, opts Options) *model.CleanBuild {
	opts.fill()
	clean := &model.CleanBuild{
		Game:         cb.Game,
		ChampionID:   cb.ChampionID,
		ChampionKey:  cb.ChampionKey,
		ChampionName: cb.ChampionName,
		Role:         cb.Role,
		Patch:        cb.Patch,
		Source:       cb.Source,
		Total:        cb.Total,
		WinRate:      cb.Total.WinRate(),
	}
	total := cb.Total.Games

	clean.CoreItems = coreItems(cb, total, opts)
	clean.Boots = rankItems(cb.Boots, total, opts)
	clean.Wards = rankItems(cb.Wards, total, opts)
	clean.RunePages = rankRunePages(cb.RunePages, total, opts)
	clean.SkillOrder = bestSkillOrder(cb.Skills, total, cb.SkillRulesFor(), opts)
	clean.SkillAlphabet = cb.SkillRulesFor().Alphabet
	clean.CanonicalSkillOrder = cb.SkillRulesFor().CanonicalOrder
	clean.AbilityNames = cb.AbilityNames
	clean.Matchups = cb.Matchups
	clean.Synergies = cb.Synergies
	clean.WRTrend = cb.WRTrend
	// Los slots y combos ya vienen ordenados por presencia desde la
	// fuente; se conservan los primeros combos con muestra razonable.
	clean.ItemSlots = cb.ItemSlots
	clean.ItemCombos = topCombos(cb.ItemCombos, 3)
	clean.Spells = cb.Spells
	clean.StartItems = cb.StartItems
	return clean
}

// topCombos conserva los primeros n combos tal como los ordenó la
// fuente (por presencia).
func topCombos(in []model.ItemCombo, n int) []model.ItemCombo {
	if len(in) > n {
		return in[:n]
	}
	return in
}

// BuildAll aplica Build a un lote completo.
func BuildAll(cbs []*model.ChampionBuilds, opts Options) []*model.CleanBuild {
	out := make([]*model.CleanBuild, 0, len(cbs))
	for _, cb := range cbs {
		out = append(out, Build(cb, opts))
	}
	return out
}

// pickRate es la fracción de partidas con el ítem/página sobre el
// total del campeón. Un total desconocido (0) da 0 y no +Inf: el
// valor se ordena y se pinta como un porcentaje.
func pickRate(games, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(games) / float64(total)
}

// rankItems filtra, puntúa y ordena ítems por Wilson descendente.
func rankItems(items []model.ItemStats, total int, opts Options) []model.RankedItem {
	var ranked []model.RankedItem
	for _, it := range items {
		if !opts.PassesFilter(it.Games, total) {
			continue
		}
		ranked = append(ranked, model.RankedItem{
			ItemStats: it,
			WinRate:   it.WinRate(),
			PickRate:  pickRate(it.Games, total),
			Wilson:    WilsonLower(it.Wins, it.Games),
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Wilson > ranked[j].Wilson })
	if len(ranked) > opts.MaxItems {
		ranked = ranked[:opts.MaxItems]
	}
	return ranked
}

// coreSize es el nº de ítems del core presentados como build fija.
const coreSize = 3

// coreItems rankea y elige el core. En LoL el orden de Wilson premia a
// los ítems que se compran en casi todas las partidas, así que el core
// son los más comprados, presentados en orden de compra. Deadlock
// conserva Wilson: allí el dato de compra ya viene limpio.
func coreItems(cb *model.ChampionBuilds, total int, opts Options) []model.RankedItem {
	wide := opts
	wide.MaxItems = 1 << 20 // rankear todo antes de recortar
	all := rankItems(cb.Items, total, wide)
	if cb.Game == model.GameLoL {
		all = coreByPick(all, coreSize)
		all = dropRarezas(all, coreSize)
	}
	if len(all) > opts.MaxItems {
		all = all[:opts.MaxItems]
	}
	return all
}

// coreByPick pone primero los n ítems más comprados, en orden de
// compra; el resto sigue por Wilson (situacionales).
func coreByPick(r []model.RankedItem, n int) []model.RankedItem {
	core := append([]model.RankedItem(nil), r...)
	sort.SliceStable(core, func(i, j int) bool { return core[i].PickRate > core[j].PickRate })
	if len(core) > n {
		core = core[:n]
	}
	sort.SliceStable(core, func(i, j int) bool { return core[i].AvgBuyS < core[j].AvgBuyS })
	in := make(map[int]bool, len(core))
	for _, it := range core {
		in[it.ID] = true
	}
	out := core
	for _, it := range r {
		if !in[it.ID] {
			out = append(out, it)
		}
	}
	return out
}

// minPickSituacional es el pick rate mínimo de un situacional: menos
// que esto es una rareza (se compra en menos de 1 de cada 100
// partidas) y confunde más de lo que recomienda.
const minPickSituacional = 0.01

// dropRarezas deja el core intacto y recorta del resto los ítems con
// pick rate ínfimo: con muestra absoluta grande pasan el filtro de
// partidas aunque casi nadie los compre.
func dropRarezas(r []model.RankedItem, core int) []model.RankedItem {
	head := r[:min(core, len(r))]
	out := append([]model.RankedItem(nil), head...)
	for _, it := range r[min(core, len(r)):] {
		if it.PickRate >= minPickSituacional {
			out = append(out, it)
		}
	}
	return out
}

// rankRunePages hace lo propio con páginas de runas completas.
func rankRunePages(pages []model.RunePageStats, total int, opts Options) []model.RankedRunePage {
	var ranked []model.RankedRunePage
	for _, p := range pages {
		if !opts.PassesFilter(p.Games, total) {
			continue
		}
		ranked = append(ranked, model.RankedRunePage{
			RunePageStats: p,
			WinRate:       p.WinRate(),
			PickRate:      pickRate(p.Games, total),
			Wilson:        WilsonLower(p.Wins, p.Games),
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Wilson > ranked[j].Wilson })
	if len(ranked) > opts.MaxRunePages {
		ranked = ranked[:opts.MaxRunePages]
	}
	return ranked
}

// bestSkillOrder elige la secuencia válida con más partidas (a igualdad,
// mayor Wilson) y deriva de ella la prioridad de maximización según las
// reglas del juego.
func bestSkillOrder(seqs []model.SkillSequenceStats, total int, rules model.SkillRules, opts Options) model.SkillOrder {
	var best *model.SkillSequenceStats
	for i := range seqs {
		s := &seqs[i]
		if !opts.PassesFilter(s.Games, total) {
			continue
		}
		if best == nil ||
			s.Games > best.Games ||
			(s.Games == best.Games && WilsonLower(s.Wins, s.Games) > WilsonLower(best.Wins, best.Games)) {
			best = s
		}
	}
	order := model.SkillOrder{}
	if best == nil {
		return order
	}

	// Prioridad publicada por la FUENTE (independiente de la secuencia).
	// Se usa como respaldo cuando la secuencia por nivel no es legal: es
	// un dato real de la fuente, no una invención, y no hay que
	// rechazarla por culpa de la otra representación.
	srcPrio := func() []rune {
		prio := make([]rune, 0, 4)
		for _, r := range best.Priority {
			prio = append(prio, r)
		}
		return prio
	}

	// La secuencia se propaga SOLO si es legal con las reglas del juego.
	// Antes se devolvía la secuencia cruda y la prioridad derivada; si
	// la secuencia era inválida, se descartaba todo (incluida la
	// prioridad válida). Ahora se conserva lo que sí es utilizable.
	prio, err := model.PriorityFromSequence(best.Sequence, rules)
	if err != nil {
		// Secuencia inválida: se descarta. La prioridad de la fuente, si
		// existe, sigue siendo información legítima.
		if p := srcPrio(); len(p) > 0 {
			order.Priority = p
			order.Stats = best.Stats
		}
		return order
	}
	order.Sequence = best.Sequence
	// La prioridad derivada de una secuencia legal es la más granular;
	// si no apareciera, se prefiere la de la fuente.
	if len(prio) > 0 {
		order.Priority = prio
	} else if p := srcPrio(); len(p) > 0 {
		order.Priority = p
	}
	order.Stats = best.Stats
	return order
}
