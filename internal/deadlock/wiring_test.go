package deadlock

// Pruebas de cableado: el índice de habilidades debe construirse desde el
// catálogo COMPLETO, no desde shop. Si alguien vuelve a derivarlo de
// shop, estas pruebas fallan.
//
// Son deterministas: sirven un catálogo mínimo con httptest, así que no
// dependen de data/cache (ignorada por git) ni de la red. Las variantes
// que validan contra el catálogo REAL viven en wiring_real_test.go y se
// omiten cuando la caché no está.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/model"
)

// wiringFixture monta una API mínima pero suficiente para el cableado
// canónico: un héroe con sus cuatro firmas, un catálogo con muchas más
// habilidades que objetos comprables, y las analíticas de ese héroe.
type wiringFixture struct {
	t   *testing.T
	api *APIClient
}

// kitSlot es una habilidad del kit del héroe.
type kitSlot struct {
	id    int
	name  string
	class string
}

func newWiringFixture(t *testing.T) *wiringFixture {
	t.Helper()

	// Kit del héroe: cuatro firmas resolubles por class_name.
	kit := []kitSlot{
		{id: 9001, name: "Wiring Signature One", class: "ability_wiring_1"},
		{id: 9002, name: "Wiring Signature Two", class: "ability_wiring_2"},
		{id: 9003, name: "Wiring Signature Three", class: "ability_wiring_3"},
		{id: 9004, name: "Wiring Signature Four", class: "ability_wiring_4"},
	}

	// Catálogo: el kit + habilidades extra para superar el centenar y
	// demostrar que el índice sale del catálogo completo.
	const padding = 120
	items := make([]map[string]any, 0, len(kit)+padding+2)
	for _, k := range kit {
		items = append(items, map[string]any{
			"id": k.id, "name": k.name, "type": "ability",
			"class_name": k.class, "hero": 1,
		})
	}
	for i := 0; i < padding; i++ {
		items = append(items, map[string]any{
			"id": 10000 + i, "name": fmt.Sprintf("Pad %d", i), "type": "ability",
			"class_name": fmt.Sprintf("ability_pad_%d", i), "hero": 0,
		})
	}
	// Dos comprables: shop debe contener solo esto.
	items = append(items,
		map[string]any{
			"id": 500, "name": "Wiring Shop Item", "type": "upgrade",
			"class_name": "upgrade_wiring", "item_slot_type": "weapon", "item_tier": 1,
		},
		map[string]any{
			"id": 501, "name": "Wiring Component", "type": "upgrade",
			"class_name": "upgrade_wiring_component", "item_slot_type": "weapon",
			"item_tier": 2, "component_items": []string{"upgrade_wiring"},
		},
	)
	itemsBody := mustJSON(t, items)

	heroesBody := mustJSON(t, []map[string]any{{
		"id": 1, "name": "Solomon", "player_selectable": true,
		"items": map[string]string{
			"signature1": kit[0].class,
			"signature2": kit[1].class,
			"signature3": kit[2].class,
			"signature4": kit[3].class,
		},
	}})

	heroStatsBody := mustJSON(t, []map[string]any{
		{"hero_id": 1, "wins": 500, "matches": 1000},
	})
	monthlyBody := mustJSON(t, []map[string]any{
		{"hero_id": 1, "bucket": 100, "wins": 250, "matches": 500},
	})
	itemStatsBody := mustJSON(t, []map[string]any{
		{"item_id": 500, "wins": 120, "matches": 200, "avg_buy_time_s": 300},
		{"item_id": 501, "wins": 90, "matches": 200, "avg_buy_time_s": 900},
		// Una habilidad en las stats de ítems: no debe colarse en shop.
		{"item_id": 9001, "wins": 10, "matches": 10},
	})
	// Secuencias de 15 puntos sobre el kit, con topes 5/4/4/2.
	sequence := []int{9001, 9001, 9001, 9002, 9001, 9003, 9002, 9004,
		9001, 9002, 9003, 9002, 9003, 9003, 9004}
	abilityStatsBody := mustJSON(t, []map[string]any{
		{"abilities": sequence, "wins": 260, "matches": 500},
	})

	mux := http.NewServeMux()
	json := func(body []byte) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(body)
		}
	}
	mux.HandleFunc("/v1/assets/client-versions", json([]byte(`[5044,6512]`)))
	mux.HandleFunc("/v1/assets/items", json(itemsBody))
	mux.HandleFunc("/v1/assets/heroes", json(heroesBody))
	mux.HandleFunc("/v1/analytics/hero-stats", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("bucket") != "" {
			json(monthlyBody)(w, r)
			return
		}
		json(heroStatsBody)(w, r)
	})
	mux.HandleFunc("/v1/analytics/item-stats", json(itemStatsBody))
	mux.HandleFunc("/v1/analytics/ability-order-stats", json(abilityStatsBody))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return &wiringFixture{
		t: t,
		api: NewAPI(c, filepath.Join("..", "..", "testdata", "deadlock")).
			WithAPIBase(srv.URL).
			WithHTTPClient(srv.Client()),
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestWiringIndexComesFromFullCatalog: el índice de class_name debe
// contener habilidades, no solo objetos comprables.
func TestWiringIndexComesFromFullCatalog(t *testing.T) {
	api := newWiringFixture(t).api

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
	if len(shop) != 2 {
		t.Fatalf("shop = %d ítems, se esperaban 2 comprables", len(shop))
	}

	// El índice debe cubrir el catálogo completo, no solo shop.
	idx := api.abilityClassIndex()
	if len(idx) < 100 {
		t.Fatalf("abilityClassIndex tiene %d entradas: parece construido desde shop", len(idx))
	}
	// Y debe resolver las firmas del héroe conocido.
	heroes, err := api.heroes()
	if err != nil {
		t.Fatalf("héroes: %v", err)
	}
	var sol apiHero
	for _, h := range heroes {
		if h.Name == "Solomon" {
			sol = h
		}
	}
	if sol.ID == 0 {
		t.Fatal("Solomon no está en el catálogo de prueba")
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
	api := newWiringFixture(t).api

	cbs, err := api.LoadBuilds()
	if err != nil {
		t.Fatalf("carga: %v", err)
	}
	if len(cbs) != 1 {
		t.Fatalf("builds = %d, se esperaba 1", len(cbs))
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

	// Los cuatro espacios llevan el nombre de la firma correspondiente:
	// el slot de cada habilidad sale del orden del juego, no del uso.
	cb := cbs[0]
	want := map[rune]string{
		'1': "Wiring Signature One",
		'2': "Wiring Signature Two",
		'3': "Wiring Signature Three",
		'4': "Wiring Signature Four",
	}
	for slot, name := range want {
		if got := cb.AbilityNames[slot]; got != name {
			t.Errorf("slot %c = %q, se esperaba %q", slot, got, name)
		}
	}
	// La habilidad MÁS usada es la 1; el slot 3 debe seguir siendo la 3.
	if len(cb.Rules.Alphabet) != 4 {
		t.Fatalf("alfabeto = %v", cb.Rules.Alphabet)
	}
	if cap := cb.Rules.MaxPoints['1']; cap != 5 {
		t.Errorf("tope del slot 1 = %d, se esperaba 5", cap)
	}
	if cap := cb.Rules.MaxPoints['4']; cap != 2 {
		t.Errorf("tope del slot 4 = %d, se esperaba 2", cap)
	}
}
