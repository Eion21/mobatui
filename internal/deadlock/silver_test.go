package deadlock

// Silver: héroe con cambio de forma. Siete registros estadísticos deben
// colapsar en cuatro espacios canónicos. La relación sale de los
// metadatos del juego (dependent_abilities + DisplayAsSubAbility), no
// de similitud de nombres.

import "testing"

// silverWant es el fixture de referencia: tecla → forma humana.
// La forma transformada comparte espacio con su principal.
var silverWant = [4]struct{ primary, transformed string }{
	{"Slam Fire", "Go For The Throat"},
	{"Boot Kick", "Mauling Leap"},
	{"Entangling Bola", "Tail Whack"},
	{"Lycan Curse", ""}, // controla la transformación
}

func silverHero(t *testing.T, api *APIClient) apiHero {
	t.Helper()
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	for _, h := range heroes {
		if h.Name == "Silver" {
			return h
		}
	}
	t.Skip("Silver no está en el catálogo")
	return apiHero{}
}

// TestSilverFourCanonicalSlots: cuatro espacios, no siete.
func TestSilverFourCanonicalSlots(t *testing.T) {
	api := diagClient(t)
	h := silverHero(t, api)
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	shop, names, _ := api.shopItems()

	cb, err := api.loadHeroBuilds(apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
		h.Name, shop, names, "t", h, byClass, itemsByClass)
	if err != nil || cb == nil {
		t.Skipf("sin muestra: %v", err)
	}
	if !cb.Rules.CanonicalOrder {
		t.Error("Silver debe tener orden canónico, no fallback")
	}
	if len(cb.Rules.Alphabet) != 4 {
		t.Fatalf("alfabeto = %q, esperado 4 espacios", string(cb.Rules.Alphabet))
	}
	for i, want := range silverWant {
		slot := rune('1' + i)
		if got := cb.AbilityNames[slot]; got != want.primary {
			t.Errorf("slot %c = %q, esperado %q (forma principal)", slot, got, want.primary)
		}
	}
}

// TestSilverSevenRecordsCollapse: los siete ids observados se agrupan en
// cuatro espacios, sin perder ninguno.
func TestSilverSevenRecordsCollapse(t *testing.T) {
	api := diagClient(t)
	h := silverHero(t, api)
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()

	slots, primaries := abilitySlotMap(h, byClass, itemsByClass)
	if len(slots) != 7 {
		t.Errorf("slotMap tiene %d ids, esperado 7 (4 firmas + 3 sub-habilidades)", len(slots))
	}
	if len(primaries) != 4 {
		t.Errorf("primaries = %d, esperado 4", len(primaries))
	}
	// Cada espacio debe tener al menos su principal.
	for slot := 1; slot <= 4; slot++ {
		found := false
		for _, s := range slots {
			if s == slot {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("espacio %d sin ninguna habilidad mapeada", slot)
		}
	}
	// Los tres espacios con transformación deben tener dos ids.
	counts := map[int]int{}
	for _, s := range slots {
		counts[s]++
	}
	for slot := 1; slot <= 3; slot++ {
		if counts[slot] != 2 {
			t.Errorf("espacio %d tiene %d ids, esperado 2 (principal + transformada)", slot, counts[slot])
		}
	}
	if counts[4] != 1 {
		t.Errorf("espacio 4 tiene %d ids, esperado 1 (solo la transformación)", counts[4])
	}
}

// TestSilverTransformationMapping: cada forma transformada cae en el
// espacio de su principal, según los metadatos.
func TestSilverTransformationMapping(t *testing.T) {
	api := diagClient(t)
	h := silverHero(t, api)
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	slots, _ := abilitySlotMap(h, byClass, itemsByClass)

	for i, want := range silverWant {
		slot := i + 1
		primaryID, ok := byClass[h.Items[signatureKey(slot)]]
		if !ok {
			t.Fatalf("signature%d no resuelve", slot)
		}
		if got := slots[int64(primaryID)]; got != slot {
			t.Errorf("%s mapeada a espacio %d, esperado %d", want.primary, got, slot)
		}
		if want.transformed == "" {
			continue
		}
		// La transformada debe estar en el MISMO espacio.
		var subID int
		for class, it := range itemsByClass {
			if it.Name != want.transformed {
				continue
			}
			if id, ok := byClass[class]; ok {
				subID = id
			}
		}
		if subID == 0 {
			t.Errorf("%s no encontrada en el catálogo", want.transformed)
			continue
		}
		if got := slots[int64(subID)]; got != slot {
			t.Errorf("%s mapeada a espacio %d, esperado %d (mismo que %s)",
				want.transformed, got, slot, want.primary)
		}
	}
}

// TestSilverNoDoubleCounting: los topes por espacio son el máximo
// observado, no la suma de principal + transformada.
func TestSilverNoDoubleCounting(t *testing.T) {
	api := diagClient(t)
	h := silverHero(t, api)
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	shop, names, _ := api.shopItems()

	cb, err := api.loadHeroBuilds(apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
		h.Name, shop, names, "t", h, byClass, itemsByClass)
	if err != nil || cb == nil {
		t.Skipf("sin muestra: %v", err)
	}
	// En Deadlock cada habilidad admite 4 puntos como máximo: dos formas
	// del mismo espacio no pueden sumar 8.
	for slot, cap := range cb.Rules.MaxPoints {
		if cap > 4 {
			t.Errorf("espacio %c con tope %d: parece doble conteo", slot, cap)
		}
	}
}

// TestSilverPriorityUsesRealSlots: la prioridad max usa 1..4.
func TestSilverPriorityUsesRealSlots(t *testing.T) {
	api := diagClient(t)
	h := silverHero(t, api)
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	shop, names, _ := api.shopItems()

	cb, err := api.loadHeroBuilds(apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
		h.Name, shop, names, "t", h, byClass, itemsByClass)
	if err != nil || cb == nil {
		t.Skipf("sin muestra: %v", err)
	}
	if len(cb.Skills) == 0 {
		t.Fatal("sin secuencias")
	}
	for _, r := range cb.Skills[0].Sequence {
		if r < '1' || r > '4' {
			t.Errorf("secuencia contiene %q fuera de 1..4", string(r))
		}
	}
}

// signatureKey es "signatureN" sin fmt para no arrastrar imports.
func signatureKey(n int) string {
	return "signature" + string(rune('0'+n))
}
