package lolalytics

// Equivalencia de contenido entre el parser anterior y el optimizado.
//
// No basta con que parseQwik tenga éxito: se comparan los VALORES que
// consume MOBA TUI (ítems, botas, slots, runas, hechizos, matchups,
// estadísticas y tamaños de muestra) campo a campo.
//
// El parser anterior se conserva aquí como referencia (parseQwikLegacy)
// para poder comparar en el mismo proceso y con el mismo payload.

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/Eion21/mobatui/internal/model"
	"testing"
)

// parseQwikLegacy es la implementación anterior: decodifica a
// []json.RawMessage y luego hace un Unmarshal por objeto. Se mantiene
// SOLO en pruebas, como oráculo de equivalencia.
func parseQwikLegacy(html []byte) (*qwik, error) {
	m := qwikRe.FindSubmatch(html)
	if m == nil {
		return nil, fmt.Errorf("sin estado qwik/json")
	}
	var payload struct {
		Objs []json.RawMessage `json:"objs"`
	}
	if err := json.Unmarshal(m[1], &payload); err != nil {
		return nil, fmt.Errorf("estado qwik inválido: %w", err)
	}
	if len(payload.Objs) == 0 {
		return nil, fmt.Errorf("estado qwik vacío")
	}
	q := &qwik{n: len(payload.Objs), objs: make([]any, len(payload.Objs))}
	for i, raw := range payload.Objs {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			v = nil
		}
		q.objs[i] = v
	}
	return q, nil
}

// cachePayloads devuelve los HTML reales disponibles en la caché
// personal (solo lectura). Si no hay, se salta la prueba.
func cachePayloads(tb testing.TB) map[string][]byte {
	return cachePayloadsLimit(tb, 8)
}

// cachePayloadsLimit lee hasta max páginas qwik de la caché personal.
func cachePayloadsLimit(tb testing.TB, max int) map[string][]byte {
	tb.Helper()
	home, err := os.UserCacheDir()
	if err != nil {
		tb.Skip("sin caché")
	}
	dir := filepath.Join(home, "mobatui", "cache")
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Skip("sin caché personal")
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".gz" {
			continue
		}
		hash := e.Name()[:len(e.Name())-len(".json.body.gz")]
		f, err := os.Open(filepath.Join(dir, hash+".json.body.gz"))
		if err != nil {
			continue
		}
		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			continue
		}
		var body []byte
		buf := make([]byte, 64*1024)
		for {
			n, rerr := gz.Read(buf)
			body = append(body, buf[:n]...)
			if rerr != nil {
				break
			}
		}
		f.Close()
		// Solo interesan páginas con estado qwik.
		if qwikRe.Match(body) {
			out[hash] = body
		}
		if len(out) >= max {
			break
		}
	}
	if len(out) == 0 {
		tb.Skip("sin páginas qwik en caché")
	}
	return out
}

// pageFields extrae TODO lo que consume la UI de una página.
type pageFields struct {
	Games   int
	WR      float64
	Items   []flatItem
	Wards   []flatItem
	Boots   []flatItem
	Slots   [][]flatItem
	Combos  []comboRow
	Spells  []spellRow
	Starts  []flatItem
	Enemies []enemyRow
	Runes   []runeRow
}

type flatItem struct {
	ID, Games, Wins int
	WR              float64
	AvgBuyS         float64
}

type comboRow struct {
	IDs   []int
	WR    float64
	Games int
}
type spellRow struct {
	A, B  int
	WR    float64
	Games int
}
type enemyRow struct {
	ID    int
	WR    float64
	Games int
}
type runeRow struct {
	Key   string
	WR    float64
	Games int
}

// extractPage recorre la página con el parser dado y devuelve sus campos.
func extractPage(q *qwik) pageFields {
	var f pageFields
	main := q.mainData()
	if main == nil {
		return f
	}
	f.Games = q.int(main["n"])
	header := q.obj(main["header"])
	f.WR = q.num(header["wr"])

	conv := func(ts []model.ItemStats) []flatItem {
		out := make([]flatItem, 0, len(ts))
		for _, t := range ts {
			out = append(out, flatItem{ID: t.ID, Games: t.Games, Wins: t.Wins,
				WR: t.WinRate(), AvgBuyS: t.AvgBuyS})
		}
		return out
	}
	f.Items = conv(q.statTable(main["popularItem"]))
	f.Wards = conv(wardsOf(q.statTable(main["item"])))
	f.Boots = conv(dropBasicBoots(q.statTable(main["boots"])))
	for _, k := range []string{"item1", "item2", "item3", "item4", "item5"} {
		f.Slots = append(f.Slots, conv(q.statTable(main[k])))
	}
	f.Starts = conv(q.statTable(main["startItem"]))

	for _, c := range q.comboTable(main["builtBootSet3"]) {
		f.Combos = append(f.Combos, comboRow{IDs: c.IDs, WR: c.WinRate(), Games: c.Games})
	}
	for _, s := range q.spellsTable(main["spells"]) {
		f.Spells = append(f.Spells, spellRow{A: s.IDs[0], B: s.IDs[1], WR: s.WinRate(), Games: s.Games})
	}
	for _, e := range q.enemyTable(main["enemy"], "bottom") {
		f.Enemies = append(f.Enemies, enemyRow{ID: e.HeroID, WR: e.WinRate(), Games: e.Games})
	}
	summary := q.obj(main["summary"])
	if summary != nil {
		for _, k := range []string{"pick", "win"} {
			if rb, _ := q.quickbuild(summary[k], f.Games); rb != nil {
				f.Runes = append(f.Runes, runeRow{Key: fmt.Sprintf("%d/%d/%d", rb.PrimaryTree, rb.SecondaryTree, rb.KeystoneID), WR: rb.WinRate(), Games: rb.Games})
			}
		}
	}
	return f
}

// TestEquivalenciaParser compara ambos parsers campo a campo sobre
// todas las páginas disponibles en la caché personal.
func TestEquivalenciaParser(t *testing.T) {
	payloads := cachePayloads(t)
	hashes := make([]string, 0, len(payloads))
	for h := range payloads {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)

	for _, h := range hashes {
		body := payloads[h]
		oldQ, errOld := parseQwikLegacy(body)
		newQ, errNew := parseQwik(body)
		if (errOld == nil) != (errNew == nil) {
			t.Errorf("%s: error distinto: legacy=%v nuevo=%v", h[:8], errOld, errNew)
			continue
		}
		if errOld != nil {
			continue
		}
		if oldQ.n != newQ.n {
			t.Errorf("%s: nº objetos %d vs %d", h[:8], oldQ.n, newQ.n)
		}
		// Comparación profunda de TODO el array de objetos: si difieren
		// tipos numéricos, la comparación falla.
		if !reflect.DeepEqual(oldQ.objs, newQ.objs) {
			t.Errorf("%s: el array de objetos difiere", h[:8])
			// Localizar el primer índice distinto para el diagnóstico.
			for i := range oldQ.objs {
				if i < len(newQ.objs) && !reflect.DeepEqual(oldQ.objs[i], newQ.objs[i]) {
					t.Errorf("  primer objeto distinto en %d: legacy=%#v nuevo=%#v", i, oldQ.objs[i], newQ.objs[i])
					break
				}
			}
		}

		// Y los campos que consume la UI.
		fOld, fNew := extractPage(oldQ), extractPage(newQ)
		if !reflect.DeepEqual(fOld, fNew) {
			t.Errorf("%s: los campos de la página difieren", h[:8])
			if fOld.Games != fNew.Games {
				t.Errorf("  games: %d vs %d", fOld.Games, fNew.Games)
			}
			if fOld.WR != fNew.WR {
				t.Errorf("  wr: %v vs %v", fOld.WR, fNew.WR)
			}
			for name, a := range map[string]any{
				"items": fOld.Items, "wards": fOld.Wards, "boots": fOld.Boots,
				"slots": fOld.Slots, "combos": fOld.Combos, "spells": fOld.Spells,
				"starts": fOld.Starts, "enemies": fOld.Enemies, "runes": fOld.Runes,
			} {
				var b any
				switch name {
				case "items":
					b = fNew.Items
				case "wards":
					b = fNew.Wards
				case "boots":
					b = fNew.Boots
				case "slots":
					b = fNew.Slots
				case "combos":
					b = fNew.Combos
				case "spells":
					b = fNew.Spells
				case "starts":
					b = fNew.Starts
				case "enemies":
					b = fNew.Enemies
				case "runes":
					b = fNew.Runes
				}
				if !reflect.DeepEqual(a, b) {
					t.Errorf("  %s difiere", name)
				}
			}
		}
		t.Logf("EQUIV %s ok (objetos=%d items=%d slots=%d combos=%d)", h[:8], newQ.n, len(fNew.Items), len(fNew.Slots), len(fNew.Combos))
	}
}

// TestEquivalenciaCasosBorde cubre JSON incompleto, referencias
// inválidas y estados vacíos: ambos parsers deben coincidir también al
// fallar.
func TestEquivalenciaCasosBorde(t *testing.T) {
	casos := map[string]string{
		"vacio":           `<script type="qwik/json">{"objs":[]}</script>`,
		"sin-objs":        `<script type="qwik/json">{"ctx":{}}</script>`,
		"json-invalido":   `<script type="qwik/json">{no es json}</script>`,
		"objs-no-array":   `<script type="qwik/json">{"objs":"x"}</script>`,
		"referencia-mala": `<script type="qwik/json">{"objs":[{"a":"zzzz"},{"b":"~~~~"}]}</script>`,
		"ref-fuera-rango": `<script type="qwik/json">{"objs":[{"a":"zz"}]}</script>`,
	}
	for name, html := range casos {
		oldQ, errOld := parseQwikLegacy([]byte(html))
		newQ, errNew := parseQwik([]byte(html))
		if (errOld == nil) != (errNew == nil) {
			t.Errorf("%s: legacy err=%v, nuevo err=%v", name, errOld, errNew)
			continue
		}
		if errOld != nil {
			t.Logf("EQUIV-BORDE %s: ambos fallan (%v)", name, errNew)
			continue
		}
		if oldQ.n != newQ.n || !reflect.DeepEqual(oldQ.objs, newQ.objs) {
			t.Errorf("%s: contenido distinto", name)
		}
		t.Logf("EQUIV-BORDE %s: ambos ok (n=%d)", name, newQ.n)
	}
}

// ─── equivalencia en CI (sin caché personal) ─────────────────────
//
// Estas pruebas usan SOLO fixtures del repositorio: son obligatorias en
// CI y no dependen de la caché del usuario ni de la red.

// TestEquivalenciaFixtureCI compara ambos parsers sobre el fixture
// sintético, que reproduce la forma real del estado Qwik.
func TestEquivalenciaFixtureCI(t *testing.T) {
	body, err := os.ReadFile("testdata/qwik_page.html")
	if err != nil {
		t.Fatalf("fixture obligatorio: %v", err)
	}
	oldQ, errOld := parseQwikLegacy(body)
	newQ, errNew := parseQwik(body)
	if errOld != nil || errNew != nil {
		t.Fatalf("ambos deben parsear: legacy=%v nuevo=%v", errOld, errNew)
	}
	if oldQ.n != newQ.n {
		t.Fatalf("nº de objetos: %d vs %d", oldQ.n, newQ.n)
	}
	// Comparación profunda del array completo.
	if !reflect.DeepEqual(oldQ.objs, newQ.objs) {
		for i := range oldQ.objs {
			if !reflect.DeepEqual(oldQ.objs[i], newQ.objs[i]) {
				t.Fatalf("objeto %d distinto: legacy=%#v nuevo=%#v", i, oldQ.objs[i], newQ.objs[i])
			}
		}
		t.Fatal("los arrays de objetos difieren")
	}

	// Y los campos que consume la UI, con valores concretos: no basta
	// con que el parseo no falle.
	f := extractPage(newQ)
	if f.Games != 644677 {
		t.Errorf("games = %d, se esperaba 644677", f.Games)
	}
	if len(f.Items) != 2 {
		t.Errorf("items = %d, se esperaban 2", len(f.Items))
	} else if f.Items[0].ID != 3153 || f.Items[0].Games != 92114 {
		t.Errorf("item0 = %+v", f.Items[0])
	}
	if len(f.Slots) != 5 {
		t.Errorf("slots = %d, se esperaban 5", len(f.Slots))
	}
	if len(f.Slots[0]) != 1 || f.Slots[0][0].ID != 1018 {
		t.Errorf("item1 = %+v", f.Slots[0])
	}
	if len(f.Boots) != 1 || f.Boots[0].ID != 3006 {
		t.Errorf("boots = %+v", f.Boots)
	}
	if len(f.Combos) != 1 || len(f.Combos[0].IDs) != 3 || f.Combos[0].IDs[0] != 3153 {
		t.Errorf("combos = %+v", f.Combos)
	}
	if len(f.Spells) != 1 || f.Spells[0].A != 4 || f.Spells[0].B != 14 {
		t.Errorf("spells = %+v", f.Spells)
	}
	if len(f.Enemies) != 1 || f.Enemies[0].ID != 81 {
		t.Errorf("enemies = %+v", f.Enemies)
	}

	// El campo de parche se resuelve (base36 → texto).
	main := newQ.mainData()
	if main == nil {
		t.Fatal("sin mainData")
	}
	header := newQ.obj(main["header"])
	if header == nil {
		t.Fatal("sin header")
	}
	if got := newQ.str(header["patch"]); got != "13.22" {
		t.Errorf("patch = %q, se esperaba 13.22", got)
	}
}

// TestEquivalenciaCasosBordeCI cubre los casos de error con fixtures:
// ambos parsers deben coincidir también al fallar.
func TestEquivalenciaCasosBordeCI(t *testing.T) {
	casos := []string{
		"testdata/qwik_vacio.html",
		"testdata/qwik_sin_objs.html",
		"testdata/qwik_invalido.html",
		"testdata/qwik_objs_no_array.html",
		"testdata/qwik_ref_rota.html",
	}
	for _, path := range casos {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		oldQ, errOld := parseQwikLegacy(body)
		newQ, errNew := parseQwik(body)
		if (errOld == nil) != (errNew == nil) {
			t.Errorf("%s: legacy err=%v, nuevo err=%v", path, errOld, errNew)
			continue
		}
		if errOld != nil {
			// Mismo tipo de fallo. El mensaje puede diferir en el
			// nombre del tipo destino ([]jsontext.Value vs
			// []interface{}), que es exactamente el cambio de
			// implementación; lo que importa es que ambos FALLEN y con
			// la misma causa.
			if !strings.HasPrefix(errNew.Error(), "lolalytics: ") {
				t.Errorf("%s: falta el prefijo del paquete: %v", path, errNew)
			}
			tipo := ""
			switch {
			case strings.Contains(errOld.Error(), "estado qwik vacío"):
				tipo = "vacio"
			case strings.Contains(errOld.Error(), "estado qwik inválido"):
				tipo = "invalido"
			default:
				tipo = "otro"
			}
			if !strings.Contains(errNew.Error(), map[string]string{
				"vacio":    "estado qwik vacío",
				"invalido": "estado qwik inválido",
				"otro":     "",
			}[tipo]) {
				t.Errorf("%s: causa distinta:\n  legacy: %v\n  nuevo:  %v", path, errOld, errNew)
			}
			continue
		}
		if oldQ.n != newQ.n || !reflect.DeepEqual(oldQ.objs, newQ.objs) {
			t.Errorf("%s: contenido distinto", path)
		}
	}
}
