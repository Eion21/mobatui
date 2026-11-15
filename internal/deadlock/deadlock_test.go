package deadlock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Eion21/mobatui/internal/model"
)

const abrams = `{
  "hero_id": 1, "hero_key": "Abrams", "patch": "2025.10",
  "overview": [9840, 4930],
  "items": [[101, 8100, 4230], [306, 0, 0]],
  "skills": [
    ["121314121242233433", 4200, 2260],
    ["QQQQQQQQQQQQQQQQQQ", 500, 250]
  ]
}`

func TestParse(t *testing.T) {
	cb, err := Parse([]byte(abrams))
	if err != nil {
		t.Fatal(err)
	}
	if cb.Game != model.GameDeadlock || cb.Role != model.RoleAll {
		t.Errorf("metadatos incorrectos: juego=%s rol=%s", cb.Game, cb.Role)
	}
	if cb.Total.Games != 9840 {
		t.Errorf("overview = %d", cb.Total.Games)
	}
	// Ítem con 0 partidas fuera; secuencia con letras de LoL fuera.
	if len(cb.Items) != 1 || cb.Items[0].ID != 101 {
		t.Errorf("items = %+v", cb.Items)
	}
	if len(cb.Skills) != 1 {
		t.Fatalf("skills = %+v, se esperaba solo la válida", cb.Skills)
	}
	if cb.Skills[0].Sequence != "121314121242233433" {
		t.Errorf("secuencia = %q", cb.Skills[0].Sequence)
	}
}

func TestParseSinHeroKey(t *testing.T) {
	if _, err := Parse([]byte(`{"hero_id":1,"overview":[1,1]}`)); err == nil {
		t.Fatal("hero_key vacío debía fallar")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "1.json"), []byte(abrams), 0o644); err != nil {
		t.Fatal(err)
	}
	cbs, err := LoadDir(dir)
	if err != nil || len(cbs) != 1 {
		t.Fatalf("LoadDir = %v, %v", cbs, err)
	}
	if _, err := LoadDir(t.TempDir()); err == nil {
		t.Error("directorio vacío debía fallar")
	}
}

func TestLoadDirFixturesReales(t *testing.T) {
	cbs, err := LoadDir("../../testdata/deadlock/builds")
	if err != nil {
		t.Fatal(err)
	}
	if len(cbs) != 14 {
		t.Errorf("héroes cargados = %d, se esperaban 14", len(cbs))
	}
}

func TestProveedorEstatico(t *testing.T) {
	c := New("../../testdata/deadlock")
	v, err := c.Version()
	if err != nil || v != "2025.10" {
		t.Errorf("version = %q, %v", v, err)
	}
	heroes, err := c.Heroes()
	if err != nil {
		t.Fatal(err)
	}
	if heroes["GreyTalon"].Name != "Grey Talon" {
		t.Errorf("GreyTalon = %+v", heroes["GreyTalon"])
	}
	items, err := c.Items()
	if err != nil || items[301] != "Cargador titánico" {
		t.Errorf("item 301 = %q, %v", items[301], err)
	}
	runes, trees, err := c.Runes()
	if err != nil || len(runes) != 0 || len(trees) != 0 {
		t.Error("Deadlock no tiene runas: debía devolver mapas vacíos")
	}
	if _, err := c.ChampionIcon("Abrams"); err != nil {
		t.Errorf("icono de héroe: %v (¿genicons ejecutado?)", err)
	}
	if _, err := c.ItemIcon(101); err != nil {
		t.Errorf("icono de ítem: %v (¿genicons ejecutado?)", err)
	}
}
