package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/game"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// TestAuditUICostMatchesPlan: la suma de los importes que la UI muestra
// debe coincidir con el gasto real que reporta el plan.
func TestAuditUICostMatchesPlan(t *testing.T) {
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
	s := &Session{
		Game: model.GameDeadlock, Builds: builds,
		Names: Names{Items: names}, ItemArt: map[int]string{}, ItemMeta: meta,
		Routes:    map[string][]RouteStep{},
		Champions: groupByChampion(builds, heroes, game.For(model.GameDeadlock).Roles),
	}
	var champ *ChampEntry
	for _, e := range s.Champions {
		if e.Key == "Violet" {
			champ = e
		}
	}
	if champ == nil {
		t.Skip("Violet no está")
	}
	cat, err := api.ProgressionCatalog()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("runs: %v", err)
	}
	hb, err := deadlock.BuildHybrid(runs, cat, deadlock.BuildOptions{})
	if err != nil {
		t.Skipf("build: %v", err)
	}
	conv := func(es []deadlock.BuildEntry) []HybridStep {
		out := make([]HybridStep, 0, len(es))
		for _, e := range es {
			var o []string
			for _, x := range e.Origin {
				o = append(o, string(x))
			}
			out = append(out, HybridStep{Item: e.Item, Name: e.Name, Phase: int(e.Phase),
				Minute: e.Minute, Confidence: e.Confidence, Share: e.Share,
				ConsumedBy: e.ConsumedBy, Chain: e.Chain, Origin: o, Reason: e.Reason})
		}
		return out
	}
	view := &HybridBuildView{
		Standard: conv(hb.Standard), Extensions: conv(hb.Extensions),
		Situationals: conv(hb.Situationals),
		Confidence:   hb.Confidence, FinalItems: hb.Plan.FinalItems(), Spent: hb.Plan.Spent,
	}
	s.HybridLoader = hybridStub{build: view}
	s.Hybrids = map[string]*HybridBuildView{"Violet": view}

	d := newDetailView(champ, s)
	d.width, d.height = 120, 44
	d.layout(0)

	// Sumar el coste que la UI muestra en cada tarjeta de la ruta.
	cards := d.cardList()
	var uiSum int
	upgrades := 0
	for _, c := range cards {
		if c.hybridSection != SectionStandard {
			continue
		}
		info, ok := s.ItemMeta[c.item.ID]
		if !ok || info.Cost <= 0 {
			continue
		}
		if c.kind == kindUpgrade && c.baseID != 0 {
			upgrades++
			base, ok := s.ItemMeta[c.baseID]
			if ok && base.Cost > 0 && info.Cost >= base.Cost {
				uiSum += info.Cost - base.Cost
				continue
			}
		}
		uiSum += info.Cost
	}
	t.Logf("entradas en ruta: %d (mejoras: %d)", len(cards), upgrades)
	t.Logf("suma de importes visibles en la UI = %d", uiSum)
	t.Logf("gasto real del plan              = %d", hb.Plan.Spent)
	if uiSum != hb.Plan.Spent {
		t.Errorf("la UI muestra %d almas pero el plan gastó %d", uiSum, hb.Plan.Spent)
	}
}
