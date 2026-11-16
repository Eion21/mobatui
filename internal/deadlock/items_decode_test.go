package deadlock

// Regresión del contrato de /v1/assets/items: el endpoint entrega una
// LISTA de ítems (no un objeto {data: …}) y el decodificador debe
// aceptar además la forma reducida del fixture local. El fallo original
// (json: cannot unmarshal object into Go value of type []deadlock.apiItem)
// rompía shopItems, abilityClassIndex y, con ello, el orden canónico.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
)

// TestDecodeItemsResponseCatalogShapes fija las dos formas aceptadas y
// el rechazo explícito de un objeto que no es el fixture.
func TestDecodeItemsResponseCatalogShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []apiItem
	}{
		{
			name: "endpoint array",
			body: `[{"id":7,"name":"Array item","type":"ability","class_name":"ability_array"}]`,
			want: []apiItem{{ID: 7, Name: "Array item", Type: "ability", ClassName: "ability_array"}},
		},
		{
			name: "fixture data object keyed by id",
			body: `{"data":{"102":{"name":"Aguante extra"},"101":{"name":"Salud extra","tier":1,"class_name":"item_101"}}}`,
			// El fixture no declara "type": sus entradas son comprables.
			// Su "tier" tampoco es el campo del endpoint (item_tier), así
			// que no se rellena ItemTier: no se inventan datos.
			want: []apiItem{
				{ID: 101, Name: "Salud extra", Type: "upgrade", ClassName: "item_101"},
				{ID: 102, Name: "Aguante extra", Type: "upgrade"},
			},
		},
		{
			// Un documento que sí declara tipos no se reescribe: la API
			// real siempre trae "type" en las 782 entradas.
			name: "data object with declared types",
			body: `{"data":{"1":{"name":"Habilidad","type":"ability"},"2":{"name":"Objeto","type":"upgrade"}}}`,
			want: []apiItem{{ID: 1, Name: "Habilidad", Type: "ability"}, {ID: 2, Name: "Objeto", Type: "upgrade"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var items []apiItem
			if err := decodeItemsResponse([]byte(tt.body), &items); err != nil {
				t.Fatalf("decodeItemsResponse: %v", err)
			}
			if !reflect.DeepEqual(items, tt.want) {
				t.Fatalf("items = %#v, se esperaba %#v", items, tt.want)
			}
		})
	}

	// El orden del objeto no puede filtrarse a la salida: el índice de
	// habilidades y la tienda se construyen recorriendo la lista.
	var una, otra []apiItem
	a := `{"data":{"2":{"name":"dos"},"1":{"name":"uno"},"3":{"name":"tres"}}}`
	b := `{"data":{"3":{"name":"tres"},"1":{"name":"uno"},"2":{"name":"dos"}}}`
	if err := decodeItemsResponse([]byte(a), &una); err != nil {
		t.Fatal(err)
	}
	if err := decodeItemsResponse([]byte(b), &otra); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(una, otra) {
		t.Fatalf("el orden de las claves altera el resultado: %#v vs %#v", una, otra)
	}

	if err := decodeItemsResponse([]byte(`{"unexpected":true}`), new([]apiItem)); err == nil {
		t.Fatal("un objeto sin data debía rechazarse")
	}
	if err := decodeItemsResponse([]byte(`not-json`), new([]apiItem)); err == nil {
		t.Fatal("un cuerpo inválido debía rechazarse")
	}
}

// TestDecodeItemsResponseRejectsDegenerateBodies fija la frontera entre
// "catálogo vacío legítimo" y "cuerpo que no es un catálogo": ninguno de
// los segundo debe convertirse en un catálogo vacío silencioso.
func TestDecodeItemsResponseRejectsDegenerateBodies(t *testing.T) {
	invalid := map[string]string{
		"null":            "null decodifica a nil sin error, pero no es catálogo",
		"string":          `"texto"`,
		"number":          `0`,
		"objeto sin data": `{"unexpected":true}`,
		"data nula":       `{"data":null}`,
		"truncado":        `[{"id":1`,
	}
	for name, body := range invalid {
		t.Run("rechaza "+name, func(t *testing.T) {
			var items []apiItem
			if err := decodeItemsResponse([]byte(body), &items); err == nil {
				t.Fatalf("%q debía rechazarse, devolvió %d ítems", body, len(items))
			}
		})
	}

	// Una lista vacía sí es un catálogo vacío válido.
	for _, body := range []string{`[]`, " [ ]\n", `{"data":{}}`} {
		var items []apiItem
		if err := decodeItemsResponse([]byte(body), &items); err != nil {
			t.Fatalf("%q es un catálogo vacío válido: %v", body, err)
		}
		if len(items) != 0 {
			t.Fatalf("%q debía dar 0 ítems, dio %d", body, len(items))
		}
	}
}

// TestDecodeItemsResponseRejectsDuplicateIDs: dos claves con el mismo id
// dejarían el orden indefinido, así que el documento es corrupto.
func TestDecodeItemsResponseRejectsDuplicateIDs(t *testing.T) {
	body := `{"data":{"1":{"id":5,"name":"a"},"2":{"id":5,"name":"b"}}}`
	var items []apiItem
	if err := decodeItemsResponse([]byte(body), &items); err == nil {
		t.Fatalf("ids duplicados debían rechazarse, devolvió %d ítems", len(items))
	}
}

// TestDecodeItemsResponseRealCatalogShape: con la forma REAL del
// endpoint (lista, tipos declarados) el resultado debe ser idéntico al
// de json.Unmarshal directo: el decodificador no altera el catálogo
// remoto ni inventa clasificaciones.
func TestDecodeItemsResponseRealCatalogShape(t *testing.T) {
	// Subconjunto fiel: los tres tipos que publica la API, con los
	// campos que consume el cliente.
	body := `[
		{"id":2521299219,"class_name":"ability_golden_idol","name":"Soul Urn","type":"ability","hero":0},
		{"id":1548066885,"class_name":"upgrade_clip_size","name":"Extended Magazine","type":"upgrade",
		 "item_slot_type":"weapon","item_tier":1,"cost":500,
		 "component_items":["upgrade_basic_magazine"]},
		{"id":1009965641,"class_name":"citadel_weapon_shotgun","name":"Shotgun","type":"weapon","item_tier":0}
	]`

	var want []apiItem
	if err := json.Unmarshal([]byte(body), &want); err != nil {
		t.Fatal(err)
	}
	var got []apiItem
	if err := decodeItemsResponse([]byte(body), &got); err != nil {
		t.Fatalf("decodeItemsResponse: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("el catálogo remoto cambió:\n got=%#v\nwant=%#v", got, want)
	}
	// Y ninguna clasificación inferida: los tipos son los declarados.
	byID := map[int]string{}
	for _, it := range got {
		byID[it.ID] = it.Type
	}
	if byID[2521299219] != "ability" || byID[1548066885] != "upgrade" || byID[1009965641] != "weapon" {
		t.Fatalf("tipos alterados: %v", byID)
	}
}

// TestDecodeItemsResponseLocalFixture decodifica el fixture real del
// repositorio: es la forma que el respaldo local entrega sin red.
func TestDecodeItemsResponseLocalFixture(t *testing.T) {
	body, err := os.ReadFile("../../testdata/deadlock/items.json")
	if err != nil {
		t.Fatal(err)
	}
	var items []apiItem
	if err := decodeItemsResponse(body, &items); err != nil {
		t.Fatalf("fixture local: %v", err)
	}
	if len(items) != 20 {
		t.Fatalf("ítems del fixture = %d, se esperaban 20", len(items))
	}
	// Los ids salen de las claves del objeto, no del cuerpo.
	if items[0].ID != 101 || items[len(items)-1].ID != 306 {
		t.Fatalf("ids del fixture = %d..%d, se esperaban 101..306", items[0].ID, items[len(items)-1].ID)
	}
	if items[0].Name != "Salud extra" || items[0].ClassName != "item_101" {
		t.Fatalf("primer ítem = %#v", items[0])
	}
	// Sin red, el respaldo debe seguir sirviendo la tienda: el fixture no
	// declara "type" en ninguna entrada.
	for _, it := range items {
		if it.Type != "upgrade" {
			t.Fatalf("ítem %d del fixture sin clasificar: type=%q", it.ID, it.Type)
		}
	}
}

// TestItemsEndpointFeedsCanonicalOrder prueba el cableado completo con
// la MISMA forma que documenta la API (lista): catálogo → índice de
// class_name → orden canónico signature1..4. Sin este decodificador el
// catálogo queda vacío y canonicalAbilityOrder siempre devuelve nil.
func TestItemsEndpointFeedsCanonicalOrder(t *testing.T) {
	items := []map[string]any{
		{"id": 10, "name": "Uno", "type": "ability", "class_name": "ability_uno", "hero": 1},
		{"id": 20, "name": "Dos", "type": "ability", "class_name": "ability_dos", "hero": 1},
		{"id": 30, "name": "Tres", "type": "ability", "class_name": "ability_tres", "hero": 1},
		{"id": 40, "name": "Cuatro", "type": "ability", "class_name": "ability_cuatro", "hero": 1},
		{"id": 50, "name": "Objeto", "type": "upgrade", "class_name": "upgrade_objeto", "item_slot_type": "weapon"},
	}
	body, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assets/items", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(c, "../../testdata/deadlock").
		WithAPIBase(srv.URL).
		WithHTTPClient(srv.Client())

	// La lista llega entera: habilidades y comprables.
	if got := len(api.allItems()); got != len(items) {
		t.Fatalf("allItems() = %d ítems, se esperaban %d", got, len(items))
	}
	idx := api.abilityClassIndex()
	if len(idx) != 4 {
		t.Fatalf("abilityClassIndex = %d entradas, se esperaban 4: %v", len(idx), idx)
	}
	if idx["ability_tres"] != 30 {
		t.Fatalf("ability_tres = %d, se esperaba 30", idx["ability_tres"])
	}

	hero := apiHero{ID: 1, Items: map[string]string{
		"signature1": "ability_tres",
		"signature2": "ability_uno",
		"signature3": "ability_cuatro",
		"signature4": "ability_dos",
	}}
	want := []int64{30, 10, 40, 20}
	if got := canonicalAbilityOrder(hero, idx); !reflect.DeepEqual(got, want) {
		t.Fatalf("orden canónico = %v, se esperaba %v", got, want)
	}

	// El mismo cliente sirve la tienda desde esa única respuesta.
	shop, names, err := api.shopItems()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := shop[50]; !ok || len(shop) != 1 {
		t.Fatalf("shop = %v, se esperaba solo el ítem 50", shop)
	}
	if names[10] != "Uno" || len(names) != 4 {
		t.Fatalf("abilityNames = %v", names)
	}
}

// TestItemsEndpointRejectsInvalidBody: un cuerpo que no es catálogo debe
// fallar en decodificación, no devolver un catálogo vacío en silencio.
func TestItemsEndpointRejectsInvalidBody(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assets/items", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"error":"boom"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	api := NewAPI(c, "../../testdata/deadlock").
		WithAPIBase(srv.URL).
		WithHTTPClient(srv.Client())

	var items []apiItem
	if err := getItems(api, &items); err == nil {
		t.Fatal("un catálogo con forma desconocida debía devolver error")
	}
	if len(api.allItems()) != 0 {
		t.Fatal("allItems() no debe inventar un catálogo tras un cuerpo inválido")
	}
}
