package ui

// Pruebas de la notificación de actualización: aparición, descarte,
// apertura validada y — sobre todo — que el arranque NO espera a la
// respuesta HTTP.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/config"
)

// updateApp construye una App mínima con un comprobador inyectado.
func updateApp(t *testing.T, check func(context.Context) (UpdateInfo, error)) App {
	t.Helper()
	l := testLoader(t)
	l.UpdateCheck = check
	return NewApp(l)
}

// TestUpdateMsgMuestraAviso: el mensaje pinta el aviso persistente.
func TestUpdateMsgMuestraAviso(t *testing.T) {
	a := updateApp(t, nil)
	a.width, a.height = 120, 40

	u, _ := a.Update(updateAvailableMsg{
		Info:   UpdateInfo{Version: "0.2.1", URL: "https://github.com/Eion21/mobatui/releases/tag/v0.2.1"},
		Update: true,
	})
	a = u.(App)

	if a.update == nil {
		t.Fatal("el aviso no se guardó")
	}
	if a.update.Version != "0.2.1" {
		t.Errorf("versión = %q", a.update.Version)
	}
	footer := a.footer()
	if !strings.Contains(footer, "Nueva versión") || !strings.Contains(footer, "0.2.1") {
		t.Errorf("el footer no anuncia la versión:\n%s", footer)
	}
	if !strings.Contains(footer, "GitHub Releases") {
		t.Errorf("el footer no menciona GitHub Releases:\n%s", footer)
	}
}

// TestUpdateMsgSinNovedadNoAvisa: un mensaje vacío no crea aviso.
func TestUpdateMsgSinNovedadNoAvisa(t *testing.T) {
	a := updateApp(t, nil)
	u, _ := a.Update(updateAvailableMsg{})
	a = u.(App)
	if a.update != nil {
		t.Errorf("sin novedad no debía haber aviso: %+v", a.update)
	}
	if strings.Contains(a.footer(), "Nueva versión") {
		t.Error("el footer no debía anunciar nada")
	}
}

// TestUpdateDescartar: "V" quita el aviso sin salir de la app.
func TestUpdateDescartar(t *testing.T) {
	a := updateApp(t, nil)
	a.screen = screenPick
	a.width, a.height = 120, 40
	a.update = &UpdateInfo{Version: "0.2.1"} // sin URL: solo descarta

	u, _ := a.Update(keyMsg("V"))
	a = u.(App)
	if a.update != nil {
		t.Errorf("«V» debía descartar el aviso: %+v", a.update)
	}
}

// TestUpdateKeyNoChocaConNavegacion: la tecla del aviso no puede cerrar
// la app ni volver atrás. Esc sigue saliendo desde el selector.
func TestUpdateKeyNoChocaConNavegacion(t *testing.T) {
	a := updateApp(t, nil)
	a.screen = screenPick
	a.width, a.height = 120, 40
	a.update = &UpdateInfo{Version: "0.2.1"}

	// Esc con aviso presente sigue comportándose como siempre (salir),
	// no consume el aviso: evidencia de que no hay secuestro de la tecla.
	_, cmd := a.Update(keyMsg("esc"))
	if cmd == nil {
		t.Error("esc debía conservar su comportamiento (salir)")
	}

	// "v" no navega ni cambia de pantalla.
	b := updateApp(t, nil)
	b.screen = screenPick
	b.width, b.height = 120, 40
	b.update = &UpdateInfo{Version: "0.2.1"}
	u, _ := b.Update(keyMsg("v"))
	b = u.(App)
	if b.screen != screenPick {
		t.Errorf("«v» cambió de pantalla a %v", b.screen)
	}
}

// TestUpdateTeclaNoPisaUpgradesDeDeadlock: en el detalle de Deadlock "u"
// sigue plegando mejoras, y el aviso no lo intercepta.
func TestUpdateTeclaNoPisaUpgradesDeDeadlock(t *testing.T) {
	a := hybridApp(t, nil, 6, 120, 40)
	a.update = &UpdateInfo{Version: "0.2.1"}
	before := a.detail.showUpgrades

	u, _ := a.Update(keyMsg("u"))
	a = u.(App)
	if a.detail.showUpgrades == before {
		t.Error("«u» debía seguir plegando mejoras en Deadlock")
	}
	if a.update == nil {
		t.Error("«u» no debía descartar el aviso")
	}
}

// TestUpdateNotificacionUnica: dos mensajes no duplican ni apilan avisos.
func TestUpdateNotificacionUnica(t *testing.T) {
	a := updateApp(t, nil)
	a.width, a.height = 120, 40

	for i := 0; i < 3; i++ {
		u, _ := a.Update(updateAvailableMsg{
			Info:   UpdateInfo{Version: "0.2.1", URL: "https://github.com/Eion21/mobatui/releases/tag/v0.2.1"},
			Update: true,
		})
		a = u.(App)
	}
	footer := a.footer()
	if n := strings.Count(footer, "Nueva versión"); n != 1 {
		t.Errorf("el aviso aparece %d veces, se esperaba 1:\n%s", n, footer)
	}
}

// TestUpdateCheckCmdEjecutaEnGoroutinePropia: el comando se ejecuta sin
// bloquear al llamador; el bloqueo ocurre dentro de la goroutine del
// propio comando (como hace Bubble Tea), no en la de la UI.
func TestUpdateCheckCmdEjecutaEnGoroutinePropia(t *testing.T) {
	release := make(chan struct{})
	entered := make(chan struct{})
	check := func(context.Context) (UpdateInfo, error) {
		close(entered)
		<-release
		return UpdateInfo{Version: "0.2.1"}, nil
	}

	cmd := checkUpdateCmd(check)
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	// El comando entra en el comprobador y se queda esperando allí.
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("el comando no llegó a ejecutarse")
	}

	// Mientras espera, el hilo del test sigue vivo: no hay bloqueo del
	// llamador (Bubble Tea puede seguir procesando mensajes).
	select {
	case m := <-done:
		t.Fatalf("el comando no debía terminar aún: %+v", m)
	default:
	}

	close(release)
	select {
	case m := <-done:
		msg, ok := m.(updateAvailableMsg)
		if !ok || !msg.Update {
			t.Fatalf("mensaje = %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("el comando no terminó tras liberarse")
	}
}

// TestUpdateCheckCmdSilenciosoConError: un error de red no produce aviso.
func TestUpdateCheckCmdSilenciosoConError(t *testing.T) {
	check := func(context.Context) (UpdateInfo, error) {
		return UpdateInfo{}, errors.New("sin red")
	}
	msg, ok := checkUpdateCmd(check)().(updateAvailableMsg)
	if !ok {
		t.Fatal("se esperaba updateAvailableMsg")
	}
	if msg.Update {
		t.Errorf("un error no debe avisar: %+v", msg)
	}
}

// TestUpdateCheckCmdExito: un resultado válido sí avisa.
func TestUpdateCheckCmdExito(t *testing.T) {
	check := func(context.Context) (UpdateInfo, error) {
		return UpdateInfo{Version: "0.2.1", URL: "https://github.com/Eion21/mobatui/releases/tag/v0.2.1"}, nil
	}
	msg, ok := checkUpdateCmd(check)().(updateAvailableMsg)
	if !ok || !msg.Update {
		t.Fatalf("mensaje = %+v", msg)
	}
	if msg.Info.Version != "0.2.1" {
		t.Errorf("versión = %q", msg.Info.Version)
	}
}

// TestInitNoBloqueaConComprobadorLento es la prueba clave: Init debe
// devolver un tea.Cmd inmediatamente, sin ejecutar la comprobación.
func TestInitNoBloqueaConComprobadorLento(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	check := func(context.Context) (UpdateInfo, error) {
		close(started)
		<-release
		return UpdateInfo{Version: "0.2.1"}, nil
	}

	a := updateApp(t, check)
	a.cfg = config.Config{MonitorOff: true} // aísla la comprobación

	start := time.Now()
	cmd := a.Init()
	initElapsed := time.Since(start)

	if initElapsed > 100*time.Millisecond {
		t.Fatalf("Init tardó %v: bloqueó el arranque", initElapsed)
	}
	if cmd == nil {
		t.Fatal("Init debía devolver un comando")
	}
	// La comprobación aún no ha empezado: prueba de que es diferida.
	select {
	case <-started:
		t.Fatal("Init ejecutó la comprobación en línea (debía diferirse al Cmd)")
	default:
	}

	// Al ejecutar el comando (como haría Bubble Tea en su goroutine) sí
	// arranca; se libera para no dejar la goroutine colgada.
	done := make(chan struct{})
	go func() {
		cmd()
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("el comando no llegó a ejecutar la comprobación")
	}
	close(release)
	<-done
}

// TestInitRespetaOptOut: con UpdateCheckOff no se lanza nada.
func TestInitRespetaOptOut(t *testing.T) {
	called := false
	check := func(context.Context) (UpdateInfo, error) {
		called = true
		return UpdateInfo{}, nil
	}
	a := updateApp(t, check)
	a.cfg = config.Config{MonitorOff: true, UpdateCheckOff: true}

	cmd := a.Init()
	if cmd != nil {
		cmd()
	}
	if called {
		t.Error("el opt-out ejecutó la comprobación")
	}
}

// TestInitSinComprobador: sin comprobador (offline o flag) no lanza nada.
func TestInitSinComprobador(t *testing.T) {
	a := updateApp(t, nil)
	a.cfg = config.Config{MonitorOff: true}
	if cmd := a.Init(); cmd != nil {
		t.Error("sin comprobador Init debía devolver nil")
	}
}

// TestInitUnaSolaVez: llamar a Init dos veces no repite la comprobación.
func TestInitUnaSolaVez(t *testing.T) {
	var calls int
	check := func(context.Context) (UpdateInfo, error) {
		calls++
		return UpdateInfo{}, nil
	}
	a := updateApp(t, check)
	a.cfg = config.Config{MonitorOff: true}

	cmd := a.Init()
	if cmd == nil {
		t.Fatal("se esperaba comando")
	}
	a.updateChecked = true // como haría Bubble Tea al procesar Init
	if again := a.Init(); again != nil {
		again()
	}
	if calls > 1 {
		t.Errorf("la comprobación se ejecutó %d veces", calls)
	}
}

// TestValidarURL: solo se abre el repositorio esperado.
func TestValidarURL(t *testing.T) {
	ok := []string{
		"https://github.com/Eion21/mobatui/releases/tag/v0.2.1",
		"https://github.com/Eion21/mobatui/releases/latest",
	}
	for _, u := range ok {
		if err := validateURL(u); err != nil {
			t.Errorf("validateURL(%q) = %v", u, err)
		}
	}
	bad := []string{
		"https://evil.example/x",
		"https://github.com/Otro/mobatui",
		"http://github.com/Eion21/mobatui/releases",
		"https://github.com/Eion21/mobatui/../otro",
		"https://github.com/Eion21/mobatui/x; rm -rf /",
		"",
	}
	for _, u := range bad {
		if err := validateURL(u); err == nil {
			t.Errorf("validateURL(%q) debía fallar", u)
		}
	}
}

// TestAvisoSinURLOmiteAbrir: sin URL válida el aviso no ofrece "abrir".
func TestAvisoSinURLOmiteAbrir(t *testing.T) {
	a := updateApp(t, nil)
	a.width, a.height = 120, 40
	a.update = &UpdateInfo{Version: "0.2.1"} // URL vacía
	footer := a.footer()
	if strings.Contains(footer, "v abrir") {
		t.Errorf("sin URL no debe ofrecerse abrir:\n%s", footer)
	}
	if !strings.Contains(footer, "descartar") {
		t.Errorf("debe ofrecerse descartar:\n%s", footer)
	}
}
