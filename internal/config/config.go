// Package config persiste las preferencias de la app (último juego,
// héroe, rol y pestaña, modo de retrato, orden de la lista y la pista
// inicial) en un JSON pequeño dentro de os.UserConfigDir().
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config es el estado que sobrevive entre ejecuciones.
type Config struct {
	LastGame     string `json:"last_game"`     // "lol" | "deadlock"
	LastChampion string `json:"last_champion"` // clave ("Ahri", "Abrams")
	LastRole     string `json:"last_role"`     // rol activo en el detalle
	LastTab      int    `json:"last_tab"`      // pestaña activa
	ArtMode      int    `json:"art_mode"`      // retrato: mini/ascii/mono
	SortMode     int    `json:"sort_mode"`     // lista: WR/partidas/nombre
	SeenHint     bool   `json:"seen_hint"`     // ya vio la pista inicial
	MonitorOff   bool   `json:"monitor_off"`   // detector de juego desactivado
	// UpdateCheckOff desactiva la comprobación de nuevas versiones en
	// GitHub. Por defecto está activa (false).
	UpdateCheckOff bool `json:"update_check_off"`
}

// path devuelve la ruta del archivo de configuración. BUILDS_CONFIG
// la sustituye (tests y -config en el futuro).
func path() (string, error) {
	if p := os.Getenv("BUILDS_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mobatui", "config.json"), nil
}

// Load lee la configuración; si no existe o está corrupta devuelve
// los ceros sin fallar: las preferencias son optativas.
func Load() Config {
	p, err := path()
	if err != nil {
		return Config{}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Config{}
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}
	}
	return c
}

// Save escribe la configuración de forma atómica (tmp + rename).
func (c Config) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
