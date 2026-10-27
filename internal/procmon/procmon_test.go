package procmon

import (
	"context"
	"testing"
	"time"
)

// TestTransicionesYEmisionSoloEnCambio cubre el flujo lógico completo:
// IDLE→activo emite Started una vez, ciclos sin cambio no emiten
// nada, y el cierre emite Ended y vuelve a IDLE.
func TestTransicionesYEmisionSoloEnCambio(t *testing.T) {
	ch := make(chan Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Script de estados por ciclo.
	script := []Game{"", "", LOL, LOL, LOL, "", "", DEADLOCK, ""}
	cycle := 0
	go Run(ctx, time.Millisecond, func() Game {
		if cycle >= len(script) {
			return ""
		}
		g := script[cycle]
		cycle++
		return g
	}, func(Game) string { return "" }, ch)

	var got []Event
	deadline := time.After(2 * time.Second)
	for len(got) < 4 {
		select {
		case ev := <-ch:
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("eventos incompletos: %+v", got)
		}
	}
	want := []Event{
		{Type: Started, Game: LOL},
		{Type: Ended, Game: LOL},
		{Type: Started, Game: DEADLOCK},
		{Type: Ended, Game: DEADLOCK},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("evento %d = %+v, se esperaba %+v", i, got[i], w)
		}
	}
	// Sin eventos extra tras el último cambio (dar margen).
	select {
	case ev := <-ch:
		t.Errorf("evento inesperado: %+v", ev)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestCancelacionCortaElBucle(t *testing.T) {
	ch := make(chan Event, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, time.Millisecond, func() Game { return LOL }, func(Game) string { return "" }, ch)
		close(done)
	}()
	cancel()
	select {
	case <-done:
		// el bucle salió limpiamente
	case <-time.After(time.Second):
		t.Fatal("el worker no terminó tras cancelar (riesgo de zombie)")
	}
}

func TestDetectorRealNoRevienta(t *testing.T) {
	// El detector real corre en el host: no debe paniquear y debe
	// terminar rápido (los procesos protegidos se saltan en silencio).
	start := time.Now()
	g := NewProcessDetector()()
	if time.Since(start) > time.Second {
		t.Errorf("escaneo lento: %v (objetivo: milisegundos)", time.Since(start))
	}
	if g != "" && g != LOL && g != DEADLOCK {
		t.Errorf("juego inválido: %q", g)
	}
}

func TestEstadoJuegoRedondo(t *testing.T) {
	if gameOf(stateFor(LOL)) != LOL || gameOf(stateFor(DEADLOCK)) != DEADLOCK || gameOf(StateIdle) != "" {
		t.Error("roundtrip estado↔juego roto")
	}
}

// TestDeteccionPersonajeSoloEnCambio cubre el flujo del personaje: se
// sondea solo con juego activo, se emite solo cuando cambia, y el
// cierre del juego resetea el personaje (una partida nueva vuelve a
// emitir el mismo personaje).
func TestDeteccionPersonajeSoloEnCambio(t *testing.T) {
	ch := make(chan Event, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Guion por ciclo: (juego detectado, personaje sondeado). detect y
	// probe comparten el contador de ciclo, que avanza con el sondeo.
	script := []struct {
		game Game
		char string
	}{
		{"", ""},            // idle: no se sondea el personaje
		{LOL, ""},           // Started, todavía sin personaje
		{LOL, "Ahri"},       // CharacterChanged
		{LOL, "Ahri"},       // sin cambios: nada
		{LOL, "MonkeyKing"}, // cambio de personaje
		{"", ""},            // Ended y reset
		{LOL, "Ahri"},       // partida nueva: vuelve a emitir
	}
	cycle := 0
	go Run(ctx, time.Millisecond,
		func() Game {
			if cycle >= len(script) {
				return ""
			}
			g := script[cycle].game
			cycle++
			return g
		},
		func(g Game) string {
			i := cycle - 1 // ciclo en curso: detect ya avanzó el contador
			if i >= 0 && i < len(script) && script[i].game == "" {
				t.Errorf("personaje sondeado en ciclo idle (ciclo %d)", i)
			}
			if i >= 0 && i < len(script) {
				return script[i].char
			}
			return ""
		},
		ch)

	var got []Event
	deadline := time.After(2 * time.Second)
	// 6 eventos del guion + el Ended final (el guion agotado deja el
	// juego activo, y el siguiente ciclo lo cierra).
	for len(got) < 7 {
		select {
		case ev := <-ch:
			got = append(got, ev)
		case <-deadline:
			t.Fatalf("eventos incompletos: %+v", got)
		}
	}
	want := []Event{
		{Type: Started, Game: LOL},
		{Type: Char, Game: LOL, Character: "Ahri"},
		{Type: Char, Game: LOL, Character: "MonkeyKing"},
		{Type: Ended, Game: LOL},
		{Type: Started, Game: LOL},
		{Type: Char, Game: LOL, Character: "Ahri"},
		{Type: Ended, Game: LOL}, // cierre del guion agotado
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("evento %d = %+v, se esperaba %+v", i, got[i], w)
		}
	}
	select {
	case ev := <-ch:
		t.Errorf("evento inesperado: %+v", ev)
	case <-time.After(20 * time.Millisecond):
	}
}
