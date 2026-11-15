package ui

// Nombres de habilidad sin truncar cuando hay espacio.

import (
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// skillApp arma un detalle con nombres de habilidad dados.
func skillApp(t *testing.T, w int, names []string) App {
	t.Helper()
	s := loadOffline(t, model.GameDeadlock)
	champ := findChampion(t, s, "Abrams")
	b := champ.Build(0)
	b.CanonicalSkillOrder = true
	b.AbilityNames = map[rune]string{}
	b.SkillAlphabet = nil
	var seq []rune
	for i, n := range names {
		slot := rune('1' + i)
		b.SkillAlphabet = append(b.SkillAlphabet, slot)
		b.AbilityNames[slot] = n
		for k := 0; k < 3; k++ {
			seq = append(seq, slot)
		}
	}
	b.SkillOrder.Sequence = string(seq)
	b.SkillOrder.Priority = append([]rune(nil), b.SkillAlphabet...)

	d := newDetailView(champ, s)
	d.width, d.height = w, 40
	d.tab = 3
	d.layout(0)
	d.sync()
	return App{screen: screenDetail, session: s, detail: d, width: w, height: 40}
}

// TestSkillNamesNotTruncatedWide: en terminal ancha, nombres completos.
func TestSkillNamesNotTruncatedWide(t *testing.T) {
	names := []string{"Living Ink", "Flow State", "Still Life", "Abstract Landscape"}
	a := skillApp(t, 140, names)
	plain := collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
	for _, n := range names {
		if !strings.Contains(plain, n) {
			t.Errorf("nombre %q truncado en terminal ancha", n)
		}
	}
}

// TestSkillNamesTruncatedNarrow: en terminal estrecha sí se recorta,
// pero la matriz sigue alineada.
func TestSkillNamesTruncatedNarrow(t *testing.T) {
	names := []string{"Living Ink", "Flow State", "Still Life", "Abstract Landscape"}
	a := skillApp(t, 60, names)
	// No debe desbordar el ancho.
	for _, line := range strings.Split(a.View(), "\n") {
		if width(line) > 60 {
			t.Errorf("línea excede 60: %q", line)
		}
	}
	// La matriz de niveles debe seguir presente.
	plain := collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
	if !strings.Contains(plain, "1") {
		t.Error("se perdió la matriz de niveles")
	}
}

// TestSkillLabelWidthAdapts: el ancho se ajusta al nombre más largo.
func TestSkillLabelWidthAdapts(t *testing.T) {
	a := skillApp(t, 140, []string{"A", "BB", "CCC", "Abstract Landscape"})
	got := a.detail.skillLabelWidth()
	// "Abstract Landscape" = 18 + 2 = 20.
	if got < 20 {
		t.Errorf("ancho %d, esperado al menos 20 para el nombre más largo", got)
	}
}

// TestSkillLabelWidthCapped: no se come la matriz en terminal estrecha.
func TestSkillLabelWidthCapped(t *testing.T) {
	a := skillApp(t, 60, []string{"Un Nombre Extraordinariamente Largo De Habilidad"})
	got := a.detail.skillLabelWidth()
	// 60 - 4 - 45 = 11 deja sitio a la matriz.
	if got > 20 {
		t.Errorf("ancho %d demasiado grande para 60 columnas", got)
	}
}

// TestSkillGridAlignment: las filas de la matriz quedan alineadas.
func TestSkillGridAlignment(t *testing.T) {
	names := []string{"Living Ink", "Flow State", "Still Life", "Abstract Landscape"}
	a := skillApp(t, 140, names)
	plain := collapsePlain(ansiRE.ReplaceAllString(a.View(), ""))
	var rows []string
	for _, line := range strings.Split(plain, "\n") {
		for _, n := range names {
			if strings.Contains(line, n) {
				rows = append(rows, line)
			}
		}
	}
	if len(rows) < 4 {
		t.Fatalf("filas encontradas: %d", len(rows))
	}
	// Todas las filas deben tener el mismo ancho visible.
	want := width(rows[0])
	for i, r := range rows {
		if width(r) != want {
			t.Errorf("fila %d ancho %d != %d", i, width(r), want)
		}
	}
}
