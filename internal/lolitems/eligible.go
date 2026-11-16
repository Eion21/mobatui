package lolitems

// Elegibilidad de candidatos: qué objetos pueden siquiera considerarse
// como alternativa final. Es un filtro estructural, NO una
// recomendación: que un objeto pase el filtro no significa que se use.
// La evidencia de uso la aporta Lolalytics.

// MapSummonersRift es el id de la Grieta del Invocador en Data Dragon.
const MapSummonersRift = 11

// Rejection explica por qué un objeto no es candidato.
type Rejection string

const (
	RejectNone           Rejection = ""
	RejectWrongMap       Rejection = "no disponible en la Grieta del Invocador"
	RejectNotPurchasable Rejection = "no comprable"
	RejectNotInStore     Rejection = "fuera de la tienda"
	RejectConsumable     Rejection = "consumible"
	RejectTrinket        Rejection = "objeto de visión (trinket)"
	RejectComponent      Rejection = "componente, no objeto completo"
	RejectSpecial        Rejection = "objeto especial o de modo alternativo"
	RejectBootsUpgrade   Rejection = "mejora de botas (ranura de misión aparte)"
	RejectNoTags         Rejection = "sin metadatos utilizables"
)

// Reason devuelve el motivo por el que el objeto no es elegible, o
// RejectNone si sí lo es.
//
// El criterio de "objeto completo" sale de la cadena de mejoras, no de
// `depth`: en el parche actual hay objetos finales con depth=2 (Filo
// infinito) y componentes con depth=2, así que depth no distingue.
//
// Las botas son un caso aparte porque su cadena tiene dos niveles:
//
//	básicas (1001) → definitivas (3006) → mejora de misión (3172)
//
// Las básicas son componente; las definitivas son el candidato real de
// la ranura de botas; la mejora de misión depende de la temporada y se
// trata como objeto especial, no como alternativa normal.
func Reason(it Item) Rejection {
	if len(it.Maps) > 0 && !it.Maps[MapSummonersRift] {
		return RejectWrongMap
	}
	if it.Consumable {
		return RejectConsumable
	}
	if it.Trinket {
		return RejectTrinket
	}
	if it.SpecialRecipe > 0 || it.RequiredAlly != "" {
		return RejectSpecial
	}
	if !it.InStore {
		return RejectNotInStore
	}
	if it.Gold.Total > 0 && !it.Gold.Purchasable {
		return RejectNotPurchasable
	}
	if len(it.Tags) == 0 {
		return RejectNoTags
	}

	if IsBoots(it) {
		switch {
		case IsBootsUpgrade(it):
			return RejectBootsUpgrade
		case len(it.From) == 0:
			// Botas básicas: se mejoran, no son objeto final.
			return RejectComponent
		default:
			// Botas definitivas: candidato válido de su ranura.
			return RejectNone
		}
	}

	// Resto: si tiene una mejora directa, es un componente.
	if len(it.Into) > 0 {
		return RejectComponent
	}
	return RejectNone
}

// Eligible indica si el objeto puede ser candidato a alternativa final.
func Eligible(it Item) bool { return Reason(it) == RejectNone }

// IsBoots indica si el objeto es un par de botas (tag de la fuente).
func IsBoots(it Item) bool { return containsString(it.Tags, "Boots") }

// IsBootsUpgrade detecta las botas mejoradas por la misión de rol: son
// las que se construyen a partir de unas botas definitivas pero ya no
// se mejoran a nada. Se apoya en la cadena de mejoras, no en el nombre
// ni en el coste.
func IsBootsUpgrade(it Item) bool {
	return IsBoots(it) && len(it.From) > 0 && len(it.Into) == 0 && !isBasicBoots(it)
}

// isBasicBoots distingue las botas de 300g, que son el primer eslabón.
// Se reconoce por no tener componentes (nada más barato las forma).
func isBasicBoots(it Item) bool { return IsBoots(it) && len(it.From) == 0 }

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// QuestItems son los objetos de misión de soporte (el "support item"
// de la temporada): se eligen al empezar y se mejoran con oro de
// misión. No son compras normales de build y ocupan su propia ranura.
//
// La lista es explícita: Data Dragon los distingue por el tag "Lane"
// combinado con un coste de 400 y la cadena 3865→3867→final, pero esa
// estructura puede cambiar entre temporadas, así que se enumeran.
var questItems = map[int]bool{
	3865: true, // Atlas mundial (inicial)
	3866: true, // Brújula rúnica (inicial)
	3867: true, // Botín universal (inicial)
	3869: true, // Detracción celestial (final)
	3870: true, // Tejesueños (final)
	3871: true, // Perforaplanos de Zaz'Zak (final)
	3872: true, // Mirada del ocaso (final)
	3873: true, // Báculo del vacío (final)
	3874: true, // Filo del solsticio (final)
	3876: true, // Trineo del solsticio (final)
}

// IsQuestItem indica si el objeto es de la misión de soporte.
func IsQuestItem(id int) bool { return questItems[id] }
