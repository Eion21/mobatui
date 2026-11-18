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

// BenchmarkLoadSessionLoL mide el arranque de la sesión de LoL en
// offline con Data Dragon local: catálogo, nombres, runas e ítems.
func BenchmarkLoadSessionLoL(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c, err := cache.New(filepath.Join(b.TempDir(), "c"))
		if err != nil {
			b.Fatal(err)
		}
		c.Offline = true
		ddc := ddragon.New(c, "../../testdata/ddragon", "es_ES").WithHTTPClient(&http.Client{Transport: failTransport{}})
		l := Loader{
			UggDir:      "../../testdata/ugg",
			DeadlockDir: "../../testdata/deadlock/builds",
			Providers:   map[model.Game]staticdata.Provider{model.GameLoL: ddc},
		}
		t0 := time.Now()
		if _, err := LoadSession(model.GameLoL, l); err != nil {
			b.Fatalf("LoadSession: %v", err)
		}
		if i == 0 {
			b.ReportMetric(float64(time.Since(t0).Microseconds()), "session_us")
		}
	}
}

// BenchmarkLoadSessionDeadlock mide lo mismo para Deadlock.
func BenchmarkLoadSessionDeadlock(b *testing.B) {
	for i := 0; i < b.N; i++ {
		c, err := cache.New(filepath.Join(b.TempDir(), "c"))
		if err != nil {
			b.Fatal(err)
		}
		c.Offline = true
		dl := deadlock.New("../../testdata/deadlock")
		l := Loader{
			UggDir:      "../../testdata/ugg",
			DeadlockDir: "../../testdata/deadlock/builds",
			Providers:   map[model.Game]staticdata.Provider{model.GameDeadlock: dl},
		}
		t0 := time.Now()
		if _, err := LoadSession(model.GameDeadlock, l); err != nil {
			b.Fatalf("LoadSession: %v", err)
		}
		if i == 0 {
			b.ReportMetric(float64(time.Since(t0).Microseconds()), "session_us")
		}
	}
}
