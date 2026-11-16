package lolitems

// Pruebas del catálogo y la clasificación. Usan testdata/ddragon/
// items-catalog.json (fixture versionado): sin red y sin caché personal.

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
)

// fixtureItem carga el catálogo de prueba.
func fixtureItems(t *testing.T) map[int]Item {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/ddragon/items-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Data map[string]struct {
			Name          string   `json:"name"`
			Plaintext     string   `json:"plaintext"`
			Tags          []string `json:"tags"`
			Into          []string `json:"into"`
			From          []string `json:"from"`
			InStore       *bool    `json:"inStore"`
			Depth         int      `json:"depth"`
			Consumable    bool     `json:"consumable"`
			SpecialRecipe int      `json:"specialRecipe"`
			RequiredAlly  string   `json:"requiredAlly"`
			Gold          struct {
				Total       int  `json:"total"`
				Purchasable bool `json:"purchasable"`
			} `json:"gold"`
			Maps map[string]bool `json:"maps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	out := make(map[int]Item, len(f.Data))
	for key, v := range f.Data {
		var id int
		if err := json.Unmarshal([]byte(key), &id); err != nil {
			continue
		}
		it := Item{
			ID: id, Name: v.Name, Plain: v.Plaintext, Tags: v.Tags,
			Into: toInts(v.Into), From: toInts(v.From),
			Depth: v.Depth, Consumable: v.Consumable,
			SpecialRecipe: v.SpecialRecipe, RequiredAlly: v.RequiredAlly,
			Trinket: containsString(v.Tags, "Trinket"),
			Gold:    Gold{Total: v.Gold.Total, Purchasable: v.Gold.Purchasable},
			Maps:    map[int]bool{},
		}
		it.InStore = true
		if v.InStore != nil {
			it.InStore = *v.InStore
		}
		for k, ok := range v.Maps {
			var n int
			if json.Unmarshal([]byte(k), &n) == nil {
				it.Maps[n] = ok
			}
		}
		out[id] = it
	}
	return out
}

func toInts(list []string) []int {
	out := make([]int, 0, len(list))
	for _, s := range list {
		var n int
		if json.Unmarshal([]byte(s), &n) == nil {
			out = append(out, n)
		}
	}
	return out
}

// TestFixtureCargaMetadatos comprueba que la decodificación conserva lo
// que publica Data Dragon.
func TestFixtureCargaMetadatos(t *testing.T) {
	items := fixtureItems(t)
	if len(items) < 150 {
		t.Fatalf("catálogo de prueba con %d objetos: demasiado pequeño", len(items))
	}
	tests := []struct {
		id       int
		name     string
		wantTags []string
		wantGold int
		wantInto bool
		wantFrom bool
	}{
		{3006, "Grebas de berserker", []string{"AttackSpeed", "Boots"}, 1100, true, true},
		{3031, "Filo infinito", []string{"CriticalStrike", "Damage"}, 3500, false, true},
		{3165, "Morellonomicón", nil, 2850, false, true},
	}
	for _, tt := range tests {
		it, ok := items[tt.id]
		if !ok {
			t.Errorf("falta el objeto %d", tt.id)
			continue
		}
		if it.Name != tt.name {
			t.Errorf("%d nombre = %q, se esperaba %q", tt.id, it.Name, tt.name)
		}
		if it.Gold.Total != tt.wantGold {
			t.Errorf("%d gold = %d, se esperaba %d", tt.id, it.Gold.Total, tt.wantGold)
		}
		for _, tag := range tt.wantTags {
			if !containsString(it.Tags, tag) {
				t.Errorf("%d sin tag %q (tiene %v)", tt.id, tag, it.Tags)
			}
		}
		if tt.wantInto != (len(it.Into) > 0) {
			t.Errorf("%d Into = %v", tt.id, it.Into)
		}
		if tt.wantFrom != (len(it.From) > 0) {
			t.Errorf("%d From = %v", tt.id, it.From)
		}
		if len(it.Maps) > 0 && !it.Maps[MapSummonersRift] {
			t.Errorf("%d no está en la Grieta", tt.id)
		}
	}
}

// TestCategoriasMultiples: un objeto puede pertenecer a varias.
func TestCategoriasMultiples(t *testing.T) {
	items := fixtureItems(t)

	// Filo infinito: daño físico declarado.
	inf := Classify(items[3031])
	if !inf.Has(CatPhysicalDamage) {
		t.Errorf("Filo infinito sin daño físico: %v", inf.Categories())
	}
	if inf.Has(CatMagicDamage) {
		t.Errorf("Filo infinito no debe ser daño mágico: %v", inf.Categories())
	}

	// Cuchilla negra: vida, daño, penetración, movimiento y utilidad.
	bc := Classify(items[3071])
	for _, want := range []Category{CatHealth, CatPhysicalDamage, CatArmorPen, CatMovement, CatUtility} {
		if !bc.Has(want) {
			t.Errorf("Cuchilla negra sin %q: %v", want, bc.Categories())
		}
	}
	if len(bc.Categories()) < 5 {
		t.Errorf("se esperaban varias categorías: %v", bc.Categories())
	}

	// Morellonomicón: vida + daño mágico + utilidad, y anticuración
	// derivada (no declarada por tag).
	mo := Classify(items[3165])
	for _, want := range []Category{CatHealth, CatMagicDamage, CatUtility, CatAntiHeal} {
		if !mo.Has(want) {
			t.Errorf("Morellonomicón sin %q: %v", want, mo.Categories())
		}
	}
	// La anticuración es derivada, no declarada.
	if containsCat(mo.Declared, CatAntiHeal) {
		t.Error("la anticuración no puede venir de un tag: Data Dragon no la declara")
	}
	if !containsCat(mo.Derived, CatAntiHeal) {
		t.Error("la anticuración debe figurar como derivada")
	}
}

// TestVelocidadAtaqueVsMovimiento: no se confunden.
func TestVelocidadAtaqueVsMovimiento(t *testing.T) {
	items := fixtureItems(t)

	// Grebas de berserker: velocidad de ataque y botas, no movimiento
	// "NonbootsMovement".
	bers := Classify(items[3006])
	if !bers.Has(CatAttackSpeed) {
		t.Errorf("Grebas de berserker sin velocidad de ataque: %v", bers.Categories())
	}
	if !bers.Has(CatBoots) {
		t.Errorf("Grebas de berserker sin botas: %v", bers.Categories())
	}
	if bers.Has(CatMovement) {
		t.Errorf("las botas no deben clasificarse como movimiento adicional: %v", bers.Categories())
	}

	// Un objeto de movimiento puro no aporta velocidad de ataque.
	for _, it := range items {
		c := Classify(it)
		if c.Has(CatMovement) && !containsString(it.Tags, "AttackSpeed") &&
			it.Stats.FlatAttackSpeed == 0 && it.Stats.PercentAttackSpeed == 0 {
			if c.Has(CatAttackSpeed) {
				t.Errorf("%s: movimiento implicó velocidad de ataque: %v", it.Name, c.Categories())
			}
		}
	}

	// Botas de hechicero: botas + penetración mágica, sin ataque.
	pen := Classify(items[3020])
	if !pen.Has(CatBoots) || !pen.Has(CatMagicPen) {
		t.Errorf("Botas de hechicero: %v", pen.Categories())
	}
	if pen.Has(CatAttackSpeed) {
		t.Errorf("Botas de hechicero no da velocidad de ataque: %v", pen.Categories())
	}
}

// TestAnticurancionVerificada: solo los ids del texto de Data Dragon.
func TestAnticurancionVerificada(t *testing.T) {
	verified := []int{3123, 3916, 3011, 3165, 3033}
	for _, id := range verified {
		if !IsAntiHeal(id) {
			t.Errorf("%d debía estar en la lista verificada", id)
		}
	}
	// Objetos que suenan parecido pero NO aplican Heridas Graves.
	for _, id := range []int{3031, 3089, 3036, 3071, 3153, 6672} {
		if IsAntiHeal(id) {
			t.Errorf("%d no debía clasificarse como anticuración", id)
		}
	}
	// Nunca por nombre: un id desconocido no es anticuración.
	if IsAntiHeal(999999) {
		t.Error("un id desconocido no puede ser anticuración")
	}
}

// TestFiltraNoElegibles: consumibles, trinkets, componentes y mejoras.
func TestFiltraNoElegibles(t *testing.T) {
	items := fixtureItems(t)
	cases := []struct {
		id   int
		want Rejection
	}{
		{1001, RejectComponent},    // botas básicas
		{3123, RejectComponent},    // componente de anticuración
		{3916, RejectComponent},    // componente de anticuración
		{3340, RejectTrinket},      // guardián invisible
		{3363, RejectTrinket},      // visión lejana
		{3168, RejectBootsUpgrade}, // mejora de botas de misión
	}
	for _, tt := range cases {
		it, ok := items[tt.id]
		if !ok {
			t.Skipf("el fixture no trae %d", tt.id)
		}
		if got := Reason(it); got != tt.want {
			t.Errorf("%s (%d): motivo = %q, se esperaba %q", it.Name, tt.id, got, tt.want)
		}
	}

	// Y los objetos finales sí pasan.
	for _, id := range []int{3006, 3047, 3020, 3153, 3031, 3165, 3089, 3036, 3071} {
		if it, ok := items[id]; ok {
			if !Eligible(it) {
				t.Errorf("%s (%d) debía ser elegible: %s", it.Name, id, Reason(it))
			}
		}
	}
}

// TestObjetosSinTags: un objeto sin metadatos no se clasifica ni se
// propone como candidato.
func TestObjetosSinTags(t *testing.T) {
	vacio := Item{ID: 1, Name: "Sin datos", Gold: Gold{Total: 500, Purchasable: true}, InStore: true}
	if got := Reason(vacio); got != RejectNoTags {
		t.Errorf("motivo = %q, se esperaba %q", got, RejectNoTags)
	}
	c := Classify(vacio)
	if len(c.Categories()) != 0 {
		t.Errorf("sin tags no debe haber categorías: %v", c.Categories())
	}
	// Un objeto sin mapa declarado no se descarta por el mapa.
	if got := Reason(Item{Tags: []string{"Damage"}, Gold: Gold{Total: 1000, Purchasable: true}, InStore: true}); got != RejectNone {
		t.Errorf("maps ausente no debe excluir: %q", got)
	}
}

// TestOrdenDeterminista: el orden no depende del mapa ni del orden de
// entrada.
func TestOrdenDeterminista(t *testing.T) {
	items := fixtureItems(t)
	ids := make([]int, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	first := make(map[int][]Category, len(ids))
	for _, id := range ids {
		first[id] = Classify(items[id]).Categories()
	}
	// Recorrer al revés debe dar exactamente lo mismo.
	for i := len(ids) - 1; i >= 0; i-- {
		id := ids[i]
		got := Classify(items[id]).Categories()
		if len(got) != len(first[id]) {
			t.Fatalf("%d: %v vs %v", id, got, first[id])
		}
		for j := range got {
			if got[j] != first[id][j] {
				t.Fatalf("%d: orden inestable %v vs %v", id, got, first[id])
			}
		}
	}
	// El orden es siempre el canónico de AllCategories.
	rank := map[Category]int{}
	for i, c := range AllCategories {
		rank[c] = i
	}
	for _, id := range ids {
		cats := first[id]
		for i := 1; i < len(cats); i++ {
			if rank[cats[i-1]] > rank[cats[i]] {
				t.Fatalf("%d: orden no canónico %v", id, cats)
			}
		}
	}
}

// TestClasificacionPorCampeon valida con los objetos que realmente usan
// los cuatro campeones del fixture de la fase 4.
func TestClasificacionPorCampeon(t *testing.T) {
	items := fixtureItems(t)

	// Cada campeón con su objeto más característico y su categoría.
	type caso struct {
		champ string
		id    int
		want  Category
	}
	casos := []caso{
		{"Kai'Sa", 3031, CatPhysicalDamage}, // Filo infinito
		{"Kai'Sa", 3153, CatAttackSpeed},    // Hoja del rey arruinado
		{"Ahri", 3089, CatMagicDamage},      // Rabadon
		{"Ahri", 3165, CatAntiHeal},         // Morellonomicón
		{"Malphite", 3071, CatArmorPen},     // Cuchilla negra
		{"Malphite", 3047, CatArmor},        // Botas blindadas
		{"Soraka", 3111, CatMagicResist},    // Botas de mercurio
		{"Soraka", 3165, CatAntiHeal},       // Morellonomicón
	}
	for _, c := range casos {
		it, ok := items[c.id]
		if !ok {
			t.Errorf("%s: falta el objeto %d", c.champ, c.id)
			continue
		}
		cl := Classify(it)
		if !cl.Has(c.want) {
			t.Errorf("%s / %s: sin %q → %v", c.champ, it.Name, c.want, cl.Categories())
		}
		if !Eligible(it) {
			t.Errorf("%s / %s debía ser candidato: %s", c.champ, it.Name, Reason(it))
		}
	}
}

// TestCompatibleNombresEImagenes: el catálogo enriquecido no altera los
// nombres ni la resolución de iconos del proveedor existente.
func TestCompatibleNombresEImagenes(t *testing.T) {
	// Los nombres del catálogo deben ser los mismos que publica
	// item.json para los ids compartidos con el fixture ligero.
	ligero := map[int]string{
		3020: "Botas de hechicero",
		3089: "Sombrero mortal de Rabadon",
	}
	items := fixtureItems(t)
	for id, name := range ligero {
		if got := items[id].Name; got != name {
			t.Errorf("%d = %q, se esperaba %q", id, got, name)
		}
	}
	// Los iconos se resuelven por id (NNNN.png): el catálogo no cambia
	// ese contrato.
	for id := range items {
		if id <= 0 {
			t.Errorf("id inválido en el catálogo: %d", id)
		}
	}
}
