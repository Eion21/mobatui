package ddragon

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
)

// conIHDRCache es igual que conIHDR pero local a este archivo.
func conIHDRCache(base []byte, mut func(d []byte)) []byte {
	out := append([]byte(nil), base...)
	const off = 8
	d := out[off+8 : off+8+13]
	mut(d)
	binary.BigEndian.PutUint32(out[off+8+13:off+8+17], crc32.ChecksumIEEE(out[off+4:off+4+4+13]))
	return out
}

// TestAutocuracionConCacheReal: entrada de disco con IHDR ilegal y CRC
// correcto, servidor devolviendo el PNG real ⇒ la lectura descarta la
// entrada y devuelve el icono correcto.
func TestAutocuracionConCacheReal(t *testing.T) {
	real, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		t.Skip("sin fixture")
	}
	envenenado := conIHDRCache(real, func(d []byte) { d[8] = 3 }) // bitDepth 3

	var peticiones int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/versions.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`["16.21.1"]`))
	})
	mux.HandleFunc("/cdn/16.21.1/img/item/3026.png", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&peticiones, 1)
		w.Write(real)
	})

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cl := New(c, t.TempDir(), "es_ES").WithHTTPClient(srv.Client())
	cl.baseURL = srv.URL

	// Se siembra la entrada envenenada saltándose el validador, como la
	// dejaría una versión anterior.
	url, err := cl.ItemIconURL(3026)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(url, 720*60*60*1e9, func() ([]byte, error) { return envenenado, nil }); err != nil {
		t.Fatal(err)
	}

	// Lectura real: debe descartar la entrada y devolver el icono bueno.
	body, err := cl.ItemIcon(3026)
	if err != nil {
		t.Fatalf("la lectura debía recuperarse, no fallar: %v", err)
	}
	if bytes.Equal(body, envenenado) {
		t.Fatal("devolvió la entrada envenenada")
	}
	if !bytes.Equal(body, real) {
		t.Fatalf("devolvió %d bytes, se esperaban los %d reales", len(body), len(real))
	}
	if got := atomic.LoadInt32(&peticiones); got != 1 {
		t.Errorf("peticiones = %d, se esperaba 1 (descarte + recarga)", got)
	}
	// Y en disco ya está el bueno.
	body2, err := cl.ItemIcon(3026)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body2, real) {
		t.Error("la entrada en disco no quedó curada")
	}
	if got := atomic.LoadInt32(&peticiones); got != 1 {
		t.Errorf("la segunda lectura no debía pedir: peticiones=%d", got)
	}
}
