package cache

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return base }
	return c
}

func TestHitMissYStale(t *testing.T) {
	c := newTestCache(t)
	calls := 0
	fetch := func() ([]byte, error) {
		calls++
		return []byte("datos"), nil
	}

	// Miss: primera llamada hace fetch y persiste.
	r, err := c.Get("k", time.Hour, fetch)
	if err != nil || r.State != StateMiss || string(r.Body) != "datos" {
		t.Fatalf("primera get: %+v, %v", r, err)
	}

	// Hit: fresca, sin fetch.
	r, err = c.Get("k", time.Hour, fetch)
	if err != nil || r.State != StateHit || calls != 1 {
		t.Fatalf("segunda get debía ser hit sin fetch: %+v calls=%d", r, calls)
	}

	// Stale: fetch que falla con entrada caducada se sirve igual.
	c.now = func() time.Time { return time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC) }
	r, err = c.Get("k", time.Hour, func() ([]byte, error) { return nil, errors.New("red caída") })
	if err != nil || r.State != StateStale || string(r.Body) != "datos" {
		t.Fatalf("caducada sin red debía servir stale: %+v, %v", r, err)
	}
}

func TestOffline(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.Get("inexistente", time.Hour, func() ([]byte, error) { return nil, errors.New("f") }); err == nil {
		t.Fatal("clave inexistente debía fallar")
	}
	// Poblada en online; después, offline solo lee de la caché.
	if _, err := c.Get("k", time.Hour, func() ([]byte, error) { return []byte("x"), nil }); err != nil {
		t.Fatal(err)
	}
	c.Offline = true
	r, err := c.Get("k", time.Hour, nil)
	if err != nil || r.State != StateHit {
		t.Fatalf("offline con caché fresca: %+v, %v", r, err)
	}
	// Caducada en offline: se sirve stale en vez de romper.
	c.now = func() time.Time { return time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC) }
	r, err = c.Get("k", time.Hour, nil)
	if err != nil || r.State != StateStale {
		t.Fatalf("offline con caché caducada: %+v, %v", r, err)
	}
}

func TestFetchFallidoSinCache(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.Get("nueva", time.Hour, func() ([]byte, error) { return nil, errors.New("404") }); err == nil {
		t.Fatal("fetch fallido sin entrada previa debía propagar el error")
	}
}

func TestInvalidate(t *testing.T) {
	c := newTestCache(t)
	fetch := func() ([]byte, error) { return []byte("v1"), nil }
	if _, err := c.Get("k", time.Hour, fetch); err != nil {
		t.Fatal(err)
	}
	if err := c.Invalidate("k"); err != nil {
		t.Fatal(err)
	}
	r, err := c.Get("k", time.Hour, func() ([]byte, error) { return []byte("v2"), nil })
	if err != nil || string(r.Body) != "v2" {
		t.Fatalf("tras invalidate debía refetchear: %+v %v", r, err)
	}
	if err := c.Invalidate("no-existe"); err != nil {
		t.Fatalf("invalidate de clave inexistente no debía dar error: %v", err)
	}
}

// TestLastEstado: la UI necesita saber si lo servido viene de red, de
// caché o de una entrada caducada. Antes el estado se perdía dentro de
// Get y la app mentía sirviendo datos de hace horas como si fueran de
// ahora.
func TestLastEstado(t *testing.T) {
	c := newTestCache(t)
	if _, _, ok := c.Last(); ok {
		t.Fatal("sin peticiones, Last no debía tener estado")
	}
	if _, err := c.Get("k", time.Hour, func() ([]byte, error) { return []byte("v1"), nil }); err != nil {
		t.Fatal(err)
	}
	st, at, ok := c.Last()
	if !ok || st != StateMiss {
		t.Fatalf("tras un fetch: state=%q ok=%v", st, ok)
	}
	// El reloj del test está congelado: la fecha debe ser la del fetch.
	if want := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Errorf("fecha de los datos = %s, se esperaba %s", at, want)
	}
	// Segunda lectura: ahora es un hit y la fecha no cambia.
	if _, err := c.Get("k", time.Hour, func() ([]byte, error) {
		t.Fatal("no debía refetchear")
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	st, at2, _ := c.Last()
	if st != StateHit {
		t.Errorf("state = %q, se esperaba hit", st)
	}
	if !at2.Equal(at) {
		t.Errorf("la fecha de los datos cambió al servir de caché: %s → %s", at, at2)
	}
}

// TestLastCaducado: con el fetch roto se sirve la entrada vieja, y eso
// debe reportarse como caducado con su antigüedad real.
func TestLastCaducado(t *testing.T) {
	c := newTestCache(t)
	if _, err := c.Get("k", time.Hour, func() ([]byte, error) { return []byte("v1"), nil }); err != nil {
		t.Fatal(err)
	}
	// Reloj dos horas por delante: la entrada caduca.
	base := time.Now()
	c.now = func() time.Time { return base.Add(2 * time.Hour) }
	r, err := c.Get("k", time.Hour, func() ([]byte, error) { return nil, errors.New("sin red") })
	if err != nil || r.State != StateStale {
		t.Fatalf("debía servir la caducada: %+v %v", r, err)
	}
	st, at, ok := c.Last()
	if !ok || st != StateStale {
		t.Fatalf("state=%q ok=%v, se esperaba stale", st, ok)
	}
	if age := base.Add(2 * time.Hour).Sub(at); age < time.Hour {
		t.Errorf("la antigüedad reportada es %s, se esperaba ~2 h", age)
	}
}
