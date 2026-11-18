package cache

import (
	"bytes"
	"errors"
	"fmt"
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

// TestStaleNoPasaPorElValidador demuestra que el respaldo stale se
// sirve sin validar.
func TestStaleNoPasaPorElValidador(t *testing.T) {
	dir := t.TempDir()
	c, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	clk := &reloj{t: time.Unix(1000, 0)}
	c.now = clk.now

	const clave = "https://cdn.invalido/icono.png"
	corrupto := []byte("cuerpo corrupto viejo")

	// 1. Se guarda un cuerpo corrupto SIN validador (como haría una
	//    versión anterior, o un Get normal).
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) { return corrupto, nil }); err != nil {
		t.Fatal(err)
	}

	// 2. Se adelanta el reloj para que caduque.
	clk.t = clk.t.Add(2 * time.Hour)

	// 3. El fetch falla: debe servirse la entrada caducada.
	validadorLlamado := 0
	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return nil, errors.New("red caída") },
		func(b []byte) error {
			validadorLlamado++
			if bytes.Contains(b, []byte("corrupto")) {
				return errors.New("no es un PNG")
			}
			return nil
		})
	if err != nil {
		t.Fatalf("debía servir la entrada caducada: %v", err)
	}
	if res.State != StateStale {
		t.Errorf("estado = %v, se esperaba StateStale", res.State)
	}
	if !bytes.Equal(res.Body, corrupto) {
		t.Errorf("cuerpo servido = %q", res.Body)
	}
	// HALLAZGO: el validador NO se aplica al respaldo stale.
	if validadorLlamado != 0 {
		t.Errorf("el validador se llamó %d veces en el camino stale; se esperaba 0", validadorLlamado)
	}
	t.Logf("CONFIRMADO: el respaldo stale se sirve SIN validar "+
		"(cuerpo=%q, estado=%v, validador=%d llamadas)", res.Body, res.State, validadorLlamado)
}

// TestOfflineSirveCorrupto: en modo offline también se sirve la entrada
// sin validar.
func TestOfflineSirveCorrupto(t *testing.T) {
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
	llamado := 0
	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("offline no debe hacer fetch"); return nil, nil },
		func(b []byte) error { llamado++; return errors.New("inválido") })
	if err != nil {
		t.Fatalf("offline debía servir la entrada: %v", err)
	}
	if llamado != 0 {
		t.Errorf("el validador se llamó %d veces en offline", llamado)
	}
	if !bytes.Equal(res.Body, corrupto) {
		t.Errorf("cuerpo = %q", res.Body)
	}
	t.Logf("CONFIRMADO: en offline se sirve el cuerpo sin validar (%q)", res.Body)
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
