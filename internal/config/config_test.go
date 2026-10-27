package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("BUILDS_CONFIG", p)
	if c := Load(); c.LastGame != "" || c.SeenHint {
		t.Fatalf("config inexistente debía dar ceros: %+v", c)
	}
	want := Config{
		LastGame: "deadlock", LastChampion: "Abrams", LastRole: "ALL",
		LastTab: 1, ArtMode: 2, SortMode: 1, SeenHint: true,
	}
	if err := want.Save(); err != nil {
		t.Fatal(err)
	}
	got := Load()
	if got != want {
		t.Fatalf("roundtrip:\n got %+v\nwant %+v", got, want)
	}
}

func TestCorruptaDaCeros(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	t.Setenv("BUILDS_CONFIG", p)
	if err := os.WriteFile(p, []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := Load(); c.LastGame != "" {
		t.Fatalf("config corrupta debía dar ceros: %+v", c)
	}
}
