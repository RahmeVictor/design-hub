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

func TestDiffRegions(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}

	// Base: all white. Other: two separated black blobs — 40x40 at (10,10)
	// and 20x20 at (150,150).
	img := image.NewRGBA(image.Rect(0, 0, 200, 200))
	blob := func(x0, y0, size int) {
		for y := y0; y < y0+size; y++ {
			for x := x0; x < x0+size; x++ {
				img.Set(x, y, black)
			}
		}
	}
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			img.Set(x, y, white)
		}
	}
	blob(10, 10, 40)
	blob(150, 150, 20)
	writeSolidPNG(t, a, 200, 200, white, 0, white)
	f, err := os.Create(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	out := filepath.Join(dir, "d.png")
	res, err := DiffPNG(a, b, out, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Regions) != 2 {
		t.Fatalf("expected 2 regions, got %+v", res.Regions)
	}
	r0, r1 := res.Regions[0], res.Regions[1]
	if r0.X != 10 || r0.Y != 10 || r0.W != 40 || r0.H != 40 {
		t.Fatalf("biggest region should be the 40x40 blob at (10,10), got %+v", r0)
	}
	if r1.X != 150 || r1.Y != 150 || r1.W != 20 || r1.H != 20 {
		t.Fatalf("second region should be the 20x20 blob at (150,150), got %+v", r1)
	}
	if r0.SharePercent < r1.SharePercent {
		t.Fatal("regions must be ranked biggest first")
	}
	if math.Abs(r0.SharePercent-80) > 0.5 || math.Abs(r1.SharePercent-20) > 0.5 {
		t.Fatalf("expected ~80/20 shares, got %f / %f", r0.SharePercent, r1.SharePercent)
	}

	// The outline is drawn on the diff image at the region border.
	df, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()
	diffImg, err := png.Decode(df)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.RGBAModel.Convert(diffImg.At(10, 10)); got != regionOutline {
		t.Fatalf("expected the orange outline at (10,10), got %v", got)
	}
}

func TestDiffRegionsNoiseFiltered(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.png")
	white := color.RGBA{255, 255, 255, 255}
	black := color.RGBA{0, 0, 0, 255}
	// One big 100x100 blob and one single stray pixel: the stray holds
	// ~0.01% of the mismatch, far under minRegionShare — dropped.
	img := image.NewRGBA(image.Rect(0, 0, 300, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 300; x++ {
			img.Set(x, y, white)
		}
	}
	for y := 20; y < 120; y++ {
		for x := 20; x < 120; x++ {
			img.Set(x, y, black)
		}
	}
	img.Set(250, 250, black)
	writeSolidPNG(t, a, 300, 300, white, 0, white)
	f, err := os.Create(b)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err := DiffPNG(a, b, "", 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Regions) != 1 {
		t.Fatalf("stray pixel should be filtered, got %+v", res.Regions)
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
