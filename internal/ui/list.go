package ui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/model"
)

// champItem adapta un ChampEntry a la lista de bubbles: un campeón por
// fila, con todos sus roles resumidos en la descripción.
type champItem struct {
	champ   *ChampEntry
	session *Session
}

func (i champItem) Title() string {
	return i.champ.Name
}

func (i champItem) Description() string {
	b := i.champ.Build(i.champ.MainRoleIdx())
	if b == nil {
		// Campeón sin rol canónico: no hay build que describir.
		return stDim.Render("sin datos")
	}
	top := "—"
	if len(b.CoreItems) > 0 {
		top = i.session.ItemName(b.CoreItems[0].ID)
	}
	// Juegos sin roles (Deadlock): solo WR y partidas.
	if len(i.champ.Roles) == 1 && i.champ.Roles[0] == model.RoleAll {
		return fmt.Sprintf("WR %s · %s partidas · top: %s",
			winRateStyle(b.WinRate).Render(pct(b.WinRate)),
			stDim.Render(nThousand(b.Total.Games)),
			top)
	}
	parts := make([]string, 0, len(i.champ.Roles))
	for _, r := range i.champ.Roles {
		b := i.champ.ByRole[r]
		parts = append(parts, fmt.Sprintf("%s %s",
			roleShort(r),
			winRateStyle(b.WinRate).Render(pct(b.WinRate))))
	}
	return strings.Join(parts, stDim.Render(" · ")) +
		stDim.Render("  │  top: ") + top
}

// FilterValue indexa el nombre del campeón para el filtro.
func (i champItem) FilterValue() string {
	return i.champ.Name
}

// roleShort abrevia el rol para la descripción compacta.
func roleShort(r model.Role) string {
	switch r {
	case model.RoleTop:
		return "TOP"
	case model.RoleJungle:
		return "JG"
	case model.RoleMiddle:
		return "MID"
	case model.RoleBottom:
		return "ADC"
	case model.RoleSupport:
		return "SUP"
	}
	return r.String()
}

// buildList envuelve la lista de bubbles con los tipos del dominio.
type buildList struct {
	list.Model
	session *Session
	// sortMode: 0 WR desc · 1 partidas desc · 2 nombre A-Z.
	sortMode int
	// filterBuf acumula el texto del filtro propio (SetFilterText).
	filterBuf string
}

// SortModes son los órdenes disponibles, en ciclo con `tab`.
var SortModes = []string{"WR ▼", "Partidas ▼", "Nombre"}

func newBuildList() buildList {
	l := list.New([]list.Item{}, newListDelegate(), 60, 12)
	l.Title = "Campeones"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = stSection
	l.Styles.TitleBar = lipgloss.NewStyle().Padding(0, 0, 0, 1)
	l.FilterInput.Prompt = stValue.Render("filtrar: ")
	// El filtro lo lleva buildList (escribir = filtrar): la tecla `/`
	// de bubbles abriría un segundo modo de filtrado que pelea con él.
	l.KeyMap.Filter.SetEnabled(false)
	return buildList{Model: l}
}

// reservedRunes: teclas con función propia en la lista que no deben
// arrancar el filtro al escribir. Solo `q`, para que nadie quede
// atrapado sin salir; el resto de letras escribe. `esc` y `tab` no son
// letras, así que no colisionan.
var reservedRunes = map[rune]bool{
	'q': true, // salir
}

func newListDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalTitle = stKeyName
	d.Styles.NormalDesc = stDim
	d.Styles.SelectedTitle = lipgloss.NewStyle().
		Foreground(colGoldLight).Bold(true).
		BorderLeft(true).BorderLeftForeground(colGold).PaddingLeft(1)
	d.Styles.SelectedDesc = lipgloss.NewStyle().
		Foreground(colTeal).
		BorderLeft(true).BorderLeftForeground(colGold).PaddingLeft(1)
	d.SetHeight(2)
	return d
}

// sortChampions ordena una copia de la lista según el modo.
func sortChampions(champs []*ChampEntry, mode int) []*ChampEntry {
	out := append([]*ChampEntry(nil), champs...)
	switch mode {
	case 1: // partidas
		sort.SliceStable(out, func(i, j int) bool {
			return mainGames(out[i]) > mainGames(out[j])
		})
	case 2: // nombre
		sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	default: // WR por límite inferior de Wilson
		// Ordenar por el WR crudo pone arriba a quien tiene 3 partidas
		// al 100%: Wilson penaliza las muestras pequeñas, igual que en
		// ítems y runas.
		sort.SliceStable(out, func(i, j int) bool {
			wi, wj := mainWilson(out[i]), mainWilson(out[j])
			if wi != wj {
				return wi > wj
			}
			return mainWR(out[i]) > mainWR(out[j])
		})
	}
	return out
}

// mainWR/mainGames: métricas del rol principal para ordenar la lista.
// Build() ya resuelve el índice con guard, así que un campeón sin roles
// no puede desbordar aquí.
func mainWR(c *ChampEntry) float64 {
	if b := c.Build(c.MainRoleIdx()); b != nil {
		return b.WinRate
	}
	return 0
}

func mainGames(c *ChampEntry) int {
	if b := c.Build(c.MainRoleIdx()); b != nil {
		return b.Total.Games
	}
	return 0
}

// mainWilson es el límite inferior de Wilson del rol principal: la
// métrica con la que se ordena "WR ▼" para no premiar muestras
// pequeñas.
func mainWilson(c *ChampEntry) float64 {
	if b := c.Build(c.MainRoleIdx()); b != nil {
		return clean.WilsonLower(b.Total.Wins, b.Total.Games)
	}
	return 0
}

// LoadSession rellena la lista con un ítem por campeón/héroe, ordenado
// según el modo.
func (bl *buildList) LoadSession(s *Session, sortMode int) {
	bl.session = s
	bl.sortMode = sortMode
	bl.filterBuf = ""
	bl.Model.ResetFilter()
	bl.Title = "Campeones"
	if s.Game == model.GameDeadlock {
		bl.Title = "Héroes"
	}
	items := make([]list.Item, 0, len(s.Champions))
	for _, c := range sortChampions(s.Champions, sortMode) {
		items = append(items, champItem{champ: c, session: s})
	}
	_ = bl.Model.SetItems(items)
}

// SelectChampion mueve el cursor al campeón indicado (restaurar la
// última sesión); false si no está.
func (bl *buildList) SelectChampion(key string) bool {
	for i, it := range bl.Items() {
		if ci, ok := it.(champItem); ok && ci.champ.Key == key {
			bl.Select(i)
			return true
		}
	}
	return false
}

// CycleSort avanza al siguiente orden y reordena la lista.
func (bl *buildList) CycleSort() string {
	bl.sortMode = (bl.sortMode + 1) % len(SortModes)
	if bl.session != nil {
		items := make([]list.Item, 0, len(bl.session.Champions))
		for _, c := range sortChampions(bl.session.Champions, bl.sortMode) {
			items = append(items, champItem{champ: c, session: bl.session})
		}
		_ = bl.Model.SetItems(items)
	}
	return SortModes[bl.sortMode]
}

// Counts devuelve (visibles, total) respetando el filtro.
func (bl buildList) Counts() (int, int) {
	return len(bl.VisibleItems()), len(bl.Items())
}

// SelectedChampion devuelve el campeón bajo el cursor (nil si no hay).
func (bl buildList) SelectedChampion() *ChampEntry {
	it, ok := bl.SelectedItem().(champItem)
	if !ok {
		return nil
	}
	return it.champ
}

// FilterActive expone si el filtro está en uso (escribiendo o
// aplicado).
func (bl buildList) FilterActive() bool {
	return bl.FilterState() != list.Unfiltered
}

// Update enruta los mensajes y filtra al escribir: la primera letra
// no reservada activa el filtro; mientras está aplicado, letras y
// dígitos lo refinan, backspace borra y esc cancela (desde App).
func (bl buildList) Update(msg tea.Msg) (buildList, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		s := km.String()
		switch bl.FilterState() {
		case list.Unfiltered:
			if len(s) == 1 {
				if r := []rune(s)[0]; unicode.IsLetter(r) && !reservedRunes[unicode.ToLower(r)] {
					bl.filterBuf = s
					bl.Model.SetFilterText(s)
					return bl, nil
				}
			}
		case list.FilterApplied:
			switch {
			case s == "esc":
				bl.filterBuf = ""
				bl.Model.ResetFilter()
				return bl, nil
			case s == "backspace":
				if rs := []rune(bl.filterBuf); len(rs) > 0 {
					bl.filterBuf = string(rs[:len(rs)-1])
				}
				if bl.filterBuf == "" {
					bl.Model.ResetFilter()
				} else {
					bl.Model.SetFilterText(bl.filterBuf)
				}
				return bl, nil
			case len(s) == 1 && (unicode.IsLetter([]rune(s)[0]) || unicode.IsDigit([]rune(s)[0])):
				bl.filterBuf += s
				bl.Model.SetFilterText(bl.filterBuf)
				return bl, nil
			}
		}
	}
	m, cmd := bl.Model.Update(msg)
	bl.Model = m
	return bl, cmd
}

func (bl *buildList) Resize(w, h int) {
	// Reserva 5 líneas de cabecera y pie.
	bl.Model.SetSize(w, max(6, h-5))
}

// pct formatea 0.539 → "53.9%".
func pct(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}

// nThousand formatea 48213 → "48.2k".
func nThousand(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}
