package cache

// Regresión de la procedencia por petición.
//
// Riesgo corregido: `Result` no llevaba fecha, así que el consumidor
// tenía que llamar a `Cache.Last()`, que guarda el estado de la ÚLTIMA
// respuesta servida GLOBALMENTE. Con consultas concurrentes podía
// recibir la fecha de otra respuesta.
//
// Estas pruebas no dependen de `time.Sleep` para provocar el
// intercalado: usan canales como barreras.

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestResultLlevaSuPropiaFecha: dos consultas seguidas devuelven fechas
// distintas, cada una la suya, sin depender de Last().
func TestResultLlevaSuPropiaFecha(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return t0 }

	resA, err := c.Get("A", time.Hour, func() ([]byte, error) { return []byte("a"), nil })
	if err != nil {
		t.Fatal(err)
	}
	t1 := t0.Add(90 * time.Minute)
	c.now = func() time.Time { return t1 }

	resB, err := c.Get("B", time.Hour, func() ([]byte, error) { return []byte("b"), nil })
	if err != nil {
		t.Fatal(err)
	}

	if !resA.FetchedAt.Equal(t0) {
		t.Errorf("A.FetchedAt = %v, se esperaba %v", resA.FetchedAt, t0)
	}
	if !resB.FetchedAt.Equal(t1) {
		t.Errorf("B.FetchedAt = %v, se esperaba %v", resB.FetchedAt, t1)
	}
	// Last() sí apunta a la última (B): por eso no sirve por petición.
	if _, at, ok := c.Last(); !ok || !at.Equal(t1) {
		t.Errorf("Last() = %v, se esperaba la de B (%v)", at, t1)
	}
}

// TestFechasEnCadaEstado comprueba que hit, stale, miss y offline
// informan la fecha correcta.
func TestFechasEnCadaEstado(t *testing.T) {
	t0 := time.Date(2026, 2, 2, 8, 0, 0, 0, time.UTC)
	newClock := func(at time.Time) func() time.Time { return func() time.Time { return at } }

	// MISS: la fecha es la de la descarga.
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.now = newClock(t0)
	miss, err := c.Get("k", time.Hour, func() ([]byte, error) { return []byte("x"), nil })
	if err != nil {
		t.Fatal(err)
	}
	if miss.State != StateMiss {
		t.Fatalf("estado = %v", miss.State)
	}
	if !miss.FetchedAt.Equal(t0) {
		t.Errorf("miss: %v, se esperaba %v", miss.FetchedAt, t0)
	}

	// HIT: misma fecha, la de la entrada.
	c.now = newClock(t0.Add(10 * time.Minute))
	hit, err := c.Get("k", time.Hour, func() ([]byte, error) { return nil, errors.New("no debe llamarse") })
	if err != nil {
		t.Fatal(err)
	}
	if hit.State != StateHit {
		t.Fatalf("estado = %v", hit.State)
	}
	if !hit.FetchedAt.Equal(t0) {
		t.Errorf("hit: %v, se esperaba %v", hit.FetchedAt, t0)
	}

	// STALE: caducó y el fetch falla; la fecha es la de la entrada vieja.
	c.now = newClock(t0.Add(2 * time.Hour))
	stale, err := c.Get("k", time.Hour, func() ([]byte, error) { return nil, errors.New("fuente caída") })
	if err != nil {
		t.Fatal(err)
	}
	if stale.State != StateStale {
		t.Fatalf("estado = %v", stale.State)
	}
	if !stale.FetchedAt.Equal(t0) {
		t.Errorf("stale: %v, se esperaba %v", stale.FetchedAt, t0)
	}

	// OFFLINE con entrada: se sirve como stale con su fecha.
	c.Offline = true
	off, err := c.Get("k", time.Hour, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !off.FetchedAt.Equal(t0) {
		t.Errorf("offline: %v, se esperaba %v", off.FetchedAt, t0)
	}
}

// TestFechaDesconocidaNoSeInventa: sin entrada no hay fecha, y el error
// lo dice; no se rellena con el reloj.
func TestFechaDesconocidaNoSeInventa(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c.Offline = true
	res, err := c.Get("no-existe", time.Hour, nil)
	if err == nil {
		t.Fatal("se esperaba error en offline sin entrada")
	}
	if !res.FetchedAt.IsZero() {
		t.Errorf("no debe inventarse una fecha: %v", res.FetchedAt)
	}
}

// TestConcurrenciaFechasPropias es el caso obligatorio: la consulta A
// devuelve sus datos, la consulta B sirve los suyos ANTES de que A lea
// la procedencia, y A conserva la fecha y el estado que le
// corresponden.
//
// El intercalado se fuerza con canales, no con sleeps: A espera en una
// barrera a que B haya completado su Get.
func TestConcurrenciaFechasPropias(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tA := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	tB := tA.Add(6 * time.Hour)

	var (
		aGot       Result
		aErr       error
		bDone      = make(chan struct{})
		aReadsDate = make(chan struct{})
		wg         sync.WaitGroup
	)

	// Consulta B: espera a que A tenga su resultado, luego sirve los
	// suyos y avisa para que A lea la procedencia después.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-aReadsDate
		c.now = func() time.Time { return tB }
		if _, err := c.Get("hero-B", time.Hour, func() ([]byte, error) { return []byte("b"), nil }); err != nil {
			t.Errorf("B: %v", err)
		}
		close(bDone)
	}()

	// Consulta A.
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.now = func() time.Time { return tA }
		aGot, aErr = c.Get("hero-A", time.Hour, func() ([]byte, error) { return []byte("a"), nil })
		// Avisar a B para que pise el estado global, y esperar a que
		// termine antes de comprobar nada.
		close(aReadsDate)
		<-bDone
	}()

	wg.Wait()

	if aErr != nil {
		t.Fatalf("A: %v", aErr)
	}
	// A conserva SU fecha, aunque B se sirviera después.
	if !aGot.FetchedAt.Equal(tA) {
		t.Errorf("A.FetchedAt = %v, se esperaba %v (fecha de B: %v)",
			aGot.FetchedAt, tA, tB)
	}
	if aGot.State != StateMiss {
		t.Errorf("A.State = %v", aGot.State)
	}
	if string(aGot.Body) != "a" {
		t.Errorf("A.Body = %q", aGot.Body)
	}
	// Y Last() sí quedó en B, que es justo lo que no debe usarse para
	// saber la procedencia de A.
	if _, at, ok := c.Last(); !ok || !at.Equal(tB) {
		t.Logf("Last() = %v (la de B: %v)", at, tB)
	}
}

// TestConcurrenciaSinCarreras comprueba con -race que lecturas y
// escrituras concurrentes no comparten estado indebido.
//
// OJO: no se inyecta el reloj por goroutine. `c.now` es un campo
// compartido; pisarlo desde varias goroutines sería una carrera del
// propio test, no del código. Se usa el reloj real y se comprueba que
// cada resultado trae su propio cuerpo y una fecha no vacía.
func TestConcurrenciaSinCarreras(t *testing.T) {
	c, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			key := "k" + string(rune('A'+i))
			res, err := c.Get(key, time.Hour, func() ([]byte, error) {
				return []byte(key), nil
			})
			if err != nil {
				t.Errorf("%s: %v", key, err)
				return
			}
			if string(res.Body) != key {
				t.Errorf("%s: cuerpo %q", key, res.Body)
			}
			if res.FetchedAt.IsZero() {
				t.Errorf("%s: sin fecha", key)
			}
		}(i)
	}
	wg.Wait()
}
