package ui

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDeadlockRouteSemanticsStates(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  *runsLoadedMsg
	}{
		{name: "loading"},
		{name: "success", msg: &runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{{Item: 9002, MedT: 300}, {Item: 9000, MedT: 700}, {Item: 9001, MedT: 1300}}}},
		{name: "error", msg: &runsLoadedMsg{ChampKey: "Abrams", Err: errors.New("sin red")}},
		{name: "empty", msg: &runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := routeLoadingApp(t, 4, 100)
			if tc.msg != nil {
				a = routeLoadingUpdate(a, *tc.msg)
			}
			check := func(view string) {
				t.Helper()
				lower := strings.ToLower(view)
				for _, claim := range []string{"situacional", "alternativa", "óptima", "óptimo"} {
					if strings.Contains(lower, claim) {
						t.Errorf("unsupported claim %q in %s", claim, view)
					}
				}
			}
			got := routeLoadingText(a)
			check(got)
			if tc.name == "success" {
				// The compact header appears before EARLY; long explanations
				// are available in the item popup, not repeated per phase.
				for _, want := range []string{"Ruta observada", "Fuera de la ruta", "Compras registradas en las estadísticas generales que no forman parte de esta ruta"} {
					if !strings.Contains(got, want) {
						t.Errorf("missing %q: %s", want, got)
					}
				}
				// EARLY must be the first section after the compact header.
				assertInOrder(t, got, "Ruta observada", "EARLY", "Objeto prueba 02", "MID", "Objeto prueba 00", "LATE", "Objeto prueba 01", "Fuera de la ruta", "Objeto prueba 03")
				// Verbose description no longer shown per phase.
				if strings.Contains(got, "Orden habitual observado en partidas recientes") {
					t.Error("verbose description should not appear above EARLY phases")
				}
			}
			a = routeLoadingUpdate(a, keyMsg("enter"))
			check(routeLoadingText(a))
			a = routeLoadingUpdate(a, keyMsg("right"))
			if !strings.Contains(routeLoadingText(a), "Objeto prueba") {
				t.Error("popup navigation lost the selected item")
			}
			check(routeLoadingText(a))
		})
	}
}

func TestDeadlockRouteProvenance(t *testing.T) {
	for _, known := range []bool{false, true} {
		t.Run(fmt.Sprintf("known=%v", known), func(t *testing.T) {
			a := routeLoadingApp(t, 4, 100)
			step := RouteStep{Item: 9000, MedT: 300, N: 41, Share: .3}
			if known {
				v := reflect.ValueOf(&step).Elem()
				size, share := v.FieldByName("SampleSize"), v.FieldByName("MinShare")
				if !size.IsValid() || !share.IsValid() {
					t.Fatal("route step does not carry actual sample size and minimum share")
				}
				size.SetInt(137)
				share.SetFloat(.3)
			}
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{step}})
			got := routeLoadingText(a)
			lower := strings.ToLower(got)
			for _, word := range []string{"estadísticas generales", "muestras", "diferentes"} {
				if !strings.Contains(lower, word) {
					t.Errorf("missing sample disclaimer %q: %s", word, got)
				}
			}
			if known {
				if !strings.Contains(got, "137") || !(strings.Contains(got, "30%") || strings.Contains(got, "30.0%") || strings.Contains(got, "30 %")) {
					t.Errorf("missing actual sample or threshold: %s", got)
				}
			} else if strings.Contains(got, "137") || strings.Contains(got, "250 partidas") || strings.Contains(got, "30%") || strings.Contains(got, "30.0%") {
				t.Errorf("invented provenance with unknown metadata: %s", got)
			}
		})
	}
}

func TestDeadlockRouteDescriptionsWrapAndScroll(t *testing.T) {
	for _, width := range []int{60, 100} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := routeLoadingApp(t, 8, 100)
			a.width, a.detail.width = width, width
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{{Item: 9000, MedT: 300}}})
			got := routeLoadingText(a)
			// Compact header always appears; sample disclaimer still present.
			for _, want := range []string{"Ruta observada", "Compras registradas en las estadísticas generales que no forman parte de esta ruta"} {
				if !strings.Contains(got, want) {
					t.Errorf("wrapped description missing %q", want)
				}
			}
			// EARLY is the first section heading after the compact header.
			assertInOrder(t, got, "Ruta observada", "EARLY")
			// No line exceeds the terminal width.
			for _, line := range strings.Split(a.View(), "\n") {
				if lipgloss.Width(line) > width {
					t.Errorf("line exceeds width %d: %q", width, line)
				}
			}
		})
	}
}

// TestCompactHeaderFormat verifies the compact header format before EARLY.
// Wide terminals show sample size + threshold; narrow terminals show at minimum
// "Ruta observada · N partidas". The header must never be truncated.
func TestCompactHeaderFormat(t *testing.T) {
	for _, tc := range []struct {
		name       string
		width      int
		sampleSize int
		minShare   float64
		wantParts  []string
		wantAbsent []string
	}{
		{
			name: "wide_with_provenance", width: 120,
			sampleSize: 137, minShare: 0.3,
			wantParts: []string{"Ruta observada", "137 partidas", "30%"},
		},
		{
			name: "medium_no_threshold", width: 50,
			sampleSize: 137, minShare: 0.3,
			wantParts: []string{"Ruta observada", "137 partidas"},
		},
		{
			name: "no_provenance", width: 120,
			sampleSize: 0, minShare: 0,
			wantParts: []string{"Ruta observada"},
			// Should not invent numbers.
			wantAbsent: []string{"137", "30%", "250 partidas"},
		},
		{
			name: "narrow_terminal", width: 40,
			sampleSize: 137, minShare: 0.3,
			wantParts: []string{"Ruta observada"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := routeLoadingApp(t, 3, 100)
			a.width, a.detail.width = tc.width, tc.width
			step := RouteStep{Item: 9000, MedT: 300, SampleSize: tc.sampleSize, MinShare: tc.minShare}
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{step}})
			got := routeLoadingText(a)
			for _, want := range tc.wantParts {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q in view: %s", want, got)
				}
			}
			for _, absent := range tc.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("should not contain %q: %s", absent, got)
				}
			}
			// No line must exceed terminal width.
			for _, line := range strings.Split(a.View(), "\n") {
				if lipgloss.Width(line) > tc.width {
					t.Errorf("line exceeds width %d: %q", tc.width, line)
				}
			}
		})
	}
}

// TestEarlyIsFirstSectionAfterHeader verifies that after a successful route
// load, EARLY appears immediately after the compact header (no extra
// descriptions between them).
func TestEarlyIsFirstSectionAfterHeader(t *testing.T) {
	for _, width := range []int{40, 80, 120, 200} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			a := routeLoadingApp(t, 3, 100)
			a.width, a.detail.width = width, width
			step := RouteStep{Item: 9000, MedT: 300, SampleSize: 137, MinShare: 0.3}
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{step}})
			got := routeLoadingText(a)
			// Find "Ruta observada" and "EARLY" positions.
			hdrPos := strings.Index(got, "Ruta observada")
			earlyPos := strings.Index(got, "EARLY")
			if hdrPos < 0 {
				t.Fatal("compact header 'Ruta observada' not found")
			}
			if earlyPos < 0 {
				t.Fatal("'EARLY' phase heading not found")
			}
			if earlyPos < hdrPos {
				t.Error("EARLY appears before compact header")
			}
			// Nothing between header and EARLY should be a phase heading.
			between := got[hdrPos+len("Ruta observada") : earlyPos]
			for _, phaseName := range []string{"MID", "LATE"} {
				if strings.Contains(between, phaseName) {
					t.Errorf("phase %q appears between header and EARLY", phaseName)
				}
			}
		})
	}
}

// TestRouteHeaderNarrowFits verifies that the compact header always fits
// within the terminal width for terminals as narrow as 30 columns.
func TestRouteHeaderNarrowFits(t *testing.T) {
	for _, width := range []int{30, 40, 50, 60} {
		t.Run(fmt.Sprintf("w%d", width), func(t *testing.T) {
			a := routeLoadingApp(t, 2, 100)
			a.width, a.detail.width = width, width
			step := RouteStep{Item: 9000, MedT: 300, SampleSize: 9999, MinShare: 0.3}
			a = routeLoadingUpdate(a, runsLoadedMsg{ChampKey: "Abrams", Steps: []RouteStep{step}})
			// The view must not exceed the terminal width on any line.
			for _, line := range strings.Split(a.View(), "\n") {
				if lipgloss.Width(line) > width {
					t.Errorf("width %d: line too wide (%d): %q", width, lipgloss.Width(line), line)
				}
			}
			// Header must still appear.
			got := routeLoadingText(a)
			if !strings.Contains(got, "Ruta observada") {
				t.Errorf("compact header missing at width %d", width)
			}
		})
	}
}
