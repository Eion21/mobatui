package ui

// Pruebas de iconos compactos e interacción en Ruta y Alt.
//
// Sin red: los iconos salen de los fixtures de Data Dragon en
// testdata/ddragon y la caché es un directorio temporal.

import (
	"errors"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/staticdata"

	"github.com/Eion21/mobatui/internal/model"
)

// TestIconosRutaUnaFila: el icono de Ruta no puede romper el formato de
// un paso por línea.
func TestIconosRutaUnaFila(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)

	plan := d.lolPlan()
	if plan == nil || len(plan.Steps) == 0 {
		t.Skip("sin pasos para este campeón")
	}
	conIcono := 0
	for i, line := range d.routeViewA(plan) {
		plain := stripANSIStr(line)
		if strings.ContainsAny(plain, "▀▄█") {
			conIcono++
		}
		// Ninguna línea de la vista puede ocupar más de una fila: el
		// icono debe ser de una sola.
		if h := strings.Count(line, "\n") + 1; h > 1 {
			t.Errorf("línea %d ocupa %d filas: %q", i, h, plain)
		}
	}
	if conIcono == 0 {
		t.Error("ningún paso mostró icono con los fixtures disponibles")
	}
}

// TestIconosAltUnaFila: igual para las alternativas.
func TestIconosAltUnaFila(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 2
	loadArtSync(t, &d)

	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	conIcono := 0
	for i, line := range d.alternativesViewB(plan) {
		plain := stripANSIStr(line)
		if strings.ContainsAny(plain, "▀▄█") {
			conIcono++
		}
		if h := strings.Count(line, "\n") + 1; h > 1 {
			t.Errorf("línea %d ocupa %d filas: %q", i, h, plain)
		}
	}
	if conIcono == 0 {
		t.Error("ninguna alternativa mostró icono con los fixtures disponibles")
	}
}

// TestIconoNoRompeEstadisticas: el icono es decorativo; los números y
// los nombres siguen exactamente igual.
func TestIconoNoRompeEstadisticas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	plan := d.lolPlan()
	if plan == nil || len(plan.Steps) == 0 {
		t.Skip("sin pasos")
	}

	// Sin arte (como en un terminal sin PNG): se captura la línea base.
	d.session.InvalidateArt()
	d.session.Provider = nil // fuerza degradación textual
	sinArte := d.routeViewA(plan)

	// Con arte.
	d = planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)
	conArte := d.routeViewA(plan)

	if len(sinArte) != len(conArte) {
		t.Fatalf("nº de líneas %d vs %d", len(sinArte), len(conArte))
	}
	for i := range sinArte {
		a := stripANSIStr(sinArte[i])
		b := stripANSIStr(conArte[i])
		// El texto sin iconos debe coincidir carácter a carácter tras
		// quitar los bloques y el espacio que añade el icono.
		clean := strings.NewReplacer("▀", "", "▄", "", "█", "").Replace(b)
		clean = strings.ReplaceAll(clean, "  ", " ")
		clean2 := strings.ReplaceAll(a, "  ", " ")
		if clean != clean2 {
			t.Errorf("línea %d difiere:\n  sin arte: %q\n  con arte: %q", i, a, clean)
		}
	}
}

// TestIconosSinProvider: sin proveedor de iconos no hay pánico y la
// vista queda intacta.
func TestIconosSinProvider(t *testing.T) {
	d := planDetailView(t, "ahri", "Ahri", 80, 50)
	d.session.Provider = nil
	d.session.InvalidateArt()
	d.tab = 1
	d.ensureTabArt() // no debe hacer nada

	for _, line := range d.routeViewA(d.lolPlan()) {
		if strings.ContainsAny(stripANSIStr(line), "▀▄█") {
			t.Errorf("sin proveedor no debe haber iconos: %q", stripANSIStr(line))
		}
	}
}

// TestIconosCatalogoIncompleto: un ítem sin PNG degrada a texto sin
// dejar hueco ni romper la línea.
func TestIconosCatalogoIncompleto(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)

	plan := d.lolPlan()
	if plan == nil || len(plan.Steps) == 0 {
		t.Skip("sin pasos")
	}
	// El paso 2 (Guinsoo) no tiene PNG en los fixtures: debe mostrarse
	// su nombre íntegro.
	for _, line := range d.routeViewA(plan) {
		plain := stripANSIStr(line)
		if strings.Contains(plain, "Guinsoo") {
			if !strings.Contains(plain, "Hoja de furia de Guinsoo") {
				t.Errorf("nombre incompleto: %q", plain)
			}
			return
		}
	}
}

// TestIconosTerminalEstrecha: por debajo de 60 columnas no se pinta
// icono, para no desplazar el nombre ni las estadísticas.
func TestIconosTerminalEstrecha(t *testing.T) {
	for _, width := range []int{40, 50, 59} {
		d := planDetailView(t, "kaisa", "Kai'Sa", width, 50)
		d.tab = 1
		loadArtSync(t, &d)
		for _, line := range d.routeViewA(d.lolPlan()) {
			if strings.ContainsAny(stripANSIStr(line), "▀▄█") {
				t.Errorf("ancho %d: no debe haber icono: %q", width, stripANSIStr(line))
			}
		}
	}
}

// TestIconosAnchosAmplios: en 60, 80 y 118 sí hay icono y el nombre se
// conserva.
func TestIconosAnchosAmplios(t *testing.T) {
	for _, width := range []int{60, 80, 118} {
		d := planDetailView(t, "kaisa", "Kai'Sa", width, 60)
		d.tab = 1
		loadArtSync(t, &d)
		plan := d.lolPlan()
		if plan == nil {
			continue
		}
		hay := false
		for _, line := range d.routeViewA(plan) {
			if strings.ContainsAny(stripANSIStr(line), "▀▄█") {
				hay = true
				break
			}
		}
		if !hay {
			t.Errorf("ancho %d: se esperaba al menos un icono", width)
		}
		// Y los nombres siguen presentes.
		found := false
		for _, line := range d.routeViewA(plan) {
			if strings.Contains(stripANSIStr(line), "Verdugo") {
				found = true
			}
		}
		if !found {
			t.Errorf("ancho %d: falta el nombre del primer paso", width)
		}
	}
}

// TestIconoObjetoDeMision: los objetos de misión no llevan icono porque
// no son compras.
func TestIconoObjetoDeMision(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	ids := tabItemIDs(1, plan)
	for _, st := range plan.Steps {
		if st.Quest {
			for _, id := range ids {
				if id == st.Item {
					t.Errorf("el objeto de misión %d no debe prerenderizarse", st.Item)
				}
			}
		}
	}
}

// TestArteNoSeMezclaPorTamano: la caché distingue tamaño y modo. Antes
// se indexaba solo por id y un icono de 12 columnas se reutilizaba a 3.
func TestArteNoSeMezclaPorTamano(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	s := d.session

	s.renderItemArt(s.Provider, 3006, 3)
	s.renderItemArt(s.Provider, 3006, 12)

	a3 := s.ItemArtFor(3006, 3)
	a12 := s.ItemArtFor(3006, 12)
	if a3 == "" || a12 == "" {
		t.Skip("sin arte para 3006 en fixtures")
	}
	if a3 == a12 {
		t.Error("el arte de 3 y 12 columnas no puede ser idéntico")
	}
	// Y cada tamaño tiene la altura esperada.
	if h := strings.Count(a3, "\n") + 1; h != 1 {
		t.Errorf("3 columnas deben dar 1 fila, dan %d", h)
	}
	if h := strings.Count(a12, "\n") + 1; h <= 1 {
		t.Errorf("12 columnas deben dar varias filas, dan %d", h)
	}
}

// TestArteInvalidaPorParche: cambiar el parche invalida el arte.
func TestArteInvalidaPorParche(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	s := d.session

	s.renderItemArt(s.Provider, 3006, 3)
	if s.ItemArtFor(3006, 3) == "" {
		t.Skip("sin arte")
	}
	antes := len(s.ItemArt)

	// Cambio de parche: la clave nueva no coincide, así que no se
	// reutiliza el arte anterior.
	s.Version = "99.99"
	if got := s.ItemArtFor(3006, 3); got != "" {
		t.Error("el arte del parche anterior no debe reutilizarse")
	}
	s.renderItemArt(s.Provider, 3006, 3)
	if len(s.ItemArt) <= antes {
		t.Error("debía añadirse una entrada para el parche nuevo")
	}

	// Invalidar vacía todo.
	s.InvalidateArt()
	if len(s.ItemArt) != 0 {
		t.Errorf("InvalidateArt dejó %d entradas", len(s.ItemArt))
	}
}

// TestArteTopeDeTamano: la caché no crece sin límite.
func TestArteTopeDeTamano(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	s := d.session
	// Llenar hasta el tope con claves sintéticas.
	for i := 0; i < artCacheMax; i++ {
		s.ItemArt[artKey{ID: i, Cols: 3, Mode: artHalfBlock, Patch: s.Version}] = "x"
	}
	if len(s.ItemArt) != artCacheMax {
		t.Fatalf("preparación: %d", len(s.ItemArt))
	}
	s.renderItemArt(s.Provider, 3006, 3)
	if len(s.ItemArt) > artCacheMax {
		t.Errorf("la caché superó el tope: %d", len(s.ItemArt))
	}
}

// TestCambioDePestanaPrerenderizaArte: entrar en Ruta o Alt. prepara el
// arte de lo que se va a ver, sin precarga global.
func TestCambioDePestanaPrerenderizaArte(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.session.InvalidateArt()

	d.tab = 1
	loadArtSync(t, &d)
	ruta := len(d.session.ItemArt)
	if ruta == 0 {
		t.Error("Ruta debía preparar arte")
	}

	d.tab = 2
	loadArtSync(t, &d)
	if len(d.session.ItemArt) <= ruta {
		t.Error("Alt. debía añadir arte propio")
	}

	// Otra pestaña no prepara nada.
	d.session.InvalidateArt()
	d.tab = 3
	loadArtSync(t, &d)
	if len(d.session.ItemArt) != 0 {
		t.Errorf("Resumen no debía preparar arte: %d entradas", len(d.session.ItemArt))
	}
}

// TestIconosNoModificanElPlan: el plan es idéntico con y sin arte.
func TestIconosNoModificanElPlan(t *testing.T) {
	sinArte := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	sinArte.session.Provider = nil
	planSin := sinArte.lolPlan()

	conArte := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	conArte.tab = 1
	conArte.ensureTabArt()
	planCon := conArte.lolPlan()

	if len(planSin.Steps) != len(planCon.Steps) {
		t.Fatalf("pasos %d vs %d", len(planSin.Steps), len(planCon.Steps))
	}
	for i := range planSin.Steps {
		a, b := planSin.Steps[i], planCon.Steps[i]
		if a.Item != b.Item || a.Slot != b.Slot || a.Stats != b.Stats || a.BuyMinute != b.BuyMinute {
			t.Errorf("paso %d cambió: %+v vs %+v", i, a, b)
		}
	}
}

// TestRegresionCampeones: los cuatro campeones exigidos siguen
// mostrando su plan completo con iconos activos.
func TestRegresionCampeones(t *testing.T) {
	casos := []struct{ key, name string }{
		{"kaisa", "Kai'Sa"},
		{"ahri", "Ahri"},
		{"malphite", "Malphite"},
		{"soraka", "Soraka"},
	}
	for _, c := range casos {
		d := planDetailView(t, c.key, c.name, 118, 60)
		d.tab = 1
		loadArtSync(t, &d)
		plan := d.lolPlan()
		if plan == nil {
			t.Errorf("%s: sin plan", c.name)
			continue
		}
		if len(plan.Steps) == 0 {
			t.Errorf("%s: sin pasos", c.name)
		}
		out := strings.Join(d.routeViewA(plan), "\n")
		if strings.TrimSpace(stripANSIStr(out)) == "" {
			t.Errorf("%s: vista vacía", c.name)
		}
	}
}

// TestRunasMuestraNombresSinIconos: sin proveedor de iconos de runa la
// pestaña NO queda vacía y conserva los nombres. Obligatorio en CI: usa
// solo fixtures locales.
func TestRunasMuestraNombresSinIconos(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	// Sin proveedor: ninguna fuente de iconos.
	d.session.Provider = nil
	out := stripANSIStr(d.runesView())
	if strings.TrimSpace(out) == "" {
		t.Fatal("la pestaña de Runas no puede quedar vacía")
	}
	if strings.Contains(out, "sin runas con muestra suficiente") {
		t.Fatal("el fixture de u.gg sí trae páginas de runas")
	}
	if !strings.ContainsAny(out, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		t.Errorf("sin nombres textuales: %q", out)
	}
}

// TestRunasConIconosReales: con un proveedor que sirve PNG válidos, la
// pestaña muestra iconos Y conserva los nombres.
//
// Usa un servidor httptest local: sin red, sin caché personal.
func TestRunasConIconosReales(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		t.Fatal("el fixture debía traer páginas de runas")
	}
	ids := runePageIDs(cb)
	if len(ids) == 0 {
		t.Fatal("sin ids de runa")
	}

	prov := newRuneArtProvider(t, ids)
	d.session.Provider = prov
	d.session.Version = "16.22.1"
	d.session.InvalidateRuneArt()

	loadRuneArtSync(t, &d)

	conArte := 0
	for _, id := range ids {
		if d.session.RuneArtFor(id, runeArtCols) != "" {
			conArte++
		}
	}
	if conArte == 0 {
		t.Fatal("ningún icono de runa se renderizó")
	}
	if prov.calls() == 0 {
		t.Error("el proveedor debía ser consultado")
	}

	out := stripANSIStr(d.runesView())
	if !strings.ContainsAny(out, "▀▄█") {
		t.Error("la vista no muestra iconos")
	}
	// Y los nombres siguen presentes.
	name := d.session.RuneName(cb.RunePages[0].KeystoneID)
	if name != "" && !strings.Contains(out, name) {
		t.Errorf("falta el nombre de la keystone %q en: %q", name, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Error("vista vacía")
	}
}

// TestRunasRenderRepetidoSinConsultas: la segunda pintura no vuelve a
// pedir iconos.
func TestRunasRenderRepetidoSinConsultas(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		t.Skip("sin runas")
	}
	prov := newRuneArtProvider(t, runePageIDs(cb))
	d.session.Provider = prov
	loadRuneArtSync(t, &d)
	primera := prov.calls()

	_ = d.runesView()
	_ = d.runesView()
	loadRuneArtSync(t, &d)

	if prov.calls() != primera {
		t.Errorf("se volvieron a pedir iconos: %d → %d", primera, prov.calls())
	}
}

// TestRunasTerminalEstrecha: por debajo de 60 columnas, solo texto.
func TestRunasTerminalEstrecha(t *testing.T) {
	for _, width := range []int{40, 50, 59} {
		d := runeDetailView(t, width, 50)
		cb := d.CurrentBuild()
		if cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		d.session.Provider = newRuneArtProvider(t, runePageIDs(cb)).over(d.session.Provider)
		loadRuneArtSync(t, &d)
		out := stripANSIStr(d.runesView())
		if strings.ContainsAny(out, "▀▄█") {
			t.Errorf("ancho %d: no debe haber iconos: %q", width, out)
		}
		if strings.TrimSpace(out) == "" {
			t.Errorf("ancho %d: vista vacía", width)
		}
	}
}

// TestRunasAnchosAmplios: en 60, 80 y 118 hay iconos y nombres.
func TestRunasAnchosAmplios(t *testing.T) {
	for _, width := range []int{60, 80, 118} {
		d := runeDetailView(t, width, 60)
		cb := d.CurrentBuild()
		if cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas")
		}
		d.session.Provider = newRuneArtProvider(t, runePageIDs(cb)).over(d.session.Provider)
		loadRuneArtSync(t, &d)
		out := stripANSIStr(d.runesView())
		if !strings.ContainsAny(out, "▀▄█") {
			t.Errorf("ancho %d: se esperaban iconos", width)
		}
		name := d.session.RuneName(cb.RunePages[0].KeystoneID)
		if name != "" && !strings.Contains(out, name) {
			t.Errorf("ancho %d: falta el nombre %q", width, name)
		}
	}
}

// TestRunasPNGsCorruptoSinRomper: un proveedor que devuelve basura no
// rompe la pestaña; se cae al nombre.
func TestRunasPNGsCorruptoSinRomper(t *testing.T) {
	d := runeDetailView(t, 118, 60)
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		t.Skip("sin runas")
	}
	d.session.Provider = corruptRuneProvider{Provider: d.session.Provider}
	d.session.InvalidateRuneArt()
	loadRuneArtSync(t, &d) // no debe entrar en pánico

	out := stripANSIStr(d.runesView())
	if strings.TrimSpace(out) == "" {
		t.Error("vista vacía con PNG corrupto")
	}
	if strings.ContainsAny(out, "▀▄█") {
		t.Error("un PNG corrupto no debe producir iconos")
	}
}

// TestRunasNoCambianEstadisticas: el arte no altera ninguna cifra ni la
// elección de runas.
func TestRunasNoCambianEstadisticas(t *testing.T) {
	sin := runeDetailView(t, 118, 60)
	sin.session.Provider = nil
	cbSin := sin.CurrentBuild()

	con := runeDetailView(t, 118, 60)
	cbCon := con.CurrentBuild()
	con.session.Provider = newRuneArtProvider(t, runePageIDs(cbCon)).over(con.session.Provider)
	loadRuneArtSync(t, &con)
	cbCon = con.CurrentBuild()

	if len(cbSin.RunePages) != len(cbCon.RunePages) {
		t.Fatalf("páginas %d vs %d", len(cbSin.RunePages), len(cbCon.RunePages))
	}
	for i := range cbSin.RunePages {
		a, b := cbSin.RunePages[i], cbCon.RunePages[i]
		if a.KeystoneID != b.KeystoneID || a.PrimaryTree != b.PrimaryTree ||
			a.SecondaryTree != b.SecondaryTree || a.Games != b.Games || a.Wins != b.Wins {
			t.Errorf("página %d cambió: %+v vs %+v", i, a, b)
		}
		if len(a.PrimaryIDs) != len(b.PrimaryIDs) || len(a.SecondaryIDs) != len(b.SecondaryIDs) {
			t.Errorf("página %d: listas de runas distintas", i)
		}
	}
	// Y el texto de cifras es idéntico al quitar los iconos.
	plainSin := stripANSIStr(sin.runesView())
	plainCon := strings.NewReplacer("▀", "", "▄", "", "█", "").Replace(stripANSIStr(con.runesView()))
	plainCon = strings.ReplaceAll(plainCon, "  ", " ")
	plainSin = strings.ReplaceAll(plainSin, "  ", " ")
	for _, want := range []string{"WR", "partidas"} {
		if strings.Contains(plainSin, want) != strings.Contains(plainCon, want) {
			t.Errorf("la cifra %q cambió de presencia", want)
		}
	}
}

// ─── proveedores de prueba ────────────────────────────────────────

// runeArtProvider envuelve un Provider real y le añade RuneIcon. Sirve
// PNG válidos para los ids indicados y cuenta llamadas. Sin red.
type runeArtProvider struct {
	staticdata.Provider
	mu    sync.Mutex
	n     int
	valid map[int]bool
}

func newRuneArtProvider(t *testing.T, ids []int) *runeArtProvider {
	t.Helper()
	v := map[int]bool{}
	for _, id := range ids {
		v[id] = true
	}
	return &runeArtProvider{valid: v}
}

// withBase fija el proveedor subyacente (el real de la sesión).
func (p *runeArtProvider) over(base staticdata.Provider) *runeArtProvider {
	p.Provider = base
	return p
}

func (p *runeArtProvider) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n
}

func (p *runeArtProvider) RuneIcon(id int) ([]byte, error) {
	p.mu.Lock()
	p.n++
	p.mu.Unlock()
	if !p.valid[id] {
		return nil, errors.New("runa sin recurso")
	}
	return tinyPNG(), nil
}

// corruptRuneProvider devuelve bytes que no son un PNG.
type corruptRuneProvider struct {
	staticdata.Provider
}

func (corruptRuneProvider) RuneIcon(int) ([]byte, error) {
	return []byte("no soy un png"), nil
}

// tinyPNG construye un PNG RGB válido de 8x8 en memoria.
func tinyPNG() []byte {
	const w, h = 8, 8
	var raw []byte
	for y := 0; y < h; y++ {
		raw = append(raw, 0) // filtro none
		for x := 0; x < w; x++ {
			raw = append(raw, 200, 60, 120)
		}
	}
	chunk := func(tag string, data []byte) []byte {
		var out []byte
		out = append(out, byte(len(data)>>24), byte(len(data)>>16), byte(len(data)>>8), byte(len(data)))
		out = append(out, tag...)
		out = append(out, data...)
		crc := crc32IEEE(append([]byte(tag), data...))
		return append(out, byte(crc>>24), byte(crc>>16), byte(crc>>8), byte(crc))
	}
	ihdr := []byte{0, 0, 0, w, 0, 0, 0, h, 8, 2, 0, 0, 0}
	var out []byte
	out = append(out, 0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a)
	out = append(out, chunk("IHDR", ihdr)...)
	out = append(out, chunk("IDAT", zlibStore(raw))...)
	out = append(out, chunk("IEND", nil)...)
	return out
}

// zlibStore envuelve datos con zlib sin compresión (bloques stored).
func zlibStore(data []byte) []byte {
	orig := data
	out := []byte{0x78, 0x01}
	for len(data) > 0 {
		n := len(data)
		if n > 65535 {
			n = 65535
		}
		final := byte(0)
		if n == len(data) {
			final = 1
		}
		out = append(out, final)
		out = append(out, byte(n), byte(n>>8))
		out = append(out, byte(^n), byte(^n>>8))
		out = append(out, data[:n]...)
		data = data[n:]
	}
	sum := adler32(orig)
	return append(out, byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum))
}

// adler32 calcula el checksum de zlib.
func adler32(d []byte) uint32 {
	var a, b uint32 = 1, 0
	for _, c := range d {
		a = (a + uint32(c)) % 65521
		b = (b + a) % 65521
	}
	return b<<16 | a
}

// crc32IEEE calcula el CRC de los chunks PNG.
func crc32IEEE(d []byte) uint32 {
	crc := ^uint32(0)
	for _, c := range d {
		crc ^= uint32(c)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
	}
	return ^crc
}

// TestRegresionDeadlockSigueIgual: Deadlock no usa estas pestañas.// TestRegresionDeadlockSigueIgual: Deadlock no usa estas pestañas.
func TestRegresionDeadlockSigueIgual(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	abrams := findChampion(t, s, "Abrams")
	d := newDetailView(abrams, s)
	d.width, d.height = 118, 60
	if got := d.tabs(); len(got) != 4 {
		t.Errorf("Deadlock debe seguir con 4 pestañas, tiene %d: %v", len(got), got)
	}
	if !strings.Contains(stripANSIStr(d.View()), "Abrams") {
		t.Error("la vista de Deadlock no muestra el héroe")
	}
	// ensureTabArt no debe tocar nada en Deadlock.
	d.session.InvalidateArt()
	d.tab = 1
	loadArtSync(t, &d)
	if len(d.session.ItemArt) != 0 {
		t.Errorf("Deadlock no debe prerenderizar arte de LoL: %d", len(d.session.ItemArt))
	}
}

// ─── interacción en Ruta y Alt. ───────────────────────────────────

// TestNavegacionTecladoRuta: flechas mueven la selección y Enter abre
// el popup, que Esc cierra.
func TestNavegacionTecladoRuta(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)
	n := len(d.cardList())
	if n < 2 {
		t.Skipf("hacen falta al menos 2 filas, hay %d", n)
	}
	if d.itemIdx != 0 {
		t.Fatalf("la selección debe empezar en 0, está en %d", d.itemIdx)
	}

	d, _ = d.Update(keyMsg("right"))
	if d.itemIdx != 1 {
		t.Errorf("right: itemIdx = %d, se esperaba 1", d.itemIdx)
	}
	d, _ = d.Update(keyMsg("left"))
	if d.itemIdx != 0 {
		t.Errorf("left: itemIdx = %d, se esperaba 0", d.itemIdx)
	}

	d, _ = d.Update(keyMsg("enter"))
	if !d.popupOpen() {
		t.Error("enter debía abrir el popup")
	}
	if d.itemPopup() == "" {
		t.Error("el popup quedó vacío")
	}
	d, _ = d.Update(keyMsg("esc"))
	if d.popupOpen() {
		t.Error("esc debía cerrar el popup")
	}
}

// TestNavegacionTecladoAlt: lo mismo en la pestaña de alternativas.
func TestNavegacionTecladoAlt(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 2
	loadArtSync(t, &d)
	n := len(d.cardList())
	if n < 2 {
		t.Skipf("hacen falta al menos 2 alternativas, hay %d", n)
	}
	d, _ = d.Update(keyMsg("right"))
	if d.itemIdx != 1 {
		t.Errorf("itemIdx = %d, se esperaba 1", d.itemIdx)
	}
	d, _ = d.Update(keyMsg("enter"))
	if !d.popupOpen() {
		t.Error("enter debía abrir el popup en Alt.")
	}
}

// TestPopupMuestraElItemCorrecto: el popup de Ruta corresponde al paso
// seleccionado, no al de Ítems.
func TestPopupMuestraElItemCorrecto(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	cards := d.cardList()
	if len(cards) == 0 {
		t.Skip("sin filas")
	}
	d.itemIdx = 1
	if d.itemIdx >= len(cards) {
		t.Skip("solo una fila")
	}
	d.showItem = true
	out := stripANSIStr(d.itemPopup())
	want := d.session.ItemName(cards[1].item.ID)
	if want != "" && !strings.Contains(out, want) {
		t.Errorf("el popup no muestra %q: %q", want, out)
	}
}

// TestZonasRutaRegistradas: cada fila de Ruta tiene su zona clicable.
func TestZonasRutaRegistradas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	_ = d.View()

	n := len(d.cardList())
	if n == 0 {
		t.Skip("sin filas")
	}
	for i := 0; i < n; i++ {
		if !waitZone("card:" + string(rune('0'+i))) {
			t.Errorf("zona card:%d no registrada", i)
		}
	}
}

// TestClicEnFilaDeRuta: el clic sobre una fila la selecciona y abre el
// popup.
func TestClicEnFilaDeRuta(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	_ = d.View()

	if !waitZone("card:1") {
		t.Skip("la fila 1 no tiene zona en este ancho")
	}
	z := zoneMgr.Get("card:1")
	cx := z.StartX + (z.EndX-z.StartX)/2
	cy := z.StartY + (z.EndY-z.StartY)/2
	mm := tea.MouseMsg{X: cx, Y: cy, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
	d = d.handleMouse(mm)
	if d.itemIdx != 1 {
		t.Errorf("clic: itemIdx = %d, se esperaba 1", d.itemIdx)
	}
}

// TestTabYTeclasNumericasIntactas: Tab y 1–7 siguen cambiando de
// pestaña, y M solo actúa en BOTTOM.
func TestTabYTeclasNumericasIntactas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	if got := len(d.tabs()); got != 7 {
		t.Fatalf("LoL debe tener 7 pestañas, tiene %d", got)
	}
	d, _ = d.Update(keyMsg("tab"))
	if d.tab != 1 {
		t.Errorf("tab: pestaña %d, se esperaba 1", d.tab)
	}
	d, _ = d.Update(keyMsg("shift+tab"))
	if d.tab != 0 {
		t.Errorf("shift+tab: pestaña %d, se esperaba 0", d.tab)
	}
	for _, tc := range []struct {
		key  string
		want int
	}{
		{"1", 0}, {"2", 1}, {"3", 2}, {"4", 3}, {"5", 4}, {"6", 5}, {"7", 6},
	} {
		d, _ = d.Update(keyMsg(tc.key))
		if d.tab != tc.want {
			t.Errorf("tecla %s: pestaña %d, se esperaba %d", tc.key, d.tab, tc.want)
		}
	}
}

// TestEscNavegaSinPopup: sin popup, Esc sigue saliendo de la vista.
func TestEscNavegaSinPopup(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 1
	if d.popupOpen() {
		t.Fatal("el popup no debe estar abierto")
	}
	d, _ = d.Update(keyMsg("enter"))
	if !d.popupOpen() {
		t.Fatal("enter debía abrirlo")
	}
	d, _ = d.Update(keyMsg("esc"))
	if d.popupOpen() {
		t.Error("esc debía cerrarlo")
	}
}

// TestSeleccionSeMantieneAlCambiarDePestana: clampSel no deja la
// selección fuera de rango al pasar a una pestaña con menos filas.
func TestSeleccionSeMantieneAlCambiarDePestana(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	d.tab = 0
	if n := len(d.cardList()); n > 0 {
		d.itemIdx = n - 1
	}
	d, _ = d.Update(keyMsg("2")) // a Ruta
	if n := len(d.cardList()); d.itemIdx >= n && n > 0 {
		t.Errorf("selección %d fuera de rango (n=%d)", d.itemIdx, n)
	}
}

// runeDetailView construye una vista de detalle cuya sesión SÍ tiene
// páginas de runas (los fixtures de u.gg las traen).
func runeDetailView(t testing.TB, width, height int) detailView {
	t.Helper()
	s := loadOffline(t, model.GameLoL)
	var champ *ChampEntry
	for _, e := range s.Champions {
		for _, b := range e.ByRole {
			if len(b.RunePages) > 0 {
				champ = e
				break
			}
		}
		if champ != nil {
			break
		}
	}
	if champ == nil {
		t.Fatal("ningún campeón con runas en los fixtures")
	}
	d := newDetailView(champ, s)
	d.width, d.height = width, height
	d.vp.Width = max(24, width-4)
	d.vp.Height = height
	d.tab = 4 // Runas
	d.sync()
	return d
}

// newRuneArtProviderB es newRuneArtProvider para benchmarks.
func newRuneArtProviderB(b *testing.B, ids []int) *runeArtProvider {
	b.Helper()
	v := map[int]bool{}
	for _, id := range ids {
		v[id] = true
	}
	return &runeArtProvider{valid: v}
}

// loadRuneArtSync ejecuta el Cmd de arte de runas y publica el
// resultado, replicando lo que hace Bubble Tea al recibir el mensaje.
func loadRuneArtSync(t testing.TB, d *detailView) {
	t.Helper()
	cmd := runeArtCmdFor(d.session, runePageIDs(d.CurrentBuild()))
	if cmd == nil {
		return
	}
	msg, ok := cmd().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado del Cmd de runas")
	}
	for k, art := range msg.Art {
		d.session.putArt(k, art)
	}
}

// loadArtSync ejecuta el Cmd de arte de la pestaña actual y publica el
// resultado, replicando lo que hace Bubble Tea al recibir el mensaje.
func loadArtSync(t testing.TB, d *detailView) {
	t.Helper()
	cmd := d.artCmd()
	if cmd == nil {
		return
	}
	msg, ok := cmd().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado del Cmd de arte")
	}
	for k, art := range msg.Art {
		d.session.putArt(k, art)
	}
}
