package ddragon

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Eion21/builds-tui/internal/cache"
)

// newClient construye un cliente con caché en tempdir y respaldo en los
// fixtures reales del repositorio.
func newClient(t *testing.T) *Client {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cache")
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return New(c, "../../testdata/ddragon", "es_ES")
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errSinRed
}

type netError struct{}

func (*netError) Error() string { return "sin red" }

var errSinRed = &netError{}

func TestFallbackAFixturesSinRed(t *testing.T) {
	cl := newClient(t)
	cl.http = &http.Client{Transport: failingTransport{}}

	// Sin red y sin caché: todo debe servirse desde el respaldo local.
	if v, err := cl.Version(); err != nil || v != "15.20.1" {
		t.Errorf("versión vía fixture = %q, %v; se esperaba 15.20.1", v, err)
	}
	if items, err := cl.Items(); err != nil || items[3020] != "Botas de hechicero" {
		t.Errorf("ítems vía fixture: %v, %v", items[3020], err)
	}
}

func TestChampionsYItemsYRunas(t *testing.T) {
	cl := newClient(t)
	champs, err := cl.Champions()
	if err != nil {
		t.Fatal(err)
	}
	ahri, ok := champs["Ahri"]
	if !ok || ahri.Key != "103" || ahri.Title == "" {
		t.Errorf("Ahri = %+v", ahri)
	}
	items, err := cl.Items()
	if err != nil {
		t.Fatal(err)
	}
	if items[3089] != "Sombrero mortal de Rabadon" {
		t.Errorf("ítem 3089 = %q", items[3089])
	}
	runes, trees, err := cl.Runes()
	if err != nil {
		t.Fatal(err)
	}
	if runes[8229] != "Cometa arcano" {
		t.Errorf("runa 8229 = %q", runes[8229])
	}
	if trees[8200] != "Brujería" {
		t.Errorf("árbol 8200 = %q", trees[8200])
	}
	if Shards[5008] == "" {
		t.Error("shards estático vacío")
	}
}

func TestCacheEvitaSegundoFetch(t *testing.T) {
	cl := newClient(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if filepath.Base(r.URL.Path) == "versions.json" {
			w.Write([]byte(`["15.20.1"]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	cl.baseURL = srv.URL
	cl.http = srv.Client()

	for i := 0; i < 3; i++ {
		if _, err := cl.Version(); err != nil {
			t.Fatal(err)
		}
	}
	if hits != 1 {
		t.Errorf("el servidor recibió %d peticiones; la caché debía limitarlo a 1", hits)
	}
}
