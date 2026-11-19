package ui

import (
	"testing"

	"github.com/Eion21/mobatui/internal/lolbuild"
	"github.com/Eion21/mobatui/internal/model"
)

// ─── integridad del planificador (referencia de la Fase 1) ──────────
//
// Estos valores se capturaron del commit base f8de376 ANTES de tocar la
// UI y se verificaron idénticos después. La Fase 1 solo cambia cómo se
// PRESENTAN los datos: ninguna compra, orden, botas, inventario,
// candidato, alternativa, win rate, partidas, runa ni habilidad puede
// cambiar.
//
// Si un test de aquí falla, NO hay que actualizar el valor esperado sin
// diagnosticar primero: significa que se alteró una decisión.

// refPlan es el resultado esperado de un campeón/rol de referencia.
type refPlan struct {
	key     string
	champ   string
	role    model.Role
	patch   string
	games   int
	winRate float64
	// steps son (slot, item, partidas) en orden.
	steps [][3]int
	// boots son (item, partidas).
	boots [][2]int
	// final y finalMain son los inventarios por id.
	final     []int
	finalMain []int
	// mainSlots, bootsSlot, questSlot y questUse describen la misión.
	mainSlots int
	bootsSlot bool
	questSlot bool
	questUse  string
	complete  bool
	// candidates son los ids de los candidatos para completar.
	candidates []int
	// altCount es el número de alternativas listadas.
	altCount int
}

// planesDeReferencia son los cinco campeones obligatorios, con los
// valores reales del commit base.
var planesDeReferencia = []refPlan{
	{
		key: "kaisa", champ: "Kai'Sa", role: model.RoleBottom,
		patch: "16.21", games: 335558, winRate: 0.5175,
		steps:     [][3]int{{1, 6672, 256561}, {2, 3124, 285399}, {3, 3302, 129934}, {4, 3157, 82129}, {5, 3089, 11162}},
		boots:     [][2]int{{3006, 269533}, {3008, 44941}},
		final:     []int{6672, 3124, 3302, 3157, 3089, 3006},
		finalMain: []int{6672, 3124, 3302, 3157, 3089},
		mainSlots: 5, bootsSlot: true, complete: true,
		altCount: 12,
	},
	{
		key: "ahri", champ: "Ahri", role: model.RoleMiddle,
		patch: "16.21", games: 156563, winRate: 0.53,
		steps:     [][3]int{{1, 3118, 94358}, {2, 4645, 57056}, {3, 3157, 32205}, {4, 3089, 12148}, {5, 3135, 1710}},
		boots:     [][2]int{{3020, 90050}, {3158, 44266}},
		final:     []int{3118, 4645, 3157, 3089, 3135, 3020},
		finalMain: []int{3118, 4645, 3157, 3089, 3135},
		mainSlots: 5, bootsSlot: true, complete: true,
		altCount: 11,
	},
	{
		key: "briar", champ: "Briar", role: model.RoleMiddle,
		patch: "16.21", games: 3083, winRate: 0.5115,
		steps:     [][3]int{{1, 3748, 888}, {2, 3071, 870}, {3, 6333, 464}, {4, 3031, 107}},
		boots:     [][2]int{{3111, 1057}, {3008, 924}},
		final:     []int{3748, 3071, 6333, 3031, 3111},
		finalMain: []int{3748, 3071, 6333, 3031},
		mainSlots: 5, bootsSlot: true, complete: false,
		candidates: []int{6676, 3036, 3153},
		altCount:   9,
	},
	{
		key: "malphite", champ: "Malphite", role: model.RoleTop,
		patch: "16.21", games: 111671, winRate: 0.5304,
		steps:     [][3]int{{1, 3068, 42047}, {2, 3075, 27796}, {3, 3110, 11458}, {4, 6665, 4231}, {5, 3143, 655}},
		boots:     [][2]int{{3047, 69062}, {3111, 25584}},
		final:     []int{3068, 3075, 3110, 6665, 3143, 3047},
		finalMain: []int{3068, 3075, 3110, 6665, 3143},
		mainSlots: 5, bootsSlot: true, complete: true,
		altCount: 9,
	},
	{
		key: "soraka", champ: "Soraka", role: model.RoleSupport,
		patch: "16.21", games: 74281, winRate: 0.5258,
		steps:     [][3]int{{1, 3870, 49176}, {2, 6617, 60672}, {3, 3107, 21865}, {4, 6621, 7031}, {5, 3504, 456}},
		boots:     [][2]int{{3158, 50565}, {3009, 16840}},
		final:     []int{3870, 6617, 3107, 6621, 3504, 3158},
		finalMain: []int{3870, 6617, 3107, 6621, 3504},
		mainSlots: 5, bootsSlot: true, questSlot: true,
		questUse: "guardianes de control", complete: true,
		altCount: 7,
	},
}

// TestPlanificadorSinCambiosFase1 es la prueba de referencia: todas las
// decisiones siguen idénticas al commit base.
func TestPlanificadorSinCambiosFase1(t *testing.T) {
	for _, ref := range planesDeReferencia {
		t.Run(ref.champ, func(t *testing.T) {
			s, champ := planSession(t, ref.key, ref.champ)
			d := newDetailView(champ, s)
			d.width, d.height = 118, 60
			d.sync()
			plan := d.lolPlan()
			if plan == nil {
				t.Fatal("sin plan")
			}

			// Rol y parche de los datos.
			if plan.Role != ref.role {
				t.Errorf("rol = %q, se esperaba %q", plan.Role, ref.role)
			}
			if plan.Patch != ref.patch {
				t.Errorf("parche de datos = %q, se esperaba %q", plan.Patch, ref.patch)
			}
			if plan.Total.Games != ref.games {
				t.Errorf("partidas = %d, se esperaba %d", plan.Total.Games, ref.games)
			}
			if diff := plan.WinRate - ref.winRate; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("WR = %v, se esperaba %v", plan.WinRate, ref.winRate)
			}

			// Pasos: orden exacto y muestra de cada uno.
			if len(plan.Steps) != len(ref.steps) {
				t.Fatalf("%d pasos, se esperaban %d", len(plan.Steps), len(ref.steps))
			}
			for i, st := range plan.Steps {
				slot, item, games := ref.steps[i][0], ref.steps[i][1], ref.steps[i][2]
				if st.Slot != slot || st.Item != item || st.Stats.Games != games {
					t.Errorf("paso %d = (%d, %d, %d partidas), se esperaba (%d, %d, %d)",
						i+1, st.Slot, st.Item, st.Stats.Games, slot, item, games)
				}
			}

			// Botas y su orden.
			if len(plan.Boots) != len(ref.boots) {
				t.Fatalf("%d botas, se esperaban %d", len(plan.Boots), len(ref.boots))
			}
			for i, b := range plan.Boots {
				if b.Item != ref.boots[i][0] || b.Stats.Games != ref.boots[i][1] {
					t.Errorf("bota %d = (%d, %d), se esperaba (%d, %d)",
						i, b.Item, b.Stats.Games, ref.boots[i][0], ref.boots[i][1])
				}
			}

			// Inventario final.
			if !iguales(plan.Final, ref.final) {
				t.Errorf("inventario final = %v, se esperaba %v", plan.Final, ref.final)
			}
			if !iguales(plan.FinalMain, ref.finalMain) {
				t.Errorf("inventario principal = %v, se esperaba %v", plan.FinalMain, ref.finalMain)
			}

			// Reglas de misión.
			if plan.Inventory.MainSlots != ref.mainSlots {
				t.Errorf("espacios principales = %d", plan.Inventory.MainSlots)
			}
			if plan.Inventory.BootsSlot != ref.bootsSlot {
				t.Errorf("ranura de botas = %v", plan.Inventory.BootsSlot)
			}
			if plan.Inventory.QuestSlot != ref.questSlot {
				t.Errorf("ranura de misión = %v", plan.Inventory.QuestSlot)
			}
			if string(plan.Inventory.QuestSlotUse) != ref.questUse {
				t.Errorf("uso de la ranura = %q, se esperaba %q",
					plan.Inventory.QuestSlotUse, ref.questUse)
			}

			// Completitud.
			if plan.Complete != ref.complete {
				t.Errorf("completo = %v, se esperaba %v (motivo: %q)",
					plan.Complete, ref.complete, plan.Reason)
			}

			// Candidatos para completar: mismos ids y mismo orden.
			cands := plan.CandidatesFor()
			if len(cands) != len(ref.candidates) {
				t.Fatalf("%d candidatos, se esperaban %d", len(cands), len(ref.candidates))
			}
			for i, c := range cands {
				if c.Item != ref.candidates[i] {
					t.Errorf("candidato %d = %d, se esperaba %d", i, c.Item, ref.candidates[i])
				}
			}

			// Alternativas y sus posiciones.
			var alts []lolbuild.Alternative
			for _, g := range plan.Alternatives {
				alts = append(alts, g.Items...)
			}
			if len(alts) != ref.altCount {
				t.Errorf("%d alternativas, se esperaban %d", len(alts), ref.altCount)
			}
		})
	}
}

// TestRunasYHabilidadesSinCambios: el orden de runas y de habilidades
// tampoco se altera.
//
// Ojo: los fixtures de PLAN (lolbuild) no traen ni runas ni habilidades
// —solo el reparto por posiciones—, así que sería inventar afirmar sus
// valores. Se comprueban sobre los fixtures que SÍ los publican
// (ugg/ddragon offline), que incluyen dos de los campeones de
// referencia: Ahri y Malphite.
func TestRunasYHabilidadesSinCambios(t *testing.T) {
	s := loadOffline(t, model.GameLoL)
	esperado := map[string]struct {
		role  model.Role
		runes int
		prio  []rune
		skill string
	}{
		// Valores del commit base.
		"Ahri":     {role: model.RoleMiddle, runes: 2, prio: []rune("QWE"), skill: "QWEQQRQWQWRWWEEREE"},
		"Malphite": {role: model.RoleTop, runes: 2, prio: []rune("QWE"), skill: "QWEQQRQWQWRWWEEREE"},
	}
	for key, quiere := range esperado {
		t.Run(key, func(t *testing.T) {
			champ := findChampion(t, s, key)
			b := champ.ByRole[quiere.role]
			if b == nil {
				t.Fatalf("%s sin build de %s", key, quiere.role)
			}
			if len(b.RunePages) != quiere.runes {
				t.Errorf("%d páginas de runas, se esperaban %d",
					len(b.RunePages), quiere.runes)
			}
			if got := string(b.SkillOrder.Priority); got != string(quiere.prio) {
				t.Errorf("prioridad = %q, se esperaba %q", got, string(quiere.prio))
			}
			if b.SkillOrder.Sequence != quiere.skill {
				t.Errorf("secuencia = %q, se esperaba %q",
					b.SkillOrder.Sequence, quiere.skill)
			}
		})
	}

	// Los cinco campeones de referencia no deben perder la build al
	// cambiar la presentación: el plan sigue existiendo y es el mismo.
	for _, ref := range planesDeReferencia {
		t.Run("plan/"+ref.champ, func(t *testing.T) {
			d := planDetailView(t, ref.key, ref.champ, 118, 60)
			if d.CurrentBuild() == nil {
				t.Fatal("la build desapareció")
			}
			if d.lolPlan() == nil {
				t.Fatal("el plan desapareció")
			}
		})
	}
}

// TestReglasDeMisionBottom: Kai'Sa BOTTOM mantiene las reglas de misión
// (6 principales tras completarla, botas en la ranura de misión).
func TestReglasDeMisionBottom(t *testing.T) {
	d := planDetailView(t, "kaisa", "Kai'Sa", 118, 60)
	plan := d.lolPlan()
	if plan == nil {
		t.Fatal("sin plan")
	}
	antes := plan.Inventory
	if antes.MainSlots != 5 {
		t.Errorf("BOTTOM sin misión: %d principales", antes.MainSlots)
	}
	if !antes.BootsSlot {
		t.Error("BOTTOM sin misión: las botas ocupan ranura principal")
	}

	// Alternar el estado de misión cambia el inventario, y ese
	// comportamiento debe seguir intacto.
	nd, _ := d.Update(keyMsg("M"))
	d2 := nd
	d2.sync()
	despues := d2.lolPlan()
	if despues == nil {
		t.Fatal("sin plan tras M")
	}
	if despues.Inventory.MainSlots != 6 {
		t.Errorf("BOTTOM con misión: %d principales, se esperaban 6",
			despues.Inventory.MainSlots)
	}
}

// iguales compara dos slices de enteros en orden.
func iguales(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
