package lolalytics

import (
	"os"
	"testing"
)

// TestDebugVivo inspecciona una página real descargada a mano.
// Uso: curl -o /tmp/ahri_mid.html 'https://lolalytics.com/lol/ahri/build/?lane=middle'
// BUILDS_DEBUG_PAGE=/tmp/ahri_mid.html go test ./internal/lolalytics/ -run DebugVivo -v
func TestDebugVivo(t *testing.T) {
	path := os.Getenv("BUILDS_DEBUG_PAGE")
	if path == "" {
		t.Skip("solo con BUILDS_DEBUG_PAGE")
	}
	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	q, err := parseQwik(html)
	if err != nil {
		t.Fatal(err)
	}
	m := q.mainData()
	if m == nil {
		t.Fatal("sin mainData")
	}
	summary := q.obj(m["summary"])
	t.Logf("summary: %v", summary)
	for _, k := range []string{"pick", "win"} {
		qb := q.obj(summary[k])
		if qb == nil {
			t.Logf("%s: nil", k)
			continue
		}
		so := q.obj(qb["skillorder"])
		t.Logf("%s.skillorder = %v", k, so)
		if so != nil {
			v := q.scalar(so["id"])
			t.Logf("  id=%#v n=%v wr=%v", v, q.int(so["n"]), q.num(so["wr"]))
		}
		r := q.obj(qb["runes"])
		if r != nil {
			t.Logf("%s.runes = %v", k, r)
		}
	}
}
