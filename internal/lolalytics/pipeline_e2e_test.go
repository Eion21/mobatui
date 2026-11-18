package lolalytics

// Prueba end-to-end reproducible del pipeline de LoL: carga páginas Qwik
// sintéticas servidas por httptest y comprueba qué se obtiene, cuántas
// peticiones se hacen y cuánto se tarda.
//
// POR QUÉ: el benchmark LoadSession de la fase 1 cargaba fixtures de u.gg
// y CERO páginas Qwik. Servía para medir el arranque con fixtures, no el
// parser. Esta prueba mide el flujo real de páginas, que es lo que el
// usuario paga cuando hay datos de Lolalytics.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
)

// pageServer sirve un estado Qwik sintético por campeón×carril y cuenta
// peticiones.
type pageServer struct {
	mu   sync.Mutex
	hits int
	body []byte
}

func newPageServer(t *testing.T) (*pageServer, *httptest.Server) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "qwik_page.html"))
	if err != nil {
		t.Fatalf("fixture obligatorio: %v", err)
	}
	ps := &pageServer{body: body}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		ps.hits++
		ps.mu.Unlock()
		// Cualquier ruta de campeón devuelve el mismo estado: lo que se
		// mide es el pipeline, no el contenido por campeón.
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(ps.body)
	}))
	return ps, srv
}

func (p *pageServer) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits
}

// TestPipelineEndToEndQwik mide el pipeline real sobre páginas Qwik
// servidas localmente: peticiones, páginas procesadas, tiempo y
// resultado utilizable.
func TestPipelineEndToEndQwik(t *testing.T) {
	ps, srv := newPageServer(t)
	defer srv.Close()

	c, err := cache.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c).WithBase(srv.URL)

	// 5 páginas campeón×carril, como una carga representativa.
	nPages := 5
	var total time.Duration
	parsed := 0
	for i := 0; i < nPages; i++ {
		start := time.Now()
		bd, err := cl.buildPage(fmt.Sprintf("campeon%d", i), "bottom")
		total += time.Since(start)
		if err != nil {
			t.Fatalf("página %d: %v", i, err)
		}
		if bd == nil {
			t.Fatalf("página %d sin datos", i)
		}
		// Datos UTILIZABLES, no solo parseados.
		if bd.games <= 0 {
			t.Errorf("página %d: games = %d", i, bd.games)
		}
		if len(bd.items) == 0 {
			t.Errorf("página %d: sin ítems", i)
		}
		if len(bd.slots) != 5 {
			t.Errorf("página %d: slots = %d, se esperaban 5", i, len(bd.slots))
		}
		parsed++
	}

	t.Logf("E2E páginas_solicitadas=%d páginas_parseadas=%d peticiones_http=%d",
		nPages, parsed, ps.count())
	t.Logf("E2E tiempo_total=%v media_por_página=%v", total.Round(time.Microsecond),
		(total / time.Duration(nPages)).Round(time.Microsecond))
	t.Logf("E2E bytes_por_página=%d", len(ps.body))

	if parsed != nPages {
		t.Errorf("se parsearon %d de %d páginas", parsed, nPages)
	}
	// Una petición por página: la caché de disco está vacía la primera vez.
	if got := ps.count(); got != nPages {
		t.Errorf("peticiones = %d, se esperaban %d", got, nPages)
	}
}

// TestPipelineSegundaCargaUsaCache: la segunda carga no vuelve a pedir
// las páginas (caché de disco fresca).
func TestPipelineSegundaCargaUsaCache(t *testing.T) {
	ps, srv := newPageServer(t)
	defer srv.Close()

	dir := t.TempDir()
	c, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cl := New(c).WithBase(srv.URL)

	if _, err := cl.buildPage("kaisa", "bottom"); err != nil {
		t.Fatal(err)
	}
	primera := ps.count()

	// Mismo directorio de caché: la entrada está fresca.
	c2, err := cache.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	cl2 := New(c2).WithBase(srv.URL)
	if _, err := cl2.buildPage("kaisa", "bottom"); err != nil {
		t.Fatal(err)
	}

	if ps.count() != primera {
		t.Errorf("la segunda carga volvió a pedir: %d → %d", primera, ps.count())
	}
	t.Logf("E2E2 primera=%d segunda_total=%d (sin peticiones nuevas)", primera, ps.count())
}

// TestPipelineDatosIdenticosEntreCargas: dos cargas del mismo fixture
// producen exactamente los mismos datos. Es la garantía de que el
// pipeline no altera estadísticas.
func TestPipelineDatosIdenticosEntreCargas(t *testing.T) {
	_, srv := newPageServer(t)
	defer srv.Close()

	mk := func() *buildData {
		c, err := cache.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		bd, err := New(c).WithBase(srv.URL).buildPage("kaisa", "bottom")
		if err != nil {
			t.Fatal(err)
		}
		return bd
	}

	a, b := mk(), mk()
	if a.games != b.games {
		t.Errorf("games %d vs %d", a.games, b.games)
	}
	if a.wr != b.wr {
		t.Errorf("wr %v vs %v", a.wr, b.wr)
	}
	if len(a.items) != len(b.items) {
		t.Fatalf("items %d vs %d", len(a.items), len(b.items))
	}
	for i := range a.items {
		if a.items[i].ID != b.items[i].ID || a.items[i].Games != b.items[i].Games {
			t.Errorf("item %d difiere", i)
		}
	}
	// Serialización estable como comprobación final.
	ja, _ := json.Marshal(a.items)
	jb, _ := json.Marshal(b.items)
	if string(ja) != string(jb) {
		t.Error("los datos serializados difieren entre cargas")
	}
}
