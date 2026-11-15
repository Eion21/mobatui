package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Eion21/mobatui/internal/cache"
	"github.com/Eion21/mobatui/internal/deadlock"
	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/ui"
)

// The API can return fewer usable runs than the requested maximum, and an
// individual item's N is not the size of the sample used by BuildRoute.
func TestDeadlockRouteProvenanceFromRunsToView(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/assets/heroes", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":6,"name":"Abrams","player_selectable":true}]`)
	})
	mux.HandleFunc("/v1/assets/items", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"id":101,"name":"First","type":"upgrade"},{"id":102,"name":"Second","type":"upgrade"},{"id":103,"name":"Rare","type":"upgrade"}]`)
	})
	mux.HandleFunc("/v1/matches/metadata", func(w http.ResponseWriter, r *http.Request) {
		matches := make([]map[string]any, 137)
		for i := range matches {
			items := []map[string]int{{"item_id": 101, "game_time_s": 300}}
			if i < 42 {
				items = append(items, map[string]int{"item_id": 102, "game_time_s": 700})
			}
			if i < 41 {
				items = append(items, map[string]int{"item_id": 103, "game_time_s": 900})
			}
			matches[i] = map[string]any{"players": []any{map[string]any{"hero_id": 6, "items": items}}}
		}
		_ = json.NewEncoder(w).Encode(matches)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, err := cache.New(filepath.Join(t.TempDir(), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	api := deadlock.NewAPI(c, "testdata/deadlock").WithAPIBase(srv.URL).WithHTTPClient(srv.Client())
	steps, err := (deadlockRuns{api: api}).LoadRuns("Abrams")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 2 || steps[0].Item != 101 || steps[1].Item != 102 || steps[1].N != 42 {
		t.Fatalf("30%% route and order changed: %+v", steps)
	}
	b := &model.CleanBuild{CoreItems: []model.RankedItem{
		{ItemStats: model.ItemStats{ID: 101, Stats: model.Stats{Games: 20000}, AvgBuyS: 300}, Wilson: .52},
		{ItemStats: model.ItemStats{ID: 102, Stats: model.Stats{Games: 20000}, AvgBuyS: 700}, Wilson: .52},
	}}
	champ := &ui.ChampEntry{Key: "Abrams", Name: "Abrams", Roles: []model.Role{model.RoleAll}, ByRole: map[model.Role]*model.CleanBuild{model.RoleAll: b}}
	session := &ui.Session{Game: model.GameDeadlock, Routes: map[string][]ui.RouteStep{"Abrams": steps}, Names: ui.Names{Items: map[int]string{101: "First", 102: "Second"}}}
	text := regexp.MustCompile(`\s+`).ReplaceAllString(regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`).ReplaceAllString(ui.RenderDetail(champ, session), ""), " ")
	if !strings.Contains(text, "137") || !(strings.Contains(text, "30%") || strings.Contains(text, "30.0%") || strings.Contains(text, "30 %")) {
		t.Errorf("actual run count and threshold not rendered: %s", text)
	}
	if strings.Contains(text, "250 partidas") {
		t.Error("requested maximum displayed as actual sample")
	}
}
