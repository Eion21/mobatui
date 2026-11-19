package ui

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
	"github.com/charmbracelet/lipgloss"
)

// ─── pestaña de Runas adaptable ────────────────────────────────────

// lineasVisibles parte la salida en líneas sin ANSI ni marcadores de zona.
func lineasVisibles(out string) []string {
	return strings.Split(stripANSIStr(out), "\n")
}

// bloqueRunas devuelve solo el contenido de la pestaña de Runas: la
// salida tras la línea que marca la pestaña activa ("n/7 Runas").
func bloqueRunas(out string) string {
	ls := strings.Split(out, "\n")
	for i, l := range ls {
		if strings.Contains(l, "/7 Runas") {
			return strings.Join(ls[i+1:], "\n")
		}
	}
	return out
}

// TestRunasNoDesbordaA30y40 es la prueba del rediseño: a 30 y 40 el
// contenido cabía mal (el borde cortaba el panel secundario) y a 40 la
// línea más ancha excedía en 1 columna. Ninguna línea visible puede
// superar el ancho; se mide con lipgloss.Width, nunca con bytes.
func TestRunasNoDesbordaA30y40(t *testing.T) {
	for _, w := range []int{30, 40, 59, 60, 99, 100, 117, 118, 160, 200} {
		d := runeDetailView(t, w, 60)
		if cb := d.CurrentBuild(); cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		// Se mide View(), que es lo que se ve. Se descarta la CABECERA
		// (banner + pestañas), que pinta otra función con su propio
		// ancho mínimo y desborda a 30 desde antes de esta fase: la
		// prueba no debe atribuirle ese defecto a Runas.
		vista := bloqueRunas(stripANSIStr(d.View()))
		for i, l := range lineasVisibles(vista) {
			// El prefijo de sangría del viewport no es contenido.
			if lw := lipgloss.Width(strings.TrimRight(l, " ")); lw > w {
				t.Errorf("ancho %d, línea %d: %d columnas visibles > %d", w, i, lw, w)
			}
		}
	}
}

// TestRunasArbolSecundarioEnEstrecho: a <60 el árbol secundario debe
// verse ENTERO (en 3a633af el borde lo cortaba), con todas sus runas.
func TestRunasArbolSecundarioEnEstrecho(t *testing.T) {
	for _, w := range []int{30, 40, 59} {
		d := runeDetailView(t, w, 60)
		cb := d.CurrentBuild()
		if cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		out := stripANSIStr(d.View())
		p := cb.RunePages[0]
		if !strings.Contains(out, d.session.TreeName(p.SecondaryTree)) {
			t.Errorf("ancho %d: falta el árbol secundario", w)
		}
		for _, id := range p.SecondaryIDs {
			if n := d.session.RuneName(id); n != "" && !strings.Contains(out, n) {
				t.Errorf("ancho %d: falta la runa secundaria %q", w, n)
			}
		}
	}
}

// TestRuneIconColsPorAncho fija la tabla de columnas de icono de runa:
// reutiliza iconColsFor, los mismos tramos que los ítems.
func TestRuneIconColsPorAncho(t *testing.T) {
	casos := map[int]int{0: 3, 30: 0, 59: 0, 60: 3, 99: 3, 100: 4, 117: 4, 118: 6, 200: 6}
	for w, want := range casos {
		// runeDetailView necesita un viewport mínimo; se mide solo la
		// función de columnas, que depende del ancho de la vista.
		d := runeDetailView(t, max(w, 24), 60)
		d.width = w
		if got := d.runeIconCols(); got != want {
			t.Errorf("ancho %d: columnas=%d, se esperaban %d", w, got, want)
		}
	}
}

// proveedorRunasCuenta cuenta solicitudes y sirve un PNG válido.
type proveedorRunasCuenta struct {
	staticdata.Provider
	solicitudes int
	raw         []byte
	err         error
}

func (p *proveedorRunasCuenta) RuneIcon(int) ([]byte, error) {
	p.solicitudes++
	return p.raw, p.err
}

func sesionRunasAdaptable(p staticdata.Provider) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    p,
	}
}

// TestCruceDeTramoPideArteUnaVez: cruzar 99→100 y 117→118 debe pedir el
// arte del tramo nuevo UNA vez y alimentar la clave nueva.
func TestCruceDeTramoPideArteUnaVez(t *testing.T) {
	p := &proveedorRunasCuenta{raw: pngValido(t)}
	s := sesionRunasAdaptable(p)
	const id = 8100

	for _, cols := range []int{artColsMedium, artColsLarge} {
		cmd := runeArtCmdFor(s, []int{id}, cols)
		if cmd == nil {
			t.Fatalf("cruzar a %d columnas debía pedir arte", cols)
		}
		msg := cmd().(artLoadedMsg)
		for k, a := range msg.Art {
			s.putArt(k, a)
		}
		k := artKey{Kind: artRune, ID: id, Cols: cols, Mode: artHalfBlock, Patch: "16.21"}
		if _, ok := msg.Art[k]; !ok {
			t.Errorf("la clave nueva (Cols=%d) no quedó alimentada", cols)
		}
	}
	if p.solicitudes != 2 {
		t.Errorf("solicitudes=%d, se esperaba 1 por tramo (2)", p.solicitudes)
	}
}

// TestTresAnchosRunasEnVueloUnaSolicitud: tres claves del mismo recurso
// con anchos distintos, sin resolver, contra 404 ⇒ 1 solicitud.
func TestTresAnchosRunasEnVueloUnaSolicitud(t *testing.T) {
	p := &proveedorRunasCuenta{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionRunasAdaptable(p)
	const id = 8200

	cmd := runeArtCmdFor(s, []int{id}, smallArtCols)
	if cmd == nil {
		t.Fatal("el primer ancho debía lanzar la descarga")
	}
	for _, cols := range []int{artColsMedium, artColsLarge} {
		if s.reserveArt(artKey{Kind: artRune, ID: id, Cols: cols, Mode: artHalfBlock, Patch: "16.21"}) {
			t.Errorf("el ancho %d no debía lanzar otra descarga", cols)
		}
	}
	_ = cmd()
	if p.solicitudes != 1 {
		t.Errorf("solicitudes=%d, se esperaba 1", p.solicitudes)
	}
}

// TestRespuestaTardiaNoPisaElTramoVigente: el arte de un tramo anterior
// se guarda bajo SU clave y no sobrescribe la del tramo vigente.
func TestRespuestaTardiaNoPisaElTramoVigente(t *testing.T) {
	p := &proveedorRunasCuenta{raw: pngValido(t)}
	s := sesionRunasAdaptable(p)
	const id = 8300

	viejo := artKey{Kind: artRune, ID: id, Cols: smallArtCols, Mode: artHalfBlock, Patch: "16.21"}
	nuevo := artKey{Kind: artRune, ID: id, Cols: artColsLarge, Mode: artHalfBlock, Patch: "16.21"}

	// Se pide el tramo viejo, se cambia al nuevo y solo entonces llega
	// la respuesta del viejo.
	cmd := runeArtCmdFor(s, []int{id}, smallArtCols)
	if cmd == nil {
		t.Fatal("debía pedir")
	}
	msg := cmd().(artLoadedMsg)
	for k, a := range msg.Art {
		s.putArt(k, a)
	}

	if s.RuneArtFor(id, smallArtCols) == "" {
		t.Error("el arte del tramo viejo debía quedar en SU clave")
	}
	if s.RuneArtFor(id, artColsLarge) != "" {
		t.Error("el arte viejo NO debe aparecer en el tramo vigente")
	}
	if _, ok := msg.Art[nuevo]; ok {
		t.Error("la respuesta no debe publicar la clave del tramo vigente")
	}
	_ = viejo
}

// TestRunasLayoutVerticalAEStrecho es la prueba que detecta "ignorar el
// ancho y usar siempre dos paneles": a <60 la vista debe apilarse en UNA
// columna y desde 60 ir en dos.
//
// Se necesita además de TestRunasNoDesborda porque el viewport recorta
// limpiamente: con dos paneles a ancho 30 ninguna línea "desborda" (se
// corta), pero el árbol secundario desaparece.
func TestRunasLayoutVerticalAEStrecho(t *testing.T) {
	// Cuenta los bordes superiores de panel (rutas de Runas que
	// empiezan en el mismo renglón). Dos paneles ⇒ 2 en una línea.
	maxBordes := func(out string) int {
		max := 0
		for _, l := range lineasVisibles(out) {
			if n := strings.Count(l, "╭"); n > max {
				max = n
			}
		}
		return max
	}
	for _, w := range []int{30, 40, 59} {
		d := runeDetailView(t, w, 60)
		if cb := d.CurrentBuild(); cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		if n := maxBordes(stripANSIStr(d.View())); n > 1 {
			t.Errorf("ancho %d: una sola columna, se vieron %d paneles", w, n)
		}
	}
	for _, w := range []int{60, 100, 160} {
		d := runeDetailView(t, w, 60)
		if cb := d.CurrentBuild(); cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		if n := maxBordes(stripANSIStr(d.View())); n < 2 {
			t.Errorf("ancho %d: debían verse 2 paneles, se vieron %d", w, n)
		}
	}
}
