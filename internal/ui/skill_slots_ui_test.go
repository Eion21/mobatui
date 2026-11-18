package ui

// Comprobación de la salida FINAL de la TUI contra el fixture
// independiente de posiciones de habilidad.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/game"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// wantSlotsUI replica el fixture de referencia (escrito a mano).
var wantSlotsUI = map[string][4]string{
	"Solomon":       {"Setup Bishop", "Kingside Knight", "Rook Battery", "Develop Queen"},
	"Paradox":       {"Pulse Grenade", "Time Wall", "Kinetic Carbine", "Paradoxical Swap"},
	"Venator":       {"Consecrating Grenade", "Gutshot", "Hex-Lined Snap Trap", "Ira Domini"},
	"Violet":        {"Living Ink", "Flow State", "Still Life", "Abstract Landscape"},
	"Deadman Danny": {"Strix Salvo", "Chimera Mk1", "Jettison Fuel", "Close The Deal"},
}

func skillsSession(t *testing.T) *Session {
	t.Helper()
	dir, _ := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	if _, err := os.Stat(dir); err != nil {
		t.Skip("sin caché")
	}
	c, _ := cache.New(dir)
	c.Offline = true
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	api := deadlock.NewAPI(c, fix)
	cbs, err := api.LoadBuilds()
	if err != nil {
		t.Skipf("carga: %v", err)
	}
	builds := clean.BuildAll(cbs, clean.Options{MaxItems: 9999})
	names, _ := api.Items()
	heroes, _ := api.Heroes()
	meta, _ := api.ItemDetails()
	if meta == nil {
		meta = map[int]staticdata.ItemInfo{}
	}
	return &Session{
		Game: model.GameDeadlock, Builds: builds,
		Names: Names{Items: names}, ItemArt: map[artKey]string{}, ItemMeta: meta,
		Routes:    map[string][]RouteStep{},
		Champions: groupByChampion(builds, heroes, game.For(model.GameDeadlock).Roles),
	}
}

// TestUISkillSlotsMatchFixture: la leyenda y la cuadrícula de la TUI
// muestran cada habilidad en su posición canónica.
func TestUISkillSlotsMatchFixture(t *testing.T) {
	s := skillsSession(t)
	for heroName, want := range wantSlotsUI {
		t.Run(heroName, func(t *testing.T) {
			var champ *ChampEntry
			for _, e := range s.Champions {
				if e.Key == heroName {
					champ = e
				}
			}
			if champ == nil {
				t.Skipf("%s no está en la sesión", heroName)
			}
			d := newDetailView(champ, s)
			d.width, d.height = 140, 50
			d.tab = 3
			d.layout(0)
			d.sync()
			plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))

			// La leyenda debe listar "1 <nombre>" en el slot correcto.
			for i, exp := range want {
				slot := string(rune('1' + i))
				wantLabel := slot + " " + exp
				if !strings.Contains(plain, wantLabel) {
					t.Errorf("falta %q en la vista", wantLabel)
				}
			}
			// Y no debe avisar de fallback: estos héroes tienen orden
			// canónico.
			if strings.Contains(plain, "orden por uso") {
				t.Errorf("la vista avisa de fallback con orden canónico disponible")
			}
		})
	}
}

// TestUIMaxLineMatchesSlots: el texto "max 1 > 2 > ..." usa los slots
// reales, no índices internos.
func TestUIMaxLineMatchesSlots(t *testing.T) {
	s := skillsSession(t)
	var champ *ChampEntry
	for _, e := range s.Champions {
		if e.Key == "Solomon" {
			champ = e
		}
	}
	if champ == nil {
		t.Skip("Solomon no está")
	}
	b := champ.Build(champ.MainRoleIdx())
	d := newDetailView(champ, s)
	d.width, d.height = 140, 50
	d.tab = 3
	d.layout(0)
	d.sync()
	plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))

	// La prioridad del modelo debe aparecer tal cual en la vista.
	prio := string(b.SkillOrder.Priority)
	if prio == "" {
		t.Skip("sin prioridad")
	}
	spaced := strings.Join(strings.Split(prio, ""), " > ")
	if !strings.Contains(plain, spaced) {
		t.Errorf("la vista no muestra la prioridad %q", spaced)
	}
	// Cada slot de la prioridad debe tener nombre en la leyenda.
	for _, r := range b.SkillOrder.Priority {
		if b.AbilityNames[r] == "" {
			t.Errorf("slot %c de la prioridad sin nombre", r)
		}
	}
}

// TestUIFallbackIsLabelled: cuando los slots no son canónicos, la vista
// lo dice en vez de presentarlos como posiciones del juego.
func TestUIFallbackIsLabelled(t *testing.T) {
	s := skillsSession(t)
	var champ *ChampEntry
	for _, e := range s.Champions {
		if e.Key == "Silver" {
			champ = e
		}
	}
	if champ == nil {
		t.Skip("Silver no está")
	}
	b := champ.Build(champ.MainRoleIdx())
	if b == nil {
		t.Skip("sin build")
	}
	if b.CanonicalSkillOrder {
		t.Skip("Silver resolvió canónico en esta caché")
	}
	d := newDetailView(champ, s)
	d.width, d.height = 140, 50
	d.tab = 3
	d.layout(0)
	d.sync()
	plain := collapsePlain(ansiRE.ReplaceAllString(d.View(), ""))
	if !strings.Contains(plain, "orden por uso") {
		t.Error("la vista no avisa de que el orden no es canónico")
	}
}
