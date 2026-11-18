package ddragon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
)

// ─── identidad y caché de los iconos ───────────────────────────────
//
// Auditoría de v0.2.2: lo que se descarga es
//
//	{base}/cdn/{parche}/img/item/{id}.png
//
// y esa URL NO depende de las columnas ni del modo de render. Estas
// pruebas lo fijan y documentan cómo se comporta la caché ante un PNG
// ilegible.

// pngValido genera un PNG real de 64x64.
func pngValido(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{200, 40, 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// clienteDePrueba monta un Client con caché en t.TempDir() y servidor HTTP
// de prueba. No toca la caché real del usuario.
func clienteDePrueba(t *testing.T, mux *http.ServeMux) (*Client, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, t.TempDir(), "es_ES").WithHTTPClient(srv.Client())
	cl.baseURL = srv.URL
	_ = hits
	return cl, &hits
}

// TestItemIconURLNoDependeDeTamanos fija que la URL es la misma
// independientemente del ancho de icono: por eso la caché de fallos debe
// compartirse entre 3, 4 y 6 columnas.
func TestItemIconURLNoDependeDeTamanos(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`["16.21.1"]`))
	})
	cl, _ := clienteDePrueba(t, mux)

	u, err := cl.ItemIconURL(3026)
	if err != nil {
		t.Fatal(err)
	}
	// La URL solo depende de base, parche e id.
	if !bytes.Contains([]byte(u), []byte("/cdn/16.21.1/img/item/3026.png")) {
		t.Fatalf("URL inesperada: %q", u)
	}
	// La forma de la URL es exactamente base + /cdn/{parche}/img/item/{id}.png:
	// no hay ningún componente adicional donde pudieran entrar columnas o
	// modo. Se comprueba sobre los segmentos, no por subcadena (buscar
	// "6." daría falsos positivos con el parche "16.21.1").
	esperado := cl.baseURL + "/cdn/16.21.1/img/item/3026.png"
	if u != esperado {
		t.Errorf("URL = %q, se esperaba %q", u, esperado)
	}
	// La ruta (sin esquema ni host) es exactamente /cdn/{parche}/img/item/{id}.png.
	ruta := u[len(cl.baseURL):]
	if ruta != "/cdn/16.21.1/img/item/3026.png" {
		t.Errorf("ruta = %q; no debe tener componentes de representación", ruta)
	}
	// Y es estable entre llamadas.
	u2, _ := cl.ItemIconURL(3026)
	if u != u2 {
		t.Errorf("la URL no es estable: %q vs %q", u, u2)
	}
}

// TestCacheConservaPNGTruncado documenta el comportamiento REAL de la
// caché ante un cuerpo ilegible:
//
//   - Se descarga UNA sola vez (no reintenta la red).
//   - El cuerpo inválido queda guardado con TTL largo (ttlIcon).
//   - Un segundo intento devuelve LOS MISMOS bytes: la caché no valida
//     el contenido, así que no puede recuperarse solo por reintentar.
//
// La consecuencia para la UI está cubierta en internal/ui: artFailCorrupt
// no se memoriza, de modo que un cambio de parche o de caché sí permite
// volver a pedirlo, pero mientras la entrada siga vigente el icono no se
// recupera. Es un límite de cache.Get, no de la UI.
func TestCacheConservaPNGTruncado(t *testing.T) {
	truncado := []byte("\x89PNG\r\n\x1a\nbasura")
	valido := pngValido(t)
	var servirValido atomic.Bool
	var hits int32

	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`["16.21.1"]`))
	})
	mux.HandleFunc("/cdn/16.21.1/img/item/4321.png", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if servirValido.Load() {
			w.Write(valido)
			return
		}
		w.Write(truncado)
	})
	cl, _ := clienteDePrueba(t, mux)

	raw1, err := cl.ItemIcon(4321)
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := png.Decode(bytes.NewReader(raw1)); derr == nil {
		t.Fatal("el primer cuerpo debía ser ilegible")
	}

	servirValido.Store(true)
	raw2, err := cl.ItemIcon(4321)
	if err != nil {
		t.Fatal(err)
	}
	_, derr2 := png.Decode(bytes.NewReader(raw2))

	// Hecho 1: no se vuelve a descargar (la caché sirve el hit).
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("descargas = %d, se esperaba 1 (la caché no revalida)", got)
	}
	// Hecho 2: el cuerpo servido es el mismo, así que sigue ilegible.
	if derr2 == nil {
		t.Log("la caché sí devolvió el cuerpo nuevo: comportamiento distinto al esperado")
	}
	if len(raw2) != len(raw1) {
		t.Errorf("la caché devolvió %d bytes, se esperaban los %d guardados",
			len(raw2), len(raw1))
	}
}

// TestCostoDeUnRecursoCorrupto mide cuántas descargas y decodificaciones
// cuesta un recurso corrupto ante N intentos. La UI no memoriza
// artFailCorrupt, así que cada intento vuelve a decodificar; la red no se
// repite porque la caché sirve el hit.
func TestCostoDeUnRecursoCorrupto(t *testing.T) {
	var peticiones int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`["16.21.1"]`))
	})
	mux.HandleFunc("/cdn/16.21.1/img/item/4321.png", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&peticiones, 1)
		w.Write([]byte("\x89PNG\r\n\x1a\ntruncado"))
	})
	cl, _ := clienteDePrueba(t, mux)

	const N = 10
	var fallos int
	for i := 0; i < N; i++ {
		raw, err := cl.ItemIcon(4321)
		if err != nil {
			t.Fatalf("intento %d: %v", i, err)
		}
		if _, derr := png.Decode(bytes.NewReader(raw)); derr != nil {
			fallos++
		}
	}
	if got := atomic.LoadInt32(&peticiones); got != 1 {
		t.Errorf("peticiones HTTP = %d, se esperaba 1", got)
	}
	if fallos != N {
		t.Errorf("decodificaciones fallidas = %d, se esperaban %d", fallos, N)
	}
	t.Logf("N=%d intentos: red=%d, decodificaciones fallidas=%d "+
		"(la red no se repite; el coste es de CPU)",
		N, atomic.LoadInt32(&peticiones), fallos)
}
