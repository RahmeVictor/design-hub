package hub

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"sort"
)

// DiffRegion is a clustered area of mismatching pixels, in diff-image
// coordinates. Regions are ranked by mismatch count, biggest first.
type DiffRegion struct {
	X                int     `json:"x"`
	Y                int     `json:"y"`
	W                int     `json:"w"`
	H                int     `json:"h"`
	MismatchedPixels int     `json:"mismatchedPixels"`
	SharePercent     float64 `json:"sharePercent"` // of all mismatched pixels
}

// DiffResult is the outcome of a pixel comparison.
type DiffResult struct {
	// Score is the fraction of compared pixels that mismatch, in [0, 1].
	Score float64
	// DimensionMismatch is set when the two images differ in size; the
	// comparison then covers only the intersection.
	DimensionMismatch bool
	// Regions are the biggest clusters of mismatch, largest first (max
	// maxRegions, clusters under minRegionShare of the mismatch dropped).
	Regions []DiffRegion
}

const (
	regionTile     = 16   // clustering granularity in pixels
	maxRegions     = 5    // keep at most this many regions
	minRegionShare = 0.01 // drop regions holding under 1% of the mismatch
)

var regionOutline = color.RGBA{R: 255, G: 149, B: 0, A: 255} // orange, distinct from the magenta pixels

// DiffPNG compares two PNG files pixel by pixel and writes a visual diff to
// outPath: the first image dimmed to grayscale, mismatching pixels in magenta,
// and the biggest mismatch clusters outlined in orange. tolerance is the
// per-channel difference (0-255) absorbed as equal — it exists to swallow
// anti-aliasing noise, not real drift.
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
	mask := make([]bool, w*h)
	tol := uint32(tolerance) << 8 // RGBA() returns 16-bit channels
	mismatched := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r1, g1, b1, _ := a.At(ab.Min.X+x, ab.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			if absDiff(r1, r2) > tol || absDiff(g1, g2) > tol || absDiff(b1, b2) > tol {
				mismatched++
				mask[y*w+x] = true
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
	res.Regions = findRegions(mask, w, h, mismatched)
	for _, r := range res.Regions {
		drawRect(out, r, regionOutline)
	}

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

// findRegions clusters the mismatch mask into regions: mismatch counts per
// regionTile-sized tile, connected components over the hot tiles
// (4-connectivity), then each component's bounds refined to the exact extent
// of its mismatching pixels.
func findRegions(mask []bool, w, h, total int) []DiffRegion {
	if total == 0 {
		return nil
	}
	tw := (w + regionTile - 1) / regionTile
	th := (h + regionTile - 1) / regionTile
	counts := make([]int, tw*th)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] {
				counts[(y/regionTile)*tw+x/regionTile]++
			}
		}
	}

	// Flood-fill hot tiles into components.
	comp := make([]int, tw*th)
	for i := range comp {
		comp[i] = -1
	}
	var regions []DiffRegion
	for start := range counts {
		if counts[start] == 0 || comp[start] >= 0 {
			continue
		}
		id := len(regions)
		stack := []int{start}
		comp[start] = id
		count := 0
		minTX, minTY, maxTX, maxTY := tw, th, -1, -1
		for len(stack) > 0 {
			t := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			tx, ty := t%tw, t/tw
			count += counts[t]
			minTX, minTY = min(minTX, tx), min(minTY, ty)
			maxTX, maxTY = max(maxTX, tx), max(maxTY, ty)
			for _, n := range [4]int{t - 1, t + 1, t - tw, t + tw} {
				if n < 0 || n >= tw*th {
					continue
				}
				if (n == t-1 && tx == 0) || (n == t+1 && tx == tw-1) {
					continue // no wrapping across rows
				}
				if counts[n] > 0 && comp[n] < 0 {
					comp[n] = id
					stack = append(stack, n)
				}
			}
		}
		r := exactBounds(mask, w, h, minTX*regionTile, minTY*regionTile,
			min((maxTX+1)*regionTile, w), min((maxTY+1)*regionTile, h))
		r.MismatchedPixels = count
		r.SharePercent = float64(count) / float64(total) * 100
		regions = append(regions, r)
	}

	sort.Slice(regions, func(i, j int) bool {
		return regions[i].MismatchedPixels > regions[j].MismatchedPixels
	})
	keep := regions[:0]
	for _, r := range regions {
		if len(keep) == maxRegions || r.SharePercent < minRegionShare*100 {
			break
		}
		keep = append(keep, r)
	}
	return keep
}

// exactBounds shrinks a tile-aligned box to the exact extent of the
// mismatching pixels inside it.
func exactBounds(mask []bool, w, h, x0, y0, x1, y1 int) DiffRegion {
	minX, minY, maxX, maxY := x1, y1, x0-1, y0-1
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if mask[y*w+x] {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX { // hot tiles from a neighboring component only — shouldn't happen
		return DiffRegion{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
	}
	return DiffRegion{X: minX, Y: minY, W: maxX - minX + 1, H: maxY - minY + 1}
}

// drawRect outlines a region with a 2px border, clamped to the image.
func drawRect(img *image.RGBA, r DiffRegion, c color.RGBA) {
	b := img.Bounds()
	x0, y0 := max(r.X, b.Min.X), max(r.Y, b.Min.Y)
	x1, y1 := min(r.X+r.W, b.Max.X), min(r.Y+r.H, b.Max.Y)
	for t := 0; t < 2; t++ {
		for x := x0; x < x1; x++ {
			if y0+t < y1 {
				img.SetRGBA(x, y0+t, c)
			}
			if y1-1-t >= y0 {
				img.SetRGBA(x, y1-1-t, c)
			}
		}
		for y := y0; y < y1; y++ {
			if x0+t < x1 {
				img.SetRGBA(x0+t, y, c)
			}
			if x1-1-t >= x0 {
				img.SetRGBA(x1-1-t, y, c)
			}
		}
	}
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
