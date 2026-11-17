// Package updatecheck avisa de una versión más reciente publicada en
// GitHub Releases. Nunca descarga ni instala nada: solo informa.
//
// Diseño:
//   - Reutiliza cache.Cache tal cual (TTL + stale-if-error + Offline),
//     sin refactorizar la caché ni tocar la de builds.
//   - Una sola comprobación por arranque; el llamador la lanza desde un
//     tea.Cmd asíncrono.
//   - Timeout corto y errores silenciosos: un fallo de red, 404, 403 o
//     429 nunca afecta al uso de la aplicación.
//   - Solo compara versiones estables con componentes numéricos.
package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
)

const (
	// DefaultEndpoint es la API de GitHub Releases.
	DefaultEndpoint = "https://api.github.com/repos/Eion21/mobatui/releases/latest"
	// DefaultTTL: la respuesta se reutiliza 24 h. Es también la barrera
	// contra el rate limit de GitHub (60 peticiones/hora sin token).
	DefaultTTL = 24 * time.Hour
	// DefaultTimeout acota la petición: el aviso es opcional, no puede
	// retrasar ni colgar nada.
	DefaultTimeout = 3 * time.Second
	// UserAgent con el que GitHub identifica la petición (obligatorio).
	UserAgent = "mobatui-updatecheck"
)

// ErrDisabled indica que la comprobación está desactivada (opt-out).
var ErrDisabled = errors.New("updatecheck: desactivada")

// Result es el desenlace de una comprobación.
type Result struct {
	// Latest es la versión remota normalizada ("0.2.1"), o "" si no hay.
	Latest string
	// URL es el enlace oficial al release, ya validado.
	URL string
	// Update indica que Latest > Current siendo ambas estables.
	Update bool
	// FromCache indica que se sirvió de la caché (vigente o caducada).
	FromCache bool
}

// Client consulta GitHub Releases con caché en disco.
type Client struct {
	// Cache es la caché de la aplicación. Obligatoria.
	Cache *cache.Cache
	// HTTP permite inyectar el cliente en pruebas. nil = uno con timeout.
	HTTP *http.Client
	// Endpoint sustituible en pruebas. "" = DefaultEndpoint.
	Endpoint string
	// Current es la versión local ya normalizada ("0.2.0" o "(devel)").
	Current string
	// TTL de la caché. 0 = DefaultTTL.
	TTL time.Duration
	// Timeout de la petición. 0 = DefaultTimeout.
	Timeout time.Duration
}

// Check consulta el último release y decide si hay actualización.
//
// Devuelve ErrDisabled si el cliente no está configurado o si la caché
// está en modo offline sin entrada: en ambos casos no hay petición HTTP
// y el llamador simplemente no muestra nada.
func (c *Client) Check(ctx context.Context) (Result, error) {
	if c == nil || c.Cache == nil {
		return Result{}, ErrDisabled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}

	// El timeout se aplica DENTRO del fetch: cache.Get es síncrono y el
	// llamador ya está en una goroutine de Bubble Tea.
	timeout := c.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	// En offline no se consulta nada. cache.Get lo respetaría igual,
	// pero se corta antes para no dejar rastro ni coste.
	if c.Cache.Offline {
		return Result{}, ErrDisabled
	}

	body, state, err := c.fetch(ctx, endpoint, ttl, timeout)
	if err != nil {
		// Cualquier fallo (red, timeout, 404, 403, 429, JSON corrupto)
		// deja la app intacta: sin aviso y sin ruido.
		return Result{}, err
	}

	latest, url, ok := parseRelease(body)
	if !ok {
		// Respuesta inesperada o release inválido: silencio.
		return Result{}, nil
	}
	res := Result{Latest: latest, URL: url, FromCache: state != cache.StateMiss}
	res.Update = IsNewer(latest, c.Current)
	return res, nil
}

// fetch pasa por cache.Get para heredar TTL, stale-if-error y offline.
func (c *Client) fetch(ctx context.Context, endpoint string, ttl, timeout time.Duration) ([]byte, cache.State, error) {
	res, err := c.Cache.Get(endpoint, ttl, func() ([]byte, error) {
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "application/vnd.github+json")

		client := c.HTTP
		if client == nil {
			client = &http.Client{Timeout: timeout}
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			// 403/429 son rate limit: se devuelve error para que la caché
			// NO guarde basura y para no reintentar en este arranque.
			return nil, fmt.Errorf("updatecheck: HTTP %d", resp.StatusCode)
		}
		return readLimited(resp)
	})
	if err != nil {
		return nil, "", err
	}
	return res.Body, res.State, nil
}

// readLimited lee el cuerpo con un tope: un release JSON es diminuto.
func readLimited(resp *http.Response) ([]byte, error) {
	const max = 1 << 20
	return io.ReadAll(io.LimitReader(resp.Body, max))
}

// release es el subconjunto que nos interesa de la API de GitHub.
type release struct {
	TagName    string `json:"tag_name"`
	HTMLURL    string `json:"html_url"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

// AllowedURLPrefix acota los enlaces que se pueden abrir.
const AllowedURLPrefix = "https://github.com/Eion21/mobatui/"

// parseRelease extrae versión y URL si el release es utilizable.
func parseRelease(body []byte) (version, url string, ok bool) {
	var r release
	if err := json.Unmarshal(body, &r); err != nil {
		return "", "", false
	}
	// Un draft o un prerelease no se anuncia como versión estable.
	if r.Draft || r.Prerelease {
		return "", "", false
	}
	v, ok := ParseVersion(r.TagName)
	if !ok {
		return "", "", false
	}
	return v.String(), SafeURL(r.HTMLURL), true
}

// SafeURL valida que el enlace pertenezca al repositorio esperado. Si no
// es así, devuelve "" y la UI mostrará el aviso sin acción de apertura.
func SafeURL(u string) string {
	if !strings.HasPrefix(u, AllowedURLPrefix) {
		return ""
	}
	// Evita prefijos engañosos tipo .../mobatui/../otro.
	if strings.Contains(u, "..") {
		return ""
	}
	return u
}

// ─── versiones ─────────────────────────────────────────────────────

// Version es una versión estable con componentes numéricos.
type Version struct {
	Major, Minor, Patch int
}

// String devuelve "X.Y.Z".
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// ParseVersion acepta "0.2.1" y "v0.2.1". Rechaza prereleases
// ("0.2.1-rc1"), metadatos ("+dirty"), "(devel)" y cualquier cosa que no
// sean tres componentes numéricos.
func ParseVersion(s string) (Version, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if s == "" || s == "(devel)" {
		return Version{}, false
	}
	// Prerelease o metadatos: no es una versión estable comparable.
	if strings.ContainsAny(s, "-+") {
		return Version{}, false
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	nums := make([]int, 3)
	for i, p := range parts {
		if p == "" {
			return Version{}, false
		}
		// Solo dígitos: descarta "1a", " 1" y negativos.
		for _, r := range p {
			if r < '0' || r > '9' {
				return Version{}, false
			}
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}, false
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, true
}

// Compare devuelve -1, 0 o 1 comparando por componentes numéricos.
// Compara números, no cadenas: 0.2.10 > 0.2.9.
func Compare(a, b Version) int {
	switch {
	case a.Major != b.Major:
		return sign(a.Major - b.Major)
	case a.Minor != b.Minor:
		return sign(a.Minor - b.Minor)
	default:
		return sign(a.Patch - b.Patch)
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

// IsNewer dice si latest es una versión estable superior a current.
// Un current no estable ("(devel)", vacío, prerelease) nunca produce
// aviso: no se presenta un build de desarrollo como versión publicada.
func IsNewer(latest, current string) bool {
	lv, ok := ParseVersion(latest)
	if !ok {
		return false
	}
	cv, ok := ParseVersion(current)
	if !ok {
		return false
	}
	return Compare(lv, cv) > 0
}
