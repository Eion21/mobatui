// Evaluación estadística de candidatos de compra.
//
// El algoritmo anterior (BuildRoute) filtra por share y ordena por
// precedencia. Este evaluador NO lo reemplaza: produce una puntuación
// por candidato con su incertidumbre, para que el constructor híbrido
// pueda decidir con más información que "cuánta gente lo compra".
//
// Reglas:
//   - No se interpreta un WR alto como causalidad.
//   - La confianza depende de muestra, incertidumbre y consistencia,
//     no solo del win rate.
//   - Los parámetros previos del Beta-Binomial salen de la muestra
//     agregada del propio héroe, no de constantes elegidas a gusto.
package deadlock

import (
	"math"
	"sort"

	"github.com/Eion21/mobatui/internal/clean"
)

// Candidate es un objeto evaluado para entrar en una build.
type Candidate struct {
	Item  int
	N     int     // partidas de la muestra en las que aparece
	Share float64 // fracción de la muestra que lo compra
	// ShareReached es la fracción de partidas que ALCANZARON la fase
	// del objeto y lo compraron. Denominador honesto: una compra tardía
	// no se divide entre partidas que terminaron antes.
	ShareReached float64
	ReachedN     int // partidas que alcanzaron la fase del objeto
	MedT         int // segundo mediano de compra
	Phase        int // 0 EARLY, 1 MID, 2 LATE

	WR     float64 // win rate observado en la muestra
	Wilson float64 // límite inferior de Wilson 95 %
	// Posterior es la media posterior Beta-Binomial con el prior del
	// héroe. Nunca se usa como prueba causal.
	Posterior  float64
	PostLo     float64 // cuantil 5 % de la posterior
	PostHi     float64 // cuantil 95 % de la posterior
	PriorAlpha float64
	PriorBeta  float64

	// Consistency es la estabilidad entre particiones de la muestra
	// (1 = aparece en todas las mitades con share similar).
	Consistency float64
	// Confidence es la confianza global (0..1), derivada de muestra,
	// incertidumbre y consistencia.
	Confidence float64
	// Score es la puntuación final usada para ordenar.
	Score float64
	// Origins son las procedencias del candidato.
	Origins []string
	// Notes explica por qué entró o quedó fuera.
	Notes []string
}

// EvalOptions parametriza el evaluador. Los ceros activan los valores
// por defecto, todos documentados.
type EvalOptions struct {
	// MinShare es el umbral de aparición de la ruta observada. Se
	// conserva en 0.30 durante la migración.
	MinShare float64
	// MinN es la muestra mínima para considerar un candidato.
	MinN int
	// PhaseCuts son los minutos que separan EARLY/MID/LATE.
	PhaseEarly, PhaseMid int
	// PriorWeight es el peso (en partidas equivalentes) del prior del
	// héroe en la estimación Beta-Binomial. 0 = derivado de la muestra.
	PriorWeight float64
}

func (o *EvalOptions) fill() {
	if o.MinShare == 0 {
		o.MinShare = 0.30
	}
	if o.MinN == 0 {
		o.MinN = 20
	}
	if o.PhaseEarly == 0 {
		o.PhaseEarly = 10
	}
	if o.PhaseMid == 0 {
		o.PhaseMid = 20
	}
}

// phaseOfMin clasifica un minuto en fase con los cortes configurados.
func (o EvalOptions) phaseOfMin(min int) int {
	switch {
	case min < o.PhaseEarly:
		return 0
	case min < o.PhaseMid:
		return 1
	default:
		return 2
	}
}

// Evaluate puntúa todos los candidatos de una muestra de partidas.
// Devuelve la lista ordenada por Score descendente.
func Evaluate(runs []Run, opts EvalOptions) []Candidate {
	opts.fill()
	if len(runs) == 0 {
		return nil
	}
	total := len(runs)

	// Denominadores por fase: una partida "alcanzó" una fase si su
	// última compra registrada llega a esa fase. Es el mejor
	// denominador disponible sin duración de partida en la muestra.
	reached := make([]int, 3)
	for _, r := range runs {
		last := 0
		for _, e := range r.Evs {
			if e.T > last {
				last = e.T
			}
		}
		for p := 0; p < 3; p++ {
			if opts.phaseOfMin(last/60) >= p {
				reached[p]++
			}
		}
	}

	type acc struct {
		t, net []int
		wins   int
	}
	by := map[int]*acc{}
	for _, r := range runs {
		seen := map[int]bool{}
		for _, e := range r.Evs {
			if seen[e.Item] {
				continue
			}
			seen[e.Item] = true
			a := by[e.Item]
			if a == nil {
				a = &acc{}
				by[e.Item] = a
			}
			a.t = append(a.t, e.T)
			a.net = append(a.net, e.Net)
			if r.Win {
				a.wins++
			}
		}
	}

	// Prior del héroe: el WR agregado de la muestra, con peso igual a
	// la mediana de partidas por objeto. No es una constante elegida
	// para producir un resultado: sale de los datos.
	heroWins, heroGames := 0, 0
	for _, r := range runs {
		heroGames++
		if r.Win {
			heroWins++
		}
	}
	heroWR := 0.5
	if heroGames > 0 {
		heroWR = float64(heroWins) / float64(heroGames)
	}
	priorWeight := opts.PriorWeight
	if priorWeight <= 0 {
		// El prior pesa lo mismo que la muestra mínima que ya
		// consideramos utilizable (MinN). No se deriva del N del
		// propio objeto: si el peso creciera con la muestra, la
		// posterior no se encogería en muestras pequeñas y la
		// regularización no haría nada.
		priorWeight = float64(opts.MinN)
		if priorWeight < 1 {
			priorWeight = 1
		}
	}
	priorAlpha := heroWR * priorWeight
	priorBeta := (1 - heroWR) * priorWeight

	var out []Candidate
	for id, a := range by {
		n := len(a.t)
		if n < opts.MinN {
			continue
		}
		medT := pctDev(a.t, 0.5)
		phase := opts.phaseOfMin(medT / 60)
		share := float64(n) / float64(total)

		c := Candidate{
			Item: id, N: n, Share: share,
			MedT: medT, Phase: phase,
			WR:         float64(a.wins) / float64(n),
			Wilson:     clean.WilsonLower(a.wins, n),
			PriorAlpha: priorAlpha, PriorBeta: priorBeta,
		}
		// Denominador por fase: partidas que llegaron a esa fase.
		if reached[phase] > 0 {
			c.ReachedN = reached[phase]
			c.ShareReached = float64(n) / float64(reached[phase])
		}
		// Posterior Beta-Binomial con el prior del héroe.
		c.Posterior = (priorAlpha + float64(a.wins)) / (priorAlpha + priorBeta + float64(n))
		c.PostLo, c.PostHi = betaQuantiles(priorAlpha+float64(a.wins), priorBeta+float64(n-a.wins))

		// Consistencia entre mitades de la muestra.
		c.Consistency = consistency(runs, id)

		c.Confidence = confidence(c)
		c.Score = score(c)
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Item < out[j].Item // determinismo
	})
	return out
}

// consistency mide si el objeto aparece con una frecuencia parecida en
// las dos mitades de la muestra. 1 = idéntico, 0 = solo en una mitad.
func consistency(runs []Run, item int) float64 {
	if len(runs) < 4 {
		return 0
	}
	half := len(runs) / 2
	count := func(rs []Run) (int, int) {
		n := 0
		for _, r := range rs {
			for _, e := range r.Evs {
				if e.Item == item {
					n++
					break
				}
			}
		}
		return n, len(rs)
	}
	a, na := count(runs[:half])
	b, nb := count(runs[half:])
	if na == 0 || nb == 0 {
		return 0
	}
	fa, fb := float64(a)/float64(na), float64(b)/float64(nb)
	if fa+fb == 0 {
		return 0
	}
	// Diferencia relativa normalizada.
	diff := math.Abs(fa-fb) / math.Max(fa, fb)
	return math.Max(0, 1-diff)
}

// confidence combina muestra, incertidumbre y consistencia. No usa el
// win rate como atajo: un objeto con WR alto y muestra pequeña no es
// "alta confianza".
func confidence(c Candidate) float64 {
	// Muestra: satura a partir de ~200 observaciones.
	sample := math.Min(1, math.Log1p(float64(c.N))/math.Log1p(200))
	// Incertidumbre: ancho del intervalo posterior (menos es mejor).
	width := c.PostHi - c.PostLo
	uncertainty := math.Max(0, 1-width/0.20) // 20 puntos de ancho = 0
	// Consistencia entre mitades.
	cons := c.Consistency
	return 0.45*sample + 0.30*uncertainty + 0.25*cons
}

// score combina popularidad, rendimiento ajustado y confianza. Los
// pesos son explícitos y documentados; no se ajustaron para producir
// un resultado concreto.
//
// El rendimiento se pondera por la confianza a propósito: una ventaja
// de WR sobre muestra pequeña no puede desbancar a un básico que se
// compra en 9 de cada 10 partidas. Sin ese factor, un objeto con
// share 0.19 y posterior alta se colaba por delante de Improved Spirit
// (share 0.87) en la muestra real de Violet — el sesgo de "mayor WR
// = mejor" que el diseño prohíbe.
func score(c Candidate) float64 {
	// Popularidad: raíz del share (comprime la cola larga).
	pop := math.Sqrt(math.Max(0, c.Share))
	// Rendimiento ajustado: posterior menos el 50 % neutro, acotado.
	perf := (c.Posterior - 0.5) * 4
	if perf > 1 {
		perf = 1
	}
	if perf < -1 {
		perf = -1
	}
	// La ventaja de rendimiento solo cuenta en la medida en que la
	// evidencia la sostiene.
	return 1.0*pop + 1.2*perf*c.Confidence + 0.8*c.Confidence
}

// ConfidenceLabel traduce la confianza numérica a una etiqueta legible.
func ConfidenceLabel(c float64) string {
	switch {
	case c >= 0.70:
		return "alta"
	case c >= 0.45:
		return "moderada"
	default:
		return "insuficiente"
	}
}

// medianInt devuelve la mediana de una lista de enteros (0 si vacía).
func medianInt(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	return pctDev(xs, 0.5)
}

// betaQuantiles aproxima los cuantiles 5 % y 95 % de una Beta(a,b) por
// la normal con la varianza exacta de la Beta. Es una aproximación
// declarada, no un cálculo exacto: suficiente para etiquetar
// incertidumbre y nunca usada como prueba.
func betaQuantiles(a, b float64) (lo, hi float64) {
	if a <= 0 || b <= 0 {
		return 0, 1
	}
	mean := a / (a + b)
	variance := (a * b) / ((a + b) * (a + b) * (a + b + 1))
	sd := math.Sqrt(variance)
	lo = clamp01(mean - 1.645*sd)
	hi = clamp01(mean + 1.645*sd)
	return lo, hi
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
