// Package ui contiene la interfaz de terminal (Bubble Tea + Lip Gloss).
// theme.go centraliza la paleta y los estilos: oro y azul oscuro de la
// identidad de League of Legends, paneles con borde redondeado y
// acentos teal para datos.
package ui

import "github.com/charmbracelet/lipgloss"

// Paleta principal.
var (
	colGold      = lipgloss.Color("#C8AA6E")
	colGoldLight = lipgloss.Color("#F0E6D2")
	colTeal      = lipgloss.Color("#0AC8B9")
	colBg        = lipgloss.Color("#010A13")
	colPanel     = lipgloss.Color("#0A1428")
	colPanelAlt  = lipgloss.Color("#091428")
	colBorder    = lipgloss.Color("#785A28")
	// colGoldDim marca la pareja de cadena de la tarjeta seleccionada:
	// visible como relación, sin competir con el foco.
	colGoldDim = lipgloss.Color("#4a3b20")
	colDim     = lipgloss.Color("#5B5A56")
	colRed     = lipgloss.Color("#E84057")
	colGreen   = lipgloss.Color("#3FB68B")
	colPurple  = lipgloss.Color("#B689F0")
	colBlue    = lipgloss.Color("#5B8DEF")
)

// Estilos compartidos.
var (
	stTitle = lipgloss.NewStyle().
		Foreground(colGoldLight).
		Bold(true)

	stLogo = lipgloss.NewStyle().
		Foreground(colGold).
		Bold(true)

	stSubtitle = lipgloss.NewStyle().
			Foreground(colDim)

	stPanel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Background(colPanel).
		Padding(1, 2)

	stSection = lipgloss.NewStyle().
			Foreground(colGold).
			Bold(true).
			MarginBottom(0)

	stKeyName = lipgloss.NewStyle().
			Foreground(colGoldLight).
			Bold(true)

	stValue = lipgloss.NewStyle().
		Foreground(colTeal)

	stDim = lipgloss.NewStyle().
		Foreground(colDim)

	stWinrateGood = lipgloss.NewStyle().Foreground(colGreen)
	stWinrateBad  = lipgloss.NewStyle().Foreground(colRed)

	stSkillQ = lipgloss.NewStyle().Foreground(colGold).Bold(true)
	stSkillW = lipgloss.NewStyle().Foreground(colBlue).Bold(true)
	stSkillE = lipgloss.NewStyle().Foreground(colRed).Bold(true)
	stSkillR = lipgloss.NewStyle().Foreground(colPurple).Bold(true)

	stLevelCell = lipgloss.NewStyle().
			Foreground(colDim).
			Padding(0)

	stStatusBar = lipgloss.NewStyle().
			Foreground(colGoldLight).
			Background(lipgloss.Color("#0A1428")).
			Padding(0, 1)

	stHelp = lipgloss.NewStyle().
		Foreground(colDim)
)

// winRateStyle elige verde/rojo según la tasa (uso general).
func winRateStyle(wr float64) lipgloss.Style {
	if wr >= 0.5 {
		return stWinrateGood
	}
	return stWinrateBad
}

// Regla de acentos: dorado para títulos y selección; el win rate usa
// la escala siguiente (muestras pequeñas en gris para no engañar); el
// pick y las partidas siempre en gris tenue.
var (
	colWrGreat   = lipgloss.Color("#34d399")
	colWrGood    = lipgloss.Color("#a7f3d0")
	colWrNeutral = lipgloss.Color("#e5e7eb")
	colWrBad     = lipgloss.Color("#f87171")
)

// lowSampleGames es el suelo de partidas por debajo del cual el WR se
// atenúa: un 66% con 120 partidas engaña.
const lowSampleGames = 500

// wrStyle colorea un win rate (0..1) por umbrales reales y atenúa las
// muestras pequeñas.
func wrStyle(wr float64, games int) lipgloss.Style {
	if games < lowSampleGames {
		return stDim
	}
	p := wr * 100
	switch {
	case p >= 58:
		return lipgloss.NewStyle().Foreground(colWrGreat).Bold(true)
	case p >= 52:
		return lipgloss.NewStyle().Foreground(colWrGood)
	case p >= 48:
		return lipgloss.NewStyle().Foreground(colWrNeutral)
	default:
		return lipgloss.NewStyle().Foreground(colWrBad)
	}
}

// skillStyle devuelve el estilo de cada habilidad (letras para LoL,
// dígitos para Deadlock).
func skillStyle(sk rune) lipgloss.Style {
	switch sk {
	case 'Q', '1':
		return stSkillQ
	case 'W', '2':
		return stSkillW
	case 'E', '3':
		return stSkillE
	case 'R', '4':
		return stSkillR
	}
	return stValue
}

// skillColorOf devuelve el color crudo de la habilidad (fondos de la
// rejilla de habilidades).
func skillColorOf(sk rune) lipgloss.Color {
	switch sk {
	case 'Q', '1':
		return colGold
	case 'W', '2':
		return colBlue
	case 'E', '3':
		return colRed
	case 'R', '4':
		return colPurple
	}
	return colTeal
}
