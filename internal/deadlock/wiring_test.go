package deadlock

// Prueba de cableado: el índice de habilidades debe construirse desde el
// catálogo COMPLETO, no desde shop. Si alguien vuelve a derivarlo de
// shop, esta prueba falla.

import (
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// TestWiringIndexComesFromFullCatalog: el índice de class_name debe
// contener habilidades, no solo objetos comprables.
func TestWiringIndexComesFromFullCatalog(t *testing.T) {
	api := diagClient(t)
	shop, _, err := api.shopItems()
	if err != nil {
		t.Fatalf("catálogo de tienda: %v", err)
	}
	// shop solo trae comprables: ninguna habilidad.
	fromShop := 0
	for _, it := range shop {
		if it.Type == "ability" {
			fromShop++
		}
	}
	if fromShop != 0 {
		t.Fatalf("precondición rota: shop contiene %d habilidades", fromShop)
	}

	// El índice real debe tener cientos de habilidades.
	idx := api.abilityClassIndex()
	if len(idx) < 100 {
		t.Fatalf("abilityClassIndex tiene %d entradas: parece construido desde shop", len(idx))
	}
	// Y debe resolver las firmas de un héroe conocido.
	heroes, _ := api.heroes()
	var sol apiHero
	for _, h := range heroes {
		if h.Name == "Solomon" {
			sol = h
		}
	}
	if sol.ID == 0 {
		t.Skip("Solomon no está")
	}
	for i := 1; i <= 4; i++ {
		class := sol.Items[signatureKey(i)]
		if _, ok := idx[class]; !ok {
			t.Errorf("signature%d (%q) no resuelve en el índice", i, class)
		}
	}
}

// TestWiringLoadBuildsResolvesCanonical: el flujo real de LoadBuilds
// produce slots canónicos, no fallback.
func TestWiringLoadBuildsResolvesCanonical(t *testing.T) {
	api := diagClient(t)
	cbs, err := api.LoadBuilds()
	if err != nil {
		t.Skipf("carga: %v", err)
	}
	if len(cbs) == 0 {
		t.Skip("sin builds")
	}
	canonical, fallback := 0, 0
	for _, cb := range cbs {
		if cb.Game != model.GameDeadlock {
			continue
		}
		if len(cb.Rules.Alphabet) == 0 {
			continue
		}
		if cb.Rules.CanonicalOrder {
			canonical++
		} else {
			fallback++
		}
	}
	t.Logf("héroes con orden canónico: %d · con fallback: %d", canonical, fallback)
	if canonical == 0 {
		t.Fatal("ningún héroe resolvió canónico: el cableado está roto")
	}
	// Con el índice correcto, el fallback debe ser la excepción.
	if fallback > canonical {
		t.Errorf("más héroes en fallback (%d) que canónicos (%d)", fallback, canonical)
	}
}
