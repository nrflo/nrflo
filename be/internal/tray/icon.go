package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// iconBytes is a 22x22 template icon (black filled circle on transparent).
// macOS renders template icons correctly in both light and dark mode.
var iconBytes []byte

func init() {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	center := float64(size) / 2
	radius := float64(size)/2 - 3

	for y := range size {
		for x := range size {
			dx := float64(x) - center + 0.5
			dy := float64(y) - center + 0.5
			if math.Sqrt(dx*dx+dy*dy) <= radius {
				img.Set(x, y, color.RGBA{0, 0, 0, 255})
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	iconBytes = buf.Bytes()
}
