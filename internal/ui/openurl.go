package ui

// Apertura del enlace oficial de un release. Nunca descarga ni instala
// nada: solo entrega la URL al navegador del sistema.

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// errURLNoPermitida se devuelve si el enlace no pertenece al repositorio
// esperado. La validación se repite aquí (defensa en profundidad)
// además de en updatecheck.
var errURLNoPermitida = errors.New("ui: enlace no permitido")

// allowedURLPrefix es el único prefijo que se permite abrir.
const allowedURLPrefix = "https://github.com/Eion21/mobatui/"

// openURL abre el enlace en el navegador del sistema.
func openURL(url string) error {
	if err := validateURL(url); err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		// "start" es un builtin de cmd.exe; el primer argumento vacío
		// ocupa el lugar del título.
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

// validateURL comprueba que el enlace sea del repositorio esperado.
func validateURL(url string) error {
	if !strings.HasPrefix(url, allowedURLPrefix) {
		return fmt.Errorf("%w: %q", errURLNoPermitida, url)
	}
	if strings.Contains(url, "..") || strings.ContainsAny(url, " \t\n\"'`$") {
		return fmt.Errorf("%w: %q", errURLNoPermitida, url)
	}
	return nil
}
