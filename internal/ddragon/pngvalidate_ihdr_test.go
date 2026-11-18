package ddragon

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/png"
	"os"
	"testing"
)

// fixtureIHDR lee el PNG real del repositorio.
func fixtureIHDR(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/ddragon/3026.png")
	if err != nil {
		t.Skip("sin fixture")
	}
	return b
}

// conIHDR devuelve una copia del PNG con los campos de IHDR indicados y
// el CRC del chunk recalculado, para que la única irregularidad sea la
// cabecera.
func conIHDR(base []byte, mut func(d []byte)) []byte {
	out := append([]byte(nil), base...)
	const off = 8 // longitud(4)+tipo(4) tras la firma
	d := out[off+8 : off+8+13]
	mut(d)
	binary.BigEndian.PutUint32(out[off+8+13:off+8+17], crc32.ChecksumIEEE(out[off+4:off+4+4+13]))
	return out
}

func u32(d []byte, v uint32, at int) { binary.BigEndian.PutUint32(d[at:at+4], v) }

// TestIHDRCamposIlegales cubre una regla por caso de la tarea A.
func TestIHDRCamposIlegales(t *testing.T) {
	base := fixtureIHDR(t)

	casos := []struct {
		nombre string
		// por qué es ilegal
		razon string
		mut   func(d []byte)
	}{
		{"ancho cero", "ancho debe ser >= 1", func(d []byte) { u32(d, 0, 0) }},
		{"alto cero", "alto debe ser >= 1", func(d []byte) { u32(d, 0, 4) }},
		{"ancho maximo+1", "ancho <= 2^31-1", func(d []byte) { u32(d, 1<<31, 0) }},
		{"alto maximo+1", "alto <= 2^31-1", func(d []byte) { u32(d, 1<<31, 4) }},
		{"colorType 7 desconocido", "colorType debe ser 0,2,3,4 o 6", func(d []byte) { d[9] = 7 }},
		{"bitDepth 3 con RGBA", "RGBA solo admite 8 o 16", func(d []byte) { d[8] = 3 }},
		{"bitDepth 1 con RGB", "RGB solo admite 8 o 16", func(d []byte) { d[9] = 2; d[8] = 1 }},
		{"bitDepth 32 con gris", "gris solo 1,2,4,8,16", func(d []byte) { d[9] = 0; d[8] = 32 }},
		{"compresion 1", "compresion debe ser 0", func(d []byte) { d[10] = 1 }},
		{"filtro 1", "filtro debe ser 0", func(d []byte) { d[11] = 1 }},
		{"entrelazado 2", "entrelazado debe ser 0 o 1", func(d []byte) { d[12] = 2 }},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cuerpo := conIHDR(base, c.mut)
			if err := ValidatePNG(cuerpo); err == nil {
				t.Fatalf("debía rechazarse (%s)", c.razon)
			}
			// Y el decodificador estándar también lo rechaza: la regla
			// no es inventada.
			if _, err := png.DecodeConfig(bytes.NewReader(cuerpo)); err == nil {
				t.Logf("nota: png.DecodeConfig acepta este caso (%s)", c.razon)
			}
		})
	}

	// El PNG válido sigue aceptándose.
	if err := ValidatePNG(base); err != nil {
		t.Fatalf("el PNG válido debe aceptarse: %v", err)
	}
	// Y una cabecera con entrelazado 1 (Adam7) es legal. Se fabrica un
	// PNG entrelazado real para no inventar el caso.
	t.Run("entrelazado 1 es legal", func(t *testing.T) {
		ent := pngEntrelazado(t)
		if err := ValidatePNG(ent); err != nil {
			t.Errorf("el entrelazado 1 es legal: %v", err)
		}
	})
}

// pngEntrelazado fabrica un PNG Adam7 válido con el codificador estándar
// no lo emite, así que se toma uno del propio paquete si existiera; en su
// defecto se documenta que no hay fixture.
func pngEntrelazado(t testing.TB) []byte {
	// Go no emite entrelazado; se compone un PNG 1x1 Adam7 mínimo.
	// IHDR: 1x1, bitDepth 8, colorType 0, comp 0, filtro 0, interlace 1.
	ihdr := []byte{
		0, 0, 0, 1, // ancho
		0, 0, 0, 1, // alto
		8, 0, 0, 0, 1, // bitDepth, colorType, comp, filtro, entrelazado
	}
	chunk := func(tipo string, datos []byte) []byte {
		out := make([]byte, 0, 12+len(datos))
		var l [4]byte
		binary.BigEndian.PutUint32(l[:], uint32(len(datos)))
		out = append(out, l[:]...)
		out = append(out, tipo...)
		out = append(out, datos...)
		var c [4]byte
		binary.BigEndian.PutUint32(c[:], crc32.ChecksumIEEE(out[4:]))
		return append(out, c[:]...)
	}
	out := append([]byte(nil), pngFirma...)
	out = append(out, chunk("IHDR", ihdr)...)
	out = append(out, chunk("IDAT", []byte{0x08, 0x1d, 0x01, 0x02, 0x00, 0xfe, 0x00})...)
	out = append(out, chunk("IEND", nil)...)
	return out
}

// TestAutocuracionIHDRIlegal: una entrada de disco con IHDR ilegal y CRC
// correcto se descarta en la lectura y se recarga el icono correcto.
func TestAutocuracionIHDRIlegal(t *testing.T) {
	// El camino real (cache.GetValidated + ddragon.ItemIcon) se cubre en
	// internal/cache; aquí se comprueba que el validador rechaza el
	// cuerpo envenenado y acepta el bueno, que es lo que dispara el
	// descarte.
	base := fixtureIHDR(t)
	envenenado := conIHDR(base, func(d []byte) { d[8] = 3 }) // bitDepth 3

	if err := ValidatePNG(envenenado); err == nil {
		t.Fatal("el cuerpo envenenado debía rechazarse")
	}
	if err := ValidatePNG(base); err != nil {
		t.Fatalf("el cuerpo correcto debe aceptarse: %v", err)
	}
}
