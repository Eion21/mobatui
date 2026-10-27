// Package staticdata define la interfaz común de los proveedores de
// datos estáticos (nombres de héroes/campeones, ítems, runas e
// iconos). Data Dragon la implementa para LoL con caché + respaldo;
// el proveedor de Deadlock la implementa sobre deadlock-api.com.
package staticdata

import "time"

// HeroInfo es lo que la UI necesita de un héroe/campeón.
type HeroInfo struct {
	Name  string
	Title string
	ID    int // id numérico del proveedor (para matchups)
}

// ItemInfo enriquece un ítem con los metadatos que algunas fuentes
// traen (Deadlock): tier, categoría de tienda, coste y descripción.
type ItemInfo struct {
	Name        string
	Tier        int    // 1..N; 0 = desconocido
	Category    string // weapon | vitality | spirit | ""
	Cost        int    // 0 = desconocido
	Description string // texto plano (sin HTML); "" si no hay
}

// Provider resuelve nombres e iconos para un juego.
type Provider interface {
	// Version del patch/datos mostrado en la barra de estado.
	Version() (string, error)
	// Heroes indexados por clave del proveedor ("Ahri", "Abrams").
	Heroes() (map[string]HeroInfo, error)
	// Items id → nombre.
	Items() (map[int]string, error)
	// Runas id → nombre y árboles id → nombre (vacío si el juego no
	// tiene runas, p.ej. Deadlock).
	Runes() (runes map[int]string, trees map[int]string, err error)
	// ChampionIcon devuelve el PNG del icono del héroe.
	ChampionIcon(key string) ([]byte, error)
	// ItemIcon devuelve el PNG del icono del ítem.
	ItemIcon(id int) ([]byte, error)
}

// ItemDetailsProvider lo implementan las fuentes con metadatos de
// ítem (tier, categoría, coste, descripción).
type ItemDetailsProvider interface {
	ItemDetails() (map[int]ItemInfo, error)
}

// HeroNamesByIDProvider lo implementan las fuentes que pueden resolver
// id→nombre para TODOS los héroes publicados, incluidos los jugables
// marcados in_development: los endpoints de cruces (matchups,
// sinergias) los incluyen aunque el roster no los liste.
type HeroNamesByIDProvider interface {
	HeroNamesByID() map[int]string
}

// SummonerSpellsProvider lo implementan las fuentes con nombres de
// hechizos de invocador (LoL vía Data Dragon).
type SummonerSpellsProvider interface {
	SummonerSpells() (map[int]string, error)
}

// ItemUpgradesProvider lo implementan las fuentes que conocen la
// cadena de mejoras de Deadlock: mapa base → ids de sus mejoras
// directas (un ítem que lista a otro en component_items).
type ItemUpgradesProvider interface {
	ItemUpgrades() (map[int][]int, error)
}

// FreshnessProvider lo implementan las fuentes que pueden decir si lo
// que sirven viene de red, de caché caducada o de un respaldo local.
// La UI lo usa para avisar en vez de mentir con datos viejos.
type FreshnessProvider interface {
	// DataState resume el origen de los datos: "red", "caché",
	// "caducado" o "fixtures", más la antigüedad.
	DataState() (kind string, age time.Duration, ok bool)
}
