// Package diff computes pixel-level differences between two PNG screenshots
// and writes a red-overlay highlighting changed pixels.
package diff

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

// Result is the outcome of a pixel diff.
type Result struct {
	Ratio        float64 // 0..1 fraction of pixels differing beyond Tolerance
	OverlayPath  string  // path to the red-overlay PNG
	SizeMismatch bool    // true if the two images had different dimensions
}

// Diff loads two PNGs, computes the per-pixel max channel delta, marks any
// delta > tolerance as a changed pixel, and writes a red-highlighted overlay
// of the baseline showing where the change is.
func Diff(baselinePath, currentPath, overlayPath string, tolerance uint8) (Result, error) {
	baseline, err := loadPNG(baselinePath)
	if err != nil {
		return Result{}, fmt.Errorf("load baseline: %w", err)
	}
	current, err := loadPNG(currentPath)
	if err != nil {
		return Result{}, fmt.Errorf("load current: %w", err)
	}

	bb := baseline.Bounds()
	cb := current.Bounds()
	sizeMismatch := false

	// Resize current to baseline dims (nearest-neighbour) if they differ —
	// keeps the overlay aligned with the baseline.
	if !cb.Eq(bb) {
		sizeMismatch = true
		current = resizeNearest(current, bb)
	}

	overlay := image.NewRGBA(bb)
	draw.Draw(overlay, bb, baseline, bb.Min, draw.Src)

	changed := 0
	total := bb.Dx() * bb.Dy()
	red := color.RGBA{R: 255, G: 0, B: 0, A: 255}

	for y := bb.Min.Y; y < bb.Max.Y; y++ {
		for x := bb.Min.X; x < bb.Max.X; x++ {
			br, bg, bbl, _ := baseline.At(x, y).RGBA()
			cr, cg, cbl, _ := current.At(x, y).RGBA()
			dr := absDiff8(br>>8, cr>>8)
			dg := absDiff8(bg>>8, cg>>8)
			db := absDiff8(bbl>>8, cbl>>8)
			d := maxU8(dr, maxU8(dg, db))
			if d > tolerance {
				overlay.Set(x, y, red)
				changed++
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		return Result{}, err
	}
	f, err := os.Create(overlayPath)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	if err := png.Encode(f, overlay); err != nil {
		return Result{}, err
	}

	ratio := 0.0
	if total > 0 {
		ratio = float64(changed) / float64(total)
	}
	return Result{Ratio: ratio, OverlayPath: overlayPath, SizeMismatch: sizeMismatch}, nil
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func absDiff8(a, b uint32) uint8 {
	if a > b {
		return uint8(a - b)
	}
	return uint8(b - a)
}

func maxU8(a, b uint8) uint8 {
	if a > b {
		return a
	}
	return b
}

// resizeNearest is a tiny nearest-neighbour resizer (no extra deps).
func resizeNearest(src image.Image, target image.Rectangle) image.Image {
	dst := image.NewRGBA(target)
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	tw, th := target.Dx(), target.Dy()
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			sx := sb.Min.X + x*sw/tw
			sy := sb.Min.Y + y*sh/th
			dst.Set(target.Min.X+x, target.Min.Y+y, src.At(sx, sy))
		}
	}
	return dst
}
