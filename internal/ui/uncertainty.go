package ui

import (
	"fmt"

	"github.com/Eion21/mobatui/internal/clean"
)

// ─── presentación de incertidumbre estadística ──────────────────────
//
// Tres conceptos que NO son intercambiables y que la UI mantiene
// separados a propósito:
//
//	WR observado        la proporción medida (wins/games de la muestra)
//	incertidumbre       cuánto puede moverse ese WR por azar de muestreo
//	calidad de evidencia cuánta muestra hay y de qué tipo
//
// El ranking del planificador es un CUARTO concepto y vive en
// clean.WilsonLower: no se toca aquí. Este archivo solo decide cómo se
// PRESENTA una muestra ya calculada; no filtra, no reordena y no cambia
// qué objetos se recomiendan.
//
// Origen del problema (v0.2.1): "68.9 % WR · 148 partidas" pintaba el
// número en verde de certeza. Con 148 partidas el intervalo de Wilson
// al 95 % es amplio, y el 68.9 % describe sobre todo las circunstancias
// en que se compró el objeto, no el efecto de comprarlo.

// evidenceLevel es la calidad de la muestra, en tres tramos.
//
// Los umbrales NO son nuevos: reutilizan los que el proyecto ya tenía.
//   - lowSampleGames (500) es el suelo bajo el cual el WR ya se atenuaba
//     a gris en toda la UI (theme.go).
//   - clean.Options.MinGames (100) es el mínimo que el pipeline exige
//     para considerar una muestra usable.
//
// Un tramo intermedio (100..500) evita el salto brusco entre "usable" y
// "atenuado": es el mismo criterio, presentado en dos pasos.
type evidenceLevel uint8

const (
	// evidenceUnknown: no hay muestra o no se conoce el número de
	// partidas. No se fabrica ninguna métrica.
	evidenceUnknown evidenceLevel = iota
	// evidenceWeak: menos de clean.Options.MinGames (100) partidas.
	evidenceWeak
	// evidenceLimited: entre 100 y lowSampleGames-1 (500) partidas.
	evidenceLimited
	// evidenceSolid: lowSampleGames (500) partidas o más.
	evidenceSolid
)

// evidenceMinGames replica el mínimo del pipeline (clean.Options.MinGames
// por defecto). Se declara aquí como constante para poder citarlo en la
// UI sin construir un Options.
const evidenceMinGames = 100

// classifyEvidence traduce un tamaño de muestra a su tramo.
func classifyEvidence(games int) evidenceLevel {
	switch {
	case games <= 0:
		return evidenceUnknown
	case games < evidenceMinGames:
		return evidenceWeak
	case games < lowSampleGames:
		return evidenceLimited
	default:
		return evidenceSolid
	}
}

// sampleLabel es la etiqueta corta de calidad de muestra.
//
// En Ruta y Alternativas solo caben unas pocas palabras, así que el
// término es corto y estable: "muestra limitada" / "muestra insuficiente".
func sampleLabel(games int) string {
	switch classifyEvidence(games) {
	case evidenceWeak:
		return "muestra insuficiente"
	case evidenceLimited:
		return "muestra limitada"
	default:
		return ""
	}
}

// sampleSuffix devuelve " · muestra limitada" (o vacío). Se compone con
// el texto de partidas que ya se pinta para no repetir el recuento.
func sampleSuffix(games int) string {
	if label := sampleLabel(games); label != "" {
		return " · " + label
	}
	return ""
}

// wilsonBand devuelve el intervalo de Wilson al 95 % como texto corto,
// o "" si no hay muestra.
//
// Caso importante: en LoL los datos de Lolalytics traen games y WR,
// pero NO un recuento de victorias observado; Wins se DERIVA con
// int(wr/100*games). Presentar eso como "el intervalo de la muestra"
// sería vender una precisión que no existe. Por eso la UI solo muestra
// la banda cuando la llama con un Stats cuyas victorias son reales (el
// total campeón/rol y el WR de un paso comprado, ambos publicados como
// n + wr), y el popup la etiqueta siempre como "si la muestra fuera
// aleatoria".
func wilsonBand(stats fmtStats) string {
	if stats.Games <= 0 {
		return ""
	}
	lo, hi := clean.WilsonInterval(stats.Wins, stats.Games)
	return fmt.Sprintf("%s–%s", pct(lo), pct(hi))
}

// fmtStats es el mínimo que la presentación necesita de una muestra.
type fmtStats struct {
	Games int
	Wins  int
}

// evidenceWord es la etiqueta cualitativa de la calidad de la muestra,
// para el popup (donde hay espacio para la palabra completa).
func evidenceWord(games int) string {
	switch classifyEvidence(games) {
	case evidenceUnknown:
		return "sin muestra"
	case evidenceWeak:
		return "insuficiente"
	case evidenceLimited:
		return "limitada"
	default:
		return "sólida"
	}
}

// uncertaintyExplanation explica en una frase qué significa la
// etiqueta. Va en el popup / ayuda contextual, nunca en Ruta.
//
// Se mantiene por debajo de ~46 columnas: el popup tiene 50 celdas
// útiles y una frase más larga se parte en tres líneas y deja de
// leerse.
func uncertaintyExplanation(games int) string {
	switch classifyEvidence(games) {
	case evidenceUnknown:
		return "Sin partidas de muestra: no se puede estimar."
	case evidenceWeak:
		return fmt.Sprintf("Menos de %d partidas: el WR puede moverse mucho por azar.",
			evidenceMinGames)
	case evidenceLimited:
		return fmt.Sprintf("Entre %d y %d partidas: el WR es orientativo.",
			evidenceMinGames, lowSampleGames)
	default:
		return fmt.Sprintf("Desde %d partidas el WR está bien medido.", lowSampleGames)
	}
}

// wilsonCaveat aclara qué es (y qué no es) el intervalo de Wilson. Una
// sola vez, en el popup.
//
// Se queda en "solo error de muestreo" porque el popup tiene 44 celdas
// útiles: la parte de sesgo de selección la cubre biasExplanation, y
// repetirla aquí partiría la línea.
func wilsonCaveat() string {
	return "Wilson 95%: solo error de muestreo."
}

// biasWarning es el aviso de sesgo de selección.
//
// Antes decía "⚠ se compra yendo por delante", que afirma un
// comportamiento de las partidas que los datos agregados NO demuestran:
// solo se sabe que el WR del objeto supera al del campeón. La frase
// nueva describe el dato y nombra la explicación probable sin
// presentarla como hecho comprobado.
func biasWarning(delta float64) string {
	if delta < deltaAviso {
		return ""
	}
	return "⚠ WR elevado; posible sesgo de selección"
}

// biasExplanation es la versión larga del aviso, para el popup.
//
// No afirma que el sesgo exista: dice que el WR PUEDE reflejar las
// circunstancias de compra y que no demuestra causalidad. Tampoco
// promete que Wilson o el Beta-Binomial limpien el sesgo: no lo hacen,
// porque corrigen error de muestreo, no de selección.
func biasExplanation() string {
	return "Este WR puede reflejar las circunstancias en que se compra el objeto. " +
		"No demuestra que comprarlo aumente la probabilidad de victoria."
}

// wrWithSample pinta un WR ya coloreado y le añade la etiqueta de
// muestra cuando corresponde: "68.9% · muestra limitada".
//
// Regla de color: un WR de muestra débil o limitada NO usa el verde
// intenso de certeza aunque el número sea alto. wrStyle ya lo resuelve
// atenuando por debajo de lowSampleGames; aquí solo se garantiza que la
// etiqueta acompañe al número para que el color no contradiga el texto.
func wrWithSample(wr float64, games int) string {
	return wrStyle(wr, games).Render(pct(wr)) + stDim.Render(sampleSuffix(games))
}
