package ui

import (
	"testing"

	"github.com/Eion21/mobatui/internal/ddragon"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

// ─── defecto latente de runas (Fase 3) ─────────────────────────────
//
// runeArtCmdFor NO consulta artDidFail ni deduplica por recurso, así que
// el defecto 2 (404 redescargado al cambiar de ancho) sigue vivo en
// runas. Hoy no se nota porque runeArtCols es FIJO; la Fase 3 hará las
// runas adaptables al ancho y entonces se manifestará.
//
// Este archivo NO falla: documenta el estado actual con evidencia
// numérica. El diff propuesto está en el informe, sin aplicar.

// runeProvider simula un proveedor de runas que responde 404.
type runeProvider struct {
	staticdata.Provider
	iconos int
	err    error
}

func (r *runeProvider) RuneIcon(int) ([]byte, error) {
	r.iconos++
	return nil, r.err
}

func sesionRunas(p staticdata.Provider) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    p,
	}
}

// TestRuna404SeRedescargaPorAncho demuestra el defecto latente: con un
// 404 en una runa, cambiar de ancho provoca otra solicitud, porque
// runeArtCmdFor no consulta artDidFail.
func TestRuna404SeRedescargaPorAncho(t *testing.T) {
	p := &runeProvider{err: &ddragon.HTTPError{URL: "u", StatusCode: 404}}
	s := sesionRunas(p)

	// runeArtCols es fijo hoy; se simula lo que hará la Fase 3 pidiendo
	// el mismo recurso con dos anchos distintos.
	cmd1 := runeArtCmdFor(s, []int{8000})
	if cmd1 == nil {
		t.Fatalf("debía pedir la runa (iconos=%d)", p.iconos)
	}
	_ = cmd1()
	tras1 := p.iconos

	// Segundo intento (equivalente a cambiar de ancho en la Fase 3).
	cmd2 := runeArtCmdFor(s, []int{8000})
	if cmd2 != nil {
		_ = cmd2()
	}
	tras2 := p.iconos

	t.Logf("solicitudes al proveedor: tras el 1er intento=%d, tras el 2º=%d", tras1, tras2)
	if tras2 > tras1 {
		t.Logf("DEFECTO LATENTE CONFIRMADO: el 404 de la runa se vuelve a pedir "+
			"(%d -> %d). runeArtCmdFor no consulta artDidFail. No se manifiesta "+
			"hoy porque runeArtCols es fijo.", tras1, tras2)
		return
	}
	t.Log("sin re-descarga: el guard ya cubre las runas")
}
