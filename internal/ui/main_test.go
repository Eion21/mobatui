package ui

import (
	"os"
	"testing"
)

// TestMain aísla las preferencias persistentes de los tests: ningún
// test debe leer ni escribir la configuración real del usuario.
func TestMain(m *testing.M) {
	f, err := os.CreateTemp("", "builds-test-config-*.json")
	if err != nil {
		os.Exit(1)
	}
	os.Setenv("BUILDS_CONFIG", f.Name())
	os.Exit(m.Run())
}
