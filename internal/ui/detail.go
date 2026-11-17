package ui

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/config"
	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/model"
)

// detailView es la pantalla de un campeón/héroe: cabecera horizontal
// con sus datos, pestañas de contenido (Ítems / Runas|Matchups /
// Habilidades) y tarjetas navegables con foco visible. Los roles se
// conmutan con r/R (o clic en la píldora), el estilo del retrato con
// `a` y Enter abre la tarjeta activa.
type detailView struct {
	champ    *ChampEntry
	session  *Session
	roleIdx  int
	tab      int  // índice dentro de tabs()
	itemIdx  int  // tarjeta seleccionada (lista plana core+botas)
	artMode  int  // 0 mini half-block · 1 ASCII color · 2 ASCII mono
	showItem bool // popup con el detalle de la tarjeta activa
	// showUpgrades es el toggle `u`: plegar las tarjetas de mejora y
	// dejar solo compras nuevas (Deadlock con ruta).
	showUpgrades bool
	width        int
	height       int            // alto de terminal (viewport)
	vp           viewport.Model // contenido scrolleable
	// scrollAnchor es la tarjeta cuyo scroll ya se fijó: evita que el
	// auto-scroll pisotee la rueda del ratón entre frames.
	scrollAnchor int
}

func newDetailView(c *ChampEntry, s *Session) detailView {
	vp := viewport.New(80, 24)
	// Rueda del ratón activa: el contenido largo se alcanza sin teclas.
	vp.MouseWheelEnabled = true
	return detailView{champ: c, session: s, roleIdx: c.MainRoleIdx(), width: 80, vp: vp, scrollAnchor: -1, showUpgrades: true}
}

// applyCfg restaura las preferencias persistentes (rol, pestaña y
// modo de retrato) cuando aplican al campeón actual.
func (d *detailView) applyCfg(c config.Config) {
	if c.LastRole != "" {
		for i, r := range d.champ.Roles {
			if r.String() == c.LastRole {
				d.roleIdx = i
				break
			}
		}
	}
	if c.LastTab < len(d.tabs()) {
		d.tab = c.LastTab
	}
	if c.ArtMode >= 0 && c.ArtMode < 3 {
		d.artMode = c.ArtMode
	}
	d.clampSel()
}

// lastTab es el índice de la última pestaña del juego.
func lastTab(d *detailView) int { return len(d.tabs()) - 1 }

// RenderDetail expone la vista de un campeón (rol principal) para la
// inspección sin TUI (cmd/render y tests).
func RenderDetail(c *ChampEntry, s *Session) string {
	return newDetailView(c, s).View()
}

// NewDetailForRender expone la vista con estado para cmd/render.
func NewDetailForRender(c *ChampEntry, s *Session) detailView {
	return newDetailView(c, s)
}

// SelectTabForRender selecciona pestaña por nombre (cmd/render).
func (d detailView) SelectTabForRender(name string) detailView {
	for i, t := range d.tabs() {
		if strings.EqualFold(t, name) {
			d.tab = i
		}
	}
	return d
}

// tabs del juego: Deadlock no tiene runas; ambos tienen resumen y
// Lolalytics trae matchups (tabla enemy) también para LoL.
func (d detailView) tabs() []string {
	if d.session.Game == model.GameDeadlock {
		return []string{"Ítems", "Resumen", "Matchups", "Habilidades"}
	}
	// Nombres cortos: siete pestañas deben caber en 80 columnas junto
	// con su número. El nombre largo va en el contenido de la vista.
	return []string{"Ítems", "Ruta", "Alt.", "Resumen", "Runas", "Habil.", "Match."}
}

// lolPlan devuelve el plan de build del campeón/rol actual, o nil si no
// hay datos para construirlo. El cálculo está cacheado en la sesión por
// (campeón, rol, parche, misión), así que llamarlo desde el render no
// recalcula nada.
func (d detailView) lolPlan() *lolbuild.Plan {
	if d.session == nil || d.session.Game != model.GameLoL || d.champ == nil {
		return nil
	}
	cb := d.CurrentBuild()
	if cb == nil {
		return nil
	}
	mission := d.session.MissionStateFor(cb.Role)
	return d.session.LolPlan(cb, d.champ.Key, cb.Role, mission)
}

// cardW leaves enough space for readable Deadlock names and purchase cues.
func (d detailView) cardW() int {
	if d.session.Game == model.GameDeadlock {
		return min(32, max(18, d.width-10))
	}
	return 24
}

// cardsPerRow se calcula del ancho real: se llena la pantalla en vez
// de fijar columnas (tope 8 por legibilidad).
func (d detailView) cardsPerRow() int {
	w := d.cardW() + 2
	if d.session.Game == model.GameDeadlock {
		return max(1, min(6, (d.width-8)/w))
	}
	return max(1, min(8, (d.width-8)/w))
}

// popupOpen dice si el popup de tarjeta está abierto (esc lo cierra
// antes de subir de nivel).
func (d detailView) popupOpen() bool { return d.showItem }

// artModeName nombra el modo de retrato para el toast.
func (d detailView) artModeName() string {
	switch d.artMode {
	case 1:
		return "ASCII color"
	case 2:
		return "ASCII mono"
	default:
		return "mini"
	}
}

// Update conmuta roles (r/R y también [/]), pestañas (tab/1..3),
// tarjetas (flechas y hjkl), abre el popup con Enter, alterna el
// retrato con `a` (notificando el modo) y atiende clics en las
// píldoras de rol.
// handleMouse resuelve un clic contra las zonas marcadas: role:N,
// tab:N y card:N.
func (d detailView) handleMouse(mm tea.MouseMsg) detailView {
	for i := range d.champ.Roles {
		if z := zoneMgr.Get(fmt.Sprintf("role:%d", i)); !z.IsZero() && z.InBounds(mm) {
			d.roleIdx = i
			d.clampSel()
			d.vp.GotoTop()
			return d
		}
	}
	for i := range d.tabs() {
		if z := zoneMgr.Get(fmt.Sprintf("tab:%d", i)); !z.IsZero() && z.InBounds(mm) {
			d.tab = i
			d.clampSel()
			d.vp.GotoTop()
			return d
		}
	}
	if d.tab == 0 {
		for i := 0; i < len(d.cardList()); i++ {
			if z := zoneMgr.Get(fmt.Sprintf("card:%d", i)); !z.IsZero() && z.InBounds(mm) {
				d.itemIdx = i
				return d
			}
		}
	}
	return d
}

func (d detailView) Update(msg tea.Msg) (detailView, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		// La rueda del ratón salta de fila en fila: cortar una tarjeta
		// por la mitad (solo el icono visible) deja la sección
		// ilegible. El resto de eventos de mouse, al viewport.
		if mm, isMouse := msg.(tea.MouseMsg); isMouse {
			if (mm.Button == tea.MouseButtonWheelUp || mm.Button == tea.MouseButtonWheelDown) &&
				mm.Action == tea.MouseActionPress {
				if mm.Button == tea.MouseButtonWheelUp {
					d.vp.LineUp(d.rowScrollH())
				} else {
					d.vp.LineDown(d.rowScrollH())
				}
				return d, nil
			}
			d.vp, _ = d.vp.Update(mm)
		}
		return d, nil
	}
	// Popup modal: mientras está abierto solo atienden esc, enter y
	// ←→. Si no, tab/r/a/flechas cambiaban el estado de debajo sin que
	// se viera nada, y el popup quedaba mintiendo.
	if d.showItem {
		switch km.String() {
		case "esc", "enter":
			d.showItem = false
		case "left", "h":
			if d.itemIdx > 0 {
				d.itemIdx--
			}
		case "right", "l":
			if n := len(d.cardList()); d.itemIdx < n-1 {
				d.itemIdx++
			}
		}
		return d, nil
	}
	nRoles := len(d.champ.Roles)
	if nRoles == 0 {
		nRoles = 1
	}
	tabs := d.tabs()
	nCards := len(d.cardList())
	// El scroll se recalcula al final: Update guarda el receptor
	// completo y View solo recibe una copia, así que el offset hay que
	// fijarlo aquí para que persista entre frames.
	switch km.String() {
	case "r", "]":
		d.roleIdx = (d.roleIdx + 1) % nRoles
		d.clampSel()
		d.vp.GotoTop()
	case "R", "[":
		d.roleIdx = (d.roleIdx - 1 + nRoles) % nRoles
		d.clampSel()
		d.vp.GotoTop()
	case "tab":
		d.tab = (d.tab + 1) % len(tabs)
		d.clampSel()
		d.vp.GotoTop()
	case "shift+tab":
		d.tab = (d.tab - 1 + len(tabs)) % len(tabs)
		d.clampSel()
		d.vp.GotoTop()
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if idx := int(km.String()[0] - '1'); idx < len(tabs) {
			d.tab = idx
			d.clampSel()
			d.vp.GotoTop()
		}
	case "M":
		// Estado de misión de BOT (antes/después). Es una preferencia
		// de VISTA: no se deduce el estado real de la partida. Solo
		// aplica donde la misión cambia el inventario (BOT).
		cb := d.CurrentBuild()
		if cb == nil || !d.session.ToggleBotMission(cb.Role) {
			break
		}
		d.sync()
		if d.session.botMissionDone {
			return d, toastCmd("BOT: después de la misión")
		}
		return d, toastCmd("BOT: antes de la misión")
	case "a":
		d.artMode = (d.artMode + 1) % 3
		d.sync()
		return d, toastCmd("Retrato: " + d.artModeName())
	case "u":
		if d.session.Game == model.GameDeadlock {
			d.showUpgrades = !d.showUpgrades
			d.clampSel()
			d.scrollAnchor = -1 // la lista cambia: el auto-scroll reacomoda
			d.sync()
			estado := "plegadas"
			if d.showUpgrades {
				estado = "visibles"
			}
			return d, toastCmd("Mejoras: " + estado)
		}
	case "left", "h":
		if d.itemIdx > 0 {
			d.itemIdx--
		}
	case "right", "l":
		if d.itemIdx < nCards-1 {
			d.itemIdx++
		}
	case "up", "k":
		if d.itemIdx -= d.cardsPerRow(); d.itemIdx < 0 {
			d.itemIdx = 0
		}
	case "down", "j":
		if d.itemIdx += d.cardsPerRow(); d.itemIdx > nCards-1 {
			d.itemIdx = nCards - 1
		}
	case "enter":
		if d.tab == 0 && nCards > 0 {
			d.showItem = true
		}
	case "esc":
		d.showItem = false
	}
	// Teclas de página para el contenido que no cabe: las flechas ya
	// mueven la selección de tarjeta, así que al viewport solo le
	// llegan las que no usamos (pgup/pgdn/home/end y u/d).
	switch km.String() {
	case "pgup", "pgdown", "home", "end", "ctrl+u", "ctrl+d":
		d.vp, _ = d.vp.Update(msg)
	}
	d.sync()
	return d, nil
}

// clampSel ajusta la selección al cambiar de rol/pestaña.
func (d *detailView) clampSel() {
	if n := len(d.cardList()); d.itemIdx >= n {
		d.itemIdx = max(0, n-1)
	}
}

// CurrentBuild es el build del rol activo.
func (d detailView) CurrentBuild() *model.CleanBuild {
	return d.champ.Build(d.roleIdx)
}

// cardKind distingue compra directa de mejora en la ruta de compra.
type cardKind int

const (
	kindBuy cardKind = iota
	kindUpgrade
)

// cardItem es una tarjeta de ítem: core, botas o ward. Con ruta de
// compra cargada (Deadlock), cada ítem es su propia tarjeta: la mejora
// es un tipo distinto (borde doble, ↑, `de <base>`, coste neto) y el
// orden viene del consenso de partidas. Sin ruta: compras solamente,
// con las cifras de item-stats.
type cardItem struct {
	item         model.RankedItem
	boot         bool
	ward         bool
	kind         cardKind
	order        int     // posición en la ruta de consenso (1..n), 0 = sin ruta
	step         int     // número de paso visible (1..m), 0 = fuera de la ruta
	baseID       int     // de la mejora: su base (0 = compra directa)
	baseName     string  // nombre de la base para la línea `de X`
	chainRate    float64 // de la mejora: fracción que viene de baseID
	upgradedRate float64 // de la compra: fracción de compradores que luego la mejoran
	routeT       int     // segundo mediano de la ruta (0 = usar AvgBuyS)
	// Campos de la build híbrida (0/vacío = tarjeta de la ruta observada).
	hybrid       bool
	hybridPhase  int
	hybridConf   float64
	hybridShare  float64
	hybridOrigin []string
	hybridReason string
	// hybridSection distingue ruta, ampliaciones y situacionales.
	hybridSection HybridSection
}

// dlMinuteOf elige el minuto que representa la tarjeta: el mediano de
// la ruta cuando hay, el medio de item-stats como respaldo.
func (c cardItem) dlMinuteOf() int {
	if c.routeT > 0 {
		return c.routeT
	}
	return int(c.item.AvgBuyS)
}

// dlDecorate aplica la ruta de consenso a las tarjetas de Deadlock:
// clasifica mejora (ChainRate ≥ 50 %), numera en orden de consenso
// (compras y mejoras juntas) y elige fase por el minuto propio.
func (d detailView) dlDecorate(cards []cardItem) []cardItem {
	steps := d.session.Routes[d.champ.Key]
	if len(steps) == 0 {
		return cards
	}
	byID := make(map[int]RouteStep, len(steps))
	for i, st := range steps {
		st.Order = i + 1
		byID[st.Item] = st
	}
	for i := range cards {
		st, ok := byID[cards[i].item.ID]
		if !ok {
			continue
		}
		c := &cards[i]
		c.order = st.Order
		c.routeT = st.MedT
		c.chainRate = st.ChainRate
		c.upgradedRate = st.UpgradedRate
		if st.UpgradeOf != 0 && st.ChainRate >= 0.5 {
			c.kind = kindUpgrade
			c.baseID = st.UpgradeOf
			c.baseName = d.session.ItemName(st.UpgradeOf)
		}
	}
	return cards
}

// hybridCards construye las tarjetas desde la build híbrida del héroe.
// Devuelve ok=false cuando no hay build calculada: el llamador cae a la
// ruta observada.
//
// El orden es el de la secuencia de compra (minuto ascendente), con las
// ampliaciones y los situacionales al final en sus propias secciones.
// Los números de paso son continuos sobre la ruta estándar.
func (d detailView) hybridCards() ([]cardItem, bool) {
	hb := d.session.Hybrids[d.champ.Key]
	if hb == nil || len(hb.Standard) == 0 {
		return nil, false
	}
	// Una build calculada con otra clave (parche, filtros) está
	// obsoleta: no se sirve. Se cae a la ruta observada mientras se
	// recalcula.
	if hb.Key != d.session.HybridKey {
		return nil, false
	}
	byID := make(map[int]model.RankedItem, len(d.CurrentBuild().CoreItems))
	for _, it := range d.CurrentBuild().CoreItems {
		byID[it.ID] = it
	}
	var out []cardItem
	step := 0
	for _, hs := range hb.Standard {
		it, ok := byID[hs.Item]
		if !ok {
			continue // sin estadísticas del ítem: no se inventa una tarjeta
		}
		step++
		c := cardItem{
			item:          it,
			order:         step,
			step:          step,
			routeT:        hs.Minute * 60,
			hybrid:        true,
			hybridPhase:   hs.Phase,
			hybridConf:    hs.Confidence,
			hybridShare:   hs.Share,
			hybridOrigin:  hs.Origin,
			hybridReason:  hs.Reason,
			hybridSection: SectionStandard,
		}
		// Cadena: si el objeto consume un componente, la tarjeta lo
		// declara como mejora para que la relación sea legible.
		if hs.ConsumedBy == 0 && len(hs.Chain) > 1 {
			base := hs.Chain[len(hs.Chain)-2]
			if _, ok := byID[base]; ok {
				c.kind = kindUpgrade
				c.baseID = base
				c.baseName = d.session.ItemName(base)
				c.chainRate = 1
			}
		}
		if hs.ConsumedBy != 0 {
			// Paso intermedio: se marca como mejora de su base y se
			// sabe que otro objeto lo consume.
			if len(hs.Chain) > 1 {
				base := hs.Chain[len(hs.Chain)-2]
				c.kind = kindUpgrade
				c.baseID = base
				c.baseName = d.session.ItemName(base)
				c.chainRate = 1
			}
		}
		out = append(out, c)
	}
	// Ampliaciones y situacionales: sin número de paso (no son parte de
	// la ruta), en secciones propias.
	addExtra := func(steps []HybridStep, section HybridSection) {
		for _, hs := range steps {
			it, ok := byID[hs.Item]
			if !ok {
				continue
			}
			out = append(out, cardItem{
				item:          it,
				order:         0,
				step:          0,
				routeT:        hs.Minute * 60,
				hybrid:        true,
				hybridPhase:   hs.Phase,
				hybridConf:    hs.Confidence,
				hybridShare:   hs.Share,
				hybridOrigin:  hs.Origin,
				hybridReason:  hs.Reason,
				hybridSection: section,
			})
		}
	}
	addExtra(hb.Extensions, SectionExtensions)
	addExtra(hb.Situationals, SectionSituationals)
	return out, true
}

// cardList aplana core+botas para la navegación. Un campeón sin build
// (rol no canónico) devuelve lista vacía en vez de reventar.
func (d detailView) cardList() []cardItem {
	b := d.CurrentBuild()
	if b == nil {
		return nil
	}
	var out []cardItem
	for _, it := range b.CoreItems {
		out = append(out, cardItem{item: it})
	}
	for _, bt := range b.Boots {
		out = append(out, cardItem{item: bt, boot: true})
	}
	if d.session.Game == model.GameDeadlock {
		// Con build híbrida disponible, las tarjetas salen de ella: el
		// motor de dominio ya decidió qué entra, en qué orden y con qué
		// cadena. La UI no repite estadística.
		if cards, ok := d.hybridCards(); ok {
			return cards
		}
		// Sin build híbrida (cargando, error o muestra corta): ruta
		// observada, comportamiento anterior intacto.
		return numberSteps(d.filterUpgrades(d.dlBuildCards(d.dlDecorate(out))))
	}
	// LoL: una sola bota como recomendación visible; el resto va
	// plegado al pie de la sección BOTAS (casi nadie compra la cuarta
	// opción y confunde más de lo que ayuda).
	if len(b.Boots) > 1 {
		out = out[:len(b.CoreItems)+1]
	}
	// El mejor ward cierra la lista en su propia sección WARD.
	if len(b.Wards) > 0 {
		out = append(out, cardItem{item: b.Wards[0], ward: true})
	}
	return out
}

// foldedBootsResume nombra las botas plegadas con su WR para la línea
// informativa de la sección BOTAS.
func (d detailView) foldedBootsResume() string {
	b := d.CurrentBuild()
	if d.session.Game == model.GameDeadlock || len(b.Boots) <= 1 {
		return ""
	}
	rest := b.Boots[1:]
	parts := make([]string, 0, len(rest))
	for _, bt := range rest {
		parts = append(parts, fmt.Sprintf("%s %s", d.session.ItemName(bt.ID), pct(bt.WinRate)))
	}
	return fmt.Sprintf("otras botas (%d): %s", len(rest), strings.Join(parts, " · "))
}

// dlBuildCards elige la build de Deadlock como la construyen los
// trackers: popularidad relativa al ítem más comprado del héroe (no
// al total de partidas, que infla las cortas), piso de uso del 8%,
// puntuación por calidad significativa (delta Wilson sobre el WR base
// del héroe) y presentación en fases por el minuto PROPIO de cada
// tarjeta (mediano de la ruta cuando hay; medio de item-stats como
// respaldo) — no el de su base.
func (d detailView) dlBuildCards(all []cardItem) []cardItem {
	const (
		minRelUse = 0.08 // piso de uso relativo
		perPhase  = 8
	)
	b := d.CurrentBuild()
	// Popularidad relativa al ítem más comprado.
	top := 0
	for _, c := range all {
		if c.item.Games > top {
			top = c.item.Games
		}
	}
	if top == 0 {
		return nil
	}
	type scored struct {
		c     cardItem
		score float64
	}
	var candidates []scored
	for _, c := range all {
		rel := float64(c.item.Games) / float64(top)
		if rel < minRelUse {
			continue
		}
		// Calidad significativa: límite inferior de Wilson del ítem
		// menos el WR base del héroe (corrige "se compra cuando ya
		// ganas" y héroes con WR global bajo).
		sig := c.item.Wilson - b.WinRate
		score := 2.0*math.Sqrt(rel) + 6.0*sig
		candidates = append(candidates, scored{c, score})
	}
	// Dentro de cada fase: top-K por score; el minuto que decide la
	// fase es el propio de la tarjeta (mediano de la ruta cuando hay;
	// medio de item-stats como respaldo) — no el de su base.
	var build []cardItem
	for phase := 0; phase < 3; phase++ {
		var inPhase []scored
		for _, s := range candidates {
			if dlPhaseOf(s.c.dlMinuteOf()) == phase {
				inPhase = append(inPhase, s)
			}
		}
		sort.SliceStable(inPhase, func(i, j int) bool { return inPhase[i].score > inPhase[j].score })
		if len(inPhase) > perPhase {
			inPhase = inPhase[:perPhase]
		}
		for _, s := range inPhase {
			build = append(build, s.c)
		}
	}
	// Route order wins over timing estimates; unranked alternatives follow it.
	sort.SliceStable(build, func(i, j int) bool {
		a, b := build[i], build[j]
		if a.order > 0 && b.order > 0 {
			return a.order < b.order
		}
		if (a.order > 0) != (b.order > 0) {
			return a.order > 0
		}
		return a.dlMinuteOf() < b.dlMinuteOf()
	})
	// Cadena completa: el corte por fase puede dejar fuera la base de
	// una mejora visible, y entonces la tarjeta dice "Mejora de X" sin
	// que X esté antes. Se recupera desde la ruta aunque exceda el tope
	// de la fase: sin la base, la tarjeta cuenta media historia.
	byID := make(map[int]cardItem, len(all))
	for _, c := range all {
		byID[c.item.ID] = c
	}
	present := make(map[int]bool, len(build))
	for _, c := range build {
		present[c.item.ID] = true
	}
	for _, c := range build {
		if c.kind != kindUpgrade || c.baseID == 0 || present[c.baseID] {
			continue
		}
		base, ok := byID[c.baseID]
		if !ok || base.order == 0 {
			continue // sin paso de ruta no hay una posición con sentido
		}
		build = append(build, base)
		present[base.item.ID] = true
	}
	sort.SliceStable(build, func(i, j int) bool {
		a, b := build[i], build[j]
		if a.order > 0 && b.order > 0 {
			return a.order < b.order
		}
		if (a.order > 0) != (b.order > 0) {
			return a.order > 0
		}
		return a.dlMinuteOf() < b.dlMinuteOf()
	})
	return build
}

// numberSteps asigna a cada tarjeta de la ruta un número de paso
// consecutivo (1..m) sobre la lista YA filtrada. `order` es la posición
// en la ruta de consenso y se conserva para el detalle y las
// explicaciones, pero no puede usarse como número visible: al cortar
// por fase o plegar mejoras quedan huecos (7 → 9). El número de paso
// describe lo que el jugador ve, no lo que la ruta contenía.
func numberSteps(cards []cardItem) []cardItem {
	step := 0
	for i := range cards {
		if cards[i].order == 0 {
			cards[i].step = 0 // fuera de la ruta: sin número
			continue
		}
		step++
		cards[i].step = step
	}
	return cards
}

// filterUpgrades aplica el toggle `u` (mejoras sí/no): quien solo
// quiere compras nuevas puede plegar las tarjetas de mejora.
func (d detailView) filterUpgrades(cards []cardItem) []cardItem {
	if d.showUpgrades {
		return cards
	}
	out := make([]cardItem, 0, len(cards))
	for _, c := range cards {
		if c.kind != kindUpgrade {
			out = append(out, c)
		}
	}
	return out
}

// routeStepOf busca el paso de ruta del ítem (Deadlock con ruta).
func (d detailView) routeStepOf(id int) (RouteStep, bool) {
	for _, st := range d.session.Routes[d.champ.Key] {
		if st.Item == id {
			return st, true
		}
	}
	return RouteStep{}, false
}

// relatedSel devuelve los ids de las tarjetas emparentadas con la
// seleccionada: su base (si es mejora) o sus mejoras (si es base).
// La pareja se resalta con dorado tenue y la línea de selección
// explica la cadena.
func (d detailView) relatedSel() map[int]bool {
	if d.tab != 0 {
		return nil
	}
	cards := d.cardList()
	if d.itemIdx >= len(cards) {
		return nil
	}
	sel := cards[d.itemIdx]
	var rel map[int]bool
	if sel.baseID != 0 {
		rel = map[int]bool{sel.baseID: true}
	}
	for _, c := range cards {
		if c.kind == kindUpgrade && c.baseID == sel.item.ID {
			if rel == nil {
				rel = map[int]bool{}
			}
			rel[c.item.ID] = true
		}
	}
	return rel
}

// dlPhaseOf clasifica un tiempo de compra (segundos) en fase.
func dlPhaseOf(s int) int {
	switch {
	case s < 600: // 10 min
		return 0
	case s < 1200: // 20 min
		return 1
	default:
		return 2
	}
}

var dlPhaseNames = []string{"EARLY", "MID", "LATE"}

// ─── vista ─────────────────────────────────────────────────────────

// layout fija el Width/Height del viewport con el layout real: el
// alto de la cabecera cambia con el retrato lateral y el pie solo
// existe en la pestaña de ítems.
func (d *detailView) layout(extra int) {
	d.vp.Width = max(24, d.width-4)
	// 1 cabecera de la app · cabecera del detalle (variable) ·
	// 1 pestañas · pie (selección+leyenda) · 1 pie de la app ·
	// 2 bordes del panel · 1 de aire. `extra` reserva la línea de
	// estado del scroll (indicador o título fijo de sección).
	fixed := 1 + lipgloss.Height(d.headerBlock()) + 1 + lipgloss.Height(d.itemsFooter()) + 1 + 2 + 1 + extra
	d.vp.Height = max(6, d.height-fixed)
	if d.height <= 0 {
		// Sin WindowSizeMsg (cmd/render, tests): mostrar todo el
		// contenido sin recortar.
		d.vp.Height = 200
	}
}

// scrollSelected lleva el viewport hasta la tarjeta activa: si empieza
// por encima del borde superior sube, y si no cabe entera baja lo justo
// para que se vea. Solo actúa cuando la selección cambió (anchor), para
// no pisotear el scroll manual con la rueda.
func (d *detailView) scrollSelected() {
	if d.tab != 0 || d.showItem || d.scrollAnchor == d.itemIdx {
		return
	}
	cards := d.cardList()
	if len(cards) == 0 || d.itemIdx >= len(cards) {
		return
	}
	_, starts := d.itemsSections(cards)
	// El contenido va dentro del panel: borde (1) + padding (1).
	line0 := starts[d.itemIdx] + 2
	lineH := lipgloss.Height(d.itemCard(d.itemIdx, cards[d.itemIdx]))
	switch {
	case d.itemIdx < d.cardsPerRow():
		// Primera fila: arriba del todo, sin dejar el borde cortado.
		d.vp.SetYOffset(0)
	case line0 < d.vp.YOffset:
		// Sube con un poco de aire para no pegar la tarjeta al borde.
		d.vp.SetYOffset(max(0, line0-2))
	case line0+lineH > d.vp.YOffset+d.vp.Height:
		// Si la tarjeta no cabe entera (iconos + nombre + métricas),
		// alinear por abajo escondería su principio: en ese caso se
		// prioriza que se vea desde arriba.
		if lineH > d.vp.Height-2 {
			d.vp.SetYOffset(max(0, line0-2))
		} else {
			d.vp.SetYOffset(line0 + lineH - d.vp.Height)
		}
	}
	d.scrollAnchor = d.itemIdx
}

// sync deja el viewport listo: tamaño, contenido y scroll. Se llama en
// Update (donde el estado persiste) y en View para el render suelto.
func (d *detailView) sync() {
	d.layout(0)
	if d.showItem {
		return
	}
	d.vp.SetContent(stPanel.Render(keepPanelBG(d.tabView())))
	// La línea de estado del scroll solo se reserva cuando el
	// contenido desborda: sin desborde, esa línea es espacio útil.
	if d.overflow() {
		d.layout(1)
		d.vp.SetContent(stPanel.Render(keepPanelBG(d.tabView())))
	}
	d.scrollSelected()
}

// overflow dice si el contenido de la pestaña activa no entra en el
// viewport (es cuando la línea de estado del scroll tiene sentido).
func (d detailView) overflow() bool {
	return !d.showItem && d.vp.TotalLineCount() > d.vp.Height
}

// rowScrollH es cuántas líneas avanza la rueda: una fila de tarjetas
// en la pestaña de ítems, 3 líneas en el resto de pestañas.
func (d detailView) rowScrollH() int {
	if d.tab != 0 {
		return 3
	}
	if d.compactMode() {
		return 5
	}
	cards := d.cardList()
	if len(cards) == 0 {
		return 3
	}
	return lipgloss.Height(d.itemCard(0, cards[0]))
}

func (d detailView) View() string {
	// El receptor es una copia: fijar aquí el ancho del viewport es
	// seguro (no se fuga entre frames) y evita el ancho heredado.
	d.vp.Width = max(24, d.width-4)
	header := d.headerBlock()
	// El popup sustituye el contenido y no se mezcla con el scroll:
	// si viviera dentro del viewport, su pie ("esc cierra") podría
	// quedar por debajo del pliegue.
	if d.showItem {
		return zoneMgr.Scan(lipgloss.JoinVertical(lipgloss.Left,
			header, d.tabsView(), d.itemsFooter(),
			lipgloss.NewStyle().Padding(1, 0).Render(d.itemPopup()),
		))
	}
	// El contenido vive en un viewport: cabecera y pestañas fijas,
	// rueda del ratón y sin cortes por arriba ni por abajo. La línea
	// de estado del scroll vive en el borde INFERIOR del panel (ya no
	// queda pegada a las pestañas como un resto de texto); el título
	// fijo de sección sigue arriba, que es donde sirve como contexto.
	view := d
	view.sync()
	pinned := ""
	var hint string
	if view.overflow() {
		pinned = view.activeSection()
		hint = scrollHint(!view.vp.AtTop(), view.vp.TotalLineCount()-view.vp.YOffset-view.vp.Height)
	}
	parts := []string{header, view.tabsView()}
	if pinned != "" {
		parts = append(parts, pinned)
	}
	parts = append(parts, view.vp.View())
	if footer := view.itemsFooter(); footer != "" {
		parts = append(parts, footer)
	}
	if pinned == "" && hint != "" {
		parts = append(parts, hint)
	}
	return zoneMgr.Scan(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

// scrollHint avisa lo que queda fuera del viewport. `above` dice si
// quedan líneas ocultas arriba; `below` devuelve cuántas quedan abajo
// (0 = se llegó al final).
func scrollHint(above bool, below int) string {
	if !above && below <= 0 {
		return ""
	}
	var parts []string
	if above {
		parts = append(parts, "▲ más arriba")
	}
	if below > 0 {
		parts = append(parts, fmt.Sprintf("▼ quedan %d líneas", below))
	}
	return stDim.Render("  " + strings.Join(parts, " · "))
}

// activeSection devuelve el título de la sección cuyo bloque quedó
// bajo el borde superior del viewport, para mantenerlo fijo mientras
// se scrollea la pestaña de ítems. Vacío cuando se ve el principio.
func (d detailView) activeSection() string {
	if d.tab != 0 {
		return ""
	}
	cards := d.cardList()
	if len(cards) == 0 {
		return ""
	}
	sections, _ := d.itemsSections(cards)
	// El contenido va dentro del panel: borde (1) + padding (1).
	off := d.vp.YOffset - 2
	if off <= 0 {
		return ""
	}
	line := 0
	active := ""
	for i, s := range sections {
		if i%2 == 0 { // posiciones pares: títulos de sección
			trimmed := strings.TrimSpace(s)
			if strings.Contains(trimmed, "◆ ") {
				if line > off {
					break
				}
				active = trimmed
			}
		}
		line += lipgloss.Height(s)
	}
	return active
}

// headerBlock es la cabecera del detalle, con el retrato grande (modos
// ASCII) como panel lateral cuando toca.
func (d detailView) headerBlock() string {
	header := d.headerView()
	if d.artMode > 0 {
		if side := d.portraitSide(); side != "" {
			header = lipgloss.JoinHorizontal(lipgloss.Top, header, " ", side)
		}
	}
	// Cabecera compacta en dos casos:
	//   1. La completa no deja viewport utilizable. El umbral se calcula
	//      con el alto real de la cabecera, no con un número fijo: con
	//      retrato son ~10 líneas, así que a 20 filas ya no cabe nada.
	//   2. El usuario se desplazó: mientras navega compras interesa el
	//      contenido, no el retrato. Al volver arriba se recupera.
	if d.height > 0 && !d.headerFits(header) {
		return d.compactHeader()
	}
	if d.scrolled() {
		return d.compactHeader()
	}
	return header
}

// headerFits dice si la cabecera completa deja un viewport usable.
// Se reserva el mínimo del viewport (6) más pestañas, pie y bordes.
func (d detailView) headerFits(header string) bool {
	const minViewport = 6
	const chrome = 1 + 1 + 2 + 1 // pestañas + pie + bordes + aire
	return d.height-lipgloss.Height(header)-chrome >= minViewport
}

// scrolled indica que el viewport no está al principio. Solo cuenta en
// la pestaña de ítems: en las demás el scroll no cambia la cabecera.
func (d detailView) scrolled() bool {
	return d.tab == 0 && d.vp.YOffset > 0
}

// compactHeader es la cabecera de una línea para terminales muy bajas:
// nombre y, si cabe, el parche. Sin retrato ni tendencia.
func (d detailView) compactHeader() string {
	b := d.CurrentBuild()
	name := stTitle.Render(d.champ.Name)
	if b == nil {
		return name
	}
	rest := stDim.Render(" · " + b.Patch)
	if lipgloss.Width(name)+lipgloss.Width(rest) > d.width-2 {
		return name
	}
	return name + rest
}

// keepPanelBG repone el fondo del panel tras cada reset ANSI interno:
// sin esto, los \x1b[0m de los estilos hijos dejan franjas con el
// fondo de la terminal.
func keepPanelBG(s string) string {
	return strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+panelBgEsc)
}

// headerView: retrato mini + nombre/roles + métricas, en horizontal.
func (d detailView) headerView() string {
	b := d.CurrentBuild()
	name := lipgloss.JoinHorizontal(lipgloss.Bottom,
		stTitle.Render(d.champ.Name), "  ", d.rolesBadges())
	var metrics string
	if b == nil {
		// Campeón sin rol canónico: cabecera sin métricas en vez de
		// desreferenciar nil.
		metrics = stDim.Render("sin datos para los roles de este juego")
	} else {
		// El separador final solo se dibuja si hay prioridad de
		// habilidades: sin ella quedaba un "·" colgando al final.
		parts := []string{
			stDim.Render("Patch " + b.Patch + " · " + nThousand(b.Total.Games) + " partidas · "),
			wrStyle(b.WinRate, b.Total.Games).Render("WR " + pct(b.WinRate)),
		}
		if prio := d.priorityLine(); prio != "" {
			parts = append(parts, stDim.Render("  ·  "), prio)
		}
		metrics = lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	}
	// Las métricas envuelven por palabras para no desbordar en
	// terminales estrechas. El retrato mini se come 15 columnas.
	headerW := max(20, d.width-10)
	if d.artMode > 0 {
		headerW -= 26 // deja sitio al retrato lateral
	} else if d.width >= 100 && d.session.ChampionArt[d.champ.Key].Mini != "" {
		headerW -= 15 // retrato mini + separación
	}
	metrics = lipgloss.NewStyle().Width(max(20, headerW)).Render(metrics)
	info := lipgloss.JoinVertical(lipgloss.Left, name, "", metrics)
	// Franja de tendencia mensual (Deadlock con datos reales).
	if d.width >= 90 {
		if trend := d.trendStrip(); trend != "" {
			info = lipgloss.JoinVertical(lipgloss.Left, info, trend)
		}
	}
	// El retrato mini se oculta en terminales estrechos.
	if d.width >= 100 && d.artMode == 0 {
		if mini := d.session.ChampionArt[d.champ.Key].Mini; mini != "" {
			info = lipgloss.JoinHorizontal(lipgloss.Center, mini, "   ", info)
		}
	}
	return stPanel.Render(keepPanelBG(info))
}

// sparkRunes mapea 0..1 a un mini-gráfico de una celda.
var sparkRunes = []rune("▁▂▃▄▅▆▇█")

// trendStrip: WR de los últimos meses con chispas, si la fuente lo
// trae (Deadlock real).
func (d detailView) trendStrip() string {
	b := d.CurrentBuild()
	if b == nil || len(b.WRTrend) < 2 {
		return ""
	}
	const lo, hi = 0.45, 0.60
	// Con 2-3 puntos, dos bloques sueltos no dicen nada: cada uno
	// lleva su mes ("55,2 % (oct 25)"); con 4+ la chispa ya se lee
	// como línea de tendencia.
	labeled := len(b.WRTrend) < 4
	parts := []string{stDim.Render("Tendencia ")}
	for i, p := range b.WRTrend {
		if i > 0 {
			parts = append(parts, stDim.Render("→ "))
		}
		if labeled {
			parts = append(parts, wrStyle(p.WR, p.Games).Render(pct(p.WR)),
				stDim.Render(" ("+monthLabel(p.Time)+") "))
			continue
		}
		f := (p.WR - lo) / (hi - lo)
		f = max(0, min(1, f))
		spark := sparkRunes[int(f*float64(len(sparkRunes)-1)+0.5)]
		parts = append(parts, wrStyle(p.WR, p.Games).Render(pct(p.WR)+string(spark)), " ")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

// meses nombra las ventanas de la tendencia en español.
var meses = []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// monthLabel nombra la ventana mensual de la tendencia: "oct 25".
func monthLabel(t int64) string {
	m := time.Unix(t, 0).UTC()
	return fmt.Sprintf("%s %02d", meses[int(m.Month())-1], m.Year()%100)
}

// portraitSide: retrato ASCII como panel lateral (≈20×10 celdas).
func (d detailView) portraitSide() string {
	art := d.session.ChampionArt[d.champ.Key]
	s := art.ASCII
	if d.artMode == 2 {
		s = art.Mono
	}
	if s == "" || d.width < 70 {
		return ""
	}
	return stPanel.Render(keepPanelBG(s))
}

// rolesBadges pinta los roles como píldoras clicables con la activa
// resaltada y la pista del atajo al lado. Con un solo rol no se
// muestra nada.
func (d detailView) rolesBadges() string {
	if !(len(d.champ.Roles) > 1) || d.champ.Roles[0] == model.RoleAll {
		return ""
	}
	parts := make([]string, 0, len(d.champ.Roles))
	for i, r := range d.champ.Roles {
		var pill string
		if i == d.roleIdx {
			pill = stKeyName.Background(colBorder).Render(" " + r.String() + " ")
		} else {
			pill = stDim.Render(" " + r.String() + " ")
		}
		parts = append(parts, zoneMgr.Mark(fmt.Sprintf("role:%d", i), pill))
	}
	return strings.Join(parts, " ") + "  " + stHelp.Render("r cambiar")
}

// tabsView: pestañas que parecen pestañas — la activa en dorado con
// subrayado y su número de atajo visible.
func (d detailView) tabsView() string {
	tabOn := lipgloss.NewStyle().Bold(true).Foreground(colGold).Underline(true)
	tabOff := lipgloss.NewStyle().Foreground(colDim)
	// En terminales angostas la barra completa no cabe: abreviar y
	// cerrar el separador.
	names := d.tabs()
	sep := "   "
	short := map[string]string{"Resumen": "Resum.", "Habilidades": "Habil.", "Matchups": "Match."}
	if lipgloss.Width(strings.Join(names, sep)) > d.width-4 {
		sep = "  "
		for i, n := range names {
			if s, ok := short[n]; ok {
				names[i] = s
			}
		}
	}
	// Última reserva para terminales muy estrechas: si aun abreviado no
	// cabe, se muestra solo la pestaña activa con su posición. Nunca se
	// desborda el ancho (la TUI debe funcionar en terminales pequeñas).
	barWidth := 0
	for i, name := range names {
		barWidth += len([]rune(fmt.Sprintf("%d %s", i+1, name)))
	}
	barWidth += (len(names) - 1) * len([]rune(sep))
	if barWidth+2 > d.width-4 {
		label := fmt.Sprintf("%d/%d %s", d.tab+1, len(names), shortTabName(names[d.tab]))
		return lipgloss.NewStyle().Padding(0, 1).Render(tabOn.Render(label))
	}
	parts := make([]string, 0, len(names))
	for i, name := range names {
		label := fmt.Sprintf("%d %s", i+1, name)
		var rendered string
		if i == d.tab {
			rendered = tabOn.Render(label)
		} else {
			rendered = tabOff.Render(label)
		}
		parts = append(parts, zoneMgr.Mark(fmt.Sprintf("tab:%d", i), rendered))
	}
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, sep))
}

// tabView despacha a la pestaña activa. Un campeón sin build (rol no
// canónico) no tiene ninguna: se avisa en vez de desreferenciar nil.
func (d detailView) tabView() string {
	if d.CurrentBuild() == nil {
		return stDim.Render("sin datos de build para este campeón en este juego")
	}
	// Despacho por índice canónico. Las etiquetas se abrevian para que
	// siete pestañas quepan en 80 columnas (`Habil.`, `Match.`), así que
	// comparar por texto dejaba las dos últimas cayendo en el caso por
	// defecto y mostraban la misma vista.
	//
	// LoL:  0 Ítems · 1 Ruta · 2 Alt. · 3 Resumen · 4 Runas · 5 Habil. · 6 Match.
	// DL:   0 Ítems · 1 Resumen · 2 Matchups · 3 Habilidades
	if d.session.Game == model.GameDeadlock {
		switch d.tab {
		case 0:
			return d.itemsView()
		case 1:
			return d.resumenView()
		case 2:
			return d.matchupsView()
		default:
			return d.skillsView()
		}
	}
	switch d.tab {
	case 0:
		return d.itemsView()
	case 1:
		return d.routeTabView()
	case 2:
		return d.alternativesTabView()
	case 3:
		return d.resumenView()
	case 4:
		return d.runesView()
	case 5:
		return d.skillsView()
	default:
		return d.matchupsView()
	}
}

// ─── pestaña ítems ─────────────────────────────────────────────────

// itemsView: tarjetas unificadas (icono + nombre + WR + pick) en
// secciones CORE / SITUACIONALES / BOTAS, con foco visible y línea de
// detalle de la tarjeta seleccionada. Al pie, la leyenda de umbrales
// de WR para que la escala de color no sea un secreto.
func (d detailView) itemsView() string {
	cards := d.cardList()
	if len(cards) == 0 {
		message := stDim.Render("sin ítems con muestra suficiente (filtro: ≥100 partidas o ≥20 con ≥5% pick)")
		if d.session.Game == model.GameDeadlock {
			if status := d.routeStatus(); status != "" {
				return stDim.Render(status) + "\n" + message
			}
		}
		return message
	}
	sections, _ := d.itemsSections(cards)
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// itemsSections devuelve las secciones de la pestaña de ítems y, para
// cada tarjeta, la línea (relativa al contenido) donde empieza. Las
// alturas salen del render real de cada fila, no de constantes a mano:
// así el auto-scroll del viewport no se desvía al cambiar el ancho.
func (d detailView) itemsSections(cards []cardItem) (sections []string, starts []int) {
	if d.session.Game == model.GameDeadlock {
		return d.itemsSectionsDL(cards)
	}
	b := d.CurrentBuild()
	perRow := d.cardsPerRow()
	starts = make([]int, len(cards))
	line := 0
	// Resumen jugable antes de las tarjetas: hechizos y orden de
	// compra medido, lo primero que se mira para jugar.
	for _, extra := range [][]string{d.spellsSection(), d.orderSection()} {
		if len(extra) == 0 {
			continue
		}
		sections = append(sections, extra...)
		for _, s := range extra {
			line += lipgloss.Height(s)
		}
	}
	split := func(items []cardItem, offset int, title string) {
		if len(items) == 0 {
			return
		}
		rows := make([]string, 0, (len(items)+perRow-1)/perRow)
		for i := 0; i < len(items); i += perRow {
			end := min(i+perRow, len(items))
			rowCards := make([]string, 0, end-i)
			for j := i; j < end; j++ {
				rowCards = append(rowCards, zoneMgr.Mark(fmt.Sprintf("card:%d", offset+j), d.itemCard(offset+j, cards[offset+j])))
			}
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rowCards...))
		}
		// +1 por el título de sección; cada fila mide lo que mida de
		// verdad (tarjeta con o sin tier/coste).
		rowH := lipgloss.Height(rows[0])
		for i := 0; i < len(items); i += perRow {
			base := line + 1 + (i/perRow)*rowH
			for j := i; j < min(i+perRow, len(items)); j++ {
				starts[offset+j] = base
			}
		}
		sections = append(sections,
			stSection.Render("◆ "+title),
			strings.Join(rows, "\n"))
		line += 1 + len(rows)*rowH
	}
	nCore := len(b.CoreItems)
	coreEnd := min(nCore, 3)
	core := cards[:coreEnd]
	situ := cards[coreEnd:nCore] // vacío cuando hay 3 o menos core
	boots := cards[nCore:]
	ward := cards[:0]
	if n := len(cards); n > 0 && cards[n-1].ward {
		ward = cards[n-1:]
		boots = cards[nCore : n-1]
	}
	split(core, 0, "CORE")
	split(situ, coreEnd, "SITUACIONALES")
	split(boots, nCore, "BOTAS")
	// Botas alternativas plegadas: una línea informativa en vez de
	// tarjetas que nadie compra.
	if r := d.foldedBootsResume(); r != "" {
		sections = append(sections, stDim.Render("  "+r))
	}
	// El mejor ward, en su propia sección.
	split(ward, len(cards)-1, "WARD")
	return sections, starts
}

// itemsFooter es el pie fijo de la pestaña de ítems: la línea de
// selección (con el nombre completo, que la tarjeta trunca), la
// leyenda de umbrales y —cuando la fuente los trae— el WR del combo
// real de 3 ítems y el orden de compra medido por turnos. Vive fuera
// del viewport para que no se pierda al hacer scroll. Vacío en las
// demás pestañas.
func (d detailView) itemsFooter() string {
	if d.tab != 0 || len(d.cardList()) == 0 || d.showItem {
		return ""
	}
	// La línea de selección y la leyenda envuelven por palabras al
	// ancho disponible.
	if d.session.Game == model.GameDeadlock {
		return d.selectionLine()
	}
	inner := max(24, d.width-10)
	wrap := lipgloss.NewStyle().Width(inner)
	lines := []string{
		wrap.Render(d.selectionLine()),
		wrap.Render(wrLegend()),
	}
	if b := d.CurrentBuild(); b != nil {
		if len(b.ItemCombos) > 0 {
			lines = append(lines, wrap.Render(comboLine(d.session.ItemName, b.ItemCombos[0])))
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// comboLine describe el trío con más presencia: lo que tres WR sueltos
// no pueden decir, el WR de comprarlos juntos.
func comboLine(name func(int) string, c model.ItemCombo) string {
	names := make([]string, 0, len(c.IDs))
	for _, id := range c.IDs {
		names = append(names, name(id))
	}
	return fmt.Sprintf("combo: %s · WR %s · %s partidas",
		strings.Join(names, " + "), pct(c.WinRate()), nThousand(c.Games))
}

// orderEntries arma la lista deduplicada del orden de compra medido
// (item1..item5): el ítem más usado como enésima compra que aún no se
// haya recomendado; el turno cubierto muestra su siguiente opción y el
// turno agotado se omite.
func orderEntries(slots [][]model.ItemStats) []model.ItemStats {
	used := map[int]bool{}
	var out []model.ItemStats
	for _, slot := range slots {
		for _, it := range slot {
			if used[it.ID] || it.ID == 0 {
				continue
			}
			used[it.ID] = true
			out = append(out, it)
			break
		}
	}
	return out
}

// orderBlock renderiza el orden de compra medido: 1.º X 11' → 2.º Y 19'.
func (d detailView) orderBlock() string {
	entries := orderEntries(d.CurrentBuild().ItemSlots)
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for i, it := range entries {
		s := fmt.Sprintf("%d.º %s", i+1, stKeyName.Render(d.session.ItemName(it.ID)))
		if it.AvgBuyS > 0 {
			s += stDim.Render(fmt.Sprintf(" %d'", int(it.AvgBuyS/60+0.5)))
		}
		parts = append(parts, s)
	}
	inner := max(24, d.width-10)
	return lipgloss.NewStyle().Width(inner).Render(strings.Join(parts, stDim.Render(" → ")))
}

// spellsLine resume el par de hechizos más usado del campeón/rol.
func (d detailView) spellsLine() string {
	b := d.CurrentBuild()
	if b == nil || len(b.Spells) == 0 {
		return ""
	}
	top := b.Spells[0]
	return fmt.Sprintf("%s · WR %s · %s partidas",
		d.session.SpellPairName(top.IDs), pct(top.WinRate()), nThousand(top.Games))
}

// spellsSection: título y línea del par de hechizos más usado.
func (d detailView) spellsSection() []string {
	if s := d.spellsLine(); s != "" {
		return []string{stSection.Render("◆ HECHIZOS"), s}
	}
	return nil
}

// orderSection: título y línea del orden de compra medido.
func (d detailView) orderSection() []string {
	if s := d.orderBlock(); s != "" {
		return []string{stSection.Render("◆ ORDEN DE COMPRA"), s}
	}
	return nil
}

// routeStatus distinguishes an in-flight request from an unavailable route.
func (d detailView) routeStatus() string {
	// La build híbrida manda cuando existe; si está en curso, se dice.
	if d.session.HybridLoader != nil {
		if hb, requested := d.session.Hybrids[d.champ.Key]; requested {
			if hb != nil {
				return ""
			}
			return "Calculando build…"
		}
	}
	steps, requested := d.session.Routes[d.champ.Key]
	if len(steps) > 0 {
		return ""
	}
	if (requested && steps == nil) || (!requested && d.session.Runs != nil) {
		return "Cargando ruta de compra…"
	}
	// La ausencia tiene causas distintas: se dice cuál en vez de un
	// texto único. El detalle técnico queda en los logs.
	if why, ok := d.session.RouteWhy[d.champ.Key]; ok {
		if s := why.short(); s != "" {
			return "Ruta no disponible (" + s + ") — mostrando estadísticas generales"
		}
	}
	return "Ruta no disponible — mostrando estadísticas generales"
}

// dlRouteHeader builds the compact single-line header shown before the
// first phase (EARLY). Format adapts to terminal width:
//
//	wide:   "Ruta observada · 137 partidas · aparición ≥30 %"
//	narrow: "Ruta observada · 137 partidas"
//	no route: ""
func (d detailView) dlRouteHeader() string {
	steps := d.session.Routes[d.champ.Key]
	if len(steps) == 0 {
		return ""
	}
	st := steps[0]
	// Always show at minimum: "Ruta observada · N partidas"
	parts := []string{"Ruta observada"}
	if st.SampleSize > 0 {
		parts = append(parts, fmt.Sprintf("%d partidas", st.SampleSize))
	}
	header := strings.Join(parts, " · ")
	// Append threshold only when it fits (terminals ≥50 cols).
	if st.MinShare > 0 && d.width >= 50 {
		threshold := fmt.Sprintf("aparición ≥%g%%", st.MinShare*100)
		full := header + " · " + threshold
		if lipgloss.Width(full) <= d.width-4 {
			header = full
		}
	}
	return stDim.Render(header)
}

// hasHybridCards informa si las tarjetas vienen de la build híbrida.
func (d detailView) hasHybridCards(cards []cardItem) bool {
	return len(cards) > 0 && cards[0].hybrid
}

// hybridSections pinta la build híbrida: la ruta estándar agrupada por
// fase (EARLY, MID, LATE), luego AMPLIACIONES y SITUACIONALES. Las
// tarjetas conservan su número de paso continuo; las ampliaciones y
// situacionales no llevan número porque no son parte de la ruta.
func (d detailView) hybridSections(cards []cardItem) (sections []string, starts []int) {
	perRow := d.cardsPerRow()
	starts = make([]int, len(cards))
	line := 0

	emit := func(title string, start, end int) {
		var rows []string
		line += lipgloss.Height(title)
		for i := start; i < end; i += perRow {
			rowEnd := min(i+perRow, end)
			var rowCards []string
			for j := i; j < rowEnd; j++ {
				rowCards = append(rowCards, zoneMgr.Mark(fmt.Sprintf("card:%d", j), d.itemCard(j, cards[j])))
				starts[j] = line
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top, rowCards...)
			rows = append(rows, row)
			line += lipgloss.Height(row)
		}
		sections = append(sections, stSection.Render(title), strings.Join(rows, "\n"))
	}

	// emitIndices emite una sección con un subconjunto arbitrario de
	// tarjetas (no contiguo). Se usa para agrupar situacionales por
	// función sin reordenar la lista de navegación.
	emitIndices := func(title string, cards []cardItem, idx []int) {
		var rows []string
		line += lipgloss.Height(title)
		for i := 0; i < len(idx); i += perRow {
			end := min(i+perRow, len(idx))
			var rowCards []string
			for j := i; j < end; j++ {
				k := idx[j]
				rowCards = append(rowCards, zoneMgr.Mark(fmt.Sprintf("card:%d", k), d.itemCard(k, cards[k])))
				starts[k] = line
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top, rowCards...)
			rows = append(rows, row)
			line += lipgloss.Height(row)
		}
		sections = append(sections, stSection.Render(title), strings.Join(rows, "\n"))
	}

	// emitFiltered emite las tarjetas de [start,end) cuya función sea fn.
	emitFiltered := func(title string, cards []cardItem, start, end int, fn string) {
		var idx []int
		for i := start; i < end; i++ {
			if cards[i].hybridReason == fn {
				idx = append(idx, i)
			}
		}
		if len(idx) == 0 {
			return
		}
		emitIndices(title, cards, idx)
	}

	// Cabecera compacta: una sola línea, sin explicaciones largas.
	if hdr := d.hybridHeader(); hdr != "" {
		sections = append(sections, hdr, "")
		line += lipgloss.Height(hdr)
	}

	// Ruta estándar por fase, en el orden en que llegan las tarjetas.
	phase := -1
	for start := 0; start < len(cards); {
		if cards[start].hybridSection != SectionStandard {
			break
		}
		if p := cards[start].hybridPhase; p > phase {
			phase = p
		}
		end := start + 1
		for end < len(cards) {
			if cards[end].hybridSection != SectionStandard {
				break
			}
			if cards[end].hybridPhase > phase {
				break
			}
			end++
		}
		emit("◆ "+dlPhaseNames[min(phase, len(dlPhaseNames)-1)], start, end)
		start = end
	}

	// Ampliaciones: una sola sección, ya vienen en orden cronológico.
	if start, end := sectionRange(cards, SectionExtensions); start >= 0 {
		emit("◆ AMPLIACIONES", start, end)
	}

	// Situacionales agrupados por función: la etiqueta ya existe y es
	// verificable; agruparla convierte una lista larga en categorías
	// escaneables. El orden de los grupos es fijo (no alfabético) para
	// que la posición sea estable entre héroes.
	if start, end := sectionRange(cards, SectionSituationals); start >= 0 {
		for _, fn := range situationalOrder {
			gStart, gEnd := -1, -1
			for i := start; i < end; i++ {
				if cards[i].hybridReason != fn {
					continue
				}
				if gStart < 0 {
					gStart = i
				}
				gEnd = i + 1
			}
			if gStart < 0 {
				continue
			}
			// Un grupo puede tener huecos si el orden mezcla funciones:
			// se emiten solo las tarjetas de esa función.
			emitFiltered("◆ SITUACIONALES · "+fn, cards, gStart, gEnd, fn)
		}
		// Objetos sin función reconocida: se listan al final, sin
		// inventarles categoría.
		var rest []int
		for i := start; i < end; i++ {
			known := false
			for _, fn := range situationalOrder {
				if cards[i].hybridReason == fn {
					known = true
					break
				}
			}
			if !known {
				rest = append(rest, i)
			}
		}
		if len(rest) > 0 {
			emitIndices("◆ SITUACIONALES", cards, rest)
		}
	}
	return sections, starts
}

// situationalOrder fija el orden de los grupos de situacionales.
var situationalOrder = []string{
	"Anticuración",
	"Defensa contra balas",
	"Defensa espiritual",
	"Resistencia al control",
	"Supervivencia",
	"Daño adicional",
}

// sectionRange devuelve el rango contiguo de tarjetas de una sección.
func sectionRange(cards []cardItem, kind HybridSection) (start, end int) {
	start = -1
	for i, c := range cards {
		if c.hybridSection == kind {
			start = i
			break
		}
	}
	if start < 0 {
		return -1, -1
	}
	end = start
	for end < len(cards) && cards[end].hybridSection == kind {
		end++
	}
	return start, end
}

// hybridHeader es la línea de contexto de la build híbrida: confianza
// global y objetos finales. Corta y sin jerga estadística.
func (d detailView) hybridHeader() string {
	hb := d.session.Hybrids[d.champ.Key]
	if hb == nil {
		return ""
	}
	parts := []string{"Build híbrida"}
	if hb.FinalItems > 0 {
		parts = append(parts, fmt.Sprintf("%d objetos finales", hb.FinalItems))
	}
	parts = append(parts, "confianza "+ConfidenceWord(hb.Confidence))
	hdr := strings.Join(parts, " · ")
	if lipgloss.Width(hdr) > d.width-4 {
		hdr = "Build híbrida · confianza " + ConfidenceWord(hb.Confidence)
	}
	return stDim.Render(hdr)
}

// ConfidenceWord traduce la confianza a una palabra. Nunca se presenta
// como probabilidad de victoria.
func ConfidenceWord(c float64) string {
	switch {
	case c >= 0.70:
		return "alta"
	case c >= 0.45:
		return "moderada"
	default:
		return "baja"
	}
}

// itemsSectionsDL preserves route order; phase headings only annotate
// contiguous time bands, never move an action ahead of another route
// step. A step whose own minute falls in an earlier band stays with
// the current band (no EARLY reopens below MID): the route is the
// story, the phase is a reading aid. Items outside the route close the
// list in one section of their own.
//
// A single compact header appears before the first phase; the extended
// provenance (sample differences, "Fuera de la ruta" explanation) is
// available in the item popup (Enter) and the existing help.
func (d detailView) itemsSectionsDL(cards []cardItem) (sections []string, starts []int) {
	// Con build híbrida, las secciones son las de la build (EARLY, MID,
	// LATE, AMPLIACIONES, SITUACIONALES), no las de la ruta observada.
	if d.hasHybridCards(cards) {
		return d.hybridSections(cards)
	}
	perRow := d.cardsPerRow()
	starts = make([]int, len(cards))
	line := 0
	withRoute := len(d.session.Routes[d.champ.Key]) > 0

	emit := func(title string, start, end int, outside bool) {
		// activeSection expects alternating title/body pairs.
		var rows []string
		line += lipgloss.Height(title)
		if outside {
			desc := lipgloss.NewStyle().Width(max(24, d.width-10)).Render(
				stDim.Render("Compras registradas en las estadísticas generales que no forman parte de esta ruta"))
			rows = append(rows, desc)
			line += lipgloss.Height(desc)
		}
		for i := start; i < end; i += perRow {
			rowEnd := min(i+perRow, end)
			var rowCards []string
			for j := i; j < rowEnd; j++ {
				rowCards = append(rowCards, zoneMgr.Mark(fmt.Sprintf("card:%d", j), d.itemCard(j, cards[j])))
				starts[j] = line
			}
			row := lipgloss.JoinHorizontal(lipgloss.Top, rowCards...)
			rows = append(rows, row)
			line += lipgloss.Height(row)
		}
		sections = append(sections, stSection.Render(title), strings.Join(rows, "\n"))
	}

	if !withRoute {
		title := stDim.Render(d.routeStatus()) + "\n" + stSection.Render("◆ Compras frecuentes")
		emit(title, 0, len(cards), false)
		return sections, starts
	}

	// Single compact header before the first phase.
	if hdr := d.dlRouteHeader(); hdr != "" {
		sections = append(sections, hdr, "")
		line += lipgloss.Height(hdr)
	}

	disclaimerEmitted := false
	emitWithDisclaimer := func(title string, start, end int, outside bool) {
		if !disclaimerEmitted {
			disclaimerEmitted = true
			var rows []string
			line += lipgloss.Height(title)

			disc := lipgloss.NewStyle().Width(max(24, d.width-10)).Render(
				stDim.Render("Las estadísticas generales y la ruta pueden proceder de muestras diferentes"))
			rows = append(rows, disc)
			line += lipgloss.Height(disc)
			if outside {
				out := lipgloss.NewStyle().Width(max(24, d.width-10)).Render(
					stDim.Render("Compras registradas en las estadísticas generales que no forman parte de esta ruta"))
				rows = append(rows, out)
				line += lipgloss.Height(out)
			}
			for i := start; i < end; i += perRow {
				rowEnd := min(i+perRow, end)
				var rowCards []string
				for j := i; j < rowEnd; j++ {
					rowCards = append(rowCards, zoneMgr.Mark(fmt.Sprintf("card:%d", j), d.itemCard(j, cards[j])))
					starts[j] = line
				}
				row := lipgloss.JoinHorizontal(lipgloss.Top, rowCards...)
				rows = append(rows, row)
				line += lipgloss.Height(row)
			}
			sections = append(sections, stSection.Render(title), strings.Join(rows, "\n"))
			return
		}
		emit(title, start, end, outside)
	}

	phase := -1
	for start := 0; start < len(cards); {
		if withRoute && cards[start].order == 0 {
			emit("◆ Fuera de la ruta", start, len(cards), true)
			break
		}
		if p := dlPhaseOf(cards[start].dlMinuteOf()); p > phase {
			phase = p
		}
		end := start + 1
		for end < len(cards) {
			if withRoute && cards[end].order == 0 {
				break
			}
			if dlPhaseOf(cards[end].dlMinuteOf()) > phase {
				break
			}
			end++
		}
		emitWithDisclaimer("◆ "+dlPhaseNames[phase], start, end, false)
		start = end
	}
	return sections, starts
}

// itemCard keeps the LoL layout separate from the compact Deadlock route.
func (d detailView) itemCard(idx int, c cardItem) string {
	if d.session.Game == model.GameDeadlock {
		return d.dlItemCard(idx, c)
	}
	it := c.item
	name := d.session.ItemName(it.ID)
	// Delta contra el WR del campeón, no WR absoluto: un 79.9% en un
	// héroe al 52.7% es verde absoluto y sesgo de bola de nieve; el
	// WR real sigue en la línea de detalle.
	base := d.CurrentBuild().WinRate
	delta := (it.WinRate - base) * 100
	meta := deltaStyle(delta).Render(fmt.Sprintf("%+.1f", delta)) + snowMark(delta) +
		" " + stDim.Render("p"+pct(it.PickRate))
	icon := d.session.ItemArt[it.ID]
	if icon == "" {
		icon = d.fallbackIcon(name)
	}
	// Nombre en hasta dos líneas por palabra: la rejilla deja de
	// truncar con "…".
	nameLines := wrapName(name, d.cardW()-6)
	for i := range nameLines {
		nameLines[i] = stKeyName.Render(nameLines[i])
	}
	var body []string
	if !d.compactMode() {
		body = append(body, icon)
	}
	body = append(body, nameLines...)
	body = append(body, meta)
	border := lipgloss.Color("#1f2937")
	if idx == d.itemIdx {
		border = colBorder // el foco pisa el color de categoría
	}
	box := lipgloss.RoundedBorder()
	return lipgloss.NewStyle().
		Width(d.cardW()).Align(lipgloss.Center).Padding(0, 1).
		Border(box).BorderForeground(border).
		Render(lipgloss.JoinVertical(lipgloss.Center, body...))
}

// wrapName parte el nombre en hasta dos líneas al ancho dado, por
// palabra: la rejilla deja de truncar con "…".
func wrapName(s string, w int) []string {
	if runewidth.StringWidth(s) <= w {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) < 2 {
		return []string{truncate(s, w)}
	}
	first := words[0]
	i := 1
	for ; i < len(words); i++ {
		if runewidth.StringWidth(first+" "+words[i]) > w {
			break
		}
		first += " " + words[i]
	}
	if i >= len(words) {
		return []string{truncate(s, w)}
	}
	return []string{first, truncate(strings.Join(words[i:], " "), w)}
}

// deltaAviso es el delta de WR (puntos) desde el que un ítem huele a
// sesgo de bola de nieve: con un WR muy por encima del campeón, la
// muestra viene mayormente de partidas yendo por delante.
const deltaAviso = 10.0

// snowMark marca con ⚠ los ítems con WR muy por encima del campeón.
func snowMark(delta float64) string {
	if delta >= deltaAviso {
		return " ⚠"
	}
	return ""
}

// compactMode achata las tarjetas (sin icono) cuando el viewport es
// bajo: sin esto, tres secciones de tarjetas completas no entran y el
// usuario termina viendo solo la mitad de la lista.
func (d detailView) compactMode() bool {
	return d.vp.Height > 0 && d.vp.Height < 28
}

// deltaStyle colorea la diferencia de WR sobre el héroe (puntos).
// Con |Δ| < 1 la diferencia es ruido: gris tenue, para que lo que
// importa (los verdes y rojos) destaque sin esfuerzo.
func deltaStyle(delta float64) lipgloss.Style {
	switch {
	case delta >= 1.5:
		return lipgloss.NewStyle().Foreground(colWrGreat).Bold(true)
	case delta >= 1.0:
		return lipgloss.NewStyle().Foreground(colWrGood)
	case delta > -1.0:
		return lipgloss.NewStyle().Foreground(colDim)
	default:
		return lipgloss.NewStyle().Foreground(colWrBad)
	}
}

// catColors: un color por categoría de tienda de Deadlock.
var catColors = map[string]lipgloss.Color{
	"weapon":   lipgloss.Color("#e8a33d"),
	"vitality": lipgloss.Color("#6bbf59"),
	"spirit":   lipgloss.Color("#b072e8"),
}

func catColorStyle(cat string) lipgloss.Style {
	if col, ok := catColors[cat]; ok {
		return lipgloss.NewStyle().Foreground(col).Bold(true)
	}
	return stKeyName
}

// romanTier: I..IV (y V si algún día llega); "?" si se desconoce.
func romanTier(tier int) string {
	romans := []string{"?", "I", "II", "III", "IV", "V"}
	if tier >= 0 && tier < len(romans) {
		return romans[tier]
	}
	return "?"
}

// fallbackIcon: recuadro con la inicial cuando falta el PNG, para que
// la tarjeta no se descuadre.
func (d detailView) fallbackIcon(name string) string {
	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}
	return lipgloss.NewStyle().
		Width(12).Height(4).AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center).
		Border(lipgloss.RoundedBorder()).BorderForeground(colDim).
		Foreground(colDim).Bold(true).
		Render(initial)
}

// selectionLine: nombre completo de la tarjeta seleccionada (la rejilla
// trunca; aquí no hace falta).
func (d detailView) selectionLine() string {
	cards := d.cardList()
	if d.itemIdx >= len(cards) {
		return ""
	}
	c := cards[d.itemIdx]
	name := d.session.ItemName(c.item.ID)
	if c.boot {
		name = stDim.Render("[botas] ") + name
	}
	if c.ward {
		name = stDim.Render("[ward] ") + name
	}
	if d.session.Game == model.GameDeadlock {
		return d.dlSelectionLine(c)
	}
	line := fmt.Sprintf("▸ %s · WR %s · pick %s · %s partidas",
		stKeyName.Render(name),
		wrStyle(c.item.WinRate, c.item.Games).Render(pct(c.item.WinRate)),
		stDim.Render(pct(c.item.PickRate)),
		stDim.Render(nThousand(c.item.Games)))
	if delta := (c.item.WinRate - d.CurrentBuild().WinRate) * 100; delta >= deltaAviso {
		line += stDim.Render("  · ⚠ se compra yendo por delante")
	}
	if c.item.Games < lowSampleGames {
		line += stDim.Render("  (muestra baja)")
	}
	return line
}

// ─── pestaña resumen ────────────────────────────────────────────────

// resumenView: una pantalla con lo que hace falta para sentarse a
// jugar: hechizos, ítems iniciales, runa clave, orden de compra,
// botas y prioridad de habilidades. Solo muestra lo que la fuente
// trae; sin datos no dibuja la sección.
func (d detailView) resumenView() string {
	if d.CurrentBuild() == nil {
		return stDim.Render("sin datos de build")
	}
	var sec []string
	add := func(title, body string) {
		if body == "" {
			return
		}
		sec = append(sec, stSection.Render("◆ "+title), body)
	}
	add("HECHIZOS", d.spellsLine())
	add("INICIO", d.startItemsLine())
	add("RUNA CLAVE", d.keyRuneLine())
	add("ORDEN DE COMPRA", d.orderBlock())
	add("BOTAS", d.bootsLine())
	add("HABILIDADES", d.priorityLine())
	add("ÍTEMS", d.coreTopLine())
	if len(sec) == 0 {
		return stDim.Render("sin datos de build")
	}
	return lipgloss.JoinVertical(lipgloss.Left, sec...)
}

// startItemsLine nombra los ítems iniciales más comprados.
func (d detailView) startItemsLine() string {
	b := d.CurrentBuild()
	if len(b.StartItems) == 0 {
		return ""
	}
	parts := make([]string, 0, min(3, len(b.StartItems)))
	for _, it := range b.StartItems[:min(3, len(b.StartItems))] {
		parts = append(parts, stKeyName.Render(d.session.ItemName(it.ID)))
	}
	return strings.Join(parts, stDim.Render(" + "))
}

// keyRuneLine resume la página de runas más jugada.
func (d detailView) keyRuneLine() string {
	b := d.CurrentBuild()
	if len(b.RunePages) == 0 {
		return ""
	}
	p := b.RunePages[0]
	return fmt.Sprintf("%s · %s / %s · WR %s · %s partidas",
		stKeyName.Render(d.session.RuneName(p.KeystoneID)),
		d.session.TreeName(p.PrimaryTree), d.session.TreeName(p.SecondaryTree),
		wrStyle(p.WinRate, p.Games).Render(pct(p.WinRate)), nThousand(p.Games))
}

// bootsLine resume la bota recomendada.
func (d detailView) bootsLine() string {
	b := d.CurrentBuild()
	if len(b.Boots) == 0 {
		return ""
	}
	bt := b.Boots[0]
	return fmt.Sprintf("%s · WR %s · pick %s · %s partidas",
		stKeyName.Render(d.session.ItemName(bt.ID)),
		wrStyle(bt.WinRate, bt.Games).Render(pct(bt.WinRate)),
		stDim.Render(pct(bt.PickRate)), stDim.Render(nThousand(bt.Games)))
}

// coreTopLine nombra los 3 ítems más comprados (resumen de Deadlock).
func (d detailView) coreTopLine() string {
	b := d.CurrentBuild()
	if d.session.Game != model.GameDeadlock || len(b.CoreItems) == 0 {
		return ""
	}
	parts := make([]string, 0, min(3, len(b.CoreItems)))
	for _, it := range b.CoreItems[:min(3, len(b.CoreItems))] {
		parts = append(parts, stKeyName.Render(d.session.ItemName(it.ID)))
	}
	return strings.Join(parts, stDim.Render(" → "))
}

// ─── pestaña matchups (Deadlock y LoL) ──────────────────────────────

// minMatchupGames filtra cruces sin muestra significativa.
const minMatchupGames = 200

// matchupsView: contra quién rinde peor y mejor (WR del enfrentamiento)
// y con quién sinergia, en tres columnas como U.GG.
func (d detailView) matchupsView() string {
	b := d.CurrentBuild()
	if len(b.Matchups) == 0 && len(b.Synergies) == 0 {
		if d.session.Game == model.GameDeadlock {
			return stDim.Render("la fuente no publica enfrentamientos para este héroe")
		}
		return stDim.Render("la fuente no publica enfrentamientos por campeón en este rol")
	}
	nameOf := d.session.Names.HeroNameByID
	usable := func(refs []model.HeroRef) []model.HeroRef {
		var out []model.HeroRef
		for _, r := range refs {
			if r.Games >= minMatchupGames {
				out = append(out, r)
			}
		}
		return out
	}
	worstRef := usable(b.Matchups)
	// Ordenar por el WR crudo premia los cruces de 200 partidas con
	// rachas: se usa el límite inferior de Wilson, igual que en el
	// resto de la app. "DIFÍCILES" ordena por la tasa de derrota.
	sort.Slice(worstRef, func(i, j int) bool {
		a, b := worstRef[i], worstRef[j]
		return clean.WilsonLower(a.Games-a.Wins, a.Games) > clean.WilsonLower(b.Games-b.Wins, b.Games)
	})
	bestRef := append([]model.HeroRef(nil), worstRef...)
	sort.Slice(bestRef, func(i, j int) bool {
		a, b := bestRef[i], bestRef[j]
		return clean.WilsonLower(a.Wins, a.Games) > clean.WilsonLower(b.Wins, b.Games)
	})
	synRef := usable(b.Synergies)
	sort.Slice(synRef, func(i, j int) bool {
		a, b := synRef[i], synRef[j]
		return clean.WilsonLower(a.Wins, a.Games) > clean.WilsonLower(b.Wins, b.Games)
	})

	list := func(title string, refs []model.HeroRef) string {
		var lines []string
		lines = append(lines, stSection.Render("◆ "+title))
		if len(refs) == 0 {
			lines = append(lines, stDim.Render("sin muestra"))
		}
		for _, r := range refs[:min(5, len(refs))] {
			name := nameOf[r.HeroID]
			if name == "" {
				name = fmt.Sprintf("Héroe %d", r.HeroID)
			}
			lines = append(lines, fmt.Sprintf("• %-14s %s %s",
				truncate(name, 14),
				wrStyle(r.WinRate(), r.Games).Render(pct(r.WinRate())),
				stDim.Render(nThousand(r.Games))))
		}
		return stPanel.Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		list("DIFÍCILES", worstRef), " ",
		list("FÁCILES", bestRef), " ",
		list("SINERGIAS", synRef))
}

// ─── pestaña runas ─────────────────────────────────────────────────

// runesView: keystone protagonista, resto en dos columnas (árbol
// primario / secundario + fragmentos) y la alternativa al pie.
func (d detailView) runesView() string {
	b := d.CurrentBuild()
	if len(b.RunePages) == 0 {
		return stDim.Render("sin runas con muestra suficiente")
	}
	p := b.RunePages[0]
	head := lipgloss.JoinVertical(lipgloss.Left,
		stSection.Render("◆ "+d.session.RuneName(p.KeystoneID)),
		stDim.Render(fmt.Sprintf("%s / %s · WR %s · %s partidas",
			stValue.Render(d.session.TreeName(p.PrimaryTree)),
			stValue.Render(d.session.TreeName(p.SecondaryTree)),
			wrStyle(p.WinRate, p.Games).Render(pct(p.WinRate)),
			nThousand(p.Games),
		)))
	bullet := func(ids []int) []string {
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			out = append(out, "• "+d.session.RuneName(id))
		}
		return out
	}
	prim := lipgloss.JoinVertical(lipgloss.Left, append([]string{
		stValue.Render(d.session.TreeName(p.PrimaryTree))}, bullet(p.PrimaryIDs)...)...)
	secLines := append([]string{stValue.Render(d.session.TreeName(p.SecondaryTree))},
		bullet(p.SecondaryIDs)...)
	shards := make([]string, 0, len(p.Shards))
	for _, id := range p.Shards {
		shards = append(shards, d.session.ShardName(id))
	}
	secLines = append(secLines, "", stDim.Render("fragmentos: "+strings.Join(shards, " · ")))
	sec := lipgloss.JoinVertical(lipgloss.Left, secLines...)
	cols := lipgloss.JoinHorizontal(lipgloss.Top,
		stPanel.Render(prim), "  ", stPanel.Render(sec))
	out := lipgloss.JoinVertical(lipgloss.Left, head, "", cols)
	if len(b.RunePages) > 1 {
		alt := b.RunePages[1]
		out = lipgloss.JoinVertical(lipgloss.Left, out,
			stDim.Render(fmt.Sprintf("alt: %s (%s partidas)",
				d.session.RuneName(alt.KeystoneID), nThousand(alt.Games))))
	}
	return out
}

// emptySkillsReason distingue POR QUÉ no hay rejilla de habilidades:
// la fuente no publica órdenes, la muestra no llega al umbral, o los
// datos existen pero la secuencia no es válida con las reglas del juego
// (y por eso no se puede derivar la prioridad). Un mensaje único
// "sin muestra suficiente" ocultaba casos muy distintos.
func (d detailView) emptySkillsReason(b *model.CleanBuild) string {
	if b == nil {
		return "sin datos de build"
	}
	// ¿La fuente trajo algo? El planificador recibe la lista cruda, así
	// que una build sin secuencia y sin prioridad viene de un descarte.
	if b.SkillOrder.Sequence == "" && len(b.SkillOrder.Priority) == 0 {
		if d.session.Game == model.GameDeadlock {
			return "la fuente no publica órdenes de habilidades para este héroe"
		}
		return "la fuente no publica una secuencia de habilidades válida para este campeón"
	}
	return "sin secuencia con muestra suficiente"
}

// ─── pestaña habilidades ───────────────────────────────────────────

// skillsView: rejilla habilidad × nivel con marcas ◆, prioridad y
// leyenda (Deadlock nombra las habilidades del kit).
func (d detailView) skillsView() string {
	b := d.CurrentBuild()
	seq := []rune(b.SkillOrder.Sequence)
	if len(seq) == 0 {
		return stDim.Render(d.emptySkillsReason(b))
	}
	seqWR := b.SkillOrder.WinRate()
	head := lipgloss.JoinHorizontal(lipgloss.Top,
		d.priorityLine(),
		stDim.Render(fmt.Sprintf("  ·  WR %s · %s partidas",
			wrStyle(seqWR, b.SkillOrder.Games).Render(pct(seqWR)),
			nThousand(b.SkillOrder.Games))))
	grid := d.abilityGrid(seq)
	out := lipgloss.JoinVertical(lipgloss.Left, head, "", grid)
	if legend := d.abilityLegend(); legend != "" {
		out = lipgloss.JoinVertical(lipgloss.Left, out, "", legend)
	}
	// Aviso ARRIBA del panel, no escondido en la leyenda: si las
	// posiciones no están verificadas, el jugador debe saberlo antes de
	// leer la matriz.
	if b.Game == model.GameDeadlock && !b.CanonicalSkillOrder {
		warn := stWinrateBad.Render("Orden por uso · posiciones no verificadas")
		out = lipgloss.JoinVertical(lipgloss.Left, warn, "", out)
	}
	return out
}

// skillLabelWidth calcula el ancho de la columna de nombres de
// habilidad. Se ajusta al nombre más largo para no truncar cuando hay
// espacio, y se recorta solo si la matriz de niveles no cabría.
//
// Antes era fijo en 18: "Abstract Landscape" (18 caracteres + tecla)
// se truncaba aunque la terminal tuviera sitio de sobra.
func (d detailView) skillLabelWidth() int {
	b := d.CurrentBuild()
	if b == nil {
		return 18
	}
	// Ancho necesario: tecla + espacio + nombre más largo.
	longest := 0
	for _, slot := range b.SkillAlphabet {
		if n, ok := b.AbilityNames[slot]; ok {
			if w := runewidth.StringWidth(n); w > longest {
				longest = w
			}
		}
	}
	want := longest + 2 // "1 " delante
	if want < 6 {
		want = 6
	}
	// Tope: dejar sitio a la matriz (3 celdas por nivel, 15 niveles).
	const levels = 15
	const cell = 3
	maxW := d.width - 4 - levels*cell
	if maxW < 10 {
		maxW = 10
	}
	if want > maxW {
		want = maxW
	}
	return want
}

// abilityGrid: filas por habilidad, columnas por nivel, celdas de 3.
func (d detailView) abilityGrid(seq []rune) string {
	b := d.CurrentBuild()
	labelW := 6
	if b.Game == model.GameDeadlock {
		labelW = d.skillLabelWidth()
	}
	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", labelW))
	for lvl := range seq {
		fmt.Fprintf(&sb, "%s", stDim.Render(fmt.Sprintf("%2d ", lvl+1)))
	}
	sb.WriteByte('\n')
	dark := lipgloss.Color("#010A13")
	for _, ab := range b.SkillAlphabet {
		fmt.Fprintf(&sb, "%s", d.abilityLabel(ab, labelW))
		for _, s := range seq {
			if s == ab {
				sb.WriteString(lipgloss.NewStyle().
					Background(skillColorOf(ab)).Foreground(dark).Render(" ◆ "))
			} else {
				sb.WriteString(stDim.Render(" · "))
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// slotLabel devuelve la referencia visible de un espacio de habilidad.
// Con orden canónico es la tecla real (1..4). Sin él, una letra neutral
// (A, B, C…): presentar "5" o "6" como si fueran teclas sería engañoso.
func slotLabel(ab rune, canonical bool) string {
	if canonical {
		return string(ab)
	}
	// '1' → 'A', '2' → 'B', …
	if ab >= '1' && ab <= '9' {
		return string(rune('A' + (ab - '1')))
	}
	return string(ab)
}

// abilityLabel: "Q" para LoL; "1 Siphon Life" para Deadlock.
func (d detailView) abilityLabel(ab rune, w int) string {
	b := d.CurrentBuild()
	if b.Game == model.GameDeadlock {
		if n, ok := b.AbilityNames[ab]; ok {
			label := fmt.Sprintf("%s %s", slotLabel(ab, b.CanonicalSkillOrder), truncate(n, w-2))
			return padStyled(skillStyle(ab).Render(label), w)
		}
	}
	return padStyled(skillStyle(ab).Render(slotLabel(ab, b.CanonicalSkillOrder)), w)
}

// abilityLegend nombra cada slot de habilidad cuando el juego lo
// permite (Deadlock real).
func (d detailView) abilityLegend() string {
	b := d.CurrentBuild()
	names := b.AbilityNames
	if len(names) == 0 {
		return ""
	}
	parts := make([]string, 0, len(names))
	for _, slot := range d.CurrentBuild().SkillAlphabet {
		if n, ok := names[slot]; ok {
			parts = append(parts, skillStyle(slot).Render(slotLabel(slot, b.CanonicalSkillOrder))+" "+stDim.Render(n))
		}
	}
	return strings.Join(parts, stDim.Render(" · "))
}

// priorityLine: orden de maximización con letras coloreadas.
func (d detailView) priorityLine() string {
	b := d.CurrentBuild()
	if b == nil {
		return ""
	}
	var parts []string
	for _, sk := range b.SkillOrder.Priority {
		parts = append(parts, skillStyle(sk).Render(slotLabel(sk, b.CanonicalSkillOrder)))
	}
	if len(parts) == 0 {
		return ""
	}
	return stDim.Render("max ") + strings.Join(parts, stDim.Render(" > "))
}

// ─── ayudantes ─────────────────────────────────────────────────────

// wrLegend explica los umbrales de color donde se usan.
func wrLegend() string {
	cell := func(c lipgloss.Color, label string) string {
		return lipgloss.NewStyle().Foreground(c).Render("■") + label
	}
	return stDim.Render("WR ") +
		cell(colWrGreat, "≥58% ") +
		cell(colWrGood, "≥52% ") +
		cell(colWrNeutral, "≥48% ") +
		cell(colWrBad, "<48%") +
		stDim.Render("   ·   gris = muestra baja (<"+fmt.Sprint(lowSampleGames)+" partidas)")
}

// itemPopup: detalle ampliado de la tarjeta activa (Enter). La fuente
// no trae descripciones de ítems: se amplían las métricas, el límite
// de Wilson y el contexto de la build.
func (d detailView) itemPopup() string {
	cards := d.cardList()
	if d.itemIdx >= len(cards) {
		d.showItem = false
		return ""
	}
	c := cards[d.itemIdx]
	it := c.item
	name := stKeyName.Render(d.session.ItemName(it.ID))
	tag := ""
	if c.boot {
		tag = stDim.Render(" · botas")
	}
	if c.ward {
		tag = stDim.Render(" · ward")
	}
	// Ruta de compra (Deadlock): orden de consenso, cadena y tasa de
	// mejora de la base. Sin ruta: el popup queda como siempre.
	if c.step > 0 {
		tag = stDim.Render(fmt.Sprintf(" · paso %d", c.step))
	}
	if c.kind == kindUpgrade {
		tag = stDim.Render(fmt.Sprintf(" · de %s (%d%%)",
			c.baseName, int(c.chainRate*100+0.5)))
	} else if c.upgradedRate >= 0.05 {
		tag += stDim.Render(fmt.Sprintf(" · ↗ %d%% la mejoran", int(c.upgradedRate*100+0.5)))
	}
	b := d.CurrentBuild()
	lines := []string{
		name + tag,
		"",
		fmt.Sprintf("  WR        %s   (límite Wilson 95%%: %s)",
			wrStyle(it.WinRate, it.Games).Render(pct(it.WinRate)),
			wrStyle(it.Wilson, it.Games).Render(pct(it.Wilson))),
		fmt.Sprintf("  pick      %s", stDim.Render(pct(it.PickRate))),
		fmt.Sprintf("  muestra   %s partidas", stDim.Render(nThousand(it.Games))),
		"",
		stDim.Render(fmt.Sprintf("  build: %s · %s · patch %s · fuente %s",
			d.champ.Name, b.Role, b.Patch, b.Source)),
	}
	if d.session.Game == model.GameDeadlock {
		delta := (it.WinRate - b.WinRate) * 100
		lines = append(lines, fmt.Sprintf("  Δ WR      %+.1f puntos vs. héroe", delta))
		if delta >= deltaAviso {
			lines = append(lines, "  ⚠ se compra yendo por delante")
		}
	}
	// Ruta de compra: orden, posición en la secuencia y patrimonio
	// mediano al comprarlo (net_worth_at_buy es acumulado, no almas en
	// mano — se muestra como "patrimonio", nunca "almas al comprar").
	if st, ok := d.routeStepOf(it.ID); ok {
		ruta := fmt.Sprintf("  ruta      #%d · posición %d–%d · patrim. ~%s",
			st.Order, st.PosLo, st.PosHi, nThousand(st.MedNet))
		if c.kind == kindUpgrade {
			ruta += fmt.Sprintf(" · el %d%% viene de %s",
				int(st.ChainRate*100+0.5), c.baseName)
		} else if st.UpgradedRate >= 0.05 {
			ruta += fmt.Sprintf(" · el %d%% luego la mejora",
				int(st.UpgradedRate*100+0.5))
		}
		lines = append(lines[:3], append([]string{"",
			stDim.Render(ruta)}, lines[3:]...)...)
	}
	// Build híbrida: confianza, procedencia y motivo. La confianza se
	// presenta como etiqueta cualitativa, nunca como probabilidad.
	if c.hybrid {
		conf := fmt.Sprintf("  confianza %s", ConfidenceWord(c.hybridConf))
		if c.hybridShare > 0 {
			conf += fmt.Sprintf(" · aparece en %.0f%% de las partidas", c.hybridShare*100)
		}
		lines = append(lines[:3], append([]string{"", stDim.Render(conf)}, lines[3:]...)...)
		if len(c.hybridOrigin) > 0 {
			lines = append(lines[:4], append([]string{
				stDim.Render("  procedencia " + strings.Join(c.hybridOrigin, ", ")),
			}, lines[4:]...)...)
		}
		if c.hybridReason != "" && c.hybridSection != SectionSituationals {
			lines = append(lines, "", stValue.Render("  "+c.hybridReason))
		}
		if c.hybridSection == SectionSituationals {
			lines = append(lines, "", stValue.Render("  función: "+c.hybridReason))
		}
	}
	// Metadatos del ítem (Deadlock): tier, categoría, coste y pasiva.
	if info, ok := d.session.ItemMeta[it.ID]; ok && info.Category != "" {
		lines = append(lines[:3], append([]string{
			fmt.Sprintf("  tier      %s · %s · %s",
				catColorStyle(info.Category).Render(romanTier(info.Tier)),
				catColorStyle(info.Category).Render(info.Category),
				stDim.Render(fmt.Sprintf("%d souls", info.Cost))),
		}, lines[3:]...)...)
		if info.Description != "" {
			lines = append(lines[:6], append([]string{
				"",
				lipgloss.NewStyle().Width(min(50, max(28, d.width-16))).Render(
					stValue.Render(info.Description)),
			}, lines[6:]...)...)
		}
	}
	if it.Games < lowSampleGames {
		lines = append(lines, "", stWinrateBad.Render("  ⚠ muestra baja: el WR puede no ser fiable"))
	}
	lines = append(lines, "", stDim.Render("esc cierra"))
	boxW := min(56, max(30, d.width-6))
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).
		Padding(1, 3).Width(boxW).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	return lipgloss.NewStyle().Width(d.width).Align(lipgloss.Center).Render(box)
}

// padStyled completa con espacios hasta el ancho visible.
func padStyled(s string, w int) string {
	// Los códigos ANSI no cuentan: medir solo los runes visibles.
	visible := lipgloss.Width(s)
	if visible >= w {
		return s
	}
	return s + strings.Repeat(" ", w-visible)
}

// truncate recorta con elipsis al ancho de celda indicado. Medir en
// runas engañaba con nombres anchos (CJK, emoji, acentos combinables):
// un "…" que ocupa 1 celda se contaba como 1 runa y la tarjeta
// desbordaba. runewidth ya era dependencia (vía lipgloss).
func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return runewidth.Truncate(s, max, "…")
}

func joinWithGap(parts []string, gap string) []string {
	out := make([]string, 0, len(parts)*2-1)
	for i, p := range parts {
		if i > 0 {
			out = append(out, gap)
		}
		out = append(out, p)
	}
	return out
}

// ─── pestaña Ruta (LoL) ────────────────────────────────────────────

// routeTabView: ruta sugerida + inventario final. Es una vista de
// lectura: no añade tarjetas navegables para no complicar la selección.
func (d detailView) routeTabView() string {
	plan := d.lolPlan()
	if plan == nil {
		return stDim.Render("sin datos suficientes para construir la ruta de compra")
	}
	lines := d.routeViewA(plan)
	if m := d.missionHint(); m != "" {
		lines = append(lines, "", stDim.Render("  "+m))
	}
	return lipgloss.JoinVertical(lipgloss.Left, d.wrapLines(lines)...)
}

// alternativesTabView: alternativas agrupadas por función.
func (d detailView) alternativesTabView() string {
	plan := d.lolPlan()
	if plan == nil {
		return stDim.Render("sin datos suficientes para proponer alternativas")
	}
	return lipgloss.JoinVertical(lipgloss.Left, d.wrapLines(d.alternativesViewB(plan))...)
}

// missionHint explica cómo cambiar el estado de misión cuando aplica.
func (d detailView) missionHint() string {
	cb := d.CurrentBuild()
	if cb == nil || cb.Role != model.RoleBottom {
		return ""
	}
	if d.session.botMissionDone {
		return "mostrando BOT después de la misión · M vuelve a antes de misión"
	}
	return "mostrando BOT antes de la misión · M simula después de la misión"
}

// shortTabName abrevia el nombre de una pestaña para barras estrechas.
func shortTabName(name string) string {
	switch name {
	case "Resumen":
		return "Resum."
	case "Habilidades":
		return "Habil."
	case "Matchups":
		return "Match."
	case "Alternativas":
		return "Alt."
	default:
		return name
	}
}
