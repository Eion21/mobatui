package ui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// RouteStep son los datos de la ruta de compra que consume la UI (uno
// por ítem frecuente del héroe, en orden de consenso). Llegan de
// deadlock.BuildRoute vía RunsLoader; el WR y el pick oficiales siguen
// saliendo de item-stats: la ruta aporta orden, minuto, cadena y
// tasas.
type RouteStep struct {
	Item         int
	Order        int     // posición en la ruta de consenso (1..n)
	N            int     // partidas de la muestra en las que aparece
	Share        float64 // fracción de partidas que lo compran
	MedT         int     // segundo mediano de compra
	MedNet       int     // patrimonio mediano al comprarlo (acumulado)
	PosLo, PosHi int     // posición de compra, P25–P75
	UpgradeOf    int     // base más habitual (0 = compra directa)
	ChainRate    float64 // de quienes lo compran, fracción que viene de UpgradeOf
	UpgradedRate float64 // de la base: fracción de compradores que luego la mejoran
}

// RunsLoader trae la ruta de compra de un héroe bajo demanda (al abrir
// el detalle), no para todos los héroes al arrancar. Nil, error o
// muestra insuficiente: la build se muestra sin ruta, sin números.
type RunsLoader interface {
	LoadRuns(champKey string) ([]RouteStep, error)
}

// runsLoadedMsg llega cuando la ruta del héroe terminó de cargar.
type runsLoadedMsg struct {
	ChampKey string
	Steps    []RouteStep
	Err      error
}

func loadRunsCmd(l RunsLoader, champKey string) tea.Cmd {
	return func() tea.Msg {
		steps, err := l.LoadRuns(champKey)
		return runsLoadedMsg{ChampKey: champKey, Steps: steps, Err: err}
	}
}
