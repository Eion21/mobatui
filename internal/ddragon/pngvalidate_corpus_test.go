package ddragon

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"math/rand"
	"os"
	"strings"
	"testing"
)

// TestClasificacionCorpusDiferencial repite el corpus de 12.000 casos y
// clasifica por causa los que ValidatePNG acepta y png.Decode rechaza.
func TestClasificacionCorpusDiferencial(t *testing.T) {
	base, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		t.Skip("sin fixture")
	}
	rng := rand.New(rand.NewSource(777001))

	leerChunks := func(b []byte) [][]byte {
		var out [][]byte
		off := 8
		for off+8 <= len(b) {
			l := binary.BigEndian.Uint32(b[off : off+4])
			fin := off + 8 + int(l) + 4
			if fin > len(b) || l > uint32(len(b)) {
				break
			}
			out = append(out, append([]byte(nil), b[off:fin]...))
			off = fin
		}
		return out
	}
	chunks := leerChunks(base)

	var aceptados, decodeFalla int
	causas := map[string]int{}
	total := 12000
	for i := 0; i < total; i++ {
		var b []byte
		if i%2 == 0 {
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
			b = append([]byte(nil), base[:8]...)
			for _, idx := range rng.Perm(len(chunks)) {
				c := append([]byte(nil), chunks[idx]...)
				if len(c) >= 12 {
					c[8] ^= byte(1 << uint(rng.Intn(8)))
					binary.BigEndian.PutUint32(c[len(c)-4:], crc32.ChecksumIEEE(c[4:len(c)-4]))
				}
				b = append(b, c...)
			}
		}
		if ValidatePNG(b) != nil {
			continue
		}
		aceptados++
		if _, derr := png.Decode(bytes.NewReader(b)); derr != nil {
			decodeFalla++
			causas[clasifica(b, chunks, derr)]++
		}
	}
	t.Logf("corpus=%d: ValidatePNG aceptó %d; de esos, png.Decode falló en %d",
		total, aceptados, decodeFalla)
	for k, v := range causas {
		t.Logf("  causa %-42s %5d casos", k, v)
	}
}

// clasifica da un nombre a la causa del fallo de png.Decode.
func clasifica(b []byte, originales [][]byte, err error) string {
	msg := err.Error()
	// IHDR localizado por escaneo, no por offset fijo: en la mitad
	// recompuesta los chunks van en otro orden.
	buscaIHDR := func(d []byte) []byte {
		off := 8
		for off+8 <= len(d) {
			l := binary.BigEndian.Uint32(d[off : off+4])
			fin := off + 8 + int(l) + 4
			if fin > len(d) || l > uint32(len(d)) {
				return nil
			}
			if string(d[off+4:off+8]) == "IHDR" {
				return d[off+8 : fin-4]
			}
			off = fin
		}
		return nil
	}
	ih, ihOrig := buscaIHDR(b), buscaIHDR(originales[0])
	if ih != nil && ihOrig != nil && !bytes.Equal(ih, ihOrig) {
		return "IHDR alterado (semántica)"
	}
	// ¿Faltan, sobran o están desordenados los chunks?
	n := 0
	off := 8
	for off+8 <= len(b) {
		l := binary.BigEndian.Uint32(b[off : off+4])
		off += 8 + int(l) + 4
		n++
	}
	switch {
	case strings.Contains(msg, "out of order"):
		return "chunks desordenados"
	case strings.Contains(msg, "too much pixel data"):
		return "IDAT sobrante o duplicado"
	case strings.Contains(msg, "not enough pixel data"):
		return "IDAT insuficiente o truncado"
	case strings.Contains(msg, "invalid checksum"):
		return "IDAT con CRC recalculado sobre datos corruptos"
	case strings.Contains(msg, "unsupported feature"):
		return "campo no soportado"
	}
	return "otra: " + msg
}
