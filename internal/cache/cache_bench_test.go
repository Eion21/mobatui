package cache

import (
	"bytes"
	"os"
	"testing"
	"time"
)

// iconoBench es un PNG real del repositorio (3.883 B), sin usar la caché
// del usuario.
func iconoBench(b *testing.B) []byte {
	b.Helper()
	raw, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		b.Skip("sin fixture")
	}
	return raw
}

// validadorBench replica el coste del validador real sin importar el
// paquete ddragon (evita un ciclo de importación desde un _test).
func validadorBench(body []byte) error {
	if !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
		return errInvalido
	}
	return nil
}

var errInvalido = bytes.ErrTooLarge

func cacheBench(b *testing.B) *Cache {
	dir := b.TempDir()
	c, err := New(dir)
	if err != nil {
		b.Fatal(err)
	}
	return c
}

// BenchmarkGetValidatedHit: lectura de un hit fresco con validador.
func BenchmarkGetValidatedHit(b *testing.B) {
	c := cacheBench(b)
	raw := iconoBench(b)
	const k = "https://cdn/icono.png"
	if _, err := c.GetValidated(k, 720*time.Hour, func() ([]byte, error) { return raw, nil }, validadorBench); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.GetValidated(k, 720*time.Hour, func() ([]byte, error) {
			b.Fatal("no debe hacer fetch")
			return nil, nil
		}, validadorBench); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetValidatedHitSinValidador es la referencia: el mismo hit
// sin pagar el validador.
func BenchmarkGetValidatedHitSinValidador(b *testing.B) {
	c := cacheBench(b)
	raw := iconoBench(b)
	const k = "https://cdn/icono.png"
	if _, err := c.Get(k, 720*time.Hour, func() ([]byte, error) { return raw, nil }); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.Get(k, 720*time.Hour, func() ([]byte, error) {
			b.Fatal("no debe hacer fetch")
			return nil, nil
		}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetValidatedMiss: descarga simulada + validación + store.
func BenchmarkGetValidatedMiss(b *testing.B) {
	c := cacheBench(b)
	raw := iconoBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := "https://cdn/icono-" + string(rune('a'+i%26)) + ".png"
		if _, err := c.GetValidated(k, 720*time.Hour, func() ([]byte, error) { return raw, nil }, validadorBench); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetValidatedHitEnvenenadoMideRecarga: la entrada en disco es
// inválida, así que cada lectura la descarta, valida el cuerpo nuevo y
// lo reescribe.
func BenchmarkGetValidatedHitEnvenenadoMideRecarga(b *testing.B) {
	c := cacheBench(b)
	raw := iconoBench(b)
	const k = "https://cdn/envenenado.png"
	// Se siembra sin validador.
	if _, err := c.Get(k, 720*time.Hour, func() ([]byte, error) {
		return []byte("BASURA"), nil
	}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.GetValidated(k, 720*time.Hour, func() ([]byte, error) { return raw, nil }, validadorBench); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkDiscard es el coste puro de descartar la entrada envenenada:
// dos os.Remove, sin lectura ni escritura de cuerpos.
func BenchmarkDiscard(b *testing.B) {
	c := cacheBench(b)
	const k = "https://cdn/x.png"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if _, err := c.Get(k, time.Hour, func() ([]byte, error) { return []byte("x"), nil }); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		c.discard(k)
	}
}
