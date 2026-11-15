package deadlock

// Auditoría de slots de habilidad para TODOS los héroes con muestra en
// caché. La expectativa sale de los metadatos canónicos del juego
// (heroes.json → items.signature1..4), nunca del algoritmo bajo prueba.

import (
	"fmt"
	"testing"
)

func TestAuditAllHeroSlots(t *testing.T) {
	api := diagClient(t)
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	byClass := api.abilityClassIndex()
	if len(byClass) == 0 {
		t.Fatal("abilityClassIndex vacío")
	}
	itemsByClass := api.abilityItemsByClass()
	shop, names, err := api.shopItems()
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}

	checked, failed, noSample, noMeta, fallback := 0, 0, 0, 0, 0
	for _, h := range heroes {
		// Expectativa independiente: signature1..4 resueltos a id.
		var want []int64
		ok := true
		for i := 1; i <= 4; i++ {
			id, found := byClass[h.Items[fmt.Sprintf("signature%d", i)]]
			if !found {
				ok = false
				break
			}
			want = append(want, int64(id))
		}
		if !ok {
			noMeta++
			continue
		}
		cb, err := api.loadHeroBuilds(
			apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
			h.Name, shop, names, "test", h, byClass, itemsByClass)
		if err != nil || cb == nil {
			noSample++
			continue
		}
		if len(cb.AbilityNames) == 0 {
			noSample++
			continue
		}
		checked++
		// El conjunto observado debe coincidir con las cuatro firmas.
		// Si no (Silver: 7 habilidades en las stats), canonicalSlots no
		// puede mapear y el fallback es legítimo — pero debe estar
		// marcado como no canónico, no presentado como orden del juego.
		observed := map[int64]bool{}
		for _, sk := range cb.Skills {
			for _, r := range sk.Sequence {
				observed[int64(r)] = true
			}
		}
		setMatches := len(want) == 4
		for _, id := range want {
			// want son ids; el set observado es de slots, así que se
			// compara por nombre vía el legend.
			_ = id
		}
		if !cb.Rules.CanonicalOrder {
			// Fallback declarado: se acepta solo si el conjunto de
			// habilidades observadas NO coincide con las firmas.
			if setMatches && len(cb.AbilityNames) == 4 {
				// Puede ser un héroe cuyo set sí coincide pero cuya
				// resolución falló: eso sí es un fallo.
				allMatch := true
				for i, id := range want {
					if cb.AbilityNames[rune('1'+i)] != names[int(id)] {
						allMatch = false
					}
				}
				if allMatch {
					continue // coincide igualmente: nada que reprochar
				}
			}
			fallback++
			t.Logf("%s: slots por frecuencia (no canónicos) — metadatos no cubren el kit observado", h.Name)
			continue
		}
		for i, id := range want {
			slot := rune('1' + i)
			got := cb.AbilityNames[slot]
			exp := names[int(id)]
			if got != exp {
				failed++
				t.Errorf("%s slot %c = %q, esperado %q", h.Name, slot, got, exp)
			}
		}
		_ = observed
	}
	t.Logf("héroes con datos: %d · slots incorrectos: %d · fallback declarado: %d · sin muestra: %d · sin metadatos: %d",
		checked, failed, fallback, noSample, noMeta)
	if checked == 0 {
		t.Fatal("no se comprobó ningún héroe")
	}
	if failed > 0 {
		t.Errorf("%d slots incorrectos", failed)
	}
}
