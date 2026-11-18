package ui

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

func loaderOffline(b testing.TB, g model.Game) Loader {
	c, err := cache.New(filepath.Join(b.TempDir(), "c"))
	if err != nil {
		b.Fatal(err)
	}
	c.Offline = true
	if g == model.GameLoL {
		ddc := ddragon.New(c, "../../testdata/ddragon", "es_ES").
			WithHTTPClient(&http.Client{Transport: failTransport{}})
		return Loader{UggDir: "../../testdata/ugg", DeadlockDir: "../../testdata/deadlock/builds",
			Providers: map[model.Game]staticdata.Provider{model.GameLoL: ddc}}
	}
	dl := deadlock.New("../../testdata/deadlock")
	return Loader{UggDir: "../../testdata/ugg", DeadlockDir: "../../testdata/deadlock/builds",
		Providers: map[model.Game]staticdata.Provider{model.GameDeadlock: dl}}
}

// BenchmarkArranqueConIconos mide el arranque COMPLETO (sesión +
// prerender de iconos de todos los campeones/héroes).
func BenchmarkArranqueConIconos(b *testing.B) {
	for _, g := range []model.Game{model.GameLoL, model.GameDeadlock} {
		b.Run(map[model.Game]string{model.GameLoL: "lol", model.GameDeadlock: "deadlock"}[g], func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				t0 := time.Now()
				s, err := LoadSession(g, loaderOffline(b, g))
				if err != nil {
					b.Fatalf("%v", err)
				}
				d := time.Since(t0)
				if i == 0 {
					b.ReportMetric(float64(d.Microseconds()), "total_us")
					b.ReportMetric(float64(len(s.ItemArt)), "iconos")
					b.ReportMetric(float64(len(s.ChampionArt)), "retratos")
				}
			}
		})
	}
}
