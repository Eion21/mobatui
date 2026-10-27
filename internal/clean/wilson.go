package clean

import "math"

// z95 es el z-score para un intervalo de confianza del 95%.
const z95 = 1.96

// WilsonLower devuelve el límite inferior del intervalo de confianza de
// Wilson para una proporción wins/games. Rankear por este límite (y no
// por el win rate crudo) penaliza las muestras pequeñas: un 100% con 3
// partidas puntúa por debajo de un 53% sostenido sobre 40.000.
func WilsonLower(wins, games int) float64 {
	if games <= 0 {
		return 0
	}
	n := float64(games)
	p := float64(wins) / n
	z2 := z95 * z95
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	margin := z95 * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (centre - margin) / denom
}
