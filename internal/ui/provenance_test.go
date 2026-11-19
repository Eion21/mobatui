package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// ─── procedencia de parches ─────────────────────────────────────────
//
// Estos tests fijan la separación entre el parche de DATOS (estadísticas
// de partidas) y el de CATÁLOGO (Data Dragon), que es la causa del
// "Patch 16.22" en cabecera contra "16.23.1" en el pie.

// TestNormalizacionDeParches: la comparación de parches usa la serie
// (major.minor) para no confundir "16.23" con "16.23.1", que son el
// mismo parche de juego con distinto formato de publicación.
func TestNormalizacionDeParches(t *testing.T) {
	casos := []struct{ in, want string }{
		{"16.22", "16.22"},
		{"16.23.1", "16.23"},
		{"16.22.10", "16.22"},
		{"16.22.1", "16.22"},
		{" 16.22 ", "16.22"},
		{"", ""},
		{"16", "16"},
		{"dev", "dev"},
		{"16.x", "16.x"},
		{"16..1", "16..1"},
	}
	for _, c := range casos {
		if got := normalizePatch(c.in); got != c.want {
			t.Errorf("normalizePatch(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
}

// TestRelacionEntreParches cubre la clasificación normal / distinto /
// desconocido. Una diferencia de serie NO es incompatibilidad.
func TestRelacionEntreParches(t *testing.T) {
	casos := []struct {
		stats, catalog string
		want           patchRelation
	}{
		{"16.22", "16.23.1", patchRelationSeriesDiff},
		{"16.22", "16.22.1", patchRelationSame}, // misma serie
		{"16.22", "16.22", patchRelationSame},
		{"16.23", "16.23.1", patchRelationSame},
		{"16.22", "", patchRelationUnknown},
		{"", "16.23.1", patchRelationUnknown},
		{"", "", patchRelationUnknown},
		{"dev", "16.23.1", patchRelationSeriesDiff},
	}
	for _, c := range casos {
		p := patchProvenance{StatsPatch: c.stats, CatalogPatch: c.catalog}
		if got := p.relation(); got != c.want {
			t.Errorf("relación(%q, %q) = %v, se esperaba %v", c.stats, c.catalog, got, c.want)
		}
	}
}

// TestCabeceraYFondoAtribuyenCadaParche: la cabecera lleva el parche de
// DATOS y el pie el de CATÁLOGO, cada uno con su etiqueta.
func TestCabeceraYFondoAtribuyenCadaParche(t *testing.T) {
	p := patchProvenance{StatsPatch: "16.22", CatalogPatch: "16.23.1"}

	ancho := headerPatchLabel(p, true)
	if !strings.Contains(ancho, "Datos 16.22") {
		t.Errorf("la cabecera no atribuye el parche de datos: %q", ancho)
	}
	if !strings.Contains(ancho, "Catálogo 16.23.1") {
		t.Errorf("la cabecera ancha no menciona el catálogo: %q", ancho)
	}
	if !strings.Contains(ancho, "parches distintos") {
		t.Errorf("la cabecera no señala la diferencia: %q", ancho)
	}
	// No debe quedar la etiqueta ambigua anterior.
	if strings.Contains(ancho, "Patch 16.22") {
		t.Errorf("sigue la etiqueta ambigua 'Patch': %q", ancho)
	}

	estrecho := headerPatchLabel(p, false)
	if estrecho != "Datos 16.22" {
		t.Errorf("en estrecho debe priorizarse el parche de datos: %q", estrecho)
	}

	if got := footerPatchLabel(p, false); !strings.Contains(got, "16.23.1") {
		t.Errorf("el pie no lleva el parche de catálogo: %q", got)
	}
}

// TestMismoParcheNoAvisa: si las series coinciden no se añade ruido.
func TestMismoParcheNoAvisa(t *testing.T) {
	p := patchProvenance{StatsPatch: "16.22", CatalogPatch: "16.22.1"}
	label := headerPatchLabel(p, true)
	if strings.Contains(label, "parches distintos") {
		t.Errorf("16.22 y 16.22.1 son la misma serie, no debe avisar: %q", label)
	}
	if got := staleNote(p); got != "" {
		t.Errorf("misma serie no es dato anterior: %q", got)
	}
}

// TestParcheDesconocidoNoSeInventa: sin versión no se fabrica un número.
func TestParcheDesconocidoNoSeInventa(t *testing.T) {
	// Sin NINGUNA de las dos identidades: ninguna etiqueta puede llevar
	// un número de parche, y tampoco puede quedar vacía.
	vacio := patchProvenance{}
	for _, wide := range []bool{false, true} {
		for _, got := range []string{headerPatchLabel(vacio, wide), footerPatchLabel(vacio, wide)} {
			if got == "" {
				t.Errorf("la etiqueta no puede quedar vacía (wide=%v)", wide)
			}
			if strings.ContainsAny(got, "0123456789") {
				t.Errorf("se inventó un parche sin datos: %q", got)
			}
		}
	}

	// Con UNA sola identidad se muestra esa, y solo esa: no se rellena
	// la que falta con la que hay (serían parches distintos).
	soloDatos := patchProvenance{StatsPatch: "16.22"}
	if got := headerPatchLabel(soloDatos, true); got != "Datos 16.22" {
		t.Errorf("con solo datos: %q", got)
	}
	if got := footerPatchLabel(soloDatos, true); strings.Contains(got, "16.22") {
		t.Errorf("el pie no debe reutilizar el parche de datos como catálogo: %q", got)
	}

	soloCatalogo := patchProvenance{CatalogPatch: "16.23.1"}
	if got := footerPatchLabel(soloCatalogo, true); !strings.Contains(got, "16.23.1") {
		t.Errorf("con solo catálogo, el pie debe mostrarlo: %q", got)
	}
	if got := headerPatchLabel(soloCatalogo, true); strings.Contains(got, "Catálogo 16.23.1 ·") {
		t.Errorf("la cabecera no debe inventar una diferencia: %q", got)
	}
}

// TestProcedenciaEnLaSesion comprueba que la procedencia sale de los
// campos reales: stats de la build, catálogo de la sesión.
func TestProcedenciaEnLaSesion(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Version = "16.23.1" // catálogo
	// El fixture de Briar trae el parche de datos real (16.21).
	b := d.CurrentBuild()
	if b.Patch == "" {
		t.Fatal("el fixture debe traer parche de datos")
	}
	p := d.provenance()
	if p.StatsPatch != b.Patch {
		t.Errorf("StatsPatch = %q, se esperaba el de la build %q", p.StatsPatch, b.Patch)
	}
	if p.CatalogPatch != "16.23.1" {
		t.Errorf("CatalogPatch = %q", p.CatalogPatch)
	}
	if p.relation() != patchRelationSeriesDiff {
		t.Error("16.21 contra 16.23.1 es una diferencia de serie")
	}
}

// TestBriarMiddleSeparaLosDosParches: el caso reportado. La cabecera y
// el pie muestran cada uno SU parche, etiquetado.
func TestBriarMiddleSeparaLosDosParches(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Version = "16.23.1"
	stats := d.CurrentBuild().Patch

	cabecera := stripANSIStr(d.headerView())
	if !strings.Contains(cabecera, "Datos "+normalizePatch(stats)) &&
		!strings.Contains(cabecera, "Datos "+stats) {
		t.Errorf("la cabecera no atribuye el parche de datos: %q", cabecera)
	}
	pie := statusLine(d.session)
	if !strings.Contains(pie, "16.23.1") {
		t.Errorf("el pie no muestra el parche de catálogo: %q", pie)
	}
	if !strings.Contains(pie, "Cat") {
		t.Errorf("el pie no atribuye el parche al catálogo: %q", pie)
	}
}

// TestPieSinVersionDeCatalogo: sin versión conocida se dice, no se deja
// un hueco con un guion sin explicación.
func TestPieSinVersionDeCatalogo(t *testing.T) {
	s := &Session{Game: model.GameLoL}
	line := statusLine(s)
	if !strings.Contains(line, "catálogo") && !strings.Contains(line, "Cat") {
		t.Errorf("el pie debe indicar que no hay versión de catálogo: %q", line)
	}
}

// TestPieDeadlockSigueFuncionando: el cambio no rompe la barra de
// Deadlock, que usa otro proveedor.
func TestPieDeadlockSigueFuncionando(t *testing.T) {
	s := &Session{Game: model.GameDeadlock, Version: "2026-11-10"}
	line := statusLine(s)
	if !strings.Contains(line, "héroes") {
		t.Errorf("Deadlock debe seguir diciendo 'héroes': %q", line)
	}
	if !strings.Contains(line, "2026-11-10") {
		t.Errorf("el pie debe llevar el parche del proveedor: %q", line)
	}
}

// TestCambioDeCampeonOActualizaParche: la procedencia se recalcula por
// build, no se queda pegada al campeón anterior.
func TestCambioDeCampeonOActualizaParche(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Version = "16.23.1"
	primer := d.provenance()

	// Cambiar de campeón (Kai'Sa) en la misma sesión.
	s2, champ2 := planSession(t, "kaisa", "Kai'Sa")
	s2.Version = "16.23.1"
	d2 := newDetailView(champ2, s2)
	d2.width, d2.height = 118, 60
	(&d2).sync()
	segundo := d2.provenance()

	if primer.StatsPatch == "" || segundo.StatsPatch == "" {
		t.Fatal("ambos campeones deben traer parche de datos")
	}
	// Ambos fixtures son 16.21, pero lo importante es que el valor se
	// lea de la build activa y no de un estado compartido.
	if segundo.StatsPatch != d2.CurrentBuild().Patch {
		t.Errorf("procedencia pegada: %q vs %q",
			segundo.StatsPatch, d2.CurrentBuild().Patch)
	}
	if segundo.CatalogPatch != "16.23.1" {
		t.Errorf("catálogo = %q", segundo.CatalogPatch)
	}
}

// TestJuegoLoLAaDeadlockSinParcheCruzado: al cambiar de juego no se
// arrastra la identidad del otro proveedor.
func TestJuegoLoLAaDeadlockSinParcheCruzado(t *testing.T) {
	lol := &Session{Game: model.GameLoL, Version: "16.23.1"}
	dl := &Session{Game: model.GameDeadlock, Version: "2026-11-10"}

	lolLine, dlLine := statusLine(lol), statusLine(dl)
	if strings.Contains(dlLine, "16.23.1") {
		t.Errorf("el pie de Deadlock arrastra el parche de LoL: %q", dlLine)
	}
	if strings.Contains(lolLine, "2026-11-10") {
		t.Errorf("el pie de LoL arrastra el parche de Deadlock: %q", lolLine)
	}
	if !strings.Contains(dlLine, "héroes") || !strings.Contains(lolLine, "campeones") {
		t.Error("cada juego mantiene su vocabulario")
	}
}

// TestPopupExplicaLaProcedencia: el popup separa datos y catálogo.
func TestPopupExplicaLaProcedencia(t *testing.T) {
	d := planDetailView(t, "briar", "Briar", 118, 60)
	d.session.Version = "16.23.1"
	lines := provenanceLines(d.provenance(), "MIDDLE", "lolalytics.com", "Briar")
	if len(lines) == 0 {
		t.Fatal("sin líneas de procedencia")
	}
	juntas := strings.Join(lines, "\n")
	if !strings.Contains(juntas, "datos ") || !strings.Contains(juntas, "catálogo ") {
		t.Errorf("el popup no separa las dos identidades: %q", juntas)
	}
	if !strings.Contains(juntas, "Briar") || !strings.Contains(juntas, "MIDDLE") {
		t.Errorf("el popup no da contexto: %q", juntas)
	}
}

// TestCambioDeRolActualizaProcedencia: al cambiar de rol (tecla r) la
// cabecera debe releer el parche de la build ACTIVA, no conservar el del
// rol anterior. Es el otro eje de "cambio de campeón o rol".
func TestCambioDeRolActualizaProcedencia(t *testing.T) {
	s, champ := planSession(t, "kaisa", "Kai'Sa")
	// Segundo rol con otra muestra y otro parche de datos.
	otro := *champ.ByRole[model.RoleBottom]
	otro.Role = model.RoleMiddle
	otro.Patch = "16.22"
	otro.Total.Games = 1234
	champ.Roles = append(champ.Roles, model.RoleMiddle)
	champ.ByRole[model.RoleMiddle] = &otro

	d := newDetailView(champ, s)
	d.width, d.height = 118, 60
	(&d).sync()
	s.Version = "16.23.1"

	antes := d.provenance()
	if antes.StatsPatch != "16.21" {
		t.Fatalf("rol inicial: parche %q, se esperaba 16.21", antes.StatsPatch)
	}

	// Ciclar de rol con la tecla que ya existe.
	for i := 0; i < len(champ.Roles); i++ {
		nd, _ := d.Update(keyMsg("r"))
		d = nd
		d.sync()
		if d.provenance().StatsPatch == "16.22" {
			break
		}
	}
	despues := d.provenance()
	if despues.StatsPatch != "16.22" {
		t.Errorf("tras cambiar de rol el parche de datos es %q, se esperaba 16.22",
			despues.StatsPatch)
	}
	if despues.CatalogPatch != "16.23.1" {
		t.Errorf("el catálogo no debe cambiar al cambiar de rol: %q",
			despues.CatalogPatch)
	}
	// Y la cabecera lo refleja.
	cab := stripANSIStr(d.headerBlock())
	if !strings.Contains(cab, "Datos 16.22") {
		t.Errorf("la cabecera no refleja el parche del rol nuevo: %q", cab)
	}
}

// ─── constancia de la incertidumbre de la Fase 1 ────────────────────
//
// Este archivo NO prueba comportamiento: deja escrito en el propio
// repositorio un riesgo que quedó abierto durante la Fase 1, para que no
// dependa de la memoria de nadie ni de un informe fuera del árbol.
//
// RIESGO ABIERTO (no resuelto, documentado):
//
//	Durante el trabajo de v0.2.2 desaparecieron dos directorios
//	untracked del repositorio principal:
//
//	  .gr/      — de su contenido se observaron .gr/cache y
//	              .gr/mod/cache; consistente con un GOPATH/workspace
//	              con caché de módulos, es decir, un ARTEFACTO
//	              REPRODUCIBLE, no datos del usuario.
//	  .pubtest/ — contenido desconocido; no se observó a tiempo.
//
//	Lo que SÍ está establecido con evidencia:
//	  · Nunca estuvieron en el índice de git, en ningún commit, en el
//	    stash ni en las reglas de ignorado.
//	  · No desapareció ningún archivo TRACKEADO (git status sin 'D').
//	  · El stash, las ramas, los tags y los WIP del master local siguen
//	    íntegros.
//
//	Lo que NO pudo establecerse:
//	  · El mecanismo exacto del borrado. Sin sudo (bloqueado por
//	    "no new privileges"), sin snapshots de btrfs accesibles y sin
//	    rastro en historial de shell, papelera ni temporales.
//	  · El contenido original de .pubtest/.
//
//	Mitigación: si .pubtest/ se vuelve a necesitar, hay que regenerarlo;
//	no hay copia recuperable conocida. .gr/ se regenera solo con
//	cualquier operación de Go que use ese GOPATH.
func TestRiesgoAbiertoDeDirectoriosPerdidos(t *testing.T) {
	// Se afirma el estado actual: los directorios no están. Si alguien
	// los restaura, este test avisa para actualizar la nota.
	raiz := "../.."
	for _, dir := range []string{".gr", ".pubtest"} {
		if _, err := os.Stat(raiz + "/" + dir); err == nil {
			t.Logf("NOTA: %s/ existe de nuevo; actualiza el comentario de "+
				"este archivo y confirma si su contenido se recuperó", dir)
		}
	}

	// La garantía que sí importa y que sigue vigente: el fixture de
	// referencia del planificador está congelado, no se regenera solo.
	fixture := raiz + "/testdata/lolbuild/plans-16.21.json"
	info, err := os.Stat(fixture)
	if err != nil {
		t.Fatalf("el fixture de referencia desapareció: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("el fixture de referencia está vacío")
	}
}
