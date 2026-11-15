package ui

// Render de la pantalla real de Violet con datos de caché, para
// inspección visual en texto. No es validación visual del terminal.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/game"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

func TestRenderVioletHybrid(t *testing.T) {
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
	d.width, d.height = 100, 44
	d.layout(0)
	a := App{screen: screenDetail, session: s, detail: d, width: 100, height: 44}
	a.detail.sync()

	var sb strings.Builder
	sb.WriteString(hybridText(a))
	for i := 0; i < 12; i++ {
		a = routeLoadingUpdate(a, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		sb.WriteString("\n" + hybridText(a))
	}
	fmt.Println("========== VIOLET 100x44 (recorrido completo) ==========")
	fmt.Println(sb.String())
}
