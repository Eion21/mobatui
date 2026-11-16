package deadlock

// Variantes de cableado contra el catálogo REAL de deadlock-api.com.
//
// Estas pruebas consumen data/cache (caché de aplicación, ignorada por
// git) y por eso se OMITEN cuando no hay entradas locales. Las
// comprobaciones equivalentes y deterministas viven en wiring_test.go,
// que no necesita caché ni red.
//
// Ejecutar solo estas:  go test ./internal/deadlock -run 'Real' -v

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// realCatalogClient devuelve un cliente offline sobre data/cache, pero
// se OMITE si no hay ni una sola entrada de deadlock-api.com: un
// checkout limpio tiene el directorio vacío o inexistente.
func realCatalogClient(t *testing.T) *APIClient {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	if err != nil {
		t.Skipf("ruta de caché: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("sin caché local en %s", dir)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		meta, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var m struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(meta, &m) != nil {
			continue
		}
		if strings.Contains(m.URL, "deadlock-api.com") {
			return diagClient(t)
		}
	}
	t.Skipf("sin entradas de deadlock-api en %s: caché personal no versionada", dir)
	return nil
}

// TestWiringIndexComesFromFullCatalogReal: con el catálogo real
// cacheado, el índice debe cubrir cientos de habilidades.
func TestWiringIndexComesFromFullCatalogReal(t *testing.T) {
	api := realCatalogClient(t)

	idx := api.abilityClassIndex()
	if len(idx) < 100 {
		t.Fatalf("abilityClassIndex tiene %d entradas: parece construido desde shop", len(idx))
	}
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	var sol apiHero
	for _, h := range heroes {
		if h.Name == "Solomon" {
			sol = h
		}
	}
	if sol.ID == 0 {
		t.Skip("Solomon no está en la caché")
	}
	for i := 1; i <= 4; i++ {
		class := sol.Items[signatureKey(i)]
		if _, ok := idx[class]; !ok {
			t.Errorf("signature%d (%q) no resuelve en el índice", i, class)
		}
	}
}

// TestWiringLoadBuildsResolvesCanonicalReal: con datos reales, la
// mayoría de los héroes debe resolver su orden canónico.
func TestWiringLoadBuildsResolvesCanonicalReal(t *testing.T) {
	api := realCatalogClient(t)

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
	if fallback > canonical {
		t.Errorf("más héroes en fallback (%d) que canónicos (%d)", fallback, canonical)
	}
}
