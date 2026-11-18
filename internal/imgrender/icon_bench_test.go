package imgrender

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"testing"
)

// BenchmarkRenderAspect mide el render de un icono real a los tamaños
// que usa la UI. Es el coste que se paga por cada icono NUEVO.
func BenchmarkRenderAspect(b *testing.B) {
	// Icono real del fixture de Data Dragon.
	path := "../../testdata/ddragon/3070.png"
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Skip("sin icono real")
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		b.Fatalf("decode: %v", err)
	}
	for _, cols := range []int{4, 8, 12, 24} {
		b.Run(string(rune('0'+cols/10))+string(rune('0'+cols%10)), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = RenderAspect(img, cols, color.RGBA{0x0A, 0x14, 0x28, 0xFF})
			}
		})
	}
}

// BenchmarkPNGDecode aísla el coste de decodificar el PNG del icono,
// que se repite por cada icono no cacheado.
func BenchmarkPNGDecode(b *testing.B) {
	raw, err := os.ReadFile("../../testdata/ddragon/3070.png")
	if err != nil {
		b.Skip("sin icono real")
	}
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			b.Fatal(err)
		}
	}
}
