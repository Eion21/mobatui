// Package version resuelve la versión de mobatui sin tocar la red, la
// caché ni la TUI. Lo usan `--version`, `--help` y los tests.
//
// Prioridad:
//  1. Versión inyectada por GoReleaser con -ldflags "-X ...".
//  2. Versión del módulo desde runtime/debug.ReadBuildInfo() (cubre
//     `go install ...@vX.Y.Z`).
//  3. Identificador de desarrollo con el commit, si se conoce.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// injected la rellena GoReleaser en los builds de release:
//
//	-X github.com/Eion21/mobatui/internal/version.injected={{.Version}}
//
// Vacía en compilaciones normales (go build / go install).
var injected string

// devVersion es lo que se muestra cuando no hay ni inyección ni
// metadatos del módulo: un binario compilado a mano desde el repo.
const devVersion = "(devel)"

// Info es la versión resuelta y de dónde salió.
type Info struct {
	// Version es la versión legible ("0.1.1", "0.2.0-dev+abc1234").
	Version string
	// Commit es el hash corto si se conoce, o "".
	Commit string
	// Source indica de dónde salió la versión.
	Source Source
}

// Source documenta la procedencia de la versión, para diagnóstico.
type Source string

const (
	SourceInjected Source = "ldflags"   // GoReleaser
	SourceModule   Source = "buildinfo" // runtime/debug.ReadBuildInfo
	SourceDev      Source = "(devel)"   // compilación local
)

// Get resuelve la versión del binario actual.
func Get() Info {
	return resolve(injected, readBuildInfo)
}

// buildInfo es lo mínimo que necesitamos de debug.BuildInfo, para poder
// inyectarlo en los tests sin compilar binarios distintos.
type buildInfo struct {
	ok       bool
	version  string
	commit   string
	modified bool
}

// readBuildInfo lee los metadatos que Go incrusta al compilar.
func readBuildInfo() buildInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return buildInfo{}
	}
	out := buildInfo{ok: true, version: bi.Main.Version}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			out.commit = s.Value
		case "vcs.modified":
			out.modified = s.Value == "true"
		}
	}
	return out
}

// resolve aplica la prioridad. Es una función pura para poder probar los
// tres orígenes sin recompilar.
func resolve(ldflags string, read func() buildInfo) Info {
	if v := normalizeTag(strings.TrimSpace(ldflags)); v != "" {
		return Info{Version: v, Source: SourceInjected}
	}

	bi := read()
	commit := shortCommit(bi.commit)

	// Go deja "(devel)" o "v0.2.0-0.2025...-dirty" en builds locales; un
	// módulo publicado trae "v1.2.3". Solo sirve el segundo.
	if bi.ok {
		if v := moduleVersion(bi.version); v != "" {
			return Info{Version: v, Commit: commit, Source: SourceModule}
		}
	}

	// Desarrollo: "0.2.0-dev.<commit>" cuando se conoce el commit, o
	// "(devel)" si no. El sufijo ".dirty" delata cambios sin commitear.
	ver := devVersion
	if commit != "" {
		ver = fmt.Sprintf("%s+%s", devVersion, commit)
	}
	if bi.modified {
		ver += ".dirty"
	}
	return Info{Version: ver, Commit: commit, Source: SourceDev}
}

// normalizeTag normaliza "v0.1.1" → "0.1.1". Deja pasar tal cual lo que
// ya viene sin prefijo.
func normalizeTag(v string) string {
	return strings.TrimPrefix(v, "v")
}

// moduleVersion devuelve la versión si el módulo fue instalado con
// `go install ...@vX.Y.Z`. Solo se acepta un tag de release estable.
//
// Antes se filtraban cadenas parciales ("-0.", "+dirty"), lo que dejaba
// pasar pseudoversiones de Go con otra forma: en un build de Pull
// Request sin tag, `debug.BuildInfo` informa algo como
// `v0.0.0-20261117182017-432de1f90106`, que se anunciaba como release
// en vez de como desarrollo.
//
// La validación es ahora estricta: `v` + versión semántica completa, sin
// prerelease ni metadatos de build. Es la forma que garantiza que solo
// un tag de release publicado supere el filtro.
func moduleVersion(v string) string {
	if !isStableTag(v) {
		return ""
	}
	return normalizeTag(v)
}

// isStableTag reconoce exactamente `vMAJOR.MINOR.PATCH` con números
// decimales. Rechaza, entre otros:
//
//	v0.0.0-20261117182017-432de1f90106   pseudoversión de un checkout
//	v0.2.0-0.20261115161714-eb69f29de0a0 pseudoversión sobre un tag
//	v0.1.1-0.20261115161714-eb69f29de0a0 idem
//	v0.2.0-rc.1                          prerelease
//	v0.2.0+dirty                         árbol sucio
//	v0.2 / v1 / V0.2.0 / (devel)         incompletas o marcadores
//
// Es intencionadamente estricto: cualquier forma no reconocida se trata
// como "sin versión de release", que es el comportamiento seguro.
func isStableTag(v string) bool {
	if len(v) < 2 || v[0] != 'v' {
		return false
	}
	parts := strings.Split(v[1:], ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if !isDecimal(p) {
			return false
		}
	}
	return true
}

// isDecimal dice si s son dígitos ASCII no vacíos. Sin ceros a la
// izquierda no hace falta: una versión así sería inválida de todas
// formas y no es ambiguo para comparar.
func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// shortCommit recorta el hash VCS a 7 caracteres.
func shortCommit(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// String devuelve la línea que imprime `--version`.
func (i Info) String() string {
	if i.Commit != "" && i.Source != SourceInjected {
		return fmt.Sprintf("mobatui %s (%s, commit %s)", i.Version, i.Source, i.Commit)
	}
	return fmt.Sprintf("mobatui %s", i.Version)
}

// Line es la versión en una sola línea, sin la etiqueta de procedencia:
// la que acompaña a --help y a la barra de estado.
func (i Info) Line() string {
	return "mobatui " + i.Version
}

// Verbose es el volcado de `--version`: versión, procedencia, commit y
// toolchain. Sin red ni caché.
func (i Info) Verbose() string {
	var b strings.Builder
	fmt.Fprintf(&b, "mobatui %s\n", i.Version)
	fmt.Fprintf(&b, "  fuente:    %s\n", i.Source)
	if i.Commit != "" {
		fmt.Fprintf(&b, "  commit:    %s\n", i.Commit)
	} else {
		b.WriteString("  commit:    (desconocido)\n")
	}
	fmt.Fprintf(&b, "  go:        %s\n", runtime.Version())
	fmt.Fprintf(&b, "  plataforma: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return b.String()
}
