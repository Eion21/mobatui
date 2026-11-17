package lolalytics

// Regresión del orden de habilidades de LoL.
//
// Semántica demostrada con las páginas reales de Lolalytics (parche
// 16.22), contrastada de tres formas independientes:
//
//   1. El dígito 4 aparece SIEMPRE en los niveles 6 y 11 en Ahri,
//      Malphite y Soraka — exactamente donde la R está disponible.
//      Eso fija 1..4 = Q/W/E/R sin ambigüedad.
//   2. De las 24 permutaciones posibles, ninguna supera 58/80 filas
//      legales, y todas las mejores fijan 4=R.
//   3. De las 20 combinaciones de puntos que suman 15 con topes 5/5/5/3,
//      NINGUNA tiene Q=6 ni E=6.
//
// Los dígitos SON niveles: 15 dígitos = niveles 1..15. El fallo de
// Kai'Sa no es de mapeo: su secuencia publicada es imposible (6 unos y
// 6 treses con tope 5), así que se rechaza y se usa la PRIORIDAD, que
// la fuente publica aparte y sí es válida.
//
// Estas pruebas usan el fixture versionado: sin red ni caché personal.

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

// skillFixture es la forma del fixture de habilidades.
type skillFixture struct {
	ChampionID    int     `json:"champion_id"`
	ChampionName  string  `json:"champion_name"`
	Role          string  `json:"role"`
	Patch         string  `json:"patch"`
	SkillOrderID  float64 `json:"skillorder_id"`
	SkillOrderN   int     `json:"skillorder_n"`
	SkillOrderWR  float64 `json:"skillorder_wr"`
	SkillPriority string  `json:"skillpriority"`
	PriorityN     int     `json:"skillpriority_n"`
	PriorityWR    float64 `json:"skillpriority_wr"`
	Note          string  `json:"note"`
}

// loadSkills carga el fixture de los cuatro campeones.
func loadSkills(t *testing.T) map[string]skillFixture {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/lolalytics/skills-16.22.json")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]skillFixture
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 4 {
		t.Fatalf("el fixture debe traer 4 campeones, trae %d", len(out))
	}
	return out
}

// lolRules son las reglas de LoL: 15 niveles, topes 5/5/5/3, R en 6/11/16.
func lolRules() model.SkillRules {
	return model.SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
}

// TestDigitsSonNiveles: los 15 dígitos son niveles 1..15 y 1..4 es
// QWER. Se demuestra con la posición de la R: en las secuencias que la
// fuente publica como legales, el 4 cae exactamente en 6 y 11.
func TestDigitsSonNiveles(t *testing.T) {
	fix := loadSkills(t)
	for _, key := range []string{"ahri", "malphite", "soraka"} {
		f := fix[key]
		seq, ok := digitsToQWER(digitsOf(f.SkillOrderID))
		if !ok {
			t.Errorf("%s: no se pudo decodificar", f.ChampionName)
			continue
		}
		if len(seq) != 15 {
			t.Errorf("%s: secuencia de %d, se esperaban 15", f.ChampionName, len(seq))
		}
		var rLevels []int
		for i, sk := range seq {
			if sk == 'R' {
				rLevels = append(rLevels, i+1)
			}
		}
		if len(rLevels) == 0 {
			t.Errorf("%s: sin R en la secuencia", f.ChampionName)
		}
		for _, lv := range rLevels {
			if lv != 6 && lv != 11 && lv != 16 {
				t.Errorf("%s: R en nivel %d, ilegal", f.ChampionName, lv)
			}
		}
	}
}

// TestSecuenciasLegalesSeAceptan: Ahri, Malphite y Soraka producen una
// secuencia legal y conservan su prioridad.
func TestSecuenciasLegalesSeAceptan(t *testing.T) {
	fix := loadSkills(t)
	for _, key := range []string{"ahri", "malphite", "soraka"} {
		f := fix[key]
		seq, ok := digitsToQWER(digitsOf(f.SkillOrderID))
		if !ok {
			t.Fatalf("%s: no decodifica", f.ChampionName)
		}
		if err := model.ValidateSequence(seq, lolRules()); err != nil {
			t.Errorf("%s: la secuencia publicada no valida: %v", f.ChampionName, err)
		}
	}
}

// TestSecuenciaDeKaisaEsImposible: no es un fallo de mapeo. Con 6 unos
// y 6 treses y tope 5 por habilidad, ninguna ordenación es legal.
func TestSecuenciaDeKaisaEsImposible(t *testing.T) {
	f := loadSkills(t)["kaisa"]
	seq, ok := digitsToQWER(digitsOf(f.SkillOrderID))
	if !ok {
		t.Fatal("la secuencia de Kai'Sa debe decodificar (formato válido)")
	}
	if err := model.ValidateSequence(seq, lolRules()); err == nil {
		t.Fatal("la secuencia de Kai'Sa NO puede ser legal")
	}
	// La prueba independiente del orden: los conteos por sí solos la
	// hacen imposible.
	count := map[rune]int{}
	for _, sk := range seq {
		count[sk]++
	}
	if count['Q'] <= 5 && count['E'] <= 5 {
		t.Fatalf("Q=%d E=%d: se esperaba que excedieran el tope", count['Q'], count['E'])
	}
}

// TestPrioridadSobreviveASecuenciaInvalida: la prioridad es un dato
// aparte de la fuente y no se pierde porque la secuencia no valide.
func TestPrioridadSobreviveASecuenciaInvalida(t *testing.T) {
	fix := loadSkills(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		f := fix[key]
		if !isSkillPriority(f.SkillPriority) {
			t.Errorf("%s: prioridad %q mal formada", f.ChampionName, f.SkillPriority)
		}
		// Es una permutación sin repeticiones de QWER.
		seen := map[rune]bool{}
		for _, r := range f.SkillPriority {
			if seen[r] {
				t.Errorf("%s: prioridad con %c repetida", f.ChampionName, r)
			}
			seen[r] = true
		}
	}
}

// TestEntradasCorruptasSeRechazan: formatos inválidos no producen
// secuencia ni prioridad.
func TestEntradasCorruptasSeRechazan(t *testing.T) {
	invalidas := []string{
		"",                 // vacía
		"123",              // corta
		"12311413113343",   // 14 dígitos
		"1231141311334330", // 16 dígitos
		"123114131133500",  // dígito fuera de 1..4
		"abcdefghijklmno",  // sin dígitos
		"000000000000000",  // ceros
	}
	for _, s := range invalidas {
		if _, ok := digitsToQWER(s); ok {
			t.Errorf("%q no debería decodificar", s)
		}
	}
	// Prioridades corruptas.
	for _, p := range []string{"", "Q", "QQQE", "QWF", "QWERX", "qwe"} {
		if isSkillPriority(p) {
			t.Errorf("prioridad %q no debería aceptarse", p)
		}
	}
	// Y las válidas sí.
	for _, p := range []string{"QWE", "QEW", "WQE", "EQW", "QWER"} {
		if !isSkillPriority(p) {
			t.Errorf("prioridad %q debería aceptarse", p)
		}
	}
}

// TestSinDatosNoInventaSkill: sin campos de habilidad no se fabrica una
// secuencia.
func TestSinDatosNoInventaSkill(t *testing.T) {
	// Un id 0 o vacío no decodifica.
	if seq, ok := digitsToQWER(""); ok || seq != "" {
		t.Error("un id vacío no debe producir secuencia")
	}
	if seq, ok := digitsToQWER("0"); ok || seq != "" {
		t.Error("un id 0 no debe producir secuencia")
	}
}

// TestDeterministico: decodificar dos veces da lo mismo.
func TestDeterministico(t *testing.T) {
	fix := loadSkills(t)
	for _, key := range []string{"kaisa", "ahri", "malphite", "soraka"} {
		f := fix[key]
		a, ok1 := digitsToQWER(digitsOf(f.SkillOrderID))
		b, ok2 := digitsToQWER(digitsOf(f.SkillOrderID))
		if ok1 != ok2 || a != b {
			t.Errorf("%s: decodificación no determinista", f.ChampionName)
		}
	}
}

// digitsOf formatea el id del fixture como la cadena de dígitos que usa
// el parser real: FormatFloat 'f' con 0 decimales (el JSON puede dar
// notación científica para enteros grandes).
func digitsOf(v float64) string {
	return strconv.FormatFloat(v, 'f', 0, 64)
}
