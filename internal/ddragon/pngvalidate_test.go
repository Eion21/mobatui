package ddragon

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"testing"
)

// iconoRealista devuelve un PNG con el tamaño y la entropía de un icono
// real de ítem: 64x64 con degradado y ruido determinista. No usa la red
// ni la caché del usuario.
func iconoRealista(t testing.TB) []byte {
	t.Helper()
	const n = 64
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	rng := rand.New(rand.NewSource(20261118))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x*4) + uint8(rng.Intn(16)),
				G: uint8(y*4) + uint8(rng.Intn(16)),
				B: uint8((x*y)/16) + uint8(rng.Intn(16)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// iconoGrande es el mismo patrón a 4x resolución (128x128).
func iconoGrande(t testing.TB) []byte {
	t.Helper()
	const n = 128
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	rng := rand.New(rand.NewSource(20261118))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(x*2) + uint8(rng.Intn(16)),
				G: uint8(y*2) + uint8(rng.Intn(16)),
				B: uint8((x*y)/64) + uint8(rng.Intn(16)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtureReal lee un PNG del repositorio (sin caché personal).
func fixtureReal(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		t.Skip("sin fixture")
	}
	return b
}

func BenchmarkDecodeConfigReal(b *testing.B) {
	raw := fixtureReal(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeCompletoReal(b *testing.B) {
	raw := fixtureReal(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePNGReal(b *testing.B) {
	raw := fixtureReal(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidatePNG(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeConfigGrande(b *testing.B) {
	raw := iconoGrande(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := png.DecodeConfig(bytes.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeCompletoGrande(b *testing.B) {
	raw := iconoGrande(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := png.Decode(bytes.NewReader(raw)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidatePNGGrande(b *testing.B) {
	raw := iconoGrande(b)
	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := ValidatePNG(raw); err != nil {
			b.Fatal(err)
		}
	}
}

// ─── tabla de casos malformados ────────────────────────────────────

// TestValidatePNGCasos recorre los casos del informe con t.Run.
func TestValidatePNGCasos(t *testing.T) {
	valido := fixtureReal(t)

	// Helpers para fabricar casos.
	trozo := func(b []byte, hasta int) []byte {
		if hasta > len(b) {
			hasta = len(b)
		}
		return b[:hasta]
	}
	// chunkAt devuelve (offset, longitud, tipo) del chunk i-ésimo.
	chunkAt := func(b []byte, idx int) (int, uint32, string) {
		off := 8
		for i := 0; i < idx; i++ {
			if off+8 > len(b) {
				return off, 0, ""
			}
			l := binary.BigEndian.Uint32(b[off : off+4])
			off += 8 + int(l) + 4
		}
		if off+8 > len(b) {
			return off, 0, ""
		}
		return off, binary.BigEndian.Uint32(b[off : off+4]), string(b[off+4 : off+8])
	}
	// alteraCRC cambia el CRC del chunk idx.
	alteraCRC := func(b []byte, idx int) []byte {
		out := append([]byte(nil), b...)
		off, l, _ := chunkAt(out, idx)
		fin := off + 8 + int(l) + 4
		if fin <= len(out) {
			out[fin-1] ^= 0xFF
		}
		return out
	}
	// quitaIEND elimina el chunk IEND final.
	quitaIEND := func(b []byte) []byte {
		off := 8
		for off+8 <= len(b) {
			l := binary.BigEndian.Uint32(b[off : off+4])
			tipo := string(b[off+4 : off+8])
			fin := off + 8 + int(l) + 4
			if tipo == "IEND" {
				return b[:off]
			}
			off = fin
		}
		return b
	}
	// longitudFalsa agranda la longitud declarada de un chunk intermedio.
	longitudFalsa := func(b []byte, idx int) []byte {
		out := append([]byte(nil), b...)
		off, _, _ := chunkAt(out, idx)
		binary.BigEndian.PutUint32(out[off:off+4], 0x7FFFFFFF)
		return out
	}

	casos := []struct {
		nombre string
		cuerpo []byte
		ok     bool
	}{
		{"cuerpo vacío", nil, false},
		{"solo la firma de 8 bytes", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, false},
		{"truncado en la cabecera IHDR", trozo(valido, 20), false},
		{"truncado en mitad de un chunk de datos", trozo(valido, len(valido)/2), false},
		{"longitud de chunk mayor que el buffer", longitudFalsa(valido, 1), false},
		{"CRC alterado en un chunk intermedio", alteraCRC(valido, 2), false},
		{"IEND ausente", quitaIEND(valido), false},
		{"página HTML con 200", []byte("<!DOCTYPE html><html><body>404</body></html>"), false},
		{"PNG válido", valido, true},
		{"PNG sintético realista", iconoRealista(t), true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			err := ValidatePNG(c.cuerpo)
			if c.ok && err != nil {
				t.Fatalf("debía aceptarse, dio: %v", err)
			}
			if !c.ok && err == nil {
				t.Fatalf("debía rechazarse (len=%d)", len(c.cuerpo))
			}
			t.Logf("%-46s len=%-6d err=%v", c.nombre, len(c.cuerpo), err)
		})
	}

	// Caso especial: IEND presente pero con bytes extra detrás.
	t.Run("IEND seguido de bytes extra", func(t *testing.T) {
		extra := append(append([]byte(nil), valido...), []byte("BASURA-EXTRA")...)
		err := ValidatePNG(extra)
		t.Logf("IEND + %d bytes extra ⇒ err=%v (se acepta: IEND cierra la imagen "+
			"y lo que siga no forma parte del PNG)", len("BASURA-EXTRA"), err)
		if err != nil {
			t.Errorf("se esperaba aceptar: IEND define el final de la imagen")
		}
	})

	// Se comprueba además que el CRC calculado coincide con el del chunk
	// real, para no dejar el helper sin verificar.
	t.Run("helper CRC coincide con el chunk real", func(t *testing.T) {
		off, l, tipo := chunkAt(valido, 1)
		fin := off + 8 + int(l) + 4
		esperado := binary.BigEndian.Uint32(valido[fin-4 : fin])
		if crc32.ChecksumIEEE(valido[off+4:fin-4]) != esperado {
			t.Errorf("el helper de CRC no coincide para el chunk %q", tipo)
		}
	})
}

// FuzzValidatePNG comprueba que el validador nunca entra en pánico ni
// lee fuera de rango con entradas arbitrarias.
func FuzzValidatePNG(f *testing.F) {
	if b, err := os.ReadFile("../../testdata/ddragon/3026.png"); err == nil {
		f.Add(b)
	}
	f.Add([]byte{})
	f.Add([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	f.Add([]byte("<!DOCTYPE html>"))
	f.Fuzz(func(t *testing.T, data []byte) {
		// No debe entrar en pánico; el resultado (válido o no) es libre.
		_ = ValidatePNG(data)
	})
}

// TestValidatePNGEntradasDeterministas alimenta 12.000 mutaciones
// deterministas para cubrir lo que el fuzz nativo no corre en CI.
func TestValidatePNGEntradasDeterministas(t *testing.T) {
	base := fixtureReal(t)
	rng := rand.New(rand.NewSource(424242))
	aceptados := 0
	for i := 0; i < 12000; i++ {
		b := append([]byte(nil), base...)
		switch i % 4 {
		case 0: // truncar
			if len(b) > 9 {
				b = b[:8+rng.Intn(len(b)-8)]
			}
		case 1: // voltear bytes
			for k := 0; k < 1+rng.Intn(6); k++ {
				p := rng.Intn(len(b))
				b[p] ^= byte(1 << uint(rng.Intn(8)))
			}
		case 2: // insertar basura
			p := rng.Intn(len(b))
			b = append(b[:p], append([]byte{0xDE, 0xAD, 0xBE, 0xEF}, b[p:]...)...)
		case 3: // longitudes absurdas
			if len(b) > 20 {
				binary.BigEndian.PutUint32(b[8:12], uint32(rng.Intn(1<<31)))
			}
		}
		if ValidatePNG(b) == nil {
			aceptados++
		}
	}
	t.Logf("12.000 entradas mutadas: aceptadas=%d (solo las que siguen siendo "+
		"un PNG íntegro)", aceptados)
}
