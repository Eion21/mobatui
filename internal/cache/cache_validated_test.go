package cache

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ─── GetValidated frente a stale-if-error ──────────────────────────
//
// Get/GetValidated sirven una entrada CADUCADA cuando el fetch falla
// (stale-if-error). Ese camino NO pasa por el validador, así que una
// entrada vieja corrupta puede servirse como respaldo. Estas pruebas
// documentan el comportamiento exacto.

// relojControlable permite mover el tiempo sin dormir.
type reloj struct{ t time.Time }

func (r *reloj) now() time.Time { return r.t }

// TestStaleCorruptoNoSeSirve: el respaldo stale pasa por el validador.
// Un cuerpo vencido que no lo supera no es respaldo de nada: se descarta
// y la operación falla.
func TestStaleCorruptoNoSeSirve(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	clk := &reloj{t: time.Unix(1000, 0)}
	c.now = clk.now

	const clave = "https://cdn.invalido/icono.png"
	corrupto := []byte("cuerpo corrupto viejo")

	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) { return corrupto, nil }); err != nil {
		t.Fatal(err)
	}
	clk.t = clk.t.Add(2 * time.Hour) // caduca

	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return nil, errors.New("red caída") },
		func(b []byte) error {
			if bytes.Contains(b, []byte("corrupto")) {
				return errors.New("no es un PNG")
			}
			return nil
		})
	if err == nil {
		t.Fatalf("no debe servirse un respaldo inválido; sirvió %q", res.Body)
	}
	if !errors.Is(err, ErrInvalidBody) {
		t.Errorf("error = %v, se esperaba ErrInvalidBody", err)
	}
	if bytes.Contains(res.Body, []byte("corrupto")) {
		t.Errorf("sirvió el cuerpo corrupto: %q", res.Body)
	}
}

// TestOfflineCorruptoNoSeSirve: en offline tampoco se sirve un cuerpo
// inválido, pero la entrada se CONSERVA (sin red no se puede reponer).
func TestOfflineCorruptoNoSeSirve(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	const clave = "https://cdn.invalido/otro.png"
	corrupto := []byte("basura offline")
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) { return corrupto, nil }); err != nil {
		t.Fatal(err)
	}

	c.Offline = true
	// Fuera de la ventana de frescura para forzar la rama offline.
	base := c.timeNow()
	c.now = func() time.Time { return base.Add(2 * time.Hour) }
	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("offline no debe hacer fetch"); return nil, nil },
		func(b []byte) error { return errors.New("inválido") })
	if err == nil {
		t.Fatalf("offline debe rechazar el cuerpo inválido; sirvió %q", res.Body)
	}
	if !errors.Is(err, ErrInvalidBody) {
		t.Errorf("error = %v, se esperaba ErrInvalidBody", err)
	}
	if bytes.Contains(res.Body, []byte("basura")) {
		t.Errorf("sirvió el cuerpo: %q", res.Body)
	}
	// La entrada sigue en disco: se recuperará al reconectar.
	if _, statErr := os.Stat(c.path(clave)); statErr != nil {
		t.Errorf("la entrada debía conservarse en offline: %v", statErr)
	}
}

// TestValidadorSoloProtegeEscriturasNuevas: un cuerpo inválido recién
// descargado se rechaza y NO se guarda; pero una entrada previa válida
// caducada no se sustituye.
func TestValidadorSoloProtegeEscriturasNuevas(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	const clave = "https://cdn.invalido/nuevo.png"

	// Fetch devuelve corrupto: GetValidated debe rechazar sin guardar.
	_, err = c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return []byte("corrupto nuevo"), nil },
		func(b []byte) error { return errors.New("no válido") })
	if err == nil {
		t.Fatal("debía rechazar")
	}
	if !errors.Is(err, ErrInvalidBody) {
		t.Errorf("error = %v, se esperaba ErrInvalidBody", err)
	}
	// No debe haber entrada: un Get posterior tiene que pedir.
	pedido := false
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		pedido = true
		return []byte("ok"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if !pedido {
		t.Error("no debía haber entrada guardada")
	}
}

// TestGetSinValidadorNoCambia fija la compatibilidad de Get.
func TestGetSinValidadorNoCambia(t *testing.T) {
	c, err := New(filepath.Join(t.TempDir(), "c"))
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := []byte("cualquier cosa")
	if _, err := c.Get("k", time.Hour, func() ([]byte, error) { return cuerpo, nil }); err != nil {
		t.Fatal(err)
	}
	res, err := c.Get("k", time.Hour, func() ([]byte, error) {
		t.Fatal("no debía pedir")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.State != StateHit || !bytes.Equal(res.Body, cuerpo) {
		t.Errorf("estado=%v cuerpo=%q", res.State, res.Body)
	}
}

var _ = fmt.Sprintf

// TestInvalidateSoloBorraSuClave fija que -refresh (que invalida solo
// versions.json en main.go:93) NO limpia las entradas de iconos.
func TestInvalidateSoloBorraSuClave(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const versiones = "https://ddragon.leagueoflegends.com/api/versions.json"
	const icono = "https://ddragon.leagueoflegends.com/cdn/16.21.1/img/item/3026.png"

	for _, k := range []string{versiones, icono} {
		if _, err := c.Get(k, 720*time.Hour, func() ([]byte, error) {
			return []byte("cuerpo-" + k), nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Lo que hace -refresh: invalidar SOLO versions.json.
	if err := c.Invalidate(versiones); err != nil {
		t.Fatal(err)
	}

	// El icono debe seguir cacheado: -refresh no lo toca.
	pedido := false
	res, err := c.Get(icono, 720*time.Hour, func() ([]byte, error) {
		pedido = true
		return []byte("nuevo"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if pedido {
		t.Error("-refresh no debía invalidar el icono")
	}
	if res.State != StateHit {
		t.Errorf("estado del icono = %v, se esperaba StateHit", res.State)
	}
	t.Logf("CONFIRMADO: -refresh solo invalida %s; los iconos (ttlIcon) "+
		"NO se limpian, así que una entrada envenenada persiste 720 h", versiones)
}
