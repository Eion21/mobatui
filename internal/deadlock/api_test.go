package deadlock

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Eion21/builds-tui/internal/cache"
	"github.com/Eion21/builds-tui/internal/clean"
	"github.com/Eion21/builds-tui/internal/model"
)

// progressCall registra un aviso de progreso para los tests.
type progressCall struct {
	done, total int
	name        string
}

// cannedAPI monta un httptest que simula los endpoints de
// deadlock-api.com con datos mínimos pero fieles a las formas reales:
// 4 habilidades de Abrams con topes 4/4/4/3 y 15 puntos de habilidad.
func cannedAPI(t *testing.T) (*APIClient, *httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	base := "" // URL del CDN dentro del propio servidor de pruebas
	mux := http.NewServeMux()
	jsonHandler := func(body func() string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, body())
		}
	}
	mux.HandleFunc("/v1/assets/heroes", jsonHandler(func() string {
		return fmt.Sprintf(`[
			{"id":6,"name":"Abrams","player_selectable":true,
			 "images":{"icon_image_small":"%s/cdn/bull_sm.png"},
			 "description":{"role":"Brawler frontliner"}},
			{"id":99,"name":"SecretHero","player_selectable":true,"in_development":true}
		]`, base)
	}))
	mux.HandleFunc("/v1/assets/items", jsonHandler(func() string {
		return fmt.Sprintf(`[
			{"id":7409189,"name":"Melee Charge","type":"upgrade","image":"%s/cdn/melee.png","shop_image":"%s/cdn/shop/melee.png","cost":1600},
			{"id":3633614685,"name":"Extra Health","type":"upgrade","class_name":"item_extra_health","image":"","shop_image":"%s/cdn/shop/health.png","cost":800},
			{"id":111222333,"name":"Health Nova","type":"upgrade","component_items":["item_extra_health"],"shop_image":"%s/cdn/shop/nova.png"},
			{"id":2824119765,"name":"Shoulder Charge","type":"ability","hero":6},
			{"id":715762406,"name":"Infernal Resilience","type":"ability","hero":6},
			{"id":509856396,"name":"Seismic Impact","type":"ability","hero":6},
			{"id":4072270083,"name":"Siphon Life","type":"ability","hero":6}
		]`, base, base, base, base)
	}))
	mux.HandleFunc("/v1/assets/client-versions", jsonHandler(func() string {
		return `[5044, 6512]`
	}))
	mux.HandleFunc("/v1/analytics/item-stats", jsonHandler(func() string {
		return `[
			{"item_id":7409189,"wins":5300,"matches":10200},
			{"item_id":3633614685,"wins":8200,"matches":16000},
			{"item_id":2824119765,"wins":3,"matches":3}
		]`
	}))
	// Secuencias de 15 puntos con ids reales del kit de Abrams
	// (topes 4/4/4/3): siphon=4072270083, shoulder=2824119765,
	// seismic=509856396, infernal=715762406.
	mux.HandleFunc("/v1/analytics/ability-order-stats", jsonHandler(func() string {
		return `[
			{"abilities":[4072270083,2824119765,4072270083,509856396,4072270083,715762406,4072270083,2824119765,509856396,2824119765,715762406,2824119765,509856396,509856396,715762406],
			 "wins":2260,"matches":4200},
			{"abilities":[2824119765,4072270083,2824119765,509856396,2824119765,715762406,2824119765,4072270083,509856396,4072270083,715762406,4072270083,509856396,509856396,715762406],
			 "wins":930,"matches":1800}
		]`
	}))
	mux.HandleFunc("/cdn/bull_sm.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNG-BYTES"))
	})
	mux.HandleFunc("/cdn/shop/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("SHOP-PNG " + filepath.Base(r.URL.Path)))
	})
	mux.HandleFunc("/v1/analytics/hero-stats", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("bucket") == "start_time_month" {
			fmt.Fprint(w, `[
				{"hero_id":6,"bucket":100,"wins":1000,"matches":2000},
				{"hero_id":6,"bucket":200,"wins":1100,"matches":2000},
				{"hero_id":6,"bucket":300,"wins":1200,"matches":2000}
			]`)
			return
		}
		fmt.Fprint(w, `[{"hero_id":6,"wins":49300,"matches":98400}]`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	base = srv.URL

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := NewAPI(c, "../../testdata/deadlock").
		WithAPIBase(srv.URL).
		WithHTTPClient(srv.Client())
	return cl, srv, calls
}

// TestHeroNamesByIDParaMatchups: los cruces (matchups/sinergias)
// incluyen héroes jugables marcados in_development (Violet, Baba en la
// API real), así que la resolución id→nombre los trae a todos aunque
// el roster siga sin listarlos.
func TestHeroNamesByIDParaMatchups(t *testing.T) {
	cl, _, _ := cannedAPI(t)
	byID := cl.HeroNamesByID()
	if byID[6] != "Abrams" {
		t.Errorf("byID[6] = %q, se esperaba Abrams", byID[6])
	}
	if byID[99] != "SecretHero" {
		t.Errorf("byID[99] = %q: un héroe jugable en desarrollo debe resolverse en los cruces", byID[99])
	}
	// El roster no cambia: sigue sin listar al héroe en desarrollo.
	roster, err := cl.Heroes()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := roster["SecretHero"]; ok {
		t.Error("el roster no debía listar al héroe en desarrollo")
	}
}

// TestItemUpgradesDL: component_items de la API arma el mapa base →
// mejoras directas (Health Nova contiene a Extra Health como
// componente, mapeando class_name → id).
func TestItemUpgradesDL(t *testing.T) {
	cl, _, _ := cannedAPI(t)
	m, err := cl.ItemUpgrades()
	if err != nil {
		t.Fatal(err)
	}
	got := m[3633614685]
	if len(got) != 1 || got[0] != 111222333 {
		t.Errorf("upgrades de Extra Health = %v, se esperaba [111222333]", got)
	}
	if len(m) > 1 {
		t.Errorf("mapa con entradas espurias: %v", m)
	}
}

// TestLoadBuildsReportaProgreso: cada héroe resuelto avisa por el
// callback, con total fijo y el nombre del héroe recién cargado.
func TestLoadBuildsReportaProgreso(self *testing.T) {
	cl, _, _ := cannedAPI(self)
	var mu sync.Mutex
	var calls []progressCall
	cl.OnProgress(func(done, total int, name string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, progressCall{done, total, name})
	})
	cbs, err := cl.LoadBuilds()
	if err != nil {
		self.Fatal(err)
	}
	if len(cbs) != 1 {
		self.Fatalf("builds = %d, se esperaba 1", len(cbs))
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		self.Fatalf("avisos = %d, se esperaba 1 héroe reportado", len(calls))
	}
	if calls[0].done != 1 || calls[0].total != 1 {
		self.Errorf("aviso = %d/%d, se esperaba 1/1", calls[0].done, calls[0].total)
	}
	if calls[0].name != "Abrams" {
		self.Errorf("nombre = %q, se esperaba Abrams", calls[0].name)
	}
}

func TestAPIClientProvider(t *testing.T) {
	cl, _, _ := cannedAPI(t)
	v, err := cl.Version()
	if err != nil || v != "client 6512" {
		t.Errorf("Version = %q, %v", v, err)
	}
	heroes, err := cl.Heroes()
	if err != nil {
		t.Fatal(err)
	}
	if h, ok := heroes["Abrams"]; !ok || h.Title != "Brawler frontliner" {
		t.Errorf("Abrams = %+v", h)
	}
	if _, ok := heroes["SecretHero"]; ok {
		t.Error("un héroe en desarrollo no debía listarse")
	}
	items, err := cl.Items()
	if err != nil {
		t.Fatal(err)
	}
	if items[3633614685] != "Extra Health" {
		t.Errorf("ítem 3633614685 = %q", items[3633614685])
	}
	if runes, trees, err := cl.Runes(); err != nil || len(runes) != 0 || len(trees) != 0 {
		t.Error("Runes debía ser vacío sin error")
	}
	icon, err := cl.ChampionIcon("Abrams")
	if err != nil || string(icon) != "PNG-BYTES" {
		t.Errorf("icono = %q, %v", icon, err)
	}
	// Los ítems usan el icono de la TIENDA (shop_image), no el interno.
	shopIcon, err := cl.ItemIcon(3633614685)
	if err != nil || string(shopIcon) != "SHOP-PNG health.png" {
		t.Errorf("icono de ítem = %q, %v (se esperaba shop_image)", shopIcon, err)
	}
}

func TestAPIClientLoadBuilds(t *testing.T) {
	cl, _, _ := cannedAPI(t)
	cbs, err := cl.LoadBuilds()
	if err != nil {
		t.Fatal(err)
	}
	if len(cbs) != 1 {
		t.Fatalf("héroes cargados = %d, se esperaba 1", len(cbs))
	}
	cb := cbs[0]
	if cb.Source != "deadlock-api.com" || cb.Role != model.RoleAll {
		t.Errorf("metadatos: %+v", cb)
	}
	if cb.Total.Games != 98400 {
		t.Errorf("overview = %d", cb.Total.Games)
	}
	// Las habilidades que colaran en item-stats (3/3, trampa) quedan
	// fuera por no ser ítems de tienda.
	for _, it := range cb.Items {
		if it.ID == 2824119765 {
			t.Error("una habilidad coló como ítem de tienda")
		}
	}
	if len(cb.Items) != 2 {
		t.Errorf("ítems = %d, se esperaban 2 de tienda", len(cb.Items))
	}

	// Reglas por héroe: 15 puntos, 4 slots, topes derivados (4/4/4/3).
	if len(cb.Rules.Alphabet) != 4 || cb.Rules.Length != 15 {
		t.Fatalf("reglas = %+v", cb.Rules)
	}
	for _, slot := range cb.Rules.Alphabet {
		if cap := cb.Rules.MaxPoints[slot]; cap != 4 && cap != 3 {
			t.Errorf("tope inesperado para %c = %d", slot, cap)
		}
	}
	// Slots por uso total; a igualdad, id ascendente: las tres
	// habilidades de 4 puntos empatan y Seismic Impact (id menor)
	// queda como slot 1. La leyenda trae el nombre de cada slot.
	if cb.AbilityNames['1'] != "Seismic Impact" {
		t.Errorf("leyenda = %+v", cb.AbilityNames)
	}
	if len(cb.AbilityNames) != 4 {
		t.Errorf("leyenda = %+v", cb.AbilityNames)
	}

	// El pipeline valida la secuencia contra las reglas derivadas.
	sorted := clean.BuildAll(cbs, clean.Options{})
	b := sorted[0]
	prio := string(b.SkillOrder.Priority)
	if len(prio) != 4 {
		t.Errorf("prioridad = %q", prio)
	}
	// La secuencia top empieza por Siphon Life = slot 3 en este mapeo.
	if b.SkillOrder.Sequence[0] != '3' || b.AbilityNames['3'] != "Siphon Life" {
		t.Errorf("secuencia top = %s (slot0=%c)", b.SkillOrder.Sequence, b.SkillOrder.Sequence[0])
	}
	if b.AbilityNames[b.SkillOrder.Priority[0]] == "" {
		t.Error("la prioridad máxima no tiene nombre en la leyenda")
	}
}

func TestAPIClientFallbackAFixtures(t *testing.T) {
	// Servidor caído y sin caché: los builds caen a los fixtures locales
	// y los estáticos al respaldo.
	cl, _, _ := cannedAPI(t)
	cl.WithHTTPClient(&http.Client{Transport: failTransport{}})
	cbs, err := cl.LoadBuilds()
	if err != nil {
		t.Fatal(err)
	}
	if len(cbs) != 14 {
		t.Errorf("fixtures = %d, se esperaban 14", len(cbs))
	}
	for _, cb := range cbs {
		if cb.Source != "tracklock (fixtures)" {
			t.Errorf("source del fallback = %q", cb.Source)
		}
	}
}

type failTransport struct{}

func (failTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("sin red (test)")
}
