package ui

// Presentación de habilidades: canónica vs fallback. El fallback no debe
// presentar números que parezcan teclas reales.

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// fakeSkills arma un CleanBuild con habilidades sintéticas.
func fakeSkills(canonical bool, n int) *model.CleanBuild {
	b := &model.CleanBuild{
		Game:                model.GameDeadlock,
		CanonicalSkillOrder: canonical,
		AbilityNames:        map[rune]string{},
		SkillOrder:          model.SkillOrder{},
	}
	seq := make([]rune, 0, n*3)
	for i := 0; i < n; i++ {
		slot := rune('1' + i)
		b.SkillAlphabet = append(b.SkillAlphabet, slot)
		b.AbilityNames[slot] = "Habilidad " + string(slot)
		for k := 0; k < 3; k++ {
			seq = append(seq, slot)
		}
	}
	b.SkillOrder.Sequence = string(seq)
	b.SkillOrder.Priority = append([]rune(nil), b.SkillAlphabet...)
	return b
}

// TestSlotLabelCanonical: con orden canónico se muestran las teclas.
func TestSlotLabelCanonical(t *testing.T) {
	for _, r := range []rune{'1', '2', '3', '4'} {
		if got := slotLabel(r, true); got != string(r) {
			t.Errorf("slotLabel(%c, true) = %q", r, got)
		}
	}
}

// TestSlotLabelFallbackNeutral: sin orden canónico se usan letras.
func TestSlotLabelFallbackNeutral(t *testing.T) {
	want := map[rune]string{'1': "A", '2': "B", '3': "C", '4': "D", '5': "E", '6': "F", '7': "G"}
	for r, exp := range want {
		if got := slotLabel(r, false); got != exp {
			t.Errorf("slotLabel(%c, false) = %q, esperado %q", r, got, exp)
		}
	}
}

// TestFallbackShowsWarningAbove: el aviso aparece arriba del panel.
func TestFallbackShowsWarningAbove(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CanonicalSkillOrder = false
	b.AbilityNames = map[rune]string{'1': "Alfa", '2': "Beta", '3': "Gamma", '4': "Delta", '5': "Épsilon"}
	b.SkillAlphabet = []rune{'1', '2', '3', '4', '5'}
	b.SkillOrder.Sequence = "1234512345"
	b.SkillOrder.Priority = []rune{'1', '2', '3', '4', '5'}

	d := newDetailView(champ, s)
	d.width, d.height = 120, 40
	d.tab = 3
	d.layout(0)
	d.sync()
	plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))

	if !strings.Contains(plain, "posiciones no verificadas") {
		t.Error("falta el aviso de posiciones no verificadas")
	}
	// El aviso debe ir ANTES de la matriz de niveles.
	warnAt := strings.Index(plain, "posiciones no verificadas")
	gridAt := strings.Index(plain, "Alfa")
	if warnAt < 0 || gridAt < 0 || warnAt > gridAt {
		t.Errorf("el aviso no está arriba (aviso=%d, matriz=%d)", warnAt, gridAt)
	}
	// Y no debe haber un "5" presentado como tecla.
	if strings.Contains(plain, "5 Épsilon") {
		t.Error("el fallback presenta un número de tecla inexistente")
	}
	if !strings.Contains(plain, "E Épsilon") {
		t.Error("el fallback no usa referencia neutral para el quinto espacio")
	}
}

// TestCanonicalNoWarning: con orden canónico no hay aviso.
func TestCanonicalNoWarning(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CanonicalSkillOrder = true
	b.AbilityNames = map[rune]string{'1': "Alfa", '2': "Beta", '3': "Gamma", '4': "Delta"}
	b.SkillAlphabet = []rune{'1', '2', '3', '4'}
	b.SkillOrder.Sequence = "123412341234"
	b.SkillOrder.Priority = []rune{'1', '2', '3', '4'}

	d := newDetailView(champ, s)
	d.width, d.height = 120, 40
	d.tab = 3
	d.layout(0)
	d.sync()
	plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))

	if strings.Contains(plain, "posiciones no verificadas") {
		t.Error("aviso de fallback con orden canónico")
	}
	if !strings.Contains(plain, "1 Alfa") {
		t.Error("no muestra la tecla real 1")
	}
}

// TestCanonicalFourRows: exactamente cuatro filas con orden canónico.
func TestCanonicalFourRows(t *testing.T) {
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CanonicalSkillOrder = true
	b.AbilityNames = map[rune]string{'1': "Alfa", '2': "Beta", '3': "Gamma", '4': "Delta"}
	b.SkillAlphabet = []rune{'1', '2', '3', '4'}
	b.SkillOrder.Sequence = "123412341234"
	b.SkillOrder.Priority = []rune{'1', '2', '3', '4'}

	d := newDetailView(champ, s)
	d.width, d.height = 120, 40
	d.tab = 3
	d.layout(0)
	d.sync()
	plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))
	for _, name := range []string{"Alfa", "Beta", "Gamma", "Delta"} {
		if !strings.Contains(plain, name) {
			t.Errorf("falta la fila %q", name)
		}
	}
}
