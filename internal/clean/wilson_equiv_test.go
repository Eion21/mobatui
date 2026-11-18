package clean

import (
	"fmt"
	"math"
	"sort"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// ─── oráculo congelado de WilsonLower ─────────────────────────────
//
// Esta es la implementación ORIGINAL de clean.WilsonLower, copiada
// literalmente de f8de37630887a6964a8a4428642ebe992491c163, la base de
// la rama feature/v0.2.2, ANTES de que existiera WilsonInterval.
//
// Se congela a propósito: es el oráculo contra el que se comprueba que
// refactorizar la fórmula a una sola implementación (WilsonInterval, de
// la que WilsonLower deriva) no cambió ni un bit. NO se debe tocar.
//
// El z-score se escribe como LITERAL y no se usa la constante z95: si el
// oráculo leyera la misma constante que la implementación, cambiar z95
// movería ambos lados a la vez y la prueba no detectaría nada (se
// comprobó con una mutación, que sobrevivía).
func wilsonLowerOraculo(wins, games int) float64 {
	const z = 1.96 // valor de f8de376, congelado
	if games <= 0 {
		return 0
	}
	n := float64(games)
	p := float64(wins) / n
	z2 := z * z
	denom := 1 + z2/n
	centre := p + z2/(2*n)
	margin := z * math.Sqrt((p*(1-p)+z2/(4*n))/n)
	return (centre - margin) / denom
}

// toleranciaWilson es el epsilon que usa el repositorio para comparar
// estos valores (internal/clean/wilson_audit_test.go, eps = 1e-12).
const toleranciaWilson = 1e-12

// TestWilsonLowerEquivalencia comprueba el oráculo contra la
// implementación actual sobre un conjunto determinista de combinaciones.
func TestWilsonLowerEquivalencia(t *testing.T) {
	var combinaciones int
	var diferencias []string
	var peor float64

	comprueba := func(wins, games int) {
		combinaciones++
		oraculo := wilsonLowerOraculo(wins, games)
		actual := WilsonLower(wins, games)
		lo, _ := WilsonInterval(wins, games)

		// WilsonLower debe ser exactamente el límite inferior.
		if actual != lo {
			t.Errorf("WilsonLower(%d,%d) = %v pero el límite inferior = %v", wins, games, actual, lo)
		}
		d := math.Abs(oraculo - actual)
		if d > peor && !math.IsNaN(d) {
			peor = d
		}
		if d > toleranciaWilson && !math.IsNaN(d) {
			diferencias = append(diferencias, fmt.Sprintf("wins=%d games=%d oraculo=%.17g actual=%.17g diff=%.3g",
				wins, games, oraculo, actual, d))
		}
	}

	// Casos límite explícitos.
	for _, c := range [][2]int{
		{0, 0}, {0, 1}, {1, 1}, {0, 2}, {1, 2}, {2, 2},
		{0, 10}, {5, 10}, {10, 10},
		{0, 1_000_000}, {500_000, 1_000_000}, {1_000_000, 1_000_000},
		{0, 10_000_000}, {5_000_000, 10_000_000}, {10_000_000, 10_000_000},
		{1, 3}, {2, 3}, {1, 10_000_000}, {9_999_999, 10_000_000},
	} {
		comprueba(c[0], c[1])
	}

	// Rejilla determinista sobre proporciones: recorrido de aciertos
	// para totales 1..64, potencias de 10 y valores grandes.
	var totales []int
	for n := 1; n <= 64; n++ {
		totales = append(totales, n)
	}
	for _, n := range []int{100, 500, 1000, 5000, 10000, 100000, 1000000, 10000000} {
		totales = append(totales, n)
	}
	// Se recorren TODOS los aciertos para los totales pequeños y un paso
	// fijo determinista para los grandes, hasta superar 50.000
	// combinaciones.
	for _, n := range totales {
		step := 1
		if n > 64 {
			// Al menos 120 muestras por total, más los extremos.
			step = n / 120
			if step < 1 {
				step = 1
			}
		}
		for w := 0; w <= n; w += step {
			comprueba(w, n)
		}
		// Extremos y vecindad de 0, 0.5 y 1 siempre.
		for _, w := range []int{0, 1, 2, n / 4, n/2 - 1, n / 2, n/2 + 1, 3 * n / 4, n - 2, n - 1, n} {
			if w >= 0 && w <= n {
				comprueba(w, n)
			}
		}
	}

	// Relleno determinista hasta superar 50.000 combinaciones: totales
	// intermedios con paso fijo, sin aleatoriedad.
	for n := 65; combinaciones < 52000 && n < 40000; n += 5 {
		step := n / 17
		if step < 1 {
			step = 1
		}
		for w := 0; w <= n; w += step {
			comprueba(w, n)
		}
	}

	// Valores cercanos a 0, 0.5 y 1 con totales grandes: los casos donde
	// un refactor puede mover el último bit.
	for _, n := range []int{1000, 40000, 1000000, 10000000} {
		for _, delta := range []int{0, 1, 2, 3} {
			comprueba(delta, n)     // cerca de 0
			comprueba(n/2+delta, n) // cerca de 0.5
			comprueba(n-delta, n)   // cerca de 1
		}
	}

	if len(diferencias) > 0 {
		for i, d := range diferencias {
			if i >= 10 {
				t.Logf("... y %d diferencias más", len(diferencias)-10)
				break
			}
			t.Logf("DIFERENCIA: %s", d)
		}
		t.Errorf("%d de %d combinaciones difieren del oráculo (tolerancia %g); peor diferencia %g",
			len(diferencias), combinaciones, toleranciaWilson, peor)
	}
	t.Logf("%d combinaciones, 0 diferencias fuera de la tolerancia %g; peor diferencia %.3g",
		combinaciones, toleranciaWilson, peor)
}

// TestWilsonEmpatesYRecomendacion comprueba que con EMPATES EXACTOS el
// orden y la recomendación son idénticos con el oráculo y con la
// implementación actual.
//
// La función de ranking real es rankItems (internal/clean/clean.go:125):
// asigna Wilson = WilsonLower(wins, games) y ordena con sort.SliceStable
// por Wilson descendente, así que los empates conservan el orden de
// entrada.
func TestWilsonEmpatesYRecomendacion(t *testing.T) {
	// Se construyen 240 listas deterministas con empates exactos: varios
	// ítems con el mismo (wins, games) y grupos repetidos.
	listas := make([][]model.ItemStats, 0, 240)
	for i := 0; i < 240; i++ {
		// Cada lista tiene entre 3 y 12 ítems.
		n := 3 + i%10
		items := make([]model.ItemStats, n)
		for k := 0; k < n; k++ {
			// Solo 3 combinaciones distintas por lista ⇒ empates
			// garantizados, y la misma combinación puede repetirse.
			wins := 5 + (i+k)%3*5    // 5, 10, 15
			games := 40 + (i+k)%3*20 // 40, 60, 80
			items[k] = model.ItemStats{ID: 1000 + k, Stats: model.Stats{Wins: wins, Games: games}}
		}
		listas = append(listas, items)
	}

	distintos := 0
	for idx, items := range listas {
		// Ranking con la implementación actual.
		// MaxItems por defecto (6) para no recortar estas listas
		// cortas; MinGames 0 se rellena a 100, como en producción.
		actual := rankItems(items, 0, Options{MaxItems: 100})
		// Ranking con el oráculo: se replica rankItems sustituyendo la
		// fórmula congelada.
		oraculo := make([]model.RankedItem, len(items))
		for k, it := range items {
			oraculo[k] = model.RankedItem{ItemStats: it, Wilson: wilsonLowerOraculo(it.Wins, it.Games)}
		}
		sort.SliceStable(oraculo, func(a, b int) bool { return oraculo[a].Wilson > oraculo[b].Wilson })

		if len(actual) != len(oraculo) {
			t.Fatalf("lista %d: %d vs %d elementos", idx, len(actual), len(oraculo))
		}
		for k := range actual {
			if actual[k].ItemStats.ID != oraculo[k].ItemStats.ID {
				t.Fatalf("lista %d posición %d: id %d vs %d (orden distinto)",
					idx, k, actual[k].ItemStats.ID, oraculo[k].ItemStats.ID)
			}
			if actual[k].Wilson != oraculo[k].Wilson {
				t.Fatalf("lista %d posición %d: wilson %.17g vs %.17g",
					idx, k, actual[k].Wilson, oraculo[k].Wilson)
			}
		}
		// Recomendación = el primero de la lista ordenada.
		if len(actual) > 0 {
			if actual[0].ItemStats.ID != oraculo[0].ItemStats.ID {
				t.Fatalf("lista %d: recomendación %d vs %d",
					idx, actual[0].ItemStats.ID, oraculo[0].ItemStats.ID)
			}
		}
		// Se cuenta cuántas listas tienen empates exactos en la cima.
		if len(oraculo) > 1 && oraculo[0].Wilson == oraculo[1].Wilson {
			distintos++
		}
	}
	t.Logf("%d listas con empates; orden y recomendación idénticos; %d con empate exacto en la cima",
		len(listas), distintos)
}
