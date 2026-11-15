package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/model"
)

type routeLoadingStub struct{}

func (routeLoadingStub) LoadRuns(string) ([]RouteStep, error) { return nil, nil }

// routeLoadingApp starts with the same in-flight sentinel as opening a hero,
// but never executes the loader command or touches the network.
func routeLoadingApp(t *testing.T, count, height int) App {
	t.Helper()
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CoreItems, b.Boots = nil, nil
	s.Names.Items = map[int]string{}
	for i := 0; i < count; i++ {
		id := 9000 + i
		s.Names.Items[id] = fmt.Sprintf("Objeto prueba %02d", i)
		b.CoreItems = append(b.CoreItems, model.RankedItem{
			ItemStats: model.ItemStats{ID: id, Stats: model.Stats{Games: 20000, Wins: 10600}, AvgBuyS: float64(100 + i*100)},
			WinRate:   .53, PickRate: .4, Wilson: .523,
		})
	}
	s.Runs = routeLoadingStub{}
	d := newDetailView(champ, s)
	d.width, d.height = 120, height
	a := App{screen: screenDetail, session: s, detail: d, width: 120, height: height}
	if cmd := runsCmdFor(a); cmd == nil {
		t.Fatal("opening the hero did not start loading its route")
	}
	a.detail.sync()
	return a
}

func routeLoadingUpdate(a App, msg tea.Msg) App {
	m, _ := a.Update(msg)
	return m.(App)
}

func routeLoadingText(a App) string {
	return collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
}

func assertInOrder(t *testing.T, text string, parts ...string) {
	t.Helper()
	pos := 0
	for _, part := range parts {
		i := strings.Index(text[pos:], part)
		if i < 0 {
			t.Errorf("missing %q in order after byte %d: %s", part, pos, text)
			return
		}
		pos += i + len(part)
	}
}

func TestRouteLoadingTransitions(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  runsLoadedMsg
		ok   bool
	}{
		{name: "success", msg: runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{
			{Item: 9002, MedT: 300}, {Item: 9000, MedT: 700}, {Item: 9001, MedT: 1300},
		}}, ok: true},
		{name: "error", msg: runsLoadedMsg{ChampKey: "Abrams", Err: errors.New("sin red")}},
		{name: "empty", msg: runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := routeLoadingApp(t, 4, 100)
			loading := routeLoadingText(a)
			assertInOrder(t, loading, "Cargando ruta de compra…", "Compras frecuentes", "Objeto prueba 00", "Objeto prueba 01", "Objeto prueba 02", "Objeto prueba 03")
			for _, unwanted := range []string{"EARLY", "MID", "LATE", "SITUACIONALES", "Ruta no disponible"} {
				if strings.Contains(loading, unwanted) {
					t.Errorf("loading view contains %q", unwanted)
				}
			}

			// Select item 02 before the response; success moves it to the first route position.
			a = routeLoadingUpdate(a, keyMsg("right"))
			a = routeLoadingUpdate(a, keyMsg("right"))
			if !strings.Contains(routeLoadingText(a), "▸ Objeto prueba 02") {
				t.Fatal("precondition: item 02 was not selected")
			}
			a = routeLoadingUpdate(a, tc.msg)
			got := routeLoadingText(a)
			selected := "▸ Objeto prueba 02"
			if tc.ok {
				selected = "▸ Paso 1 · Objeto prueba 02"
			}
			if !strings.Contains(got, selected) {
				t.Errorf("selected item changed after %s: %s", tc.name, got)
			}
			if strings.Contains(got, "Cargando ruta de compra…") {
				t.Error("loading message remains after response")
			}
			if tc.ok {
				assertInOrder(t, got, "EARLY", "Objeto prueba 02", "MID", "Objeto prueba 00", "LATE", "Objeto prueba 01", "Fuera de la ruta", "Objeto prueba 03")
				if strings.Contains(got, "Ruta no disponible") || strings.Contains(got, "Compras frecuentes") {
					t.Error("successful route retains a provisional/failure label")
				}
			} else {
				assertInOrder(t, got, "Ruta no disponible — mostrando estadísticas generales", "Compras frecuentes", "Objeto prueba 00", "Objeto prueba 01", "Objeto prueba 02", "Objeto prueba 03")
				for _, unwanted := range []string{"EARLY", "MID", "LATE", "SITUACIONALES", "RUTA"} {
					if strings.Contains(got, unwanted) {
						t.Errorf("unavailable route invents section %q", unwanted)
					}
				}
			}
		})
	}
}

func TestRouteUnavailableWithoutLoader(t *testing.T) {
	a := routeLoadingApp(t, 4, 100)
	a.session.Runs = nil
	a.session.Routes = nil
	a.detail.sync()
	got := routeLoadingText(a)
	assertInOrder(t, got, "Ruta no disponible — mostrando estadísticas generales", "Compras frecuentes", "Objeto prueba 00")
	for _, unwanted := range []string{"Cargando ruta de compra…", "EARLY", "MID", "LATE"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("no loader view contains %q", unwanted)
		}
	}
}

func TestRouteArrivalForOtherHeroKeepsDetail(t *testing.T) {
	a := routeLoadingApp(t, 4, 100)
	a = routeLoadingUpdate(a, keyMsg("right"))
	before := routeLoadingText(a)
	a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "otro", Steps: []RouteStep{{Item: 9000, MedT: 300}}})
	if got := routeLoadingText(a); got != before {
		t.Error("another hero's route changed the active detail")
	}
}

func TestRouteStatusScrollFitsTerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  *runsLoadedMsg
	}{
		{name: "loading"},
		{name: "unavailable", msg: &runsLoadedMsg{ChampKey: "Abrams", Err: errors.New("sin red")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := routeLoadingApp(t, 18, 24)
			if tc.msg != nil {
				a = routeLoadingUpdate(a, *tc.msg)
			}
			for i := 0; i < 3; i++ {
				a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
			}
			if a.detail.vp.YOffset <= 2 {
				t.Fatal("precondition: viewport did not scroll past the section heading")
			}
			view := a.View()
			if !strings.Contains(view, "Compras frecuentes") {
				t.Error("scrolled view lost the section heading")
			}
			if got := lipgloss.Height(view); got > a.height {
				t.Errorf("scrolled view height %d exceeds terminal height %d", got, a.height)
			}
		})
	}
}

func TestRouteArrivalPreservesScrollAndPopup(t *testing.T) {
	for _, popup := range []bool{false, true} {
		t.Run(fmt.Sprintf("popup=%v", popup), func(t *testing.T) {
			a := routeLoadingApp(t, 18, 24)
			// Move to item 05 and then scroll independently of selection.
			for i := 0; i < 5; i++ {
				a = routeLoadingUpdate(a, keyMsg("right"))
			}
			for i := 0; i < 3; i++ {
				a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
			}
			before := a.detail.vp.YOffset
			if before == 0 {
				t.Fatal("precondition: viewport did not scroll")
			}
			if popup {
				a = routeLoadingUpdate(a, keyMsg("enter"))
				if !strings.Contains(routeLoadingText(a), "Objeto prueba 05") {
					t.Fatal("precondition: popup does not show selected item")
				}
			}
			steps := make([]RouteStep, 0, 18)
			for i := 17; i >= 0; i-- {
				steps = append(steps, RouteStep{Item: 9000 + i, MedT: 300 + (17-i)*100})
			}
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: steps})
			if a.detail.vp.YOffset != before {
				t.Errorf("route arrival moved viewport: %d → %d", before, a.detail.vp.YOffset)
			}
			got := routeLoadingText(a)
			if !strings.Contains(got, "Objeto prueba 05") {
				t.Errorf("route arrival changed selected item or popup: %s", got)
			}
			if popup {
				a = routeLoadingUpdate(a, keyMsg("esc"))
				if a.detail.vp.YOffset != before {
					t.Errorf("closing popup moved viewport: %d → %d", before, a.detail.vp.YOffset)
				}
				if !strings.Contains(routeLoadingText(a), "▸ Paso ") ||
					!strings.Contains(routeLoadingText(a), "Objeto prueba 05") {
					t.Error("selection changed after closing popup")
				}
			}
		})
	}
}
