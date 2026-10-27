package ui

import (
	"fmt"
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/builds-tui/internal/model"
)

// TestDiagnosticoVistaLista vuelca la vista completa de la lista y el
// detalle para inspección manual. Actívalo con BUILDS_DIAG=1 go test
// ./internal/ui/ -run Diagnostico.
func TestDiagnosticoVistaLista(t *testing.T) {
	if os.Getenv("BUILDS_DIAG") == "" {
		t.Skip("solo con BUILDS_DIAG=1")
	}
	gameID := model.GameLoL
	if os.Getenv("BUILDS_DIAG") == "deadlock" {
		gameID = model.GameDeadlock
	}
	app := NewApp(testLoader(t))
	s := loadOffline(t, gameID)
	m, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	app = m.(App)
	m, _ = app.Update(sessionLoadedMsg{session: s})
	app = m.(App)
	fmt.Println("──── lista ────")
	fmt.Println(app.View())
	m, _ = app.Update(keyMsg("enter"))
	app = m.(App)
	fmt.Println("──── detalle ────")
	fmt.Println(app.View())
}
