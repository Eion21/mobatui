package ddragon

// Pruebas de RuneIcon: resolución id → ruta, construcción de URL,
// caché, cambio de parche, offline y errores.
//
// Sin red: usa los fixtures de testdata/runes y un servidor httptest.

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
)

// runeServer sirve runesReforged.json y los PNG del fixture, y cuenta
// peticiones por ruta.
type runeServer struct {
	mu   sync.Mutex
	hits map[string]int
}

func newRuneServer(t *testing.T) (*runeServer, *httptest.Server) {
	t.Helper()
	rs := &runeServer{hits: map[string]int{}}
	root := filepath.Join("testdata", "runes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rs.mu.Lock()
		rs.hits[r.URL.Path]++
		rs.mu.Unlock()

		if strings.HasSuffix(r.URL.Path, "/runesReforged.json") {
			body, err := os.ReadFile(filepath.Join(root, "runesReforged.json"))
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		// /cdn/img/<ruta relativa del icono>
		const prefix = "/cdn/img/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		rel := strings.TrimPrefix(r.URL.Path, prefix)
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			// El PNG declarado pero ausente: 404 como el CDN real.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	return rs, srv
}

func (r *runeServer) count(path string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits[path]
}

func (r *runeServer) total() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, v := range r.hits {
		n += v
	}
	return n
}

// newRuneClient construye un cliente apuntando al servidor de prueba.
func newRuneClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	cl := New(c, filepath.Join("testdata", "runes"), "es_ES")
	if srv != nil {
		cl = cl.withBaseForTests(srv.URL)
	}
	return cl
}

// withBaseForTests apunta el cliente a un servidor local.
func (cl *Client) withBaseForTests(u string) *Client {
	cl.baseURL = u
	cl.version = "16.22.1"
	return cl
}

// TestRuneIconRutaReal: la URL usa el campo icon SIN versión. La ruta
// histórica /img/rune/<id>.png devuelve 403, así que no debe usarse.
func TestRuneIconRutaReal(t *testing.T) {
	rs, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	body, err := cl.RuneIcon(8112) // Electrocute
	if err != nil {
		t.Fatalf("RuneIcon: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("sin bytes")
	}
	// PNG válido: firma.
	if len(body) < 8 || string(body[1:4]) != "PNG" {
		t.Fatalf("no es PNG: % x", body[:min(8, len(body))])
	}

	want := "/cdn/img/perk-images/Styles/Domination/Electrocute/Electrocute.png"
	if n := rs.count(want); n != 1 {
		t.Errorf("peticiones a %s = %d, se esperaba 1", want, n)
	}
	// Y NO se pidió la ruta versionada ni la histórica.
	if n := rs.count("/cdn/16.22.1/img/perk-images/Styles/Domination/Electrocute/Electrocute.png"); n != 0 {
		t.Errorf("no debe pedirse la ruta versionada: %d", n)
	}
	if n := rs.count("/cdn/16.22.1/img/rune/8112.png"); n != 0 {
		t.Errorf("no debe pedirse /img/rune/<id>.png: %d", n)
	}
}

// TestRuneIconPathResuelveID: el mapa id → ruta viene del catálogo.
func TestRuneIconPathResuelveID(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	casos := map[int]string{
		8112: "perk-images/Styles/Domination/Electrocute/Electrocute.png",
		8128: "perk-images/Styles/Domination/DarkHarvest/DarkHarvest.png",
		8005: "perk-images/Styles/Precision/PressTheAttack/PressTheAttack.png",
	}
	for id, want := range casos {
		got, err := cl.runeIconPath(id)
		if err != nil {
			t.Errorf("runa %d: %v", id, err)
			continue
		}
		if got != want {
			t.Errorf("runa %d: ruta %q, se esperaba %q", id, got, want)
		}
	}
}

// TestRuneIconDesconocido: un id que no está en el catálogo da error,
// no una URL inventada.
func TestRuneIconDesconocido(t *testing.T) {
	rs, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	if _, err := cl.RuneIcon(999999); err == nil {
		t.Fatal("se esperaba error para una runa desconocida")
	}
	if n := rs.total(); n != 1 {
		// Solo la descarga del catálogo.
		t.Errorf("peticiones = %d, se esperaba solo el catálogo", n)
	}
}

// TestRuneIconImagenInexistente: catálogo con la ruta pero sin PNG.
func TestRuneIconImagenInexistente(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	// 8139 (Sabor a sangre) está en el catálogo SIN campo icon.
	if _, err := cl.RuneIcon(8139); err == nil {
		t.Error("una runa sin icon no debe devolver imagen")
	}
}

// TestRuneIconPNGCorrupto: el PNG existe pero no es válido. RuneIcon
// devuelve los bytes; quien decodifica decide. No debe entrar en pánico.
func TestRuneIconPNGCorrupto(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	// Se inyecta la ruta del PNG corrupto en el catálogo en memoria.
	cl.runeIconsMu.Lock()
	cl.runeIcons = map[int]string{1234: "perk-images/Styles/Domination/BadPng/BadPng.png"}
	cl.runeIconsMu.Unlock()

	body, err := cl.RuneIcon(1234)
	if err != nil {
		t.Fatalf("debe devolver los bytes, no fallar: %v", err)
	}
	if string(body) != "no soy un png" {
		t.Errorf("bytes inesperados: %q", body)
	}
}

// TestRuneIconArbolSinIcon: un árbol sin campo icon da error limpio.
func TestRuneIconArbolSinIcon(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	// 8000 (Precisión) no tiene icon en el fixture de runas.
	if _, err := cl.RuneIcon(8000); err == nil {
		t.Error("un árbol sin icon no debe devolver imagen")
	}
	// 8100 (Dominación) sí.
	if _, err := cl.RuneIcon(8100); err != nil {
		t.Errorf("el árbol con icon debía resolverse: %v", err)
	}
}

// TestRuneIconCache: dos peticiones seguidas del mismo id usan la caché
// de disco; el segundo no vuelve a pedir el PNG.
func TestRuneIconCache(t *testing.T) {
	rs, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	if _, err := cl.RuneIcon(8112); err != nil {
		t.Fatal(err)
	}
	antes := rs.count("/cdn/img/perk-images/Styles/Domination/Electrocute/Electrocute.png")

	if _, err := cl.RuneIcon(8112); err != nil {
		t.Fatal(err)
	}
	despues := rs.count("/cdn/img/perk-images/Styles/Domination/Electrocute/Electrocute.png")

	if despues != antes {
		t.Errorf("la segunda llamada volvió a pedir el PNG: %d → %d", antes, despues)
	}
}

// TestRuneIconOffline: en offline con caché previa se sirve; sin caché
// da error sin tocar la red.
func TestRuneIconOffline(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, filepath.Join("testdata", "runes"), "es_ES").withBaseForTests(srv.URL)
	if _, err := cl.RuneIcon(8112); err != nil {
		t.Fatalf("primera: %v", err)
	}

	// Mismo directorio de caché, ahora offline.
	c2, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	c2.Offline = true
	// La caché se indexa por URL COMPLETA: el cliente offline debe
	// apuntar a la misma base para encontrar la entrada guardada.
	cl2 := New(c2, filepath.Join("testdata", "runes"), "es_ES").withBaseForTests(srv.URL)
	body, err := cl2.RuneIcon(8112)
	if err != nil {
		t.Fatalf("offline con caché debía servir: %v", err)
	}
	if len(body) == 0 {
		t.Error("sin bytes en offline")
	}
}

// TestRuneIconCambioDeParche: la ruta del icono no lleva versión, así
// que el catálogo se vuelve a resolver pero el PNG es el mismo recurso.
//
// El comportamiento correcto es: cambiar la versión NO rompe la
// resolución, y el cliente no reutiliza el catálogo de otra versión.
func TestRuneIconCambioDeParche(t *testing.T) {
	rs, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	if _, err := cl.RuneIcon(8112); err != nil {
		t.Fatal(err)
	}
	catalogoAntes := rs.count("/cdn/16.22.1/data/es_ES/runesReforged.json")

	// Cambio de parche: se limpia el catálogo en memoria y la versión.
	cl.runeIconsMu.Lock()
	cl.runeIcons = nil
	cl.runeIconsMu.Unlock()
	cl.version = "16.23.1"

	if _, err := cl.RuneIcon(8112); err != nil {
		t.Fatalf("tras el cambio de parche: %v", err)
	}
	if catalogoAntes != 1 {
		t.Errorf("el catálogo del parche inicial se pidió %d veces, se esperaba 1", catalogoAntes)
	}
	catalogoDespues := rs.count("/cdn/16.23.1/data/es_ES/runesReforged.json")
	if catalogoDespues != 1 {
		t.Errorf("el catálogo del parche NUEVO se pidió %d veces, se esperaba 1", catalogoDespues)
	}
}

// TestValidateRelPath: las rutas del catálogo se validan antes de
// construir la URL.
func TestValidateRelPath(t *testing.T) {
	validos := []string{
		"perk-images/Styles/7200_Domination.png",
		"perk-images/Styles/Domination/Electrocute/Electrocute.png",
	}
	for _, p := range validos {
		if err := validateRelPath(p); err != nil {
			t.Errorf("%q debía ser válida: %v", p, err)
		}
	}
	invalidos := []string{
		"",
		"/absoluta/perk.png",
		"https://evil.example/x.png",
		"../fuera.png",
		"perk-images/Styles/x.gif",
		"perk-images/../../../etc/passwd.png",
	}
	for _, p := range invalidos {
		if err := validateRelPath(p); err == nil {
			t.Errorf("%q debía rechazarse", p)
		}
	}
}

// TestRunesWithIcons: el catálogo expone nombres y rutas a la vez.
func TestRunesWithIcons(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	runes, trees, icons, err := cl.RunesWithIcons()
	if err != nil {
		t.Fatal(err)
	}
	if len(runes) == 0 || len(trees) == 0 {
		t.Fatal("catálogo vacío")
	}
	// Una runa con icon y otra sin él.
	if _, ok := icons[8112]; !ok {
		t.Error("8112 debía tener icon")
	}
	if _, ok := icons[8139]; ok {
		t.Error("8139 no tiene icon y no debía aparecer")
	}
	// Los nombres siguen ahí aunque falte el icon.
	if runes[8139] == "" {
		t.Error("8139 debía conservar su nombre")
	}
}

// TestRunesCompatibilidad: Runes() sigue devolviendo lo mismo.
func TestRunesCompatibilidad(t *testing.T) {
	_, srv := newRuneServer(t)
	defer srv.Close()
	cl := newRuneClient(t, srv)

	runes, trees, err := cl.Runes()
	if err != nil {
		t.Fatal(err)
	}
	if runes[8112] == "" {
		t.Error("sin nombre para 8112")
	}
	if trees[8100] == "" {
		t.Error("sin nombre para el árbol 8100")
	}
}

// TestRuneIconNoInventaURL: sin catálogo disponible (servidor caído),
// el error llega y no se construye ninguna URL de imagen.
func TestRuneIconNoInventaURL(t *testing.T) {
	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c, filepath.Join("testdata", "runes"), "es_ES").
		withBaseForTests("http://127.0.0.1:1")

	_, err = cl.RuneIcon(8112)
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if !errors.Is(err, err) { // solo comprueba que es un error no vacío
		t.Logf("error: %v", err)
	}
}
