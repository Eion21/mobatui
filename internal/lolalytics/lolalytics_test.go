package lolalytics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Eion21/builds-tui/internal/cache"
	"github.com/Eion21/builds-tui/internal/clean"
	"github.com/Eion21/builds-tui/internal/model"
)

// qwikBuilder construye un estado qwik con refs base36 como el real.
type qwikBuilder struct {
	objs []any
}

func (b *qwikBuilder) add(v any) string {
	b.objs = append(b.objs, v)
	return strconv.FormatInt(int64(len(b.objs)-1), 36)
}

// rawJSON marca un valor ya serializado.
type jsonRaw string

func rawJSON(s string) jsonRaw { return jsonRaw(s) }

func mustJSON(v any) string {
	switch t := v.(type) {
	case string:
		return strconv.Quote(t)
	case int:
		return strconv.Itoa(t)
	case float64:
		return fmt.Sprintf("%v", t)
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = mustJSON(e)
		}
		return "[" + strings.Join(parts, ",") + "]"
	case jsonRaw:
		return string(t)
	}
	return fmt.Sprintf("%v", v)
}

// tierlistHTML construye una página de tierlist con filas campeón×carril.
func tierlistHTML(lane string, rows [][3]any) string {
	b := &qwikBuilder{}
	var rowObjs []any
	for _, r := range rows {
		cid := r[0].(int)
		games := r[2].(int)
		laneRef := b.add(lane)
		gamesRef := b.add(games)
		pctRef := b.add(99.0)
		wrRef := b.add(52.5)
		rowJSON := fmt.Sprintf(`{"lane":%q,"pctLane":%q,"wr":%q,"games":%q}`,
			laneRef, pctRef, wrRef, gamesRef)
		rowRef := b.add(rawJSON(rowJSON))
		cidRef := b.add(cid)
		rowObjs = append(rowObjs, rawJSON(fmt.Sprintf(`{"$$nav":"0","cid":%q,"row":%q,"$$fields":"0","placeholder":"x"}`, cidRef, rowRef)))
	}
	// índice champPath y patch.
	paths := map[string]string{"103": "ahri", "64": "leesin"}
	b.add(rawJSON(`{"champPath":` + refMapJSON(b, paths) + `,"patch":"16.21"}`))
	var sb strings.Builder
	sb.WriteString(`<html><script type="qwik/json">{"objs":[`)
	for _, o := range b.objs {
		sb.WriteString(mustJSON(o))
		sb.WriteByte(',')
	}
	for _, o := range rowObjs {
		sb.WriteString(mustJSON(o))
		sb.WriteByte(',')
	}
	sb.WriteString(`"fin"]}</script></html>`)
	return sb.String()
}

func refMapJSON(b *qwikBuilder, m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("%q:%q", k, b.add(v)))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// itemRowAt arma una fila [id, wr%, pick, games, minuto] con refs.
func itemRowAt(b *qwikBuilder, id int, wr float64, games int, minute float64) any {
	return rawJSON(fmt.Sprintf(`[%q,%q,%q,%q,%q]`,
		b.add(id), b.add(wr), b.add(50.0), b.add(games), b.add(minute)))
}

// buildPageHTML construye la página /build/ con mainData completo.
func buildPageHTML() string { return buildPageHTMLConPopular(true) }

// buildPageHTMLConPopular permite omitir popularItem para cubrir el
// respaldo sobre la tabla item.
func buildPageHTMLConPopular(popular bool) string {
	b := &qwikBuilder{}
	// Tabla de ítems: [id, wr%, pick, games, minuto] (elementos por ref).
	itemRow := func(id int, wr float64, games int) any {
		return itemRowAt(b, id, wr, games, 0)
	}
	items := []any{itemRow(6653, 53.5, 39210), itemRow(9999, 80.0, 5000), itemRow(3020, 52.0, 31022), itemRow(2055, 55.0, 12000)}
	itemsRef := b.add(items)
	// popularItem: solo ítems completados, con su minuto de compra.
	popRows := []any{
		itemRowAt(b, 6653, 53.5, 30000, 24),
		itemRowAt(b, 3157, 52.0, 12000, 16),
		itemRowAt(b, 3089, 54.0, 8000, 30),
	}
	popRef := b.add(popRows)
	// Boots incluye la base 1001, que no es una recomendación.
	bootsRef := b.add([]any{
		itemRowAt(b, 1001, 51.0, 30000, 5),
		itemRowAt(b, 3020, 52.0, 20000, 14),
	})

	// Slots por turno (item1..item5): el top de cada uno con su minuto;
	// el slot 2 además lleva un segundo ítem.
	slotRefs := make([]string, 5)
	for i := 0; i < 5; i++ {
		rows := []any{itemRowAt(b, 3116, 53.0, 2000, float64(12+i*6))}
		if i == 1 {
			rows = append(rows, itemRowAt(b, 3065, 51.0, 900, 20))
		}
		slotRefs[i] = b.add(rows)
	}
	// builtBootSet3: tríos con WR conjunto, ya en orden de presencia.
	combosRef := b.add([]any{
		rawJSON(`["3116_3047_6653","51.54","2.13","130"]`),
		rawJSON(`["6657_3047_3116","55.56","2.69","126"]`),
	})
	// spells: pares de hechizos concatenados y ordenados ("414" = 4+14).
	spellsRef := b.add([]any{
		itemRowAt(b, 414, 53.4, 26605, 0),
		itemRowAt(b, 411, 52.0, 1200, 0),
		itemRowAt(b, 99, 55.0, 900, 0), // no es un par válido: fuera
	})
	// startItem: ítems iniciales (mismo formato que item).
	startRef := b.add([]any{
		itemRowAt(b, 2003, 51.0, 30000, 0),
		itemRowAt(b, 3340, 51.5, 16000, 0),
	})
	// enemy: por carril, filas [id, wr%, pick%, delta, %games, games].
	enemyRow := func(id int, wr, pick, delta, pctG float64, games int) any {
		return rawJSON(fmt.Sprintf(`[%q,%q,%q,%q,%q,%q]`,
			b.add(id), b.add(wr), b.add(pick), b.add(delta), b.add(pctG), b.add(games)))
	}
	enemyRef := b.add(rawJSON(fmt.Sprintf(`{"middle":%q}`, b.add([]any{
		enemyRow(157, 49.69, 1.64, -0.26, 5.56, 2105),
		enemyRow(777, 52.54, 4.27, 2.37, 4.47, 1690),
		enemyRow(0, 50.0, 1.0, 0.0, 1.0, 500), // id inválido: fuera
	}))))

	// Quickbuild pick: runas + skillorder (id como entero grande).
	priIDs := []any{b.add(8112), b.add(8126), b.add(8138)}
	secIDs := []any{b.add(8210), b.add(8237)}
	modIDs := []any{b.add(5008), b.add(5005), b.add(5001)}
	setRef := b.add(rawJSON(fmt.Sprintf(`{"pri":%q,"sec":%q,"mod":%q}`,
		b.add(priIDs), b.add(secIDs), b.add(modIDs))))
	pageRef := b.add(rawJSON(fmt.Sprintf(`{"pri":%q,"sec":%q}`, b.add(1), b.add(2)))) // 1=Dom 2=Bruj
	runesRef := b.add(rawJSON(fmt.Sprintf(`{"wr":%q,"n":%q,"page":%q,"set":%q}`,
		b.add(52.8), b.add(21020), pageRef, setRef)))
	skillRef := b.add(rawJSON(fmt.Sprintf(`{"id":%q,"n":%q,"wr":%q}`,
		b.add(2.13114121242233e+14), b.add(12804), b.add(52.5))))
	qbPick := b.add(rawJSON(fmt.Sprintf(`{"runes":%q,"skillorder":%q}`, runesRef, skillRef)))

	// Quickbuild win (solo runas, sin skillorder válido).
	skillWin := b.add(rawJSON(fmt.Sprintf(`{"id":%q,"n":%q,"wr":%q}`,
		b.add("corto"), b.add(3000), b.add(55.0))))
	qbWin := b.add(rawJSON(fmt.Sprintf(`{"runes":%q,"skillorder":%q}`, runesRef, skillWin)))

	summaryRef := b.add(rawJSON(fmt.Sprintf(`{"pick":%q,"win":%q}`, qbPick, qbWin)))
	headerRef := b.add(rawJSON(fmt.Sprintf(`{"wr":%q,"pr":%q}`, b.add(52.8), b.add(4.2))))
	nRef := b.add(51933)
	mainFields := fmt.Sprintf(`"header":%q,"summary":%q,"item":%q`,
		headerRef, summaryRef, itemsRef)
	if popular {
		mainFields += fmt.Sprintf(`,"popularItem":%q`, popRef)
	}
	mainFields += fmt.Sprintf(`,"boots":%q,"skillOrder":["x"],"n":%q`, bootsRef, nRef)
	mainFields += fmt.Sprintf(`,"item1":%q,"item2":%q,"item3":%q,"item4":%q,"item5":%q,"builtBootSet3":%q`,
		slotRefs[0], slotRefs[1], slotRefs[2], slotRefs[3], slotRefs[4], combosRef)
	mainFields += fmt.Sprintf(`,"spells":%q`, spellsRef)
	mainFields += fmt.Sprintf(`,"startItem":%q,"enemy":%q`, startRef, enemyRef)
	main := b.add(rawJSON("{" + mainFields + "}"))

	var sb strings.Builder
	sb.WriteString(`<html><script type="qwik/json">{"objs":[`)
	for _, o := range b.objs {
		sb.WriteString(mustJSON(o))
		sb.WriteByte(',')
	}
	sb.WriteString(fmt.Sprintf(`"fin"]}</script><!-- main=%s --></html>`, main))
	return sb.String()
}

func TestParseQwikYMainData(t *testing.T) {
	html := buildPageHTML()
	q, err := parseQwik([]byte(html))
	if err != nil {
		t.Fatal(err)
	}
	m := q.mainData()
	if m == nil {
		t.Fatal("sin mainData")
	}
	if got := q.int(m["n"]); got != 51933 {
		t.Errorf("n = %d", got)
	}
}

func TestStatTableFiltraPlaceholders(t *testing.T) {
	html := buildPageHTML()
	q, _ := parseQwik([]byte(html))
	m := q.mainData()
	items := q.statTable(m["item"])
	// 3 filas: 6653, 3020 y el ward 2055 (los wards los separa
	// buildPage, no statTable).
	if len(items) != 3 {
		t.Fatalf("ítems = %d (el 9999 debía filtrarse)", len(items))
	}
	if items[0].ID != 6653 || items[0].Games != 39210 {
		t.Errorf("fila ítem = %+v", items[0])
	}
	wr := items[0].WinRate()
	if wr < 0.53 || wr > 0.54 {
		t.Errorf("wr del ítem = %f (debía ~0.535)", wr)
	}
}

func TestDigitsToQWER(t *testing.T) {
	got, ok := digitsToQWER("213114121242233")
	if !ok || got != "WQEQQRQWQWRWWEE" {
		t.Errorf("digitsToQWER = %q, %v", got, ok)
	}
	if _, ok := digitsToQWER("213"); ok {
		t.Error("longitud corta aceptada")
	}
	if _, ok := digitsToQWER("21311412124223X"); ok {
		t.Error("dígito inválido aceptado")
	}
}

func TestTreeFromIndex(t *testing.T) {
	if treeFromIndex(1) != 8100 || treeFromIndex(2) != 8200 {
		t.Errorf("treeFromIndex(1)=%d treeFromIndex(2)=%d", treeFromIndex(1), treeFromIndex(2))
	}
	if treeFromIndex(8400) != 8400 {
		t.Error("un id existente debía pasar tal cual")
	}
}

func TestLoadBuildsServidorCompleto(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/lol/tierlist/", func(w http.ResponseWriter, r *http.Request) {
		l := r.URL.Query().Get("lane")
		if l == "" {
			l = "top" // tierlist global (para el patch)
		}
		rows := [][3]any{{103, l, 4200}}
		if l == "middle" {
			rows = append(rows, [3]any{64, l, 100}) // sin muestra
		}
		w.Write([]byte(tierlistHTML(l, rows)))
	})
	mux.HandleFunc("/lol/ahri/build/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTML()))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	cbs, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cbs) == 0 || len(cbs) > 1*len(lanes) {
		t.Fatalf("builds = %d", len(cbs))
	}
	cb := cbs[0]
	if cb.Game != model.GameLoL || cb.ChampionKey != "Ahri" || cb.Source != "lolalytics.com" {
		t.Errorf("metadatos: %+v", cb)
	}
	if cb.Patch != "16.21" {
		t.Errorf("patch = %q", cb.Patch)
	}
	// Roles con muestra: el lane con 100 partidas queda fuera.
	if len(cb.RunePages) == 0 {
		t.Fatal("sin runas")
	}
	rp := cb.RunePages[0]
	if rp.PrimaryTree != 8100 || rp.SecondaryTree != 8200 || rp.KeystoneID != 8112 {
		t.Errorf("runas = %+v", rp)
	}
	if len(cb.Skills) == 0 || cb.Skills[0].Sequence != "WQEQQRQWQWRWWEE" {
		t.Fatalf("skills = %+v", cb.Skills)
	}
	// Reglas LoL-15 y validación de la secuencia.
	if err := model.ValidateSequence(cb.Skills[0].Sequence, cb.Rules); err != nil {
		t.Errorf("secuencia inválida con reglas derivadas: %v", err)
	}
	// El pipeline completo funciona con estos datos.
	cleaned := clean.BuildAll(cbs, clean.Options{})
	if len(cleaned) == 0 || len(cleaned[0].SkillOrder.Priority) != 3 {
		t.Errorf("pipeline: %+v", cleaned)
	}
}

// TestBuildPageUsaPopularItemYBotasFinales: la tabla item trae
// componentes, consumibles y wards, y el ranking por Wilson los premia
// porque se compran casi siempre. buildPage debe leer popularItem (ya
// filtrado por Lolalytics), guardar el minuto de compra y descartar las
// botas base; sin popularItem, cae a item.
func TestBuildPageUsaPopularItemYBotasFinales(t *testing.T) {
	casos := []struct {
		nombre  string
		popular bool
		want    []int
	}{
		{"popularItem", true, []int{6653, 3157, 3089}},
		{"respaldo item", false, []int{6653, 3020}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(buildPageHTMLConPopular(c.popular)))
			}))
			defer srv.Close()
			cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
			if err != nil {
				t.Fatal(err)
			}
			cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
			bd, err := cl.buildPage("ahri", "middle")
			if err != nil {
				t.Fatal(err)
			}
			var got []int
			for _, it := range bd.items {
				got = append(got, it.ID)
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("items = %v, se esperaba %v", got, c.want)
			}
			for _, b := range bd.boots {
				if b.ID == 1001 {
					t.Error("las botas base (1001) no debían conservarse")
				}
			}
		})
	}

	// El minuto de compra de popularItem llega en segundos.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTMLConPopular(true)))
	}))
	defer srv.Close()
	cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	bd, err := cl.buildPage("ahri", "middle")
	if err != nil {
		t.Fatal(err)
	}
	if bd.items[0].AvgBuyS != 24*60 {
		t.Errorf("AvgBuyS = %v, se esperaba %v (24 min)", bd.items[0].AvgBuyS, 24*60)
	}
}

// TestBuildPageSlotsYCombos: item1..item5 dan el índice de compra con
// su minuto medio y builtBootSet3 los tríos con WR conjunto.
func TestBuildPageSlotsYCombos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTMLConPopular(true)))
	}))
	defer srv.Close()
	cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	bd, err := cl.buildPage("ahri", "middle")
	if err != nil {
		t.Fatal(err)
	}
	if len(bd.slots) != 5 {
		t.Fatalf("slots = %d, se esperaban 5", len(bd.slots))
	}
	if got := bd.slots[0][0]; got.ID != 3116 || got.AvgBuyS != 12*60 {
		t.Errorf("slots[0][0] = %+v, se esperaba 3116 a los 12 min", got)
	}
	if got := bd.slots[1][1]; got.ID != 3065 {
		t.Errorf("slots[1][1] = %+v, se esperaba 3065", got)
	}
	if got := bd.slots[4][0]; got.ID != 3116 || got.AvgBuyS != 36*60 {
		t.Errorf("slots[4][0] = %+v, se esperaba 3116 a los 36 min", got)
	}
	if len(bd.combos) != 2 {
		t.Fatalf("combos = %d, se esperaban 2", len(bd.combos))
	}
	c := bd.combos[0]
	if want := []int{3116, 3047, 6653}; !slices.Equal(c.IDs, want) {
		t.Errorf("combo[0].IDs = %v, se esperaba %v", c.IDs, want)
	}
	if c.Games != 130 || c.PickRate != 2.13 {
		t.Errorf("combo[0] = %+v", c)
	}
	if wr := c.WinRate(); wr < 0.515 || wr > 0.516 {
		t.Errorf("combo[0].WR = %f, se esperaba ~0.5154", wr)
	}
}

// TestParseComboIDs: un combo con celda inválida o fuera de rango se
// descarta entero.
func TestParseComboIDs(t *testing.T) {
	if got := parseComboIDs("3116_3047_6653"); !slices.Equal(got, []int{3116, 3047, 6653}) {
		t.Errorf("combo válido = %v", got)
	}
	for _, s := range []string{"", "3116", "3116_x_6653", "0_3047", "9999_3047_3116", "3116__3047"} {
		if got := parseComboIDs(s); got != nil {
			t.Errorf("parseComboIDs(%q) = %v, se esperaba nil", s, got)
		}
	}
}

// TestParseSpellPair: el id concatenado de la tabla spells se parte en
// los dos hechizos de la Grieta, en orden ascendente; lo que no parte
// limpio se descarta.
func TestParseSpellPair(t *testing.T) {
	casos := []struct {
		in   string
		a, b int
		ok   bool
	}{
		{"414", 4, 14, true}, // Destello + Encender
		{"411", 4, 11, true}, // Destello + Castigo (jungla)
		{"34", 3, 4, true},   // Agotar + Destello
		{"114", 1, 14, true}, // ordenado: 1+14, no 11+4
		{"1214", 12, 14, true},
		{"7", 7, 0, true},  // un solo hechizo
		{"44", 4, 4, true}, // doble Destello (arena)
		{"99", 0, 0, false},
		{"4142", 0, 0, false},
	}
	for _, c := range casos {
		a, b, ok := parseSpellPair(c.in)
		if ok != c.ok || a != c.a || b != c.b {
			t.Errorf("parseSpellPair(%q) = %d,%d,%v; se esperaba %d,%d,%v", c.in, a, b, ok, c.a, c.b, c.ok)
		}
	}
}

// TestBuildPageHechizos: la tabla spells da los pares de hechizos más
// usados con su WR conjunto.
func TestBuildPageHechizos(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTMLConPopular(true)))
	}))
	defer srv.Close()
	cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	bd, err := cl.buildPage("ahri", "middle")
	if err != nil {
		t.Fatal(err)
	}
	if len(bd.spells) != 2 {
		t.Fatalf("spells = %+v, se esperaban 2 pares válidos", bd.spells)
	}
	if bd.spells[0].IDs != [2]int{4, 14} || bd.spells[0].Games != 26605 {
		t.Errorf("spells[0] = %+v, se esperaba [4 14] con 26605 partidas", bd.spells[0])
	}
	if bd.spells[1].IDs != [2]int{4, 11} {
		t.Errorf("spells[1] = %+v, se esperaba [4 11]", bd.spells[1])
	}
}

// TestBuildPageEnemyYInicio: la tabla enemy (por carril) da los
// matchups con partidas y WR contra cada enemigo, y startItem los
// ítems iniciales.
func TestBuildPageEnemyYInicio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTMLConPopular(true)))
	}))
	defer srv.Close()
	cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	bd, err := cl.buildPage("ahri", "middle")
	if err != nil {
		t.Fatal(err)
	}
	if len(bd.enemys) != 2 {
		t.Fatalf("enemys = %+v, se esperaban 2 válidos", bd.enemys)
	}
	if bd.enemys[0].HeroID != 157 || bd.enemys[0].Games != 2105 {
		t.Errorf("enemys[0] = %+v, se esperaba 157 con 2105 partidas", bd.enemys[0])
	}
	wr := bd.enemys[0].WinRate()
	if wr < 0.496 || wr > 0.497 {
		t.Errorf("WR contra 157 = %f, se esperaba ~0.4969", wr)
	}
	if len(bd.starts) != 2 || bd.starts[0].ID != 2003 {
		t.Errorf("starts = %+v, se esperaba 2003 primero", bd.starts)
	}
}

// TestBuildPageWards: la tabla item trae los wards (2055/3340/3363);
// se extraen aparte de los ítems y quedan disponibles para su sección.
func TestBuildPageWards(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(buildPageHTMLConPopular(true)))
	}))
	defer srv.Close()
	cc, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(cc).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	bd, err := cl.buildPage("ahri", "middle")
	if err != nil {
		t.Fatal(err)
	}
	if len(bd.wards) != 1 || bd.wards[0].ID != 2055 || bd.wards[0].Games != 12000 {
		t.Errorf("wards = %+v, se esperaba solo 2055 con 12000 partidas", bd.wards)
	}
	for _, it := range bd.items {
		if it.ID == 2055 {
			t.Error("el ward 2055 no debía colarse en los ítems")
		}
	}
}

func TestLoadBuildsFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
	called := false
	cbs, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, func() ([]*model.ChampionBuilds, error) {
		called = true
		return []*model.ChampionBuilds{}, nil
	})
	if err != nil || !called || len(cbs) != 0 {
		t.Errorf("fallback = %v, llamado=%v, err=%v", cbs, called, err)
	}
}
