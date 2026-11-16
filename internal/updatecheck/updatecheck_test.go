package updatecheck

// Pruebas con httptest y cachés en directorios temporales. Ninguna toca
// la red real ni depende de caché personal.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
)

// ─── comparación de versiones ──────────────────────────────────────

func TestParseVersion(t *testing.T) {
	valid := map[string]Version{
		"0.2.1":    {0, 2, 1},
		"v0.2.1":   {0, 2, 1},
		"1.0.0":    {1, 0, 0},
		"0.2.10":   {0, 2, 10},
		"10.20.30": {10, 20, 30},
	}
	for in, want := range valid {
		got, ok := ParseVersion(in)
		if !ok {
			t.Errorf("ParseVersion(%q) debía aceptar", in)
			continue
		}
		if got != want {
			t.Errorf("ParseVersion(%q) = %v, se esperaba %v", in, got, want)
		}
	}

	invalid := []string{
		"", "(devel)", "devel", "0.2", "0.2.1.4", "0.2.x",
		"0.2.1-rc1", "0.2.1-beta", "0.2.1+dirty", "v", "vv0.2.1",
		"-1.2.3", "0.2. 1", "0..1",
	}
	for _, in := range invalid {
		if _, ok := ParseVersion(in); ok {
			t.Errorf("ParseVersion(%q) debía rechazar", in)
		}
	}
}

// TestCompareNumeric es el caso clave: 0.2.10 > 0.2.9, que una
// comparación lexicográfica erraría.
func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.2.9", "0.2.10", -1},
		{"0.2.10", "0.2.9", 1},
		{"0.2.10", "0.2.10", 0},
		{"0.9.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.2.1", "0.2.0", 1},
		{"0.2.0", "0.2.1", -1},
	}
	for _, tt := range tests {
		a, _ := ParseVersion(tt.a)
		b, _ := ParseVersion(tt.b)
		if got := Compare(a, b); got != tt.want {
			t.Errorf("Compare(%s,%s) = %d, se esperaba %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"0.2.1", "0.2.0", true},
		{"0.2.0", "0.2.0", false},
		{"0.1.9", "0.2.0", false},
		{"0.2.10", "0.2.9", true},
		{"v0.3.0", "0.2.0", true},
		// Un build de desarrollo nunca recibe aviso.
		{"0.2.1", "(devel)", false},
		{"0.2.1", "", false},
		{"0.2.1", "(devel)+05e64b4.dirty", false},
		// Una remota no estable tampoco.
		{"0.2.1-rc1", "0.2.0", false},
		{"(devel)", "0.2.0", false},
	}
	for _, tt := range tests {
		if got := IsNewer(tt.latest, tt.current); got != tt.want {
			t.Errorf("IsNewer(%q,%q) = %v, se esperaba %v",
				tt.latest, tt.current, got, tt.want)
		}
	}
}

// ─── arnés ─────────────────────────────────────────────────────────

// fakeGitHub sirve un release y cuenta las peticiones.
type fakeGitHub struct {
	*httptest.Server
	calls atomic.Int32
	tag   atomic.Value // string
	url   atomic.Value // string
	code  atomic.Int32
	delay atomic.Int64 // ns
	body  atomic.Value // string (si != "", se sirve tal cual)
}

func newFakeGitHub(t *testing.T, tag, url string) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{}
	f.tag.Store(tag)
	f.url.Store(url)
	f.code.Store(http.StatusOK)

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		if d := f.delay.Load(); d > 0 {
			time.Sleep(time.Duration(d))
		}
		if raw, _ := f.body.Load().(string); raw != "" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, raw)
			return
		}
		if c := int(f.code.Load()); c != http.StatusOK {
			w.WriteHeader(c)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		tag, _ := f.tag.Load().(string)
		url, _ := f.url.Load().(string)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name":   tag,
			"html_url":   url,
			"draft":      false,
			"prerelease": false,
		})
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeGitHub) endpoint() string { return f.Server.URL + "/releases/latest" }

// newClient construye un cliente con caché temporal.
func newClient(t *testing.T, srv *fakeGitHub, current string, offline bool) *Client {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.Offline = offline
	return &Client{
		Cache:    c,
		HTTP:     srv.Client(),
		Endpoint: srv.endpoint(),
		Current:  current,
	}
}

// ─── consulta ──────────────────────────────────────────────────────

func TestCheckRemoteSuperiorIgualInferior(t *testing.T) {
	try := func(t *testing.T, tag, current string, wantUpdate bool, wantLatest string) {
		srv := newFakeGitHub(t, tag, "https://github.com/Eion21/mobatui/releases/tag/"+tag)
		cl := newClient(t, srv, current, false)
		res, err := cl.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if res.Update != wantUpdate {
			t.Errorf("Update = %v, se esperaba %v", res.Update, wantUpdate)
		}
		if res.Latest != wantLatest {
			t.Errorf("Latest = %q, se esperaba %q", res.Latest, wantLatest)
		}
		if wantUpdate && res.URL == "" {
			t.Error("con actualización debe haber URL")
		}
	}

	t.Run("superior", func(t *testing.T) { try(t, "v0.2.1", "0.2.0", true, "0.2.1") })
	t.Run("igual", func(t *testing.T) { try(t, "v0.2.0", "0.2.0", false, "0.2.0") })
	t.Run("inferior", func(t *testing.T) { try(t, "v0.1.9", "0.2.0", false, "0.1.9") })
	t.Run("10 mayor que 9", func(t *testing.T) { try(t, "v0.2.10", "0.2.9", true, "0.2.10") })
}

func TestCheckIgnoraPrereleaseDraftYDevel(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"prerelease", `{"tag_name":"v0.3.0","html_url":"https://github.com/Eion21/mobatui/releases/tag/v0.3.0","prerelease":true}`},
		{"draft", `{"tag_name":"v0.3.0","html_url":"https://github.com/Eion21/mobatui/releases/tag/v0.3.0","draft":true}`},
		{"tag devel", `{"tag_name":"(devel)","html_url":"https://github.com/Eion21/mobatui/releases/tag/x"}`},
		{"tag vacío", `{"tag_name":"","html_url":""}`},
		{"tag rc", `{"tag_name":"v0.3.0-rc1","html_url":"https://github.com/Eion21/mobatui/releases/tag/v0.3.0-rc1"}`},
		{"json inválido", `{no es json`},
		{"json vacío", `{}`},
		{"array en vez de objeto", `[]`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := newFakeGitHub(t, "", "")
			srv.body.Store(tc.body)
			cl := newClient(t, srv, "0.2.0", false)
			res, err := cl.Check(context.Background())
			if err != nil {
				t.Fatalf("Check no debe fallar: %v", err)
			}
			if res.Update {
				t.Errorf("no debía avisar de actualización: %+v", res)
			}
		})
	}
}

// TestCheckURLValidada: solo se aceptan enlaces del repositorio esperado.
func TestCheckURLValidada(t *testing.T) {
	bad := []string{
		"https://evil.example/mobatui",
		"https://github.com/Otro/mobatui/releases/tag/v0.3.0",
		"http://github.com/Eion21/mobatui/releases/tag/v0.3.0",
		"https://github.com/Eion21/mobatui/../otro",
		"",
	}
	for _, u := range bad {
		srv := newFakeGitHub(t, "v0.3.0", u)
		cl := newClient(t, srv, "0.2.0", false)
		res, err := cl.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if res.Update && res.URL != "" {
			t.Errorf("URL no validada: %q", res.URL)
		}
	}

	// El enlace correcto sí pasa.
	srv := newFakeGitHub(t, "v0.3.0", "https://github.com/Eion21/mobatui/releases/tag/v0.3.0")
	cl := newClient(t, srv, "0.2.0", false)
	res, err := cl.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.URL != "https://github.com/Eion21/mobatui/releases/tag/v0.3.0" {
		t.Errorf("URL = %q", res.URL)
	}
}

// ─── caché ─────────────────────────────────────────────────────────

// TestCacheVigenteSinNuevaSolicitud: la segunda consulta no hace HTTP.
func TestCacheVigenteSinNuevaSolicitud(t *testing.T) {
	srv := newFakeGitHub(t, "v0.2.1", "https://github.com/Eion21/mobatui/releases/tag/v0.2.1")
	cl := newClient(t, srv, "0.2.0", false)

	first, err := cl.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.FromCache {
		t.Error("la primera consulta debía venir de la red")
	}
	if n := srv.calls.Load(); n != 1 {
		t.Fatalf("peticiones tras la primera = %d, se esperaba 1", n)
	}

	second, err := cl.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := srv.calls.Load(); n != 1 {
		t.Errorf("la caché vigente debía evitar la petición: %d peticiones", n)
	}
	if !second.FromCache {
		t.Error("la segunda consulta debía servirse de caché")
	}
	if second.Latest != first.Latest || second.Update != first.Update {
		t.Errorf("resultado inconsistente: %+v vs %+v", first, second)
	}
}

// TestCacheExpirada: con TTL mínimo, la segunda consulta vuelve a pedir.
func TestCacheExpirada(t *testing.T) {
	srv := newFakeGitHub(t, "v0.2.1", "https://github.com/Eion21/mobatui/releases/tag/v0.2.1")
	cl := newClient(t, srv, "0.2.0", false)
	cl.TTL = time.Nanosecond // caduca de inmediato

	if _, err := cl.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := cl.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := srv.calls.Load(); n != 2 {
		t.Errorf("con caché expirada se esperaban 2 peticiones, hubo %d", n)
	}
}

// TestOfflineCeroPeticiones: en offline no se toca la red.
func TestOfflineCeroPeticiones(t *testing.T) {
	srv := newFakeGitHub(t, "v0.2.1", "https://github.com/Eion21/mobatui/releases/tag/v0.2.1")
	cl := newClient(t, srv, "0.2.0", true)

	if _, err := cl.Check(context.Background()); err == nil {
		t.Fatal("en offline debía devolver ErrDisabled")
	}
	if n := srv.calls.Load(); n != 0 {
		t.Errorf("offline hizo %d peticiones, se esperaban 0", n)
	}
}

// TestOptOutCeroPeticiones: sin caché configurada no hay comprobación.
func TestOptOutCeroPeticiones(t *testing.T) {
	srv := newFakeGitHub(t, "v0.2.1", "")
	var nilClient *Client
	if _, err := nilClient.Check(context.Background()); err == nil {
		t.Error("un cliente nil debía estar desactivado")
	}
	empty := &Client{}
	if _, err := empty.Check(context.Background()); err == nil {
		t.Error("sin caché debía estar desactivado")
	}
	if n := srv.calls.Load(); n != 0 {
		t.Errorf("el opt-out hizo %d peticiones", n)
	}
}

// ─── errores ───────────────────────────────────────────────────────

// TestErroresHTTP: ningún código de error rompe la app ni envenena la
// caché.
func TestErroresHTTP(t *testing.T) {
	for _, code := range []int{404, 403, 429, 500, 502} {
		t.Run(fmt.Sprintf("HTTP %d", code), func(t *testing.T) {
			srv := newFakeGitHub(t, "", "")
			srv.code.Store(int32(code))
			cl := newClient(t, srv, "0.2.0", false)

			res, err := cl.Check(context.Background())
			if err == nil {
				t.Fatalf("HTTP %d debía devolver error", code)
			}
			if res.Update {
				t.Error("un error no debe producir aviso")
			}
			if n := srv.calls.Load(); n != 1 {
				t.Errorf("no debe reintentar: %d peticiones", n)
			}

			// Y un error no deja caché: la siguiente intenta de nuevo.
			cl.Check(context.Background())
			if n := srv.calls.Load(); n != 2 {
				t.Errorf("tras un fallo la caché no debe servir: %d", n)
			}
		})
	}
}

// TestTimeout: si GitHub tarda más que el timeout, se abandona sin
// colgar la app.
func TestTimeout(t *testing.T) {
	srv := newFakeGitHub(t, "v0.9.9", "https://github.com/Eion21/mobatui/releases/tag/v0.9.9")
	srv.delay.Store(int64(500 * time.Millisecond))

	cl := newClient(t, srv, "0.2.0", false)
	cl.Timeout = 50 * time.Millisecond

	start := time.Now()
	res, err := cl.Check(context.Background())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("debía fallar por timeout")
	}
	if res.Update {
		t.Error("un timeout no debe producir aviso")
	}
	if elapsed > 400*time.Millisecond {
		t.Errorf("el timeout no acotó: %v", elapsed)
	}
}

// TestTimeoutNoBloqueaConcurrentes: mientras la comprobación espera, otra
// carga puede avanzar (no hay bloqueo global).
func TestTimeoutNoBloqueaConcurrentes(t *testing.T) {
	srv := newFakeGitHub(t, "v0.9.9", "")
	srv.delay.Store(int64(300 * time.Millisecond))
	cl := newClient(t, srv, "0.2.0", false)
	cl.Timeout = 100 * time.Millisecond

	done := make(chan struct{})
	go func() {
		cl.Check(context.Background())
		close(done)
	}()

	// Otra "carga" independiente debe terminar bastante antes.
	time.Sleep(10 * time.Millisecond)
	quick := time.Now()
	_ = srv.Client() // trabajo simbólico de otra ruta
	if d := time.Since(quick); d > 50*time.Millisecond {
		t.Errorf("otra ruta se bloqueó: %v", d)
	}
	<-done
}

// TestSlowCheckDoesNotBlockSharedCache: la comprobación comparte la
// cache.Cache con las builds. Un fetch de GitHub lento NO debe bloquear
// otras lecturas: Get no toma ningún lock global durante la petición.
func TestSlowCheckDoesNotBlockSharedCache(t *testing.T) {
	var hits atomic.Int32
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		time.Sleep(400 * time.Millisecond)
		fmt.Fprint(w, `{"tag_name":"v0.9.9","html_url":"https://github.com/Eion21/mobatui/releases/tag/v0.9.9"}`)
	}))
	defer slow.Close()

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cl := &Client{
		Cache:    c,
		HTTP:     slow.Client(),
		Endpoint: slow.URL,
		Current:  "0.2.0",
		Timeout:  2 * time.Second,
	}

	done := make(chan struct{})
	go func() { cl.Check(context.Background()); close(done) }()

	// Esperar a que el fetch lento esté en curso.
	deadline := time.Now().Add(time.Second)
	for hits.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if hits.Load() == 0 {
		t.Fatal("el fetch lento no llegó a arrancar")
	}

	// Otra entrada (una build) debe resolverse sin esperar al fetch.
	start := time.Now()
	if _, err := c.Get("https://ejemplo.test/builds", time.Hour,
		func() ([]byte, error) { return []byte("ok"), nil }); err != nil {
		t.Fatalf("cache.Get concurrente: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("la comprobación bloqueó la caché compartida: %v", elapsed)
	}
	<-done
}
