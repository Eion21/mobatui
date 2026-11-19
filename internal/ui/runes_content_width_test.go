package ui

import (
	"strings"
	"testing"

	"errors"
	"github.com/Eion21/mobatui/internal/staticdata"
	"github.com/charmbracelet/lipgloss"
)

// ─── ancho del CONTENIDO de Runas antes del viewport ──────────────
//
// La prueba de desbordamiento anterior medía d.View(), donde el viewport
// ya recorta: ninguna línea "desbordaba" aunque el contenido real no
// cupiera. Esta mide runesView() tal cual sale, con iconos reales, y con
// lipgloss.Width (nunca bytes: los iconos son secuencias ANSI).

// vistaRunasConIconos prepara la vista con el fixture de
// TestRunasConIconosReales: iconos de runa reales para esa página.
func vistaRunasConIconos(t *testing.T, width int) detailView {
	t.Helper()
	d := runeDetailView(t, max(width, 24), 60)
	d.width = width
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		t.Skip("sin runas en los fixtures")
	}
	ids := runePageIDs(cb)
	if len(ids) == 0 {
		t.Skip("sin ids de runa")
	}
	d.session.Provider = newRuneArtProvider(t, ids)
	d.session.Version = "16.22.1"
	d.session.InvalidateRuneArt()
	loadRuneArtSync(t, &d)
	return d
}

// anchosBarrido es TODO el rango exigible.
//
// Piso: 24 columnas. Por debajo, el viewport no puede mostrar la pestaña
// (d.vp.Width = max(24, width-4) en runeDetailView) y el contenido no
// tiene sitio físico para envolverse; no es un ancho de terminal real.
const anchoMinRunas = 24
const anchoMaxRunas = 200

// TestRunasContenidoNoExcedeElAncho barre TODOS los anchos de 24 a 200
// midiendo runesView() antes del viewport. Ninguna línea puede exceder.
func TestRunasContenidoNoExcedeElAncho(t *testing.T) {
	for w := anchoMinRunas; w <= anchoMaxRunas; w++ {
		d := vistaRunasConIconos(t, w)
		var max int
		var exceden []int
		for i, l := range strings.Split(d.runesView(), "\n") {
			lw := lipgloss.Width(l)
			if lw > max {
				max = lw
			}
			if lw > w {
				exceden = append(exceden, i)
			}
		}
		if len(exceden) > 0 {
			t.Fatalf("ancho %d: %d líneas exceden (máx %d); índices %v",
				w, len(exceden), max, exceden[:8])
		}
	}
}

// TestRunasPresenciaEnTodoElBarrido: keystone, ambos árboles,
// fragmentos, alt:, WR y partidas están SIEMPRE. La tabla esperado de la
// fase anterior solo puede subir.
func TestRunasPresenciaEnTodoElBarrido(t *testing.T) {
	for _, w := range []int{24, 30, 40, 59, 60, 89, 90, 93, 94, 99, 100, 117, 118, 160, 200} {
		d := vistaRunasConIconos(t, w)
		cb := d.CurrentBuild()
		p := cb.RunePages[0]
		out := stripANSIStr(d.runesView())

		falta := func(cond bool, que string) {
			if !cond {
				t.Errorf("ancho %d: falta %s", w, que)
			}
		}
		falta(strings.Contains(out, d.session.RuneName(p.KeystoneID)), "la keystone")
		falta(strings.Contains(out, d.session.TreeName(p.PrimaryTree)), "el árbol primario")
		falta(strings.Contains(out, d.session.TreeName(p.SecondaryTree)), "el árbol secundario")
		falta(strings.Contains(out, "fragmentos:"), "los fragmentos")
		falta(strings.Contains(out, "alt: "), "la alternativa")
		falta(strings.Contains(out, "WR "), "el win rate")
		falta(strings.Contains(out, "partidas"), "las partidas")
	}
}

// TestRunasFronteraDelLayout: el modo cambia exactamente en el umbral
// derivado, no en un número elegido a mano.
func TestRunasFronteraDelLayout(t *testing.T) {
	d := runeDetailView(t, 200, 60)
	b := d.CurrentBuild()
	if b == nil || len(b.RunePages) == 0 {
		t.Skip("sin runas")
	}
	umbral := d.runesMinWidth(b)
	if umbral <= 0 {
		t.Fatal("el umbral debe derivarse del contenido")
	}
	for _, w := range []int{umbral - 1, umbral, umbral + 1, 200} {
		d.width = w
		lay := d.runesLayout(b)
		esperaVertical := w < umbral
		if lay.Vertical != esperaVertical {
			t.Errorf("ancho %d: vertical=%v, se esperaba %v (umbral %d)",
				w, lay.Vertical, esperaVertical, umbral)
		}
	}
	// Y el umbral no es arbitrario: es el ancho real de los dos paneles.
	if got := d.runesPanelsMinWidth(b); got != umbral {
		t.Errorf("el umbral cacheado (%d) no coincide con el medido (%d)", umbral, got)
	}
}

// TestRunasColumnasUnaSolaFuente: el ancho que pinta el render es el
// mismo que pide la precarga, en toda la tabla.
func TestRunasColumnasUnaSolaFuente(t *testing.T) {
	for _, w := range []int{24, 30, 40, 59, 60, 89, 90, 93, 94, 99, 100, 117, 118, 160, 200} {
		d := vistaRunasConIconos(t, w)
		b := d.CurrentBuild()
		render := d.runeIconCols()
		precarga := d.runesLayout(b).IconCols
		if render != precarga {
			t.Errorf("ancho %d: el render pide %d columnas y la precarga %d", w, render, precarga)
		}
		// El icono pintado no puede añadir más que su ancho + el espacio.
		linea := lineaDeRuna(d.runesView(), d.session.RuneName(b.RunePages[0].KeystoneID))
		if lw := lipgloss.Width(linea); lw > w {
			t.Errorf("ancho %d: la línea del keystone mide %d", w, lw)
		}
	}
}

// lineaDeRuna devuelve la línea que contiene el nombre dado.
func lineaDeRuna(out, nombre string) string {
	for _, l := range strings.Split(stripANSIStr(out), "\n") {
		if strings.Contains(l, nombre) {
			return l
		}
	}
	return ""
}

// sinIconosProvider anula el arte: permite medir la misma línea sin
// icono para aislar su aporte de ancho.
type sinIconosProvider struct{ staticdata.Provider }

func (sinIconosProvider) RuneIcon(int) ([]byte, error) {
	return nil, errors.New("sin arte")
}
