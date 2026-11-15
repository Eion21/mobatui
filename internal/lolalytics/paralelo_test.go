package lolalytics

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/model"
)

// servidorLento monta un servidor que tarda `latencia` en cada página
// de build, como la red real. Devuelve el cliente apuntando a él.
func servidorLento(t *testing.T, latencia time.Duration, hits *int64) *Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/lol/tierlist/", func(w http.ResponseWriter, r *http.Request) {
		l := r.URL.Query().Get("lane")
		if l == "" {
			l = "top"
		}
		// Un campeón distinto por carril para tener varias parejas.
		rows := [][3]any{{103, l, 4200}}
		w.Write([]byte(tierlistHTML(l, rows)))
	})
	mux.HandleFunc("/lol/ahri/build/", func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			atomic.AddInt64(hits, 1)
		}
		time.Sleep(latencia)
		w.Write([]byte(buildPageHTML()))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return New(c).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0)
}

// servidorConcurrente cuenta cuántas peticiones de build hay en vuelo a
// la vez y devuelve el máximo observado. Mide la propiedad que importa
// (solapamiento) sin depender de la velocidad de la máquina.
func servidorConcurrente(t *testing.T, latencia time.Duration) (*Client, *int64) {
	t.Helper()
	var enVuelo, maximo int64
	mux := http.NewServeMux()
	mux.HandleFunc("/lol/tierlist/", func(w http.ResponseWriter, r *http.Request) {
		l := r.URL.Query().Get("lane")
		if l == "" {
			l = "top"
		}
		rows := [][3]any{{103, l, 4200}}
		w.Write([]byte(tierlistHTML(l, rows)))
	})
	mux.HandleFunc("/lol/ahri/build/", func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&enVuelo, 1)
		for {
			m := atomic.LoadInt64(&maximo)
			if n <= m || atomic.CompareAndSwapInt64(&maximo, m, n) {
				break
			}
		}
		time.Sleep(latencia)
		atomic.AddInt64(&enVuelo, -1)
		w.Write([]byte(buildPageHTML()))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return New(c).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(0), &maximo
}

// TestCargaEnParaleloEsMasRapida: el pool debe solapar las peticiones.
//
// La comprobación principal es la concurrencia observada en el servidor:
// con carga secuencial el máximo en vuelo sería 1, con pool debe ser
// mayor. Eso no depende de la latencia de la máquina, a diferencia de un
// umbral de tiempo absoluto, que en Windows resultó frágil.
func TestCargaEnParaleloEsMasRapida(t *testing.T) {
	const latencia = 60 * time.Millisecond
	cl, maximo := servidorConcurrente(t, latencia)

	ini := time.Now()
	cbs, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	transcurrido := time.Since(ini)
	if len(cbs) == 0 {
		t.Fatal("sin builds")
	}

	if got := atomic.LoadInt64(maximo); got < 2 {
		t.Errorf("máximo de peticiones en vuelo = %d: la carga no está solapando", got)
	}
	// Comprobación secundaria, holgada: secuencial serían ~5 × 60 ms.
	// El margen es amplio a propósito para no depender del runner.
	if transcurrido > 600*time.Millisecond {
		t.Errorf("la carga tardó %s: demasiado para un pool con solapamiento", transcurrido)
	}
}

// TestProgresoDeCarga: el callback recibe avisos monótonos que acaban
// en total/total con el nombre del campeón.
func TestProgresoDeCarga(t *testing.T) {
	cl := servidorLento(t, 0, nil)
	type aviso struct {
		done, total int
		name        string
	}
	var avisos []aviso
	cl.OnProgress(func(done, total int, name string) {
		avisos = append(avisos, aviso{done, total, name})
	})
	if _, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(avisos) == 0 {
		t.Fatal("sin avisos de progreso")
	}
	last := avisos[len(avisos)-1]
	if last.done != last.total {
		t.Errorf("el último aviso es %d/%d, debería estar completo", last.done, last.total)
	}
	if last.name == "" {
		t.Error("el aviso no trae el nombre del campeón")
	}
	// Monótono: nunca retrocede (varios workers, orden de llegada libre).
	max := 0
	for _, a := range avisos {
		if a.done < max {
			t.Errorf("el progreso retrocedió: %d tras %d", a.done, max)
		}
		max = a.done
	}
	// Un aviso por página resuelta.
	if len(avisos) != last.total {
		t.Errorf("%d avisos para %d páginas", len(avisos), last.total)
	}
}

// TestResultadoIdenticoSecuencialYParalelo: el pool no debe cambiar el
// contenido, solo el orden de llegada. Se comparan dos cargas seguidas
// (la segunda sale de caché) y deben coincidir build a build.
func TestResultadoIdenticoSecuencialYParalelo(t *testing.T) {
	cl := servidorLento(t, 0, nil)
	primera, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	segunda, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(primera) != len(segunda) {
		t.Fatalf("nº de builds distinto: %d y %d", len(primera), len(segunda))
	}
	for i := range primera {
		a, b := primera[i], segunda[i]
		if a.ChampionKey != b.ChampionKey || a.Role != b.Role ||
			a.Total.Games != b.Total.Games || a.Patch != b.Patch {
			t.Fatalf("build %d difiere: %+v vs %+v", i, a, b)
		}
	}
	// Y el orden es estable entre cargas.
	for i := range primera {
		if primera[i].ChampionID != segunda[i].ChampionID || primera[i].Role != segunda[i].Role {
			t.Errorf("orden inestable en %d: %v/%v vs %v/%v", i,
				primera[i].ChampionID, primera[i].Role,
				segunda[i].ChampionID, segunda[i].Role)
		}
	}
	_ = model.GameLoL
}

// TestRitmoCortesNoSeMultiplica: el pool no debe acelerar el ritmo de
// peticiones contra la fuente. Con un delay de cortesía, dos descargas
// deben seguir separadas al menos ese delay.
func TestRitmoCortesNoSeMultiplica(t *testing.T) {
	const delay = 40 * time.Millisecond
	mux := http.NewServeMux()
	var hits int64
	mux.HandleFunc("/lol/tierlist/", func(w http.ResponseWriter, r *http.Request) {
		l := r.URL.Query().Get("lane")
		if l == "" {
			l = "top"
		}
		w.Write([]byte(tierlistHTML(l, [][3]any{{103, l, 4200}})))
	})
	mux.HandleFunc("/lol/ahri/build/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Write([]byte(buildPageHTML()))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c).WithBase(srv.URL).WithHTTPClient(srv.Client()).WithDelay(delay)

	ini := time.Now()
	if _, err := cl.LoadBuilds(map[int]string{103: "Ahri"}, nil); err != nil {
		t.Fatal(err)
	}
	transcurrido := time.Since(ini)
	n := atomic.LoadInt64(&hits)
	// 5 tierlists + N builds, cada una con su delay serializado.
	if want := time.Duration(n) * delay; transcurrido < want {
		t.Errorf("%d peticiones en %s: menos del ritmo cortés (%s)", n, transcurrido, want)
	}
}
