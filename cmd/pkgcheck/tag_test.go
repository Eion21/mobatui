//go:build packages

// Prueba de regresión del checkout del tag en publish-packages.yml.
//
// El bug real: actions/checkout sin 'ref' toma el ref del evento, que en
// workflow_dispatch es el master del momento. Publicar manifiestos para
// v0.2.1 generaba el binario desde el código de master, no desde el tag.
//
// Estas pruebas comprueban la lógica que ahora lo impide. No necesitan
// red ni una release publicada: trabajan con un repositorio git temporal.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// semverTag replica el patrón del workflow.
var semverTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

// gitRepo crea un repositorio de prueba con dos commits y un tag en el
// primero, de modo que HEAD (segundo commit) y el tag NO coinciden.
func gitRepo(t *testing.T) (dir, taggedSHA, headSHA string) {
	t.Helper()
	dir = t.TempDir()

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}

	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "primero")
	taggedSHA = run("rev-parse", "HEAD")
	run("tag", "-a", "v0.2.1", "-m", "release")
	run("commit", "-q", "--allow-empty", "-m", "segundo")
	headSHA = run("rev-parse", "HEAD")

	return dir, taggedSHA, headSHA
}

// resolveTag resuelve un tag a su commit, desreferenciando los anotados,
// igual que hace el workflow con `git rev-parse refs/tags/X^{commit}`.
func resolveTag(t *testing.T, dir, tag string) (string, error) {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "-q", "--verify", "refs/tags/"+tag+"^{commit}")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// TestTagNoEsHead: el caso que motivó el bug. Si se pide v0.2.1 pero HEAD
// es otro commit (master avanzado), la verificación DEBE fallar.
func TestTagNoEsHead(t *testing.T) {
	dir, taggedSHA, headSHA := gitRepo(t)

	if taggedSHA == headSHA {
		t.Fatal("el repositorio de prueba debe tener HEAD distinto del tag")
	}

	got, err := resolveTag(t, dir, "v0.2.1")
	if err != nil {
		t.Fatalf("resolver el tag: %v", err)
	}
	if got != taggedSHA {
		t.Errorf("el tag resolvió a %s, se esperaba %s", got, taggedSHA)
	}

	// Esta es la comprobación del workflow: HEAD == commit del tag.
	if headSHA == got {
		t.Fatal("HEAD coincide con el tag: la prueba no reproduce el bug")
	}
	t.Logf("correcto: HEAD (%s) != tag v0.2.1 (%s) → el workflow debe abortar",
		headSHA[:7], got[:7])
}

// TestTagInexistente: pedir un tag que no existe debe fallar, no caer
// silenciosamente en HEAD.
func TestTagInexistente(t *testing.T) {
	dir, _, _ := gitRepo(t)

	if _, err := resolveTag(t, dir, "v9.9.9"); err == nil {
		t.Fatal("resolver un tag inexistente debía fallar y no lo hizo")
	}
}

// TestTagAnotadoSeDesreferencia: el tag es anotado, así que el SHA del
// objeto tag NO es el del commit. Sin ^{commit} la comparación fallaría
// siempre y el workflow nunca publicaría.
func TestTagAnotadoSeDesreferencia(t *testing.T) {
	dir, taggedSHA, _ := gitRepo(t)

	// SHA del objeto tag (anotado).
	cmd := exec.Command("git", "rev-parse", "refs/tags/v0.2.1")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse del tag: %v", err)
	}
	tagObjSHA := strings.TrimSpace(string(out))

	if tagObjSHA == taggedSHA {
		t.Fatal("el tag debería ser anotado (objeto propio), no ligero")
	}

	got, err := resolveTag(t, dir, "v0.2.1")
	if err != nil {
		t.Fatalf("resolver: %v", err)
	}
	if got != taggedSHA {
		t.Errorf("con ^{commit} debía dar %s, dio %s", taggedSHA, got)
	}
	if got == tagObjSHA {
		t.Error("se usó el SHA del objeto tag en lugar del commit")
	}
}

// TestCheckoutConRefDeTag: simula lo que hace actions/checkout con
// 'ref: refs/tags/X'. Tras él, HEAD debe ser el commit del tag y no el
// de la rama.
func TestCheckoutConRefDeTag(t *testing.T) {
	dir, taggedSHA, headSHA := gitRepo(t)

	cmd := exec.Command("git", "checkout", "-q", "--detach", "refs/tags/v0.2.1")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout del tag: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	now := strings.TrimSpace(string(out))

	if now != taggedSHA {
		t.Errorf("tras checkout del tag, HEAD = %s, se esperaba %s", now, taggedSHA)
	}
	if now == headSHA {
		t.Error("HEAD sigue en el commit de la rama: el checkout no usó el tag")
	}
	t.Logf("checkout con ref de tag deja HEAD en %s (no en %s)", now[:7], headSHA[:7])
}

// TestFormatoDeTag: el workflow rechaza tags que no sean vX.Y.Z antes de
// mirar nada más. Se comprueba aquí para que el patrón no se relaje.
func TestFormatoDeTag(t *testing.T) {
	valid := []string{"v0.2.1", "v1.0.0", "v10.20.30", "v0.0.1"}
	invalid := []string{"0.2.1", "latest", "vabc", "v1.2", "", "V1.2.3"}

	// Mismo criterio que el paso de shell: ^v[0-9]+\.[0-9]+\.[0-9]+$
	isValid := func(tag string) bool {
		return semverTag.MatchString(tag)
	}

	for _, tag := range valid {
		if !isValid(tag) {
			t.Errorf("%q debería ser válido", tag)
		}
	}
	for _, tag := range invalid {
		if isValid(tag) {
			t.Errorf("%q debería rechazarse", tag)
		}
	}
}

// TestWorkflowFijaElRef comprueba que el workflow realmente usa
// 'ref: refs/tags/${{ inputs.tag }}'. Es una prueba de regresión sobre el
// YAML: si alguien quita el ref, este test falla.
func TestWorkflowFijaElRef(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "publish-packages.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("workflow no accesible desde %s: %v", path, err)
	}
	src := string(raw)

	if !strings.Contains(src, "ref: refs/tags/${{ inputs.tag }}") {
		t.Error("publish-packages.yml no fija 'ref: refs/tags/${{ inputs.tag }}' en actions/checkout.\n" +
			"Sin eso, workflow_dispatch usa el master del momento y se generan\n" +
			"manifiestos desde un commit distinto al del tag.")
	}
	if !strings.Contains(src, `refs/tags/$TAG^{commit}`) {
		t.Error("el workflow no desreferencia el tag anotado con ^{commit};\n" +
			"comparar contra el SHA del objeto tag nunca coincidiría con HEAD.")
	}
	// La comprobación de igualdad debe existir.
	if !strings.Contains(src, `test "$HEAD_SHA" = "$TAG_SHA"`) {
		t.Error("falta la comprobación HEAD == commit del tag")
	}
}
