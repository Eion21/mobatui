// Constructor híbrido de builds: combina la evidencia estadística
// (Evaluate) con el motor de progresiones (progression) para producir
// una ruta VÁLIDA y completa, no solo una lista de objetos frecuentes.
//
// Diferencias con el generador anterior:
//   - No corta por un número fijo de tarjetas por fase: la ruta se
//     construye por objetos finales, así que una cadena de tres
//     componentes no consume tres huecos de presentación.
//   - Garantiza que cada mejora tenga su base antes en la ruta.
//   - Separa ruta estándar, ampliaciones y situacionales.
//   - Registra por qué cada objeto entró o quedó fuera.
//
// No sustituye al generador anterior: convive con él para comparar.
package deadlock

import (
	"fmt"
	"sort"

	"github.com/Eion21/mobatui/internal/progression"
)

// BuildOptions parametriza el constructor.
type BuildOptions struct {
	// Capacity es la capacidad del inventario (Deadlock: 12).
	Capacity int
	// MinConfidence es la confianza mínima para entrar en la ruta
	// estándar. Por debajo, el objeto solo puede ser situacional.
	MinConfidence float64
	// MinShare es el umbral de aparición heredado de la ruta observada.
	MinShare float64
	// MaxStandard es un tope opcional de entradas de la ruta estándar.
	// 0 = sin tope explícito: la restricción real la impone la
	// capacidad del inventario, que el simulador ya respeta.
	MaxStandard int
	// MaxExtensions es el tope de ampliaciones.
	MaxExtensions int
	// BeamWidth acota la exploración de secuencias.
	BeamWidth int
}

func (o *BuildOptions) fill() {
	if o.Capacity == 0 {
		o.Capacity = 12
	}
	if o.MinConfidence == 0 {
		o.MinConfidence = 0.45
	}
	if o.MinShare == 0 {
		o.MinShare = 0.30
	}
	// MaxStandard queda en 0 por defecto: el inventario es la
	// restricción real. Un tope artificial cortaba mejoras tardías
	// (Boundless Spirit) antes de que pudieran consumir su base.
	if o.MaxExtensions == 0 {
		o.MaxExtensions = 4
	}
	if o.BeamWidth == 0 {
		o.BeamWidth = 24
	}
}

// BuildEntry es un objeto de la build con su justificación.
type BuildEntry struct {
	Item       int
	Name       string
	Phase      progression.Phase
	Minute     int
	Origin     []progression.Origin
	Confidence float64
	Score      float64
	Share      float64
	// Reason explica en una línea por qué está aquí.
	Reason string
	// Chain es la cadena de componentes que hay que comprar para
	// llegar a este objeto, en orden (incluye el propio objeto).
	Chain []int
	// ConsumedBy es el objeto posterior que consume a este como
	// componente (0 = queda en el inventario final). Permite a la TUI
	// distinguir "compra que permanece" de "paso intermedio".
	ConsumedBy int
}

// Exclusion explica por qué un objeto no entró.
type Exclusion struct {
	Item   int
	Name   string
	Reason string
}

// HybridBuild es el resultado completo del constructor.
type HybridBuild struct {
	Standard     []BuildEntry
	Extensions   []BuildEntry
	Situationals []BuildEntry
	Excluded     []Exclusion
	Plan         progression.Plan
	Rejects      []progression.Rejection
	// Confidence es la confianza global de la build (la mínima de la
	// ruta estándar: la build no es más fiable que su eslabón débil).
	Confidence float64
}

// BuildHybrid construye la build a partir de la muestra y el catálogo.
// Es determinista: misma muestra + mismo catálogo = misma build.
func BuildHybrid(runs []Run, cat *progression.Catalog, opts BuildOptions) (*HybridBuild, error) {
	opts.fill()
	if cat == nil {
		return nil, fmt.Errorf("build: catálogo nil")
	}
	cands := Evaluate(runs, EvalOptions{MinShare: opts.MinShare})
	if len(cands) == 0 {
		return nil, fmt.Errorf("build: sin candidatos (muestra insuficiente)")
	}

	byItem := make(map[int]Candidate, len(cands))
	for _, c := range cands {
		byItem[c.Item] = c
	}

	out := &HybridBuild{}

	// 1. Elegir objetos finales de la ruta estándar. La ruta estándar
	//    es lo que compra la mayoría: exige el umbral de aparición
	//    heredado (MinShare, 30 %) Y confianza suficiente. Un objeto
	//    con 18 % de aparición es situacional por definición, no
	//    "ruta estándar" por tener buena confianza.
	var finals []Candidate
	for _, c := range cands {
		if !cat.Has(c.Item) {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item), "no está en el catálogo comprable"})
			continue
		}
		if c.Share < opts.MinShare {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				fmt.Sprintf("aparición %.0f%% por debajo del umbral %.0f%%",
					c.Share*100, opts.MinShare*100)})
			continue
		}
		if c.Confidence < opts.MinConfidence {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				fmt.Sprintf("confianza insuficiente (%.2f < %.2f)", c.Confidence, opts.MinConfidence)})
			continue
		}
		finals = append(finals, c)
	}
	if len(finals) == 0 {
		return nil, fmt.Errorf("build: ningún objeto alcanza umbral y confianza")
	}

	// 2. Preferir objetos finales sobre sus componentes: si un
	//    candidato es componente de otro candidato que también supera
	//    el umbral, el componente es un PASO de la cadena, no un
	//    destino.
	//
	//    Pero un básico que se compra en la inmensa mayoría de las
	//    partidas ES un destino por sí mismo (Extra Spirit, Extra
	//    Charge): no se degrada a ampliación solo porque exista su
	//    mejora. La condición es que la base no domine por aparición:
	//    si su share supera el de la mejora, sigue siendo una compra
	//    de ruta y la cadena se resuelve comprando ambas.
	finalSet := map[int]bool{}
	for _, c := range finals {
		finalSet[c.Item] = true
	}
	superseded := map[int]int{} // componente → objeto final que lo consume
	for _, c := range finals {
		for _, comp := range cat.Components(c.Item) {
			if !finalSet[comp] {
				continue
			}
			cc, ok := byItem[comp]
			if !ok {
				continue
			}
			// Si la base se compra más que su mejora, es un destino
			// propio: se conserva en la ruta y la mejora se compra
			// después (consumiéndola).
			if cc.Share > c.Share {
				continue
			}
			superseded[comp] = c.Item
		}
	}
	// Un componente puede ser "mejorado" por un objeto que a su vez
	// quedó superado: se resuelve transitivamente para no perder la
	// cadena completa (Extra Spirit → Improved Spirit → Boundless).
	for comp := range superseded {
		seen := map[int]bool{comp: true}
		cur := comp
		for {
			up, ok := superseded[cur]
			if !ok || seen[up] {
				break
			}
			seen[up] = true
			cur = up
		}
		superseded[comp] = cur
	}

	var chosen []Candidate
	for _, c := range finals {
		if up, ok := superseded[c.Item]; ok && up != c.Item {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				fmt.Sprintf("componente de %s, que puntúa igual o mejor", nameOf(cat, up))})
			continue
		}
		chosen = append(chosen, c)
	}

	// 3. Ordenar por minuto de compra: una build es una secuencia
	//    temporal. Ordenar por fase y luego por score metía objetos
	//    EARLY de cola larga por delante de básicos de MID.
	sort.SliceStable(chosen, func(i, j int) bool {
		if chosen[i].MedT != chosen[j].MedT {
			return chosen[i].MedT < chosen[j].MedT
		}
		if chosen[i].Score != chosen[j].Score {
			return chosen[i].Score > chosen[j].Score
		}
		return chosen[i].Item < chosen[j].Item
	})

	// 4. Tope opcional. Por defecto no hay: el inventario manda. Un
	//    corte previo por número de entradas no puede saber qué
	//    entradas son pasos intermedios (una mejora tardía consume su
	//    base y libera hueco), así que cortaba cadenas legítimas.
	if opts.MaxStandard > 0 && len(chosen) > opts.MaxStandard {
		for _, c := range chosen[opts.MaxStandard:] {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				fmt.Sprintf("fuera del tope de %d entradas", opts.MaxStandard)})
		}
		chosen = chosen[:opts.MaxStandard]
	}

	// 5. Construir la secuencia de compras con las cadenas completas.
	seq, entries, rejects := assembleSequence(chosen, cat, byItem, opts)
	out.Standard = entries
	out.Rejects = rejects

	// 6. Simular para obtener el inventario real.
	plan, simRejects := progression.Simulate(cat, opts.Capacity, seq)
	out.Plan = plan
	out.Rejects = append(out.Rejects, simRejects...)

	// 6b. Marcar qué entradas son pasos intermedios: si un objeto
	//     posterior de la ruta la consume como componente, no es un
	//     objeto final aunque aparezca en la secuencia.
	for i := range out.Standard {
		for _, st := range plan.Steps {
			for _, comp := range st.Consumed {
				if comp == out.Standard[i].Item {
					out.Standard[i].ConsumedBy = st.Item
				}
			}
		}
	}

	// 7. Ampliaciones: objetos válidos que no entraron en la ruta
	//    estándar (por tope o por umbral), para partidas largas.
	//
	//    Una ampliación NO puede ser componente de algo que ya está en
	//    la build: sería una compra redundante que el jugador nunca
	//    haría (Mystic Burst cuando Tankbuster ya está en el inventario,
	//    Compress Cooldown cuando Transcendent Cooldown cierra la
	//    cadena). Se excluye también cualquier objeto que la ruta ya
	//    compra como paso intermedio.
	used := map[int]bool{}
	for _, e := range out.Standard {
		used[e.Item] = true
	}
	// Componentes (transitivos) de todo lo que la build ya contiene.
	consumedByBuild := map[int]bool{}
	for _, e := range out.Standard {
		for _, id := range cat.Chain(e.Item) {
			if id != e.Item {
				consumedByBuild[id] = true
			}
		}
	}
	for _, c := range cands {
		if len(out.Extensions) >= opts.MaxExtensions {
			break
		}
		if used[c.Item] || !cat.Has(c.Item) {
			continue
		}
		if c.Confidence < opts.MinConfidence {
			continue
		}
		if consumedByBuild[c.Item] {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				"componente de un objeto que la ruta ya compra"})
			continue
		}
		// Ampliación = compra para partidas largas. Un objeto que se
		// compra en EARLY no amplía nada: si no entró en la ruta es
		// porque su evidencia no alcanza, no porque llegue tarde.
		if c.Phase != int(progression.PhaseLate) {
			out.Excluded = append(out.Excluded, Exclusion{
				c.Item, nameOf(cat, c.Item),
				fmt.Sprintf("no es de partida larga (fase %s)", progression.Phase(c.Phase))})
			continue
		}
		out.Extensions = append(out.Extensions, entryFor(c, cat, byItem))
		used[c.Item] = true
	}
	// Orden cronológico: una ampliación se compra en un momento, y la
	// lista debe leerse en ese orden. El score desempata a igual minuto.
	// Antes salían en el orden del ranking estadístico, que no coincide
	// con la cronología visible (~29, ~20, ~33, ~22 min).
	sort.SliceStable(out.Extensions, func(i, j int) bool {
		if out.Extensions[i].Minute != out.Extensions[j].Minute {
			return out.Extensions[i].Minute < out.Extensions[j].Minute
		}
		if out.Extensions[i].Score != out.Extensions[j].Score {
			return out.Extensions[i].Score > out.Extensions[j].Score
		}
		return out.Extensions[i].Item < out.Extensions[j].Item
	})

	// 8. Situacionales: candidatos con función identificable que no
	//    entraron en la ruta ni en ampliaciones. Tampoco se proponen
	//    componentes de lo que la build ya compra: comprarlos sería
	//    redundante.
	out.Situationals = situationals(cands, used, consumedByBuild, cat)

	// 9. Confianza global: el eslabón más débil de la ruta estándar.
	out.Confidence = 1
	if len(out.Standard) == 0 {
		out.Confidence = 0
	}
	for i, e := range out.Standard {
		if i == 0 || e.Confidence < out.Confidence {
			out.Confidence = e.Confidence
		}
	}
	return out, nil
}

// assembleSequence arma la secuencia de compras respetando las cadenas:
// antes de un objeto final van sus componentes, en orden.
func assembleSequence(chosen []Candidate, cat *progression.Catalog,
	byItem map[int]Candidate, opts BuildOptions) ([]progression.Purchase, []BuildEntry, []progression.Rejection) {

	var seq []progression.Purchase
	var entries []BuildEntry
	added := map[int]bool{}
	var rejects []progression.Rejection

	for _, c := range chosen {
		// Cadena desde la base hasta el objeto.
		chain := cat.Chain(c.Item)
		if len(chain) == 0 {
			chain = []int{c.Item}
		}
		// Verificar que la cadena es comprable y sin ciclos ya lo
		// garantiza el catálogo.
		var missing []int
		for _, id := range chain {
			if !cat.Has(id) {
				missing = append(missing, id)
			}
		}
		if len(missing) > 0 {
			rejects = append(rejects, progression.Rejection{
				Item: c.Item,
				Reason: fmt.Sprintf("cadena incompleta: falta %v en el catálogo",
					namesOf(cat, missing)),
			})
			continue
		}
		// Cada elemento de la cadena que se compra tiene su entrada.
		//
		// Antes solo se emitía entrada para el objeto final: los
		// componentes intermedios se compraban (y se pagaban) pero no
		// aparecían, así que la mejora decía "Mejora de X" con X
		// invisible y el total de almas no cuadraba con lo mostrado.
		for i, id := range chain {
			if added[id] {
				continue // ya comprado por una cadena anterior
			}
			minute := c.MedT / 60
			if i < len(chain)-1 {
				// Componente: unos minutos antes del final.
				minute -= (len(chain) - 1 - i)
				if minute < 1 {
					minute = 1
				}
			}
			origins := []progression.Origin{}
			cc, inCands := byItem[id]
			if inCands {
				origins = append(origins, progression.OriginStatistical)
				if cc.Share >= opts.MinShare {
					origins = append(origins, progression.OriginObserved)
				}
			} else {
				origins = append(origins, progression.OriginProgression)
			}
			seq = append(seq, progression.Purchase{
				Item: id, Minute: minute, Origin: origins,
			})
			added[id] = true

			if id == c.Item {
				entries = append(entries, entryFor(c, cat, byItem))
				continue
			}
			// Componente intermedio: entrada propia, consumida por el
			// siguiente eslabón de la cadena.
			entries = append(entries, chainStepEntry(id, chain[i+1], minute, cat, byItem))
		}
	}
	// La secuencia se compra en orden temporal; las entradas deben
	// contarlo igual. El sort es estable: a igual minuto se conserva el
	// orden de la cadena.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Minute < entries[j].Minute
	})
	return seq, entries, rejects
}

// chainStepEntry describe un componente que se compra como paso hacia
// un objeto final. Su confianza es la del objeto que lo consume: la
// decisión de comprarlo no es independiente.
func chainStepEntry(id, consumedBy, minute int, cat *progression.Catalog,
	byItem map[int]Candidate) BuildEntry {

	e := BuildEntry{
		Item:       id,
		Name:       nameOf(cat, id),
		Phase:      progression.PhaseOf(minute),
		Minute:     minute,
		Chain:      cat.Chain(id),
		ConsumedBy: consumedBy,
		Origin:     []progression.Origin{progression.OriginProgression},
	}
	if c, ok := byItem[id]; ok {
		e.Confidence = c.Confidence
		e.Score = c.Score
		e.Share = c.Share
		e.Origin = append(e.Origin, progression.OriginStatistical)
		if c.Share >= 0.30 {
			e.Origin = append(e.Origin, progression.OriginObserved)
		}
	}
	if cb, ok := byItem[consumedBy]; ok {
		e.Confidence = cb.Confidence
	}
	e.Reason = fmt.Sprintf("paso hacia %s", nameOf(cat, consumedBy))
	return e
}

// entryFor convierte un candidato en una entrada de build con su
// justificación legible.
func entryFor(c Candidate, cat *progression.Catalog, byItem map[int]Candidate) BuildEntry {
	e := BuildEntry{
		Item:       c.Item,
		Name:       nameOf(cat, c.Item),
		Phase:      progression.Phase(c.Phase),
		Minute:     c.MedT / 60,
		Confidence: c.Confidence,
		Score:      c.Score,
		Share:      c.Share,
		Chain:      cat.Chain(c.Item),
		Origin:     []progression.Origin{progression.OriginStatistical},
	}
	if c.Share >= 0.30 {
		e.Origin = append(e.Origin, progression.OriginObserved)
	}
	e.Reason = fmt.Sprintf("share %.0f%% · confianza %s (%.2f)",
		c.Share*100, ConfidenceLabel(c.Confidence), c.Confidence)
	if len(e.Chain) > 1 {
		e.Reason += fmt.Sprintf(" · cadena de %d", len(e.Chain))
	}
	return e
}

// situationals clasifica por función los candidatos que no entraron en
// la ruta. Las etiquetas salen de propiedades verificables del objeto
// (categoría y nombre), no de una afirmación sobre héroes concretos.
func situationals(cands []Candidate, used, consumedByBuild map[int]bool, cat *progression.Catalog) []BuildEntry {
	var out []BuildEntry
	for _, c := range cands {
		if used[c.Item] || consumedByBuild[c.Item] {
			continue
		}
		if !cat.Has(c.Item) {
			continue
		}
		e := entryFor(c, cat, nil)
		e.Reason = situationalFunction(cat, c.Item)
		if e.Reason == "" {
			continue // sin función identificable no se propone
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Item < out[j].Item
	})
	return out
}

// situationalFunction devuelve la etiqueta de función de un objeto, o
// vacío si no hay ninguna verificable. Las reglas son explícitas y se
// apoyan en el nombre/categoría del catálogo, no en suposiciones sobre
// enfrentamientos.
func situationalFunction(cat *progression.Catalog, id int) string {
	it, ok := cat.Item(id)
	if !ok {
		return ""
	}
	name := it.Name
	// El orden importa: las etiquetas más específicas primero. Un
	// objeto de curación no es "daño" por llamarse "... Shot".
	switch {
	case containsAny(name, "Healbane", "Toxic", "Anti-Heal", "Decay"):
		return "Anticuración"
	case containsAny(name, "Spirit Resilience", "Spirit Armor", "Spirit Shielding"):
		return "Defensa espiritual"
	case containsAny(name, "Shielding", "Armor", "Barrier", "Bulwark"):
		return "Defensa contra balas"
	case containsAny(name, "Debuff", "Unstoppable", "Purge", "Dispel"):
		return "Resistencia al control"
	case containsAny(name, "Regen", "Lifesteal", "Healing", "Restorative",
		"Stamina", "Health", "Booster", "Leech"):
		return "Supervivencia"
	case containsAny(name, "Cooldown", "Burst", "Rounds", "Striker",
		"Exposure", "Vulnerability", "Shot", "Surge"):
		return "Daño adicional"
	}
	return ""
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) <= len(s) && indexFold(s, sub) >= 0 {
			return true
		}
	}
	return false
}

// indexFold busca sub en s sin distinguir mayúsculas.
func indexFold(s, sub string) int {
	n := len(sub)
	if n == 0 {
		return 0
	}
	for i := 0; i+n <= len(s); i++ {
		if equalFold(s[i:i+n], sub) {
			return i
		}
	}
	return -1
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func nameOf(cat *progression.Catalog, id int) string {
	if it, ok := cat.Item(id); ok && it.Name != "" {
		return it.Name
	}
	return fmt.Sprintf("id=%d", id)
}

func namesOf(cat *progression.Catalog, ids []int) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, nameOf(cat, id))
	}
	return out
}

// ProgressionCatalog construye el catálogo de progresión desde el
// catálogo de tienda del cliente. Se memoiza por cliente: el catálogo
// es estático dentro de una sesión y reconstruirlo en cada héroe sería
// trabajo repetido.
func (a *APIClient) ProgressionCatalog() (*progression.Catalog, error) {
	a.progCatMu.Lock()
	defer a.progCatMu.Unlock()
	if a.progCat != nil {
		return a.progCat, nil
	}
	shop, _, err := a.shopItems()
	if err != nil {
		return nil, err
	}
	raw := make([]progression.RawItem, 0, len(shop))
	for id, it := range shop {
		raw = append(raw, progression.RawItem{
			ID: id, Name: it.Name, Class: it.ClassName,
			Slot: it.ItemSlotType, Tier: it.ItemTier, Cost: it.Cost,
			Comps: it.ComponentItems, Shopable: true,
		})
	}
	cat, _, err := progression.BuildCatalog(raw)
	if err != nil {
		return nil, err
	}
	a.progCat = cat
	return cat, nil
}
