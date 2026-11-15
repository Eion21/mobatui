package ui

// Diagnóstico de Fase 0: mide qué tarjetas sobreviven el pipeline real
// (clean.Build → dlBuildCards → filterUpgrades) y con qué numeración.
// Usa la caché en disco, sin red.
//
//	go test ./internal/ui -run TestDiagVioletCards -v

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/clean"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/game"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// diagSession carga la sesión de Deadlock desde caché offline.
func diagSession(t *testing.T) *Session {
	t.Helper()
	dir, _ := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("sin caché: %v", err)
	}
	c, err := cache.New(dir)
	if err != nil {
		t.Fatalf("caché: %v", err)
	}
	c.Offline = true
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	api := deadlock.NewAPI(c, fix)

	cbs, err := api.LoadBuilds()
	if err != nil {
		t.Skipf("carga offline falló: %v", err)
	}
	opts := clean.Options{MaxItems: 9999}
	builds := clean.BuildAll(cbs, opts)
	names, err := api.Items()
	if err != nil {
		t.Skipf("sin nombres de ítems: %v", err)
	}
	heroes, err := api.Heroes()
	if err != nil {
		t.Skipf("sin héroes: %v", err)
	}
	meta, _ := api.ItemDetails()
	if meta == nil {
		meta = map[int]staticdata.ItemInfo{}
	}
	s := &Session{
		Game:      model.GameDeadlock,
		Builds:    builds,
		Names:     Names{Items: names},
		ItemArt:   map[int]string{},
		ItemMeta:  meta,
		Routes:    map[string][]RouteStep{},
		Champions: groupByChampion(builds, heroes, game.For(model.GameDeadlock).Roles),
	}
	return s
}

// TestDiagVioletCards imprime la lista de tarjetas que la TUI mostraría
// para Violet, con su número de ruta y su fase.
func TestDiagVioletCards(t *testing.T) {
	s := diagSession(t)
	var champ *ChampEntry
	for _, e := range s.Champions {
		if e.Key == "Violet" {
			champ = e
			break
		}
	}
	if champ == nil {
		t.Skip("Violet no está en la sesión offline")
	}
	var roles []model.Role
	for r := range champ.ByRole {
		roles = append(roles, r)
	}
	t.Logf("roles disponibles para Violet: %v", roles)
	if len(roles) == 0 {
		t.Skip("Violet sin builds")
	}
	bb := champ.ByRole[roles[0]]
	t.Logf("build Violet: core=%d boots=%d wards=%d WR=%.3f total=%d",
		len(bb.CoreItems), len(bb.Boots), len(bb.Wards), bb.WinRate, bb.Total)

	// Ruta real desde la muestra cacheada.
	dir, _ := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	c, _ := cache.New(dir)
	c.Offline = true
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	api := deadlock.NewAPI(c, fix)
	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}
	steps := deadlock.BuildRoute(runs, 0.3)
	var route []RouteStep
	for _, st := range steps {
		route = append(route, RouteStep{
			Item: st.Item, N: st.N, Share: st.Share, SampleSize: len(runs), MinShare: 0.3,
			MedT: st.MedT, MedNet: st.MedNet, PosLo: st.PosLo, PosHi: st.PosHi,
			UpgradeOf: st.UpgradeOf, ChainRate: st.ChainRate, UpgradedRate: st.UpgradedRate,
		})
	}
	s.Routes["Violet"] = route

	d := newDetailView(champ, s)
	d.width, d.height = 120, 44
	cards := d.cardList()

	t.Logf("=== TARJETAS VISIBLES Violet ===")
	t.Logf("ruta: %d pasos · tarjetas: %d", len(route), len(cards))
	t.Logf("")
	t.Logf("%-6s %-28s %-6s %-8s %-6s %s", "orden", "objeto", "fase", "tipo", "min", "base")
	seen := map[int]bool{}
	var orders []int
	for _, c := range cards {
		name := s.ItemName(c.item.ID)
		kind := "compra"
		if c.kind == kindUpgrade {
			kind = "mejora"
		}
		base := ""
		if c.baseID != 0 {
			base = s.ItemName(c.baseID)
		}
		t.Logf("%-6d %-28s %-6d %-8s %-6d %s",
			c.step, name, dlPhaseOf(c.dlMinuteOf()), kind, c.dlMinuteOf()/60, base)
		if c.step > 0 {
			orders = append(orders, c.step)
		}
		seen[c.item.ID] = true
	}

	t.Logf("")
	t.Logf("--- numeración de ruta visible ---")
	t.Logf("órdenes presentes: %v", orders)
	for i := 1; i < len(orders); i++ {
		if orders[i] != orders[i-1]+1 {
			t.Logf("  HUECO: %d → %d", orders[i-1], orders[i])
		}
	}

	t.Logf("")
	t.Logf("--- pasos de ruta SIN tarjeta ---")
	for i, st := range route {
		if !seen[st.Item] {
			t.Logf("  #%d %-28s share=%.2f min=%d", i+1, s.ItemName(st.Item), st.Share, st.MedT/60)
		}
	}

	// Inventario final de las tarjetas visibles.
	consumed := map[int]bool{}
	for _, c := range cards {
		if c.kind == kindUpgrade && c.baseID != 0 {
			consumed[c.baseID] = true
		}
	}
	var finals []string
	for _, c := range cards {
		if !consumed[c.item.ID] {
			finals = append(finals, s.ItemName(c.item.ID))
		}
	}
	t.Logf("")
	t.Logf("tarjetas visibles: %d", len(cards))
	t.Logf("objetos finales: %d → %v", len(finals), finals)
}

// TestDiagPhaseCut muestra el ranking por fase ANTES del corte de
// perPhase=8 en dlBuildCards, para identificar qué compras se pierden
// y en qué puesto quedaban.
func TestDiagPhaseCut(t *testing.T) {
	s := diagSession(t)
	var champ *ChampEntry
	for _, e := range s.Champions {
		if e.Key == "Violet" {
			champ = e
		}
	}
	if champ == nil {
		t.Skip("Violet no está")
	}
	dir, _ := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	c, _ := cache.New(dir)
	c.Offline = true
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	api := deadlock.NewAPI(c, fix)
	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}
	steps := deadlock.BuildRoute(runs, 0.3)
	var route []RouteStep
	for _, st := range steps {
		route = append(route, RouteStep{Item: st.Item, N: st.N, Share: st.Share,
			SampleSize: len(runs), MinShare: 0.3, MedT: st.MedT, MedNet: st.MedNet,
			PosLo: st.PosLo, PosHi: st.PosHi, UpgradeOf: st.UpgradeOf,
			ChainRate: st.ChainRate, UpgradedRate: st.UpgradedRate})
	}
	s.Routes["Violet"] = route

	d := newDetailView(champ, s)
	d.width, d.height = 120, 44
	b := d.CurrentBuild()

	// Reconstruir la lista completa que entra a dlBuildCards.
	var all []cardItem
	for _, it := range b.CoreItems {
		all = append(all, cardItem{item: it})
	}
	all = d.dlDecorate(all)

	top := 0
	for _, c := range all {
		if c.item.Games > top {
			top = c.item.Games
		}
	}
	type sc struct {
		name  string
		score float64
		rel   float64
		order int
	}
	byPhase := map[int][]sc{}
	for _, c := range all {
		rel := float64(c.item.Games) / float64(top)
		if rel < 0.08 {
			continue
		}
		sig := c.item.Wilson - b.WinRate
		score := 2.0*math.Sqrt(rel) + 6.0*sig
		p := dlPhaseOf(c.dlMinuteOf())
		byPhase[p] = append(byPhase[p], sc{s.ItemName(c.item.ID), score, rel, c.order})
	}
	for p := 0; p < 3; p++ {
		list := byPhase[p]
		sort.SliceStable(list, func(i, j int) bool { return list[i].score > list[j].score })
		t.Logf("--- fase %d (%s): %d candidatos, corte en 8 ---", p, dlPhaseNames[p], len(list))
		for i, x := range list {
			cut := ""
			if i >= 8 {
				cut = "  ← CORTADO"
			}
			mark := ""
			if x.order > 0 {
				mark = fmt.Sprintf("ruta#%d", x.order)
			}
			t.Logf("  %2d. %-28s score=%.3f rel=%.2f %-8s%s", i+1, x.name, x.score, x.rel, mark, cut)
		}
	}
}
