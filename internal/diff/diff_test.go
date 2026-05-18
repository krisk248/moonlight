package diff

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writeSolidPNG writes a w×h PNG of a single color.
func writeSolidPNG(t *testing.T, path string, w, h int, c color.Color) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestDiff_Identical_RatioZero(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.png")
	b := filepath.Join(tmp, "b.png")
	out := filepath.Join(tmp, "out.png")
	writeSolidPNG(t, a, 50, 50, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	writeSolidPNG(t, b, 50, 50, color.RGBA{R: 100, G: 100, B: 100, A: 255})

	r, err := Diff(a, b, out, 5)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ratio != 0 {
		t.Errorf("identical images should give ratio=0, got %v", r.Ratio)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("overlay should exist: %v", err)
	}
}

func TestDiff_Different_RatioHigh(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.png")
	b := filepath.Join(tmp, "b.png")
	out := filepath.Join(tmp, "out.png")
	writeSolidPNG(t, a, 30, 30, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	writeSolidPNG(t, b, 30, 30, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	r, err := Diff(a, b, out, 5)
	if err != nil {
		t.Fatal(err)
	}
	if r.Ratio < 0.99 {
		t.Errorf("solid black vs white should be ~1.0, got %v", r.Ratio)
	}
}

func TestDiff_Tolerance_SuppressesSmallChange(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.png")
	b := filepath.Join(tmp, "b.png")
	out := filepath.Join(tmp, "out.png")
	writeSolidPNG(t, a, 20, 20, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	writeSolidPNG(t, b, 20, 20, color.RGBA{R: 105, G: 100, B: 100, A: 255})

	// tolerance >= 5 → no diff, tolerance < 5 → full diff
	r1, _ := Diff(a, b, out, 10)
	if r1.Ratio != 0 {
		t.Errorf("tolerance=10 should suppress delta=5, got %v", r1.Ratio)
	}
	r2, _ := Diff(a, b, out, 2)
	if r2.Ratio < 0.99 {
		t.Errorf("tolerance=2 should flag delta=5 as changed, got %v", r2.Ratio)
	}
}

func TestDiff_SizeMismatchIsHandled(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.png")
	b := filepath.Join(tmp, "b.png")
	out := filepath.Join(tmp, "out.png")
	writeSolidPNG(t, a, 40, 40, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	writeSolidPNG(t, b, 60, 60, color.RGBA{R: 100, G: 100, B: 100, A: 255})

	r, err := Diff(a, b, out, 5)
	if err != nil {
		t.Fatal(err)
	}
	if !r.SizeMismatch {
		t.Errorf("expected SizeMismatch=true")
	}
}
