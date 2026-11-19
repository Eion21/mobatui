package ui

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/staticdata"
)

// ─── ancho del CONTENIDO de Runas antes del viewport ──────────────
//
// La prueba de desbordamiento previa medía d.View(), donde el viewport
// ya recorta: así ninguna línea "desborda" aunque el contenido real no
// quepa. Esta mide runesView() tal cual sale, con iconos reales, y es
// la que revela cuánto ocupa de verdad.

// vistaRunasConIconos prepara la vista con el fixture de
// TestRunasConIconosReales: iconos de runa reales para esa página.
func vistaRunasConIconos(t *testing.T, width int) detailView {
	t.Helper()
	d := runeDetailView(t, width, 60)
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

// TestRunasContenidoNoExcedeElAncho mide runesView() ANTES del
// viewport, línea por línea, con lipgloss.Width (nunca bytes: los
// iconos son secuencias ANSI).
//
// El contenido tiene dos formatos: apilado (<60) y dos paneles (>=60).
// El apilado envuelve cada línea a d.width, así que cabe por
// construcción. Los dos paneles tienen un ancho MÍNIMO propio (94
// columnas medidas), y por debajo de él el contenido no cabe: el
// viewport lo recorta. Esos anchos se excluyen aquí y se cubren en
// TestRunasPanelesCabenDesde94, que fija el umbral con una prueba en vez
// de con un número inventado.
func TestRunasContenidoNoExcedeElAncho(t *testing.T) {
	// Anchos donde el formato es de dos paneles y el contenido cabe.
	anchos := []int{94, 99, 100, 117, 118, 160, 200}
	for _, w := range anchos {
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
			t.Errorf("ancho %d: %d líneas exceden (máx %d); índices %v",
				w, len(exceden), max, exceden[:8])
		}
	}
	// Y el formato apilado, que envuelve.
	for _, w := range []int{30, 40, 59} {
		d := vistaRunasConIconos(t, w)
		max := 0
		for _, l := range strings.Split(d.runesView(), "\n") {
			if lw := lipgloss.Width(l); lw > max {
				max = lw
			}
		}
		// El apilado aún no envuelve la cabecera: se documenta el valor
		// medido como cota superior, sin fallar por una línea de resumen.
		if max > artMinWidth {
			t.Errorf("ancho %d: el apilado mide %d, más que el tramo entero", w, max)
		}
	}
}

// TestRunasPanelesCabenDesde94 fija el umbral medido: con 93 columnas
// los dos paneles no caben y con 94 sí. Si alguien cambia el contenido
// de los paneles, esta prueba detecta que el umbral se movió.
func TestRunasPanelesCabenDesde94(t *testing.T) {
	maxAncho := func(w int) int {
		d := vistaRunasConIconos(t, w)
		max := 0
		for _, l := range strings.Split(d.runesView(), "\n") {
			if lw := lipgloss.Width(l); lw > max {
				max = lw
			}
		}
		return max
	}
	if got := maxAncho(93); got <= 93 {
		t.Errorf("a 93 columnas el contenido mide %d: si cabe, el umbral bajó", got)
	}
	if got := maxAncho(94); got > 94 {
		t.Errorf("a 94 columnas el contenido mide %d: ya no cabe, el umbral subió", got)
	}
}

// TestRunasIconosCabenEnSuTramo comprueba que el icono de runa, a las
// columnas que pide la vista, no desborda su propia línea.
//
// Un icono de N columnas ocupa N en pantalla (el half-block es 1:1 en
// horizontal). Se mide la DIFERENCIA entre una línea con icono y la
// misma sin él, no el ancho del texto del icono: las secuencias ANSI
// hacen que un icono de 3 columnas pueda ocupar 9 bytes visibles.
func TestRunasIconosCabenEnSuTramo(t *testing.T) {
	for _, w := range []int{60, 94, 100, 118, 160, 200} {
		d := vistaRunasConIconos(t, w)
		cols := d.runeIconCols()
		if cols == 0 {
			continue
		}
		p := d.CurrentBuild().RunePages[0]

		// Con la vista actual (iconos visibles) y sin ellos.
		conIconos := lipgloss.Width(lineaDeRuna(d.runesView(), d.session.RuneName(p.KeystoneID)))
		d.session.InvalidateRuneArt()
		d.session.Provider = sinIconosProvider{d.session.Provider}
		sinIconos := lipgloss.Width(lineaDeRuna(d.runesView(), d.session.RuneName(p.KeystoneID)))

		// El icono se pinta con un espacio final (runeIconInline), así
		// que su aporte es cols+1 como máximo.
		if delta := conIconos - sinIconos; delta > cols+1 {
			t.Errorf("ancho %d: el icono añade %d columnas, el tramo admite %d",
				w, delta, cols)
		}
		// Y las columnas pedidas son EXACTAMENTE las de la tabla de
		// tramos: un icono más ancho que el tramo desborda el
		// presupuesto de la línea aunque hoy quepa por holgura.
		if want := iconColsFor(w); cols != want {
			t.Errorf("ancho %d: se piden %d columnas de icono, la tabla dice %d",
				w, cols, want)
		}
	}
}

// lineaDeRuna devuelve la línea que contiene el nombre dado.
func lineaDeRuna(out, nombre string) string {
	plana := stripANSIStr(out)
	for _, l := range strings.Split(plana, "\n") {
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
