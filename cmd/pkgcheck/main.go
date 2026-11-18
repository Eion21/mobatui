//go:build packages

// Verificador de los manifiestos de gestores de paquetes.
//
// Comprueba que los manifiestos generados apuntan a artefactos que
// EXISTEN en una release pública concreta, y que su SHA-256 coincide con
// el del archivo servido por GitHub.
//
//	go run -tags packages ./cmd/pkgcheck -tag v0.2.1 -dist dist -repo Eion21/mobatui
//
// La etiqueta de compilación lo excluye de la suite normal: requiere red.
//
// Diferencia clave con un snapshot local: aquí NO se valida contra los
// archivos de dist/, sino contra los assets publicados en GitHub. Un ZIP
// generado en local puede tener el hash correcto y aun así no existir en
// la release: publicar un manifiesto así rompe la instalación del usuario.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// target identifica un gestor de paquetes.
type target string

const (
	targetHomebrew target = "homebrew"
	targetScoop    target = "scoop"
	targetWinget   target = "winget"
)

// parseTargets convierte la lista separada por comas en un conjunto.
//
// Rechaza vacío, duplicados y valores desconocidos. Un duplicado no es
// inocuo: 'homebrew,homebrew' sugiere que quien lo escribió esperaba dos
// gestores distintos, y verificarlo una sola vez ocultaría el error.
func parseTargets(raw string) (map[target]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("vacío; indica al menos uno de homebrew, scoop, winget")
	}

	selected := make(map[target]bool, 3)
	var unknown, dup []string
	for _, part := range strings.Split(trimmed, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("contiene un elemento vacío en %q", raw)
		}
		t := target(name)
		switch t {
		case targetHomebrew, targetScoop, targetWinget:
		default:
			unknown = append(unknown, name)
			continue
		}
		if selected[t] {
			dup = append(dup, name)
			continue
		}
		selected[t] = true
	}

	if len(unknown) > 0 {
		return nil, fmt.Errorf("gestor desconocido: %s (válidos: homebrew, scoop, winget)",
			strings.Join(unknown, ", "))
	}
	if len(dup) > 0 {
		return nil, fmt.Errorf("gestor duplicado: %s", strings.Join(dup, ", "))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("ningún gestor válido en %q", raw)
	}
	return selected, nil
}

// sortedTargets devuelve los seleccionados en orden fijo, para que la
// salida sea reproducible.
func sortedTargets(selected map[target]bool) []string {
	var out []string
	for _, t := range []target{targetHomebrew, targetScoop, targetWinget} {
		if selected[t] {
			out = append(out, string(t))
		}
	}
	return out
}

func main() {
	tag := flag.String("tag", "", "tag publicado, p. ej. v0.2.1")
	dist := flag.String("dist", "dist", "directorio con los manifiestos generados")
	repo := flag.String("repo", "Eion21/mobatui", "repositorio de la release")
	targets := flag.String("targets", "homebrew,scoop,winget",
		"gestores a verificar, separados por coma: homebrew, scoop, winget")
	noNet := flag.Bool("no-network", false, "solo comprobaciones locales (no verifica assets)")
	requireAssets := flag.Bool("require-assets", true, "exigir que cada URL sea un asset real de la release")
	apiBase := flag.String("api-base", "", "raíz de la API de GitHub (por defecto https://api.github.com)")
	flag.Parse()

	if *tag == "" {
		fmt.Fprintln(os.Stderr, "-tag es obligatorio")
		os.Exit(2)
	}

	// Los targets se validan ANTES de tocar nada: un valor vacío,
	// duplicado o desconocido aborta con exit 2 en lugar de verificar
	// menos gestores de los pedidos (o ninguno) y salir con éxito.
	selected, err := parseTargets(*targets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "-targets: %v\n", err)
		os.Exit(2)
	}

	c := &checker{
		tag: *tag, dist: *dist, repo: *repo,
		requireAssets: *requireAssets,
		apiBase:       *apiBase,
		targets:       selected,
	}
	if !*noNet {
		c.client = &http.Client{Timeout: 120 * time.Second}
	}

	fmt.Printf("targets: %s\n", strings.Join(sortedTargets(selected), ","))

	// Solo los verificadores de los gestores seleccionados. Los omitidos
	// no se comprueban: en una ejecución con targets=homebrew,scoop sus
	// manifiestos ni siquiera existen, y exigirlos daría un falso fallo.
	if selected[targetHomebrew] {
		c.checkCask()
	}
	if selected[targetScoop] {
		c.checkScoop()
	}
	if selected[targetWinget] {
		c.checkWinget()
	}

	fmt.Printf("\n%d comprobaciones superadas, %d fallos\n", c.total, len(c.failures))
	for _, f := range c.failures {
		fmt.Printf("  FALLA %s\n", f)
	}
	if len(c.failures) > 0 {
		os.Exit(1)
	}
	if c.total == 0 {
		// Sin comprobaciones no se puede afirmar nada: equivale a fallo.
		fmt.Fprintln(os.Stderr, "no se ejecutó ninguna comprobación")
		os.Exit(1)
	}
	fmt.Printf("manifiestos de %s: apuntan a artefactos publicados y verificados\n",
		strings.Join(sortedTargets(selected), ","))
}

type asset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

type checker struct {
	tag           string
	dist          string
	repo          string
	client        *http.Client
	requireAssets bool
	// apiBase es la raíz de la API de GitHub. Se puede cambiar para
	// apuntar a un servidor de prueba; por defecto, la real.
	apiBase string
	// targets indica qué gestores se verifican. Los no seleccionados no
	// se miran: sus manifiestos no existen en esa ejecución.
	targets  map[target]bool
	total    int
	failures []string

	assets     map[string]asset // nombre -> asset de la release
	assetsOnce bool
	assetsErr  error
}

func (c *checker) fail(f string, a ...any) { c.failures = append(c.failures, fmt.Sprintf(f, a...)) }
func (c *checker) ok()                     { c.total++ }

func (c *checker) expect(cond bool, f string, a ...any) {
	c.total++
	if !cond {
		c.failures = append(c.failures, fmt.Sprintf(f, a...))
	}
}

func (c *checker) read(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		c.fail("no se pudo leer %s: %v", path, err)
		return "", false
	}
	return string(b), true
}

// loadAssets consulta la API de GitHub UNA vez y cachea los assets de la
// release. Es la fuente de verdad: lo que no esté aquí no se puede
// instalar, por muy correcto que sea el manifiesto.
func (c *checker) loadAssets() (map[string]asset, error) {
	if c.assetsOnce {
		return c.assets, c.assetsErr
	}
	c.assetsOnce = true

	if c.client == nil {
		c.assetsErr = fmt.Errorf("sin red: no se pueden verificar los assets")
		return nil, c.assetsErr
	}

	base := c.apiBase
	if base == "" {
		base = "https://api.github.com"
	}
	url := fmt.Sprintf("%s/repos/%s/releases/tags/%s", base, c.repo, c.tag)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		c.assetsErr = err
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.assetsErr = fmt.Errorf("consulta de la release falló: %w", err)
		return nil, c.assetsErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		c.assetsErr = fmt.Errorf("la release %s no existe en %s (HTTP %d): %s",
			c.tag, c.repo, resp.StatusCode, strings.TrimSpace(string(body)))
		return nil, c.assetsErr
	}

	var rel struct {
		TagName    string  `json:"tag_name"`
		Draft      bool    `json:"draft"`
		Prerelease bool    `json:"prerelease"`
		Assets     []asset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		c.assetsErr = fmt.Errorf("respuesta de la API ilegible: %w", err)
		return nil, c.assetsErr
	}

	if rel.TagName != c.tag {
		c.assetsErr = fmt.Errorf("la API devolvió el tag %q, se pedía %q", rel.TagName, c.tag)
		return nil, c.assetsErr
	}
	if rel.Draft {
		c.assetsErr = fmt.Errorf("la release %s es un borrador", c.tag)
		return nil, c.assetsErr
	}

	c.assets = make(map[string]asset, len(rel.Assets))
	for _, a := range rel.Assets {
		c.assets[a.Name] = a
	}
	if len(c.assets) == 0 {
		c.assetsErr = fmt.Errorf("la release %s no tiene assets", c.tag)
		return nil, c.assetsErr
	}
	fmt.Printf("release %s: %d assets publicados\n", c.tag, len(c.assets))
	return c.assets, nil
}

// checkAsset exige que la URL pertenezca a la release declarada y que el
// nombre del archivo exista entre sus assets publicados.
func (c *checker) checkAsset(who, url, wantSHA string) {
	if !c.requireAssets {
		fmt.Printf("  OMITIDO %s: sin verificación de assets (-require-assets=false)\n", who)
		return
	}

	prefix := fmt.Sprintf("https://github.com/%s/releases/download/%s/", c.repo, c.tag)
	c.expect(strings.HasPrefix(url, prefix),
		"%s: la URL no pertenece a la release %s\n    url      %s\n    esperado %s<archivo>",
		who, c.tag, url, prefix)

	name := strings.TrimPrefix(url, prefix)
	if name == url || name == "" {
		// La comprobación anterior ya falló; no seguir con un nombre falso.
		return
	}

	assets, err := c.loadAssets()
	if err != nil {
		c.fail("%s: %v", who, err)
		return
	}

	a, ok := assets[name]
	c.expect(ok, "%s: el asset %q NO existe en la release %s (publicados: %s)",
		who, name, c.tag, strings.Join(sortedKeys(assets), ", "))
	if !ok {
		return
	}
	c.expect(a.Size > 0, "%s: el asset %q está vacío (%d bytes)", who, name, a.Size)

	c.verifyRemoteSHA(who, a, wantSHA)
}

// verifyRemoteSHA descarga el asset desde su URL de API y compara el hash
// con el del manifiesto.
func (c *checker) verifyRemoteSHA(who string, a asset, want string) {
	if want == "" {
		c.fail("%s: hash vacío en el manifiesto", who)
		return
	}

	req, err := http.NewRequest(http.MethodGet, a.URL, nil)
	if err != nil {
		c.fail("%s: %v", who, err)
		return
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		c.fail("%s: descarga de %s falló: %v", who, a.Name, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.fail("%s: HTTP %d al descargar %s", who, resp.StatusCode, a.Name)
		return
	}

	h := sha256.New()
	n, err := io.Copy(h, resp.Body)
	if err != nil {
		c.fail("%s: lectura de %s falló: %v", who, a.Name, err)
		return
	}
	if n == 0 {
		c.fail("%s: %s se descargó vacío", who, a.Name)
		return
	}
	if a.Size > 0 && n != a.Size {
		c.fail("%s: %s tiene %d bytes, la API declara %d", who, a.Name, n, a.Size)
		return
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		c.fail("%s: hash de %s no coincide\n    manifiesto %s\n    publicado   %s", who, a.Name, want, got)
		return
	}
	c.ok()
	fmt.Printf("  OK %s: %s (%d bytes, sha256 %s…)\n", who, a.Name, n, got[:12])
}

// ---------------- Scoop ----------------

func (c *checker) checkScoop() {
	const path = "scoop/bucket/mobatui.json"
	src, ok := c.read(filepath.Join(c.dist, path))
	if !ok {
		return
	}

	var m struct {
		Version      string `json:"version"`
		Architecture map[string]struct {
			URL  string   `json:"url"`
			Bin  []string `json:"bin"`
			Hash string   `json:"hash"`
		} `json:"architecture"`
		Homepage    string `json:"homepage"`
		License     string `json:"license"`
		Description string `json:"description"`
	}
	c.expect(json.Unmarshal([]byte(src), &m) == nil, "scoop: JSON inválido")
	c.expect(m.Version == c.version(), "scoop: version %q, se esperaba %q", m.Version, c.version())
	c.expect(m.Homepage != "", "scoop: homepage vacía")
	c.expect(m.License != "", "scoop: license vacía")
	c.expect(m.Description != "", "scoop: description vacía")

	a, has64 := m.Architecture["64bit"]
	c.expect(has64, "scoop: falta architecture.64bit")
	if !has64 {
		return
	}
	c.expect(len(a.Bin) == 1 && a.Bin[0] == "mobatui.exe",
		"scoop: bin = %v, se esperaba [mobatui.exe]", a.Bin)
	// Scoop usa Expand-Archive: solo .zip.
	c.expect(strings.HasSuffix(a.URL, ".zip"),
		"scoop: la URL no es .zip (Scoop no acepta tar.gz): %s", a.URL)

	c.checkAsset("scoop", a.URL, a.Hash)
}

// ---------------- WinGet ----------------

func (c *checker) wingetDir() (string, bool) {
	base := filepath.Join(c.dist, "winget", "manifests")
	var dirs []string
	_ = filepath.Walk(base, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || !fi.IsDir() {
			return nil
		}
		for _, name := range []string{".installer.yaml", ".locale.en-US.yaml"} {
			if _, e := os.Stat(filepath.Join(p, "Eion21.MOBATUI"+name)); e == nil {
				dirs = append(dirs, p)
				return nil
			}
		}
		return nil
	})
	if len(dirs) == 0 {
		c.fail("winget: no se encontró el directorio de manifiestos en %s", base)
		return "", false
	}
	sort.Strings(dirs)
	return dirs[len(dirs)-1], true
}

func (c *checker) checkWinget() {
	dir, ok := c.wingetDir()
	if !ok {
		return
	}

	inst, ok1 := c.read(filepath.Join(dir, "Eion21.MOBATUI.installer.yaml"))
	loc, ok2 := c.read(filepath.Join(dir, "Eion21.MOBATUI.locale.en-US.yaml"))
	ver, ok3 := c.read(filepath.Join(dir, "Eion21.MOBATUI.yaml"))
	if !ok1 || !ok2 || !ok3 {
		return
	}

	for name, src := range map[string]string{"installer": inst, "locale": loc, "version": ver} {
		c.expect(strings.Contains(src, "PackageIdentifier: Eion21.MOBATUI"),
			"winget %s: falta PackageIdentifier Eion21.MOBATUI", name)
		c.expect(strings.Contains(src, "PackageVersion: "+c.version()),
			"winget %s: falta PackageVersion %s", name, c.version())
	}

	c.expect(strings.Contains(inst, "ManifestType: installer"), "winget: installer sin ManifestType")
	c.expect(strings.Contains(loc, "ManifestType: defaultLocale"), "winget: locale sin ManifestType")
	c.expect(strings.Contains(ver, "ManifestType: version"), "winget: version sin ManifestType")

	for _, k := range []string{"Publisher: ", "PackageName: ", "License: ", "ShortDescription: "} {
		c.expect(strings.Contains(loc, k), "winget locale: falta %s", strings.TrimSpace(k))
	}

	c.expect(strings.Contains(inst, "NestedInstallerType: portable"),
		"winget installer: se esperaba portable")
	c.expect(strings.Contains(inst, "PortableCommandAlias: mobatui"),
		"winget installer: falta PortableCommandAlias mobatui")
	c.expect(strings.Contains(inst, "UpgradeBehavior: uninstallPrevious"),
		"winget installer: falta UpgradeBehavior uninstallPrevious")
	c.expect(strings.Contains(inst, "Architecture: x64"),
		"winget installer: falta Architecture x64")
	c.expect(strings.Contains(inst, "RelativeFilePath: mobatui.exe"),
		"winget installer: falta RelativeFilePath mobatui.exe")
	c.expect(!strings.Contains(inst, "Architecture: arm64"),
		"winget installer: declara arm64 pero Windows arm64 no se publica")

	reURL := regexp.MustCompile(`InstallerUrl:\s*(\S+)`)
	reSHA := regexp.MustCompile(`InstallerSha256:\s*([a-f0-9]{64})`)
	u := reURL.FindStringSubmatch(inst)
	s := reSHA.FindStringSubmatch(inst)
	c.expect(len(u) == 2, "winget installer: sin InstallerUrl")
	c.expect(len(s) == 2, "winget installer: sin InstallerSha256 válido")
	if len(u) != 2 || len(s) != 2 {
		return
	}
	// WinGet extrae un ZIP portable.
	c.expect(strings.HasSuffix(u[1], ".zip"),
		"winget installer: la URL no es .zip: %s", u[1])
	c.checkAsset("winget", u[1], s[1])
}

// ---------------- Homebrew Cask ----------------

func (c *checker) checkCask() {
	var files []string
	_ = filepath.Walk(filepath.Join(c.dist, "homebrew"), func(p string, fi os.FileInfo, err error) error {
		if err == nil && fi != nil && !fi.IsDir() && strings.HasSuffix(p, ".rb") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		c.fail("cask: no se encontró ningún .rb en %s/homebrew", c.dist)
		return
	}
	sort.Strings(files)
	src, ok := c.read(files[0])
	if !ok {
		return
	}

	for _, want := range []string{
		`cask "mobatui"`,
		`binary "mobatui"`,
		"on_macos do",
		"on_arm do",
		"on_intel do",
	} {
		c.expect(strings.Contains(src, want), "cask: falta %q", want)
	}

	// La limpieza de cuarentena NO es obligatoria: se acepta tanto un Cask
	// que la incluye como uno que no. Si está, se avisa de que es una
	// decisión de seguridad explícita (ver docs/PACKAGING.md).
	if strings.Contains(src, "com.apple.quarantine") {
		fmt.Println("  AVISO cask: incluye limpieza de com.apple.quarantine")
	}

	sec := regexp.MustCompile(`(?s)on_macos do(.*?)\n  end`).FindStringSubmatch(src)
	c.expect(len(sec) == 2, "cask: no se pudo aislar on_macos")
	if len(sec) != 2 {
		return
	}
	pairs := regexp.MustCompile(`sha256 "([a-f0-9]{64})"\s*\n\s*url "([^"]+)"`).
		FindAllStringSubmatch(sec[1], -1)
	c.expect(len(pairs) == 2, "cask: se esperaban 2 pares sha256/url en macOS, hay %d", len(pairs))

	seen := map[string]bool{}
	for _, p := range pairs {
		sha, url := p[1], p[2]
		// En el Cask la URL lleva #{version} en lugar del número: se
		// resuelve antes de comprobarla.
		real := strings.ReplaceAll(url, "#{version}", c.version())
		c.expect(strings.Contains(real, c.tag), "cask: la URL no contiene %s: %s", c.tag, real)
		switch {
		case strings.Contains(real, "darwin_arm64"):
			seen["arm64"] = true
		case strings.Contains(real, "darwin_amd64"):
			seen["amd64"] = true
		default:
			c.fail("cask: arquitectura macOS desconocida: %s", real)
			c.total++
		}
		c.checkAsset("cask", real, sha)
	}
	c.expect(seen["arm64"], "cask: falta darwin_arm64")
	c.expect(seen["amd64"], "cask: falta darwin_amd64")
}

func (c *checker) version() string { return strings.TrimPrefix(c.tag, "v") }

func sortedKeys(m map[string]asset) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
