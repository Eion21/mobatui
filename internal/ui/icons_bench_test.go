package ui

import (
	"testing"

	"github.com/Eion21/mobatui/internal/model"
	"github.com/Eion21/mobatui/internal/staticdata"
)

type benchProvider struct{ staticdata.Provider }

func (benchProvider) ItemIcon(int) ([]byte, error) { return nil, nil }

func sesionBench(b *testing.B) *Session {
	return &Session{
		Game:        model.GameLoL,
		ItemArt:     map[artKey]string{},
		RuneArt:     map[artKey]string{},
		ChampionArt: map[string]champArt{},
		Version:     "16.21",
		Provider:    benchProvider{},
	}
}

// BenchmarkReserveArt mide reservar el MISMO recurso desde un ancho nuevo
// (el caso de la deduplicación): contador, pendingKeys y resourceFor.
func BenchmarkReserveArt(b *testing.B) {
	s := sesionBench(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := artKey{Kind: artItem, ID: 3026, Cols: 3 + (i % 3), Mode: artHalfBlock, Patch: "16.21"}
		s.reserveArt(k)
		s.releaseArt(k)
	}
}

// BenchmarkPendingKeysFor mide localizar todas las claves que esperan un
// recurso: es lo que permite que una descarga alimente varios anchos.
func BenchmarkPendingKeysFor(b *testing.B) {
	s := sesionBench(b)
	// Se simulan 3 anchos esperando el mismo recurso.
	for c := 0; c < 3; c++ {
		s.runePending = nil
		s.pendingKeys = nil
		k := artKey{Kind: artItem, ID: 3026, Cols: 3 + c, Mode: artHalfBlock, Patch: "16.21"}
		s.reserveArt(k)
	}
	res := s.resourceForLocked(artKey{Kind: artItem, ID: 3026, Cols: 3, Mode: artHalfBlock, Patch: "16.21"})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.pendingKeysFor(res)
	}
}

// BenchmarkItemArtCmdFor mide la construcción del Cmd (reservas
// incluidas) con 10 ids.
func BenchmarkItemArtCmdFor(b *testing.B) {
	s := sesionBench(b)
	ids := make([]int, 10)
	for i := range ids {
		ids[i] = 3000 + i
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = itemArtCmdFor(s, ids, smallArtCols)
		b.StopTimer()
		s.InvalidateArt()
		b.StartTimer()
	}
}
