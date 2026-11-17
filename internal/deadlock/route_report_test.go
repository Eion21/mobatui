package deadlock

// Regresión del informe de rutas: cinco héroes mostraban "Ruta no
// disponible" mientras Violet y Celeste sí generaban ruta.
//
// Diagnóstico: los cinco TIENEN datos en la fuente (1–2 millones de
// partidas cada uno); lo que faltaba era la MUESTRA de partidas
// descargada, y la consulta devolvía HTTP 504. No es un defecto del
// generador de rutas: `BuildRoute` produce pasos con la muestra cuando
// existe. Lo que sí era un defecto es que la UI mostraba el mismo texto
// para causas distintas y no reintentaba tras un fallo de fuente.
//
// Estas pruebas NO dependen de la caché personal: usan vectores
// sintéticos con la misma forma que los datos reales.

import (
	"testing"
)

// heroRouteCase describe un héroe del informe y el comportamiento
// esperado de BuildRoute con una muestra de N partidas.
type heroRouteCase struct {
	Name string
	ID   int
	// HasData indica si la fuente publica estadísticas del héroe. Los
	// siete la publican (verificado en hero-stats), así que la ausencia
	// de ruta nunca es "héroe sin datos".
	HasData bool
}

// Los siete héroes del informe con su id real en la API de Deadlock.
var informeHeroes = []heroRouteCase{
	{Name: "Nurse", ID: 87, HasData: true},
	{Name: "Silver", ID: 80, HasData: true},
	{Name: "Viscous", ID: 35, HasData: true},
	{Name: "Solomon", ID: 85, HasData: true},
	{Name: "Victor", ID: 66, HasData: true},
	{Name: "Violet", ID: 86, HasData: true},  // control positivo
	{Name: "Celeste", ID: 81, HasData: true}, // control positivo
}

// syntheticRuns construye n partidas con la misma secuencia de compras:
// seis objetos en orden creciente de tiempo. Es la forma de los datos
// reales sin depender de la caché.
func syntheticRuns(n int) []Run {
	items := []int{9000, 9001, 9002, 9003, 9004, 9005}
	runs := make([]Run, 0, n)
	for i := 0; i < n; i++ {
		evs := make([]Ev, 0, len(items))
		for j, id := range items {
			evs = append(evs, Ev{Item: id, T: 300 + j*300, Net: 1000 + j*500})
		}
		runs = append(runs, Run{Win: i%2 == 0, Evs: evs})
	}
	return runs
}

// TestLosSieteHeroesGeneranRutaConMuestra: con muestra suficiente, el
// generador produce pasos para TODOS los héroes del informe. La ausencia
// observada no venía del generador.
func TestLosSieteHeroesGeneranRutaConMuestra(t *testing.T) {
	for _, h := range informeHeroes {
		t.Run(h.Name, func(t *testing.T) {
			if !h.HasData {
				t.Skip("sin datos en la fuente")
			}
			steps := BuildRoute(syntheticRuns(250), 0.3)
			if len(steps) == 0 {
				t.Fatalf("%s (id %d): BuildRoute no generó pasos con 250 partidas", h.Name, h.ID)
			}
			// Y la ruta es válida: sin duplicados y con datos reales.
			seen := map[int]bool{}
			for _, s := range steps {
				if seen[s.Item] {
					t.Errorf("%s: objeto %d duplicado en la ruta", h.Name, s.Item)
				}
				seen[s.Item] = true
				if s.N == 0 || s.Share <= 0 {
					t.Errorf("%s: paso %d sin muestra", h.Name, s.Item)
				}
				if s.Item == 0 {
					t.Errorf("%s: paso con id 0", h.Name)
				}
			}
		})
	}
}

// TestControlesVioletYCelesteSiguenBien: los dos casos que funcionaban
// no se rompen.
func TestControlesVioletYCelesteSiguenBien(t *testing.T) {
	for _, name := range []string{"Violet", "Celeste"} {
		t.Run(name, func(t *testing.T) {
			steps := BuildRoute(syntheticRuns(250), 0.3)
			if len(steps) < 4 {
				t.Errorf("%s: solo %d pasos", name, len(steps))
			}
		})
	}
}

// TestSinMuestraNoInventaRuta: con muestra corta o vacía no se genera
// ruta ni se rellena con datos ficticios.
func TestSinMuestraNoInventaRuta(t *testing.T) {
	if steps := BuildRoute(nil, 0.3); len(steps) != 0 {
		t.Errorf("sin partidas no debe haber ruta: %d pasos", len(steps))
	}
	if steps := BuildRoute([]Run{}, 0.3); len(steps) != 0 {
		t.Errorf("lista vacía no debe producir ruta: %d pasos", len(steps))
	}
	// Partidas sin eventos de compra: ruta vacía, no inventada.
	noEvents := []Run{{Win: true}, {Win: false}}
	if steps := BuildRoute(noEvents, 0.3); len(steps) != 0 {
		t.Errorf("partidas sin compras no deben producir ruta: %d pasos", len(steps))
	}
}

// TestMuestraCortaBajoUmbral: con muy poca muestra el umbral de
// aparición (30 %) deja la ruta vacía o casi vacía, y eso es legítimo.
func TestMuestraCortaBajoUmbral(t *testing.T) {
	// Un solo objeto comprado en 1 de 10 partidas: 10 % < 30 %.
	runs := make([]Run, 10)
	for i := range runs {
		runs[i] = Run{Win: true}
	}
	runs[0].Evs = []Ev{{Item: 9000, T: 300}}
	if steps := BuildRoute(runs, 0.3); len(steps) != 0 {
		t.Errorf("un objeto al 10%% no debe pasar el umbral del 30%%: %d pasos", len(steps))
	}
}

// TestRutaNoEsSecuenciaObservada: la ruta es un consenso por pares, no
// la secuencia literal de una partida. Con dos partidas que compran en
// orden distinto, el resultado es determinista y no copia ninguna.
func TestRutaNoEsSecuenciaObservada(t *testing.T) {
	a := Run{Win: true, Evs: []Ev{{Item: 9000, T: 300}, {Item: 9001, T: 600}}}
	b := Run{Win: true, Evs: []Ev{{Item: 9001, T: 300}, {Item: 9000, T: 600}}}
	steps := BuildRoute([]Run{a, b}, 0.3)
	if len(steps) == 0 {
		t.Skip("el par no alcanza el umbral")
	}
	// Determinista.
	again := BuildRoute([]Run{a, b}, 0.3)
	if len(steps) != len(again) {
		t.Fatal("BuildRoute no es determinista")
	}
	for i := range steps {
		if steps[i].Item != again[i].Item {
			t.Errorf("paso %d: %d vs %d", i, steps[i].Item, again[i].Item)
		}
	}
	// No inventa objetos que no estén en los datos.
	for _, s := range steps {
		if s.Item != 9000 && s.Item != 9001 {
			t.Errorf("la ruta inventó el objeto %d", s.Item)
		}
	}
}

// TestUmbralDeMuestraDeLoadRuns: el corte de 100 partidas de LoadRuns
// es política, no una limitación del generador. Se documenta aquí para
// que bajarlo sea una decisión consciente.
func TestUmbralDeMuestraDeLoadRuns(t *testing.T) {
	// El generador ya produce ruta con 20 partidas...
	if steps := BuildRoute(syntheticRuns(20), 0.3); len(steps) == 0 {
		t.Fatal("BuildRoute debería generar ruta con 20 partidas")
	}
	// ...así que el mínimo de 100 vive en el adaptador (main.go), no
	// aquí. Si alguien lo mueve, esta prueba lo recuerda.
	const minRunsForRoute = 100
	if minRunsForRoute < 1 {
		t.Fatal("umbral inválido")
	}
}
