package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/staticdata"
)

type blockItemProvider struct {
	staticdata.Provider
	release chan struct{}
	entered chan struct{}
}

func (p *blockItemProvider) ItemIcon(int) ([]byte, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	<-p.release
	return nil, errors.New("liberado")
}

// TestItemArtNoBloqueaElBucle: abrir Ruta o Alt. no puede congelar la
// TUI aunque el CDN de iconos no responda.
//
// Regresión: ensureTabArt llamaba a renderItemArt, que descarga el PNG
// con ItemIcon de forma síncrona, desde dentro de Update. Era el mismo
// defecto que tenían las runas antes de la fase 6.
func TestItemArtNoBloqueaElBucle(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	prov := &blockItemProvider{
		Provider: d.session.Provider,
		release:  make(chan struct{}),
		entered:  make(chan struct{}, 1),
	}
	d.session.Provider = prov
	d.session.InvalidateArt()
	d.tab = 0

	start := time.Now()
	d2, cmd := d.Update(keyMsg("2"))
	el := time.Since(start)
	if el > 200*time.Millisecond {
		t.Errorf("ITEMASINC Update tardó %v: bloqueó", el)
	}
	if cmd == nil {
		t.Fatal("ITEMASINC debía devolver Cmd")
	}
	t.Logf("ITEMASINC Update devolvió en %v", el.Round(time.Microsecond))

	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()
	select {
	case <-prov.entered:
		t.Log("ITEMASINC el Cmd entró al proveedor")
	case <-time.After(2 * time.Second):
		t.Fatal("ITEMASINC el Cmd no llegó al proveedor")
	}

	alive := make(chan struct{})
	go func() {
		nd, _ := d2.Update(keyMsg("1"))
		_ = nd
		close(alive)
	}()
	select {
	case <-alive:
		t.Log("ITEMASINC la UI sigue respondiendo durante la descarga")
	case <-time.After(1500 * time.Millisecond):
		t.Error("ITEMASINC la UI quedó bloqueada")
	}

	close(prov.release)
	select {
	case m := <-msgCh:
		if _, ok := m.(artLoadedMsg); !ok {
			t.Fatalf("ITEMASINC mensaje inesperado: %T", m)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ITEMASINC el Cmd no terminó")
	}
}
