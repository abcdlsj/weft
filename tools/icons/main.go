// Command icons renders the PWA icons from the same mark the panel draws.
// Run: go run ./tools/icons
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

var (
	linen  = color.RGBA{0xf6, 0xf4, 0xef, 0xff}
	indigo = color.RGBA{0x2f, 0x3a, 0x78, 0xff}
)

func main() {
	out := "web/public/icons"
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}
	targets := []struct {
		name string
		size int
		full bool // full-bleed square for maskable icons
	}{
		{"icon-192.png", 192, false},
		{"icon-512.png", 512, false},
		{"icon-maskable-512.png", 512, true},
		{"apple-touch-icon.png", 180, false},
	}
	for _, t := range targets {
		img := render(t.size, t.full)
		f, err := os.Create(filepath.Join(out, t.name))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			log.Fatal(err)
		}
		f.Close()
		log.Printf("wrote %s", filepath.Join(out, t.name))
	}
}

// render draws the mark: a linen field, three warp threads crossed by two weft
// threads. Maskable icons keep the mark inside the 80% safe circle.
func render(size int, full bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{0, 0, 0, 0}
	if full {
		bg = linen
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, bg)
		}
	}
	// rounded square field
	r := size / 5
	if full {
		r = 0
	}
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if r > 0 && !insideRounded(x, y, size, r) {
				continue
			}
			img.Set(x, y, linen)
		}
	}

	unit := size / 8
	warp := size / 220
	if warp < 2 {
		warp = 2
	}
	// warps: three vertical threads
	for i := 0; i < 3; i++ {
		cx := size/2 + (i-1)*unit*2
		rect(img, cx-warp/2, size*30/100-unit/4, warp, size*40/100+unit/2, color.RGBA{0x2f, 0x3a, 0x78, 0x73})
	}
	// wefts: two horizontal threads
	for i := 0; i < 2; i++ {
		cy := size/2 + (i*2-1)*unit*2/2
		rect(img, size*27/100, cy-warp, size*46/100, warp*2, indigo)
	}
	return img
}

func insideRounded(x, y, size, r int) bool {
	cx, cy := size/2, size/2
	dx, dy := abs(x-cx), abs(y-cy)
	lim := size/2 - r
	if dx <= lim || dy <= lim {
		return true
	}
	return (dx-lim)*(dx-lim)+(dy-lim)*(dy-lim) <= r*r
}

func rect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			if image.Pt(xx, yy).In(img.Bounds()) {
				img.Set(xx, yy, c)
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
