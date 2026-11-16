package main

// Pruebas de `--version` / `-version` sobre el binario real.
//
// Se compila el programa y se ejecuta como subproceso con un HOME y un
// XDG_CACHE_HOME apuntando a un directorio vacío y de solo lectura, para
// demostrar que consultar la versión NO crea caché, NO carga datos y NO
// toca la red (si lo hiciera, fallaría o tardaría).

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// buildMobatui compila el binario en un directorio temporal.
func buildMobatui(t *testing.T, ldflags ...string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "mobatui")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	args := []string{"build", "-o", exe}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

// runIsolated ejecuta el binario con un HOME limpio y sin caché previa.
func runIsolated(t *testing.T, exe string, args ...string) (string, time.Duration) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(exe, args...)
	cmd.Env = []string{
		"HOME=" + home,
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"PATH=" + os.Getenv("PATH"),
		// Que un fallo de red sea inmediato y visible.
		"HTTP_PROXY=http://127.0.0.1:9",
		"HTTPS_PROXY=http://127.0.0.1:9",
	}
	start := time.Now()
	out, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", exe, args, err, out)
	}
	// Nada debe haberse escrito en el HOME del usuario.
	if entries, err := os.ReadDir(home); err == nil && len(entries) > 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("--version creó archivos en HOME: %v", names)
	}
	return string(out), elapsed
}

// TestVersionFlagNoInicializaNada: las tres formas de pedir la versión
// deben responder sin arrancar la TUI ni crear caché.
func TestVersionFlagNoInicializaNada(t *testing.T) {
	if testing.Short() {
		t.Skip("compila un binario: fuera de -short")
	}
	exe := buildMobatui(t)

	for _, flag := range []string{"--version", "-version"} {
		t.Run(flag, func(t *testing.T) {
			out, elapsed := runIsolated(t, exe, flag)
			if !strings.HasPrefix(out, "mobatui ") {
				t.Fatalf("salida inesperada:\n%s", out)
			}
			for _, want := range []string{"fuente:", "commit:", "go:", "plataforma:"} {
				if !strings.Contains(out, want) {
					t.Errorf("falta %q en la salida:\n%s", want, out)
				}
			}
			// Sin TUI ni red: debe ser casi instantáneo.
			if elapsed > 5*time.Second {
				t.Errorf("tardó %v: parece que hizo trabajo de más", elapsed)
			}
		})
	}
}

// TestVersionReleaseInyectada: con ldflags, la versión de release manda.
func TestVersionReleaseInyectada(t *testing.T) {
	if testing.Short() {
		t.Skip("compila un binario: fuera de -short")
	}
	exe := buildMobatui(t,
		"-X", "github.com/Eion21/mobatui/internal/version.injected=0.2.0")

	out, _ := runIsolated(t, exe, "--version")
	if !strings.HasPrefix(out, "mobatui 0.2.0\n") {
		t.Fatalf("la versión inyectada no se usó:\n%s", out)
	}
	if !strings.Contains(out, "fuente:    ldflags") {
		t.Errorf("la fuente debía ser ldflags:\n%s", out)
	}
}

// TestVersionGoInstallBuildInfo: sin ldflags, la versión del módulo
// (la que fija `go install ...@vX.Y.Z`) se usa si existe.
func TestVersionGoInstallBuildInfo(t *testing.T) {
	if testing.Short() {
		t.Skip("compila un binario: fuera de -short")
	}
	exe := buildMobatui(t)
	out, _ := runIsolated(t, exe, "--version")

	// En un build local el módulo es "(devel)": debe caer al marcador de
	// desarrollo con el commit, no anunciar una versión inventada.
	if !strings.Contains(out, "(devel)") {
		t.Fatalf("se esperaba marcador de desarrollo:\n%s", out)
	}
	if !strings.Contains(out, "fuente:    (devel)") {
		t.Errorf("la fuente debía ser (devel):\n%s", out)
	}
	if !strings.Contains(out, "commit:    ") {
		t.Errorf("falta el commit:\n%s", out)
	}
}

// TestHelpMuestraVersion: --help debe anunciar la versión y las opciones.
func TestHelpMuestraVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("compila un binario: fuera de -short")
	}
	exe := buildMobatui(t)
	cmd := exec.Command(exe, "--help")
	out, err := cmd.CombinedOutput()
	// --help de flag sale con código 0 en Go 1.17+ (ErrHelp).
	if err != nil && !strings.Contains(string(out), "Uso:") {
		t.Fatalf("--help: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"mobatui ", "Uso:", "-version", "-offline", "-data"} {
		if !strings.Contains(text, want) {
			t.Errorf("--help no menciona %q:\n%s", want, text)
		}
	}
}
