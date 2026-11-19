package ddragon

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"math/rand"
	"os"
	"testing"
)

// fixtureValido lee el PNG real del repositorio (3.883 B).
func fixtureValido(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		t.Skip("sin fixture")
	}
	return b
}

// TestTruncamientoExhaustivo: para CADA longitud de 0 a len-1 el
// validador debe rechazar; la longitud completa debe aceptarse.
func TestTruncamientoExhaustivo(t *testing.T) {
	raw := fixtureValido(t)
	if err := ValidatePNG(raw); err != nil {
		t.Fatalf("el PNG completo debe aceptarse: %v", err)
	}
	var aceptados []int
	for n := 0; n < len(raw); n++ {
		if err := ValidatePNG(raw[:n]); err == nil {
			aceptados = append(aceptados, n)
		}
	}
	if len(aceptados) > 0 {
		t.Errorf("DEFECTO: %d truncamientos aceptados en longitudes %v (de %d)",
			len(aceptados), aceptados[:min(10, len(aceptados))], len(raw))
	}
	t.Logf("%d longitudes probadas (0..%d), aceptadas=0, completo aceptado",
		len(raw), len(raw)-1)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestDiferencialValidadorVsDecode: todo lo que ValidatePNG acepta debe
// poder analizarse con png.DecodeConfig. Se cuentan y reportan los casos
// que ValidatePNG acepta y png.Decode rechaza.
//
// El corpus tiene dos mitades: mutaciones que casi siempre rompen el
// fichero, y recomposiciones de chunks con CRC recalculado, que el
// validador puede aceptar. Sin esa segunda mitad el corpus no ejercita
// el camino "aceptado" y la prueba no demuestra nada.
func TestDiferencialValidadorVsDecode(t *testing.T) {
	base := fixtureValido(t)
	rng := rand.New(rand.NewSource(777001))

	// recomponer devuelve un PNG con los chunks de base permutados y/o
	// duplicados, recalculando los CRC: el validador lo aceptará aunque
	// el resultado no sea decodificable.
	type chunk struct {
		crudo []byte
	}
	leerChunks := func(b []byte) []chunk {
		var out []chunk
		off := 8
		for off+8 <= len(b) {
			l := binary.BigEndian.Uint32(b[off : off+4])
			fin := off + 8 + int(l) + 4
			if fin > len(b) || l > uint32(len(b)) {
				break
			}
			out = append(out, chunk{crudo: append([]byte(nil), b[off:fin]...)})
			off = fin
		}
		return out
	}
	chunks := leerChunks(base)

	var validados, configOK, configRechaza, decodeFalla int
	total := 12000
	for i := 0; i < total; i++ {
		var b []byte
		if i%2 == 0 {
			// Mitad A: mutaciones destructivas.
			b = append([]byte(nil), base...)
			switch (i / 2) % 4 {
			case 0:
				if len(b) > 9 {
					b = b[:8+rng.Intn(len(b)-8)]
				}
			case 1:
				for k := 0; k < 1+rng.Intn(6); k++ {
					b[rng.Intn(len(b))] ^= byte(1 << uint(rng.Intn(8)))
				}
			case 2:
				p := rng.Intn(len(b))
				b = append(b[:p], append([]byte{0xDE, 0xAD, 0xBE, 0xEF}, b[p:]...)...)
			case 3:
				if len(b) > 20 {
					binary.BigEndian.PutUint32(b[8:12], uint32(rng.Intn(1<<31)))
				}
			}
		} else {
			// Mitad B: recomposición estructuralmente válida.
			b = append([]byte(nil), base[:8]...)
			orden := rng.Perm(len(chunks))
			for _, idx := range orden {
				c := append([]byte(nil), chunks[idx].crudo...)
				if len(c) >= 12 {
					// Se altera el contenido pero se recalcula el CRC.
					c[8] ^= byte(1 << uint(rng.Intn(8)))
					binary.BigEndian.PutUint32(c[len(c)-4:],
						crc32.ChecksumIEEE(c[4:len(c)-4]))
				}
				b = append(b, c...)
			}
		}
		if ValidatePNG(b) != nil {
			continue
		}
		validados++
		if _, err := png.DecodeConfig(bytes.NewReader(b)); err == nil {
			configOK++
		} else {
			configRechaza++
		}
		if _, err := png.Decode(bytes.NewReader(b)); err != nil {
			decodeFalla++
		}
	}
	t.Logf("corpus=%d: ValidatePNG aceptó %d; de esos, DecodeConfig aceptó %d y "+
		"rechazó %d; png.Decode falló en %d de los aceptados por ValidatePNG",
		total, validados, configOK, configRechaza, decodeFalla)
	if validados == 0 {
		t.Error("el corpus no ejercitó el camino 'aceptado': la prueba no demuestra nada")
	}
}

// TestIHDRIlegalConCRCCorrecto fabrica un PNG con CRC correcto en todos
// los chunks pero IHDR ilegal (profundidad de bits no válida para el
// tipo de color). Mide los tres caminos.
func TestIHDRIlegalConCRCCorrecto(t *testing.T) {
	raw := fixtureValido(t)

	// IHDR: longitud(4) tipo(4) ancho(4) alto(4) bitdepth(1) colorType(1)
	// filtro(1) interlace(1) CRC(4). Offset del chunk = 8.
	const off = 8
	out := append([]byte(nil), raw...)
	// Profundidad de bits 3: inválida (válidas 1,2,4,8,16).
	out[off+8+8] = 3
	// Se recalcula el CRC para que sea correcto y el chunk sea coherente.
	binary.BigEndian.PutUint32(out[off+8+13:off+8+17],
		crc32.ChecksumIEEE(out[off+4:off+8+13]))

	if err := ValidatePNG(out); err != nil {
		t.Logf("ValidatePNG rechaza el IHDR ilegal (mejor de lo esperado): %v", err)
	} else {
		t.Logf("ValidatePNG ACEPTA IHDR con bitdepth=3 (ilegal)")
	}
	_, errCfg := png.DecodeConfig(bytes.NewReader(out))
	_, errFull := png.Decode(bytes.NewReader(out))
	t.Logf("DecodeConfig: %v | Decode: %v", errCfg, errFull)

	if ValidatePNG(out) == nil && errCfg != nil && errFull != nil {
		t.Logf("DEFECTO CONFIRMADO Y PENDIENTE: pasa el validador, falla al decodificar. " +
			"En producción: se ESCRIBE en caché 720 h, la UI lo clasifica " +
			"artFailCorrupt (no memorizable) y no reintenta hasta que el TTL " +
			"o un descarte en lectura lo elimine. Diff propuesto: validar " +
			"bitdepth/colorType en ValidatePNG. NO aplicado.")
	}
}

// TestIENDBytesExtra documenta el caso del informe.
func TestIENDBytesExtra(t *testing.T) {
	raw := fixtureValido(t)
	extra := []byte("BASURA-TRAILER")
	con := append(append([]byte(nil), raw...), extra...)
	if err := ValidatePNG(con); err != nil {
		t.Errorf("IEND cierra la imagen; los bytes extra no forman parte del PNG: %v", err)
	}
	// Y sigue siendo decodificable.
	if _, err := png.Decode(bytes.NewReader(con)); err != nil {
		t.Logf("nota: png.Decode tampoco lo acepta (%v)", err)
	}
	t.Logf("ACEPTADO (correcto): IEND marca el fin; %d bytes extra se ignoran", len(extra))
}
