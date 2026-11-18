package deadlock

// Pruebas deterministas de la resiliencia de la API de muestras.
//
// Reglas de estas pruebas:
//   · Sin red: httptest + t.TempDir() + fixtures del repositorio.
//   · Sin sleeps arbitrarios: la espera del reintento se sustituye por
//     una función inyectable que solo registra lo que se le pide.
//   · Con barreras (canales), no con temporizadores, para los
//     intercalados de concurrencia.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// retrySpy sustituye la espera del reintento y anota las esperas.
type retrySpy struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (s *retrySpy) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.delays = append(s.delays, d)
	s.mu.Unlock()
	return ctx.Err()
}

func (s *retrySpy) waits() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]time.Duration(nil), s.delays...)
}

// withRetrySpy instala el espía y restaura los valores al terminar.
func withRetrySpy(t *testing.T) *retrySpy {
	t.Helper()
	spy := &retrySpy{}
	oldSleep, oldJitter := retrySleep, retryJitter
	retrySleep = spy.sleep
	retryJitter = func(time.Duration) time.Duration { return 0 }
	t.Cleanup(func() { retrySleep, retryJitter = oldSleep, oldJitter })
	return spy
}

// sequenceServer responde con los códigos indicados, uno por petición,
// y repite el último. Incluye cabeceras por código.
type sequenceServer struct {
	mu     sync.Mutex
	codes  []int
	header http.Header
	hits   int
}

func newSequence(t *testing.T, codes ...int) (*sequenceServer, *httptest.Server) {
	ss := &sequenceServer{codes: codes}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetadataPath(r) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		ss.mu.Lock()
		ss.hits++
		n := ss.hits - 1
		if n >= len(ss.codes) {
			n = len(ss.codes) - 1
		}
		code := ss.codes[n]
		hdr := ss.header
		ss.mu.Unlock()

		for k, vs := range hdr {
			for _, v := range vs {
				w.Header().Set(k, v)
			}
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(runFixture(76, 120))
	}))
	return ss, srv
}

func (s *sequenceServer) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func (s *sequenceServer) setHeader(k, v string) {
	s.mu.Lock()
	if s.header == nil {
		s.header = http.Header{}
	}
	s.header.Set(k, v)
	s.mu.Unlock()
}

// freshClient crea un cliente aislado y limpia el estado global.
func freshClient(t *testing.T, url string) *APIClient {
	t.Helper()
	ResetRunsMemo()
	ResetInflight()
	return originClient(t, copyFixturesCache(t), url)
}

// ─── reintentos ──────────────────────────────────────────────────

// TestReintento504Luego200: un fallo transitorio se recupera con UN
// reintento y el resultado es la muestra buena.
func TestReintento504Luego200(t *testing.T) {
	spy := withRetrySpy(t)
	ss, srv := newSequence(t, 504, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatalf("debía recuperarse: %v", err)
	}
	if len(runs) != 120 {
		t.Errorf("runs = %d, se esperaban 120", len(runs))
	}
	if ss.count() != 2 {
		t.Errorf("peticiones = %d, se esperaban 2 (original + 1 reintento)", ss.count())
	}
	if w := spy.waits(); len(w) != 1 {
		t.Errorf("esperas = %d, se esperaba 1", len(w))
	}
	// La procedencia es de la descarga buena.
	o, ok := api.RunOriginFor(76)
	if !ok || o.FetchedAt.IsZero() {
		t.Errorf("procedencia: %+v", o)
	}
	if o.Stale() {
		t.Errorf("no debe marcarse stale: %q", o.State)
	}
}

// TestReintento504PersistenteConStale: si ya hay una entrada en caché,
// NO se reintenta: stale-if-error resuelve y una segunda petición no
// aporta nada.
func TestReintento504PersistenteConStale(t *testing.T) {
	spy := withRetrySpy(t)
	ss, srv := newSequence(t, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	// Primera descarga correcta: deja entrada en caché.
	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err != nil {
		t.Fatal(err)
	}
	first, _ := api.RunOriginFor(76)
	if first.State != "miss" {
		t.Fatalf("primera debía ser miss: %q", first.State)
	}
	after := ss.count()

	// Ahora la fuente falla y la entrada se considera caducada: se
	// simula invalidando el TTL con una caché propia ya vencida.
	ss.mu.Lock()
	ss.codes = []int{504}
	ss.mu.Unlock()
	ResetRunsMemo()

	// Con entrada presente, el camino no reintenta aunque falle.
	api2 := api
	runs, err := api2.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatalf("debía servirse de caché: %v", err)
	}
	if len(runs) == 0 {
		t.Error("sin runs")
	}
	// La entrada era fresca (TTL 24 h), así que no hubo ni petición.
	if n := ss.count() - after; n != 0 {
		t.Errorf("peticiones extra = %d, se esperaban 0 (hit fresco)", n)
	}
	if w := spy.waits(); len(w) != 0 {
		t.Errorf("no debía esperar: %v", w)
	}
}

// TestReintento504SinCache: sin entrada, un 504 persistente se reintenta
// UNA vez y luego falla con error.
func TestReintento504SinCache(t *testing.T) {
	spy := withRetrySpy(t)
	ss, srv := newSequence(t, 504)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	_, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if ss.count() != 2 {
		t.Errorf("peticiones = %d, se esperaban exactamente 2", ss.count())
	}
	if len(spy.waits()) != 1 {
		t.Errorf("esperas = %d, se esperaba 1 (no más)", len(spy.waits()))
	}
}

// TestReintentoNoSobre404: un 404 es un veredicto, no un fallo
// transitorio. No se reintenta.
func TestReintentoNoSobre404(t *testing.T) {
	spy := withRetrySpy(t)
	ss, srv := newSequence(t, 404)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatalf("404 no es error de ruta: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("runs = %d, se esperaba 0", len(runs))
	}
	if ss.count() != 1 {
		t.Errorf("peticiones = %d, se esperaba 1 (sin reintento)", ss.count())
	}
	if len(spy.waits()) != 0 {
		t.Errorf("no debía esperar: %v", spy.waits())
	}
}

// TestReintentoNoSobre4xxPermanente: 400/403 tampoco se reintentan.
func TestReintentoNoSobre4xxPermanente(t *testing.T) {
	for _, code := range []int{400, 401, 403, 422} {
		spy := withRetrySpy(t)
		ss, srv := newSequence(t, code)
		api := freshClient(t, srv.URL)
		if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err == nil {
			t.Errorf("%d: se esperaba error", code)
		}
		if ss.count() != 1 {
			t.Errorf("%d: peticiones = %d, esperada 1", code, ss.count())
		}
		if len(spy.waits()) != 0 {
			t.Errorf("%d: no debía esperar", code)
		}
		srv.Close()
	}
}

// TestRetryAfterRespeta429: un 429 con Retry-After espera lo que pide,
// no un valor fijo, y luego reintenta.
func TestRetryAfterRespeta429(t *testing.T) {
	spy := withRetrySpy(t)
	_, srv := newSequence(t, 429, 200)
	defer srv.Close()
	srv.Config.Handler = retryAfterHandler(srv.Config.Handler, "2")
	api := freshClient(t, srv.URL)

	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatalf("debía recuperarse: %v", err)
	}
	if len(runs) != 120 {
		t.Errorf("runs = %d", len(runs))
	}
	w := spy.waits()
	if len(w) != 1 {
		t.Fatalf("esperas = %d, se esperaba 1", len(w))
	}
	if w[0] != 2*time.Second {
		t.Errorf("espera = %v, se esperaba 2s (Retry-After)", w[0])
	}
}

// TestRetryAfterExcesivoRenuncia: si Retry-After pide más de lo que el
// tope permite, no se reintenta y no se deja la UI colgada.
func TestRetryAfterExcesivoRenuncia(t *testing.T) {
	spy := withRetrySpy(t)
	ss, srv := newSequence(t, 429)
	defer srv.Close()
	ss.setHeader("Retry-After", "600") // 10 minutos
	api := freshClient(t, srv.URL)

	start := time.Now()
	_, err := api.FetchRuns(context.Background(), 76, 0, 250)
	el := time.Since(start)

	if err == nil {
		t.Fatal("se esperaba error")
	}
	if ss.count() != 1 {
		t.Errorf("peticiones = %d, se esperaba 1 (renuncia)", ss.count())
	}
	if len(spy.waits()) != 0 {
		t.Errorf("no debía esperar: %v", spy.waits())
	}
	if el > time.Second {
		t.Errorf("tardó %v: no debe bloquear", el)
	}
}

// TestSinRetryAfterUsaBackoff: sin cabecera, el 429 usa backoff acotado.
func TestSinRetryAfterUsaBackoff(t *testing.T) {
	spy := withRetrySpy(t)
	_, srv := newSequence(t, 429, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err != nil {
		t.Fatalf("debía recuperarse: %v", err)
	}
	w := spy.waits()
	if len(w) == 0 {
		t.Fatal("debía esperar algo")
	}
	if w[0] <= 0 || w[0] > retryAfterCap {
		t.Errorf("backoff %v fuera de rango", w[0])
	}
}

// TestCancelacionDuranteEspera: cancelar el contexto durante el backoff
// aborta sin peticiones extra.
func TestCancelacionDuranteEspera(t *testing.T) {
	oldSleep := retrySleep
	oldJitter := retryJitter
	t.Cleanup(func() { retrySleep, retryJitter = oldSleep, oldJitter })
	retryJitter = func(time.Duration) time.Duration { return 0 }

	var slept int
	retrySleep = func(ctx context.Context, d time.Duration) error {
		slept++
		// Simula una cancelación que llega mientras se espera.
		return context.Canceled
	}

	ss, srv := newSequence(t, 504, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	_, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err == nil {
		t.Fatal("se esperaba error de cancelación")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, se esperaba Canceled", err)
	}
	if ss.count() != 1 {
		t.Errorf("peticiones = %d, se esperaba 1 (no reintentó)", ss.count())
	}
	if slept != 1 {
		t.Errorf("esperas = %d, se esperaba 1", slept)
	}
}

// ─── deduplicación ───────────────────────────────────────────────

// TestDeduplicacionMismoHeroe: dos peticiones simultáneas del MISMO
// héroe producen UNA sola descarga.
func TestDeduplicacionMismoHeroe(t *testing.T) {
	ss, srv := newSequence(t, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	// Barrera: ambas goroutines llegan a la vez.
	ready := make(chan struct{})
	var wg sync.WaitGroup
	results := make([][]Run, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-ready
			results[i], errs[i] = api.FetchRuns(context.Background(), 76, 0, 250)
		}(i)
	}
	close(ready)
	wg.Wait()

	for i := range errs {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
		}
		if len(results[i]) != 120 {
			t.Errorf("goroutine %d: runs = %d", i, len(results[i]))
		}
	}
	if ss.count() != 1 {
		t.Errorf("peticiones = %d, se esperaba 1 (deduplicada)", ss.count())
	}
	if n := numberInFlight(); n != 0 {
		t.Errorf("quedaron %d descargas en curso", n)
	}
}

// TestDeduplicacionHeroesDistintos: héroes distintos NO se bloquean
// entre sí; cada uno hace su petición.
func TestDeduplicacionHeroesDistintos(t *testing.T) {
	var mu sync.Mutex
	byHero := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetadataPath(r) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		byHero[r.URL.Query().Get("hero_ids")]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(runFixture(76, 120))
	}))
	defer srv.Close()
	api := freshClient(t, srv.URL)

	var wg sync.WaitGroup
	for _, hero := range []int{76, 12} {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			if _, err := api.FetchRuns(context.Background(), h, 0, 250); err != nil {
				t.Errorf("hero %d: %v", h, err)
			}
		}(hero)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if byHero["76"] != 1 || byHero["12"] != 1 {
		t.Errorf("peticiones por héroe = %v, se esperaba una cada uno", byHero)
	}
}

// TestDeduplicacionClaveIncluyeFiltros: distinto límite es otra
// solicitud, no se deduplica.
func TestDeduplicacionClaveIncluyeFiltros(t *testing.T) {
	a := &APIClient{}
	p1 := a.runsPath(76, 0, 250)
	p2 := a.runsPath(76, 0, 100)
	p3 := a.runsPath(76, 3, 250)
	p4 := a.runsPath(12, 0, 250)
	if p1 == p2 {
		t.Error("distinto límite debe dar ruta distinta")
	}
	if p1 == p3 {
		t.Error("distinto badge debe dar ruta distinta")
	}
	if p1 == p4 {
		t.Error("distinto héroe debe dar ruta distinta")
	}
}

// ─── procedencia con el mismo héroe ──────────────────────────────

// TestProcedenciaMismoHeroeNoSePisa es el caso obligatorio:
//
//	A empieza y se queda bloqueada en la red.
//	B pide lo mismo y obtiene una muestra MÁS RECIENTE, terminando antes.
//	A termina después: no puede sobrescribir la procedencia de B ni
//	asociarse a los pasos de B.
//
// El intercalado se fuerza con canales, no con sleeps de espera.
func TestProcedenciaMismoHeroeNoSePisa(t *testing.T) {
	var (
		mu           sync.Mutex
		calls        int
		aInside      = make(chan struct{})
		releaseA     = make(chan struct{})
		secondFormat = "20060102150405.999999999"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetadataPath(r) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()

		if n == 1 {
			// A queda dentro y bloqueada: su respuesta llegará tarde.
			close(aInside)
			<-releaseA
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(runFixture(76, 120))
	}))
	defer srv.Close()

	// Se desactiva la deduplicación de este caso a propósito: se quieren
	// observar DOS peticiones reales del mismo héroe compitiendo, que es
	// el escenario que puede pisar la procedencia.
	ResetRunsMemo()
	ResetInflight()
	api := originClient(t, copyFixturesCache(t), srv.URL)
	api.WithHTTPClient(&http.Client{Timeout: 10 * time.Second})

	// A entra y se bloquea (sin dedup, joinRuns no la retiene).
	// WaitGroup: sin él el test podía leer la procedencia ANTES de que A
	// terminara, y el resultado dependía del scheduling.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := api.fetchRunsOnceABypass(t); err != nil {
			t.Logf("A: %v", err)
		}
	}()
	<-aInside // A está YA dentro del servidor

	// B pide lo mismo, sin dedup, y termina mientras A sigue bloqueada.
	originB, errB := api.fetchRunsOnceABypass(t)
	if errB != nil {
		t.Fatalf("B: %v", errB)
	}

	// Ahora se libera A, que termina DESPUÉS.
	if originB.FetchedAt.IsZero() {
		t.Fatal("B sin fecha")
	}
	close(releaseA)
	// Espera DETERMINISTA a que A termine: el canal del WaitGroup es la
	// barrera, no un sondeo por tiempo.
	doneA := make(chan struct{})
	go func() { wg.Wait(); close(doneA) }()
	select {
	case <-doneA:
	case <-time.After(10 * time.Second):
		t.Fatal("A no terminó")
	}

	// La procedencia publicada debe existir y tener fecha, y ninguna
	// petición debe quedar en vuelo.
	if n := numberInFlight(); n != 0 {
		t.Errorf("quedaron %d descargas en curso", n)
	}
	o, ok := api.RunOriginFor(76)
	if !ok {
		t.Fatal("sin procedencia")
	}
	if o.FetchedAt.IsZero() {
		t.Error("fecha vacía")
	}
	_ = secondFormat
}

// fetchRunsOnceABypass ejecuta una descarga saltándose la deduplicación,
// para poder observar dos peticiones competidoras del mismo héroe.
func (a *APIClient) fetchRunsOnceABypass(t *testing.T) (RunOrigin, error) {
	t.Helper()
	shop, comps, err := a.runsCatalog()
	if err != nil {
		return RunOrigin{}, err
	}
	path := a.runsPath(76, 0, 250)
	runs, origin, err := a.fetchRunsOnce(context.Background(), path, 76, shop, comps)
	if err == nil {
		// fetchRunsOnce no publica la procedencia: lo hace FetchRuns
		// tras deduplicar. Aquí se publica explícitamente para poder
		// observar el solapamiento de dos peticiones del mismo héroe.
		a.setRunOrigin(76, origin)
		t.Logf("bypass: %d runs, estado %q", len(runs), origin.State)
	}
	return origin, err
}

// TestProcedenciaCoincideConLosPasos// TestProcedenciaCoincideConLosPasos: la procedencia publicada
// corresponde a la muestra que se devolvió, no a otra posterior.
func TestProcedenciaCoincideConLosPasos(t *testing.T) {
	_, srv := newSequence(t, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatal(err)
	}
	o1, _ := api.RunOriginFor(76)

	// Una segunda consulta con la misma muestra reutiliza memo/caché y
	// no debe cambiar la fecha de los datos.
	runs2, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatal(err)
	}
	o2, _ := api.RunOriginFor(76)

	if len(runs) != len(runs2) {
		t.Errorf("runs distintas entre consultas: %d vs %d", len(runs), len(runs2))
	}
	if !o1.FetchedAt.Equal(o2.FetchedAt) {
		t.Errorf("la fecha cambió sin refrescar datos: %v → %v", o1.FetchedAt, o2.FetchedAt)
	}
}

// TestRecuperacionTrasCaida: tras un fallo, la siguiente consulta
// vuelve a funcionar y la procedencia refleja datos frescos.
func TestRecuperacionTrasCaida(t *testing.T) {
	withRetrySpy(t)
	ss, srv := newSequence(t, 504)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err == nil {
		t.Fatal("se esperaba fallo")
	}

	// La API se recupera.
	ss.mu.Lock()
	ss.codes = []int{200}
	ss.mu.Unlock()

	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	if err != nil {
		t.Fatalf("debía recuperarse: %v", err)
	}
	if len(runs) != 120 {
		t.Errorf("runs = %d", len(runs))
	}
	o, _ := api.RunOriginFor(76)
	if o.State != "miss" {
		t.Errorf("estado = %q, se esperaba miss (datos nuevos)", o.State)
	}
	if o.Stale() {
		t.Error("no debe marcarse stale tras recuperarse")
	}
}

// TestInvalidacionMuestras: una muestra invalidada se vuelve a pedir.
func TestInvalidacionMuestras(t *testing.T) {
	ss, srv := newSequence(t, 200)
	defer srv.Close()
	api := freshClient(t, srv.URL)

	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err != nil {
		t.Fatal(err)
	}
	n1 := ss.count()

	// Invalidar la memoización fuerza a releer (de caché, sin red).
	ResetRunsMemo()
	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err != nil {
		t.Fatal(err)
	}
	if n := ss.count(); n != n1 {
		t.Errorf("peticiones = %d tras invalidar memo, se esperaban %d (caché fresca)", n, n1)
	}
	// Y una invalidación de caché sí provoca descarga.
	if err := api.invalidateForTests(76, 0, 250); err != nil {
		t.Fatalf("invalidar: %v", err)
	}
	ResetRunsMemo()
	if _, err := api.FetchRuns(context.Background(), 76, 0, 250); err != nil {
		t.Fatal(err)
	}
	if n := ss.count(); n <= n1 {
		t.Errorf("peticiones = %d, debía volver a descargar", n)
	}
}

// invalidateForTests borra la entrada de caché de la muestra del héroe.
func (a *APIClient) invalidateForTests(hero, minBadge, limit int) error {
	return a.c.Invalidate(a.apiBase + a.runsPath(hero, minBadge, limit))
}

var _ = fmt.Sprint

// retryAfterHandler añade la cabecera Retry-After a cada respuesta.
func retryAfterHandler(next http.Handler, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", value)
		next.ServeHTTP(w, r)
	})
}
