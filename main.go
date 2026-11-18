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
	"github.com/Eion21/mobatui/internal/updatecheck"
	"github.com/Eion21/mobatui/internal/version"
)

// defaultDataDir resuelve la caché runtime fuera del directorio de
// trabajo. Un binario instalado con `go install` no corre en el repo, así
// que "data" relativo escribiría en cualquier sitio: se usa el directorio
// de caché del usuario. Si no se puede resolver, cae a "data".
func defaultDataDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "mobatui")
	}
	return "data"
}

// defaultFixtureDir resuelve los fixtures empaquetados junto al binario.
// Los fixtures son datos de prueba reproducibles: si no están (instalación
// global), la app funciona igual con la caché y la red.
func defaultFixtureDir(sub string) string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Join("testdata", sub)
	}
	candidate := filepath.Join(filepath.Dir(exe), "testdata", sub)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return filepath.Join("testdata", sub)
}

func main() {
	var (
		dataDir       = flag.String("data", defaultDataDir(), "directorio de caché runtime")
		uggDir        = flag.String("ugg", defaultFixtureDir("ugg"), "directorio de datos/dumps de u.gg")
		deadlockDir   = flag.String("deadlock", defaultFixtureDir(filepath.Join("deadlock", "builds")), "directorio de builds de Deadlock")
		locale        = flag.String("locale", "es_ES", "idioma de Data Dragon")
		offline       = flag.Bool("offline", false, "no acceder a la red: solo caché y fixtures")
		refresh       = flag.Bool("refresh", false, "invalidar la caché estática antes de arrancar")
		showVersion   = flag.Bool("version", false, "mostrar la versión y salir")
		noUpdateCheck = flag.Bool("no-update-check", false, "no comprobar si hay una versión nueva")
	)
	// Añade la versión al texto de --help y acepta -version además de
	// --version (flag ya trata ambos guiones por igual).
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "%s — builds y estadísticas de MOBA en la terminal\n\n", version.Get().Line())
		fmt.Fprintf(flag.CommandLine.Output(), "Uso:\n  mobatui [opciones]\n\nOpciones:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	// La versión se resuelve y se imprime ANTES de crear la caché, cargar
	// datos o arrancar la TUI: no debe tocar la red ni el disco.
	if *showVersion {
		fmt.Print(version.Get().Verbose())
		return
	}

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

	// Comprobación de versión: desactivada por -no-update-check o por
	// -offline. Reutiliza la caché de la app con TTL de 24 h, así que
	// tampoco repite peticiones ni envenena la caché de builds.
	if !*noUpdateCheck && !*offline {
		uc := &updatecheck.Client{
			Cache:   c,
			Current: version.Get().Version,
			TTL:     updatecheck.DefaultTTL,
			Timeout: updatecheck.DefaultTimeout,
		}
		loader.UpdateCheck = func(ctx context.Context) (ui.UpdateInfo, error) {
			// El contexto acota; el cliente ya aplica su propio timeout.
			_ = ctx
			res, err := uc.Check(ctx)
			if err != nil || !res.Update {
				return ui.UpdateInfo{}, err
			}
			return ui.UpdateInfo{Version: res.Latest, URL: res.URL}, nil
		}
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
	const minShare = 0.3
	steps := deadlock.BuildRoute(runs, minShare)
	// Procedencia de la MUESTRA: se repite en cada paso porque la
	// antigüedad pertenece a la muestra, no a cada objeto.
	origin, _ := r.api.RunOriginFor(id)
	out := make([]ui.RouteStep, 0, len(steps))
	for _, st := range steps {
		out = append(out, ui.RouteStep{
			Item: st.Item, N: st.N, Share: st.Share,
			SampleSize: len(runs), MinShare: minShare,
			MedT: st.MedT, MedNet: st.MedNet,
			PosLo: st.PosLo, PosHi: st.PosHi,
			UpgradeOf:     st.UpgradeOf,
			ChainRate:     st.ChainRate,
			UpgradedRate:  st.UpgradedRate,
			DataState:     origin.State,
			DataFetchedAt: origin.FetchedAt,
		})
	}
	return out, nil
}

// LoadHybrid implementa ui.BuildLoader: calcula la build híbrida del
// héroe con el motor de dominio y la entrega ya resuelta a la UI.
// Devuelve (nil, nil) cuando la muestra no alcanza: la UI cae a la ruta
// observada sin mostrar un error.
func (r deadlockRuns) LoadHybrid(champKey string) (*ui.HybridBuildView, error) {
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
		return nil, nil // muestra corta: sin build híbrida
	}
	cat, err := r.api.ProgressionCatalog()
	if err != nil {
		return nil, err
	}
	hb, err := deadlock.BuildHybrid(runs, cat, deadlock.BuildOptions{})
	if err != nil {
		return nil, err
	}
	// La build híbrida sale de la MISMA muestra que la ruta: comparte su
	// procedencia para que la UI advierta igual de datos caducados.
	origin, _ := r.api.RunOriginFor(id)
	out := &ui.HybridBuildView{
		Confidence:    hb.Confidence,
		FinalItems:    hb.Plan.FinalItems(),
		Spent:         hb.Plan.Spent,
		DataState:     origin.State,
		DataFetchedAt: origin.FetchedAt,
	}
	conv := func(entries []deadlock.BuildEntry) []ui.HybridStep {
		steps := make([]ui.HybridStep, 0, len(entries))
		for _, e := range entries {
			origins := make([]string, 0, len(e.Origin))
			for _, o := range e.Origin {
				origins = append(origins, string(o))
			}
			steps = append(steps, ui.HybridStep{
				Item:       e.Item,
				Name:       e.Name,
				Phase:      int(e.Phase),
				Minute:     e.Minute,
				Confidence: e.Confidence,
				Share:      e.Share,
				ConsumedBy: e.ConsumedBy,
				Chain:      e.Chain,
				Origin:     origins,
				Reason:     e.Reason,
			})
		}
		return steps
	}
	out.Standard = conv(hb.Standard)
	out.Extensions = conv(hb.Extensions)
	out.Situationals = conv(hb.Situationals)
	return out, nil
}
