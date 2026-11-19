package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/staticdata"
)

type blockingProvider struct {
	staticdata.Provider
	release chan struct{}
	entered chan struct{}
}

func (p *blockingProvider) RuneIcon(int) ([]byte, error) {
	select {
	case p.entered <- struct{}{}:
	default:
	}
	<-p.release
	return nil, errors.New("liberado")
}

// TestRuneArtNoBloqueaElBucle: el flujo real de Bubble Tea es Update →
// Cmd en otra goroutine. Con el CDN lento, la interfaz debe seguir viva.
//
// Regresión: ensureRuneArt se llamaba dentro de Update y hacía la
// petición HTTP de forma síncrona; pulsar "5" congelaba la TUI hasta
// que respondía la red.
func TestRuneArtNoBloqueaElBucle(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		t.Skip("sin runas")
	}
	prov := &blockingProvider{
		Provider: d.session.Provider,
		release:  make(chan struct{}),
		entered:  make(chan struct{}, 1),
	}
	d.session.Provider = prov
	d.session.InvalidateRuneArt()
	d.tab = 0

	// 1) Update NO debe bloquear.
	start := time.Now()
	d2, cmd := d.Update(keyMsg("5"))
	el := time.Since(start)
	if el > 200*time.Millisecond {
		t.Errorf("ASINC Update tardó %v: bloqueó esperando el CDN", el)
	}
	if cmd == nil {
		t.Fatal("ASINC debía devolver un Cmd")
	}
	t.Logf("ASINC Update devolvió en %v con Cmd", el.Round(time.Microsecond))

	// 2) El Cmd (lanzado por el runtime) es quien espera.
	msgCh := make(chan tea.Msg, 1)
	go func() { msgCh <- cmd() }()

	select {
	case <-prov.entered:
		t.Log("ASINC el Cmd entró al proveedor")
	case <-time.After(2 * time.Second):
		t.Fatal("ASINC el Cmd no llegó al proveedor")
	}

	// 3) Mientras espera, el bucle sigue vivo: cambiar de pestaña.
	alive := make(chan struct{})
	go func() {
		nd, _ := d2.Update(keyMsg("1"))
		_ = nd
		close(alive)
	}()
	select {
	case <-alive:
		t.Log("ASINC la UI sigue respondiendo durante la descarga")
	case <-time.After(1500 * time.Millisecond):
		t.Error("ASINC la UI quedó bloqueada")
	}

	// 4) View() no debe bloquear ni descargar.
	viewDone := make(chan struct{})
	go func() { _ = d2.View(); close(viewDone) }()
	select {
	case <-viewDone:
		t.Log("ASINC View() no bloqueó")
	case <-time.After(time.Second):
		t.Error("ASINC View() quedó bloqueada")
	}

	// 5) Los nombres siguen visibles sin iconos.
	if out := stripANSIStr(d2.runesView()); out == "" {
		t.Error("ASINC la vista de Runas quedó vacía")
	}

	// 6) Recoge el resultado al liberar.
	close(prov.release)
	select {
	case m := <-msgCh:
		loaded, ok := m.(artLoadedMsg)
		if !ok {
			t.Fatalf("BLOCK mensaje inesperado: %T", m)
		}
		t.Logf("ASINC resultado: %d iconos", len(loaded.Art))
	case <-time.After(3 * time.Second):
		t.Fatal("ASINC el Cmd no terminó")
	}
}

// Una respuesta tardía no debe pintar iconos de un parche anterior.
// TestRuneArtDeOtroParcheNoSeUsa: una respuesta tardía se guarda bajo
// su clave (que incluye parche), así que no pinta iconos de un parche
// anterior.
func TestRuneArtDeOtroParcheNoSeUsa(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	cb := d.CurrentBuild()
	if len(cb.RunePages) == 0 {
		t.Skip("sin runas")
	}
	d.session.InvalidateRuneArt()

	// Se simula una respuesta que llega con el parche VIEJO en la clave.
	keyViejo := artKey{ID: 8112, Cols: runeArtColsPorDefecto, Mode: artHalfBlock, Patch: "16.22.1"}
	d.session.Version = "16.23.1" // el parche cambió mientras se descargaba

	d.session.putArt(keyViejo, "arte-del-parche-viejo")

	if got := d.session.RuneArtFor(8112, runeArtColsPorDefecto); got != "" {
		t.Errorf("PARCHE se sirvió arte de otro parche: %q", got)
	}
	t.Log("PARChE el arte del parche viejo no se usa con el nuevo")
}
