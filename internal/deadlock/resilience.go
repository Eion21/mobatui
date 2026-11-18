package deadlock

// Resiliencia de la API de muestras: reintento acotado para fallos
// transitorios y deduplicación de peticiones simultáneas.
//
// Principios:
//   · Un solo reintento adicional, y solo si hay presupuesto de latencia.
//   · Nunca se reintenta un 404 ni un 4xx permanente (son veredictos).
//   · Un 429 respeta Retry-After; no se reintenta a ciegas.
//   · Si ya hay una entrada de caché utilizable (aunque esté caducada),
//     NO se reintenta: stale-if-error ya resuelve el caso y pedir dos
//     veces no aporta nada.
//   · El reintento cubre solo la descarga HTTP, no el pipeline entero.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Valores ajustables desde pruebas. En producción son los indicados.
var (
	// retrySleep permite sustituir la espera en tests.
	retrySleep = func(ctx context.Context, d time.Duration) error {
		if d <= 0 {
			return ctx.Err()
		}
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-t.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	// retryJitter añade una fracción aleatoria acotada al backoff, para
	// que varios clientes no reintenten a la vez.
	retryJitter = func(d time.Duration) time.Duration {
		if d <= 0 {
			return 0
		}
		// Hasta un 25 % extra.
		return time.Duration(rand.Int63n(int64(d)/4 + 1))
	}
	// retryBaseline es el backoff mínimo antes del reintento.
	retryBaseline = 200 * time.Millisecond
	// retryBudget es el tiempo máximo que se acepta gastar en reintentos.
	retryBudget = 3 * time.Second
	// retryAfterCap acota un Retry-After desmedido del proveedor: si
	// pide más que esto, se renuncia al reintento y se sirve lo que haya.
	retryAfterCap = 5 * time.Second
)

// errTransient marca un fallo que merece un reintento.
var errTransient = errors.New("fallo transitorio")

// transientStatus indica si un código HTTP puede resolverse reintentando.
//
// 502/503/504 son fallos del proveedor. 429 es transitorio pero exige
// respetar Retry-After, así que se trata aparte.
func transientStatus(code int) bool {
	switch code {
	case http.StatusBadGateway, // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout,     // 504
		http.StatusTooManyRequests:    // 429
		return true
	}
	return false
}

// permanentStatus indica un veredicto: reintentar no cambia el resultado.
func permanentStatus(code int) bool {
	return code == http.StatusNotFound ||
		(code >= 400 && code < 500 && !transientStatus(code))
}

// httpError envuelve un código no OK conservando si es transitorio y,
// para 429, cuánto pide esperar.
type httpError struct {
	code       int
	path       string
	retryAfter time.Duration
	hasAfter   bool
}

func (e *httpError) Error() string {
	return "deadlock-api: " + e.path + " → HTTP " + strconv.Itoa(e.code)
}

// isTransient informa si el error merece reintento.
func isTransient(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return transientStatus(he.code)
	}
	// Errores de red (timeout, conexión cortada): transitorios, salvo
	// que el contexto ya esté cancelado.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return err != nil
}

// retryDelay calcula la espera antes del reintento.
//
// Para un 429 se usa Retry-After si viene; si pide más de lo que el
// presupuesto permite, se devuelve ok=false y no se reintenta.
func retryDelay(err error, attempt int) (time.Duration, bool) {
	var he *httpError
	if errors.As(err, &he) && he.hasAfter {
		if he.retryAfter > retryAfterCap {
			return 0, false
		}
		return he.retryAfter, true
	}
	d := retryBaseline * time.Duration(attempt+1)
	return d + retryJitter(d), true
}

// hasBudget comprueba que quede tiempo para un reintento dentro del
// contexto. Sin deadline se asume que sí.
func hasBudget(ctx context.Context, need time.Duration) bool {
	dl, ok := ctx.Deadline()
	if !ok {
		return true
	}
	return time.Until(dl) > need
}

// parseRetryAfter interpreta la cabecera Retry-After (segundos o fecha
// HTTP), con un tope para no aceptar esperas absurdas.
func parseRetryAfter(h http.Header) (time.Duration, bool) {
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// attemptRuns ejecuta UNA descarga y clasifica el resultado.
func (a *APIClient) attemptRuns(ctx context.Context, path string, hero int, shopSet map[int]bool, comps map[int][]int) ([]byte, error) {
	if err := waitMetaSlot(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.apiBase+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Sin partidas no es un error: es una ruta vacía.
		return []byte("null"), nil
	}
	if resp.StatusCode != http.StatusOK {
		he := &httpError{code: resp.StatusCode, path: path}
		if resp.StatusCode == http.StatusTooManyRequests {
			if d, ok := parseRetryAfter(resp.Header); ok {
				he.retryAfter, he.hasAfter = d, true
			}
			// Drenar un poco el cuerpo para permitir reutilizar la
			// conexión; no se usa el contenido.
			_, _ = io.CopyN(io.Discard, resp.Body, 512)
		}
		return nil, he
	}

	runs, err := decodeRuns(newBufReader(resp.Body), hero, shopSet, comps)
	if err != nil {
		return nil, err
	}
	return marshalRuns(runs)
}

// downloadRuns descarga la muestra aplicando la política de reintentos.
//
// staleExists indica que la caché ya tiene una entrada (aunque esté
// caducada): en ese caso NO se reintenta, porque stale-if-error ya
// cubre el fallo y una segunda petición no mejora nada.
func (a *APIClient) downloadRuns(ctx context.Context, path string, hero int, shopSet map[int]bool, comps map[int][]int, staleExists bool) ([]byte, error) {
	body, err := a.attemptRuns(ctx, path, hero, shopSet, comps)
	if err == nil || staleExists || !isTransient(err) {
		return body, err
	}

	delay, ok := retryDelay(err, 0)
	if !ok || !hasBudget(ctx, delay+retryBaseline) {
		return nil, err
	}
	if err := retrySleep(ctx, delay); err != nil {
		return nil, err
	}
	// Un único reintento adicional.
	return a.attemptRuns(ctx, path, hero, shopSet, comps)
}

// ─── deduplicación ────────────────────────────────────────────────

// runsCall es una descarga en curso, compartida por quien pida la
// misma clave mientras esté viva.
type runsCall struct {
	done   chan struct{}
	runs   []Run
	origin RunOrigin
	err    error
}

// runsInflight agrupa descargas simultáneas por clave de solicitud. La
// clave es la URL completa (héroe, filtros, límite): peticiones de
// héroes distintos usan claves distintas y no se bloquean entre sí.
var (
	runsInflightMu sync.Mutex
	runsInflight   = map[string]*runsCall{}
)

// numberInFlight es el número de descargas en curso. Solo para pruebas.
func numberInFlight() int {
	runsInflightMu.Lock()
	defer runsInflightMu.Unlock()
	return len(runsInflight)
}

// ResetInflight limpia el registro (pruebas).
func ResetInflight() {
	runsInflightMu.Lock()
	runsInflight = map[string]*runsCall{}
	runsInflightMu.Unlock()
}

// joinRuns ejecuta fn una sola vez por clave y comparte el resultado
// con las peticiones simultáneas.
//
// No usa un mutex global durante la red: se toma el lock solo para
// consultar o insertar la entrada, y quien espera lo hace en un canal,
// con su propio contexto.
func joinRuns(ctx context.Context, key string, fn func() ([]Run, RunOrigin, error)) ([]Run, RunOrigin, error) {
	runsInflightMu.Lock()
	if c, ok := runsInflight[key]; ok {
		runsInflightMu.Unlock()
		select {
		case <-c.done:
			return c.runs, c.origin, c.err
		case <-ctx.Done():
			return nil, RunOrigin{}, ctx.Err()
		}
	}
	c := &runsCall{done: make(chan struct{})}
	runsInflight[key] = c
	runsInflightMu.Unlock()

	defer func() {
		runsInflightMu.Lock()
		if runsInflight[key] == c {
			delete(runsInflight, key)
		}
		runsInflightMu.Unlock()
		// Cerrar tras borrar: quien esperaba despierta con el resultado.
		close(c.done)
	}()

	c.runs, c.origin, c.err = fn()
	// Publicar antes de cerrar el canal: el receptor lee los campos sin
	// carrera porque el cierre del canal establece la barrera.
	return c.runs, c.origin, c.err
}

// newBufReader envuelve el cuerpo con el buffer que usa el decodificador.
func newBufReader(r io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(r, 1<<20)
}

// marshalRuns serializa las runs para la caché.
func marshalRuns(runs []Run) ([]byte, error) {
	return json.Marshal(runs)
}
