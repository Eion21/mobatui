package ui

import (
	"strings"
	"testing"
)

// ─── caracterización de la pestaña de Runas por ancho ───────────────
//
// Registra QUÉ elementos aparecen hoy en cada ancho, sin comparar
// cadenas exactas: solo presencia. Es la red de seguridad de la regla
// "a ningún ancho puede perderse algo que hoy se muestra a ese ancho".
//
// Se ejecuta sobre internal/ui/detail.go:2212 (runesView).

// anchosRunas son los anchos de la caracterización.
var anchosRunas = []int{30, 40, 59, 60, 99, 100, 117, 118, 160, 200}

// elementosRunas indica qué se ve en la pestaña de Runas de una vista.
type elementosRunas struct {
	keystone   bool
	primario   bool
	secundario bool
	fragmentos bool
	alt        bool
	winRate    bool
	partidas   bool
}

// detectaElementos inspecciona el texto plano de la pestaña.
func detectaElementos(out string, d *detailView) elementosRunas {
	cb := d.CurrentBuild()
	if cb == nil || len(cb.RunePages) == 0 {
		return elementosRunas{}
	}
	p := cb.RunePages[0]
	nombres := func(ids []int) []string {
		var v []string
		for _, id := range ids {
			if n := d.session.RuneName(id); n != "" {
				v = append(v, n)
			}
		}
		return v
	}
	contiene := func(v []string) bool {
		for _, s := range v {
			if s != "" && strings.Contains(out, s) {
				return true
			}
		}
		return false
	}
	// Todas las runas contadas deben verse: una sola basta como prueba
	// de presencia del grupo.
	return elementosRunas{
		keystone:   contiene([]string{d.session.RuneName(p.KeystoneID)}),
		primario:   contiene(nombres(p.PrimaryIDs)),
		secundario: contiene(nombres(p.SecondaryIDs)),
		fragmentos: strings.Contains(out, "fragmentos:") || contiene(shardNames(d, p.Shards)),
		alt:        len(cb.RunePages) > 1 && strings.Contains(out, "alt: "),
		winRate:    strings.Contains(out, "WR "),
		partidas:   strings.Contains(out, "partidas"),
	}
}

func shardNames(d *detailView, ids [3]int) []string {
	var v []string
	for _, id := range shardsConNombre(d, ids) {
		v = append(v, id)
	}
	return v
}

func shardsConNombre(d *detailView, ids [3]int) []string {
	var out []string
	for _, id := range ids {
		if n := d.session.ShardName(id); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// vistaRunasPlana devuelve el texto sin ANSI de lo que REALMENTE se ve:
// d.View(), que pasa por el viewport y es donde ocurre el recorte al
// ancho. runesView() por sí sola no trunca nada.
func vistaRunasPlana(d *detailView) string {
	return stripANSIStr(d.View())
}

// TestCaracterizacionRunasPorAncho documenta y fija la presencia actual
// de cada elemento en cada ancho. La regla del rediseño es que NADA de
// lo que aquí aparece puede desaparecer en un ancho dado.
func TestCaracterizacionRunasPorAncho(t *testing.T) {
	// Presencia registrada sobre 3a633af. Se actualiza SOLO si el
	// rediseño gana elementos, nunca si pierde.
	esperado := map[int]elementosRunas{
		// A 30 y 40 el borde del viewport corta el segundo panel: el
		// árbol secundario y los fragmentos NO se ven hoy (medido en
		// 3a633af). Desde 59 se ven todos.
		30:  {keystone: true, primario: true, alt: true, winRate: true, partidas: true},
		40:  {keystone: true, primario: true, alt: true, winRate: true, partidas: true},
		59:  {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		60:  {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		99:  {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		100: {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		117: {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		118: {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		160: {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
		200: {keystone: true, primario: true, secundario: true, fragmentos: true, alt: true, winRate: true, partidas: true},
	}
	for _, w := range anchosRunas {
		d := runeDetailView(t, w, 50)
		cb := d.CurrentBuild()
		if cb == nil || len(cb.RunePages) == 0 {
			t.Skip("sin runas en los fixtures")
		}
		got := detectaElementos(vistaRunasPlana(&d), &d)
		exp := esperado[w]
		t.Logf("ancho=%-4d keystone=%v primario=%v secundario=%v fragmentos=%v alt=%v WR=%v partidas=%v",
			w, got.keystone, got.primario, got.secundario, got.fragmentos, got.alt, got.winRate, got.partidas)
		if got.keystone != exp.keystone || got.primario != exp.primario ||
			got.secundario != exp.secundario || got.fragmentos != exp.fragmentos ||
			got.alt != exp.alt ||
			got.winRate != exp.winRate || got.partidas != exp.partidas {
			t.Errorf("ancho %d: presencia inesperada %+v (base %+v)", w, got, exp)
		}
	}
}
