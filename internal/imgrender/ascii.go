// ascii.go: renderer ASCII de calidad — contraste por percentiles con
// gamma, color por celda (con refuerzo de brillo/saturación),
// caracteres direccionales en los bordes marcados y dithering Bayer
// para texturizar los degradados. Mantiene el aspecto de ASCII
// "clásico" pero legible.
package imgrender

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sort"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// ASCIIOpts configura el render ASCII.
type ASCIIOpts struct {
	Ramp   string     // de menos a más denso
	Color  bool       // true: color por celda; false: tinte único
	Tint   color.RGBA // tinte si Color == false
	Edges  bool       // usar / \ | - en los bordes marcados
	Dither bool       // dithering ordenado para dar textura
}

// DefaultASCII es el ajuste recomendado para retratos.
var DefaultASCII = ASCIIOpts{
	Ramp:   " .:-=+*#%@",
	Color:  true,
	Edges:  true,
	Dither: true,
}

var bayer4 = [4][4]float64{
	{0, 8, 2, 10}, {12, 4, 14, 6}, {3, 11, 1, 9}, {15, 7, 13, 5},
}

// ASCII convierte img en arte ASCII de cols columnas, componiendo la
// transparencia sobre bg.
func ASCII(img image.Image, cols int, bg color.RGBA, o ASCIIOpts) string {
	b := img.Bounds()
	if cols <= 0 || b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}
	// la celda es ~1:2, así que las filas son la mitad
	rows := int(math.Round(float64(cols) * float64(b.Dy()) / float64(b.Dx()) * 0.5))
	if rows < 1 {
		rows = 1
	}

	// 1. reescalar y componer sobre el fondo del panel
	small := image.NewRGBA(image.Rect(0, 0, cols, rows))
	xdraw.CatmullRom.Scale(small, small.Bounds(), img, b, xdraw.Over, nil)
	canvas := image.NewRGBA(small.Bounds())
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), small, image.Point{}, draw.Over)

	// 2. luminancia y estiramiento por percentiles (1% / 99%)
	lum := make([]float64, cols*rows)
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := canvas.RGBAAt(x, y)
			lum[y*cols+x] = (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
		}
	}
	sorted := append([]float64(nil), lum...)
	sort.Float64s(sorted)
	lo, hi := sorted[len(sorted)/100], sorted[len(sorted)*99/100]
	if hi-lo < 0.05 {
		hi = lo + 0.05
	}
	for i, v := range lum {
		lum[i] = math.Max(0, math.Min(1, (v-lo)/(hi-lo)))
		lum[i] = math.Pow(lum[i], 0.85) // gamma: aclara los medios tonos
	}

	ramp := []rune(o.Ramp)
	n := len(ramp)

	var sb strings.Builder
	for y := 0; y < rows; y++ {
		fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm", bg.R, bg.G, bg.B)
		for x := 0; x < cols; x++ {
			v := lum[y*cols+x]

			// dithering ordenado: desplaza el valor antes de cuantizar
			if o.Dither {
				v += (bayer4[y%4][x%4]/16 - 0.5) / float64(n)
			}
			idx := int(math.Max(0, math.Min(float64(n-1), v*float64(n))))
			ch := ramp[idx]

			// bordes: si el gradiente es fuerte, carácter direccional
			if o.Edges && x > 0 && y > 0 && x < cols-1 && y < rows-1 {
				gx := lum[y*cols+x+1] - lum[y*cols+x-1]
				gy := (lum[(y+1)*cols+x] - lum[(y-1)*cols+x]) * 0.5 // compensa la celda alta
				if mag := math.Hypot(gx, gy); mag > 0.28 {
					a := math.Mod(math.Atan2(gy, gx)*180/math.Pi+180, 180)
					switch {
					case a < 22.5 || a >= 157.5:
						ch = '|'
					case a < 67.5:
						ch = '/'
					case a < 112.5:
						ch = '-'
					default:
						ch = '\\'
					}
				}
			}

			// color del glifo
			var fg color.RGBA
			if o.Color {
				fg = boost(canvas.RGBAAt(x, y))
			} else {
				t := 0.35 + 0.65*v
				fg = color.RGBA{uint8(float64(o.Tint.R) * t), uint8(float64(o.Tint.G) * t), uint8(float64(o.Tint.B) * t), 255}
			}
			if ch == ' ' {
				sb.WriteByte(' ')
				continue
			}
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm%c", fg.R, fg.G, fg.B, ch)
		}
		sb.WriteString("\x1b[0m")
		if y+1 < rows {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// GoldTint es el tinte dorado para el modo monocromo de la paleta.
var GoldTint = color.RGBA{212, 162, 76, 255}

// boost sube brillo y saturación para que el glifo se lea sobre fondo
// oscuro.
func boost(c color.RGBA) color.RGBA {
	r, g, b := float64(c.R), float64(c.G), float64(c.B)
	avg := (r + g + b) / 3
	const sat = 1.35
	r, g, b = avg+(r-avg)*sat, avg+(g-avg)*sat, avg+(b-avg)*sat
	m := math.Max(r, math.Max(g, b))
	if m < 140 && m > 0 {
		k := 140 / m
		r, g, b = r*k, g*k, b*k
	}
	cl := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }
	return color.RGBA{cl(r), cl(g), cl(b), 255}
}
