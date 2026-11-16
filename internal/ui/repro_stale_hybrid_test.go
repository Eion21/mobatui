package ui

import "testing"

// Repro: una respuesta tardía/obsoleta no debe reemplazar una carga más nueva en curso.
func TestReproStaleHybridDoesNotReplacePending(t *testing.T) {
	a := hybridApp(t, nil, 12, 120, 40)
	a.session.HybridKey = "nueva"
	a.session.Hybrids = map[string]*HybridBuildView{"Abrams": nil} // carga en curso más nueva

	u, _ := a.Update(hybridLoadedMsg{ChampKey: "Abrams", Key: "vieja", Build: sampleHybrid()})
	a = u.(App)

	if a.session.Hybrids["Abrams"] != nil {
		t.Fatalf("la respuesta obsoleta reemplazó la carga en curso: %#v", a.session.Hybrids["Abrams"])
	}
}
