package deadlock

// Benchmarks de la ruta de rutas de Deadlock: separan decodificación de
// la muestra y destilado del consenso (CPU) del coste de red.
//
// Ejecutar:
//   go test ./internal/deadlock -bench 'BenchmarkDecode|BenchmarkBuildRoute' -benchtime 10x

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/Eion21/mobatui/internal/cache"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// dlStats agrega una muestra de duraciones.
type dlStats struct {
	N    int
	P50  time.Duration
	P95  time.Duration
	Max  time.Duration
	Mean time.Duration
}

func summarizeDL(ds []time.Duration) dlStats {
	if len(ds) == 0 {
		return dlStats{}
	}
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	var sum time.Duration
	for _, d := range s {
		sum += d
	}
	at := func(q float64) time.Duration {
		i := int(q * float64(len(s)-1))
		if i < 0 {
			i = 0
		}
		if i >= len(s) {
			i = len(s) - 1
		}
		return s[i]
	}
	return dlStats{N: len(s), P50: at(0.50), P95: at(0.95), Max: s[len(s)-1], Mean: sum / time.Duration(len(s))}
}

func reportDL(b *testing.B, label string, st dlStats) {
	if st.N == 0 {
		return
	}
	b.ReportMetric(float64(st.P50.Microseconds()), label+"_p50_us")
	b.ReportMetric(float64(st.P95.Microseconds()), label+"_p95_us")
	b.ReportMetric(float64(st.Max.Microseconds()), label+"_max_us")
}

// cachedSample lee una muestra real de la caché personal (solo lectura).
func cachedSample(tb testing.TB, hash string) []byte {
	tb.Helper()
	home, err := os.UserCacheDir()
	if err != nil {
		tb.Skip("sin caché")
	}
	f, err := os.Open(filepath.Join(home, "mobatui", "cache", hash+".json.body.gz"))
	if err != nil {
		tb.Skipf("falta el fixture %s: %v", hash, err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		tb.Fatalf("gzip: %v", err)
	}
	var out []byte
	buf := make([]byte, 64*1024)
	for {
		n, e := gz.Read(buf)
		out = append(out, buf[:n]...)
		if e != nil {
			break
		}
	}
	return out
}

// benchCatalogReal obtiene el catálogo de ítems de la CACHÉ PERSONAL
// (solo lectura) usando el mismo código que la app: un cliente offline
// apuntando a ella. Es el trabajo real, sin red.
func benchCatalogReal(tb testing.TB) (map[int]bool, map[int][]int) {
	tb.Helper()
	home, err := os.UserCacheDir()
	if err != nil {
		tb.Skip("sin caché")
	}
	dir := filepath.Join(home, "mobatui", "cache")
	if _, err := os.Stat(dir); err != nil {
		tb.Skip("sin caché personal")
	}
	c, err := cache.New(dir)
	if err != nil {
		tb.Fatalf("cache: %v", err)
	}
	c.Offline = true
	api := NewAPI(c, "testdata/deadlock")
	shop, comps, err := api.runsCatalog()
	if err != nil || len(shop) == 0 {
		tb.Skipf("sin catálogo: %v", err)
	}
	set := make(map[int]bool, len(shop))
	for id := range shop {
		set[id] = true
	}
	tb.Logf("catálogo: %d ítems, %d con componentes", len(shop), len(comps))
	return set, comps
}

// decodeFirstMatch decodifica solo la primera partida del payload.
func decodeFirstMatch(body []byte, out *rawMatch) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	if _, err := dec.Token(); err != nil { // '['
		return err
	}
	return dec.Decode(out)
}

// BenchmarkDecodeRuns mide decodificar la muestra completa de partidas
// de un héroe. Es lo que se repite al reabrir el detalle sin memo.
func BenchmarkDecodeRuns(b *testing.B) {
	body := cachedSample(b, "7a11242de4d1a18e40953f3df8330860") // hero 76
	shop, comps := benchCatalogReal(b)
	b.SetBytes(int64(len(body)))
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		if _, err := decodeRuns(bufio.NewReaderSize(bytes.NewReader(body), 1<<20), 76, shop, comps); err != nil {
			b.Fatalf("decodeRuns: %v", err)
		}
		ds = append(ds, time.Since(t0))
	}
	reportDL(b, "decode", summarizeDL(ds))
}

// BenchmarkBuildRoute mide destilar la ruta de consenso de runs ya
// decodificadas: CPU pura del algoritmo, sin parseo.
func BenchmarkBuildRoute(b *testing.B) {
	body := cachedSample(b, "7a11242de4d1a18e40953f3df8330860")
	shop, comps := benchCatalogReal(b)
	runs, err := decodeRuns(bufio.NewReaderSize(bytes.NewReader(body), 1<<20), 76, shop, comps)
	if err != nil {
		b.Fatalf("prepare: %v", err)
	}
	b.ReportMetric(float64(len(runs)), "runs")
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		_ = BuildRoute(runs, 0.3)
		ds = append(ds, time.Since(t0))
	}
	reportDL(b, "route", summarizeDL(ds))
}

// BenchmarkBuildEvents mide solo la clasificación de compras de una
// partida, el bucle más interno de decodeRuns.
func BenchmarkBuildEvents(b *testing.B) {
	body := cachedSample(b, "7a11242de4d1a18e40953f3df8330860")
	shop, comps := benchCatalogReal(b)
	var m rawMatch
	if err := decodeFirstMatch(body, &m); err != nil {
		b.Fatalf("prepare: %v", err)
	}
	var items []rawItem
	for _, p := range m.Players {
		if p.HeroID == 76 {
			items = p.Items
			break
		}
	}
	if len(items) == 0 {
		b.Skip("sin ítems del héroe en la primera partida")
	}
	b.ReportMetric(float64(len(items)), "items")
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		_, _ = buildEvents(items, shop, comps)
		ds = append(ds, time.Since(t0))
	}
	reportDL(b, "events", summarizeDL(ds))
}
