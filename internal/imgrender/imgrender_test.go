package imgrender

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// gradientePNG genera un PNG cuadrado con gradiente y esquinas
// transparentes (como los iconos de los juegos).
func gradientePNG(t *testing.T, size int) image.Image {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := size / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-c), float64(y-c)
			d := (dx*dx + dy*dy) / float64(c*c)
			a := uint8(255)
			if d > 1 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(255 * x / size), G: uint8(255 * y / size), B: 128, A: a,
			})
		}
	}
	return img
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestRenderDimensiones(t *testing.T) {
	out := Render(gradientePNG(t, 96), 24, 12, color.RGBA{10, 20, 40, 255})
	lines := strings.Split(out, "\n")
	if len(lines) != 12 {
		t.Fatalf("líneas = %d, se esperaban 12", len(lines))
	}
	for i, l := range lines {
		if n := strings.Count(l, "▀"); n != 24 {
			t.Errorf("línea %d tiene %d celdas, se esperaban 24", i, n)
		}
	}
	if !strings.Contains(out, "\x1b[38;2;") || !strings.Contains(out, ";48;2;") {
		t.Error("faltan secuencias truecolor de fg/bg")
	}
}

func TestRenderAspect(t *testing.T) {
	// Icono cuadrado a 32 columnas ⇒ 16 filas.
	out := RenderAspect(gradientePNG(t, 120), 32, color.RGBA{10, 20, 40, 255})
	if lines := strings.Count(out, "\n") + 1; lines != 16 {
		t.Errorf("filas = %d, se esperaban 16", lines)
	}
}

func TestRenderFondoTransparente(t *testing.T) {
	// Toda la transparencia debe fundirse con el bg: el color del
	// panel aparece en las secuencias ANSI.
	bg := color.RGBA{10, 20, 40, 255}
	out := Render(gradientePNG(t, 96), 12, 6, bg)
	if !strings.Contains(out, "38;2;10;20;40") && !strings.Contains(out, "48;2;10;20;40") {
		t.Error("el fondo del panel no aparece en los píxeles transparentes")
	}
}

func TestAutoContrast(t *testing.T) {
	// Gradiente apagado 30..60: el autocontraste debe estirarlo al
	// rango completo (el píxel central, ~45, debe aclararse bastante).
	dim := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			v := uint8(30 + 3*x)
			dim.Set(x, y, color.RGBA{v, v, v, 255})
		}
	}
	out := AutoContrast(dim)
	r, _, _, _ := out.At(5, 5).RGBA()
	if r>>8 <= 45 {
		t.Errorf("autocontraste no aclaró el centro: canal R = %d", r>>8)
	}
	// Los extremos se recortan: el mínimo debe quedar cerca de 0.
	rMin, _, _, _ := out.At(0, 0).RGBA()
	if rMin>>8 > 30 {
		t.Errorf("el extremo inferior no se estiró: R = %d", rMin>>8)
	}
}

func TestCrop(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	c := Crop(img, 0.8)
	b := c.Bounds()
	if b.Dx() != 80 || b.Dy() != 80 {
		t.Errorf("recorte = %dx%d, se esperaba 80x80", b.Dx(), b.Dy())
	}
	if same := Crop(img, 1); same != image.Image(img) {
		t.Error("frac=1 debía devolver la imagen original")
	}
}

func TestRenderPNGBytesRedondo(t *testing.T) {
	// Flujo completo usado por la UI: bytes PNG → decode → render.
	raw := pngBytes(t, gradientePNG(t, 64))
	out := RenderAspect(decodePNG(t, raw), 12, color.RGBA{10, 20, 40, 255})
	if !strings.Contains(out, "▀") {
		t.Error("render vacío")
	}
}
