package slide

import (
	"image"
	"image/color"
	"math"
)

// saliencyGrid is roughly how many cells the longer side of an Image is
// reduced to before saliency is measured. Coarser is faster and ignores noise.
const saliencyGrid = 400

// Crop returns the largest rectangle of img with the aspect ratio w:h, placed
// along the axis that has to be trimmed so that it covers as much visual
// detail (local contrast) as possible. When nothing stands out, the crop is
// centered.
func Crop(img image.Image, w, h int) image.Rectangle {
	b := img.Bounds()
	cw, ch := b.Dx(), b.Dy()
	switch {
	case cw*h > ch*w:
		cw = max(1, ch*w/h)
	case cw*h < ch*w:
		ch = max(1, cw*h/w)
	default:
		return b
	}

	k := max(1, max(b.Dx(), b.Dy())/saliencyGrid)
	grid := lumaGrid(img, k)
	horizontal := cw < b.Dx()

	var profile []float64
	var window, slack int
	if horizontal {
		profile = saliencyProfile(grid, true)
		window, slack = cw, b.Dx()-cw
	} else {
		profile = saliencyProfile(grid, false)
		window, slack = ch, b.Dy()-ch
	}

	off := bestOffset(profile, k, window, slack)
	if horizontal {
		return image.Rect(b.Min.X+off, b.Min.Y, b.Min.X+off+cw, b.Max.Y)
	}
	return image.Rect(b.Min.X, b.Min.Y+off, b.Max.X, b.Min.Y+off+ch)
}

// lumaGrid averages the luma of img over k×k blocks.
func lumaGrid(img image.Image, k int) [][]float64 {
	b := img.Bounds()
	gw, gh := (b.Dx()+k-1)/k, (b.Dy()+k-1)/k
	sum := make([][]float64, gh)
	n := make([][]int, gh)
	for gy := range sum {
		sum[gy] = make([]float64, gw)
		n[gy] = make([]int, gw)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		gy := (y - b.Min.Y) / k
		for x := b.Min.X; x < b.Max.X; x++ {
			gx := (x - b.Min.X) / k
			sum[gy][gx] += float64(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y)
			n[gy][gx]++
		}
	}
	for gy := range sum {
		for gx := range sum[gy] {
			sum[gy][gx] /= float64(n[gy][gx])
		}
	}
	return sum
}

// saliencyProfile sums local contrast per grid column (byColumn) or per row.
func saliencyProfile(grid [][]float64, byColumn bool) []float64 {
	gh, gw := len(grid), len(grid[0])
	size := gh
	if byColumn {
		size = gw
	}
	profile := make([]float64, size)
	for gy := 0; gy < gh; gy++ {
		for gx := 0; gx < gw; gx++ {
			var s float64
			if gx+1 < gw {
				s += math.Abs(grid[gy][gx+1] - grid[gy][gx])
			}
			if gy+1 < gh {
				s += math.Abs(grid[gy+1][gx] - grid[gy][gx])
			}
			if byColumn {
				profile[gx] += s
			} else {
				profile[gy] += s
			}
		}
	}
	return profile
}

// bestOffset picks the pixel offset in [0, slack] whose window of the given
// pixel length covers the most saliency. Ties go to the offset nearest the
// center.
func bestOffset(profile []float64, k, window, slack int) int {
	prefix := make([]float64, len(profile)+1)
	for i, v := range profile {
		prefix[i+1] = prefix[i] + v
	}
	cells := min(len(profile), max(1, window/k))
	center := float64(slack) / 2

	best, bestScore := int(center), -1.0
	for start := 0; start+cells <= len(profile); start++ {
		off := min(start*k, slack)
		score := prefix[start+cells] - prefix[start]
		if score > bestScore ||
			(score == bestScore && math.Abs(float64(off)-center) < math.Abs(float64(best)-center)) {
			best, bestScore = off, score
		}
	}
	return best
}
