package ui

import (
	"bytes"
	"context"
	"fmt"
	"image/color"
	"image/png"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/config"
	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/game"
	"github.com/Eion21/mobatui/internal/imgrender"
	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/procmon"
	"github.com/Eion21/mobatui/internal/staticdata"
	"github.com/Eion21/mobatui/internal/ugg"
)

// panelBg es el fondo de los paneles (theme.go colPanel); las imágenes
// funden su transparencia sobre él para integrarse sin marcos.
var panelBg = color.RGBA{0x0A, 0x14, 0x28, 0xFF}

// zoneMgr resuelve los clics de pestañas/píldoras/tarjetas marcadas
// con zone.Mark en el paquete ui.
var zoneMgr = func() *zone.Manager {
	m := zone.New()
	m.SetEnabled(true)
	return m
}()

// panelBgEsc es el mismo color como secuencia ANSI: keepPanelBG lo
// repone tras los resets internos de los estilos hijos para evitar
// franjas con el fondo de la terminal.
const panelBgEsc = "\x1b[48;2;10;20;40m"

// Names resuelve IDs a nombres reales vía el proveedor del juego.
type Names struct {
	Heroes       map[string]staticdata.HeroInfo
	HeroNameByID map[int]string // id → nombre (matchups)
	Items        map[int]string
	Runes        map[int]string
	Trees        map[int]string
	SpellNames   map[int]string // id → nombre (hechizos de invocador)
}

// ChampEntry agrupa todos los builds de un campeón/héroe, uno por rol.
// La lista muestra un campeón por fila; el detalle permite moverse
// entre sus roles (los juegos sin roles tienen uno solo).
type ChampEntry struct {
	Key    string
	Name   string
	Title  string
	Roles  []model.Role // en orden canónico del juego
	ByRole map[model.Role]*model.CleanBuild
}

// Build devuelve el build del rol en la posición indicada (nil si fuera
// de rango o sin roles).
func (c *ChampEntry) Build(roleIdx int) *model.CleanBuild {
	if roleIdx < 0 || roleIdx >= len(c.Roles) || len(c.Roles) == 0 {
		return nil
	}
	b, ok := c.ByRole[c.Roles[roleIdx]]
	if !ok && len(c.Roles) > 0 {
		// Rol desaparecido del mapa: recurre al primero existente.
		for _, r := range c.Roles {
			if bb, ok := c.ByRole[r]; ok {
				return bb
			}
		}
	}
	return b
}

// MainRoleIdx es el índice del rol más jugado (selección inicial); 0
// si no hay roles (evita divisiones por cero en los llamadores).
func (c *ChampEntry) MainRoleIdx() int {
	if len(c.Roles) == 0 {
		return 0
	}
	best, bestGames := 0, -1
	for i, r := range c.Roles {
		if b := c.ByRole[r]; b != nil && b.Total.Games > bestGames {
			best, bestGames = i, b.Total.Games
		}
	}
	return best
}

// champArt son las variantes pre-renderizadas del retrato de un
// campeón: mini half-block para la cabecera y las dos variantes ASCII
// grandes (color y mono) que se muestran con el toggle `a`.
type champArt struct {
	Mini  string
	ASCII string
	Mono  string
}

// Session es todo lo que la UI necesita de un juego, precargado.
type Session struct {
	Game        model.Game
	Champions   []*ChampEntry
	Builds      []*model.CleanBuild
	Names       Names
	Version     string
	ChampionArt map[string]champArt
	ItemArt     map[int]string              // half-blocks pre-renderizados
	ItemMeta    map[int]staticdata.ItemInfo // tier/categoría/coste/desc
	// Routes guarda la ruta de compra por héroe (Deadlock, carga bajo
	// demanda al abrir el detalle): orden de consenso, cadena y tasas.
	// nil = en curso; vacía = sin datos (muestra insuficiente o error).
	Routes map[string][]RouteStep
	// Runs trae la ruta bajo demanda; nil = sin datos de ruta.
	Runs RunsLoader
	// Hybrids guarda la build híbrida por héroe (carga bajo demanda).
	// nil = en curso; ausente = no pedida; presente = calculada.
	Hybrids map[string]*HybridBuildView
	// HybridLoading registra la clave de la solicitud en curso por
	// héroe, para descartar respuestas tardías u obsoletas.
	HybridLoading map[string]string
	// HybridLoader trae la build híbrida bajo demanda; nil = sin loader.
	HybridLoader BuildLoader
	// HybridKey es la clave de vigencia de las builds híbridas de esta
	// sesión (parche + muestra + filtros). Cambiarla invalida todas.
	HybridKey string
	// DataKind y DataAge describen el origen de los datos ("red",
	// "caché", "caducado", "fixtures"); vacío = desconocido.
	DataKind string
	DataAge  time.Duration
	// Omitted son los héroes/campeones descartados por datos
	// corruptos o sin asset: se muestran en la barra de estado.
	Omitted int
	// ItemCatalog es el catálogo de objetos de LoL con metadatos
	// (Data Dragon). nil = no disponible (Deadlock o sin datos).
	ItemCatalog map[int]lolitems.Item
	// botMissionDone recuerda si el usuario consultó el inventario de
	// BOT como "después de misión". Es una PREFERENCIA DE VISTA: nunca
	// se deduce el estado real de la partida.
	botMissionDone bool
	// lolPlans cachea los planes ya calculados por (campeón, rol,
	// parche, misión) para no recalcular en cada frame.
	lolPlans map[lolPlanKey]*lolbuild.Plan
}

// renderItemArt pre-renderiza el icono de un ítem si no existe.
func (s *Session) renderItemArt(p staticdata.Provider, id, cols int) {
	if _, done := s.ItemArt[id]; done {
		return
	}
	raw, err := p.ItemIcon(id)
	if err != nil {
		return
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return
	}
	s.ItemArt[id] = imgrender.RenderAspect(img, cols, panelBg)
}

// ItemName resuelve el nombre real de un ítem con fallback.
func (s *Session) ItemName(id int) string {
	if n, ok := s.Names.Items[id]; ok && n != "" {
		return n
	}
	return fmt.Sprintf("Ítem %d", id)
}

// SpellName resuelve el nombre real de un hechizo de invocador.
func (s *Session) SpellName(id int) string {
	if n, ok := s.Names.SpellNames[id]; ok && n != "" {
		return n
	}
	return fmt.Sprintf("Hechizo %d", id)
}

// SpellPairName resuelve un par de hechizos ("Destello + Prender");
// sin segundo hechizo, el nombre solo.
func (s *Session) SpellPairName(ids [2]int) string {
	if ids[1] == 0 {
		return s.SpellName(ids[0])
	}
	return s.SpellName(ids[0]) + " + " + s.SpellName(ids[1])
}

// RuneName resuelve el nombre real de una runa con fallback.
func (s *Session) RuneName(id int) string {
	if n, ok := s.Names.Runes[id]; ok && n != "" {
		return n
	}
	return fmt.Sprintf("Runa %d", id)
}

// TreeName resuelve el nombre de un árbol con fallback.
func (s *Session) TreeName(id int) string {
	if n, ok := s.Names.Trees[id]; ok && n != "" {
		return n
	}
	return model.TreeName(id)
}

// ShardName resuelve el nombre del fragmento de estadísticas (LoL).
func (s *Session) ShardName(id int) string {
	if n, ok := ddragon.Shards[id]; ok {
		return n
	}
	return fmt.Sprintf("Fragmento %d", id)
}

// Loader configura las fuentes de datos de todos los juegos.
type Loader struct {
	UggDir      string // fixtures/dumps de u.gg (LoL)
	DeadlockDir string // fixtures/dumps de Deadlock (carpeta builds/)
	Providers   map[model.Game]staticdata.Provider
	// BuildLoaders permite sustituir la carga de builds por juego (p.ej.
	// el cliente de la API de Deadlock); si no, se usan los fixtures.
	BuildLoaders map[model.Game]func() ([]*model.ChampionBuilds, error)
	// SetReporter, si no es nil, conecta el aviso de progreso del
	// cargador de un juego con la UI ("124/200 · Aurelion Sol").
	SetReporter func(fn func(done, total int, name string)) bool
	// RunsLoaders trae la ruta de compra bajo demanda por juego
	// (Deadlock); nil = sin datos de ruta.
	RunsLoaders map[model.Game]RunsLoader
	// UpdateCheck, si no es nil, comprueba si hay una versión más
	// reciente. Se ejecuta UNA vez por arranque en segundo plano; nil =
	// comprobación desactivada (-no-update-check, config u offline).
	UpdateCheck func(context.Context) (UpdateInfo, error)
}

// loadBuilds resuelve la fuente de builds crudos de un juego.
func (l Loader) loadBuilds(g model.Game) ([]*model.ChampionBuilds, error) {
	if fn := l.BuildLoaders[g]; fn != nil {
		return fn()
	}
	switch g {
	case model.GameDeadlock:
		return deadlock.LoadDir(l.DeadlockDir)
	default:
		return ugg.LoadDir(l.UggDir)
	}
}

// LoadSession ejecuta la carga completa de un juego: fixtures/dumps →
// limpieza Wilson → enriquecimiento con nombres estáticos → pre-render
// de iconos a half-blocks truecolor. Los fallos parciales (nombres,
// iconos) se degradan con fallbacks; solo un fallo de datos crudos es
// fatal. El arte se pre-renderiza aquí (una vez) y View() solo pega
// strings.
func LoadSession(g model.Game, l Loader) (*Session, error) {
	cbs, err := l.loadBuilds(g)
	if err != nil {
		return nil, err
	}
	s := &Session{
		Game:        g,
		ChampionArt: map[string]champArt{},
		ItemArt:     map[int]string{},
	}
	// Enriquecimiento tolerante a fallos.
	if p := l.Providers[g]; p != nil {
		s.Version, _ = p.Version()
		// Clave de vigencia de las builds híbridas: si el parche o la
		// fuente cambian, las builds guardadas dejan de usarse.
		s.HybridKey = hybridKeyFor(s.Version, "")
		if heroes, err := p.Heroes(); err == nil {
			s.Names.Heroes = heroes
			s.Names.HeroNameByID = make(map[int]string, len(heroes))
			for _, h := range heroes {
				if h.ID != 0 {
					s.Names.HeroNameByID[h.ID] = h.Name
				}
			}
			for _, cb := range cbs {
				if h, ok := heroes[cb.ChampionKey]; ok {
					cb.ChampionName = h.Name
				}
			}
			// Los cruces (matchups/sinergias) incluyen héroes jugables
			// marcados in_development que el roster no lista; si el
			// proveedor resuelve el id→nombre completo, gana.
			if hb, ok := p.(staticdata.HeroNamesByIDProvider); ok {
				if byID := hb.HeroNamesByID(); len(byID) > 0 {
					s.Names.HeroNameByID = byID
				}
			}
		}
		s.Names.Items, _ = p.Items()
		s.Names.Runes, s.Names.Trees, _ = p.Runes()
		// Hechizos de invocador (LoL): nombres vía Data Dragon, con
		// fallo tolerado (la UI muestra "Hechizo 4" como fallback).
		if sp, ok := p.(staticdata.SummonerSpellsProvider); ok {
			if m, err := sp.SummonerSpells(); err == nil {
				s.Names.SpellNames = m
			}
		}
		// Metadatos de ítem (tier/categoría/coste/descripción) si la
		// fuente los trae.
		if dp, ok := p.(staticdata.ItemDetailsProvider); ok {
			s.ItemMeta, _ = dp.ItemDetails()
		}
		// Catálogo de objetos con metadatos (LoL): alimenta el
		// planificador de rutas. Reutiliza la MISMA caché de Data
		// Dragon que Items(), así que no añade peticiones.
		if ic, ok := p.(interface {
			ItemCatalog() (map[int]lolitems.Item, error)
		}); ok {
			if cat, err := ic.ItemCatalog(); err == nil && len(cat) > 0 {
				s.ItemCatalog = cat
			}
		}
		// Origen de los datos, para no servir datos viejos en silencio.
		if fp, ok := p.(staticdata.FreshnessProvider); ok {
			s.DataKind, s.DataAge, _ = fp.DataState()
		}
		if op, ok := p.(interface{ Omitted() int }); ok {
			s.Omitted = op.Omitted()
		}
	}
	// Ruta de compra bajo demanda (Deadlock): el loader vive en la
	// sesión y el detalle dispara la carga al abrir el héroe.
	if rl := l.RunsLoaders[g]; rl != nil {
		s.Runs = rl
		// El mismo adaptador sirve la build híbrida cuando lo implementa.
		if bl, ok := rl.(BuildLoader); ok {
			s.HybridLoader = bl
		}
	}

	// En Deadlock el ranking global de Wilson elige casi siempre
	// ítems tardíos (sesgo de supervivencia): se dejan pasar todos los
	// que superan el filtro y la UI rankea el top por fase de compra.
	opts := clean.Options{}
	if g == model.GameDeadlock {
		opts.MaxItems = 9999 // sin tope global: el corte es por fase
	}
	s.Builds = clean.BuildAll(cbs, opts)
	s.Champions = groupByChampion(s.Builds, s.Names.Heroes, game.For(g).Roles)

	// Pre-render de arte: mini half-block para la cabecera y variantes
	// ASCII grandes (color con bordes/dither y mono con tinte dorado)
	// para el toggle `a`. Ítems: half-blocks para las tarjetas (todas
	// las core y las botas).
	if p := l.Providers[g]; p != nil {
		const itemCols = 12
		for _, e := range s.Champions {
			raw, err := p.ChampionIcon(e.Key)
			if err != nil {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			subject := imgrender.Crop(img, 0.9)
			mono := imgrender.DefaultASCII
			mono.Color = false
			mono.Tint = imgrender.GoldTint
			s.ChampionArt[e.Key] = champArt{
				Mini:  imgrender.RenderAspect(imgrender.AutoContrast(imgrender.Crop(img, 0.85)), 12, panelBg),
				ASCII: imgrender.ASCII(subject, 20, panelBg, imgrender.DefaultASCII),
				Mono:  imgrender.ASCII(subject, 20, panelBg, mono),
			}
		}
		for _, b := range s.Builds {
			for _, it := range b.CoreItems {
				s.renderItemArt(p, it.ID, itemCols)
			}
			for _, it := range b.Boots {
				s.renderItemArt(p, it.ID, itemCols)
			}
			// El ward recomendado también merece su icono: sin esto cae
			// al fallback (la inicial) en la sección WARD.
			for _, it := range b.Wards {
				s.renderItemArt(p, it.ID, itemCols)
			}
		}
	}
	return s, nil
}

// groupByChampion agrupa builds por campeón/héroe, ordena los roles en
// el orden canónico del juego y los nombres alfabéticamente.
func groupByChampion(builds []*model.CleanBuild, heroes map[string]staticdata.HeroInfo, roles []model.Role) []*ChampEntry {
	idx := map[string]*ChampEntry{}
	var out []*ChampEntry
	for _, b := range builds {
		e, ok := idx[b.ChampionKey]
		if !ok {
			e = &ChampEntry{Key: b.ChampionKey, ByRole: map[model.Role]*model.CleanBuild{}}
			idx[b.ChampionKey] = e
			out = append(out, e)
		}
		e.ByRole[b.Role] = b
	}
	for _, e := range out {
		if h, ok := heroes[e.Key]; ok {
			e.Name, e.Title = h.Name, h.Title
		} else {
			e.Name = e.Key
		}
		for _, r := range roles {
			if _, ok := e.ByRole[r]; ok {
				e.Roles = append(e.Roles, r)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ─── modelo raíz ───────────────────────────────────────────────────

type screen int

const (
	screenPick screen = iota
	screenLoading
	screenList
	screenDetail
)

type sessionLoadedMsg struct {
	game    model.Game
	session *Session
	err     error
}

// progressMsg informa del progreso de la carga en paralelo: cuántas
// páginas van (Done de Total) y cuál se acaba de resolver.
type progressMsg struct {
	Done, Total int
	Name        string
}

// progressCh transporta el progreso desde el cargador (que corre en su
// propia goroutine) hasta el bucle de Bubble Tea.
type progressCh chan progressMsg

// waitProgress espera el siguiente aviso. Se reemite tras cada uno:
// Bubble Tea no lee un canal por su cuenta.
func waitProgress(ch progressCh) tea.Cmd {
	return func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return nil
		}
		return m
	}
}

// App es el modelo raíz de Bubble Tea: selector de juego → carga →
// lista → detalle, con overlay de ayuda (?), avisos temporales (toasts)
// y esc que sube un nivel en cada capa.
type App struct {
	screen        screen
	spinner       spinner.Model
	pickIdx       int
	list          buildList
	detail        detailView
	session       *Session
	sessions      map[model.Game]*Session // sesiones ya cargadas: reentrar es instantáneo
	errMsg        string
	failedGame    model.Game
	width         int
	height        int
	status        string
	loader        Loader
	help          help.Model
	showHelp      bool
	toast         string
	toastID       int             // id del aviso vigente (ver toastCmd)
	progress      progressCh      // avisos de la carga en curso (nil si ninguna)
	done          int             // páginas resueltas de la carga en curso
	total         int             // páginas totales de la carga en curso
	cfg           config.Config   // preferencias persistentes
	monCtx        context.Context // contexto del monitor de juegos
	monStop       context.CancelFunc
	monCh         chan procmon.Event
	detected      model.Game // juego activo según el monitor ("" = ninguno)
	detectedChamp string     // personaje detectado en el juego activo
	// update es la actualización disponible, si la comprobación en
	// segundo plano encontró una versión superior. nil = sin aviso.
	update *UpdateInfo
	// updateChecked evita repetir la comprobación en cada re-entrada.
	updateChecked bool
}

// UpdateInfo describe una versión más reciente publicada.
type UpdateInfo struct {
	Version string // "0.2.1"
	URL     string // enlace oficial, ya validado; "" = sin abrir
}

// updateAvailableMsg llega cuando la comprobación en segundo plano
// termina. Update=false significa "sin novedades o sin datos".
type updateAvailableMsg struct {
	Info   UpdateInfo
	Update bool
}

// checkUpdateCmd lanza la comprobación en una goroutine de Bubble Tea:
// no bloquea la primera pantalla.
func checkUpdateCmd(fn func(context.Context) (UpdateInfo, error)) tea.Cmd {
	return func() tea.Msg {
		if fn == nil {
			return updateAvailableMsg{}
		}
		// El timeout real vive en updatecheck; el contexto permite
		// cancelar si la app se cierra antes.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		info, err := fn(ctx)
		if err != nil || info.Version == "" {
			// Silencio: un fallo de red nunca es asunto del usuario.
			return updateAvailableMsg{}
		}
		return updateAvailableMsg{Info: info, Update: true}
	}
}

func NewApp(loader Loader) App {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = stLogo
	h := help.New()
	h.Styles.ShortKey = stValue
	h.Styles.ShortDesc = stDim
	h.Styles.FullKey = stValue
	h.Styles.FullDesc = stDim
	h.Styles.FullSeparator = stDim
	app := App{
		screen:   screenPick,
		spinner:  sp,
		list:     newBuildList(),
		loader:   loader,
		sessions: map[model.Game]*Session{},
		help:     h,
		cfg:      config.Load(),
	}
	// Restaurar el último juego en el selector.
	for i, g := range game.All {
		if string(g.ID) == app.cfg.LastGame {
			app.pickIdx = i
		}
	}
	return app
}

// ─── atajos y ayuda ────────────────────────────────────────────────

var (
	keyOpen   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "abrir"))
	keyDetail = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "detalle"))
	keyNav    = key.NewBinding(key.WithKeys("up", "down", "left", "right", "k", "j", "h", "l"), key.WithHelp("←↑↓→ hjkl", "tarjeta"))
	keyFilter = key.NewBinding(key.WithKeys("/"), key.WithHelp("escribe", "filtrar"))
	// Siete pestañas en LoL (cuatro en Deadlock): la ayuda ya no puede
	// decir "1-3". El binding base cubre las cuatro de Deadlock; LoL usa
	// keyTabLol para anunciar el rango correcto.
	keyTab = key.NewBinding(
		key.WithKeys("tab", "1", "2", "3", "4"),
		key.WithHelp("tab/1-4", "pestaña"))
	keyTabLol = key.NewBinding(
		key.WithKeys("tab", "1", "2", "3", "4", "5", "6", "7"),
		key.WithHelp("tab/1-7", "pestaña"))
	keyRole    = key.NewBinding(key.WithKeys("r", "R", "[", "]"), key.WithHelp("r/R", "rol"))
	keyArt     = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "retrato"))
	keyHelp    = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "ayuda"))
	keyBack    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "volver"))
	keyGame    = key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "juego"))
	keyReload  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "recargar"))
	keyQuit    = key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "salir"))
	keyMonitor = key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "monitor de juegos"))
	keySort    = key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "orden"))
	keyPick    = key.NewBinding(key.WithKeys("left", "right", "up", "down"), key.WithHelp("←→", "elegir"))
	keyListNav = key.NewBinding(key.WithKeys("up", "down"), key.WithHelp("↑↓", "navegar"))
	// M conmuta el inventario de BOTTOM antes/después de la misión.
	keyMission = key.NewBinding(key.WithKeys("M"), key.WithHelp("M", "misión"))
)

// shortHelp devuelve los atajos útiles según pantalla y pestaña: nada
// de una lista fija que no aplica.
func (a App) shortHelp() []key.Binding {
	switch a.screen {
	case screenPick:
		return []key.Binding{keyPick, keyOpen, keyMonitor, keyHelp, keyQuit}
	case screenList:
		return []key.Binding{keyListNav, keyDetail, keyFilter, keyMonitor, keySort, keyGame, keyHelp, keyQuit}
	case screenDetail:
		tabKey := keyTab
		if a.detail.session != nil && a.detail.session.Game == model.GameLoL {
			tabKey = keyTabLol
		}
		base := []key.Binding{tabKey, keyRole, keyHelp, keyBack}
		// La tecla de misión solo tiene efecto en BOTTOM de LoL.
		if a.detail.session != nil && a.detail.session.Game == model.GameLoL {
			if cb := a.detail.CurrentBuild(); cb != nil && cb.Role == model.RoleBottom {
				base = []key.Binding{keyTab, keyMission, keyRole, keyHelp, keyBack}
			}
		}
		if a.detail.tab == 0 {
			return append([]key.Binding{keyNav, keyDetail}, base...)
		}
		if a.detail.tab == lastTab(&a.detail) {
			return append([]key.Binding{keyArt}, base...)
		}
		return base
	}
	return []key.Binding{keyHelp, keyQuit}
}

// fullHelp agrupa todos los atajos para el overlay ?.
func (a App) fullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keyNav, keyDetail, keyListNav, keyFilter},
		{keyTab, keyRole, keyArt, keyPick},
		{keyGame, keyReload, keyMonitor, keyHelp, keyBack, keyQuit},
	}
}

// ─── toasts ────────────────────────────────────────────────────────

// toastMsg lleva un id: cada aviso invalida el borrado del anterior.
// Sin él, pulsar `a` dos veces en menos de 1,5 s borraba el segundo
// aviso con el temporizador del primero.
type toastMsg struct {
	id   int
	text string
}

type clearToastMsg struct{ id int }

var toastSeq int

func toastCmd(s string) tea.Cmd {
	toastSeq++
	id := toastSeq
	return tea.Batch(
		func() tea.Msg { return toastMsg{id: id, text: s} },
		tea.Tick(1500*time.Millisecond, func(time.Time) tea.Msg { return clearToastMsg{id: id} }),
	)
}

func loadSessionCmd(g model.Game, l Loader, ch progressCh) tea.Cmd {
	return func() tea.Msg {
		if ch != nil && l.SetReporter != nil {
			// Envío sin bloquear: si la UI va lenta, se pierde un aviso
			// de progreso, nunca la carga.
			l.SetReporter(func(done, total int, name string) {
				select {
				case ch <- progressMsg{Done: done, Total: total, Name: name}:
				default:
				}
			})
		}
		s, err := LoadSession(g, l)
		return sessionLoadedMsg{game: g, session: s, err: err}
	}
}

// startLoad abre el canal de progreso y lanza la carga con su escucha.
// Devuelve el App actualizado, porque guarda el canal abierto.
func (a App) startLoad(g model.Game) (App, tea.Cmd) {
	ch := make(progressCh, 32)
	a.progress = ch
	a.done, a.total = 0, 0
	return a, tea.Batch(loadSessionCmd(g, a.loader, ch), waitProgress(ch))
}

func (a App) Init() tea.Cmd {
	// Comprobación de versión: una sola vez por arranque y en paralelo
	// con el resto. Si no hay comprobador (opt-out, offline o -no-update-check)
	// no se lanza nada.
	if !a.updateChecked && a.loader.UpdateCheck != nil && !a.cfg.UpdateCheckOff {
		a.updateChecked = true
		var cmds []tea.Cmd
		if !a.cfg.MonitorOff {
			cmds = append(cmds, a.startMonitor())
		}
		return tea.Batch(append(cmds, checkUpdateCmd(a.loader.UpdateCheck))...)
	}
	if a.cfg.MonitorOff {
		return nil
	}
	return a.startMonitor()
}

// ─── monitor de juegos ─────────────────────────────────────────────

type monEventMsg struct{ ev procmon.Event }
type monStoppedMsg struct{}

// startMonitor lanza el worker de procesos y devuelve el comando que
// escucha su canal (un mensaje por evento; se re-ancla en Update).
func (a App) startMonitor() tea.Cmd {
	ctx, stop := context.WithCancel(context.Background())
	a.monCtx, a.monStop = ctx, stop
	a.monCh = make(chan procmon.Event, 8)
	go procmon.Run(ctx, procmon.DefaultInterval, procmon.NewProcessDetector(), procmon.NewCharacterProber(), a.monCh)
	return listenMonitor(a.monCh)
}

func listenMonitor(ch chan procmon.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return monStoppedMsg{}
		}
		return monEventMsg{ev}
	}
}

// stopMonitor apaga el worker y limpia el indicador.
func (a *App) stopMonitor() {
	if a.monStop != nil {
		a.monStop()
	}
	a.monCtx, a.monStop, a.monCh = nil, nil, nil
	a.detected = ""
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		a.list.Resize(m.Width, m.Height)
		a.detail.width = m.Width
		a.detail.height = m.Height
		// Cabecera de la app (1) + cabecera del detalle (variable con
		// retrato lateral) + pestañas (1) + pie (1) + aire: se calcula
		// tras el primer View con el layout real; 24 es un arranque
		// sano.
		a.detail.vp.Width = m.Width - 4
		a.detail.vp.Height = max(8, m.Height-10)
		a.help.Width = m.Width
		return a, nil

	case tea.MouseMsg:
		// La rueda va siempre al detalle (scroll del viewport); los
		// clics, solo con el botón izquierdo.
		if a.screen == screenDetail {
			a.detail, _ = a.detail.Update(m)
			if m.Action == tea.MouseActionPress && m.Button == tea.MouseButtonLeft {
				a.detail = a.detail.handleMouse(m)
			}
		}

	case monEventMsg:
		switch m.ev.Type {
		case procmon.Started:
			a.detected = model.Game(m.ev.Game)
			g := a.detected
			// Desde el selector, abrir el juego detectado de una vez.
			if a.screen == screenPick {
				for i, spec := range game.All {
					if spec.ID == g {
						a.pickIdx = i
						break
					}
				}
				var cmd tea.Cmd
				a, cmd = a.startLoad(g)
				return a, tea.Batch(cmd, listenMonitor(a.monCh))
			}
			return a, tea.Batch(toastCmd(g.DisplayName()+" detectado"), listenMonitor(a.monCh))
		case procmon.Ended:
			prev := a.detected
			a.detected, a.detectedChamp = "", ""
			if prev != "" {
				return a, tea.Batch(
					toastCmd(prev.DisplayName()+" cerrado"),
					listenMonitor(a.monCh))
			}
		case procmon.Char:
			// El personaje viaja siempre detrás del Started de su juego:
			// sin juego activo no se muestra.
			if a.detected != "" && m.ev.Character != a.detectedChamp {
				a.detectedChamp = m.ev.Character
				return a, tea.Batch(
					toastCmd("Personaje detectado: "+m.ev.Character),
					listenMonitor(a.monCh))
			}
		}
		return a, listenMonitor(a.monCh)

	case monStoppedMsg:
		return a, nil

	case updateAvailableMsg:
		// Aviso discreto y persistente: se guarda en el estado y el
		// usuario lo descarta con "u" o al abrirlo. Nunca interrumpe.
		if m.Update {
			a.update = &m.Info
		}
		return a, nil

	case toastMsg:
		a.toast, a.toastID = m.text, m.id
		return a, nil
	case progressMsg:
		a.done, a.total = m.Done, m.Total
		if a.screen == screenLoading {
			a.status = fmt.Sprintf("%d/%d · %s", m.Done, m.Total, m.Name)
		}
		// Se rearma: sin esto solo llegaría el primer aviso.
		return a, waitProgress(a.progress)
	case runsLoadedMsg:
		// La ruta del héroe llegó: se guarda en la sesión (vacía si
		// hubo error, para no reintentar en loop) y el detalle activo
		// se re-renderiza con los números de ruta.
		if a.session == nil || a.session.Game != model.GameDeadlock || m.ChampKey == "" {
			return a, nil
		}
		active := a.screen == screenDetail && a.detail.champ != nil && a.detail.champ.Key == m.ChampKey
		selected, hasSelection := 0, false
		if active {
			cards := a.detail.cardList()
			if a.detail.itemIdx >= 0 && a.detail.itemIdx < len(cards) {
				selected, hasSelection = cards[a.detail.itemIdx].item.ID, true
			}
		}
		if a.session.Routes == nil {
			a.session.Routes = map[string][]RouteStep{}
		}
		if m.Err != nil || len(m.Steps) == 0 {
			a.session.Routes[m.ChampKey] = []RouteStep{}
		} else {
			a.session.Routes[m.ChampKey] = m.Steps
		}
		if active {
			if hasSelection {
				for i, card := range a.detail.cardList() {
					if card.item.ID == selected {
						a.detail.itemIdx = i
						break
					}
				}
			}
			a.detail.clampSel()
			// Preserve manual scroll, including when the popup is open.
			a.detail.scrollAnchor = a.detail.itemIdx
			a.detail.sync()
		}
		if m.Err != nil {
			return a, toastCmd("Ruta de compra no disponible")
		}
		return a, nil
	case hybridLoadedMsg:
		// La build híbrida llegó: se guarda en la sesión y el detalle
		// activo se re-renderiza conservando selección y scroll.
		if a.session == nil || a.session.Game != model.GameDeadlock || m.ChampKey == "" {
			return a, nil
		}
		if a.session.Hybrids == nil {
			a.session.Hybrids = map[string]*HybridBuildView{}
		}
		// Respuesta obsoleta: ignora sin tocar el estado vigente ni la carga en curso.
		if m.Key != a.session.HybridKey {
			return a, nil
		}
		if loading, ok := a.session.HybridLoading[m.ChampKey]; !ok || loading != m.Key {
			return a, nil
		}
		delete(a.session.HybridLoading, m.ChampKey)
		if m.Err != nil || m.Build == nil {
			// Error o resultado vacío: libera la solicitud para permitir reintentos.
			delete(a.session.Hybrids, m.ChampKey)
		} else {
			m.Build.Key = m.Key
			a.session.Hybrids[m.ChampKey] = m.Build
		}
		active := a.screen == screenDetail && a.detail.champ != nil && a.detail.champ.Key == m.ChampKey
		if active {
			selected, hasSelection := 0, false
			cards := a.detail.cardList()
			if a.detail.itemIdx >= 0 && a.detail.itemIdx < len(cards) {
				selected, hasSelection = cards[a.detail.itemIdx].item.ID, true
			}
			if hasSelection {
				for i, card := range a.detail.cardList() {
					if card.item.ID == selected {
						a.detail.itemIdx = i
						break
					}
				}
			}
			a.detail.clampSel()
			a.detail.scrollAnchor = a.detail.itemIdx
			a.detail.sync()
		}
		if m.Err != nil || m.Build == nil {
			return a, toastCmd("Build híbrida no disponible")
		}
		return a, nil
	case clearToastMsg:
		// Solo borra si sigue vigente el aviso que lo programó.
		if m.id == a.toastID {
			a.toast = ""
		}
		return a, nil

	case tea.KeyMsg:
		// El overlay de ayuda se cierra con cualquier tecla.
		if a.showHelp {
			a.showHelp = false
			return a, nil
		}
		if m.String() == "?" {
			a.showHelp = true
			return a, nil
		}
		switch m.String() {
		case "m":
			if a.screen == screenPick || a.screen == screenList {
				a.cfg.MonitorOff = !a.cfg.MonitorOff
				a.cfg.Save()
				if a.cfg.MonitorOff {
					a.stopMonitor()
					return a, toastCmd("Monitor de juegos: desactivado")
				}
				cmd := a.startMonitor()
				return a, tea.Batch(cmd, toastCmd("Monitor de juegos: activo"))
			}
		case "v":
			// Aviso de versión: "v" abre el release (si hay enlace
			// validado) y lo descarta. Es una tecla libre en todas las
			// pantallas; "esc" se reserva para navegar/volver y "u" ya
			// pliega mejoras en Deadlock.
			if a.update != nil {
				url := a.update.URL
				a.update = nil
				if url != "" {
					if err := openURL(url); err != nil {
						return a, toastCmd("No se pudo abrir el enlace")
					}
					return a, toastCmd("Abriendo GitHub Releases…")
				}
				return a, toastCmd("Aviso descartado")
			}
		case "V":
			// Mayúscula: descarta sin abrir nada (útil cuando el
			// navegador no está disponible).
			if a.update != nil {
				a.update = nil
				return a, toastCmd("Aviso descartado")
			}
		case "ctrl+c":
			if a.monStop != nil {
				a.monStop()
			}
			return a, tea.Quit
		case "q":
			if a.screen == screenList && a.list.FilterActive() {
				break // q escribe en el filtro, no sale
			}
			if a.screen == screenPick || a.screen == screenLoading {
				if a.monStop != nil {
					a.monStop()
				}
				return a, tea.Quit
			}
		}

	case sessionLoadedMsg:
		if m.err != nil {
			a.errMsg = m.err.Error()
			a.failedGame = m.game
			a.screen = screenLoading // mantiene el panel de error
			return a, nil
		}
		a.session = m.session
		a.sessions[m.game] = m.session
		a.list.LoadSession(m.session, a.cfg.SortMode)
		if string(m.game) == a.cfg.LastGame {
			a.list.SelectChampion(a.cfg.LastChampion)
		}
		a.status = statusLine(m.session)
		a.screen = screenList
		return a, toastCmd(fmt.Sprintf("%s listo", m.session.Game.DisplayName()))
	}

	var cmd tea.Cmd
	switch a.screen {
	case screenPick:
		a, cmd = a.updatePick(msg)
	case screenLoading:
		if km, ok := msg.(tea.KeyMsg); ok && a.errMsg != "" {
			switch km.String() {
			case "esc":
				a.errMsg, a.screen = "", screenPick
				a.progress = nil
				return a, nil
			case "r":
				a.errMsg = ""
				a, cmd = a.startLoad(a.failedGame)
				return a, tea.Batch(cmd, a.spinner.Tick)
			}
		}
		a.spinner, cmd = a.spinner.Update(msg)
	case screenList:
		if km, ok := msg.(tea.KeyMsg); ok {
			switch km.String() {
			case "enter":
				// Con el filtro en uso, enter abre el campeón filtrado
				// (escribir refina sin pulsar nada más).
				if e := a.list.SelectedChampion(); e != nil {
					a.detail = newDetailView(e, a.session)
					a.detail.width = a.width
					a.detail.height = a.height
					a.detail.applyCfg(a.cfg)
					a.detail.sync()
					a.screen = screenDetail
					a.cfg.LastChampion = e.Key
					a.cfg.LastRole = a.detail.CurrentBuild().Role.String()
					a.cfg.LastTab = a.detail.tab
					a.cfg.SeenHint = true
					a.cfg.Save()
				}
				return a, tea.Batch(runsCmdFor(a), hybridCmdFor(a))
			case "esc":
				// Primer esc cancela el filtro (lo resuelve buildList);
				// segundo esc vuelve al selector de juego.
				if a.list.FilterActive() {
					a.list, cmd = a.list.Update(msg)
					return a, cmd
				}
				a.screen = screenPick
				a.session = nil
				return a, nil
			case "tab":
				if a.list.FilterActive() {
					break // escribiendo: tab no reordena
				}
				mode := a.list.CycleSort()
				a.cfg.SortMode = a.list.sortMode
				a.cfg.Save()
				return a, toastCmd("Orden: " + mode)
			}
		}
		a.list, cmd = a.list.Update(msg)
	case screenDetail:
		if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
			// Esc sube un nivel: primero cierra el popup del ítem.
			if a.detail.popupOpen() {
				a.detail, cmd = a.detail.Update(msg)
				return a, cmd
			}
			a.screen = screenList
			return a, nil
		}
		before := a.detail
		a.detail, cmd = a.detail.Update(msg)
		if a.detail.roleIdx != before.roleIdx || a.detail.tab != before.tab || a.detail.artMode != before.artMode {
			a.cfg.LastChampion = a.detail.champ.Key
			a.cfg.LastRole = a.detail.CurrentBuild().Role.String()
			a.cfg.LastTab = a.detail.tab
			a.cfg.ArtMode = a.detail.artMode
			a.cfg.Save()
		}
	}
	return a, cmd
}

// hybridKeyFor compone la clave de vigencia de una build híbrida. La
// muestra y el catálogo ya están fijados por la sesión (y por la caché
// de disco); el parche es lo que cambia entre sesiones. Un cambio de
// filtros que afecte a la generación debe pasar por `extra`.
func hybridKeyFor(patch, extra string) string {
	return patch + "|" + extra
}

// runsCmdFor dispara la carga bajo demanda de la ruta del héroe del
// detalle activo: solo Deadlock con loader y ruta no pedida aún.
// Devuelve nil cuando no aplica.
func runsCmdFor(a App) tea.Cmd {
	if a.session == nil || a.session.Game != model.GameDeadlock || a.session.Runs == nil {
		return nil
	}
	key := a.detail.champ.Key
	if a.session.Routes == nil {
		a.session.Routes = map[string][]RouteStep{}
	}
	if _, ok := a.session.Routes[key]; ok {
		return nil // ya cargadas o en curso
	}
	a.session.Routes[key] = nil // marca "en curso" hasta que llegue el msg
	return loadRunsCmd(a.session.Runs, key)
}

// hybridCmdFor dispara el cálculo de la build híbrida del héroe activo.
// Reutiliza el resultado ya calculado si sigue vigente en la sesión: al
// volver a un héroe no se repite el procesamiento.
func hybridCmdFor(a App) tea.Cmd {
	if a.session == nil || a.session.Game != model.GameDeadlock || a.session.HybridLoader == nil {
		return nil
	}
	key := a.detail.champ.Key
	if a.session.Hybrids == nil {
		a.session.Hybrids = map[string]*HybridBuildView{}
	}
	if hb, ok := a.session.Hybrids[key]; ok {
		// Reutilizar solo si la build guardada sigue vigente.
		if hb == nil {
			return nil // en curso
		}
		if hb.Key == a.session.HybridKey {
			return nil
		}
		// Clave distinta: resultado obsoleto, se descarta y se recalcula.
		delete(a.session.Hybrids, key)
	}
	a.session.Hybrids[key] = nil // marca "en curso"
	if a.session.HybridLoading == nil {
		a.session.HybridLoading = map[string]string{}
	}
	a.session.HybridLoading[key] = a.session.HybridKey
	return loadHybridCmd(a.session.HybridLoader, key, a.session.HybridKey)
}

// updatePick maneja el selector de juego.
func (a App) updatePick(msg tea.Msg) (App, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	n := len(game.All)
	var cmd tea.Cmd
	switch km.String() {
	case "left", "up", "h", "k":
		a.pickIdx = (a.pickIdx - 1 + n) % n
	case "right", "down", "l", "j", "tab":
		a.pickIdx = (a.pickIdx + 1) % n
	case "enter":
		g := game.All[a.pickIdx].ID
		a.cfg.LastGame = string(g)
		a.cfg.Save()
		// Sesión ya cargada en esta ejecución: cambio instantáneo,
		// sin pasar por la pantalla de carga.
		if s, ok := a.sessions[g]; ok {
			a.session = s
			a.list.LoadSession(s, a.cfg.SortMode)
			a.status = statusLine(s)
			a.screen = screenList
			return a, nil
		}
		a.status = fmt.Sprintf("cargando %s…", g.DisplayName())
		a.screen = screenLoading
		a, cmd = a.startLoad(g)
		return a, tea.Batch(cmd, a.spinner.Tick)
	case "r", "ctrl+r":
		// Recarga forzada del juego seleccionado (salta la memoria).
		g := game.All[a.pickIdx].ID
		delete(a.sessions, g)
		a.status = fmt.Sprintf("recargando %s…", g.DisplayName())
		a.screen = screenLoading
		a, cmd = a.startLoad(g)
		return a, tea.Batch(cmd, a.spinner.Tick)
	case "esc":
		return a, tea.Quit
	}
	// Atajos numéricos 1..n saltan directo al juego.
	if idx := km.String()[0]; len(km.String()) == 1 && idx >= '1' && idx <= byte('0'+n) {
		a.pickIdx = int(idx-'1') % n
	}
	return a, cmd
}

func (a App) View() string {
	if a.showHelp {
		return a.clip(a.helpView())
	}
	var v string
	if a.errMsg != "" {
		v = stPanel.Render(
			stTitle.Render(" ERROR ") + "\n\n" +
				stWinrateBad.Render(a.errMsg) +
				"\n\n" + stDim.Render("Sin datos de la fuente ni caché · esc vuelve al selector · r reintenta"))
		return a.clip(v)
	}
	switch a.screen {
	case screenPick:
		v = a.pickView()
	case screenLoading:
		v = a.loadingView()
	case screenList:
		v = a.header() + a.listMeta() + "\n" + a.list.View() + "\n" + a.hintLine() + a.footer()
	case screenDetail:
		v = a.header() + "\n" + a.detail.View() + "\n" + a.footer()
	}
	return a.clip(v)
}

// clip recorta las líneas al ancho real de la terminal: nada debe
// desbordar por la derecha.
func (a App) clip(v string) string {
	if a.width <= 0 {
		return v
	}
	return lipgloss.NewStyle().MaxWidth(a.width).Render(v)
}

// helpView: overlay centrado con todos los atajos agrupados.
func (a App) helpView() string {
	body := a.help.FullHelpView(a.fullHelp())
	box := stPanel.Render(
		stTitle.Render(" AYUDA ") + "\n\n" + body +
			"\n\n" + stDim.Render("cualquier tecla cierra"))
	return lipgloss.Place(max(a.width, 40), max(a.height, 16),
		lipgloss.Center, lipgloss.Center, box)
}

func (a App) pickView() string {
	var rows []string
	for i, g := range game.All {
		var name string
		if i == a.pickIdx {
			name = lipgloss.NewStyle().
				Background(colBorder).Foreground(colGoldLight).Bold(true).
				Render(fmt.Sprintf(" ▸ %s (%d) ", g.Name, i+1))
		} else {
			name = "   " + stKeyName.Render(g.Name) + stDim.Render(fmt.Sprintf("  (%d)", i+1))
		}
		rows = append(rows, name)
	}
	mon := stValue.Render("activo")
	if a.cfg.MonitorOff {
		mon = stDim.Render("apagado")
	}
	det := ""
	if a.detected != "" {
		det = stValue.Render(" · ● " + a.detected.DisplayName() + " detectado")
		if a.detectedChamp != "" {
			det += stValue.Render(" · " + a.detectedChamp)
		}
	}
	return stPanel.Render(
		stLogo.Render("BUILDS") + " " + stSubtitle.Render("liga de builds en tu terminal") + "\n\n" +
			strings.Join(rows, "\n") + "\n\n" +
			stHelp.Render("? todos los atajos · enter abre el seleccionado · m monitor: ") + mon + det)
}

func (a App) loadingView() string {
	body := stLogo.Render("BUILDS") + " " + stSubtitle.Render("liga de builds en tu terminal") + "\n\n" +
		a.spinner.View() + " " + a.status
	if bar := a.progressBar(); bar != "" {
		body += "\n\n" + bar
	}
	return stPanel.Render(body)
}

// progressBar dibuja el avance de la carga: una barra de 30 celdas y
// el porcentaje. Sin total conocido (fuente sin progreso) no se pinta
// nada, para no enseñar una barra que miente.
func (a App) progressBar() string {
	if a.total <= 0 {
		return ""
	}
	const cells = 30
	frac := float64(a.done) / float64(a.total)
	frac = max(0, min(1, frac))
	fill := int(frac*cells + 0.5)
	bar := lipgloss.NewStyle().Foreground(colGold).Render(strings.Repeat("█", fill)) +
		lipgloss.NewStyle().Foreground(colPanelAlt).Render(strings.Repeat("░", cells-fill))
	return bar + " " + stValue.Render(fmt.Sprintf("%3.0f%%", frac*100))
}

func (a App) header() string {
	g := a.session.Game.DisplayName()
	return stLogo.Render("◈ BUILDS") + " " +
		stTitle.Render("· "+g) + " " +
		stDim.Render("─ datos depurados por score de Wilson")
}

// footer contextual: estado + aviso temporal + atajos que aplican en
// la pantalla actual (bubbles/help trunca al ancho solo).
// listMeta: contador filtrado/total y orden activo para la cabecera
// de la lista.
func (a App) listMeta() string {
	vis, total := a.list.Counts()
	meta := stDim.Render(fmt.Sprintf("   %d de %d · Orden: ", vis, total)) +
		stValue.Render(SortModes[a.list.sortMode]) +
		stDim.Render(" (s)")
	return meta
}

// hintLine: pista de primera ejecución hasta la primera apertura.
func (a App) hintLine() string {
	if a.cfg.SeenHint {
		return ""
	}
	return stHelp.Render("  Pulsa ? para ver todos los atajos · enter abre el campeón\n")
}

func (a App) footer() string {
	toastPart := ""
	if a.toast != "" {
		toastPart = stValue.Render("· " + a.toast + " ")
	}
	detected := ""
	if a.detected != "" {
		detected = stValue.Render("● " + a.detected.DisplayName() + " en juego ")
	}
	// Aviso de versión: discreto y persistente hasta que se descarte.
	updatePart := ""
	if a.update != nil {
		label := fmt.Sprintf("Nueva versión: v%s · GitHub Releases ", a.update.Version)
		action := "· V descartar "
		if a.update.URL != "" {
			action = "· v abrir · V descartar "
		}
		updatePart = stWinrateBad.Render(label) + stDim.Render(action)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		stStatusBar.Render(a.status),
		" ",
		detected,
		toastPart,
		updatePart,
		a.help.ShortHelpView(a.shortHelp()))
}

// statusLine resume el origen de los datos de la sesión.
func statusLine(s *Session) string {
	src := "fixtures"
	if len(s.Builds) > 0 && s.Builds[0].Source != "" {
		src = s.Builds[0].Source
	}
	patch := s.Version
	if patch == "" {
		patch = "—"
	}
	who := "campeones"
	if s.Game == model.GameDeadlock {
		who = "héroes"
	}
	line := fmt.Sprintf("%s · %s · %d %s · %d builds", src, patch, len(s.Champions), who, len(s.Builds))
	line += freshnessNote(s)
	if s.Omitted > 0 {
		line += fmt.Sprintf(" · %d omitido", s.Omitted)
		if s.Omitted > 1 {
			line += "s"
		}
	}
	return line
}

// freshnessNote avisa cuando los datos no vienen frescos de la red:
// servirlos en silencio hacía pensar que eran de ahora mismo.
func freshnessNote(s *Session) string {
	switch s.DataKind {
	case "caducado":
		return fmt.Sprintf("  ⚠ datos de hace %s (sin conexión)", humanAge(s.DataAge))
	case "fixtures":
		return "  ⚠ fixtures locales (sin red ni caché)"
	case "caché":
		if s.DataAge >= time.Hour {
			return fmt.Sprintf("  ⚠ caché de hace %s", humanAge(s.DataAge))
		}
	}
	return ""
}

// humanAge comprime una antigüedad a "5 h", "3 d" o "12 min".
func humanAge(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%d d", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%d h", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return "un momento"
}
