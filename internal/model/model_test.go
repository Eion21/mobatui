package model

import "testing"

func TestValidateSequenceLoL(t *testing.T) {
	rules := GameLoL.SkillRules()
	// Ahri mid: Q en 1,4,5,7,9 · W en 2,8,10,12,13 · E en 3,14,15,17,18 · R en 6,11,16
	if _, err := ParseSequence("QWEQQRQWQWRWWEEREE", rules); err != nil {
		t.Fatalf("secuencia válida rechazada: %v", err)
	}
	cases := map[string]string{
		"longitud errónea": "QWEQQR",
		"R en nivel 1":     "RWEQQRQWQWRWWEEREE",
		"R en nivel 5":     "QWEQRRQWQWRWWEEREE",
		"letra inválida":   "XWEQQRQWQWRWWEEREE",
		"recuento mal":     "QWEQERQWQWRWWEWREE",
		"doble R temprana": "QWEQQRRQWQWRWWEERE",
	}
	for name, seq := range cases {
		if _, err := ParseSequence(seq, rules); err == nil {
			t.Errorf("%s: secuencia %q aceptada y debía rechazarse", name, seq)
		}
	}
}

func TestValidateSequenceDeadlock(t *testing.T) {
	rules := GameDeadlock.SkillRules()
	// 1 maxea en 9, 2 en 13, 3 en 18; la 4 repartida.
	good := "121314121242233433"
	if _, err := ParseSequence(good, rules); err != nil {
		t.Fatalf("secuencia Deadlock válida rechazada: %v", err)
	}
	// Sin bloqueos de nivel: la 4 puede ir donde sea…
	free := "444111112222233333"
	if _, err := ParseSequence(free, rules); err != nil {
		t.Fatalf("la 4 debe ser libre de nivel en Deadlock: %v", err)
	}
	// …pero Q/W/E/R de LoL no pertenecen al alfabeto.
	if _, err := ParseSequence("QWEQQRQWQWRWWEEREE", rules); err == nil {
		t.Error("secuencia de LoL aceptada en reglas de Deadlock")
	}
	// Recuento: cinco puntos de 4 sobran uno.
	if _, err := ParseSequence("444441211212131313", rules); err == nil {
		t.Error("4 con más puntos de los permitidos aceptada")
	}
}

func TestPriorityFromSequenceLoL(t *testing.T) {
	rules := GameLoL.SkillRules()
	prio, err := PriorityFromSequence("QWEQQRQWQWRWWEEREE", rules)
	if err != nil {
		t.Fatal(err)
	}
	want := []rune{'Q', 'W', 'E'}
	for i := range want {
		if prio[i] != want[i] {
			t.Fatalf("prioridad = %c, se esperaba %c", prio[i], want[i])
		}
	}
	// Lee Sin clásico: Q(9), E(13), W(18).
	prio, _ = PriorityFromSequence("QWEQQRQEQEREEWWRWW", rules)
	want = []rune{'Q', 'E', 'W'}
	for i := range want {
		if prio[i] != want[i] {
			t.Fatalf("prioridad = %c, se esperaba %c", prio[i], want[i])
		}
	}
}

func TestPriorityFromSequenceDeadlock(t *testing.T) {
	rules := GameDeadlock.SkillRules()
	// 1 maxea en 9, 2 en 13, 4 en 16, 3 en 18 → 1,2,4,3 (sin R forzada,
	// la 4 también entra en la prioridad).
	prio, err := PriorityFromSequence("121314121242233433", rules)
	if err != nil {
		t.Fatal(err)
	}
	want := []rune{'1', '2', '4', '3'}
	for i := range want {
		if prio[i] != want[i] {
			t.Fatalf("prioridad = %c, se esperaba %c", prio[i], want[i])
		}
	}
	// 2 > 1 > 4 > 3: 2 maxea en 9, 1 en 12, 4 en 16, 3 en 18.
	prio, _ = PriorityFromSequence("212324212141133433", rules)
	want = []rune{'2', '1', '4', '3'}
	for i := range want {
		if prio[i] != want[i] {
			t.Fatalf("prioridad = %c, se esperaba %c", prio[i], want[i])
		}
	}
}

// TestDefinitivaRetrasadaEsLegal: el nivel de la lista es el de
// DESBLOQUEO, no uno obligatorio. Retrasar el punto es legal y ocurre en
// los datos reales (1 de cada 20 secuencias de Ahri y Soraka sube la
// segunda R en el nivel 12).
func TestDefinitivaRetrasadaEsLegal(t *testing.T) {
	rules := SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
	// R en 6/11 (normal) y R en 7/13 (retrasada): ambas legales.
	for _, c := range []struct{ name, seq string }{
		{"normal", "QQQQQREEEEREWWW"},
		{"retrasada", "QQQQQEREEEEWRWW"},
	} {
		if err := ValidateSequence(c.seq, rules); err != nil {
			t.Errorf("%s: %q debía ser legal: %v", c.name, c.seq, err)
		}
		if _, err := PriorityFromSequence(c.seq, rules); err != nil {
			t.Errorf("%s: la prioridad debía derivarse: %v", c.name, err)
		}
	}
}

// TestDefinitivaPrematuraEsIlegal: subir la R antes de su nivel de
// desbloqueo sí se rechaza.
func TestDefinitivaPrematuraEsIlegal(t *testing.T) {
	rules := SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
	seq := "QQQRQQEEEEREWWW" // R en el nivel 4
	if err := ValidateSequence(seq, rules); err == nil {
		t.Errorf("la R prematura debía rechazarse: %q", seq)
	}
}

// TestKaisaSigueRechazandose: la corrección del retraso no puede
// aceptar una secuencia con 6 puntos en Q y 6 en E.
func TestKaisaSigueRechazandose(t *testing.T) {
	rules := SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
	seq := "QWEQQRQEQQEEREE" // Q=6, W=1, E=6, R=2
	if err := ValidateSequence(seq, rules); err == nil {
		t.Fatalf("Kai'Sa (Q=6, E=6) debe seguir rechazándose: %q", seq)
	}
}

// TestTopeDePuntosSigueVigente: no se permiten rangos por encima del
// máximo de la habilidad ni más puntos de los posibles.
func TestTopeDePuntosSigueVigente(t *testing.T) {
	rules := SkillRules{
		Alphabet:  []rune{'Q', 'W', 'E', 'R'},
		Length:    15,
		MaxPoints: map[rune]int{'Q': 5, 'W': 5, 'E': 5, 'R': 3},
		LockedAt:  map[rune][]int{'R': {6, 11, 16}},
	}
	if err := ValidateSequence("QQQQQREEEEREWWR", rules); err == nil {
		t.Error("cuatro puntos de R deben rechazarse")
	}
	if err := ValidateSequence("QQQQQQUWEEEREWW", rules); err == nil {
		t.Error("seis puntos de Q deben rechazarse")
	}
}
