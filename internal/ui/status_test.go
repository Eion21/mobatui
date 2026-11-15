package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/Eion21/mobatui/internal/model"
)

// TestEstadoDeDatosEnLaBarra: los datos caducados o de fixtures deben
// avisarse. Antes se servían en silencio y parecían de ahora mismo.
func TestEstadoDeDatosEnLaBarra(t *testing.T) {
	casos := []struct {
		kind  string
		age   time.Duration
		want  string
		aviso bool
	}{
		{kind: "caducado", age: 5 * time.Hour, want: "datos de hace 5 h", aviso: true},
		{kind: "fixtures", want: "fixtures locales", aviso: true},
		{kind: "caché", age: 2 * time.Hour, want: "caché de hace 2 h", aviso: true},
		{kind: "caché", age: 5 * time.Minute, aviso: false},
		{kind: "red", age: time.Second, aviso: false},
		{kind: "", aviso: false},
	}
	for _, c := range casos {
		s := &Session{Game: model.GameLoL, DataKind: c.kind, DataAge: c.age}
		line := statusLine(s)
		if !strings.Contains(line, c.want) {
			t.Errorf("%q (edad %s): la barra no dice %q → %q", c.kind, c.age, c.want, line)
		}
		tieneAviso := strings.Contains(line, "⚠")
		if tieneAviso != c.aviso {
			t.Errorf("%q (edad %s): aviso = %v, se esperaba %v → %q",
				c.kind, c.age, tieneAviso, c.aviso, line)
		}
	}
}

// TestOmitidosEnLaBarra: un héroe corrupto no tumba la carga, pero
// debe decirse cuántos se quedaron fuera.
func TestOmitidosEnLaBarra(t *testing.T) {
	s := &Session{Game: model.GameDeadlock, Omitted: 1}
	if line := statusLine(s); !strings.Contains(line, "1 omitido") {
		t.Errorf("la barra no avisa del omitido: %q", line)
	}
	s.Omitted = 3
	if line := statusLine(s); !strings.Contains(line, "3 omitidos") {
		t.Errorf("el plural no concuerda: %q", line)
	}
	s.Omitted = 0
	if line := statusLine(s); strings.Contains(line, "omitid") {
		t.Errorf("sin omitidos no debe aparecer el aviso: %q", line)
	}
}

// TestHumanAge cubre los tramos del formateo de antigüedad.
func TestHumanAge(t *testing.T) {
	for _, c := range []struct {
		in   time.Duration
		want string
	}{
		{3 * 24 * time.Hour, "3 d"},
		{5 * time.Hour, "5 h"},
		{42 * time.Minute, "42 min"},
		{10 * time.Second, "un momento"},
	} {
		if got := humanAge(c.in); got != c.want {
			t.Errorf("humanAge(%s) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
}

// TestToastNoSeBorraAntes: dos avisos seguidos en menos de 1,5 s. El
// temporizador del primero no debe borrar el segundo.
func TestToastNoSeBorraAntes(t *testing.T) {
	app := NewApp(testLoader(t))
	// Dos avisos: el segundo tiene id mayor.
	m, _ := app.Update(toastMsg{id: 1, text: "Retrato: ASCII color"})
	app = m.(App)
	m, _ = app.Update(toastMsg{id: 2, text: "Orden: Nombre"})
	app = m.(App)
	if app.toast != "Orden: Nombre" {
		t.Fatalf("toast = %q", app.toast)
	}
	// Llega el borrado del primero: no debe tocar el aviso vigente.
	m, _ = app.Update(clearToastMsg{id: 1})
	app = m.(App)
	if app.toast == "" {
		t.Error("el temporizador del primer aviso borró el segundo")
	}
	// El del segundo sí lo borra.
	m, _ = app.Update(clearToastMsg{id: 2})
	app = m.(App)
	if app.toast != "" {
		t.Errorf("el aviso vigente no se borró: %q", app.toast)
	}
}

// TestTruncatePorAnchoDeCelda: los nombres anchos deben recortarse por
// celdas, no por runas.
func TestTruncatePorAnchoDeCelda(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("no debía recortar: %q", got)
	}
	if got := truncate("Sombrero mortal de Rabadon", 10); len([]rune(got)) > 10 {
		t.Errorf("recorte de más runas: %q", got)
	}
	// Un nombre ancho chino: cada ideograma ocupa 2 celdas.
	ancho := "毁灭之王毁灭之王"
	got := truncate(ancho, 8)
	if w := runewidth.StringWidth(got); w > 8 {
		t.Errorf("%q ocupa %d celdas, se pedían 8", got, w)
	}
	if got := truncate("", 5); got != "" {
		t.Errorf("cadena vacía: %q", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Errorf("ancho 0 debía dar vacío: %q", got)
	}
}

// TestOrdenPorWilson: la lista ordena por límite inferior de Wilson,
// no por WR crudo, para no poner arriba muestras de 3 partidas.
func TestOrdenPorWilson(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	if len(s.Champions) < 2 {
		t.Skip("hacen falta al menos dos campeones")
	}
	orden := sortChampions(s.Champions, 0)
	for i := 1; i < len(orden); i++ {
		prev, cur := mainWilson(orden[i-1]), mainWilson(orden[i])
		if prev < cur {
			// Wilson descendente: solo se permite el desempate por WR.
			if prev != cur || mainWR(orden[i-1]) < mainWR(orden[i]) {
				t.Fatalf("orden no descendente en %d: %v < %v", i, prev, cur)
			}
		}
	}
}
