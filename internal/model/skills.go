package model

import (
	"fmt"
	"sort"
)

// ParseSequence convierte una secuencia tipo "QWEQQR…" o "121314…" a su
// forma validada según las reglas del juego.
func ParseSequence(s string, rules SkillRules) (string, error) {
	runes := []rune(s)
	if len(runes) != rules.Length {
		return "", fmt.Errorf("secuencia %q: se esperaban %d niveles, hay %d", s, rules.Length, len(runes))
	}
	if err := ValidateSequence(s, rules); err != nil {
		return "", err
	}
	return s, nil
}

// ValidateSequence comprueba las reglas matemáticas del orden de
// habilidades del juego: solo habilidades del alfabeto, tope de puntos
// por habilidad (la longitud de la secuencia fuerza el reparto total)
// y habilidades bloqueadas, cada una en un nivel MÍNIMO de desbloqueo.
//
// Sobre las habilidades bloqueadas (la R de LoL, que se desbloquea en
// 6/11/16): el nivel de la lista es el PRIMER nivel en que se puede
// subir, NO uno obligatorio. Retrasar el punto es legal — y ocurre:
// con datos reales de Lolalytics, 1 de cada 20 secuencias de Ahri y
// Soraka sube la segunda R en el nivel 12 en lugar del 11. Exigir el
// nivel exacto rechazaba esas secuencias válidas.
func ValidateSequence(seq string, rules SkillRules) error {
	counts := map[rune]int{}
	for _, r := range rules.Alphabet {
		counts[r] = 0
	}
	lockedIdx := map[rune]int{}
	for lvl, sk := range []rune(seq) {
		if _, ok := counts[sk]; !ok {
			return fmt.Errorf("nivel %d: habilidad %q fuera del alfabeto %s", lvl+1, sk, string(rules.Alphabet))
		}
		counts[sk]++
		if counts[sk] > rules.MaxPoints[sk] {
			return fmt.Errorf("nivel %d: %c supera su tope de %d puntos", lvl+1, sk, rules.MaxPoints[sk])
		}
		if lock, isLocked := rules.LockedAt[sk]; isLocked {
			if len(lock) == 0 {
				return fmt.Errorf("nivel %d: habilidad %q no puede subirse", lvl+1, sk)
			}
			// El punto N de una habilidad bloqueada no puede ir antes de
			// su nivel de desbloqueo, pero sí después (retraso legal).
			i := lockedIdx[sk]
			if i >= len(lock) {
				return fmt.Errorf("nivel %d: %q supera sus %d puntos posibles",
					lvl+1, sk, len(lock))
			}
			if lvl+1 < lock[i] {
				return fmt.Errorf("nivel %d: %q no puede subirse antes del nivel %d",
					lvl+1, sk, lock[i])
			}
			lockedIdx[sk]++
		}
	}
	return nil
}

// PriorityFromSequence deriva el orden de maximización (p.ej. [Q W E]
// o [1 2 3]): gana primero la habilidad que alcanza su último punto
// antes; se desempata comparando el penúltimo punto, y así.
func PriorityFromSequence(seq string, rules SkillRules) ([]rune, error) {
	if err := ValidateSequence(seq, rules); err != nil {
		return nil, err
	}
	bySkill := map[rune][]int{}
	for lvl, sk := range []rune(seq) {
		bySkill[sk] = append(bySkill[sk], lvl+1)
	}
	type skillLevels struct {
		sk     rune
		levels []int // niveles (1-based) en los que se sube, ascendente
	}
	skills := make([]skillLevels, 0, len(rules.Alphabet))
	for _, r := range rules.Alphabet {
		// Las habilidades bloqueadas a niveles fijos (la R de LoL) son
		// forzadas: no aportan información de prioridad.
		if _, locked := rules.LockedAt[r]; locked {
			continue
		}
		skills = append(skills, skillLevels{sk: r, levels: bySkill[r]})
	}
	// Menor nivel del último punto primero; a igualdad, el penúltimo,
	// etc. Las habilidades con menos puntos (p.ej. la 4 de Deadlock)
	// completan antes y van delante.
	sort.SliceStable(skills, func(i, j int) bool {
		a, b := skills[i].levels, skills[j].levels
		for k := 0; k < min(len(a), len(b)); k++ {
			ai, bi := a[len(a)-1-k], b[len(b)-1-k]
			if ai != bi {
				return ai < bi
			}
		}
		return len(a) < len(b)
	})
	prio := make([]rune, len(skills))
	for i, s := range skills {
		prio[i] = s.sk
	}
	return prio, nil
}
