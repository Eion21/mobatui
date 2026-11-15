package deadlock

// Auditoría de cobertura: distingue ausencia de estadísticas de ausencia
// de posiciones canónicas. Son cosas distintas y no deben confundirse.

import (
	"fmt"
	"testing"
)

func TestCoverageAudit(t *testing.T) {
	api := diagClient(t)
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	byClass := api.abilityClassIndex()
	itemsByClass := api.abilityItemsByClass()
	shop, names, _ := api.shopItems()

	var (
		total          = len(heroes)
		withFourSig    int
		withSample     int
		canonical      int
		fallback       int
		transforming   int
		noSample       []string
		noSignatures   []string
		fallbackHeroes []string
	)

	for _, h := range heroes {
		// ¿Tiene las cuatro firmas resolubles?
		sigs := 0
		for i := 1; i <= 4; i++ {
			if _, ok := byClass[h.Items[signatureKey(i)]]; ok {
				sigs++
			}
		}
		if sigs == 4 {
			withFourSig++
		} else {
			noSignatures = append(noSignatures, fmt.Sprintf("%s (%d/4)", h.Name, sigs))
		}
		// ¿Tiene sub-habilidades (cambio de forma)?
		hasSub := false
		for i := 1; i <= 4; i++ {
			class := h.Items[signatureKey(i)]
			if it, ok := itemsByClass[class]; ok {
				for _, dep := range it.DependentAbilities {
					if dep.IsSubAbility() {
						hasSub = true
					}
				}
			}
		}
		if hasSub {
			transforming++
		}
		// ¿Tiene muestra estadística?
		cb, err := api.loadHeroBuilds(apiHeroStat{HeroID: h.ID, Matches: 1000, Wins: 500},
			h.Name, shop, names, "t", h, byClass, itemsByClass)
		if err != nil || cb == nil || len(cb.Rules.Alphabet) == 0 {
			noSample = append(noSample, h.Name)
			continue
		}
		withSample++
		if cb.Rules.CanonicalOrder {
			canonical++
		} else {
			fallback++
			fallbackHeroes = append(fallbackHeroes, h.Name)
		}
	}

	t.Logf("=== COBERTURA ===")
	t.Logf("héroes en el catálogo:              %d", total)
	t.Logf("con las 4 firmas resolubles:       %d", withFourSig)
	t.Logf("con muestra estadística:            %d", withSample)
	t.Logf("  → con orden canónico:             %d", canonical)
	t.Logf("  → con fallback declarado:         %d %v", fallback, fallbackHeroes)
	t.Logf("con habilidades de cambio de forma: %d", transforming)
	t.Logf("sin muestra (no es falta de posiciones): %d", len(noSample))
	if len(noSignatures) > 0 {
		t.Logf("sin las 4 firmas: %v", noSignatures)
	}

	// Invariantes de la auditoría.
	if withFourSig < total {
		t.Logf("AVISO: %d héroes sin las cuatro firmas resolubles", total-withFourSig)
	}
	if canonical == 0 {
		t.Fatal("ningún héroe canónico: el cableado está roto")
	}
	if fallback > 0 {
		t.Logf("AVISO: %d héroes en fallback declarado (se presentan como tales)", fallback)
	}
}
