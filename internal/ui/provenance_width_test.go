package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/model"
)

// ─── validación en terminales estrechas ─────────────────────────────
//
// Comprueba que la información de parches y de incertidumbre no
// desborda ni se corta a 40, 60, 80 y 118 columnas, y que el nombre del
// campeón nunca se pierde.

// anchosDePrueba son los anchos obligatorios de la fase.
var anchosDePrueba = []int{40, 60, 80, 118}

// alturasDePrueba cubre una terminal muy baja (donde entra en juego la
// cabecera compacta), una media y una alta.
var alturasDePrueba = []int{16, 24, 40, 60}

// TestAnchosObligatoriosSinDesborde recorre anchos × alturas y valida
// que ninguna línea visible exceda el ancho y que la cabecera conserve
// el nombre del campeón.
func TestAnchosObligatoriosSinDesborde(t *testing.T) {
	for _, w := range anchosDePrueba {
		for _, h := range alturasDePrueba {
			d := planDetailView(t, "briar", "Briar", w, h)
			d.session.Version = "16.23.1"
			for _, tab := range []int{0, 1, 2} {
				d.tab = tab
				d.sync()
				vista := stripANSIStr(d.View())
				for _, linea := range strings.Split(vista, "\n") {
					if got := lipgloss.Width(linea); got > w {
						t.Errorf("%dx%d pestaña %d: línea de %d celdas > %d: %q",
							w, h, tab, got, w, collapsePlain(linea))
					}
				}
			}
		}
	}
}

// TestNombreDelCampeonNuncaSePierde: en cualquier ancho, el nombre sigue
// visible en la cabecera.
func TestNombreDelCampeonNuncaSePierde(t *testing.T) {
	for _, w := range anchosDePrueba {
		d := planDetailView(t, "briar", "Briar", w, 40)
		d.session.Version = "16.23.1"
		cab := stripANSIStr(d.headerBlock())
		if !strings.Contains(cab, "Briar") {
			t.Errorf("ancho %d: la cabecera perdió el nombre: %q", w, collapsePlain(cab))
		}
	}
}

// TestParcheDeDatosSiempreVisible: cuando solo cabe un parche, el que se
// muestra es el de las estadísticas (el que el jugador está leyendo).
func TestParcheDeDatosSiempreVisible(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 40, 40)
	d.session.Version = "16.23.1"
	stats := normalizePatch(d.CurrentBuild().Patch)
	cab := stripANSIStr(d.headerBlock())
	cab = collapsePlain(cab)
	if !strings.Contains(cab, "Datos "+stats) {
		t.Errorf("a 40 columnas falta el parche de datos %q: %q", stats, cab)
	}
	// El de catálogo puede no caber, pero si aparece debe ir etiquetado.
	if strings.Contains(cab, "16.23.1") && !strings.Contains(cab, "Catálogo") {
		t.Errorf("el catálogo aparece sin etiqueta: %q", cab)
	}
}

// TestAnchoCompactoNoDesbordaElPie: la barra de estado también respeta
// el ancho en terminales estrechas.
func TestAnchoCompactoNoDesbordaElPie(t *testing.T) {
	s := &Session{Game: model.GameLoL, Version: "16.23.1"}
	linea := statusLine(s)
	// La etiqueta corta debe mantenerse corta: el pie ya lleva cuatro
	// campos y en 40 columnas no cabe una frase larga.
	if strings.Contains(linea, "Catálogo ") {
		t.Errorf("el pie usa la etiqueta larga: %q", linea)
	}
	if !strings.Contains(linea, "16.23.1") {
		t.Errorf("el pie perdió la versión: %q", linea)
	}
}

// TestCambioDePestanaConservaSeleccion: cambiar de ancho no debe perder
// la pestaña activa ni la selección de tarjeta.
func TestCambioDeAnchoConservaSeleccion(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 2
	d.sync()
	if d.itemIdx > 0 {
		d.itemIdx = 1
	}
	antesTab, antesIdx := d.tab, d.itemIdx
	d.width = 60
	d.height = 24
	d.vp.Width = max(24, d.width-4)
	d.vp.Height = d.height
	d.sync()
	if d.tab != antesTab {
		t.Errorf("la pestaña cambió: %d → %d", antesTab, d.tab)
	}
	if d.itemIdx != antesIdx {
		t.Errorf("la selección cambió: %d → %d", antesIdx, d.itemIdx)
	}
}

// TestCabeceraCompactaNoDesborda: la cabecera de una línea respeta el
// ancho y no trunca el nombre.
func TestCabeceraCompactaNoDesborda(t *testing.T) {
	for _, w := range []int{40, 60, 80, 118} {
		d := planDetailView(t, "briar", "Briar", w, 16)
		d.session.Version = "16.23.1"
		cab := stripANSIStr(d.compactHeader())
		if got := lipgloss.Width(cab); got > w {
			t.Errorf("compactHeader a %d = %d celdas: %q", w, got, cab)
		}
		if !strings.Contains(cab, "Briar") {
			t.Errorf("compactHeader a %d perdió el nombre: %q", w, cab)
		}
	}
}

// TestEtiquetasDeIncertidumbreSobrevivenAlEnvolver: a 40 columnas las
// líneas de Ruta se parten por palabras dentro del viewport. La
// etiqueta de muestra tiene que SEGUIR presente en el texto visible (no
// puede quedar cortada ni perdida al envolver).
func TestEtiquetasDeIncertidumbreSobrevivenAlEnvolver(t *testing.T) {
	for _, w := range anchosDePrueba {
		d := planDetailView(t, "briar", "Briar", w, 60)
		d.session.Version = "16.23.1"
		d.tab = 1
		d.sync()
		// Texto completo de la pestaña, ya envuelto por el viewport.
		visible := collapsePlain(stripANSIStr(d.View()))
		plan := d.lolPlan()
		var huboEtiqueta bool
		for _, st := range plan.Steps {
			label := sampleLabel(st.Stats.Games)
			if label == "" {
				continue
			}
			huboEtiqueta = true
			if !strings.Contains(visible, label) {
				t.Errorf("ancho %d: la etiqueta %q se perdió al envolver", w, label)
			}
		}
		if !huboEtiqueta {
			t.Skip("el fixture no tiene pasos con muestra limitada")
		}
	}
}
