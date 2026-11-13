// Command icons renders the PWA icons and the favicon mark from one geometry.
// Run from the repo root: go run ./tools/icons
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Everything below is in the 24 unit grid of the icon and of mark.svg, so the
// PNGs, the favicon and the panel's Mark component stay the same drawing.
const box = 24

var (
	linen  = color.RGBA{0xf6, 0xf4, 0xef, 0xff} // the field: unbleached cloth
	indigo = color.RGBA{0x2f, 0x3a, 0x78, 0xff} // weft
	tint   = color.RGBA{0x8f, 0x97, 0xbd, 0xff} // warp: indigo washed out, still 2.6:1 on linen
)

const (
	tileRadius = 5.6 // rounded square of the app icon, ~23% of the side

	warpWidth = 1.4
	weftWidth = 2.0

	// Both families of threads run 4.8..19.2, so the ink is a centred square
	// and neither direction looks cut short; the caps add ~0.3 of overhang.
	spanMin = 4.8
	spanMax = 19.2
)

var (
	warpX = []float64{6.4, 12, 17.6} // three warps, 5.6 apart
	weftY = []float64{9.2, 14.8}     // two wefts, 5.6 apart: square cells
)

type segment struct{ x1, y1, x2, y2, width float64 }

// paint is one opaque pass: a colour and a signed distance in device pixels,
// negative inside. Coverage is a one pixel ramp across the edge, which is all
// the anti-aliasing an icon of straight, round-capped threads needs.
type paint struct {
	col  color.RGBA
	dist func(x, y float64) float64
}

func main() {
	out := "web/public/icons"
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	targets := []struct {
		name  string
		size  int
		full  bool    // full-bleed square: maskable, and apple-touch (iOS masks it itself)
		scale float64 // mark size relative to the app icon
	}{
		{"icon-192.png", 192, false, 1},
		{"icon-512.png", 512, false, 1},
		{"icon-maskable-512.png", 512, true, 0.8},
		{"apple-touch-icon.png", 180, true, 1},
	}
	for _, t := range targets {
		if err := writePNG(filepath.Join(out, t.name), t.size, t.full, t.scale); err != nil {
			log.Fatal(err)
		}
		log.Printf("wrote %s", filepath.Join(out, t.name))
	}

	mark := filepath.Join(out, "mark.svg")
	if err := os.WriteFile(mark, []byte(svg()), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %s", mark)
}

func writePNG(path string, size int, full bool, scale float64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	return enc.Encode(f, render(size, full, scale))
}

func render(size int, full bool, scale float64) *image.NRGBA {
	side := float64(size)
	radius := tileRadius * side / box
	if full {
		radius = 0
	}
	paints := append([]paint{{linen, roundRect(side, side, radius)}}, markPaints(side, scale)...)

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for py := 0; py < size; py++ {
		y := float64(py) + 0.5
		for px := 0; px < size; px++ {
			x := float64(px) + 0.5
			// premultiplied result, so a translucent tile corner is correct
			var r, g, b, a float64
			for _, p := range paints {
				cov := math.Min(1, math.Max(0, 0.5-p.dist(x, y)))
				if cov == 0 {
					continue
				}
				sr, sg, sb := float64(p.col.R), float64(p.col.G), float64(p.col.B)
				r = sr*cov + r*(1-cov)
				g = sg*cov + g*(1-cov)
				b = sb*cov + b*(1-cov)
				a = cov + a*(1-cov)
			}
			if a == 0 {
				continue
			}
			img.SetNRGBA(px, py, color.NRGBA{
				R: clamp8(r / a), G: clamp8(g / a), B: clamp8(b / a), A: clamp8(a * 255),
			})
		}
	}
	return img
}

func markPaints(side, scale float64) []paint {
	center := side / 2
	k := side / box * scale // device pixels per grid unit
	place := func(s segment) func(x, y float64) float64 {
		return func(x, y float64) float64 {
			gx := (x-center)/k + box/2
			gy := (y-center)/k + box/2
			return (segmentDist(gx, gy, s) - s.width/2) * k
		}
	}

	var paints []paint
	for _, x := range warpX {
		paints = append(paints, paint{tint, place(segment{x, spanMin, x, spanMax, warpWidth})})
	}
	for _, y := range weftY {
		paints = append(paints, paint{indigo, place(segment{spanMin, y, spanMax, y, weftWidth})})
	}
	return paints
}

func segmentDist(x, y float64, s segment) float64 {
	dx, dy := s.x2-s.x1, s.y2-s.y1
	t := ((x-s.x1)*dx + (y-s.y1)*dy) / (dx*dx + dy*dy)
	t = math.Min(1, math.Max(0, t))
	return math.Hypot(x-(s.x1+t*dx), y-(s.y1+t*dy))
}

// roundRect is the distance to a w×h box with radius r, grown by nothing and
// placed at the origin: the tile fills the canvas, so its corners are the only
// edge that matters.
func roundRect(w, h, r float64) func(x, y float64) float64 {
	hw, hh := w/2, h/2
	return func(x, y float64) float64 {
		qx := math.Abs(x-hw) - (hw - r)
		qy := math.Abs(y-hh) - (hh - r)
		return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	}
}

func clamp8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(math.Round(v))
}

func svg() string {
	var b strings.Builder
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">` + "\n")
	fmt.Fprintf(&b, "  <rect width=\"24\" height=\"24\" rx=\"%s\" fill=\"%s\" />\n", num(tileRadius), hex(linen))
	b.WriteString("  <g fill=\"none\" stroke-linecap=\"round\">\n")

	var warps strings.Builder
	for _, x := range warpX {
		fmt.Fprintf(&warps, "M%s %sV%s", num(x), num(spanMin), num(spanMax))
	}
	fmt.Fprintf(&b, "    <path d=\"%s\" stroke=\"%s\" stroke-width=\"%s\" />\n", warps.String(), hex(tint), num(warpWidth))

	var wefts strings.Builder
	for _, y := range weftY {
		fmt.Fprintf(&wefts, "M%s %sH%s", num(spanMin), num(y), num(spanMax))
	}
	fmt.Fprintf(&b, "    <path d=\"%s\" stroke=\"%s\" stroke-width=\"%s\" />\n", wefts.String(), hex(indigo), num(weftWidth))

	b.WriteString("  </g>\n</svg>\n")
	return b.String()
}

func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
