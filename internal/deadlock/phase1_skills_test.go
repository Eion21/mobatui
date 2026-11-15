package deadlock

// Pruebas de regresión de Fase 1: orden visual de las habilidades.
// El slot de cada habilidad sale del orden canónico del héroe
// (signature1..4), no de su frecuencia de uso.

import (
	"fmt"
	"testing"
)

// TestSkillSlotsFollowCanonicalOrder: el slot de cada habilidad sale del
// orden canónico del héroe, no de su frecuencia de uso. Reproduce el
// caso Solomon, donde la habilidad más subida es la 3 y ordenar por uso
// invertía los slots (1↔4, 2↔3).
func TestSkillSlotsFollowCanonicalOrder(t *testing.T) {
	// Cuatro habilidades; la 3 es la más subida en las secuencias.
	const (
		a1 int64 = 4011110259 // signature1
		a2 int64 = 4180486641 // signature2
		a3 int64 = 2424652896 // signature3 (la más popular)
		a4 int64 = 1389230689 // signature4
	)
	stats := []apiAbilityStat{
		{Abilities: []int64{a3, a3, a1, a4, a3, a2, a2, a1, a1, a4, a4, a4}, Matches: 100},
		{Abilities: []int64{a3, a3, a1, a4, a3, a2, a2, a1, a1, a4, a4, a3}, Matches: 80},
	}
	names := map[int]string{
		int(a1): "Setup Bishop", int(a2): "Kingside Knight",
		int(a3): "Rook Battery", int(a4): "Develop Queen",
	}
	slots := map[int64]int{a1: 1, a2: 2, a3: 3, a4: 4}

	_, _, legend, err := buildSkillSequences(stats, names, slots, map[int]int64{1: a1, 2: a2, 3: a3, 4: a4})
	if err != nil {
		t.Fatalf("secuencias: %v", err)
	}
	want := map[rune]string{
		'1': "Setup Bishop", '2': "Kingside Knight",
		'3': "Rook Battery", '4': "Develop Queen",
	}
	for slot, name := range want {
		if legend[slot] != name {
			t.Errorf("slot %c = %q, esperado %q (leyenda %+v)", slot, legend[slot], name, legend)
		}
	}
}

// TestSkillSlotsFallBackToUsage: sin orden canónico se conserva el
// comportamiento histórico (uso descendente), sin inventar slots.
func TestSkillSlotsFallBackToUsage(t *testing.T) {
	stats := []apiAbilityStat{
		{Abilities: []int64{7, 7, 9, 8, 7, 9, 8, 8}, Matches: 50},
	}
	names := map[int]string{7: "Popular", 8: "Media", 9: "Rara"}
	_, _, legend, err := buildSkillSequences(stats, names, nil, nil)
	if err != nil {
		t.Fatalf("secuencias: %v", err)
	}
	if legend['1'] != "Popular" {
		t.Errorf("slot 1 = %q, esperado Popular (fallback por uso)", legend['1'])
	}
	if legend['3'] != "Rara" {
		t.Errorf("slot 3 = %q, esperado Rara", legend['3'])
	}
}

// TestSkillSlotsRejectMismatchedCanonical: si el orden canónico no cubre
// exactamente las habilidades observadas, se ignora (no se inventa).
func TestSkillSlotsRejectMismatchedCanonical(t *testing.T) {
	stats := []apiAbilityStat{
		{Abilities: []int64{7, 7, 9, 8, 7, 9, 8, 8}, Matches: 50},
	}
	names := map[int]string{7: "Popular", 8: "Media", 9: "Rara"}
	// Mapa que no cubre la habilidad 9: debe caer al fallback.
	slots := map[int64]int{7: 1, 8: 2, 10: 3, 11: 4}
	_, _, legend, err := buildSkillSequences(stats, names, slots, nil)
	if err != nil {
		t.Fatalf("secuencias: %v", err)
	}
	if legend['1'] != "Popular" {
		t.Errorf("debía caer al orden por uso, leyenda = %+v", legend)
	}
	if _, ok := legend['4']; ok {
		t.Errorf("no debe haber un cuarto slot inventado: %+v", legend)
	}
}

// TestCanonicalAbilityOrderIncomplete: un kit sin las cuatro firmas no
// produce orden canónico.
func TestCanonicalAbilityOrderIncomplete(t *testing.T) {
	h := apiHero{Items: map[string]string{
		"signature1": "ability_a",
		"signature2": "ability_b",
	}}
	byClass := map[string]int{"ability_a": 1, "ability_b": 2}
	if got := canonicalAbilityOrder(h, byClass); got != nil {
		t.Errorf("esperado nil con kit incompleto, hay %v", got)
	}
}

// TestCanonicalAbilityOrderUnknownClass: una firma sin id conocido no
// produce orden canónico.
func TestCanonicalAbilityOrderUnknownClass(t *testing.T) {
	h := apiHero{Items: map[string]string{
		"signature1": "ability_a", "signature2": "ability_b",
		"signature3": "ability_c", "signature4": "ability_d",
	}}
	byClass := map[string]int{"ability_a": 1, "ability_b": 2, "ability_c": 3}
	if got := canonicalAbilityOrder(h, byClass); got != nil {
		t.Errorf("esperado nil con class desconocida, hay %v", got)
	}
}

// TestCanonicalAbilityOrderHappyPath: el orden sale de signature1..4.
func TestCanonicalAbilityOrderHappyPath(t *testing.T) {
	h := apiHero{Items: map[string]string{
		"signature1": "a", "signature2": "b", "signature3": "c", "signature4": "d",
	}}
	byClass := map[string]int{"a": 10, "b": 20, "c": 30, "d": 40}
	got := canonicalAbilityOrder(h, byClass)
	want := []int64{10, 20, 30, 40}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("orden = %v, esperado %v", got, want)
	}
}

// TestSkillSlotsKeepMaxPoints: los topes por slot siguen siendo el
// máximo observado, independientemente del orden asignado.
func TestSkillSlotsKeepMaxPoints(t *testing.T) {
	const (
		a1 int64 = 11
		a2 int64 = 22
		a3 int64 = 33
		a4 int64 = 44
	)
	stats := []apiAbilityStat{
		{Abilities: []int64{a1, a1, a1, a1, a2, a2, a2, a3, a3, a4}, Matches: 10},
	}
	_, rules, _, err := buildSkillSequences(stats, nil, map[int64]int{a1: 1, a2: 2, a3: 3, a4: 4}, map[int]int64{1: a1, 2: a2, 3: a3, 4: a4})
	if err != nil {
		t.Fatalf("secuencias: %v", err)
	}
	if rules.MaxPoints['1'] != 4 {
		t.Errorf("tope slot 1 = %d, esperado 4", rules.MaxPoints['1'])
	}
	if rules.MaxPoints['4'] != 1 {
		t.Errorf("tope slot 4 = %d, esperado 1", rules.MaxPoints['4'])
	}
	if rules.Length != 10 {
		t.Errorf("longitud = %d, esperado 10", rules.Length)
	}
}
