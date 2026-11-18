package cache

import (
	"bytes"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

// ─── validación en lectura (opción a) ───────────────────────────────
//
// Especificación: NINGÚN cuerpo inválido debe servirse, venga de un hit
// fresco, de un respaldo stale o del modo offline. Una entrada que no
// supera el validador se descarta del disco y se intenta recargar.

// validadorPNG acepta solo cuerpos que empiecen por "PNG:".
func validadorPNG(b []byte) error {
	if !bytes.HasPrefix(b, []byte("PNG:")) {
		return errors.New("no empieza por PNG:")
	}
	return nil
}

func nuevoCache(t *testing.T) *Cache {
	t.Helper()
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func entradasEnDisco(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range ents {
		out = append(out, e.Name())
	}
	return out
}

// TestLecturaFrescaEnvenenadaSeDescartaYRecarga: entrada fresca en disco
// con cuerpo inválido + servidor válido ⇒ devuelve el icono correcto y la
// entrada envenenada ya no está.
func TestLecturaFrescaEnvenenadaSeDescartaYRecarga(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/icono.png"
	bueno := []byte("PNG:bueno")

	// Se siembra la entrada envenenada saltándose el validador (como
	// haría una versión anterior).
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		return []byte("BASURA"), nil
	}); err != nil {
		t.Fatal(err)
	}

	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return bueno, nil }, validadorPNG)
	if err != nil {
		t.Fatalf("debía recargar y servir el icono correcto: %v", err)
	}
	if !bytes.Equal(res.Body, bueno) {
		t.Errorf("cuerpo servido = %q, se esperaba %q", res.Body, bueno)
	}
	if res.State != StateMiss {
		t.Errorf("estado = %v, se esperaba StateMiss (hubo recarga)", res.State)
	}
	// La entrada envenenada ya no debe estar.
	res2, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		t.Fatal("no debía pedir: la recarga quedó guardada")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res2.Body, bueno) {
		t.Errorf("en disco quedó %q", res2.Body)
	}
}

// TestStaleEnvenenadaNoSeSirveServidorCaido: entrada caducada envenenada
// + servidor caído ⇒ error, nunca el cuerpo corrupto.
func TestStaleEnvenenadaNoSeSirveServidorCaido(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/otro.png"
	clk := &reloj{t: time.Unix(1000, 0)}
	c.now = clk.now

	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		return []byte("BASURA"), nil
	}); err != nil {
		t.Fatal(err)
	}
	clk.t = clk.t.Add(2 * time.Hour) // caduca

	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return nil, errors.New("red caída") }, validadorPNG)
	if err == nil {
		t.Fatalf("debía fallar; sirvió %q (estado=%v)", res.Body, res.State)
	}
	if bytes.Contains(res.Body, []byte("BASURA")) {
		t.Errorf("sirvió el cuerpo corrupto como respaldo: %q", res.Body)
	}
}

// TestStaleEnvenenadaNoSeSirveRecargaCorrupta: entrada caducada
// envenenada + servidor que devuelve otra basura ⇒ no se sirve ninguna.
func TestStaleEnvenenadaNoSeSirveRecargaCorrupta(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/tercero.png"
	clk := &reloj{t: time.Unix(2000, 0)}
	c.now = clk.now

	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		return []byte("BASURA-VIEJA"), nil
	}); err != nil {
		t.Fatal(err)
	}
	clk.t = clk.t.Add(2 * time.Hour)

	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return []byte("BASURA-NUEVA"), nil }, validadorPNG)
	if err == nil {
		t.Fatalf("debía fallar; sirvió %q", res.Body)
	}
	if !errors.Is(err, ErrInvalidBody) {
		t.Errorf("error = %v, se esperaba ErrInvalidBody", err)
	}
	if bytes.Contains(res.Body, []byte("BASURA")) {
		t.Errorf("sirvió un cuerpo corrupto: %q", res.Body)
	}
}

// TestOfflineEnvenenadaNoSirveBytes: modo offline con entrada envenenada
// ⇒ error, sin bytes corruptos. La entrada NO se borra (no hay red para
// reponerla), para no empeorar la situación al volver a conectarse.
func TestOfflineEnvenenadaNoSirveBytes(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/offline.png"
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		return []byte("BASURA"), nil
	}); err != nil {
		t.Fatal(err)
	}
	c.Offline = true
	// Se adelanta el reloj para que la entrada NO sea fresca: así la
	// lectura cae forzosamente en la rama offline, no en la del hit.
	if c.now == nil {
		c.now = time.Now
	}
	base := c.now()
	c.now = func() time.Time { return base.Add(2 * time.Hour) }

	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("offline no debe hacer fetch"); return nil, nil },
		validadorPNG)
	if err == nil {
		t.Fatalf("offline debía rechazar el cuerpo inválido; sirvió %q", res.Body)
	}
	if bytes.Contains(res.Body, []byte("BASURA")) {
		t.Errorf("sirvió el cuerpo corrupto en offline: %q", res.Body)
	}
	// La entrada debe seguir en disco: borrarla offline impediría
	// recuperarla al reconectar.
	if len(entradasEnDisco(t, c.Dir)) == 0 {
		t.Error("no debía borrarse la entrada en offline")
	}
}

// TestEntradaValidaSeSirveIgual: fresca, vencida y offline con cuerpo
// válido ⇒ mismo resultado que antes del cambio.
func TestEntradaValidaSeSirveIgual(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/valido.png"
	bueno := []byte("PNG:valido")
	clk := &reloj{t: time.Unix(3000, 0)}
	c.now = clk.now

	if _, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return bueno, nil }, validadorPNG); err != nil {
		t.Fatal(err)
	}

	// Fresca ⇒ StateHit.
	res, err := c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("no debía pedir"); return nil, nil }, validadorPNG)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != StateHit || !bytes.Equal(res.Body, bueno) {
		t.Errorf("fresca: estado=%v cuerpo=%q", res.State, res.Body)
	}

	// Vencida + red caída ⇒ StateStale con el cuerpo válido.
	clk.t = clk.t.Add(2 * time.Hour)
	res, err = c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { return nil, errors.New("red caída") }, validadorPNG)
	if err != nil {
		t.Fatalf("una entrada VÁLIDA vencida debe servirse como stale: %v", err)
	}
	if res.State != StateStale || !bytes.Equal(res.Body, bueno) {
		t.Errorf("stale: estado=%v cuerpo=%q", res.State, res.Body)
	}

	// Offline ⇒ StateStale igual que antes.
	c.Offline = true
	res, err = c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("offline"); return nil, nil }, validadorPNG)
	if err != nil {
		t.Fatalf("offline con cuerpo válido debe servirse: %v", err)
	}
	if res.State != StateStale || !bytes.Equal(res.Body, bueno) {
		t.Errorf("offline: estado=%v cuerpo=%q", res.State, res.Body)
	}
}

// TestSinValidadorSinCambios: un Cache o llamada sin validador se
// comporta exactamente igual que hoy.
func TestSinValidadorSinCambios(t *testing.T) {
	c := nuevoCache(t)
	const clave = "k"
	basura := []byte("lo que sea")
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) { return basura, nil }); err != nil {
		t.Fatal(err)
	}
	res, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		t.Fatal("no debía pedir")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.State != StateHit || !bytes.Equal(res.Body, basura) {
		t.Errorf("estado=%v cuerpo=%q", res.State, res.Body)
	}
	// GetValidated con validate=nil delega en Get.
	res, err = c.GetValidated(clave, time.Hour,
		func() ([]byte, error) { t.Fatal("no debía pedir"); return nil, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != StateHit || !bytes.Equal(res.Body, basura) {
		t.Errorf("nil: estado=%v cuerpo=%q", res.State, res.Body)
	}
}

// TestDescarteConcurrenteEsSeguro: varios goroutines leyendo la misma
// entrada envenenada mientras se descarta y recarga. Sin pánico y sin
// servir nunca el cuerpo inválido.
func TestDescarteConcurrenteEsSeguro(t *testing.T) {
	c := nuevoCache(t)
	const clave = "https://cdn/concurrente.png"
	bueno := []byte("PNG:concurrente")
	if _, err := c.Get(clave, time.Hour, func() ([]byte, error) {
		return []byte("BASURA"), nil
	}); err != nil {
		t.Fatal(err)
	}

	const n = 8
	var wg sync.WaitGroup
	errores := make([]error, n)
	cuerpos := make([][]byte, n)
	estados := make([]State, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			res, err := c.GetValidated(clave, time.Hour,
				func() ([]byte, error) { return bueno, nil }, validadorPNG)
			errores[i], cuerpos[i], estados[i] = err, res.Body, res.State
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errores[i] != nil {
			t.Errorf("goroutine %d: %v", i, errores[i])
			continue
		}
		if bytes.Contains(cuerpos[i], []byte("BASURA")) {
			t.Errorf("goroutine %d sirvió el cuerpo corrupto: %q", i, cuerpos[i])
		}
		if !bytes.Equal(cuerpos[i], bueno) {
			t.Errorf("goroutine %d: cuerpo=%q", i, cuerpos[i])
		}
	}
	_ = estados
}
