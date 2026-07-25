package hub

import (
	"image"
	"image/color"
	"image/png"
	"os"
)

// DiffResult is the outcome of a pixel comparison.
type DiffResult struct {
	// Score is the fraction of compared pixels that mismatch, in [0, 1].
	Score float64
	// DimensionMismatch is set when the two images differ in size; the
	// comparison then covers only the intersection.
	DimensionMismatch bool
}

// DiffPNG compares two PNG files pixel by pixel and writes a visual diff to
// outPath: the first image dimmed to grayscale, with mismatching pixels in
// magenta. tolerance is the per-channel difference (0-255) absorbed as equal —
// it exists to swallow anti-aliasing noise, not real drift.
func DiffPNG(aPath, bPath, outPath string, tolerance uint8) (DiffResult, error) {
	a, err := readPNG(aPath)
	if err != nil {
		return DiffResult{}, err
	}
	b, err := readPNG(bPath)
	if err != nil {
		return DiffResult{}, err
	}

	ab, bb := a.Bounds(), b.Bounds()
	w := min(ab.Dx(), bb.Dx())
	h := min(ab.Dy(), bb.Dy())
	res := DiffResult{DimensionMismatch: ab.Dx() != bb.Dx() || ab.Dy() != bb.Dy()}
	if w == 0 || h == 0 {
		res.Score = 1
		return res, nil
	}

	out := image.NewRGBA(image.Rect(0, 0, w, h))
	tol := uint32(tolerance) << 8 // RGBA() returns 16-bit channels
	mismatched := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if absDiff(r1, r2) > tol || absDiff(g1, g2) > tol || absDiff(b1, b2) > tol {
				mismatched++
				out.Set(x, y, color.RGBA{R: 255, G: 0, B: 200, A: 255})
			} else {
				// Dimmed grayscale of the first image as context.
				g := uint8(((r1 + g1 + b1) / 3) >> 8)
				g = g/3 + 160
				out.Set(x, y, color.RGBA{R: g, G: g, B: g, A: 255})
			}
		}
	}
	res.Score = float64(mismatched) / float64(w*h)

	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return res, err
		}
		defer f.Close()
		if err := png.Encode(f, out); err != nil {
			return res, err
		}
	}
	return res, nil
}

func readPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
