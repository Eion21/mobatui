// Prober real del personaje: LoL expone la Live Client Data API en
// https://127.0.0.1:2999 mientras hay una partida en curso; fuera de
// partida el puerto está cerrado y la consulta falla al instante.
// Deadlock no ofrece API local del héroe activo: no hay detección.
package procmon

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"
)

const lolLiveURL = "https://127.0.0.1:2999"

// NewCharacterProber devuelve el prober real por juego. Para LoL
// consulta /liveclientdata/activeplayername, que responde el nombre
// interno del campeón (p. ej. "MonkeyKing") solo durante una partida.
func NewCharacterProber() CharacterProber {
	return lolProberAt(lolLiveURL)
}

func lolProberAt(base string) CharacterProber {
	client := &http.Client{
		Timeout: 2 * time.Second,
		// La partida sirve HTTPS con un certificado propio de Riot;
		// como el puerto solo escucha en loopback, el riesgo es nulo y
		// la validación se salta.
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	return func(g Game) string {
		if g != LOL {
			return ""
		}
		resp, err := client.Get(base + "/liveclientdata/activeplayername")
		if err != nil {
			return "" // sin partida: el puerto está cerrado
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return ""
		}
		var payload struct {
			ActivePlayerName string `json:"activePlayerName"`
		}
		if json.NewDecoder(resp.Body).Decode(&payload) != nil {
			return ""
		}
		return payload.ActivePlayerName
	}
}
