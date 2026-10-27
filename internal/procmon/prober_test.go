package procmon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProberLoLLeeCampeon(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/liveclientdata/activeplayername" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, `{"activePlayerName":"Ahri"}`)
	}))
	defer srv.Close()

	p := lolProberAt(srv.URL)
	if got := p(LOL); got != "Ahri" {
		t.Errorf("prober = %q, se esperaba %q", got, "Ahri")
	}
	// Solo LoL se sondea: los otros juegos no tienen API local.
	if got := p(DEADLOCK); got != "" {
		t.Errorf("prober de Deadlock = %q, se esperaba vacío", got)
	}
}

func TestProberSinPartida(t *testing.T) {
	// Puerto cerrado: sin partida, el prober devuelve vacío sin colgarse.
	p := lolProberAt("http://127.0.0.1:1")
	if got := p(LOL); got != "" {
		t.Errorf("prober sin partida = %q, se esperaba vacío", got)
	}
}

func TestProberRespuestaMala(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := lolProberAt(srv.URL)
	if got := p(LOL); got != "" {
		t.Errorf("prober con error 500 = %q, se esperaba vacío", got)
	}
}
