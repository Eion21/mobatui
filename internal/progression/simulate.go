package progression

import (
	"fmt"
	"sort"
)

// Phase es la banda temporal de una compra.
type Phase int

const (
	PhaseEarly Phase = iota
	PhaseMid
	PhaseLate
)

func (p Phase) String() string {
	switch p {
	case PhaseEarly:
		return "EARLY"
	case PhaseMid:
		return "MID"
	case PhaseLate:
		return "LATE"
	}
	return "?"
}

// PhaseOf clasifica un minuto en fase, con los mismos cortes que la
// TUI (10 y 20 minutos) para que ambas vistas coincidan.
func PhaseOf(minute int) Phase {
	switch {
	case minute < 10:
		return PhaseEarly
	case minute < 20:
		return PhaseMid
	default:
		return PhaseLate
	}
}

// Origin dice de dónde viene una compra. Un candidato puede tener
// varias procedencias a la vez.
type Origin string

const (
	OriginObserved    Origin = "observed"    // aparece en la ruta observada
	OriginStatistical Origin = "statistical" // puntuada por el evaluador
	OriginCommunity   Origin = "community"   // build comunitaria verificada
	OriginProgression Origin = "progression" // necesaria para completar una cadena
	OriginCurated     Origin = "curated"     // regla curada y documentada
)

// Purchase es un paso propuesto de la ruta.
type Purchase struct {
	Item   int
	Minute int      // minuto estimado de compra
	Origin []Origin // procedencias; vacío = desconocida
}

// Step es el resultado de aplicar una compra al inventario.
type Step struct {
	Purchase
	Phase        Phase
	Consumed     []int // componentes que la mejora consumió
	Cost         int   // coste absoluto del objeto (0 = desconocido)
	Incremental  int   // coste incremental sobre la base (0 = desconocido)
	HasIncrement bool
	Inventory    []int // inventario después del paso, ordenado
	FreeSlots    int   // espacios libres después del paso
}

// Plan es una ruta simulada completa.
type Plan struct {
	Steps     []Step
	Inventory []int
	Spent     int
	Unknown   int // compras con coste desconocido
}

// Rejection explica por qué una compra no pudo aplicarse.
type Rejection struct {
	Item   int
	Reason string
}

// Simulator aplica compras sobre un inventario con capacidad limitada.
type Simulator struct {
	cat      *Catalog
	capacity int
	inv      map[int]bool
	steps    []Step
	spent    int
	unknown  int
}

// NewSimulator crea un simulador con la capacidad de inventario dada.
// Deadlock usa 12 espacios (4 por categoría × 3 categorías); un valor
// <= 0 desactiva el límite.
func NewSimulator(cat *Catalog, capacity int) *Simulator {
	return &Simulator{cat: cat, capacity: capacity, inv: map[int]bool{}}
}

// Inventory devuelve el inventario actual ordenado por id.
func (s *Simulator) Inventory() []int {
	out := make([]int, 0, len(s.inv))
	for id := range s.inv {
		out = append(out, id)
	}
	sort.Ints(out)
	return out
}

// FreeSlots es el espacio disponible (0 si no hay límite).
func (s *Simulator) FreeSlots() int {
	if s.capacity <= 0 {
		return 0
	}
	return s.capacity - len(s.inv)
}

// Apply intenta comprar un objeto. Devuelve el paso aplicado o el
// motivo del rechazo; nunca modifica el inventario si rechaza.
func (s *Simulator) Apply(p Purchase) (Step, *Rejection) {
	it, ok := s.cat.Item(p.Item)
	if !ok {
		return Step{}, &Rejection{p.Item, "objeto inexistente en el catálogo"}
	}
	if it.Disabled {
		return Step{}, &Rejection{p.Item, "objeto deshabilitado"}
	}
	if s.inv[p.Item] {
		return Step{}, &Rejection{p.Item, "ya está en el inventario"}
	}

	// Componentes presentes que esta compra consumiría.
	var consumed []int
	for _, comp := range it.Comps {
		if s.inv[comp] {
			consumed = append(consumed, comp)
		}
	}

	// Espacio: cada componente consumido libera su hueco.
	if s.capacity > 0 {
		free := s.capacity - len(s.inv) + len(consumed)
		if free < 1 {
			return Step{}, &Rejection{p.Item, fmt.Sprintf(
				"sin espacio en el inventario (%d/%d ocupados)", len(s.inv), s.capacity)}
		}
	}

	// Aplicar: consumir componentes, añadir el objeto.
	for _, comp := range consumed {
		delete(s.inv, comp)
	}
	s.inv[p.Item] = true

	step := Step{
		Purchase: p,
		Phase:    PhaseOf(p.Minute),
		Consumed: consumed,
		Cost:     it.Cost,
	}
	// Coste incremental: lo que realmente se paga en este paso.
	//
	// Si el componente está en el inventario, ya se pagó antes y la
	// mejora solo cuesta la diferencia. Si no está, se compra el objeto
	// completo y se paga su precio entero. El gasto acumulado debe
	// reflejar el oro real, no la suma de precios de catálogo: sumar el
	// coste absoluto de cada mejora cuenta dos veces el componente.
	if len(consumed) > 0 {
		baseCost := 0
		known := true
		for _, comp := range consumed {
			ci, ok := s.cat.Item(comp)
			if !ok || ci.Cost <= 0 {
				known = false
				break
			}
			baseCost += ci.Cost
		}
		if known && it.Cost > 0 && it.Cost >= baseCost {
			step.Incremental, step.HasIncrement = it.Cost-baseCost, true
		}
	}
	paid := it.Cost
	if step.HasIncrement {
		paid = step.Incremental
	}
	if it.Cost > 0 {
		s.spent += paid
	} else {
		s.unknown++
	}
	step.Inventory = s.Inventory()
	step.FreeSlots = s.FreeSlots()
	s.steps = append(s.steps, step)
	return step, nil
}

// Plan cierra la simulación y devuelve el resultado.
func (s *Simulator) Plan() Plan {
	return Plan{
		Steps:     append([]Step(nil), s.steps...),
		Inventory: s.Inventory(),
		Spent:     s.spent,
		Unknown:   s.unknown,
	}
}

// Simulate aplica una secuencia completa y devuelve el plan más los
// rechazos. Es determinista: mismo catálogo + misma secuencia = mismo
// resultado.
func Simulate(cat *Catalog, capacity int, purchases []Purchase) (Plan, []Rejection) {
	sim := NewSimulator(cat, capacity)
	var rejects []Rejection
	for _, p := range purchases {
		if _, rej := sim.Apply(p); rej != nil {
			rejects = append(rejects, *rej)
		}
	}
	return sim.Plan(), rejects
}

// Validate comprueba una secuencia sin construir el plan completo y
// devuelve el primer problema encontrado (nil si es válida).
func Validate(cat *Catalog, capacity int, purchases []Purchase) error {
	_, rejects := Simulate(cat, capacity, purchases)
	if len(rejects) > 0 {
		r := rejects[0]
		return fmt.Errorf("compra %s rechazada: %s", cat.nameOf(r.Item), r.Reason)
	}
	return nil
}

// FinalItems cuenta los objetos finales distintos de un plan: los que
// quedan en el inventario. Es la métrica que el usuario percibe como
// "objetos finales de la build".
func (p Plan) FinalItems() int { return len(p.Inventory) }

// ByPhase agrupa los pasos por fase, conservando el orden.
func (p Plan) ByPhase() map[Phase][]Step {
	out := map[Phase][]Step{}
	for _, st := range p.Steps {
		out[st.Phase] = append(out[st.Phase], st)
	}
	return out
}

// HasOrigin informa si un paso tiene una procedencia concreta.
func (p Purchase) HasOrigin(o Origin) bool {
	for _, x := range p.Origin {
		if x == o {
			return true
		}
	}
	return false
}
