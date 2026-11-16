package ddragon

// Pruebas del catálogo enriquecido (fase 3). Sirven el fixture
// items-catalog.json desde httptest: sin red real ni caché personal.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/lolitems"
)

// catalogClient monta un cliente cuyo item.json es el fixture enriquecido.
func catalogClient(t *testing.T) (*Client, *int) {
	t.Helper()
	body, err := os.ReadFile("../../testdata/ddragon/items-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var loN int
	mux := http.NewServeMux()
	// versions.json: el cliente lo pide primero.
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		loN++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`["16.21.1"]`))
	})
	mux.HandleFunc("/cdn/16.21.1/data/es_ES/item.json", func(w http.ResponseWriter, r *http.Request) {
		loN++
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, "../../testdata/ddragon", "es_ES").
		WithHTTPClient(srv.Client())
	cl.baseURL = srv.URL
	return cl, &loN
}

// TestItemCatalogDecodifica ataca los metadatos nuevos.
func TestItemCatalogDecodifica(t *testing.T) {
	cl, _ := catalogClient(t)
	cat, err := cl.ItemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) < 150 {
		t.Fatalf("catálogo con %d objetos: demasiado pequeño", len(cat))
	}

	// Botas definitivas: tags, coste, cadena de mejoras y mapa.
	boots, ok := cat[3006]
	if !ok {
		t.Fatal("falta 3006")
	}
	if boots.Name != "Grebas de berserker" {
		t.Errorf("nombre = %q", boots.Name)
	}
	if len(boots.Tags) == 0 || !containsTag(boots.Tags, "Boots") {
		t.Errorf("tags = %v", boots.Tags)
	}
	if boots.Gold.Total != 1100 {
		t.Errorf("gold = %d", boots.Gold.Total)
	}
	if len(boots.From) == 0 {
		t.Error("las botas tienen componentes y deben traerlos")
	}
	if !boots.Maps[lolitems.MapSummonersRift] {
		t.Errorf("maps = %v", boots.Maps)
	}
	if !boots.InStore {
		t.Error("inStore debía ser true (ausente en el parche)")
	}

	// Trinket: se detecta por tag.
	if tr, ok := cat[3340]; ok && !tr.Trinket {
		t.Errorf("3340 debía marcarse como trinket: tags=%v", tr.Tags)
	}

	// Objeto final sin mejoras.
	inf, ok := cat[3031]
	if !ok {
		t.Fatal("falta 3031")
	}
	if len(inf.Into) != 0 {
		t.Errorf("Filo infinito no debe mejorar a nada: %v", inf.Into)
	}
	if !lolitems.Eligible(inf) {
		t.Errorf("Filo infinito debía ser elegible: %s", lolitems.Reason(inf))
	}
}

// TestItemCatalogSinPeticionesExtra: ItemCatalog reutiliza la misma URL
// y caché que Items(); no debe introducir llamadas adicionales.
func TestItemCatalogSinPeticionesExtra(t *testing.T) {
	cl, calls := catalogClient(t)
	if _, err := cl.ItemCatalog(); err != nil {
		t.Fatal(err)
	}
	after := *calls

	// Una segunda llamada sale de caché.
	if _, err := cl.ItemCatalog(); err != nil {
		t.Fatal(err)
	}
	if *calls != after {
		t.Errorf("ItemCatalog repitió peticiones: %d → %d", after, *calls)
	}

	// Items() usa la misma clave de caché: tampoco debe pedir de nuevo.
	if _, err := cl.Items(); err != nil {
		t.Fatal(err)
	}
	if *calls != after {
		t.Errorf("Items() volvió a pedir tras ItemCatalog: %d → %d", after, *calls)
	}
}

// TestItemsSigueIgual: el método existente no cambia de contrato.
func TestItemsSigueIgual(t *testing.T) {
	cl, _ := catalogClient(t)
	items, err := cl.Items()
	if err != nil {
		t.Fatal(err)
	}
	if items[3020] != "Botas de hechicero" {
		t.Errorf("3020 = %q", items[3020])
	}
	if items[3089] != "Sombrero mortal de Rabadon" {
		t.Errorf("3089 = %q", items[3089])
	}
	// El catálogo enriquecido trae los mismos nombres.
	cat, err := cl.ItemCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for id, name := range items {
		if got := cat[id].Name; got != name && got != "" {
			t.Errorf("%d: Items()=%q vs ItemCatalog()=%q", id, name, got)
		}
	}
}

// TestItemCatalogSinRed: sin red cae al fixture ligero, que no trae
// metadatos; debe devolver el catálogo sin romperse.
func TestItemCatalogSinRed(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, "../../testdata/ddragon", "es_ES").
		WithHTTPClient(&http.Client{Transport: failingTransport{}})

	cat, err := cl.ItemCatalog()
	if err != nil {
		t.Fatalf("el respaldo local debía servir el catálogo: %v", err)
	}
	if len(cat) == 0 {
		t.Fatal("catálogo vacío desde el fixture")
	}
	// El fixture ligero no tiene tags: esos objetos no son elegibles.
	sinTags := 0
	for _, it := range cat {
		if len(it.Tags) == 0 {
			sinTags++
			if lolitems.Eligible(it) {
				t.Errorf("%s sin tags no debía ser elegible", it.Name)
			}
		}
	}
	if sinTags == 0 {
		t.Error("se esperaba algún objeto sin tags en el fixture ligero")
	}
	// Y los nombres siguen resolviéndose.
	if _, ok := cat[3020]; !ok {
		t.Error("3020 debía estar")
	}
}
