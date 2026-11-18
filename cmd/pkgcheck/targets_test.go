//go:build packages

// Pruebas de selección de gestores en pkgcheck.
//
// El bug: pkgcheck ejecutaba siempre checkCask, checkScoop y checkWinget.
// Con targets=homebrew,scoop sus manifiestos de WinGet no existen, así que
// la verificación fallaba por un gestor que no se iba a publicar.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseTargets cubre la validación de la opción -targets.
func TestParseTargets(t *testing.T) {
	valid := []struct {
		raw  string
		want []string
	}{
		{"homebrew,scoop", []string{"homebrew", "scoop"}},
		{"homebrew,scoop,winget", []string{"homebrew", "scoop", "winget"}},
		{"homebrew", []string{"homebrew"}},
		{"scoop", []string{"scoop"}},
		{"winget", []string{"winget"}},
		// Espacios y orden invertido: se aceptan y se normalizan.
		{" scoop , homebrew ", []string{"homebrew", "scoop"}},
	}

	for _, tc := range valid {
		got, err := parseTargets(tc.raw)
		if err != nil {
			t.Errorf("parseTargets(%q) devolvió error: %v", tc.raw, err)
			continue
		}
		list := sortedTargets(got)
		if strings.Join(list, ",") != strings.Join(tc.want, ",") {
			t.Errorf("parseTargets(%q) = %v, se esperaba %v", tc.raw, list, tc.want)
		}
	}

	// Deben rechazarse: vacío, solo espacios, elemento vacío, desconocido
	// y duplicado. Un duplicado no es inocuo: quien escribe
	// 'homebrew,homebrew' esperaba dos gestores.
	invalid := []struct {
		raw string
		why string
	}{
		{"", "vacío"},
		{"   ", "solo espacios"},
		{"homebrew,", "elemento vacío al final"},
		{",scoop", "elemento vacío al principio"},
		{"homebrew,,scoop", "elemento vacío en medio"},
		{"chocolatey", "gestor desconocido"},
		{"homebrew,chocolatey", "uno desconocido entre válidos"},
		{"homebrew,homebrew", "duplicado"},
		{"scoop,SCOOP", "duplicado con distinto caso no es válido"},
	}

	for _, tc := range invalid {
		if _, err := parseTargets(tc.raw); err == nil {
			t.Errorf("parseTargets(%q) debía fallar (%s) y no lo hizo", tc.raw, tc.why)
		}
	}
}

// TestSoloVerificaSeleccionados: con un dist/ que solo tiene Homebrew y
// Scoop, pedir esos dos debe pasar y pedir winget debe fallar.
func TestSoloVerificaSeleccionados(t *testing.T) {
	dist := t.TempDir()
	mustWrite(t, filepath.Join(dist, "homebrew", "Casks", "mobatui.rb"), validCask)
	mustWrite(t, filepath.Join(dist, "scoop", "bucket", "mobatui.json"), validScoop)

	// Sin red: la estructura se comprueba, los assets se omiten.
	newChecker := func(targets string) *checker {
		sel, err := parseTargets(targets)
		if err != nil {
			t.Fatalf("parseTargets(%q): %v", targets, err)
		}
		return &checker{tag: "v0.2.1", dist: dist, targets: sel, requireAssets: false}
	}

	// Los dos seleccionados existen: no debe haber fallos estructurales.
	c := newChecker("homebrew,scoop")
	c.checkCask()
	c.checkScoop()
	if len(c.failures) != 0 {
		t.Errorf("homebrew,scoop no debía fallar: %v", c.failures)
	}
	if c.total == 0 {
		t.Error("no se ejecutó ninguna comprobación")
	}

	// Winget seleccionado pero sin manifiestos: debe fallar.
	c = newChecker("winget")
	c.checkWinget()
	if len(c.failures) == 0 {
		t.Error("winget sin manifiestos debía fallar y no lo hizo")
	}

	// Homebrew seleccionado sin Cask: debe fallar.
	c = newChecker("homebrew")
	c.dist = t.TempDir()
	c.checkCask()
	if len(c.failures) == 0 {
		t.Error("homebrew sin Cask debía fallar y no lo hizo")
	}
}

// TestNoExigeOmitidos: nunca se llama al verificador de un gestor no
// seleccionado, así que su ausencia no puede producir fallos.
func TestNoExigeOmitidos(t *testing.T) {
	dist := t.TempDir()
	// Solo existe el Cask: ni scoop ni winget.
	mustWrite(t, filepath.Join(dist, "homebrew", "Casks", "mobatui.rb"), validCask)

	sel, err := parseTargets("homebrew")
	if err != nil {
		t.Fatal(err)
	}
	c := &checker{tag: "v0.2.1", dist: dist, targets: sel, requireAssets: false}

	// Lo que hace main: solo los seleccionados.
	if sel[targetHomebrew] {
		c.checkCask()
	}
	if sel[targetScoop] {
		c.checkScoop()
	}
	if sel[targetWinget] {
		c.checkWinget()
	}

	if len(c.failures) != 0 {
		t.Errorf("con targets=homebrew no debe exigirse scoop ni winget: %v", c.failures)
	}
}

// TestManifiestoInvalidoFalla: un Cask mal formado debe detectarse.
func TestManifiestoInvalidoFalla(t *testing.T) {
	dist := t.TempDir()
	mustWrite(t, filepath.Join(dist, "homebrew", "Casks", "mobatui.rb"),
		"cask \"otro\" do\n  version \"0.2.1\"\nend\n")

	sel, _ := parseTargets("homebrew")
	c := &checker{tag: "v0.2.1", dist: dist, targets: sel, requireAssets: false}
	c.checkCask()

	if len(c.failures) == 0 {
		t.Error("un Cask con nombre incorrecto debía fallar")
	}
	// El mensaje real es: cask: falta "cask \"mobatui\""
	found := false
	for _, f := range c.failures {
		if strings.Contains(f, `falta "cask \"mobatui\""`) {
			found = true
		}
	}
	if !found {
		t.Errorf("el fallo debía mencionar el nombre esperado: %v", c.failures)
	}
}

// TestWorkflowPasaTargets: regresión sobre el YAML. Si alguien quita
// -targets, pkgcheck volvería a verificar los tres gestores siempre.
func TestWorkflowPasaTargets(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "publish-packages.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("workflow no accesible: %v", err)
	}
	src := string(raw)

	if !strings.Contains(src, `-targets "$TARGETS"`) {
		t.Error("publish-packages.yml no pasa -targets a pkgcheck")
	}
	if !strings.Contains(src, "TARGETS: ${{ inputs.targets }}") {
		t.Error("publish-packages.yml no define TARGETS desde inputs.targets")
	}
	// No debe haberse desactivado la verificación de assets.
	if strings.Contains(src, "cmd/pkgcheck") && strings.Contains(src, "-no-network") {
		t.Error("no debe usarse -no-network en el workflow de publicación: " +
			"desactivaría la comprobación de assets reales")
	}
}

const validCask = `cask "mobatui" do
  version "0.2.1"

  on_macos do
    on_arm do
      sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
      url "https://github.com/Eion21/mobatui/releases/download/v0.2.1/mobatui_#{version}_darwin_arm64.tar.gz"
    end
    on_intel do
      sha256 "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
      url "https://github.com/Eion21/mobatui/releases/download/v0.2.1/mobatui_#{version}_darwin_amd64.tar.gz"
    end
  end

  binary "mobatui"
end
`

const validScoop = `{
  "version": "0.2.1",
  "architecture": {
    "64bit": {
      "url": "https://github.com/Eion21/mobatui/releases/download/v0.2.1/mobatui_0.2.1_windows_amd64.zip",
      "bin": ["mobatui.exe"],
      "hash": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
    }
  },
  "homepage": "https://github.com/Eion21/mobatui",
  "license": "MIT",
  "description": "test"
}
`

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
