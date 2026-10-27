// Command genicons genera iconos PNG de respaldo (deterministas por
// semilla) para todos los campeones e ítems de los fixtures de Data
// Dragon, de modo que el conversor imagen→ASCII tenga material
// offline. Con red, la app usa los iconos reales vía caché.
//
// Uso: go run ./cmd/genicons
package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// drawSigil pinta un icono abstracto pero distintivo: gradiente radial
// con tono derivado de la semilla, anillo y banda diagonal.
func drawSigil(seed string, size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	h := float64(fnv(seed)%3600) / 10.0
	ring := fnv(seed+"ring")%2 == 0
	diag := fnv(seed+"diag")%2 == 0
	c := size / 2
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-c), float64(y-c)
			d := math.Hypot(dx, dy) / float64(c)
			// HSL rápido: tono fijo, luminosidad decreciente con el radio.
			l := 0.62*(1-d) + 0.12
			s := 0.68
			if ring && math.Abs(d-0.72) < 0.07 {
				l = 0.88
			}
			if diag && math.Abs(dx+dy) < float64(size)*0.06 && d < 0.95 {
				l = 0.85
				s = 0.3
			}
			r8, g8, b8 := hsl(h, s, l)
			a := uint8(255)
			if d > 1 {
				a = 0 // esquinas transparentes
			}
			img.SetNRGBA(x, y, color.NRGBA{R: r8, G: g8, B: b8, A: a})
		}
	}
	return img
}

// hsl→rgb simplificado (s y l en 0..1).
func hsl(h float64, s, l float64) (uint8, uint8, uint8) {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}

func fnv(s string) uint64 {
	var h uint64 = 14695981039346656037
	for _, b := range []byte(s) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	return h
}

func main() {
	n := 0
	n += genGameIcons("testdata/ddragon", "champion.json", "item.json", 120, 64)
	n += genGameIcons("testdata/deadlock", "heroes.json", "items.json", 120, 64)
	fmt.Printf("genicons: %d iconos generados\n", n)
}

// genGameIcons genera {clave}.png para cada héroe y {id}.png para cada
// ítem del juego cuyos estáticos están en dir.
func genGameIcons(dir, heroesFile, itemsFile string, heroSize, itemSize int) int {
	var heroes map[string]any
	mustReadJSON(filepath.Join(dir, heroesFile), &heroes)
	heroData, _ := heroes["data"].(map[string]any)

	var items map[string]any
	mustReadJSON(filepath.Join(dir, itemsFile), &items)
	itemData, _ := items["data"].(map[string]any)

	n := 0
	for key := range heroData {
		writeIcon(dir, key+".png", heroSize)
		n++
	}
	for id := range itemData {
		writeIcon(dir, id+".png", itemSize)
		n++
	}
	fmt.Printf("genicons: %s → %d iconos\n", dir, n)
	return n
}

func writeIcon(dir, name string, size int) {
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		fail(err)
	}
	defer f.Close()
	if err := png.Encode(f, drawSigil(name, size)); err != nil {
		fail(err)
	}
}

func mustReadJSON(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicons:", err)
	os.Exit(1)
}
