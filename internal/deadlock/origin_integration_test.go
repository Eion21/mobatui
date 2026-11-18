package deadlock

// Integración de la procedencia en FetchRuns.
//
// Regresión: `FetchRuns` calculaba `memoKey` con `Cache.Last()`, que es
// estado GLOBAL. Con dos héroes consultados a la vez, la clave de uno
// podía construirse con la fecha del otro y reutilizar una muestra que
// no le correspondía.
//
// Sin red y sin caché personal: httptest + t.TempDir().
//
// Ejecutar con -race para validar la concurrencia.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
)

// runFixture fabrica la respuesta de /v1/matches/metadata con una
// partida por héroe. Cada partida compra dos ítems del catálogo de test
// (shopItems lo resuelve con los fixtures locales).
func runFixture(hero int, n int) []byte {
	matches := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		matches = append(matches, map[string]any{
			"match_id":     int64(1000 + i),
			"start_time":   int64(1700000000 + i),
			"winning_team": "Team0",
			"players": []map[string]any{{
				"hero_id":              hero,
				"player_match_outcome": "Win",
				"team":                 "Team0",
				"items": []map[string]any{
					{"item_id": 101, "game_time_s": 300, "sold_time_s": 0, "net_worth_at_buy": 1000},
					{"item_id": 102, "game_time_s": 600, "sold_time_s": 0, "net_worth_at_buy": 2000},
				},
			}},
		})
	}
	b, _ := json.Marshal(matches)
	return b
}

// originServer sirve metadata por héroe con latencia controlada y cuenta
// peticiones. La latencia se gobierna por héroe, de modo que una
// consulta lenta y otra rápida puedan intercalarse sin sleeps.
type originServer struct {
	mu    sync.Mutex
	delay map[int]time.Duration
	hits  map[int]int
	total atomic.Int64
}

func newOriginServer(t *testing.T) (*originServer, *httptest.Server) {
	st := &originServer{delay: map[int]time.Duration{}, hits: map[int]int{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Solo cuenta la muestra: /v1/assets/items también llega aquí
		// cuando el fixture no cubre el catálogo, y no es la consulta
		// que se está midiendo.
		if !strings.Contains(r.URL.Path, "matches/metadata") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hero := 76
		if v := r.URL.Query().Get("hero_ids"); v != "" {
			if n, err := parseInt(v); err == nil {
				hero = n
			}
		}
		st.total.Add(1)
		st.mu.Lock()
		d := st.delay[hero]
		st.hits[hero]++
		st.mu.Unlock()
		if d > 0 {
			time.Sleep(d)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(runFixture(hero, 120))
	}))
	return st, srv
}

func (s *originServer) setDelay(hero int, d time.Duration) {
	s.mu.Lock()
	s.delay[hero] = d
	s.mu.Unlock()
}

func (s *originServer) hitsFor(hero int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits[hero]
}

// originClient construye un cliente offline sobre un directorio temporal
// apuntando al servidor de prueba.
func originClient(t *testing.T, dir, base string) *APIClient {
	t.Helper()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	return NewAPI(c, fix).WithAPIBase(base)
}

// copyFixturesCache deja una caché vacía en dir; el catálogo sale de los
// fixtures del repositorio.
func copyFixturesCache(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// TestFetchRunsProcedenciaPorHeroe: cada consulta devuelve SU fecha y su
// estado, aunque otra consulta se sirva en medio.
func TestFetchRunsProcedenciaPorHeroe(t *testing.T) {
	_, srv := newOriginServer(t)
	defer srv.Close()
	ResetRunsMemo()

	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)

	ctx := context.Background()
	if _, err := api.FetchRuns(ctx, 76, 0, 250); err != nil {
		t.Fatalf("hero 76: %v", err)
	}
	o76, ok := api.RunOriginFor(76)
	if !ok {
		t.Fatal("sin procedencia para 76")
	}
	if o76.FetchedAt.IsZero() {
		t.Error("76: fecha vacía")
	}

	// Segundo héroe después: su fecha debe ser distinta o igual, pero
	// nunca debe alterar la de 76.
	time.Sleep(2 * time.Millisecond)
	if _, err := api.FetchRuns(ctx, 12, 0, 250); err != nil {
		t.Fatalf("hero 12: %v", err)
	}
	o12, ok := api.RunOriginFor(12)
	if !ok {
		t.Fatal("sin procedencia para 12")
	}
	o76b, _ := api.RunOriginFor(76)
	if !o76b.FetchedAt.Equal(o76.FetchedAt) {
		t.Errorf("la procedencia de 76 cambió al consultar 12: %v → %v",
			o76.FetchedAt, o76b.FetchedAt)
	}
	if o12.FetchedAt.Before(o76.FetchedAt) {
		t.Errorf("12: fecha %v anterior a la de 76 (%v)", o12.FetchedAt, o76.FetchedAt)
	}
}

// TestFetchRunsConcurrenteClavesDistintas es el caso obligatorio: la
// consulta A obtiene su resultado, B se sirve antes de que A lea la
// procedencia, y A conserva la suya. La clave de memo de A no puede
// depender de B.
func TestFetchRunsConcurrenteClavesDistintas(t *testing.T) {
	st, srv := newOriginServer(t)
	defer srv.Close()
	ResetRunsMemo()

	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)

	// A es rápida; B lenta. Así B termina DESPUÉS de que A tenga su
	// resultado, que es el intercalado peligroso.
	st.setDelay(12, 60*time.Millisecond)

	var (
		wg   sync.WaitGroup
		oriA RunOrigin
		oriB RunOrigin
	)
	ctx := context.Background()

	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := api.FetchRuns(ctx, 76, 0, 250); err != nil {
			t.Errorf("A: %v", err)
		}
		oriA, _ = api.RunOriginFor(76)
	}()
	go func() {
		defer wg.Done()
		time.Sleep(5 * time.Millisecond) // deja que A entre primero
		if _, err := api.FetchRuns(ctx, 12, 0, 250); err != nil {
			t.Errorf("B: %v", err)
		}
		oriB, _ = api.RunOriginFor(12)
	}()
	wg.Wait()

	if oriA.FetchedAt.IsZero() || oriB.FetchedAt.IsZero() {
		t.Fatalf("fechas vacías: A=%v B=%v", oriA.FetchedAt, oriB.FetchedAt)
	}
	// Las claves de memo son distintas: héroe distinto ⇒ memorización
	// distinta, aunque compartan fecha.
	if memoKey(76, 0, 250, oriA.FetchedAt) == memoKey(12, 0, 250, oriB.FetchedAt) {
		t.Error("las claves de memo coinciden para héroes distintos")
	}
	// Y la procedencia de A sigue siendo la suya.
	again, _ := api.RunOriginFor(76)
	if !again.FetchedAt.Equal(oriA.FetchedAt) {
		t.Errorf("A cambió: %v → %v", oriA.FetchedAt, again.FetchedAt)
	}
}

// TestFetchRunsReutilizaMemoMismaFecha: dos consultas del MISMO héroe
// con la misma fecha reutilizan la entrada memorizada (una sola
// descarga), que es el comportamiento que se quiere conservar.
func TestFetchRunsReutilizaMemoMismaFecha(t *testing.T) {
	st, srv := newOriginServer(t)
	defer srv.Close()
	ResetRunsMemo()

	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)
	ctx := context.Background()

	var fechas []string
	for i := 0; i < 3; i++ {
		if _, err := api.FetchRuns(ctx, 76, 0, 250); err != nil {
			t.Fatalf("intento %d: %v", i, err)
		}
		o, _ := api.RunOriginFor(76)
		fechas = append(fechas, o.State+"@"+o.FetchedAt.Format(time.RFC3339Nano))
	}
	// La caché de disco sirve las siguientes consultas dentro del TTL:
	// el servidor se consulta UNA vez (miss) y las demás son hit con la
	// MISMA fecha de datos.
	if n := st.hitsFor(76); n != 1 {
		t.Errorf("peticiones al servidor = %d, se esperaba 1 (la 1ª es miss)", n)
	}
	if !strings.Contains(fechas[0], "miss@") {
		t.Errorf("la primera debía ser miss: %q", fechas[0])
	}
	for i, f := range fechas[1:] {
		if !strings.Contains(f, "hit@") {
			t.Errorf("la consulta %d debía ser hit: %q", i+1, f)
		}
		if f[strings.Index(f, "@"):] != fechas[0][strings.Index(fechas[0], "@"):] {
			t.Errorf("las fechas de datos difieren: %q vs %q", f, fechas[0])
		}
	}
	o, ok := api.RunOriginFor(76)
	if !ok || o.State != string(cache.StateHit) {
		t.Logf("estado final: %q (la primera fue miss)", o.State)
	}
}

// TestRunOriginClasificaEstados fija el contrato que consume la UI:
// solo un hit caducado o un respaldo local son "stale"; un hit fresco y
// una descarga nueva son lo normal.
func TestRunOriginClasificaEstados(t *testing.T) {
	casos := []struct {
		state string
		stale bool
	}{
		{string(cache.StateMiss), false}, // descarga nueva
		{string(cache.StateHit), false},  // caché fresca: normal
		{string(cache.StateStale), true}, // servida por error de fuente
		{string(cache.StateFixed), true}, // respaldo local
		{"", false},                      // desconocido: no se inventa
	}
	for _, c := range casos {
		o := RunOrigin{State: c.state}
		if got := o.Stale(); got != c.stale {
			t.Errorf("Stale() con estado %q = %v, se esperaba %v", c.state, got, c.stale)
		}
	}
}

// TestRunOriginEdadDesconocida: sin fecha no se inventa una antigüedad.
func TestRunOriginEdadDesconocida(t *testing.T) {
	o := RunOrigin{State: string(cache.StateStale)}
	if d, ok := o.Age(time.Now()); ok || d != 0 {
		t.Errorf("Age() sin fecha = %v, %v; se esperaba desconocida", d, ok)
	}
	o.FetchedAt = time.Now().Add(-48 * time.Hour)
	d, ok := o.Age(time.Now())
	if !ok {
		t.Fatal("Age() con fecha debe ser conocida")
	}
	if d < 47*time.Hour || d > 49*time.Hour {
		t.Errorf("Age() = %v, se esperaba ~48h", d)
	}
}

// errTransport falla toda petición, como una fuente caída.
type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("fuente caída (test)")
}
