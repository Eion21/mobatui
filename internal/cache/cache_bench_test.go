package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// BenchmarkGetWarm mide leer una entrada ya cacheada desde disco
// (descomprimir gzip incluido): el coste que paga cada petición servida.
func BenchmarkGetWarm(b *testing.B) {
	home, _ := os.UserCacheDir()
	src := filepath.Join(home, "mobatui", "cache")
	if _, err := os.Stat(src); err != nil {
		b.Skip("sin caché personal")
	}
	// Copia de UNA entrada a un directorio temporal: se mide lectura,
	// no se escribe en la caché del usuario.
	key := "4b43fa6444d89a63d53ab62e5aa2909f"
	tmp := b.TempDir()
	for _, name := range []string{key + ".json", key + ".json.body.gz"} {
		raw, err := os.ReadFile(filepath.Join(src, name))
		if err != nil {
			b.Skipf("falta %s", name)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), raw, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	c, err := New(tmp)
	if err != nil {
		b.Fatal(err)
	}
	c.Offline = true

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := c.Get("https://lolalytics.com/lol/kaisa/build/?lane=bottom", time.Hour, func() ([]byte, error) {
			return nil, os.ErrNotExist
		})
		if err != nil || res.Body == nil {
			b.Skipf("no se pudo leer: %v", err)
		}
	}
}

// BenchmarkGetMiss mide el camino de fallo (sin entrada): debe ser barato.
func BenchmarkGetMiss(b *testing.B) {
	c, err := New(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	c.Offline = true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Get("https://ejemplo.invalido/nada", time.Hour, func() ([]byte, error) {
			return nil, os.ErrNotExist
		})
	}
}
