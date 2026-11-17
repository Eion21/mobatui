package ui

import (
	"strings"

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
	SampleSize   int     // partidas usadas para calcular la ruta; 0 = desconocido
	MinShare     float64 // umbral de aparición; 0 = desconocido
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

// ─── build híbrida ─────────────────────────────────────────────────

// HybridStep es un paso de la build híbrida, ya resuelto por el motor
// de dominio. La UI NO calcula estadística: solo presenta estos datos.
type HybridStep struct {
	Item       int
	Name       string
	Phase      int     // 0 EARLY, 1 MID, 2 LATE
	Minute     int     // minuto estimado de compra
	Confidence float64 // 0..1; NO es probabilidad de victoria
	Share      float64 // fracción de partidas que lo compra
	// ConsumedBy es el objeto posterior que consume a este como
	// componente (0 = permanece en el inventario final).
	ConsumedBy int
	// Chain es la cadena completa de componentes, en orden de compra.
	Chain []int
	// Origin son las procedencias: observed, statistical, progression.
	Origin []string
	// Reason es la justificación de una línea.
	Reason string
}

// HybridSection agrupa la build por función.
type HybridSection int

const (
	SectionStandard HybridSection = iota
	SectionExtensions
	SectionSituationals
)

// HybridBuildView es la build híbrida lista para pintar.
type HybridBuildView struct {
	// Key identifica de qué datos salió esta build (muestra, catálogo,
	// parche, filtros). Si la clave vigente cambia, la build guardada
	// deja de usarse: nunca se sirve un resultado obsoleto.
	Key          string
	Standard     []HybridStep
	Extensions   []HybridStep
	Situationals []HybridStep
	// Confidence es la confianza global (el eslabón más débil).
	Confidence float64
	// FinalItems es el número de objetos que quedan en el inventario.
	FinalItems int
	// Spent es el gasto total en almas (0 = desconocido).
	Spent int
}

// BuildLoader trae la build híbrida de un héroe bajo demanda. Devuelve
// nil sin error cuando la muestra no alcanza: la UI cae a la ruta
// observada.
type BuildLoader interface {
	LoadHybrid(champKey string) (*HybridBuildView, error)
}

// hybridLoadedMsg llega cuando la build híbrida terminó de calcularse.
type hybridLoadedMsg struct {
	ChampKey string
	Key      string
	Build    *HybridBuildView
	Err      error
}

func loadHybridCmd(l BuildLoader, champKey, key string) tea.Cmd {
	return func() tea.Msg {
		b, err := l.LoadHybrid(champKey)
		return hybridLoadedMsg{ChampKey: champKey, Key: key, Build: b, Err: err}
	}
}

// routeReason explica POR QUÉ un héroe no tiene ruta, en una línea
// breve para la TUI. Distingue la causa real en lugar de mostrar el
// mismo texto para toda ausencia: un fallo de red, una muestra corta y
// una fuente sin partidas se arreglan de forma muy distinta.
type routeReason int

const (
	routeReasonNone routeReason = iota
	// routeReasonNoSample: la fuente respondió, pero no hay partidas
	// suficientes para destilar una ruta.
	routeReasonNoSample
	// routeReasonSource: la consulta falló (red, timeout, HTTP).
	routeReasonSource
	// routeReasonUnknown: héroe desconocido para el catálogo.
	routeReasonUnknown
)

// short devuelve el texto para la TUI.
func (r routeReason) short() string {
	switch r {
	case routeReasonNoSample:
		return "sin muestra de compras suficiente"
	case routeReasonSource:
		return "fuente no disponible"
	case routeReasonUnknown:
		return "héroe fuera del catálogo"
	default:
		return ""
	}
}

// classifyRunsError traduce el error del loader a una razón de UI.
func classifyRunsError(err error) routeReason {
	if err == nil {
		return routeReasonNone
	}
	if strings.Contains(err.Error(), "desconocido") {
		return routeReasonUnknown
	}
	return routeReasonSource
}
