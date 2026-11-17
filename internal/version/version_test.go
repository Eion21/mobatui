package version

// Pruebas de la resolución de versión. No tocan red, caché ni disco: la
// lectura de buildinfo se inyecta, así que los tres orígenes (Release,
// `go install` y desarrollo) se cubren sin recompilar binarios.

import (
	"strings"
	"testing"
)

// buildInfoDe rellena los campos con la misma forma que deja Go.
func buildInfoDe(version, commit string, modified bool) buildInfo {
	return buildInfo{ok: true, version: version, commit: commit, modified: modified}
}

// sinBuildInfo simula un binario sin metadatos (p. ej. -buildvcs=false
// y sin módulo).
func sinBuildInfo() buildInfo { return buildInfo{} }

func TestResolvePrioridad(t *testing.T) {
	tests := []struct {
		name        string
		injected    string
		bi          buildInfo
		wantVersion string
		wantSource  Source
		wantCommit  string
	}{
		{
			name:        "release con ldflags gana a todo",
			injected:    "0.2.0",
			bi:          buildInfoDe("v9.9.9", "0123456789abcdef", false),
			wantVersion: "0.2.0",
			wantSource:  SourceInjected,
		},
		{
			name:        "ldflags con prefijo v se normaliza",
			injected:    "v0.2.0",
			bi:          sinBuildInfo(),
			wantVersion: "0.2.0",
			wantSource:  SourceInjected,
		},
		{
			name:        "go install trae versión del módulo",
			injected:    "",
			bi:          buildInfoDe("v0.2.0", "0123456789abcdef", false),
			wantVersion: "0.2.0",
			wantSource:  SourceModule,
			wantCommit:  "0123456",
		},
		{
			name:        "go install con árbol modificado sigue siendo release",
			injected:    "",
			bi:          buildInfoDe("v0.2.0", "0123456789abcdef", true),
			wantVersion: "0.2.0",
			wantSource:  SourceModule,
			wantCommit:  "0123456",
		},
		{
			name:        "build local con (devel) y commit",
			injected:    "",
			bi:          buildInfoDe("(devel)", "05e64b40707a88f8", false),
			wantVersion: "(devel)+05e64b4",
			wantSource:  SourceDev,
			wantCommit:  "05e64b4",
		},
		{
			name:        "build local con (devel) y cambios sin commitear",
			injected:    "",
			bi:          buildInfoDe("(devel)", "05e64b40707a88f8", true),
			wantVersion: "(devel)+05e64b4.dirty",
			wantSource:  SourceDev,
			wantCommit:  "05e64b4",
		},
		{
			name:        "build local sin ninguna metainformación",
			injected:    "",
			bi:          sinBuildInfo(),
			wantVersion: "(devel)",
			wantSource:  SourceDev,
		},
		{
			name:        "pseudoversión de go install local no es release",
			injected:    "",
			bi:          buildInfoDe("v0.2.0-0.20251115193000-05e64b40707a", "05e64b40707a88f8", false),
			wantVersion: "(devel)+05e64b4",
			wantSource:  SourceDev,
			wantCommit:  "05e64b4",
		},
		{
			name:        "versión de módulo sucia no se anuncia como release",
			injected:    "",
			bi:          buildInfoDe("v0.2.0+dirty", "", true),
			wantVersion: "(devel).dirty",
			wantSource:  SourceDev,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolve(tt.injected, func() buildInfo { return tt.bi })
			if got.Version != tt.wantVersion {
				t.Errorf("Version = %q, se esperaba %q", got.Version, tt.wantVersion)
			}
			if got.Source != tt.wantSource {
				t.Errorf("Source = %q, se esperaba %q", got.Source, tt.wantSource)
			}
			if tt.wantCommit != "" && got.Commit != tt.wantCommit {
				t.Errorf("Commit = %q, se esperaba %q", got.Commit, tt.wantCommit)
			}
		})
	}
}

// TestStringFormat revisa la línea única y el volcado de --version.
func TestStringFormat(t *testing.T) {
	release := Info{Version: "0.2.0", Source: SourceInjected}
	if got := release.String(); got != "mobatui 0.2.0" {
		t.Errorf("String() = %q", got)
	}
	if got := release.Line(); got != "mobatui 0.2.0" {
		t.Errorf("Line() = %q", got)
	}

	dev := Info{Version: "(devel)+05e64b4", Commit: "05e64b4", Source: SourceDev}
	if got := dev.String(); !strings.Contains(got, "commit 05e64b4") {
		t.Errorf("String() de desarrollo debe nombrar el commit, dio %q", got)
	}
	if got := dev.Line(); got != "mobatui (devel)+05e64b4" {
		t.Errorf("Line() = %q", got)
	}

	verbose := dev.Verbose()
	for _, want := range []string{"mobatui (devel)+05e64b4", "commit:    05e64b4", "go:", "plataforma:"} {
		if !strings.Contains(verbose, want) {
			t.Errorf("Verbose() no contiene %q:\n%s", want, verbose)
		}
	}
	// Sin commit conocido debe decirlo, no inventarlo.
	sin := Info{Version: "0.2.0", Source: SourceInjected}.Verbose()
	if !strings.Contains(sin, "(desconocido)") {
		t.Errorf("Verbose() sin commit debe indicarlo:\n%s", sin)
	}
}

// TestGetNoTocaNada: Get() debe resolver con lo que ya está en el
// binario. Si tocara la red o la caché, este test lo delataría por el
// tiempo o por fallar en un entorno aislado.
func TestGetNoTocaNada(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Fatal("Get() devolvió una versión vacía")
	}
	if info.Source == "" {
		t.Fatal("Get() devolvió un origen vacío")
	}
	// La versión nunca debe llevar espacios ni la etiqueta de origen
	// mezclada: se imprime tal cual en --help.
	if strings.ContainsAny(info.Version, " \t\n") {
		t.Fatalf("versión con espacios: %q", info.Version)
	}
}

// TestNormalizeTag cubre el recorte del prefijo.
func TestNormalizeTag(t *testing.T) {
	for in, want := range map[string]string{
		"v0.2.0": "0.2.0",
		"0.2.0":  "0.2.0",
		"":       "",
		"vv0.2":  "v0.2",
	} {
		if got := normalizeTag(in); got != want {
			t.Errorf("normalizeTag(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

// TestModuleVersionSoloTagsEstables: `moduleVersion` acepta únicamente
// un tag de release completo y rechaza todo lo demás.
//
// Regresión del CI de la PR: en un build de Pull Request sin tag,
// debug.BuildInfo informa `v0.0.0-20261117182017-432de1f90106`. El
// filtro anterior solo miraba "-0." y "+dirty", así que esa
// pseudoversión se anunciaba como release en lugar de como desarrollo.
func TestModuleVersionSoloTagsEstables(t *testing.T) {
	acept := []struct{ in, want string }{
		{"v0.1.1", "0.1.1"},
		{"v0.2.0", "0.2.0"},
		{"v1.0.0", "1.0.0"},
		{"v10.20.30", "10.20.30"},
		{"v0.0.1", "0.0.1"},
	}
	for _, c := range acept {
		if got := moduleVersion(c.in); got != c.want {
			t.Errorf("moduleVersion(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}

	rechaz := []string{
		// Pseudoversiones de Go: la que aparece en el CI de una PR.
		"v0.0.0-20261117182017-432de1f90106",
		"v0.2.0-0.20261115161714-eb69f29de0a0",
		"v0.1.1-0.20261115161714-eb69f29de0a0",
		// Prereleases y metadatos.
		"v0.2.0-rc.1",
		"v0.2.0-beta",
		"v0.2.0+dirty",
		"v0.2.0.dirty",
		// Marcadores e incompletas.
		"(devel)",
		"",
		"v0.2",
		"v1",
		"v0.2.0.1",
		"v.1.2",
		"0.2.0",
		"V0.2.0",
		// No numéricas.
		"va.b.c",
		"v0.-1.2",
	}
	for _, v := range rechaz {
		if got := moduleVersion(v); got != "" {
			t.Errorf("moduleVersion(%q) = %q, debía rechazarse", v, got)
		}
	}
}

// TestPseudoversionNoSeAnunciaComoRelease: el caso exacto del CI. Un
// build sin tag debe mostrar (devel) con el commit, no una versión.
func TestPseudoversionNoSeAnunciaComoRelease(t *testing.T) {
	for _, ver := range []string{
		"v0.0.0-20261117182017-432de1f90106",
		"v0.2.0-0.20261115161714-eb69f29de0a0",
		"v0.1.1-0.20261115161714-eb69f29de0a0",
	} {
		bi := buildInfo{ok: true, version: ver, commit: "432de1f90106"}
		info := resolve("", func() buildInfo { return bi })
		if info.Source != SourceDev {
			t.Errorf("%q se anunció como %q, debía ser desarrollo", ver, info.Source)
		}
		if !strings.Contains(info.Version, devVersion) {
			t.Errorf("%q: versión %q sin marcador de desarrollo", ver, info.Version)
		}
	}
	// Un tag estable sí se anuncia, desde el módulo.
	bi := buildInfo{ok: true, version: "v0.2.0", commit: "abc123456789"}
	info := resolve("", func() buildInfo { return bi })
	if info.Version != "0.2.0" || info.Source != SourceModule {
		t.Errorf("un tag estable debía anunciarse: %+v", info)
	}
	// Y ldflags conserva la prioridad sobre el módulo.
	info = resolve("0.2.0", func() buildInfo { return bi })
	if info.Source != SourceInjected {
		t.Errorf("ldflags debía mandar: %s", info.Source)
	}
}
