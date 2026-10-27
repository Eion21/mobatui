// Package model define los tipos de dominio normalizados de builds de
// juegos MOBA (League of Legends, Deadlock), independientes del formato
// de origen. Los adaptadores de cada fuente traducen su JSON crudo a
// estos tipos, y el resto de la aplicación solo consume model.
package model

import "fmt"

// Game identifica el juego de una build.
type Game string

const (
	GameLoL      Game = "lol"
	GameDeadlock Game = "deadlock"
)

// SkillRules son las reglas matemáticas del orden de habilidades de un
// juego: alfabeto de habilidades, longitud de la secuencia, tope de
// puntos por habilidad y (opcional) los niveles donde una habilidad
// está bloqueada a aparecer en exclusiva (la R de LoL en 6/11/16).
// El tope es un máximo: la longitud de la secuencia fuerza el reparto
// total, así que en juegos donde la suma de topes coincide con la
// longitud (LoL, Deadlock) el reparto es exacto.
type SkillRules struct {
	Alphabet  []rune
	Length    int
	MaxPoints map[rune]int
	LockedAt  map[rune][]int // niveles 1-based; vacío = libre
}

// SkillRules devuelve las reglas del juego.
func (g Game) SkillRules() SkillRules {
	switch g {
	case GameDeadlock:
		return SkillRules{
			Alphabet:  []rune{'1', '2', '3', '4'},
			Length:    18,
			MaxPoints: map[rune]int{'1': 5, '2': 5, '3': 5, '4': 3},
		}
	default: // League of Legends
		return SkillRules{
			Alphabet:  []rune{'Q', 'W', 'E', 'R'},
			Length:    18,
			MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
			LockedAt:  map[rune][]int{'R': {6, 11, 16}},
		}
	}
}

// DisplayName devuelve el nombre del juego para la UI.
func (g Game) DisplayName() string {
	switch g {
	case GameDeadlock:
		return "Deadlock"
	default:
		return "League of Legends"
	}
}

// Role es una posición del mapa. Deadlock no tiene roles formales y
// usa RoleAll.
type Role string

const (
	RoleTop     Role = "TOP"
	RoleJungle  Role = "JUNGLE"
	RoleMiddle  Role = "MIDDLE"
	RoleBottom  Role = "BOTTOM"
	RoleSupport Role = "SUPPORT"
	RoleAll     Role = "ALL"
)

// AllRoles es el orden canónico de posiciones de LoL (u.gg 1..5).
var AllRoles = []Role{RoleTop, RoleJungle, RoleMiddle, RoleBottom, RoleSupport}

func (r Role) String() string { return string(r) }

// Árboles de runas (ids de estilo de Data Dragon); LoL only.
const (
	TreePrecision   = 8000
	TreeDomination  = 8100
	TreeSorcery     = 8200
	TreeInspiration = 8300
	TreeResolve     = 8400
)

// TreeName devuelve el nombre en español del árbol de runas.
func TreeName(id int) string {
	switch id {
	case TreePrecision:
		return "Precisión"
	case TreeDomination:
		return "Dominación"
	case TreeSorcery:
		return "Brujería"
	case TreeInspiration:
		return "Inspiración"
	case TreeResolve:
		return "Valor"
	}
	return fmt.Sprintf("Árbol %d", id)
}

// Stats es el par [partidas, victorias] sobre el que se calcula todo.
type Stats struct {
	Games int
	Wins  int
}

// WinRate devuelve wins/games; 0 si no hay partidas.
func (s Stats) WinRate() float64 {
	if s.Games <= 0 {
		return 0
	}
	return float64(s.Wins) / float64(s.Games)
}

// ItemStats es el uso de un ítem (o botas) en el pool del héroe/rol.
type ItemStats struct {
	ID int
	Stats
	// AvgBuyS es el tiempo medio de compra en segundos dentro de la
	// partida (Deadlock); 0 = desconocido.
	AvgBuyS float64
}

// HeroRef referencia a otro héroe con estadísticas enfrentadas o
// compartidas (matchups y sinergias).
type HeroRef struct {
	HeroID int
	Games  int
	Wins   int
}

// WRTrendPoint es el WR agregado de un héroe en una ventana temporal
// (mes) para la franja de tendencia.
type WRTrendPoint struct {
	Time  int64 // unix del inicio de la ventana
	WR    float64
	Games int
}

// WinRate del cruce.
func (h HeroRef) WinRate() float64 {
	if h.Games <= 0 {
		return 0
	}
	return float64(h.Wins) / float64(h.Games)
}

// RunePageStats es una página de runas completa tratada como unidad
// (solo LoL; Deadlock no tiene runas).
type RunePageStats struct {
	PrimaryTree   int
	SecondaryTree int
	KeystoneID    int
	PrimaryIDs    []int // runas del primario sin la keystone (3)
	SecondaryIDs  []int // runas del secundario (2)
	Shards        [3]int
	Stats
}

// SkillSequenceStats es una secuencia completa de habilidades, nivel
// 1..N según las reglas del juego ("QWEQQR…" o "121314…").
type SkillSequenceStats struct {
	Sequence string
	Stats
}

// ChampionBuilds son los datos crudos ya normalizados de un héroe en
// un rol (los nombres conservan "Champion" por carga histórica; para
// Deadlock son héroes).
type ChampionBuilds struct {
	Game         Game
	ChampionID   int
	ChampionKey  string // clave del proveedor estático ("Ahri", "Abrams")
	ChampionName string
	Role         Role
	Patch        string
	Source       string // "ugg", "deadlock-api.com", ...
	Total        Stats  // overview global del héroe/rol
	Items        []ItemStats
	Boots        []ItemStats
	RunePages    []RunePageStats
	Skills       []SkillSequenceStats
	// Rules permite al adaptador aportar reglas concretas por héroe
	// (Deadlock: 15 puntos y topes variables); en cero se usan las del
	// juego (Game.SkillRules).
	Rules SkillRules
	// AbilityNames nombra cada habilidad del alfabeto (Deadlock: ID de
	// habilidad → slot). Opcional, para leyendas en la UI.
	AbilityNames map[rune]string
	// Matchups y Synergies: cruces contra y con otros héroes
	// (Deadlock). Vacío si la fuente no los trae.
	Matchups  []HeroRef
	Synergies []HeroRef
	// WRTrend: WR por ventana temporal (meses, Deadlock).
	WRTrend []WRTrendPoint
	// ItemSlots trae las tablas item1..item5 de Lolalytics: los ítems
	// más usados como enésima compra, con su minuto medio. Tabla i
	// (índice 0 = 1.er ítem) puede venir vacía; nil si la fuente no los
	// trae (u.gg, Deadlock).
	ItemSlots [][]ItemStats
	// ItemCombos trae tríos con WR conjunto (builtBootSet3): un combo
	// real, no tres ítems sueltos. Ya vienen ordenados por presencia.
	ItemCombos []ItemCombo
	// Spells trae los pares de hechizos de invocador más usados con su
	// WR conjunto (tabla spells de Lolalytics, solo LoL).
	Spells []SummonerStats
	// StartItems trae los ítems iniciales (tabla startItem, LoL).
	StartItems []ItemStats
	// Wards trae los ítems de visión de la tabla item (LoL): se
	// rankean aparte porque el Control Ward se compra en casi todas
	// las partidas y ganaría cualquier ranking de ítems.
	Wards []ItemStats
}

// ItemCombo es el WR de un trío de ítems comprados juntos.
type ItemCombo struct {
	IDs []int
	Stats
	// PickRate en % de las partidas del héroe/rol (como lo trae la fila).
	PickRate float64
}

// SummonerStats es el uso conjunto de un par de hechizos de invocador
// (LoL): la tabla spells de Lolalytics trae el par como un id
// concatenado y ordenado ("414" = 4+14, Destello + Encender).
type SummonerStats struct {
	IDs [2]int // [slot1, slot2]; 0 = partida con un solo hechizo
	Stats
}

// SkillRulesFor devuelve las reglas de secuencia del héroe: las que
// aportó el adaptador si son específicas (Deadlock real), si no las
// por defecto del juego.
func (cb *ChampionBuilds) SkillRulesFor() SkillRules {
	if len(cb.Rules.Alphabet) > 0 {
		return cb.Rules
	}
	return cb.Game.SkillRules()
}

// RankedItem es un ítem con métricas derivadas y su ranking Wilson.
type RankedItem struct {
	ItemStats
	WinRate  float64
	PickRate float64
	Wilson   float64
}

// RankedRunePage es una página de runas con métricas derivadas.
type RankedRunePage struct {
	RunePageStats
	WinRate  float64
	PickRate float64
	Wilson   float64
}

// SkillOrder es el resultado del análisis de secuencias: la más jugada
// más el orden de maximización derivado (p.ej. Q,E,W o 1,2,3).
type SkillOrder struct {
	Sequence string
	Priority []rune
	Stats
}

// CleanBuild es la salida del pipeline de limpieza, lista para la UI.
type CleanBuild struct {
	Game         Game
	ChampionID   int
	ChampionKey  string
	ChampionName string
	Role         Role
	Patch        string
	Source       string
	Total        Stats
	WinRate      float64
	CoreItems    []RankedItem
	Boots        []RankedItem
	RunePages    []RankedRunePage
	SkillOrder   SkillOrder
	// SkillAlphabet son todas las habilidades del juego/héroe en orden
	// fijo (para filas de la rejilla; incluye la R de LoL, que no está
	// en Priority).
	SkillAlphabet []rune
	// AbilityNames se propaga del dato crudo para leyendas de la UI.
	AbilityNames map[rune]string
	// Matchups y Synergies se propagan del dato crudo.
	Matchups  []HeroRef
	Synergies []HeroRef
	// WRTrend se propaga del dato crudo.
	WRTrend []WRTrendPoint
	// ItemSlots e ItemCombos se propagan del dato crudo (LoL).
	ItemSlots  [][]ItemStats
	ItemCombos []ItemCombo
	// Spells se propaga del dato crudo (LoL).
	Spells []SummonerStats
	// StartItems se propaga del dato crudo (LoL).
	StartItems []ItemStats
	// Wards rankeados aparte (LoL): el mejor va a su propia sección.
	Wards []RankedItem
}
