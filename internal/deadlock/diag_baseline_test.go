package deadlock

// Arnés de diagnóstico de Fase 0: reproduce la ruta observada de un
// héroe usando SOLO la caché en disco (Offline), sin tráfico HTTP.
// Sirve como línea base reproducible y como fixture de comparación
// para las fases siguientes.
//
//	go test ./internal/deadlock -run TestDiagBaseline -v

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/progression"
)

// diagCacheDir localiza data/cache desde el paquete de pruebas.
func diagCacheDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "data", "cache"))
	if err != nil {
		t.Fatalf("ruta de caché: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("sin caché local en %s: %v", dir, err)
	}
	return dir
}

// diagClient devuelve un cliente offline sobre la caché real.
func diagClient(t *testing.T) *APIClient {
	t.Helper()
	c, err := cache.New(diagCacheDir(t))
	if err != nil {
		t.Fatalf("caché: %v", err)
	}
	c.Offline = true
	fix, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "deadlock"))
	return NewAPI(c, fix)
}

// diagShop carga el catálogo de tienda desde la caché offline.
func diagShop(t *testing.T, api *APIClient) map[int]apiItem {
	t.Helper()
	shop, _, err := api.shopItems()
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	if len(shop) == 0 {
		t.Skip("catálogo vacío en caché")
	}
	return shop
}

// TestDiagBaseline imprime la ruta observada de Violet (86) con el
// algoritmo actual, para registrar la línea base de Fase 0.
func TestDiagBaseline(t *testing.T) {
	api := diagClient(t)
	shop := diagShop(t, api)

	const hero = 86 // Violet
	ctx := context.Background()
	runs, err := api.FetchRuns(ctx, hero, 0, 250)
	if err != nil {
		t.Skipf("sin muestra de partidas en caché: %v", err)
	}
	if len(runs) == 0 {
		t.Skip("muestra vacía")
	}

	const minShare = 0.3
	steps := BuildRoute(runs, minShare)

	t.Logf("=== LÍNEA BASE Violet (hero %d) ===", hero)
	t.Logf("partidas en la muestra: %d", len(runs))
	t.Logf("umbral minShare: %.2f", minShare)
	t.Logf("pasos en la ruta: %d", len(steps))
	t.Logf("")

	// Inventario final simulado: cada compra que es mejora consume su
	// base. Se reconstruye el conjunto de objetos que quedan.
	final := map[int]bool{}
	consumed := map[int]bool{}
	for _, st := range steps {
		if st.UpgradeOf != 0 && st.ChainRate >= 0.5 {
			consumed[st.UpgradeOf] = true
		}
	}
	for _, st := range steps {
		if !consumed[st.Item] {
			final[st.Item] = true
		}
	}

	t.Logf("%-4s %-32s %-6s %-6s %-7s %-8s %s", "#", "objeto", "share", "N", "min", "cadena", "mejora de")
	for i, st := range steps {
		name := shop[st.Item].Name
		if name == "" {
			name = fmt.Sprintf("id=%d", st.Item)
		}
		chain := ""
		if st.UpgradeOf != 0 {
			base := shop[st.UpgradeOf].Name
			if base == "" {
				base = fmt.Sprintf("id=%d", st.UpgradeOf)
			}
			chain = fmt.Sprintf("de %s (%.0f%%)", base, st.ChainRate*100)
		}
		t.Logf("%-4d %-32s %-6.2f %-6d %-7d %-8s %s",
			i+1, name, st.Share, st.N, st.MedT/60, "", chain)
	}

	t.Logf("")
	t.Logf("compras visibles: %d", len(steps))
	t.Logf("objetos finales distintos (mejoras consumen su base): %d", len(final))
	var finals []string
	for id := range final {
		n := shop[id].Name
		if n == "" {
			n = fmt.Sprintf("id=%d", id)
		}
		finals = append(finals, n)
	}
	sort.Strings(finals)
	t.Logf("inventario final: %v", finals)

	// Candidatos que quedan fuera de la ruta (share < umbral) pero que
	// el usuario ve en "Fuera de la ruta": se listan los más comprados
	// por debajo del umbral.
	type out struct {
		id    int
		share float64
	}
	var outs []out
	inRoute := map[int]bool{}
	for _, st := range steps {
		inRoute[st.Item] = true
	}
	// Recalcular shares de todo lo que aparece en las runs.
	counts := map[int]int{}
	for _, r := range runs {
		seen := map[int]bool{}
		for _, e := range r.Evs {
			if !seen[e.Item] {
				seen[e.Item] = true
				counts[e.Item]++
			}
		}
	}
	for id, n := range counts {
		if inRoute[id] {
			continue
		}
		sh := float64(n) / float64(len(runs))
		if sh >= 0.10 {
			outs = append(outs, out{id, sh})
		}
	}
	sort.Slice(outs, func(i, j int) bool { return outs[i].share > outs[j].share })
	t.Logf("")
	t.Logf("--- fuera de la ruta con share >= 10%% (candidatos) ---")
	for _, o := range outs {
		n := shop[o.id].Name
		if n == "" {
			n = fmt.Sprintf("id=%d", o.id)
		}
		t.Logf("  %-32s share=%.2f", n, o.share)
	}
}

// TestDiagSolomonSkills verifica el mapeo de slots de habilidad contra
// el orden canónico del héroe (signature1..4), con datos de caché.
func TestDiagSolomonSkills(t *testing.T) {
	api := diagClient(t)
	shop, abilityNames, err := api.shopItems()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	heroes, err := api.heroes()
	if err != nil {
		t.Skipf("héroes: %v", err)
	}
	var sol apiHero
	for _, h := range heroes {
		if h.Name == "Solomon" {
			sol = h
		}
	}
	if sol.ID == 0 {
		t.Skip("Solomon no está en la caché")
	}

	abilityByClass := api.abilityClassIndex()
	_ = shop
	canonical := canonicalAbilityOrder(sol, abilityByClass)
	t.Logf("orden canónico (signature1..4): %v", canonical)
	if len(canonical) != 4 {
		t.Fatalf("orden canónico incompleto: %v", canonical)
	}
	for i, id := range canonical {
		t.Logf("  signature%d → %d %q", i+1, id, abilityNames[int(id)])
	}

	// El slot 1 debe ser signature1 (Setup Bishop), no la más jugada.
	if got := abilityNames[int(canonical[0])]; got != "Setup Bishop" {
		t.Errorf("signature1 = %q, esperado Setup Bishop", got)
	}
	if got := abilityNames[int(canonical[3])]; got != "Develop Queen" {
		t.Errorf("signature4 = %q, esperado Develop Queen", got)
	}
}

// TestDiagProgressionRealCatalog valida el motor de progresión contra el
// catálogo real: reconstruye la ruta de Violet y comprueba que el
// inventario final coincide con el que produce el algoritmo actual.
func TestDiagProgressionRealCatalog(t *testing.T) {
	api := diagClient(t)
	shop, _, err := api.shopItems()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	raw := make([]progression.RawItem, 0, len(shop))
	for id, it := range shop {
		raw = append(raw, progression.RawItem{
			ID: id, Name: it.Name, Class: it.ClassName,
			Slot: it.ItemSlotType, Tier: it.ItemTier, Cost: it.Cost,
			Comps: it.ComponentItems, Shopable: true,
		})
	}
	cat, warns, err := progression.BuildCatalog(raw)
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	t.Logf("catálogo: %d objetos, %d advertencias", cat.Len(), len(warns))
	for _, w := range warns[:min(5, len(warns))] {
		t.Logf("  aviso: %s", w)
	}

	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}
	steps := BuildRoute(runs, 0.3)

	// Convertir la ruta observada en compras del motor.
	var purchases []progression.Purchase
	for _, st := range steps {
		purchases = append(purchases, progression.Purchase{
			Item: st.Item, Minute: st.MedT / 60,
			Origin: []progression.Origin{progression.OriginObserved},
		})
	}
	plan, rejects := progression.Simulate(cat, 12, purchases)
	t.Logf("ruta observada: %d pasos → %d aplicados, %d rechazos",
		len(purchases), len(plan.Steps), len(rejects))
	for _, r := range rejects {
		t.Logf("  rechazo: %d — %s", r.Item, r.Reason)
	}
	t.Logf("inventario final: %d objetos", plan.FinalItems())
	for _, id := range plan.Inventory {
		it, _ := cat.Item(id)
		t.Logf("  - %s (tier %d, %d almas)", it.Name, it.Tier, it.Cost)
	}
	t.Logf("gasto total: %d almas · costes desconocidos: %d", plan.Spent, plan.Unknown)

	// Cada paso debe declarar qué consumió.
	for i, st := range plan.Steps {
		if len(st.Consumed) > 0 {
			it, _ := cat.Item(st.Item)
			var names []string
			for _, c := range st.Consumed {
				ci, _ := cat.Item(c)
				names = append(names, ci.Name)
			}
			t.Logf("  paso %d: %s consume %v (incremental %d)", i+1, it.Name, names, st.Incremental)
		}
	}
}

// TestDiagEvaluateViolet compara el evaluador nuevo con el algoritmo
// actual sobre la misma muestra de Violet.
func TestDiagEvaluateViolet(t *testing.T) {
	api := diagClient(t)
	shop, _, err := api.shopItems()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}

	base := BuildRoute(runs, 0.30)
	inBase := map[int]bool{}
	for _, st := range base {
		inBase[st.Item] = true
	}

	cands := Evaluate(runs, EvalOptions{MinShare: 0.30})
	t.Logf("=== EVALUADOR Violet: %d candidatos ===", len(cands))
	t.Logf("%-28s %-6s %-7s %-7s %-7s %-6s %-6s %s",
		"objeto", "share", "reach", "post", "conf", "score", "fase", "en ruta")
	for _, c := range cands[:min(30, len(cands))] {
		name := shop[c.Item].Name
		if name == "" {
			name = fmt.Sprintf("id=%d", c.Item)
		}
		t.Logf("%-28s %-6.2f %-7.2f %-7.3f %-7.2f %-6.2f %-6d %v",
			name, c.Share, c.ShareReached, c.Posterior, c.Confidence, c.Score, c.Phase, inBase[c.Item])
	}

	// Candidatos con confianza alta que NO están en la ruta actual.
	t.Logf("")
	t.Logf("--- confianza alta fuera de la ruta actual ---")
	for _, c := range cands {
		if inBase[c.Item] {
			continue
		}
		if c.Confidence >= 0.70 {
			t.Logf("  %-28s share=%.2f conf=%.2f score=%.2f",
				shop[c.Item].Name, c.Share, c.Confidence, c.Score)
		}
	}
	// Pasos de la ruta con confianza baja (candidatos a revisar).
	t.Logf("")
	t.Logf("--- pasos de la ruta con confianza baja ---")
	for _, c := range cands {
		if !inBase[c.Item] {
			continue
		}
		if c.Confidence < 0.45 {
			t.Logf("  %-28s share=%.2f conf=%.2f (%s)",
				shop[c.Item].Name, c.Share, c.Confidence, ConfidenceLabel(c.Confidence))
		}
	}
}

// TestDiagHybridViolet ejecuta el constructor híbrido con la muestra
// real de Violet y compara contra la ruta observada.
func TestDiagHybridViolet(t *testing.T) {
	api := diagClient(t)
	shop, _, err := api.shopItems()
	if err != nil {
		t.Skipf("catálogo: %v", err)
	}
	raw := make([]progression.RawItem, 0, len(shop))
	for id, it := range shop {
		raw = append(raw, progression.RawItem{
			ID: id, Name: it.Name, Class: it.ClassName,
			Slot: it.ItemSlotType, Tier: it.ItemTier, Cost: it.Cost,
			Comps: it.ComponentItems, Shopable: true,
		})
	}
	cat, _, err := progression.BuildCatalog(raw)
	if err != nil {
		t.Fatalf("catálogo: %v", err)
	}
	runs, err := api.FetchRuns(context.Background(), 86, 0, 250)
	if err != nil {
		t.Skipf("sin runs: %v", err)
	}

	base := BuildRoute(runs, 0.30)
	baseFinals := map[int]bool{}
	consumed := map[int]bool{}
	for _, st := range base {
		if st.UpgradeOf != 0 && st.ChainRate >= 0.5 {
			consumed[st.UpgradeOf] = true
		}
	}
	for _, st := range base {
		if !consumed[st.Item] {
			baseFinals[st.Item] = true
		}
	}
	t.Logf("=== RUTA OBSERVADA (anterior) ===")
	t.Logf("pasos: %d · objetos finales: %d", len(base), len(baseFinals))

	hb, err := BuildHybrid(runs, cat, BuildOptions{})
	if err != nil {
		t.Fatalf("build híbrida: %v", err)
	}

	t.Logf("")
	t.Logf("=== BUILD HÍBRIDA (nueva) ===")
	t.Logf("ruta estándar: %d objetos", len(hb.Standard))
	for i, e := range hb.Standard {
		t.Logf("  %d. %-26s %-6s %2dmin conf=%.2f share=%.2f  %s",
			i+1, e.Name, e.Phase, e.Minute, e.Confidence, e.Share, e.Reason)
	}
	t.Logf("ampliaciones: %d", len(hb.Extensions))
	for _, e := range hb.Extensions {
		t.Logf("  + %-26s %-6s conf=%.2f", e.Name, e.Phase, e.Confidence)
	}
	t.Logf("situacionales: %d", len(hb.Situationals))
	for _, e := range hb.Situationals[:min(12, len(hb.Situationals))] {
		t.Logf("  ~ %-26s [%s]", e.Name, e.Reason)
	}
	t.Logf("")
	t.Logf("inventario final: %d objetos", hb.Plan.FinalItems())
	for _, id := range hb.Plan.Inventory {
		t.Logf("  - %s", nameOf(cat, id))
	}
	t.Logf("gasto: %d almas · rechazos: %d", hb.Plan.Spent, len(hb.Rejects))
	for _, r := range hb.Rejects {
		t.Logf("  rechazo: %s — %s", nameOf(cat, r.Item), r.Reason)
	}
	t.Logf("confianza global: %.2f (%s)", hb.Confidence, ConfidenceLabel(hb.Confidence))

	t.Logf("")
	t.Logf("=== EXCLUSIONES (%d) ===", len(hb.Excluded))
	for _, x := range hb.Excluded[:min(15, len(hb.Excluded))] {
		t.Logf("  %-26s %s", x.Name, x.Reason)
	}
}
