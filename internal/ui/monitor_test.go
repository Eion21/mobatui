package ui

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/procmon"
)

// TestEventoPersonajeEnPie: el evento CHARACTER_CHANGED del monitor se
// muestra en el pie del selector, y el cierre del juego borra tanto el
// juego como el personaje.
func TestEventoPersonajeEnPie(t *testing.T) {
	app := NewApp(testLoader(t))

	m, _ := app.Update(monEventMsg{procmon.Event{Type: procmon.Started, Game: procmon.LOL}})
	app = m.(App)
	m, _ = app.Update(monEventMsg{procmon.Event{Type: procmon.Char, Game: procmon.LOL, Character: "Ahri"}})
	app = m.(App)

	if app.detectedChamp != "Ahri" {
		t.Fatalf("detectedChamp = %q, se esperaba %q", app.detectedChamp, "Ahri")
	}
	if view := app.View(); !strings.Contains(view, "Ahri") {
		t.Errorf("el pie no muestra el personaje detectado: %q", view)
	}

	// El cierre del juego limpia juego y personaje.
	m, _ = app.Update(monEventMsg{procmon.Event{Type: procmon.Ended, Game: procmon.LOL}})
	app = m.(App)
	if app.detected != "" || app.detectedChamp != "" {
		t.Errorf("tras Ended: detected=%q detectedChamp=%q, se esperaba todo vacío",
			app.detected, app.detectedChamp)
	}
	if view := app.View(); strings.Contains(view, "detectado") {
		t.Errorf("tras Ended el pie sigue mostrando detección: %q", view)
	}
}

// TestEventoPersonajeSinCambio: un evento de personaje sin juego
// activo se descarta — el personaje viaja siempre detrás del Started
// de su juego y sin él no se guarda ni se muestra.
func TestEventoPersonajeSinCambio(t *testing.T) {
	app := NewApp(testLoader(t))
	m, _ := app.Update(monEventMsg{procmon.Event{Type: procmon.Char, Game: procmon.LOL, Character: "Ahri"}})
	app = m.(App)
	if app.detectedChamp != "" {
		t.Fatalf("detectedChamp = %q sin juego detectado, se esperaba vacío", app.detectedChamp)
	}
	if view := app.View(); strings.Contains(view, "Ahri") {
		t.Errorf("el pie muestra personaje sin juego detectado: %q", view)
	}
}
