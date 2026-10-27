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
