// Package progression modela la progresión de compras de Deadlock como
// un dominio independiente de la TUI: qué objetos existen, cómo se
// encadenan las mejoras, cuánto cuesta cada paso y qué queda en el
// inventario después de cada compra.
//
// No decide qué comprar: eso es trabajo del evaluador estadístico. Este
// paquete solo garantiza que una secuencia propuesta sea VÁLIDA.
//
// Reglas duras:
//   - Nunca se inventa un objeto: todo id debe existir en el catálogo.
//   - Nunca se usa una relación de mejora que el catálogo no declare.
//   - Una mejora consume su componente (deja de ocupar espacio).
//   - No se permiten ciclos en las cadenas.
//   - No se excede la capacidad del inventario.
//   - No se exige que toda compra tenga mejora.
package progression

import (
	"fmt"
	"sort"
	"strings"
)

// Slot es la categoría de tienda de un objeto.
type Slot string

const (
	SlotWeapon   Slot = "weapon"
	SlotVitality Slot = "vitality"
	SlotSpirit   Slot = "spirit"
)

// Item es un objeto comprable del catálogo, ya normalizado.
type Item struct {
	ID       int
	Name     string
	Class    string // class_name del catálogo
	Slot     Slot
	Tier     int
	Cost     int   // coste absoluto en almas
	Comps    []int // ids de los componentes directos (mejoras)
	Disabled bool
}

// Catalog es el conjunto de objetos comprables y sus cadenas.
type Catalog struct {
	items map[int]Item
	// byClass resuelve class_name → id (el catálogo referencia por clase).
	byClass map[string]int
}

// NewCatalog construye el catálogo y valida las cadenas de mejora.
func NewCatalog(items []Item) (*Catalog, error) {
	c := &Catalog{
		items:   make(map[int]Item, len(items)),
		byClass: make(map[string]int, len(items)),
	}
	for _, it := range items {
		if it.ID == 0 {
			return nil, fmt.Errorf("progression: objeto sin id (%q)", it.Name)
		}
		if _, dup := c.items[it.ID]; dup {
			return nil, fmt.Errorf("progression: id duplicado %d", it.ID)
		}
		it.Comps = dedupe(it.Comps)
		c.items[it.ID] = it
		if it.Class != "" {
			c.byClass[it.Class] = it.ID
		}
	}
	// Toda relación de mejora debe apuntar a un objeto existente.
	for id, it := range c.items {
		for _, comp := range it.Comps {
			if comp == id {
				return nil, fmt.Errorf("progression: %q se declara componente de sí mismo", it.Name)
			}
			if _, ok := c.items[comp]; !ok {
				return nil, fmt.Errorf("progression: %q mejora de un componente inexistente (%d)", it.Name, comp)
			}
		}
	}
	if err := c.checkCycles(); err != nil {
		return nil, err
	}
	return c, nil
}

// checkCycles rechaza cadenas cíclicas: una mejora no puede depender,
// directa o transitivamente, de sí misma.
func (c *Catalog) checkCycles() error {
	const (
		white = 0 // sin visitar
		grey  = 1 // en la pila actual
		black = 2 // terminado
	)
	state := make(map[int]int, len(c.items))
	var walk func(id int, path []int) error
	walk = func(id int, path []int) error {
		switch state[id] {
		case grey:
			return fmt.Errorf("progression: ciclo de mejoras en %s", c.chainString(append(path, id)))
		case black:
			return nil
		}
		state[id] = grey
		for _, comp := range c.items[id].Comps {
			if err := walk(comp, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = black
		return nil
	}
	ids := make([]int, 0, len(c.items))
	for id := range c.items {
		ids = append(ids, id)
	}
	sort.Ints(ids) // determinismo del mensaje de error
	for _, id := range ids {
		if err := walk(id, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Catalog) chainString(ids []int) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, c.nameOf(id))
	}
	return strings.Join(parts, " → ")
}

func (c *Catalog) nameOf(id int) string {
	if it, ok := c.items[id]; ok && it.Name != "" {
		return it.Name
	}
	return fmt.Sprintf("id=%d", id)
}

// Item devuelve el objeto por id.
func (c *Catalog) Item(id int) (Item, bool) {
	it, ok := c.items[id]
	return it, ok
}

// Len es el número de objetos del catálogo.
func (c *Catalog) Len() int { return len(c.items) }

// Has informa si el objeto existe y es comprable.
func (c *Catalog) Has(id int) bool {
	it, ok := c.items[id]
	return ok && !it.Disabled
}

// Components devuelve los componentes directos de un objeto.
func (c *Catalog) Components(id int) []int {
	it, ok := c.items[id]
	if !ok {
		return nil
	}
	return append([]int(nil), it.Comps...)
}

// Upgrades devuelve los objetos que mejoran directamente a id.
func (c *Catalog) Upgrades(id int) []int {
	var out []int
	for uid, it := range c.items {
		for _, comp := range it.Comps {
			if comp == id {
				out = append(out, uid)
				break
			}
		}
	}
	sort.Ints(out) // determinismo
	return out
}

// Chain devuelve la cadena lineal de mejoras desde la base hasta id,
// en orden de compra. Si el objeto tiene varias bases, devuelve la
// primera cadena completa en orden de id (determinista); para explorar
// todas las combinaciones está ChainPaths.
func (c *Catalog) Chain(id int) []int {
	paths := c.ChainPaths(id)
	if len(paths) == 0 {
		return nil
	}
	return paths[0]
}

// ChainPaths devuelve todas las cadenas completas de componentes que
// producen id, de la base al objeto. Cada camino es determinista.
func (c *Catalog) ChainPaths(id int) [][]int {
	it, ok := c.items[id]
	if !ok {
		return nil
	}
	if len(it.Comps) == 0 {
		return [][]int{{id}}
	}
	var out [][]int
	for _, comp := range it.Comps {
		for _, sub := range c.ChainPaths(comp) {
			path := append(append([]int(nil), sub...), id)
			out = append(out, path)
		}
	}
	return out
}

// IncrementalCost es lo que cuesta pasar de la base a la mejora: el
// coste del objeto menos el de su componente. Si el catálogo no trae
// costes, devuelve 0 y ok=false (no se inventa un número).
func (c *Catalog) IncrementalCost(id, from int) (int, bool) {
	up, okUp := c.items[id]
	base, okBase := c.items[from]
	if !okUp || !okBase || up.Cost <= 0 || base.Cost <= 0 {
		return 0, false
	}
	if up.Cost < base.Cost {
		return 0, false // dato incoherente: no se inventa
	}
	return up.Cost - base.Cost, true
}

// dedupe quita ids repetidos conservando el orden.
func dedupe(ids []int) []int {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(ids))
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
