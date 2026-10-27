// Package cache implementa una caché de respuestas en disco con TTL y
// degradación tolerante (stale-if-error): si la fuente falla y existe
// una entrada caducada, se sirve la caducada antes que romper la app.
package cache

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
type Result struct {
	Body  []byte
	State State
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

// Get devuelve el cuerpo asociado a key. Con entrada fresca no llama a
// fetch; sin entrada o caducada, hace fetch y persiste con ttl. Si el
// fetch falla y existe entrada caducada, la sirve como StateStale.
func (c *Cache) Get(key string, ttl time.Duration, fetch func() ([]byte, error)) (Result, error) {
	e, err := c.load(key)
	if err == nil && c.timeNow().Before(e.ExpiresAt) {
		if body, berr := c.loadBody(key); berr == nil {
			c.report(StateHit, e.FetchedAt)
			return Result{Body: body, State: StateHit}, nil
		}
	}
	if c.Offline {
		if e != nil {
			if body, err := c.loadBody(key); err == nil {
				c.report(StateStale, e.FetchedAt)
				return Result{Body: body, State: StateStale}, nil
			}
		}
		return Result{}, fmt.Errorf("cache: modo offline y sin entrada para %q", key)
	}
	body, err := fetch()
	if err != nil {
		if e != nil {
			if stale, serr := c.loadBody(key); serr == nil {
				c.report(StateStale, e.FetchedAt)
				return Result{Body: stale, State: StateStale}, nil
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
	return Result{Body: body, State: StateMiss}, nil
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
