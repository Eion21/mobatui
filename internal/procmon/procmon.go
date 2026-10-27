// Package procmon mantiene un worker de fondo de ultra-bajo consumo
// que detecta el inicio y cierre de los clientes de juego (LoL y
// Deadlock) escaneando los procesos del sistema. Emite un evento SOLO
// cuando cambia el estado (no en cada ciclo) y se acopla a un
// context.Context: al cancelarse (Ctrl+C), el bucle se rompe y el
// worker muere con la app, sin dejar procesos zombie.
package procmon

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/process"
)

// Game identifica el juego detectado.
type Game string

const (
	LOL      Game = "lol"
	DEADLOCK Game = "deadlock"
)

// EventType distingue inicio y cierre del cliente.
type EventType string

const (
	Started EventType = "GAME_STARTED"
	Ended   EventType = "GAME_ENDED"
	Char    EventType = "CHARACTER_CHANGED"
)

// Event es la señal hacia el hilo principal. Character solo viene
// informado en Char.
type Event struct {
	Type      EventType
	Game      Game
	Character string
}

// State es la máquina de estados del worker.
type State int

const (
	StateIdle State = iota
	StateLoLActive
	StateDeadlockActive
)

// Executables objetivo: el juego real, no el cliente de Riot. Para
// Deadlock se acepta también el binario nativo de Linux (project8 sin
// extensión).
var Executables = map[Game][]string{
	LOL:      {"League of Legends.exe"},
	DEADLOCK: {"project8.exe", "project8"},
}

// DefaultInterval: menos desperdiciaría CPU; más retrasaría la carga.
const DefaultInterval = 5 * time.Second

// Detector decide qué juego está corriendo ("" = ninguno). Inyectable
// para tests; el detector real escanea procesos con gopsutil.
type Detector func() Game

// CharacterProber pregunta qué personaje está jugando el usuario en el
// juego activo ("" = desconocido o sin partida). Se sondea solo con
// juego activo. Inyectable para tests; el prober real consulta las
// APIs locales de cada juego (NewCharacterProber).
type CharacterProber func(game Game) string

// NewProcessDetector devuelve un Detector real basado en gopsutil.
// Solo pide el nombre del proceso (nada de memoria, disco o red) para
// que el escaneo tarde milisegundos; los procesos protegidos
// (antivirus, drivers) fallan al leer el nombre y se ignoran en
// silencio.
func NewProcessDetector() Detector {
	return func() Game {
		procs, err := process.Processes()
		if err != nil {
			return "" // sin acceso a la tabla de procesos: como IDLE
		}
		for _, p := range procs {
			name, err := p.Name()
			if err != nil {
				continue // AccessDenied y amigos: silencio y continuar
			}
			for g, names := range Executables {
				for _, target := range names {
					if name == target {
						return g
					}
				}
			}
		}
		return ""
	}
}

// Run es el bucle del worker: escanea cada intervalo, mantiene el
// estado local y emite por ch SOLO los cambios (juego y personaje). Se
// bloquea hasta que ctx se cancele. Un prober nil desactiva la
// detección del personaje.
func Run(ctx context.Context, interval time.Duration, detect Detector, probe CharacterProber, ch chan<- Event) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	state := StateIdle
	char := ""
	for {
		select {
		case <-ctx.Done():
			return // salida de seguridad: sin procesos zombie
		case <-ticker.C:
			g := detect()
			switch {
			case g != "" && state == StateIdle:
				state = stateFor(g)
				char = ""
				emit(ctx, ch, Event{Type: Started, Game: g})
			case g == "" && state != StateIdle:
				prev := gameOf(state)
				state = StateIdle
				char = ""
				emit(ctx, ch, Event{Type: Ended, Game: prev})
			case g != "" && state != StateIdle && gameOf(state) != g:
				// Cambió de juego sin cerrar el anterior.
				char = ""
				emit(ctx, ch, Event{Type: Ended, Game: gameOf(state)})
				state = stateFor(g)
				emit(ctx, ch, Event{Type: Started, Game: g})
			}
			// Personaje: solo se sondea con juego activo; el resultado
			// vacío actualiza el estado en silencio (no hay evento).
			if state != StateIdle && probe != nil {
				if c := probe(gameOf(state)); c != char {
					char = c
					if c != "" {
						emit(ctx, ch, Event{Type: Char, Game: gameOf(state), Character: c})
					}
				}
			}
		}
	}
}

func emit(ctx context.Context, ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	case <-ctx.Done():
	}
}

func stateFor(g Game) State {
	switch g {
	case LOL:
		return StateLoLActive
	case DEADLOCK:
		return StateDeadlockActive
	}
	return StateIdle
}

func gameOf(s State) Game {
	switch s {
	case StateLoLActive:
		return LOL
	case StateDeadlockActive:
		return DEADLOCK
	}
	return ""
}
