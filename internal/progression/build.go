package progression

import (
	"fmt"
	"sort"
)

// RawItem es la forma mínima del catálogo de Deadlock que necesita el
// motor. El paquete deadlock la rellena desde su propio apiItem sin
// exponer sus tipos internos.
type RawItem struct {
	ID       int
	Name     string
	Class    string
	Slot     string
	Tier     int
	Cost     int
	Comps    []string // class_name de los componentes directos
	Disabled bool
	Shopable bool
}

// BuildCatalog arma un Catalog desde el catálogo crudo, resolviendo las
// referencias por class_name a ids. Un componente que no se puede
// resolver se descarta (y se informa en el error acumulado) en vez de
// inventar un id.
func BuildCatalog(raw []RawItem) (*Catalog, []string, error) {
	byClass := make(map[string]int, len(raw))
	for _, r := range raw {
		if r.Class != "" {
			byClass[r.Class] = r.ID
		}
	}
	var warnings []string
	items := make([]Item, 0, len(raw))
	for _, r := range raw {
		it := Item{
			ID:       r.ID,
			Name:     r.Name,
			Class:    r.Class,
			Slot:     Slot(r.Slot),
			Tier:     r.Tier,
			Cost:     r.Cost,
			Disabled: r.Disabled || !r.Shopable,
		}
		for _, class := range r.Comps {
			id, ok := byClass[class]
			if !ok {
				warnings = append(warnings, fmt.Sprintf(
					"%s: componente %q no está en el catálogo", r.Name, class))
				continue
			}
			it.Comps = append(it.Comps, id)
		}
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	cat, err := NewCatalog(items)
	if err != nil {
		return nil, warnings, err
	}
	return cat, warnings, nil
}
