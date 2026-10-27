// Package game define las especificaciones de cada juego soportado:
// nombre visible y roles aplicables. Añadir un juego nuevo es añadir
// su Spec aquí, sus reglas de skills en model.Game.SkillRules y su
// adaptador + proveedor estático.
package game

import "github.com/Eion21/builds-tui/internal/model"

// Spec es la configuración de un juego para la UI y el agrupado.
type Spec struct {
	ID    model.Game
	Name  string
	Roles []model.Role // orden canónico; un solo elemento si no hay roles
}

// All en el orden en que se ofrecen en el selector.
var All = []Spec{
	{
		ID:    model.GameLoL,
		Name:  "League of Legends",
		Roles: model.AllRoles,
	},
	{
		ID:    model.GameDeadlock,
		Name:  "Deadlock",
		Roles: []model.Role{model.RoleAll},
	},
}

// For devuelve la Spec del juego indicado (LoL por defecto).
func For(g model.Game) Spec {
	for _, s := range All {
		if s.ID == g {
			return s
		}
	}
	return All[0]
}
