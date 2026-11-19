package clean

import "math"

// z95 es el z-score para un intervalo de confianza del 95%.
const z95 = 1.96

// WilsonInterval devuelve el intervalo de confianza de Wilson al 95 %
// para una proporción wins/games: (límite inferior, límite superior).
//
// Es la ÚNICA implementación de la fórmula en el proyecto: WilsonLower
// deriva de aquí, así que el ranking del planificador y la presentación
// de incertidumbre de la UI no pueden divergir.
//
// Qué significa y qué NO significa:
//   - Es un intervalo de confianza del 95 % para la proporción
//     observada bajo muestreo aleatorio simple.
//   - NO corrige sesgos de selección (los ítems no se compran al azar:
//     se compran cuando la partida va de una manera). Un intervalo
//     estrecho solo dice que la proporción está bien medida, no que la
//     compra sea la causa del resultado.
//   - NO es una probabilidad de ganar si compras el objeto.
func WilsonInterval(wins, games int) (lo, hi float64) {
	if games <= 0 {
		return 0, 0
	}
	n := float64(games)
	p := float64(wins) / n
	z2 := z95 * z95
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	margin := z95 * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (centre - margin) / denom, (centre + margin) / denom
}

// WilsonLower devuelve el límite inferior del intervalo de confianza de
// Wilson para una proporción wins/games. Rankear por este límite (y no
// por el win rate crudo) penaliza las muestras pequeñas: un 100% con 3
// partidas puntúa por debajo de un 53% sostenido sobre 40.000.
func WilsonLower(wins, games int) float64 {
	lo, _ := WilsonInterval(wins, games)
	return lo
}
