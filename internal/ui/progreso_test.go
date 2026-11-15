package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/model"
)

// TestBarraDeProgresoEnLaCarga: la pantalla de carga debe mostrar el
// avance y el nombre del campeón que se está resolviendo. Antes solo
// había un spinner sin información durante 1-2 minutos.
func TestBarraDeProgresoEnLaCarga(t *testing.T) {
	app := NewApp(testLoader(t))
	app.screen = screenLoading
	app.status = "cargando League of Legends…"

	// Sin total conocido no se pinta barra (no debe mentir).
	if strings.Contains(app.View(), "░") {
		t.Error("sin total no debía haber barra")
	}

	app.done, app.total = 62, 200
	app.status = "62/200 · Aurelion Sol"
	v := app.View()
	if !strings.Contains(v, "62/200") {
		t.Error("la pantalla de carga no muestra el contador")
	}
	if !strings.Contains(v, "Aurelion Sol") {
		t.Error("la pantalla de carga no muestra el campeón en curso")
	}
	if !strings.Contains(v, "31%") {
		t.Error("la pantalla de carga no muestra el porcentaje")
	}
	if !strings.Contains(v, "█") || !strings.Contains(v, "░") {
		t.Error("la barra no tiene relleno y hueco")
	}
}

// TestProgresoActualizaElEstado: el mensaje de progreso refresca la
// barra de estado de la pantalla de carga.
func TestProgresoActualizaElEstado(t *testing.T) {
	app := NewApp(testLoader(t))
	app.screen = screenLoading
	app.progress = make(progressCh, 4)

	m, _ := app.Update(progressMsg{Done: 124, Total: 200, Name: "Ahri · middle"})
	app = m.(App)
	if app.done != 124 || app.total != 200 {
		t.Fatalf("progreso = %d/%d", app.done, app.total)
	}
	if !strings.Contains(app.status, "124/200") || !strings.Contains(app.status, "Ahri") {
		t.Errorf("status = %q", app.status)
	}
}

// TestProgresoNoRompeLaLista: si llega un aviso tardío con la lista ya
// cargada, no debe pisar la barra de estado con datos de carga.
func TestProgresoNoRompeLaLista(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	app := NewApp(testLoader(t))
	m, _ := app.Update(sessionLoadedMsg{game: model.GameLoL, session: s})
	app = m.(App)
	antes := app.status

	m, _ = app.Update(progressMsg{Done: 3, Total: 10, Name: "tarde"})
	app = m.(App)
	if app.status != antes {
		t.Errorf("un aviso tardío pisó el estado de la lista: %q", app.status)
	}
}

// TestProgresoSeRearma: tras cada aviso hay que volver a escuchar el
// canal; si no, solo llegaría el primero.
func TestProgresoSeRearma(t *testing.T) {
	app := NewApp(testLoader(t))
	app.screen = screenLoading
	app.progress = make(progressCh, 4)

	_, cmd := app.Update(progressMsg{Done: 1, Total: 2, Name: "uno"})
	if cmd == nil {
		t.Fatal("no se rearmó la escucha del progreso")
	}
}

// TestErrorDeCargaPermiteSalir: con un fallo, esc vuelve al selector y
// r reintenta (la regresión del informe original).
func TestErrorDeCargaPermiteSalir(t *testing.T) {
	app := NewApp(testLoader(t))
	app.screen = screenLoading
	m, _ := app.Update(sessionLoadedMsg{game: model.GameLoL, err: errTestCarga})
	app = m.(App)
	if app.errMsg == "" || app.failedGame != model.GameLoL {
		t.Fatalf("el error no se registró: %q / %q", app.errMsg, app.failedGame)
	}
	if v := app.View(); !strings.Contains(v, "esc vuelve") {
		t.Error("la pantalla de error no indica la salida")
	}
	m, _ = app.Update(keyMsg("esc"))
	app = m.(App)
	if app.screen != screenPick || app.errMsg != "" {
		t.Errorf("esc no volvió al selector: pantalla %v, err %q", app.screen, app.errMsg)
	}
}

type errCarga struct{}

func (errCarga) Error() string { return "sin datos de la fuente ni caché" }

var errTestCarga = errCarga{}

// TestEscEnCargaSinErrorNoSale: esc no debe abortar una carga en curso
// ni salir sin querer; q sí.
func TestEscEnCargaSinErrorNoSale(t *testing.T) {
	app := NewApp(testLoader(t))
	app.screen = screenLoading
	app.status = "cargando…"
	m, cmd := app.Update(keyMsg("esc"))
	app = m.(App)
	if app.screen != screenLoading {
		t.Errorf("esc cambió de pantalla durante una carga sana: %v", app.screen)
	}
	if cmd != nil {
		t.Error("esc no debía lanzar ningún comando")
	}
}

var _ = tea.KeyMsg{}
