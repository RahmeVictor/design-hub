package hub

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func writeSolidPNG(t *testing.T, path string, w, h int, c color.RGBA, topRows int, top color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if y < topRows {
				img.Set(x, y, top)
			} else {
				img.Set(x, y, c)
			}
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

func TestDiffIdentical(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	white := color.RGBA{255, 255, 255, 255}
	writeSolidPNG(t, a, 100, 100, white, 0, white)
	writeSolidPNG(t, b, 100, 100, white, 0, white)
	res, err := DiffPNG(a, b, filepath.Join(dir, "d.png"), 16)
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 0 || res.DimensionMismatch {
		t.Fatalf("expected clean diff, got %+v", res)
	}
}

func TestDiffTenPercent(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	writeSolidPNG(t, a, 100, 100, white, 0, white)
	writeSolidPNG(t, b, 100, 100, white, 10, black) // top 10 rows differ
	res, err := DiffPNG(a, b, filepath.Join(dir, "d.png"), 16)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(res.Score-0.10) > 0.001 {
		t.Fatalf("expected score 0.10, got %f", res.Score)
	}
	// The diff image exists with the intersection dimensions.
	f, err := os.Open(filepath.Join(dir, "d.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 100 || img.Bounds().Dy() != 100 {
		t.Fatalf("unexpected diff dimensions: %v", img.Bounds())
	}
}

func TestDiffToleranceAbsorbsNoise(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	writeSolidPNG(t, a, 50, 50, color.RGBA{200, 200, 200, 255}, 0, color.RGBA{})
	writeSolidPNG(t, b, 50, 50, color.RGBA{210, 210, 210, 255}, 0, color.RGBA{}) // within tolerance 16
	res, err := DiffPNG(a, b, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 0 {
		t.Fatalf("tolerance should absorb a 10/255 shift, got score %f", res.Score)
	}
}

func TestDiffDimensionMismatch(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	white := color.RGBA{255, 255, 255, 255}
	writeSolidPNG(t, a, 100, 100, white, 0, white)
	writeSolidPNG(t, b, 80, 100, white, 0, white)
	res, err := DiffPNG(a, b, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DimensionMismatch {
		t.Fatal("expected DimensionMismatch")
	}
	if res.Score != 0 {
		t.Fatalf("identical intersection should score 0, got %f", res.Score)
	}
}
