// Package lolitems modela el catálogo de objetos de League of Legends a
// partir de Data Dragon y los clasifica por función.
//
// Principio: Data Dragon DESCRIBE los objetos; no dice qué objetos se
// usan. La evidencia de uso la aportan las estadísticas de Lolalytics,
// así que este paquete nunca recomienda nada por el mero hecho de
// pertenecer a una categoría.
package lolitems

// Item es un objeto de la tienda con los metadatos que publica Data
// Dragon. Los campos opcionales quedan en cero si el parche no los trae.
type Item struct {
	ID    int    // id numérico (clave de Data Dragon)
	Name  string // nombre localizado
	Plain string // descripción corta sin HTML

	// Tags son las etiquetas declaradas por Riot ("Damage", "Boots"…).
	// Es dato de la fuente, no una interpretación nuestra.
	Tags []string

	// Gold recoge el coste y si el objeto se puede comprar.
	Gold Gold

	// Into / From describen la cadena de mejoras: "Into" son los objetos
	// en los que este se convierte; "From" los componentes que lo forman.
	Into []int
	From []int

	// Maps es el mapa → disponible (11 = Grieta del Invocador).
	Maps map[int]bool
	// InStore indica si aparece en la tienda.
	InStore bool
	// Depth es la profundidad en el árbol de mejoras (1 = básico).
	Depth int
	// Consumable, Trinket, SpecialRecipe y RequiredAlly permiten
	// descartar objetos que no son equipamiento final.
	Consumable    bool
	Trinket       bool
	SpecialRecipe int
	RequiredAlly  string

	// Stats son los números que publica Data Dragon.
	Stats Stats
}

// Gold es el bloque económico del objeto.
type Gold struct {
	Base        int
	Total       int
	Sell        int
	Purchasable bool
}

// Stats son las estadísticas planas de Data Dragon. Solo se usan las
// que ayudan a clasificar; el resto se ignora a propósito.
type Stats struct {
	FlatArmor          float64
	FlatSpellBlock     float64
	FlatHealth         float64
	FlatMagicDamage    float64
	FlatPhysicalDamage float64
	FlatAttackSpeed    float64
	FlatMoveSpeed      float64
	FlatMP             float64
	FlatCritChance     float64
	PercentAttackSpeed float64
	PercentMoveSpeed   float64
	FlatMPRegen        float64
	FlatHPRegen        float64
	FlatMagicPen       float64
	FlatArmorPen       float64
	PercentArmorPen    float64
	PercentMagicPen    float64
	FlatLifeSteal      float64
	FlatSpellVamp      float64
	FlatCooldownRed    float64
	FlatAbilityPower   float64
}

// Category es una función del objeto dentro de una build.
type Category string

const (
	CatPhysicalDamage Category = "daño físico"
	CatMagicDamage    Category = "daño mágico"
	CatArmorPen       Category = "penetración física"
	CatMagicPen       Category = "penetración mágica"
	CatAttackSpeed    Category = "velocidad de ataque"
	CatBoots          Category = "botas"
	CatMovement       Category = "movimiento"
	CatArmor          Category = "armadura"
	CatHealth         Category = "vida"
	CatMagicResist    Category = "resistencia mágica"
	CatAntiHeal       Category = "anticuración"
	CatUtility        Category = "utilidad"
	CatSupport        Category = "apoyo"
)

// AllCategories es el orden canónico de presentación: estable y
// determinista para que la UI no cambie de orden entre ejecuciones.
var AllCategories = []Category{
	CatPhysicalDamage, CatMagicDamage,
	CatArmorPen, CatMagicPen,
	CatAttackSpeed, CatBoots, CatMovement,
	CatArmor, CatHealth, CatMagicResist,
	CatAntiHeal, CatUtility, CatSupport,
}

// Classified es un objeto con sus categorías y de dónde salió cada una.
type Classified struct {
	Item Item
	// Declared: categorías que se deducen directamente de los tags de
	// Data Dragon (dato de la fuente).
	Declared []Category
	// Derived: categorías que aportan reglas nuestras (p. ej. stats o
	// la lista explícita de anticuración).
	Derived []Category
}

// Categories devuelve la unión ordenada de declaradas y derivadas.
func (c Classified) Categories() []Category {
	seen := make(map[Category]bool, len(c.Declared)+len(c.Derived))
	out := make([]Category, 0, len(c.Declared)+len(c.Derived))
	for _, cat := range AllCategories {
		if containsCat(c.Declared, cat) || containsCat(c.Derived, cat) {
			if !seen[cat] {
				seen[cat] = true
				out = append(out, cat)
			}
		}
	}
	return out
}

// Has indica si el objeto pertenece a la categoría.
func (c Classified) Has(cat Category) bool {
	return containsCat(c.Declared, cat) || containsCat(c.Derived, cat)
}

func containsCat(list []Category, cat Category) bool {
	for _, c := range list {
		if c == cat {
			return true
		}
	}
	return false
}

// tagCategorias mapea los tags de Data Dragon a categorías. Solo se
// declara lo que la fuente dice explícitamente.
var tagCategorias = map[string]Category{
	"Damage":            CatPhysicalDamage,
	"SpellDamage":       CatMagicDamage,
	"ArmorPenetration":  CatArmorPen,
	"MagicPenetration":  CatMagicPen,
	"AttackSpeed":       CatAttackSpeed,
	"Boots":             CatBoots,
	"NonbootsMovement":  CatMovement,
	"Armor":             CatArmor,
	"Health":            CatHealth,
	"SpellBlock":        CatMagicResist,
	"AbilityHaste":      CatUtility,
	"CooldownReduction": CatUtility,
	"Aura":              CatUtility,
	"Active":            CatUtility,
	"Vision":            CatUtility,
	"Slow":              CatUtility,
	"GoldPer":           CatSupport,
	"ManaRegen":         CatSupport,
	"HealthRegen":       CatSupport,
	"Mana":              CatSupport,
}

// Classify clasifica un objeto combinando tags declarados y reglas
// propias. Es determinista: el resultado no depende del orden de Tags.
func Classify(it Item) Classified {
	declared := make([]Category, 0, 4)
	seen := map[Category]bool{}
	for _, tag := range it.Tags {
		cat, ok := tagCategorias[tag]
		if !ok || seen[cat] {
			continue
		}
		seen[cat] = true
		declared = append(declared, cat)
	}

	derived := make([]Category, 0, 3)
	addDerived := func(cat Category) {
		if !seen[cat] {
			seen[cat] = true
			derived = append(derived, cat)
		}
	}

	// Reglas propias: solo cuando Data Dragon no lo declara como tag.
	if it.Stats.FlatArmorPen > 0 || it.Stats.PercentArmorPen > 0 {
		addDerived(CatArmorPen)
	}
	if it.Stats.FlatMagicPen > 0 || it.Stats.PercentMagicPen > 0 {
		addDerived(CatMagicPen)
	}
	if it.Stats.FlatAttackSpeed > 0 || it.Stats.PercentAttackSpeed > 0 {
		addDerived(CatAttackSpeed)
	}
	if it.Stats.FlatMoveSpeed > 0 || it.Stats.PercentMoveSpeed > 0 {
		addDerived(CatMovement)
	}
	if it.Stats.FlatArmor > 0 {
		addDerived(CatArmor)
	}
	if it.Stats.FlatHealth > 0 {
		addDerived(CatHealth)
	}
	if it.Stats.FlatSpellBlock > 0 {
		addDerived(CatMagicResist)
	}
	if it.Stats.FlatMagicDamage > 0 || it.Stats.FlatAbilityPower > 0 {
		addDerived(CatMagicDamage)
	}
	if it.Stats.FlatPhysicalDamage > 0 {
		addDerived(CatPhysicalDamage)
	}
	if it.Stats.FlatLifeSteal > 0 || it.Stats.FlatSpellVamp > 0 {
		addDerived(CatUtility)
	}
	// Anticuración: no existe tag ni stat. Solo la lista explícita.
	if IsAntiHeal(it.ID) {
		addDerived(CatAntiHeal)
	}

	// Orden determinista en ambas listas.
	sortCategories(declared)
	sortCategories(derived)
	return Classified{Item: it, Declared: declared, Derived: derived}
}

// sortCategories ordena según AllCategories.
func sortCategories(list []Category) {
	rank := make(map[Category]int, len(AllCategories))
	for i, c := range AllCategories {
		rank[c] = i
	}
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && rank[list[j]] < rank[list[j-1]]; j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

// antiHealIDs: objetos que aplican Heridas Graves.
//
// Data Dragon NO expone este efecto: ni un tag ni una stat lo declaran,
// así que hay que listarlo. La lista NO se infiere del nombre: se
// verificó buscando "heridas graves" en las descripciones publicadas de
// item.json (parche 16.21.1) y quedándose solo con los objetos que la
// mencionan en su texto.
//
// Se incluyen tanto los completos como sus componentes, porque ambos
// aplican el efecto. Las variantes con prefijo (223xxx, 773xxx) son
// duplicados internos de Riot para modos alternativos: se listan por id
// exacto para no arrastrarlas a Grieta del Invocador.
var antiHealIDs = map[int]bool{
	3123: true, // Llamada del verdugo (800, componente)
	3916: true, // Orbe del olvido (800, componente)
	3011: true, // Putrificador tecnoquímico (1900)
	3165: true, // Morellonomicón (2850)
	3033: true, // Recordatorio letal (3000)
}

// IsAntiHeal indica si el objeto aplica Heridas Graves según la lista
// verificada contra el texto de Data Dragon.
func IsAntiHeal(id int) bool { return antiHealIDs[id] }
