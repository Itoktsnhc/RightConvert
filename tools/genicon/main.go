// Command genicon draws the app icon, resources/icon.png: the initials RC
// in white on a rounded square of a diagonal gradient.
//
//	go run ./tools/genicon
package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

const (
	size   = 1024
	margin = 64  // transparent border, as Windows and macOS icons have
	radius = 200 // corner radius of the square
)

var (
	from = color.NRGBA{0x4F, 0x46, 0xE5, 0xFF} // indigo, top left
	to   = color.NRGBA{0x9D, 0x3C, 0xF0, 0xFF} // violet, bottom right
)

func main() {
	out := "resources/icon.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// The rounded square's coverage, antialiased.
	var r vector.Rasterizer
	r.Reset(size, size)
	roundedRect(&r, margin, margin, size-margin, size-margin, radius)
	mask := image.NewAlpha(img.Bounds())
	r.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})

	grad := image.NewRGBA(img.Bounds())
	for y := range size {
		for x := range size {
			t := float64(x+y) / float64(2*(size-1))
			grad.Set(x, y, lerp(from, to, t))
		}
	}
	draw.DrawMask(img, img.Bounds(), grad, image.Point{}, mask, image.Point{}, draw.Over)

	drawInitials(img, "RC")

	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

func drawInitials(img *image.RGBA, s string) {
	tt, err := opentype.Parse(gobold.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face, err := opentype.NewFace(tt, &opentype.FaceOptions{Size: 440, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(err)
	}
	defer face.Close()
	d := &font.Drawer{Dst: img, Src: image.NewUniform(color.White), Face: face}
	// Center the ink of the letters, not their advance box.
	bounds, _ := d.BoundString(s)
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y
	d.Dot = fixed.Point26_6{
		X: fixed.I(size/2) - w/2 - bounds.Min.X,
		Y: fixed.I(size/2) - h/2 - bounds.Min.Y,
	}
	d.DrawString(s)
}

// roundedRect adds a rounded rectangle path, its corners approximated by
// cubic Béziers.
func roundedRect(r *vector.Rasterizer, x0, y0, x1, y1, rad float32) {
	k := rad * float32(4*(math.Sqrt2-1)/3)
	r.MoveTo(x0+rad, y0)
	r.LineTo(x1-rad, y0)
	r.CubeTo(x1-rad+k, y0, x1, y0+rad-k, x1, y0+rad)
	r.LineTo(x1, y1-rad)
	r.CubeTo(x1, y1-rad+k, x1-rad+k, y1, x1-rad, y1)
	r.LineTo(x0+rad, y1)
	r.CubeTo(x0+rad-k, y1, x0, y1-rad+k, x0, y1-rad)
	r.LineTo(x0, y0+rad)
	r.CubeTo(x0, y0+rad-k, x0+rad-k, y0, x0+rad, y0)
	r.ClosePath()
}

func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.NRGBA{m(a.R, b.R), m(a.G, b.G), m(a.B, b.B), 0xFF}
}
