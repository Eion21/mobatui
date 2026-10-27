// Command render vuelca la lista y una vista de detalle a stdout sin
// TUI interactiva — útil para verificar la salida visual (y en CI).
//
// Uso: go run ./cmd/render [juego] [clave]
//
//	juego: lol-api (defecto, datos reales) | lol (fixtures) |
//	       deadlock-api (real) | deadlock (fixtures)
//	clave: clave del campeón/héroe (Ahri, Abrams…)
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/Eion21/builds-tui/internal/cache"
	"github.com/Eion21/builds-tui/internal/ddragon"
	"github.com/Eion21/builds-tui/internal/deadlock"
	"github.com/Eion21/builds-tui/internal/lolalytics"
	"github.com/Eion21/builds-tui/internal/model"
	"github.com/Eion21/builds-tui/internal/staticdata"
	"github.com/Eion21/builds-tui/internal/ugg"
	"github.com/Eion21/builds-tui/internal/ui"
)

func main() {
	gameID := model.GameLoL
	mode := "api"
	if len(os.Args) > 1 && os.Args[1] != "" {
		switch os.Args[1] {
		case "lol":
			gameID, mode = model.GameLoL, "fixtures"
		case "lol-api":
			gameID, mode = model.GameLoL, "api"
		case "deadlock":
			gameID, mode = model.GameDeadlock, "fixtures"
		case "deadlock-api":
			gameID, mode = model.GameDeadlock, "api"
		}
	}
	key := "Ahri"
	if len(os.Args) > 2 {
		key = os.Args[2]
	}

	c, err := cache.New(filepath.Join("data", "cache"))
	if err != nil {
		fail(err)
	}
	c.Offline = os.Getenv("BUILDS_OFFLINE") != ""

	ddClient := ddragon.New(c, "testdata/ddragon", "es_ES")
	loader := ui.Loader{
		UggDir:      "testdata/ugg",
		DeadlockDir: "testdata/deadlock/builds",
		Providers: map[model.Game]staticdata.Provider{
			model.GameLoL:      ddClient,
			model.GameDeadlock: deadlock.New("testdata/deadlock"),
		},
	}
	if mode == "api" {
		switch gameID {
		case model.GameDeadlock:
			api := deadlock.NewAPI(c, "testdata/deadlock")
			loader.Providers[model.GameDeadlock] = api
			loader.BuildLoaders = map[model.Game]func() ([]*model.ChampionBuilds, error){
				model.GameDeadlock: api.LoadBuilds,
			}
		default:
			lol := lolalytics.New(c)
			loader.BuildLoaders = map[model.Game]func() ([]*model.ChampionBuilds, error){
				model.GameLoL: func() ([]*model.ChampionBuilds, error) {
					champs, err := ddClient.Champions()
					if err != nil {
						return ugg.LoadDir("testdata/ugg")
					}
					byID := make(map[int]string, len(champs))
					for _, ch := range champs {
						if id, err := strconv.Atoi(ch.Key); err == nil {
							byID[id] = ch.ID
						}
					}
					return lol.LoadBuilds(byID, func() ([]*model.ChampionBuilds, error) {
						return ugg.LoadDir("testdata/ugg")
					})
				},
			}
			loader.SetReporter = func(fn func(done, total int, name string)) bool {
				lol.OnProgress(fn)
				return true
			}
		}
	}

	s, err := ui.LoadSession(gameID, loader)
	if err != nil {
		fail(err)
	}

	fmt.Printf("═══ %s ═══\n", s.Game.DisplayName())
	for _, e := range s.Champions {
		mark := " "
		if e.Key == key {
			mark = "▸"
		}
		fmt.Printf("%s %-16s (", mark, e.Name)
		for i, r := range e.Roles {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Print(r)
		}
		fmt.Println(")")
	}

	fmt.Println("\n═══ DETALLE ═══")
	for _, e := range s.Champions {
		if e.Key != key {
			continue
		}
		d := ui.NewDetailForRender(e, s)
		if len(os.Args) > 3 {
			d = d.SelectTabForRender(os.Args[3])
		}
		fmt.Print(d.View())
		fmt.Println()
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "render:", err)
	os.Exit(1)
}
