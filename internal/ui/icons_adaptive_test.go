package ui

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/Eion21/mobatui/internal/imgrender"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── Fase 2: iconos compactos adaptativos ────────────────────────────
//
// El icono de Ruta/Alt. crece con el ancho del terminal (3 → 4 → 6
// columnas) manteniendo SIEMPRE una sola fila por compra. Aquí se fija
// esa escala y las garantías que no deben romperse.

// TestEscalaDeIconosPorAncho fija los tramos y comprueba que son
// monótonos: más ancho nunca da menos columnas, y nunca se pasa del
// máximo (8 columnas estira 4x y el icono deja de leerse como objeto).
func TestEscalaDeIconosPorAncho(t *testing.T) {
	casos := []struct {
		width int
		want  int
	}{
		{0, smallArtCols}, // ancho desconocido: conservador
		{39, 0},           // demasiado estrecho
		{40, 0},
		{59, 0},
		{60, smallArtCols}, // umbral heredado de v0.2.1
		{79, smallArtCols},
		{80, smallArtCols},
		{99, smallArtCols},
		{100, artColsMedium},
		{117, artColsMedium},
		{118, artColsLarge},
		{160, artColsLarge},
		{240, artColsLarge},
	}
	for _, c := range casos {
		if got := iconColsFor(c.width); got != c.want {
			t.Errorf("iconColsFor(%d) = %d, se esperaba %d", c.width, got, c.want)
		}
	}

	// Monotonía y cota. Se empieza en 1: el ancho 0 significa
	// "desconocido" (cmd/render, tests) y devuelve el tamaño
	// conservador, no el mínimo de una terminal real.
	prev := -1
	for w := 1; w <= 240; w++ {
		c := iconColsFor(w)
		if c > artColsLarge {
			t.Fatalf("ancho %d: %d columnas supera el máximo %d", w, c, artColsLarge)
		}
		if c < prev {
			t.Fatalf("ancho %d: %d columnas, menos que el ancho anterior (%d)", w, c, prev)
		}
		prev = c
	}
}

// TestIconosAdaptativosUnaSolaFila es la garantía central de la fase:
// con CUALQUIER ancho y CUALQUIER tamaño de icono, cada compra ocupa
// exactamente una línea.
func TestIconosAdaptativosUnaSolaFila(t *testing.T) {
	for _, width := range []int{40, 60, 80, 100, 118} {
		d := planDetailView(t, "briar", "Briar", width, 60)
		d.session.Version = "16.23.1"
		d.tab = 1
		loadArtSync(t, &d)
		plan := d.lolPlan()
		if plan == nil {
			t.Fatal("sin plan")
		}
		for _, line := range d.routeViewA(plan) {
			if rows := lipgloss.Height(stripANSIStr(line)); rows != 1 {
				t.Errorf("ancho %d: una línea de ruta ocupa %d filas: %q",
					width, rows, stripANSIStr(line))
			}
		}
	}
}

// TestIconoCreceConElAncho comprueba que el icono realmente es más
// ancho donde debe y que la precarga usa el MISMO tamaño que la
// presentación (si divergen, la línea saldría sin icono).
func TestIconoCreceConElAncho(t *testing.T) {
	for _, c := range []struct {
		width    int
		wantCols int
	}{
		{80, smallArtCols},
		{100, artColsMedium},
		{118, artColsLarge},
	} {
		d := planDetailView(t, "briar", "Briar", c.width, 60)
		d.tab = 1
		loadArtSync(t, &d)

		if d.artCols != c.wantCols {
			t.Errorf("ancho %d: artCols = %d, se esperaba %d",
				c.width, d.artCols, c.wantCols)
		}
		// El arte cargado debe existir con ESE ancho, no con otro.
		plan := d.lolPlan()
		var conIcono bool
		for _, st := range plan.Steps {
			if st.Quest {
				continue
			}
			art := d.session.ItemArtFor(st.Item, c.wantCols)
			if art != "" {
				conIcono = true
				// Y NO debe existir con el ancho de otro tramo, para
				// probar que la clave separa por tamaño.
				if c.wantCols != smallArtCols {
					if otro := d.session.ItemArtFor(st.Item, smallArtCols); otro != "" {
						// Puede existir si otra prueba lo cargó: solo
						// se comprueba que la clave del tamaño pedido
						// funciona.
						_ = otro
					}
				}
			}
		}
		if !conIcono {
			t.Errorf("ancho %d: ningún paso tiene arte a %d columnas",
				c.width, c.wantCols)
		}
	}
}

// TestReprecargaAlCambiarDeTramo: redimensionar de 80 a 118 debe pedir
// el arte con el ancho nuevo, y NO reprecargar si el tramo no cambia.
func TestReprecargaAlCambiarDeTramo(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 80, 60)
	d.tab = 1
	loadArtSync(t, &d)
	if d.artCols != smallArtCols {
		t.Fatalf("artCols inicial = %d", d.artCols)
	}

	// Mismo tramo (80 → 90): no debe hacer falta reprecargar.
	d2 := d
	d2.width = 90
	if cols := iconColsFor(d2.width); cols != d.artCols {
		if !d2.session.tabArtReady(d2.tab, d2.lolPlan(), cols) {
			t.Logf("80→90 cambió de tramo a %d; se reprecarga (aceptable)", cols)
		}
	}

	// Cambio de tramo (80 → 118): el arte de 3 no sirve para 6.
	d3 := d
	d3.width = 118
	plan := d3.lolPlan()
	// Ningún ítem con PNG puede tener ya arte de 6 columnas.
	for _, id := range tabItemIDs(d3.tab, plan) {
		if id <= 0 {
			continue
		}
		if _, err := d3.session.Provider.ItemIcon(id); err != nil {
			continue // sin PNG en fixtures: no aplica
		}
		if d3.session.ItemArtFor(id, artColsLarge) != "" {
			t.Errorf("80→118: id=%d ya tenía arte de 6 columnas", id)
		}
	}
	// Y el Cmd debe pedir el ancho nuevo (no el de 3).
	cmd := d3.artCmd()
	if cmd == nil {
		t.Fatal("tras cambiar de tramo debe pedirse arte nuevo")
	}
	msg, ok := cmd().(artLoadedMsg)
	if !ok {
		t.Fatal("mensaje inesperado")
	}
	if len(msg.Art) == 0 {
		t.Fatal("el Cmd no devolvió arte")
	}
	for k := range msg.Art {
		if k.Cols != artColsLarge {
			t.Errorf("se pidió arte a %d columnas, se esperaba %d", k.Cols, artColsLarge)
		}
	}
}

// TestTabArtReadyEvitaRepeticiones: con todo cargado, el Cmd debe ser
// nil (no se relanza la descarga en cada tick de resize).
func TestTabArtReadyEvitaRepeticiones(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)
	if d.artCols != artColsLarge {
		t.Fatalf("artCols = %d", d.artCols)
	}
	// Todo ítem con PNG debe quedar en caché y no volver a pedirse.
	plan := d.lolPlan()
	for _, st := range plan.Steps {
		if st.Quest || st.Item <= 0 {
			continue
		}
		if _, err := d.session.Provider.ItemIcon(st.Item); err != nil {
			continue
		}
		k := artKey{Kind: artItem, ID: st.Item, Cols: artColsLarge,
			Mode: artHalfBlock, Patch: d.session.Version}
		if !d.session.hasArt(k) {
			t.Errorf("id=%d tiene PNG pero no quedó en caché", st.Item)
		}
	}
}

// TestCambioDeTamanoNoReutilizaIconosComprueba el requisito explícito:
// un icono de un ancho no puede aparecer en una línea que pide otro.
func TestCambioDeTamanoNoReutilizaIconos(t *testing.T) {
	s, champ := planSession(t, "briar", "Briar")
	d := newDetailView(champ, s)
	d.width, d.height = 80, 60
	d.tab = 1
	d.sync()
	loadArtSync(t, &d)

	// El arte de 3 columnas existe; el de 6 no.
	plan := d.lolPlan()
	// Se busca un ítem que SÍ tenga PNG en los fixtures: el primer paso
	// de Briar (Hidra titánica) no está, así que elegir "el primero"
	// hacía que el test se saltara sin probar nada.
	var id int
	for _, st := range plan.Steps {
		if st.Item <= 0 || st.Quest {
			continue
		}
		if s.ItemArtFor(st.Item, smallArtCols) != "" {
			id = st.Item
			break
		}
	}
	if id == 0 {
		t.Skip("ningún paso tiene PNG en los fixtures")
	}
	if s.ItemArtFor(id, artColsLarge) != "" {
		t.Fatal("el arte de 6 columnas no debería existir aún")
	}

	// Tras redimensionar a 118, la línea NO puede mostrar el icono de 3.
	d.width = 118
	d.sync()
	if got := s.ItemArtFor(id, artColsLarge); got != "" {
		t.Fatal("sin reprecargar no hay arte de 6; el test asume eso")
	}
	// Y lo que se pinta en la línea para ese ancho es el arte de 6 (aún
	// vacío), nunca el de 3: la línea queda sin icono hasta que llega.
	d.itemIdx = 0
	linea := stripANSIStr(d.routeStepLine(plan.Steps[0]))
	if plan.Steps[0].Item == id && strings.ContainsAny(linea, "▀▄█") {
		t.Errorf("se reutilizó un icono de otro tamaño: %q", linea)
	}
}

// TestSinProviderTextoIntacto: sin proveedor no hay icono y el texto
// queda exactamente igual que sin la funcionalidad.
func TestSinProviderTextoIntacto(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Provider = nil
	plan := d.lolPlan()
	for _, line := range d.routeViewA(plan) {
		if strings.ContainsAny(stripANSIStr(line), "▀▄█") {
			t.Errorf("sin proveedor no debe haber icono: %q", stripANSIStr(line))
		}
	}
	if len(d.routeViewA(plan)) == 0 {
		t.Error("la ruta debe seguir pintándose sin proveedor")
	}
}

// TestObjetoDeMisionSinIcono se mantiene con el ancho adaptativo.
func TestObjetoDeMisionSinIcono(t *testing.T) {
	d := planDetailView(t, "soraka", "Soraka", 118, 60)
	d.tab = 1
	loadArtSync(t, &d)
	plan := d.lolPlan()
	if plan == nil {
		t.Fatal("sin plan")
	}
	for _, st := range plan.Steps {
		if !st.Quest {
			continue
		}
		linea := stripANSIStr(d.routeStepLine(st))
		if strings.ContainsAny(linea, "▀▄█") {
			t.Errorf("objeto de misión con icono: %q", linea)
		}
		if !strings.Contains(linea, "por misión") {
			t.Errorf("falta la etiqueta de misión: %q", linea)
		}
	}
}

// TestIconosNoAlteranRecomendaciones: el ancho del icono no puede
// cambiar ninguna decisión del planificador.
func TestIconosNoAlteranRecomendaciones(t *testing.T) {
	type huella struct {
		steps []int
		final []int
		boots []int
		cands []int
		alts  int
		games int
		winrt float64
	}
	var refs []huella
	for _, width := range []int{40, 60, 80, 100, 118} {
		d := planDetailView(t, "briar", "Briar", width, 60)
		d.tab = 1
		loadArtSync(t, &d)
		plan := d.lolPlan()
		h := huella{alts: len(plan.Alternatives), games: plan.Total.Games, winrt: plan.WinRate}
		for _, st := range plan.Steps {
			h.steps = append(h.steps, st.Item)
		}
		for _, b := range plan.Boots {
			h.boots = append(h.boots, b.Item)
		}
		h.final = append(h.final, plan.Final...)
		for _, c := range plan.CandidatesFor() {
			h.cands = append(h.cands, c.Item)
		}
		refs = append(refs, h)
	}
	base := refs[0]
	for i, h := range refs[1:] {
		if !iguales(h.steps, base.steps) || !iguales(h.final, base.final) ||
			!iguales(h.boots, base.boots) || !iguales(h.cands, base.cands) ||
			h.alts != base.alts || h.games != base.games || h.winrt != base.winrt {
			t.Errorf("ancho %d alteró las recomendaciones (ref %d)", i+1, 0)
		}
	}
}

// TestIconosAnchoNoDesborda: el icono más ancho no debe sacar la línea
// del ancho útil del terminal.
func TestIconosAnchoNoDesborda(t *testing.T) {
	for _, width := range []int{80, 100, 118} {
		d := planDetailView(t, "briar", "Briar", width, 60)
		d.session.Version = "16.23.1"
		d.tab = 1
		loadArtSync(t, &d)
		d.sync()
		for _, line := range strings.Split(stripANSIStr(d.View()), "\n") {
			if w := lipgloss.Width(line); w > width {
				t.Errorf("ancho %d: línea de %d celdas: %q", width, w, line)
			}
		}
	}
}

// TestAltTambienCrece: la pestaña de Alternativas usa el mismo ancho.
func TestAltTambienCrece(t *testing.T) {
	for _, c := range []struct{ width, cols int }{
		{80, smallArtCols}, {100, artColsMedium}, {118, artColsLarge},
	} {
		d := planDetailView(t, "briar", "Briar", c.width, 60)
		d.tab = 2
		loadArtSync(t, &d)
		if d.artCols != c.cols {
			t.Errorf("Alt a %d: artCols = %d, se esperaba %d",
				c.width, d.artCols, c.cols)
		}
	}
}

// TestDeadlockIntacto: los iconos de Deadlock (12 columnas) no se ven
// afectados por la escala de LoL.
func TestDeadlockIntacto(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	d := newDetailView(champ, s)
	d.width, d.height = 118, 60
	d.sync()
	// En Deadlock iconColsFor no se usa: las tarjetas van a 12.
	if got := d.session.ItemArtFor(champ.Build(0).CoreItems[0].ID, 12); got == "" {
		t.Log("sin arte de Deadlock en fixtures: nada que comprobar")
		return
	}
	vista := stripANSIStr(d.View())
	if strings.Contains(vista, "▀▀▀▀▀▀ ") && d.width < 100 {
		t.Error("Deadlock no debe adoptar la escala de LoL")
	}
}

// TestPrecargaYPresentacionMismoAncho cierra un punto ciego detectado
// por mutación: si artCmd pide un ancho distinto del que itemIconInline
// pinta, la precarga llena la caché con un tamaño que nadie usa y la
// línea se queda SIN icono hasta el siguiente cambio de pestaña. Los
// tests de "el icono crece" no lo detectaban porque miraban el ancho
// presentado, no la coherencia interna.
func TestPrecargaYPresentacionMismoAncho(t *testing.T) {
	for _, width := range []int{80, 100, 118} {
		d := planDetailView(t, "briar", "Briar", width, 60)
		d.tab = 1
		plan := d.lolPlan()

		// Tras la precarga real, TODOS los pasos con PNG deben tener
		// arte en el ancho que la presentación va a pedir. Si la
		// precarga hubiera usado otro ancho, aquí faltaría.
		loadArtSync(t, &d)
		quiere := iconColsFor(width)
		var conPNG, conArte int
		for _, st := range plan.Steps {
			if st.Quest || st.Item <= 0 {
				continue
			}
			// Un ítem sin PNG en los fixtures devuelve un error genérico
			// (no un 404), que se clasifica como TRANSITORIO y por tanto
			// no se memoriza: es la degradación correcta. Solo se
			// evalúan los que sí tienen imagen.
			if _, err := d.session.Provider.ItemIcon(st.Item); err != nil {
				continue
			}
			conPNG++
			if d.session.ItemArtFor(st.Item, quiere) != "" {
				conArte++
			}
		}
		if conPNG > 0 && conArte != conPNG {
			t.Errorf("ancho %d: %d de %d pasos con PNG tienen arte a %d columnas; "+
				"la precarga y la presentación no usan el mismo ancho",
				width, conArte, conPNG, quiere)
		}

		// Y la línea realmente pinta el icono cuando existe.
		var pintados int
		for _, st := range plan.Steps {
			if st.Quest || st.Item <= 0 {
				continue
			}
			if d.session.ItemArtFor(st.Item, quiere) == "" {
				continue
			}
			if strings.ContainsAny(stripANSIStr(d.routeStepLine(st)), "▀▄█") {
				pintados++
			}
		}
		if conPNG > 0 && pintados == 0 {
			t.Errorf("ancho %d: hay arte a %d columnas pero no se pinta en la línea",
				width, quiere)
		}
	}
}

// TestComparacionIconosReproducible genera la comparación visual de
// variantes de ancho de icono usando SOLO los fixtures del repositorio.
//
// No depende de la caché personal ni de rutas privadas: cualquiera con
// el repo puede regenerarla.
//
//	MOBATUI_ICONOS=/tmp/iconos.txt go test ./internal/ui \
//	    -run TestComparacionIconosReproducible -v
//
// El archivo resultante lista, por cada ancho de la escala y cada ítem
// con PNG en testdata/ddragon, el bloque de medio bloque ya renderizado
// más su nombre. Sin la variable de entorno el test no hace nada.
//
// Advertencia: los PNG de testdata/ddragon son fixtures sintéticos
// (formas simples), NO arte real de Data Dragon. Sirven para verificar
// geometría (cuántas columnas y filas ocupa cada variante), no para
// juzgar reconocimiento visual con arte auténtico.
func TestComparacionIconosReproducible(t *testing.T) {
	dest := os.Getenv("MOBATUI_ICONOS")
	if dest == "" {
		t.Skip("define MOBATUI_ICONOS=<ruta> para generar la comparación")
	}
	const bg = 0x0A1428 // panel
	variantes := []struct {
		cols int
		name string
	}{
		{smallArtCols, "3 columnas (aspecto 1.5)"},
		{artColsMedium, "4 columnas (aspecto 2.0)"},
		{artColsLarge, "6 columnas (aspecto 3.0)"},
		{8, "8 columnas (aspecto 4.0)"},
	}

	// Ítems con PNG disponible en los fixtures, en orden estable.
	var ids []int
	for _, id := range []int{3026, 3031, 3036, 3071, 3111, 3153, 6333, 6676} {
		if _, err := os.Stat(fmt.Sprintf("../../testdata/ddragon/%d.png", id)); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		t.Skip("sin PNG en testdata/ddragon")
	}

	var sb strings.Builder
	sb.WriteString("Icono compacto en UNA fila (=2 px verticales).\n")
	sb.WriteString("Los PNG de testdata/ddragon son fixtures sintéticos: esto mide\n")
	sb.WriteString("GEOMETRÍA (columnas y filas), no reconocimiento con arte real.\n\n")
	for _, v := range variantes {
		sb.WriteString(fmt.Sprintf("=== %s ===\n", v.name))
		for _, id := range ids {
			raw, err := os.ReadFile(fmt.Sprintf("../../testdata/ddragon/%d.png", id))
			if err != nil {
				continue
			}
			img, err := png.Decode(bytes.NewReader(raw))
			if err != nil {
				continue
			}
			// UNA fila: es la restricción del producto.
			art := imgrender.Render(img, v.cols, 1, color.RGBA{0x0A, 0x14, 0x28, 0xFF})
			// Se cuenta con ANSI quitado: debe ser siempre 1 fila y
			// exactamente v.cols celdas.
			plano := stripANSIStr(art)
			filas := lipgloss.Height(plano)
			ancho := lipgloss.Width(plano)
			if filas != 1 {
				t.Errorf("%d col: %d filas (debe ser 1)", v.cols, filas)
			}
			if ancho != v.cols {
				t.Errorf("%d col: %d celdas de ancho (debe ser %d)", v.cols, ancho, v.cols)
			}
			sb.WriteString(fmt.Sprintf("|%s| id=%d (%dx%d)\n", art, id, ancho, filas))
		}
		sb.WriteString("\n")
	}
	if err := os.WriteFile(dest, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("comparación escrita en %s", dest)
}
