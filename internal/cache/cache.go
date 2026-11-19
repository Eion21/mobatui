// Package cache implementa una caché de respuestas en disco con TTL y
// degradación tolerante (stale-if-error): si la fuente falla y existe
// una entrada caducada, se sirve la caducada antes que romper la app.
package cache

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State describe cómo se ha servido cada petición.
type State string

const (
	StateHit   State = "hit"   // entrada fresca en caché
	StateMiss  State = "miss"  // no había (o caducó) y se ha hecho fetch
	StateStale State = "stale" // fetch fallido, servida la caducada
	StateFixed State = "fixed" // sirviendo desde respaldo local (fixture)
)

// Result es la respuesta de Get.
//
// FetchedAt es la fecha de LOS DATOS de esta respuesta concreta: la
// entrada que se ha servido en un hit o stale, o el instante de la
// descarga en un miss. Es por petición, no global.
//
// Antes Result solo llevaba Body y State, así que quien necesitaba la
// fecha tenía que consultar Cache.Last() —estado compartido— y podía
// obtener la de otra consulta concurrente.
type Result struct {
	Body  []byte
	State State
	// FetchedAt es cuándo se obtuvieron estos datos. Zero si se
	// desconoce (no se inventa una fecha).
	FetchedAt time.Time
}

// entry es el envoltorio persistido en disco. El cuerpo vive en un
// archivo aparte comprimido con gzip (key.body.gz): los bytes van
// crudos, sin el +33% del base64 JSON, y el HTML de Lolalytics
// comprime muy bien.
type entry struct {
	URL       string    `json:"url"`
	FetchedAt time.Time `json:"fetched_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Cache es una caché de archivos JSON en un directorio.
type Cache struct {
	Dir string
	// Offline desactiva todo fetch: solo caché (fresca o caducada).
	Offline bool
	// now permite inyectar el reloj en tests.
	now func() time.Time

	// Último resultado servido: permite a la UI avisar de datos
	// caducados o de respaldo local en vez de servirlos en silencio.
	mu       sync.Mutex
	last     State
	lastAt   time.Time // cuándo se sirvió la última respuesta
	lastData time.Time // fecha de los datos de esa respuesta
}

func New(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("cache: crear %s: %w", dir, err)
	}
	return &Cache{Dir: dir, now: time.Now}, nil
}

func (c *Cache) timeNow() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// report apunta el estado del resultado servido, para que la UI pueda
// avisar de datos viejos. data es la fecha real de los datos.
func (c *Cache) report(s State, data time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.last, c.lastAt, c.lastData = s, c.timeNow(), data
}

// Last devuelve el estado del último resultado servido y la fecha de
// sus datos. El booleano es false si aún no se sirvió nada.
func (c *Cache) Last() (State, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastAt.IsZero() {
		return "", time.Time{}, false
	}
	return c.last, c.lastData, true
}

func (c *Cache) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(c.Dir, hex.EncodeToString(sum[:16])+".json")
}

func (c *Cache) load(key string) (*entry, error) {
	data, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil, err
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

// loadBody lee el cuerpo gzip de la entrada.
func (c *Cache) loadBody(key string) ([]byte, error) {
	f, err := os.Open(c.bodyPath(key))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

func (c *Cache) store(key string, e *entry, body []byte) error {
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp := c.path(key) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path(key)); err != nil {
		return err
	}
	// Cuerpo comprimido en archivo aparte: si el disco falla, la caché
	// sigue sirviendo; el fallo se registra en el propio body gz.
	bf, err := os.Create(c.bodyPath(key))
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(bf)
	if _, err := zw.Write(body); err != nil {
		bf.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		bf.Close()
		return err
	}
	return bf.Close()
}

// HasEntry indica si existe una entrada en disco para key, esté fresca o
// caducada. No hace fetch ni modifica el estado: sirve para decidir si
// un fallo de fuente puede resolverse con stale-if-error.
func (c *Cache) HasEntry(key string) bool {
	e, err := c.load(key)
	if err != nil || e == nil {
		return false
	}
	// La entrada solo es útil si su cuerpo se puede leer.
	if _, err := c.loadBody(key); err != nil {
		return false
	}
	return true
}

// GetValidated es Get con validación del cuerpo.
//
// El validador es POR LLAMADA, no global: un Cache sin validador se
// comporta exactamente como siempre. Así solo las entradas que declaran
// un formato (hoy los iconos PNG) pagan el coste y quedan protegidas.
//
// Valida en TODOS los caminos que devuelven un cuerpo, no solo al
// escribir:
//
//  1. hit fresco      (antes servía sin mirar el contenido)
//  2. respaldo stale  (antes servía sin mirar el contenido)
//  3. modo offline    (antes servía sin mirar el contenido)
//  4. cuerpo recién descargado, ANTES de store
//
// Si un cuerpo cacheado no supera el validador, la entrada se DESCARTA
// del disco y se intenta recargar. Si la recarga tampoco da un cuerpo
// válido, se devuelve error: NUNCA se sirve un cuerpo inválido.
//
// En modo offline no se descarta la entrada: sin red no se podría
// reponer y se perdería para siempre. Se devuelve error y la entrada
// queda para cuando vuelva la conexión.
func (c *Cache) GetValidated(key string, ttl time.Duration, fetch func() ([]byte, error), validate func([]byte) error) (Result, error) {
	if validate == nil {
		return c.Get(key, ttl, fetch)
	}
	e, err := c.load(key)

	// 1. Hit fresco: validar antes de servirlo.
	if err == nil && c.timeNow().Before(e.ExpiresAt) {
		if body, berr := c.loadBody(key); berr == nil {
			if verr := validate(body); verr == nil {
				c.report(StateHit, e.FetchedAt)
				return Result{Body: body, State: StateHit, FetchedAt: e.FetchedAt}, nil
			}
			// Entrada envenenada. En offline NO se descarta (sin red
			// no se podría reponer): se devuelve error conservándola.
			if c.Offline {
				return Result{}, fmt.Errorf("%w: %q (offline, entrada conservada)", ErrInvalidBody, key)
			}
			c.discard(key)
			e = nil
		}
	}

	// 3. Modo offline: no hay red para reponer, así que un cuerpo
	// inválido produce error pero NO se borra la entrada.
	if c.Offline {
		if e != nil {
			if body, berr := c.loadBody(key); berr == nil {
				if verr := validate(body); verr == nil {
					c.report(StateStale, e.FetchedAt)
					return Result{Body: body, State: StateStale, FetchedAt: e.FetchedAt}, nil
				}
				return Result{}, fmt.Errorf("%w: %q (offline, entrada conservada)", ErrInvalidBody, key)
			}
		}
		return Result{}, fmt.Errorf("cache: modo offline y sin entrada para %q", key)
	}

	body, ferr := fetch()
	if ferr != nil {
		// 2. Respaldo stale: validar también. Un cuerpo vencido e
		// inválido no es respaldo de nada.
		if e != nil {
			if stale, serr := c.loadBody(key); serr == nil {
				if verr := validate(stale); verr == nil {
					c.report(StateStale, e.FetchedAt)
					return Result{Body: stale, State: StateStale, FetchedAt: e.FetchedAt}, nil
				}
				c.discard(key)
				return Result{}, fmt.Errorf("%w: %q (respaldo caducado descartado): %w",
					ErrInvalidBody, key, ferr)
			}
		}
		return Result{}, fmt.Errorf("cache: fetch de %q fallido y sin caché: %w", key, ferr)
	}

	// 4. Cuerpo recién descargado: validar ANTES de store.
	if verr := validate(body); verr != nil {
		return Result{}, fmt.Errorf("%w: %q: %v", ErrInvalidBody, key, verr)
	}
	now := c.timeNow()
	if serr := c.store(key, &entry{
		URL:       key,
		FetchedAt: now,
		ExpiresAt: now.Add(ttl),
	}, body); serr != nil {
		_ = serr
	}
	c.report(StateMiss, now)
	return Result{Body: body, State: StateMiss, FetchedAt: now}, nil
}

// discard elimina del disco una entrada cuyo cuerpo no superó el
// validador, para que la siguiente lectura no vuelva a tropezar con
// ella. Los errores de borrado se ignoran: si no se puede borrar, la
// validación en lectura la rechazará otra vez.
//
// Es seguro ante concurrencia: no hay estado en memoria que corromper,
// solo dos os.Remove que son idempotentes entre sí.
func (c *Cache) discard(key string) {
	_ = os.Remove(c.path(key))
	_ = os.Remove(c.bodyPath(key))
}

// ErrInvalidBody marca el rechazo de un cuerpo por el validador de
// GetValidated. Quien lo consume puede distinguirlo de un fallo de red:
// es un fallo del CONTENIDO, no de la conexión, así que reintentarlo no
// lo arregla por sí solo.
var ErrInvalidBody = errors.New("cache: cuerpo inválido")

// Get devuelve el cuerpo asociado a key. Con entrada fresca no llama a
// fetch; sin entrada o caducada, hace fetch y persiste con ttl. Si el
// fetch falla y existe entrada caducada, la sirve como StateStale.
func (c *Cache) Get(key string, ttl time.Duration, fetch func() ([]byte, error)) (Result, error) {
	e, err := c.load(key)
	if err == nil && c.timeNow().Before(e.ExpiresAt) {
		if body, berr := c.loadBody(key); berr == nil {
			c.report(StateHit, e.FetchedAt)
			return Result{Body: body, State: StateHit, FetchedAt: e.FetchedAt}, nil
		}
	}
	if c.Offline {
		if e != nil {
			if body, err := c.loadBody(key); err == nil {
				c.report(StateStale, e.FetchedAt)
				return Result{Body: body, State: StateStale, FetchedAt: e.FetchedAt}, nil
			}
		}
		return Result{}, fmt.Errorf("cache: modo offline y sin entrada para %q", key)
	}
	body, err := fetch()
	if err != nil {
		if e != nil {
			if stale, serr := c.loadBody(key); serr == nil {
				c.report(StateStale, e.FetchedAt)
				return Result{Body: stale, State: StateStale, FetchedAt: e.FetchedAt}, nil
			}
		}
		return Result{}, fmt.Errorf("cache: fetch de %q fallido y sin caché: %w", key, err)
	}
	now := c.timeNow()
	if err := c.store(key, &entry{
		URL:       key,
		FetchedAt: now,
		ExpiresAt: now.Add(ttl),
	}, body); err != nil {
		// Falta de disco no rompe el dato: ya tenemos el cuerpo.
		_ = err
	}
	c.report(StateMiss, now)
	return Result{Body: body, State: StateMiss, FetchedAt: now}, nil
}

// bodyPath es el archivo del cuerpo gzip de la entrada.
func (c *Cache) bodyPath(key string) string {
	return c.path(key) + ".body.gz"
}

// Invalidate borra una entrada (la siguiente petición hará fetch).
func (c *Cache) Invalidate(key string) error {
	if err := os.Remove(c.path(key)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(c.bodyPath(key)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
