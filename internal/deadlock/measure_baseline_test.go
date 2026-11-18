package deadlock

// Medición del comportamiento actual de FetchRuns ante fallos HTTP.
//
// Usa httptest y un cliente offline sobre t.TempDir(): sin red, sin
// caché personal y sin tocar la API pública.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// httpScript es un servidor que responde según un guion de códigos.
// Cada petición consume el siguiente código; cuando se agotan, repite
// el último.
type httpScript struct {
	mu     sync.Mutex
	codes  []int
	header http.Header
	hits   int
	delay  time.Duration
}

func newScript(t *testing.T, codes ...int) (*httpScript, *httptest.Server) {
	sc := &httpScript{codes: codes}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMetadataPath(r) {
			// El catálogo de ítems sale de los fixtures locales; este
			// servidor solo atiende la muestra.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sc.mu.Lock()
		sc.hits++
		n := sc.hits - 1
		if n >= len(sc.codes) {
			n = len(sc.codes) - 1
		}
		code := sc.codes[n]
		hdr := sc.header
		d := sc.delay
		sc.mu.Unlock()

		if d > 0 {
			select {
			case <-time.After(d):
			case <-r.Context().Done():
				return
			}
		}
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
	return sc, srv
}

func isMetadataPath(r *http.Request) bool {
	return len(r.URL.Path) > 0 && contains(r.URL.Path, "matches/metadata")
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func (s *httpScript) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.hits
}

func (s *httpScript) setHeader(k, v string) {
	s.mu.Lock()
	if s.header == nil {
		s.header = http.Header{}
	}
	s.header.Set(k, v)
	s.mu.Unlock()
}

// measure corre un escenario y devuelve tiempo, peticiones y resultado.
type outcome struct {
	elapsed  time.Duration
	requests int
	runs     int
	err      error
	state    string
}

func measure(t *testing.T, codes ...int) outcome {
	t.Helper()
	sc, srv := newScript(t, codes...)
	defer srv.Close()
	ResetRunsMemo()
	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)

	start := time.Now()
	runs, err := api.FetchRuns(context.Background(), 76, 0, 250)
	o := outcome{elapsed: time.Since(start), requests: sc.count(), runs: len(runs), err: err}
	if or, ok := api.RunOriginFor(76); ok {
		o.state = or.State
	}
	return o
}

// TestMedirComportamientoActual documenta (no valida) el comportamiento
// ante cada código HTTP. Sirve de línea base para el rediseño.
func TestMedirComportamientoActual(t *testing.T) {
	casos := []struct {
		name  string
		codes []int
	}{
		{"200", []int{200}},
		{"502", []int{502}},
		{"503", []int{503}},
		{"504", []int{504}},
		{"429", []int{429}},
		{"404", []int{404}},
		{"504 luego 200", []int{504, 200}},
	}
	for _, c := range casos {
		o := measure(t, c.codes...)
		t.Logf("BASE %-14s tiempo=%-10v peticiones=%d runs=%-4d estado=%-6q err=%v",
			c.name, o.elapsed.Round(time.Millisecond), o.requests, o.runs, o.state, shortErr(o.err))
	}
}

func shortErr(err error) string {
	if err == nil {
		return "-"
	}
	s := err.Error()
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}

// TestMedirDuplicacionLoadRunsYHybrid reproduce lo que hace la UI al
// abrir un héroe: lanza Ruta y build híbrida a la vez. Mide cuántas
// peticiones HTTP se hacen para la MISMA muestra.
func TestMedirDuplicacionLoadRunsYHybrid(t *testing.T) {
	sc, srv := newScript(t, 200)
	defer srv.Close()
	ResetRunsMemo()
	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)

	// Simula los dos comandos concurrentes de la UI (tea.Batch).
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = api.FetchRuns(context.Background(), 76, 0, 250) // LoadRuns
	}()
	go func() {
		defer wg.Done()
		_, _ = api.FetchRuns(context.Background(), 76, 0, 250) // LoadHybrid
	}()
	wg.Wait()

	t.Logf("DUPLICACION peticiones HTTP para la misma muestra = %d (1 esperada)", sc.count())
	if sc.count() > 1 {
		t.Logf("DUPLICACION CONFIRMADA: %d peticiones por apertura de héroe", sc.count())
	}
}

// TestMedirHuecoPorRateLimit documenta el coste de waitMetaSlot: 4 req/s
// significa 250 ms entre peticiones, que se suma a cada fallo que se
// reintente.
func TestMedirHuecoPorRateLimit(t *testing.T) {
	sc, srv := newScript(t, 200)
	defer srv.Close()
	ResetRunsMemo()
	dir := copyFixturesCache(t)
	api := originClient(t, dir, srv.URL)

	// Tres héroes distintos: cada uno exige una ranura.
	start := time.Now()
	var wg sync.WaitGroup
	for _, hero := range []int{76, 12, 35} {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			_, _ = api.FetchRuns(context.Background(), h, 0, 250)
		}(hero)
	}
	wg.Wait()
	el := time.Since(start)
	t.Logf("RATELIMIT 3 héroes en %v (hueco teórico 250 ms → ≥500 ms)", el.Round(time.Millisecond))
	_ = sc
	_ = fmt.Sprint()
}
