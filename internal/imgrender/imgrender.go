// Package imgrender convierte imágenes en half-blocks truecolor
// ("▀" con color arriba y abajo), el mayor salto de calidad para
// iconos en terminales modernas sin depender de protocolos gráficos.
// Reescala con CatmullRom, funde la transparencia sobre el color del
// panel y emite ANSI de 24 bits por celda.
package imgrender

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// Render convierte img en cols×rows celdas (cada celda = 2 píxeles
// verticales). bg es el color de fondo del panel: los píxeles
// transparentes se funden con él.
func Render(img image.Image, cols, rows int, bg color.RGBA) string {
	if cols <= 0 || rows <= 0 {
		return ""
	}
	// 1. Reescalar (CatmullRom) a cols × rows*2 píxeles.
	small := image.NewRGBA(image.Rect(0, 0, cols, rows*2))
	xdraw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), xdraw.Over, nil)

	// 2. Componer sobre el fondo del panel.
	canvas := image.NewRGBA(small.Bounds())
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), small, image.Point{}, draw.Over)

	// 3. Una celda = "▀" con fg = píxel superior y bg = inferior.
	var sb strings.Builder
	for y := 0; y < rows*2; y += 2 {
		for x := 0; x < cols; x++ {
			t := canvas.RGBAAt(x, y)
			b := canvas.RGBAAt(x, y+1)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", t.R, t.G, t.B, b.R, b.G, b.B)
		}
		sb.WriteString("\x1b[0m")
		if y+2 < rows*2 {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// RenderAspect calcula las filas para mantener el aspecto de la imagen
// (cada celda es ~2× más alta que ancha) y renderiza.
func RenderAspect(img image.Image, cols int, bg color.RGBA) string {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return ""
	}
	rows := cols * b.Dy() / b.Dx() / 2
	if rows < 1 {
		rows = 1
	}
	return Render(img, cols, rows, bg)
}

// AutoContrast estira los niveles de la imagen (recorte del 1% en los
// extremos por luminancia), aplicado a los tres canales. Rescata los
// retratos oscuros.
func AutoContrast(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)

	var hist [256]int
	for i := 0; i < len(dst.Pix); i += 4 {
		l := (int(dst.Pix[i])*299 + int(dst.Pix[i+1])*587 + int(dst.Pix[i+2])*114) / 1000
		hist[l]++
	}
	total := b.Dx() * b.Dy()
	lo, hi, acc := 0, 255, 0
	for i := 0; i < 256; i++ {
		acc += hist[i]
		if acc > total/100 {
			lo = i
			break
		}
	}
	acc = 0
	for i := 255; i >= 0; i-- {
		acc += hist[i]
		if acc > total/100 {
			hi = i
			break
		}
	}
	if hi <= lo {
		return dst
	}
	for i := 0; i < len(dst.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			v := (int(dst.Pix[i+c]) - lo) * 255 / (hi - lo)
			dst.Pix[i+c] = uint8(max(0, min(255, v)))
		}
	}
	return dst
}

// Crop recorta un porcentaje central (frac 0..1 del lado) y, en
// retratos, desplaza la ventana hacia arriba (el sujeto suele estar en
// el centro-superior). Para cuadrados perfectos usa frac=1.
func Crop(img image.Image, frac float64) image.Image {
	if frac >= 1 || frac <= 0 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cw, ch := int(float64(w)*frac), int(float64(h)*frac)
	x0 := b.Min.X + (w-cw)/2
	y0 := b.Min.Y + (h-ch)/3 // sesgo hacia la cara
	type subImager interface {
		SubImage(image.Rectangle) image.Image
	}
	si, ok := img.(subImager)
	if !ok {
		return img
	}
	return si.SubImage(image.Rect(x0, y0, x0+cw, y0+ch))
}
