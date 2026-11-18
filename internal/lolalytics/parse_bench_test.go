package lolalytics

// Benchmarks de parseo de Lolalytics: aíslan CPU de red usando el HTML
// real cacheado (solo lectura de la caché personal).
//
// Ejecutar:
//   go test ./internal/lolalytics -bench 'BenchmarkParse' -benchtime 10x
//   go test ./internal/lolalytics -bench . -run XX -count 5

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// lolStats agrega una muestra de duraciones.
type lolStats struct {
	N    int
	P50  time.Duration
	P95  time.Duration
	Mean time.Duration
	Max  time.Duration
}

// summarizeLol ordena la muestra y devuelve p50/p95/max.
func summarizeLol(ds []time.Duration) lolStats {
	if len(ds) == 0 {
		return lolStats{}
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
	return lolStats{N: len(s), P50: at(0.50), P95: at(0.95), Mean: sum / time.Duration(len(s)), Max: s[len(s)-1]}
}

func reportLol(b *testing.B, label string, st lolStats) {
	if st.N == 0 {
		return
	}
	b.ReportMetric(float64(st.P50.Microseconds()), label+"_p50_us")
	b.ReportMetric(float64(st.P95.Microseconds()), label+"_p95_us")
	b.ReportMetric(float64(st.Max.Microseconds()), label+"_max_us")
}

// cachePage lee un HTML real de la caché personal sin modificarla.
func cachePage(tb testing.TB, hash string) []byte {
	tb.Helper()
	home, err := os.UserCacheDir()
	if err != nil {
		tb.Skip("sin directorio de caché")
	}
	path := filepath.Join(home, "mobatui", "cache", hash+".json.body.gz")
	f, err := os.Open(path)
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

// BenchmarkParseQwik aísla el coste de extraer el objeto Qwik del HTML:
// es el primer paso del parseo y el que recorre el índice completo.
func BenchmarkParseQwik(b *testing.B) {
	body := cachePage(b, "4b43fa6444d89a63d53ab62e5aa2909f") // Kai'Sa bottom
	b.SetBytes(int64(len(body)))
	b.ReportMetric(float64(len(body)), "html_bytes")
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		if _, err := parseQwik(body); err != nil {
			b.Fatalf("parseQwik: %v", err)
		}
		ds = append(ds, time.Since(t0))
	}
	reportLol(b, "qwik", summarizeLol(ds))
}

// BenchmarkDataFromMain aísla la extracción de tablas de las estructuras
// ya parseadas: separa "encontrar el objeto" de "leer sus tablas".
func BenchmarkDataFromMain(b *testing.B) {
	body := cachePage(b, "4b43fa6444d89a63d53ab62e5aa2909f")
	q, err := parseQwik(body)
	if err != nil {
		b.Fatalf("prepare: %v", err)
	}
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		main := q.mainData()
		if main == nil {
			b.Fatal("sin mainData")
		}
		_ = q.statTable(main["item"])
		_ = q.statTable(main["popularItem"])
		ds = append(ds, time.Since(t0))
	}
	reportLol(b, "tables", summarizeLol(ds))
	b.ReportMetric(float64(q.n), "qwik_objects")
}

// BenchmarkComboAndSlots aísla el parseo de las tablas de posiciones de
// compra y de los tríos: es lo que alimenta el planificador.
func BenchmarkComboAndSlots(b *testing.B) {
	body := cachePage(b, "4b43fa6444d89a63d53ab62e5aa2909f")
	q, err := parseQwik(body)
	if err != nil {
		b.Fatalf("prepare: %v", err)
	}
	main := q.mainData()
	b.ResetTimer()

	ds := make([]time.Duration, 0, b.N)
	for i := 0; i < b.N; i++ {
		t0 := time.Now()
		for _, k := range []string{"item1", "item2", "item3", "item4", "item5"} {
			_ = q.statTable(main[k])
		}
		_ = q.comboTable(main["builtBootSet3"])
		_ = q.spellsTable(main["spells"])
		ds = append(ds, time.Since(t0))
	}
	reportLol(b, "slots", summarizeLol(ds))
}

var _ = fmt.Sprintf
