package ugg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Eion21/builds-tui/internal/model"
)

const ahriMid = `{
  "champion_id": 103, "champion_key": "Ahri", "patch": "15.20", "position": 3,
  "overview": [48213, 24871],
  "itemizations": {
    "individual": [[6653, 39210, 21000], [3157, 0, 0]],
    "boots": [[3020, 31022, 16133]]
  },
  "runes": [
    {"primary_tree": 8200, "secondary_tree": 8100, "keystone": 8229,
     "primary": [8226, 8210, 8237], "secondary": [8126, 8135],
     "shards": [5008, 5008, 5001], "stats": [21020, 11043]}
  ],
  "skills": [
    ["QWEQQRQWQWRWWEEREE", 12804, 6720],
    ["RWEQQRQWQWRWWEEREE", 500, 250]
  ]
}`

func TestParse(t *testing.T) {
	cb, err := Parse([]byte(ahriMid))
	if err != nil {
		t.Fatal(err)
	}
	if cb.ChampionKey != "Ahri" || cb.Role != model.RoleMiddle || cb.Patch != "15.20" {
		t.Errorf("metadatos incorrectos: %+v", cb)
	}
	if cb.Total.Games != 48213 || cb.Total.Wins != 24871 {
		t.Errorf("overview incorrecto: %+v", cb.Total)
	}
	// El ítem con 0 partidas se descarta; la secuencia con R en nivel 1
	// también (queda inválida matemáticamente).
	if len(cb.Items) != 1 || cb.Items[0].ID != 6653 {
		t.Errorf("items = %+v", cb.Items)
	}
	if len(cb.Boots) != 1 || cb.Boots[0].ID != 3020 {
		t.Errorf("boots = %+v", cb.Boots)
	}
	if len(cb.RunePages) != 1 || cb.RunePages[0].KeystoneID != 8229 {
		t.Errorf("runes = %+v", cb.RunePages)
	}
	if len(cb.Skills) != 1 {
		t.Fatalf("skills = %+v, se esperaba solo la válida", cb.Skills)
	}
	if got := string(cb.Skills[0].Sequence[:]); got != "QWEQQRQWQWRWWEEREE" {
		t.Errorf("secuencia = %q", got)
	}
}

func TestParseInvalidPosition(t *testing.T) {
	bad := `{"champion_id":1,"champion_key":"X","position":9,"overview":[1,1]}`
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("posición 9 debía fallar")
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"103-3.json": ahriMid,
		"README.md":  "ignorado",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cbs, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cbs) != 1 {
		t.Fatalf("cargados %d, se esperaba 1", len(cbs))
	}
	if _, err := LoadDir(t.TempDir()); err == nil {
		t.Error("directorio vacío debía fallar")
	}
}
