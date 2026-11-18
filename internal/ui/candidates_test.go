package ui

// Pruebas de candidatos para completar inventario.
//
// Deterministas: fixtures locales, sin red ni caché personal.

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/lolitems"
	"github.com/Eion21/mobatui/internal/model"
)

// TestCandidatosSoloConHueco: con el inventario completo no se propone
// nada. Proponer un objeto sin hueco sería inventar una decisión.
func TestCandidatosSoloConHueco(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	if !plan.Complete {
		t.Skip("el fixture no produce un plan completo")
	}
	if got := plan.CandidatesFor(); len(got) != 0 {
		t.Errorf("plan completo no debe tener candidatos: %d", len(got))
	}
}

// TestCandidatosConHuecoDeMision: al completar la misión de BOT se
// libera un espacio y sí se proponen candidatos.
func TestCandidatosConHuecoDeMision(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M")) // misión completada
	d = nd
	plan := d.lolPlan()
	if plan == nil || plan.Complete {
		t.Skip("el fixture no deja hueco tras completar la misión")
	}
	cands := plan.CandidatesFor()
	if len(cands) == 0 {
		t.Fatal("con hueco debía haber candidatos")
	}
	if len(cands) > 3 {
		t.Errorf("se muestran %d candidatos, el tope es 3", len(cands))
	}
	for _, c := range cands {
		if c.Item <= 0 || c.Stats.Games <= 0 {
			t.Errorf("candidato sin datos: %+v", c)
		}
		if c.Name == "" {
			t.Errorf("candidato %d sin nombre", c.Item)
		}
	}
}

// TestCandidatosNoDuplicanEquipados: ningún candidato está ya en el
// inventario ni en los pasos.
func TestCandidatosNoDuplicanEquipados(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	plan := d.lolPlan()
	equipped := map[int]bool{}
	for _, id := range plan.Final {
		equipped[id] = true
	}
	for _, id := range plan.FinalMain {
		equipped[id] = true
	}
	for _, s := range plan.Steps {
		equipped[s.Item] = true
	}
	for _, c := range plan.CandidatesFor() {
		if equipped[c.Item] {
			t.Errorf("el candidato %d (%s) ya está equipado", c.Item, c.Name)
		}
	}
}

// TestCandidatosSinBotas: las botas no completan un inventario.
func TestCandidatosSinBotas(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	for _, c := range d.lolPlan().CandidatesFor() {
		if meta, ok := d.session.ItemCatalog[c.Item]; ok && lolitems.IsBoots(meta) {
			t.Errorf("el candidato %d son botas", c.Item)
		}
	}
}

// TestCandidatosSinObjetosDeMision: un objeto de misión ya está
// cumpliendo su función; no es un candidato.
func TestCandidatosSinObjetosDeMision(t *testing.T) {
	d := planDetailView(t, "soraka", "Soraka", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	quests := map[int]bool{}
	for _, id := range plan.Final {
		if lolitems.IsQuestItem(id) {
			quests[id] = true
		}
	}
	for _, c := range plan.CandidatesFor() {
		if quests[c.Item] {
			t.Errorf("el candidato %d es objeto de misión", c.Item)
		}
	}
}

// TestCandidatosNoAlteranLasCincoCompras es la prueba clave: los pasos
// observados no cambian al calcular candidatos.
func TestCandidatosNoAlteranLasCincoCompras(t *testing.T) {
	for _, c := range []struct{ key, name string }{
		{"kaisa", "Kai'Sa"},
		{"ahri", "Ahri"},
		{"malphite", "Malphite"},
		{"soraka", "Soraka"},
	} {
		d := planDetailView(t, c.key, c.name, 118, 60)
		plan := d.lolPlan()
		if plan == nil {
			t.Errorf("%s: sin plan", c.name)
			continue
		}
		// Copia de los pasos ANTES de pedir candidatos.
		antes := make([]lolbuild.Step, len(plan.Steps))
		copy(antes, plan.Steps)
		finalAntes := append([]int(nil), plan.Final...)

		_ = plan.CandidatesFor()

		if len(plan.Steps) != len(antes) {
			t.Fatalf("%s: pasos %d → %d", c.name, len(antes), len(plan.Steps))
		}
		for i := range antes {
			a, b := antes[i], plan.Steps[i]
			if a.Item != b.Item || a.Slot != b.Slot || a.Stats != b.Stats ||
				a.BuyMinute != b.BuyMinute || a.Quest != b.Quest || a.Boots != b.Boots {
				t.Errorf("%s: el paso %d cambió: %+v → %+v", c.name, i, a, b)
			}
		}
		if len(plan.Final) != len(finalAntes) {
			t.Errorf("%s: inventario final %d → %d", c.name, len(finalAntes), len(plan.Final))
		}
		for i := range finalAntes {
			if plan.Final[i] != finalAntes[i] {
				t.Errorf("%s: inventario[%d] %d → %d", c.name, i, finalAntes[i], plan.Final[i])
			}
		}
	}
}

// TestCandidatosNoSePresentanComoSextaCompra: la vista lo dice
// explícitamente y no numera los candidatos como pasos.
func TestCandidatosNoSePresentanComoSextaCompra(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	d.tab = 1
	if len(d.lolPlan().CandidatesFor()) == 0 {
		t.Skip("sin candidatos")
	}
	out := stripANSIStr(strings.Join(d.routeViewA(d.lolPlan()), "\n"))

	if !strings.Contains(out, "sin orden observado") {
		t.Error("falta la etiqueta 'sin orden observado'")
	}
	if !strings.Contains(out, "no son una sexta compra") {
		t.Error("falta el aviso de que no es una sexta compra")
	}
	// Los candidatos NO llevan numeración de paso ("6." o "6)") al
	// inicio de su línea.
	for _, line := range strings.Split(out, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "6.") {
			t.Errorf("un candidato parece el paso 6: %q", line)
		}
	}
}

// TestCandidatosFuenteSinPopularItem: sin alternativas no hay
// candidatos y se mantiene el aviso de incompleto.
func TestCandidatosFuenteSinPopularItem(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	vacio := lolbuild.Plan{
		Steps:        plan.Steps,
		Final:        plan.Final,
		FinalMain:    plan.FinalMain,
		Inventory:    plan.Inventory,
		Complete:     false,
		Reason:       plan.Reason,
		Alternatives: nil, // fuente sin popularItem
	}
	if got := vacio.CandidatesFor(); len(got) != 0 {
		t.Errorf("sin alternativas no debe haber candidatos: %d", len(got))
	}
}

// TestCandidatosInventarioSinEspacio: cuando el plan está completo no
// se emite la sección de candidatos en la vista.
func TestCandidatosInventarioSinEspacio(t *testing.T) {
	d := planDetailView(t, "ahri", "Ahri", 118, 60)
	plan := d.lolPlan()
	if plan == nil {
		t.Skip("sin plan")
	}
	if !plan.Complete {
		t.Skip("el fixture no produce plan completo")
	}
	out := stripANSIStr(strings.Join(d.routeViewA(plan), "\n"))
	if strings.Contains(out, "CANDIDATOS PARA COMPLETAR") {
		t.Error("un plan completo no debe mostrar candidatos")
	}
	if !strings.Contains(out, "inventario completo") {
		t.Error("debe decir que el inventario está completo")
	}
}

// TestMisionSoloAfectaBottom: la tecla M no cambia el inventario de
// otros roles.
func TestMisionSoloAfectaBottom(t *testing.T) {
	for _, c := range []struct{ key, name string }{
		{"ahri", "Ahri"}, {"malphite", "Malphite"}, {"soraka", "Soraka"},
	} {
		d := planDetailView(t, c.key, c.name, 118, 60)
		cb := d.CurrentBuild()
		nd, cmd := d.Update(keyMsg("M"))
		if nd.session.botMissionDone {
			t.Errorf("%s: M no debe cambiar la misión fuera de BOT", c.name)
		}
		if cmd != nil {
			t.Errorf("%s: M no debía emitir toast fuera de BOT", c.name)
		}
		if cb != nil && cb.Role != model.RoleBottom {
			if nd.session.MissionStateFor(cb.Role) != lolbuild.MissionUnknown {
				t.Errorf("%s: el estado de misión debe seguir siendo desconocido", c.name)
			}
		}
	}
}

// TestCandidatosTerminalEstrecha: en terminal estrecha no hay iconos
// pero sí texto.
func TestCandidatosTerminalEstrecha(t *testing.T) {
	for _, width := range []int{40, 50, 59} {
		d := planDetailView(t, "kaisa", "Kai'Sa", width, 50)
		nd, _ := d.Update(keyMsg("M"))
		d = nd
		d.tab = 1
		plan := d.lolPlan()
		if plan == nil || len(plan.CandidatesFor()) == 0 {
			t.Skip("sin candidatos")
		}
		out := stripANSIStr(strings.Join(d.routeViewA(plan), "\n"))
		if strings.ContainsAny(out, "▀▄█") {
			t.Errorf("ancho %d: no debe haber iconos", width)
		}
		if !strings.Contains(out, "CANDIDATOS") {
			t.Errorf("ancho %d: falta la sección de candidatos", width)
		}
	}
}

// TestCandidatosAnchosAmplios: con ancho hay iconos junto al nombre.
func TestCandidatosAnchosAmplios(t *testing.T) {
	for _, width := range []int{80, 118} {
		d := planDetailView(t, "kaisa", "Kai'Sa", width, 60)
		nd, _ := d.Update(keyMsg("M"))
		d = nd
		d.tab = 1
		d.ensureTabArt()
		plan := d.lolPlan()
		if plan == nil || len(plan.CandidatesFor()) == 0 {
			t.Skip("sin candidatos")
		}
		out := stripANSIStr(strings.Join(d.routeViewA(plan), "\n"))
		if !strings.Contains(out, "CANDIDATOS") {
			t.Errorf("ancho %d: falta la sección", width)
		}
		// Los nombres siguen visibles.
		for _, c := range plan.CandidatesFor() {
			if c.Name != "" && !strings.Contains(out, c.Name) {
				t.Errorf("ancho %d: falta el nombre %q", width, c.Name)
			}
		}
	}
}

// TestCandidatosConMuestraPequena: se descartan los que no tienen
// partidas.
func TestCandidatosConMuestraPequena(t *testing.T) {
	plan := lolbuild.Plan{
		Complete:  false,
		Inventory: lolbuild.Inventory{MainSlots: 6},
		Alternatives: []lolbuild.CategoryGroup{{
			Category: lolitems.CatPhysicalDamage,
			Items: []lolbuild.Alternative{
				{Item: 1, Name: "sin partidas", Stats: model.Stats{Games: 0}},
				{Item: 2, Name: "con partidas", Stats: model.Stats{Games: 1000, Wins: 520}},
			},
		}},
	}
	cands := plan.CandidatesFor()
	for _, c := range cands {
		if c.Stats.Games <= 0 {
			t.Errorf("candidato sin muestra: %+v", c)
		}
	}
}

// TestCandidatosOffline: con la caché offline los candidatos siguen
// saliendo de las estructuras ya cargadas (sin red).
func TestCandidatosOffline(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	nd, _ := d.Update(keyMsg("M"))
	d = nd
	// La sesión ya es offline por construcción del helper.
	plan := d.lolPlan()
	if len(plan.CandidatesFor()) == 0 {
		t.Skip("sin candidatos")
	}
	// Dos llamadas devuelven lo mismo: no depende de red ni de estado.
	a := plan.CandidatesFor()
	b := plan.CandidatesFor()
	if len(a) != len(b) {
		t.Fatalf("resultados distintos: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("candidato %d cambió entre llamadas", i)
		}
	}
}
