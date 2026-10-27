package deadlock

import (
	"bufio"
	"strings"
	"testing"
)

// shopDePrueba: 101 es la base (Salud extra), 201 la mejora (Recarga
// activa) y 999 un ítem sin relación; 500 es un punto de habilidad.
func shopDePrueba() (map[int]bool, map[int][]int) {
	shop := map[int]bool{101: true, 201: true, 999: true}
	comps := map[int][]int{201: {101}}
	return shop, comps
}

func TestBuildEventsMejoraEnElLugar(t *testing.T) {
	shop, comps := shopDePrueba()
	items := []rawItem{
		{ItemID: 500, GameTimeS: 100}, // habilidad: fuera
		{ItemID: 101, GameTimeS: 177, SoldTimeS: 693, NetWorthAtBuy: 800},
		{ItemID: 201, GameTimeS: 693, NetWorthAtBuy: 1600},
	}
	evs, regrets := buildEvents(items, shop, comps)
	if len(regrets) != 0 {
		t.Fatalf("regrets = %v, quieren estar vacíos", regrets)
	}
	if len(evs) != 2 {
		t.Fatalf("len(evs) = %d, quieren 2: %+v", len(evs), evs)
	}
	if evs[0] != (Ev{Item: 101, T: 177, Net: 800}) {
		t.Errorf("evs[0] = %+v, quieren base directa a los 177 s", evs[0])
	}
	// La base vendida se conserva y la compra en el mismo segundo queda
	// marcada como mejora de esa base.
	if evs[1] != (Ev{Item: 201, T: 693, Net: 1600, From: 101}) {
		t.Errorf("evs[1] = %+v, quieren mejora con From=101", evs[1])
	}
}

func TestBuildEventsMargenDeUnSegundo(t *testing.T) {
	shop, comps := shopDePrueba()
	items := []rawItem{
		{ItemID: 101, GameTimeS: 177, SoldTimeS: 693},
		{ItemID: 201, GameTimeS: 694}, // venta y compra a 1 s
	}
	evs, regrets := buildEvents(items, shop, comps)
	if len(regrets) != 0 || len(evs) != 2 || evs[1].From != 101 {
		t.Fatalf("evs = %+v, regrets = %v; el margen de 1 s cuenta como mejora", evs, regrets)
	}
}

func TestBuildEventsVentaPorCambio(t *testing.T) {
	shop, comps := shopDePrueba()
	items := []rawItem{
		{ItemID: 101, GameTimeS: 177, SoldTimeS: 693},
		{ItemID: 999, GameTimeS: 693}, // no reemplaza a la base
	}
	evs, regrets := buildEvents(items, shop, comps)
	if len(evs) != 1 || evs[0].Item != 999 {
		t.Fatalf("evs = %+v, quieren solo el ítem de reemplazo", evs)
	}
	if len(regrets) != 1 || regrets[0] != 101 {
		t.Fatalf("regrets = %v, quieren [101] (venta por cambio)", regrets)
	}
}

func TestConsensusOrderPorPrecedencia(t *testing.T) {
	// 40 partidas compran 101 antes que 999, 10 al revés: el consenso
	// pone a 101 primero aunque la mediana mida lo mismo.
	var runs []Run
	for i := 0; i < 40; i++ {
		runs = append(runs, Run{Evs: []Ev{{Item: 101, T: 300}, {Item: 999, T: 600}}})
	}
	for i := 0; i < 10; i++ {
		runs = append(runs, Run{Evs: []Ev{{Item: 999, T: 300}, {Item: 101, T: 600}}})
	}
	top := []int{999, 101}
	medT := map[int]int{101: 300, 999: 300}
	got := consensusOrder(runs, top, medT)
	if got[0] != 101 || got[1] != 999 {
		t.Fatalf("orden = %v, quieren [101 999] por precedencia 40/50", got)
	}
}

func TestConsensusOrderDesempatePorMediana(t *testing.T) {
	// Sin pares comparables (2 partidas < 30), el orden lo da medT.
	runs := []Run{
		{Evs: []Ev{{Item: 201, T: 600}, {Item: 101, T: 300}}},
		{Evs: []Ev{{Item: 201, T: 600}, {Item: 101, T: 300}}},
	}
	got := consensusOrder(runs, []int{201, 101}, map[int]int{101: 300, 201: 600})
	if got[0] != 101 || got[1] != 201 {
		t.Fatalf("orden = %v, quieren [101 201] por mediana de compra", got)
	}
}

func TestBuildRoute(t *testing.T) {
	var runs []Run
	// 10 partidas: todas compran 101 a los 300 s; 8 mejoran a 201 a los
	// 600 s; 2 partidas compran 999 (share baja y no entra).
	for i := 0; i < 10; i++ {
		evs := []Ev{{Item: 101, T: 300, Net: 1200}}
		if i < 8 {
			evs = append(evs, Ev{Item: 201, T: 600, Net: 2400, From: 101})
		} else {
			evs = append(evs, Ev{Item: 999, T: 900, Net: 3200})
		}
		runs = append(runs, Run{Win: i < 7, Evs: evs})
	}
	steps := BuildRoute(runs, 0.5)
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, quieren 2 (999 queda fuera por share): %+v", len(steps), steps)
	}
	base, up := steps[0], steps[1]
	if base.Item != 101 || up.Item != 201 {
		t.Fatalf("ruta = [%d %d], quieren [101 201] (mediana de tiempo desempata)", base.Item, up.Item)
	}
	if base.Share != 1.0 || up.Share != 0.8 {
		t.Errorf("shares = %v/%v, quieren 1.0/0.8", base.Share, up.Share)
	}
	if base.MedT != 300 || up.MedT != 600 || base.MedNet != 1200 || up.MedNet != 2400 {
		t.Errorf("medianas = %+v %+v, quieren T 300/600 y Net 1200/2400", base, up)
	}
	if base.PosLo != 1 || base.PosHi != 1 || up.PosLo != 2 || up.PosHi != 2 {
		t.Errorf("posición = %+v %+v, quieren siempre 1 y 2", base, up)
	}
	// Tasa de mejora de la base: 8 de 10 compradores de 101 llegan a 201.
	if base.UpgradedRate < 0.79 || base.UpgradedRate > 0.81 {
		t.Errorf("UpgradedRate de la base = %v, se esperaba 0.8", base.UpgradedRate)
	}
	// La mejora no mejora a nadie: su propia UpgradedRate es 0.
	if up.UpgradedRate != 0 {
		t.Errorf("UpgradedRate de la mejora = %v, se esperaba 0", up.UpgradedRate)
	}
	if up.UpgradeOf != 101 || up.ChainRate != 1.0 {
		t.Errorf("cadena = %d con tasa %v, quieren 101 con 1.0", up.UpgradeOf, up.ChainRate)
	}
	// WR de la muestra: 101 gana 7/10; 201 aparece en 8 partidas y gana
	// 7 (la mejora solo la compra quien sobrevivió a los 600 s).
	if base.WR != 0.7 || up.WR != 0.875 {
		t.Errorf("WR = %v/%v, quieren 0.7/0.875", base.WR, up.WR)
	}
}

func TestDecodeRunsFiltraHeroeYGana(t *testing.T) {
	shop, comps := shopDePrueba()
	// Dos partidas: en la primera el héroe 1 gana (outcome), en la
	// segunda el héroe 1 pierde (respaldo por equipo) y el héroe 2 se
	// ignora.
	json := `[
	 {"winning_team":"Team1","players":[
	   {"hero_id":2,"team":"Team1","player_match_outcome":"Win","items":[]},
	   {"hero_id":1,"team":"Team1","player_match_outcome":"Win",
	    "items":[{"item_id":101,"game_time_s":300,"net_worth_at_buy":1200}]}]},
	 {"winning_team":"Team0","players":[
	   {"hero_id":1,"team":"Team1",
	    "items":[{"item_id":101,"game_time_s":300,"net_worth_at_buy":1200}]}]}
	]`
	runs, err := decodeRuns(bufio.NewReader(strings.NewReader(json)), 1, shop, comps)
	if err != nil {
		t.Fatalf("decodeRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, quieren 2 (solo héroe 1)", len(runs))
	}
	if !runs[0].Win || runs[1].Win {
		t.Fatalf("Win = %v/%v, quieren true/false", runs[0].Win, runs[1].Win)
	}
}
