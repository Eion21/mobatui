package ddragon

import (
	"bytes"
	"compress/gzip"
	"errors"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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

// TestTruncadoNoEnvenenaLaCache es la prueba de regresión del defecto:
// un PNG truncado servido con HTTP 200 NO debe quedar en la caché, y un
// segundo intento explícito debe recuperar el icono cuando el origen ya
// sirve uno válido.
//
// Sin la validación en escritura esto falla: la caché se queda con los
// bytes truncados y el segundo intento devuelve los mismos bytes.
func TestTruncadoNoEnvenenaLaCache(t *testing.T) {
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

	// Primer intento: cuerpo truncado con 200.
	if _, err := cl.ItemIcon(4321); err == nil {
		t.Fatal("el primer intento debía rechazar el cuerpo truncado")
	}

	// El origen ya sirve un PNG válido.
	servirValido.Store(true)

	// Segundo intento explícito: DEBE recuperar el icono.
	raw2, err := cl.ItemIcon(4321)
	if err != nil {
		t.Fatalf("el segundo intento debía recuperar el icono, no fallar: %v", err)
	}
	if _, derr := png.Decode(bytes.NewReader(raw2)); derr != nil {
		t.Fatalf("el segundo intento devolvió un cuerpo ilegible: %v", derr)
	}
	// Se descargó dos veces: la primera no se guardó, así que no hubo hit.
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Errorf("descargas = %d, se esperaban 2 (la primera no se cachea)", got)
	}
}

// TestTruncadoNoSeEscribeEnDisco inspecciona el directorio de la caché:
// tras un cuerpo inválido no debe quedar ninguna entrada para esa URL.
func TestTruncadoNoSeEscribeEnDisco(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`["16.21.1"]`))
	})
	mux.HandleFunc("/cdn/16.21.1/img/item/5555.png", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("\x89PNG\r\n\x1a\ntruncado"))
	})

	dir := t.TempDir()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, t.TempDir(), "es_ES").WithHTTPClient(srv.Client())
	cl.baseURL = srv.URL

	if _, err := cl.ItemIcon(5555); err == nil {
		t.Fatal("debía rechazar el cuerpo truncado")
	}

	// Ningún .body.gz debe corresponder al icono: se inspecciona el
	// contenido de cada cuerpo guardado buscando la firma del truncado.
	entradas, err := filepath.Glob(filepath.Join(dir, "*.json.body.gz"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range entradas {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		zr, err := gzip.NewReader(fh)
		if err != nil {
			fh.Close()
			continue
		}
		buf := make([]byte, 64)
		n, _ := zr.Read(buf)
		zr.Close()
		fh.Close()
		if bytes.Contains(buf[:n], []byte("truncado")) {
			t.Errorf("el cuerpo truncado quedó escrito en %s", filepath.Base(f))
		}
	}
}

// TestCacheSinValidadorGuardaCualquierCuerpo fija la compatibilidad: el
// Cache normal (Get) no valida nada, así que un cuerpo cualquiera se
// guarda y se sirve. El validador es por entrada, no global.
func TestCacheSinValidadorGuardaCualquierCuerpo(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	const clave = "https://ejemplo.invalido/cualquiera.bin"
	cuerpo := []byte("esto no es un PNG ni nada parecido")
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) { return cuerpo, nil }); err != nil {
		t.Fatal(err)
	}
	res, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		t.Fatal("no debía volver a pedir: hay entrada fresca")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.Body, cuerpo) {
		t.Errorf("cuerpo servido = %q, se esperaba el guardado", res.Body)
	}
	if res.State != cache.StateHit {
		t.Errorf("estado = %v, se esperaba StateHit", res.State)
	}
}

// TestGetValidatedRechazaSinGuardar comprueba el contrato de
// GetValidated: rechaza, no guarda y el error es ErrInvalidBody.
func TestGetValidatedRechazaSinGuardar(t *testing.T) {
	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	const clave = "https://ejemplo.invalido/icono.png"
	_, err = c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return []byte("malo"), nil },
		func(b []byte) error {
			if string(b) != "bueno" {
				return errors.New("no válido")
			}
			return nil
		})
	if err == nil {
		t.Fatal("debía rechazar el cuerpo")
	}
	if !errors.Is(err, cache.ErrInvalidBody) {
		t.Errorf("el error debe ser ErrInvalidBody, es: %v", err)
	}
	// Y no debe haber entrada: el siguiente Get tiene que pedir de nuevo.
	pedido := false
	res, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		pedido = true
		return []byte("bueno"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !pedido {
		t.Error("no debía haber entrada guardada tras el rechazo")
	}
	if string(res.Body) != "bueno" {
		t.Errorf("cuerpo = %q", res.Body)
	}
}

// TestCostoDeUnRecursoCorrupto mide el EFECTO SECUNDARIO del validador:
// con un origen que sirve SIEMPRE un cuerpo corrupto, ya no hay hit de
// caché (la entrada no se guarda), así que cada intento vuelve a pedir a
// la red. Se mide para dejarlo documentado; no hay backoff ni límite.
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
		if _, err := cl.ItemIcon(4321); err != nil {
			fallos++
		}
	}
	if fallos != N {
		t.Errorf("fallos = %d, se esperaban %d", fallos, N)
	}
	t.Logf("N=%d intentos con origen persistentemente corrupto: peticiones HTTP=%d, "+
		"fallos=%d. Sin límite: cada intento reintenta la red porque no hay "+
		"entrada en caché. No se implementa backoff (fuera de alcance).",
		N, atomic.LoadInt32(&peticiones), fallos)
}

// TestValidadorDetectaTruncadoAMitad es la prueba que distingue un
// validador correcto de uno basado en png.DecodeConfig.
//
// DecodeConfig solo lee la cabecera IHDR: se midió que un PNG real de
// 64x64 truncado a la mitad devuelve err=nil (lo aceptaría). El caso de
// 14 bytes sí lo detecta por casualidad, así que hace falta este.
func TestValidadorDetectaTruncadoAMitad(t *testing.T) {
	raw := pngValido(t)
	if err := ValidatePNG(raw); err != nil {
		t.Fatalf("un PNG completo debe validar: %v", err)
	}
	for _, n := range []int{14, len(raw) / 2, len(raw) - 1} {
		if err := ValidatePNG(raw[:n]); err == nil {
			t.Errorf("un PNG truncado a %d de %d bytes debe rechazarse", n, len(raw))
		}
	}
	// Cuerpo corrupto con CRC que no cuadra.
	corrupto := append([]byte(nil), raw...)
	corrupto[len(corrupto)/2] ^= 0xFF
	if err := ValidatePNG(corrupto); err == nil {
		t.Error("un PNG con un byte alterado debe rechazarse")
	}
	// Sin firma.
	if err := ValidatePNG([]byte("no soy un png")); err == nil {
		t.Error("un cuerpo sin firma PNG debe rechazarse")
	}
}

// BenchmarkValidatePNG mide el validador de iconos sobre un PNG real de
// 64x64. Justifica la elección frente a png.Decode completo.
func BenchmarkValidatePNG(b *testing.B) {
	raw := pngValido(&testing.T{})
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidatePNG(raw); err != nil {
			b.Fatal(err)
		}
	}
}
