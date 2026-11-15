package deadlock

// Fixture INDEPENDIENTE del algoritmo: las posiciones esperadas están
// escritas a mano a partir de las referencias del juego (Deadlock
// Database, build 6860). No se derivan del código bajo prueba.

import (
	"fmt"
	"testing"
)

// wantSlots es la verdad de referencia: héroe → [slot1, slot2, slot3, slot4].
var wantSlots = map[string][4]string{
	"Solomon":       {"Setup Bishop", "Kingside Knight", "Rook Battery", "Develop Queen"},
	"Paradox":       {"Pulse Grenade", "Time Wall", "Kinetic Carbine", "Paradoxical Swap"},
	"Venator":       {"Consecrating Grenade", "Gutshot", "Hex-Lined Snap Trap", "Ira Domini"},
	"Violet":        {"Living Ink", "Flow State", "Still Life", "Abstract Landscape"},
	"Deadman Danny": {"Strix Salvo", "Chimera Mk1", "Jettison Fuel", "Close The Deal"},
}

// TestSkillSlotsKnownHeroes comprueba el modelo contra el fixture.
func TestSkillSlotsKnownHeroes(t *testing.T) {
	api := diagClient(t)
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	shop, names, err := api.shopItems()
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}

	for heroName, want := range wantSlots {
		t.Run(heroName, func(t *testing.T) {
			var h apiHero
			for _, x := range heroes {
				if x.Name == heroName {
					h = x
				}
			}
			if h.ID == 0 {
				t.Skipf("%s no está en el catálogo", heroName)
			}
			cb, err := api.loadHeroBuilds(
				apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
				h.Name, shop, names, "test", h, byClass, itemsByClass)
			if err != nil || cb == nil {
				t.Skipf("sin muestra para %s: %v", heroName, err)
			}
			if !cb.Rules.CanonicalOrder {
				t.Errorf("%s: los slots no son canónicos (cayó al fallback)", heroName)
			}
			for i, exp := range want {
				slot := rune('1' + i)
				got := cb.AbilityNames[slot]
				if got != exp {
					t.Errorf("slot %c = %q, esperado %q", slot, got, exp)
				}
			}
		})
	}
}

// TestSkillSlotsFixtureMatchesMetadata verifica que el fixture escrito a
// mano coincide con los metadatos canónicos del juego. Si el juego
// cambia un kit, este test lo detecta antes que la UI.
func TestSkillSlotsFixtureMatchesMetadata(t *testing.T) {
	api := diagClient(t)
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	byClass := api.abilityClassIndex()
	_, names, _ := api.shopItems()

	for heroName, want := range wantSlots {
		var h apiHero
		for _, x := range heroes {
			if x.Name == heroName {
				h = x
			}
		}
		if h.ID == 0 {
			continue
		}
		for i, exp := range want {
			class := h.Items[fmt.Sprintf("signature%d", i+1)]
			id, ok := byClass[class]
			if !ok {
				t.Errorf("%s signature%d: class %q no resuelve", heroName, i+1, class)
				continue
			}
			if got := names[id]; got != exp {
				t.Errorf("%s signature%d = %q (metadatos), fixture dice %q",
					heroName, i+1, got, exp)
			}
		}
	}
}
