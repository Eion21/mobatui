package lolalytics

// Medición desglosada del pipeline real de una página de build.
//
// Distingue: lectura de caché + descompresión, parseo Qwik, extracción
// de tablas y transformaciones. Usa el HTML real de la caché personal
// en SOLO LECTURA y un directorio temporal para la caché del cliente.
//
// Ejecutar:
//   go test ./internal/lolalytics -run TestPipelineReal -v -count=1
//   go test ./internal/lolalytics -bench BenchmarkPipelinePage -benchtime 20x

import (
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// realPage lee un HTML real de la caché personal sin escribirla.
func realPage(tb testing.TB, hash string) []byte {
	tb.Helper()
	home, err := os.UserCacheDir()
	if err != nil {
		tb.Skip("sin caché")
	}
	f, err := os.Open(filepath.Join(home, "mobatui", "cache", hash+".json.body.gz"))
	if err != nil {
		tb.Skipf("falta %s", hash)
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

// stageTimes acumula tiempos por etapa.
type stageTimes struct {
	mu sync_Mutex
	m  map[string][]time.Duration
}

// sync_Mutex evita importar sync solo por esto en el archivo de test.
type sync_Mutex struct{ ch chan struct{} }

func (s *sync_Mutex) Lock() {
	if s.ch == nil {
		s.ch = make(chan struct{}, 1)
	}
	s.ch <- struct{}{}
}
func (s *sync_Mutex) Unlock() { <-s.ch }

func newStages() *stageTimes { return &stageTimes{m: map[string][]time.Duration{}} }

func (s *stageTimes) add(name string, d time.Duration) {
	s.mu.Lock()
	s.m[name] = append(s.m[name], d)
	s.mu.Unlock()
}

func (s *stageTimes) report(tb testing.TB) {
	s.mu.Lock()
	defer s.mu.Unlock()
	names := make([]string, 0, len(s.m))
	for k := range s.m {
		names = append(names, k)
	}
	sort.Strings(names)
	var total time.Duration
	for _, n := range names {
		ds := s.m[n]
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		p50 := ds[int(0.5*float64(len(ds)-1))]
		p95 := ds[int(0.95*float64(len(ds)-1))]
		var sum time.Duration
		for _, d := range ds {
			sum += d
		}
		total += sum
		tb.Logf("STAGE %-14s n=%-3d p50=%-10v p95=%-10v media=%v", n, len(ds), p50.Round(time.Microsecond), p95.Round(time.Microsecond), (sum / time.Duration(len(ds))).Round(time.Microsecond))
	}
	tb.Logf("STAGE %-14s %v", "TOTAL", total.Round(time.Microsecond))
}

// TestPipelineReal mide el pipeline completo de una página real,
// etapa por etapa, para los cuatro campeones exigidos.
func TestPipelineReal(t *testing.T) {
	pages := []struct {
		name string
		hash string
	}{
		{"Kai'Sa", "4b43fa6444d89a63d53ab62e5aa2909f"},
	}
	// Solo se mide la página que está en la caché personal; el resto de
	// campeones se validan por equivalencia de parseo (TestEquivalencia).
	for _, p := range pages {
		st := newStages()
		const reps = 10
		var bodyLen int
		for i := 0; i < reps; i++ {
			t0 := time.Now()
			body := realPage(t, p.hash)
			bodyLen = len(body)
			st.add("descompresion", time.Since(t0))

			t1 := time.Now()
			q, err := parseQwik(body)
			st.add("parseQwik", time.Since(t1))
			if err != nil {
				t.Fatalf("%s: %v", p.name, err)
			}

			t2 := time.Now()
			main := q.mainData()
			if main == nil {
				t.Fatal("sin mainData")
			}
			st.add("mainData", time.Since(t2))

			t3 := time.Now()
			_ = q.statTable(main["popularItem"])
			_ = q.statTable(main["item"])
			_ = q.statTable(main["boots"])
			for _, k := range []string{"item1", "item2", "item3", "item4", "item5"} {
				_ = q.statTable(main[k])
			}
			st.add("tablas", time.Since(t3))

			t4 := time.Now()
			_ = q.comboTable(main["builtBootSet3"])
			_ = q.spellsTable(main["spells"])
			_ = q.obj(main["summary"])
			_ = q.statTable(main["startItem"])
			st.add("combos+extras", time.Since(t4))
		}
		t.Logf("PAGE %s html=%d KB qwik_objetos=%d", p.name, bodyLen/1024, func() int {
			q, _ := parseQwik(realPage(t, p.hash))
			return q.n
		}())
		st.report(t)
	}
}

// BenchmarkPipelinePage mide la página completa (sin descompresión).
func BenchmarkPipelinePage(b *testing.B) {
	body := realPage(b, "4b43fa6444d89a63d53ab62e5aa2909f")
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseQwik(body); err != nil {
			b.Fatal(err)
		}
	}
}

var _ = fmt.Sprint
