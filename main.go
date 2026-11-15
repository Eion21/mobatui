// Builds es una TUI de builds de MOBAs: League of Legends (vía u.gg)
// y Deadlock (vía trackers comunitarios). Depura los datos por score
// de Wilson, resuelve los IDs a nombres reales con proveedores
// estáticos (Data Dragon con caché / copia local) y lo presenta con
// arte ASCII de campeones e ítems.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/lolalytics"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
	"github.com/Eion21/mobatui/internal/ugg"
	"github.com/Eion21/mobatui/internal/ui"
)

func main() {
	var (
		dataDir     = flag.String("data", "data", "directorio de caché runtime")
		uggDir      = flag.String("ugg", "testdata/ugg", "directorio de datos/dumps de u.gg")
		deadlockDir = flag.String("deadlock", "testdata/deadlock/builds", "directorio de builds de Deadlock")
		locale      = flag.String("locale", "es_ES", "idioma de Data Dragon")
		offline     = flag.Bool("offline", false, "no acceder a la red: solo caché y fixtures")
		refresh     = flag.Bool("refresh", false, "invalidar la caché estática antes de arrancar")
	)
	flag.Parse()

	cacheDir := filepath.Join(*dataDir, "cache")
	os.MkdirAll(cacheDir, 0o755)

	c, err := cache.New(cacheDir)
	if err != nil {
		fatal(err)
	}
	c.Offline = *offline
	if *refresh {
		_ = c.Invalidate("https://ddragon.leagueoflegends.com/api/versions.json")
	}

	// Deadlock: datos reales de deadlock-api.com (con caché y respaldo
	// en fixtures); LoL: datos reales de lolalytics.com (u.gg está tras
	// Cloudflare), con fixtures de u.gg como respaldo.
	deadlockAPI := deadlock.NewAPI(c, "testdata/deadlock")
	lolClient := lolalytics.New(c)
	ddClient := ddragon.New(c, "testdata/ddragon", *locale)

	// Ruta de compra de Deadlock: el adaptador vive en main y responde
	// al RunsLoader de la UI bajo demanda.
	runsLoader := deadlockRuns{api: deadlockAPI}

	loader := ui.Loader{
		UggDir:      *uggDir,
		DeadlockDir: *deadlockDir,
		Providers: map[model.Game]staticdata.Provider{
			model.GameLoL:      ddClient,
			model.GameDeadlock: deadlockAPI,
		},
		BuildLoaders: map[model.Game]func() ([]*model.ChampionBuilds, error){
			model.GameDeadlock: deadlockAPI.LoadBuilds,
			model.GameLoL: func() ([]*model.ChampionBuilds, error) {
				champs, err := ddClient.Champions()
				if err != nil {
					return ugg.LoadDir(*uggDir)
				}
				byID := make(map[int]string, len(champs))
				for _, ch := range champs {
					if id, err := strconv.Atoi(ch.Key); err == nil {
						byID[id] = ch.ID
					}
				}
				return lolClient.LoadBuilds(byID, func() ([]*model.ChampionBuilds, error) {
					return ugg.LoadDir(*uggDir)
				})
			},
		},
		// Progreso de la carga en la pantalla de carga: "124/200 · Ahri"
		// (LoL, páginas campeón×carril) o "23/26 · Abrams" (Deadlock,
		// héroes).
		SetReporter: func(fn func(done, total int, name string)) bool {
			lolClient.OnProgress(fn)
			deadlockAPI.OnProgress(fn)
			return true
		},
		// Ruta de compra de Deadlock (secuencias reales de partidas,
		// bajo demanda al abrir el héroe).
		RunsLoaders: map[model.Game]ui.RunsLoader{
			model.GameDeadlock: runsLoader,
		},
	}

	p := tea.NewProgram(ui.NewApp(loader),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "builds:", err)
	os.Exit(1)
}

// deadlockRuns adapta el APIClient de Deadlock al RunsLoader de la UI:
// secuencias reales de compra del héroe destiladas en una ruta de
// consenso, bajo demanda al abrir el detalle.
type deadlockRuns struct{ api *deadlock.APIClient }

// LoadRuns implementa ui.RunsLoader. Con muestra corta devuelve sin
// pasos: la UI muestra la build sin números de ruta (mejor sin números
// que números de 20 partidas).
func (r deadlockRuns) LoadRuns(champKey string) ([]ui.RouteStep, error) {
	id := r.api.HeroID(champKey)
	if id == 0 {
		return nil, fmt.Errorf("héroe desconocido: %s", champKey)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	runs, err := r.api.FetchRuns(ctx, id, 0, 250)
	if err != nil {
		return nil, err
	}
	if len(runs) < 100 {
		return []ui.RouteStep{}, nil
	}
	steps := deadlock.BuildRoute(runs, 0.3)
	out := make([]ui.RouteStep, 0, len(steps))
	for _, st := range steps {
		out = append(out, ui.RouteStep{
			Item: st.Item, N: st.N, Share: st.Share,
			MedT: st.MedT, MedNet: st.MedNet,
			PosLo: st.PosLo, PosHi: st.PosHi,
			UpgradeOf:    st.UpgradeOf,
			ChainRate:    st.ChainRate,
			UpgradedRate: st.UpgradedRate,
		})
	}
	return out, nil
}
