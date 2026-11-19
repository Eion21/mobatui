package ddragon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
)

// ErrNotPNG indica que un cuerpo no es un PNG íntegro.
var ErrNotPNG = errors.New("ddragon: cuerpo no es un PNG íntegro")

// pngFirma son los 8 bytes iniciales de cualquier PNG.
var pngFirma = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// ValidatePNG comprueba que body es un PNG completo y no truncado.
//
// POR QUÉ NO SE USA png.DecodeConfig: solo lee la cabecera IHDR. Se midió
// que un PNG real de 64x64 truncado a la mitad devuelve err=nil, es decir,
// lo aceptaría como válido; el caso reportado (14 bytes) sí lo detecta por
// suerte (falla al leer la cabecera), pero un truncamiento a mitad de los
// datos de píxel pasaría. No sirve como validador.
//
// POR QUÉ NO png.Decode COMPLETO: detecta cualquier truncamiento, pero
// cuesta 55.821 ns/op y 62.765 B/op por icono, y además paga memoria para
// los píxeles que después se desechan.
//
// Esta comprobación recorre los chunks validando el CRC de cada uno,
// valida los CAMPOS de la cabecera IHDR y exige que aparezca IEND.
// Detecta truncamiento en cualquier punto (un chunk cortado, o la
// ausencia de IEND), cuerpos corruptos (CRC que no cuadra) y cabeceras
// sintácticamente correctas pero imposibles de decodificar.
//
// POR QUÉ SE VALIDA IHDR: el CRC solo prueba integridad, no legalidad.
// Se midió que un IHDR con todos los CRC correctos y bitDepth=3 lo
// aceptaba el validador y lo rechazaban tanto png.DecodeConfig como
// png.Decode: el cuerpo se escribía en caché con TTL de 720 h y ningún
// reintento podía recuperarlo.
func ValidatePNG(body []byte) error {
	if len(body) < len(pngFirma) {
		return ErrNotPNG
	}
	if !bytes.Equal(body[:len(pngFirma)], pngFirma) {
		return ErrNotPNG
	}
	// La cabecera IHDR es obligatoria, la primera y de longitud fija.
	if err := validarIHDR(body); err != nil {
		return err
	}
	i := len(pngFirma)
	for i+8 <= len(body) {
		long := binary.BigEndian.Uint32(body[i : i+4])
		tipo := body[i+4 : i+8]
		fin := i + 8 + int(long) + 4
		if long > uint32(len(body)) || fin > len(body) {
			return ErrNotPNG // truncado dentro de un chunk
		}
		esperado := binary.BigEndian.Uint32(body[fin-4 : fin])
		if crc32.ChecksumIEEE(body[i+4:fin-4]) != esperado {
			return ErrNotPNG // chunk corrupto
		}
		if string(tipo) == "IEND" {
			// IEND cierra la imagen: sin él, los datos están incompletos.
			return nil
		}
		i = fin
	}
	return ErrNotPNG // se acabó el cuerpo sin IEND
}

// validarIHDR comprueba los campos de la cabecera IHDR (13 bytes).
//
// Tabla colorType → bitDepth permitidos, de la especificación PNG:
//
//	0 gris          1, 2, 4, 8, 16
//	2 RGB           8, 16
//	3 indexado      1, 2, 4, 8
//	4 gris + alfa   8, 16
//	6 RGBA          8, 16
func validarIHDR(body []byte) error {
	const (
		offIHDR  = 8  // tras la firma
		lonIHDR  = 13 // ancho+alto+campos
		tipoIHDR = "IHDR"
	)
	// firma(8) + longitud(4) + tipo(4) + datos(13) + crc(4)
	if len(body) < offIHDR+8+lonIHDR+4 {
		return ErrNotPNG
	}
	if binary.BigEndian.Uint32(body[offIHDR:offIHDR+4]) != lonIHDR {
		return ErrNotPNG
	}
	if string(body[offIHDR+4:offIHDR+8]) != tipoIHDR {
		return ErrNotPNG
	}
	d := body[offIHDR+8 : offIHDR+8+lonIHDR]

	// ancho y alto: 1 .. 2^31-1 (cero es ilegal; el tope es el límite
	// que fija la especificación).
	const maxDim = 1 << 31
	if w := binary.BigEndian.Uint32(d[0:4]); w == 0 || w > maxDim-1 {
		return ErrNotPNG
	}
	if h := binary.BigEndian.Uint32(d[4:8]); h == 0 || h > maxDim-1 {
		return ErrNotPNG
	}

	bitDepth, colorType := d[8], d[9]
	if !bitDepthValida(colorType, bitDepth) {
		return ErrNotPNG
	}
	// Método de compresión y de filtro: 0 es el único definido.
	if d[10] != 0 || d[11] != 0 {
		return ErrNotPNG
	}
	// Entrelazado: 0 (ninguno) o 1 (Adam7).
	if d[12] > 1 {
		return ErrNotPNG
	}
	return nil
}

// bitDepthValida aplica la tabla de la especificación PNG.
func bitDepthValida(colorType, bitDepth byte) bool {
	var validas []byte
	switch colorType {
	case 0: // escala de grises
		validas = []byte{1, 2, 4, 8, 16}
	case 2: // RGB
		validas = []byte{8, 16}
	case 3: // indexado
		validas = []byte{1, 2, 4, 8}
	case 4: // gris + alfa
		validas = []byte{8, 16}
	case 6: // RGBA
		validas = []byte{8, 16}
	default:
		return false // colorType desconocido
	}
	for _, v := range validas {
		if v == bitDepth {
			return true
		}
	}
	return false
}
