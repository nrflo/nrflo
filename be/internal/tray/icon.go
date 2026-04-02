package tray

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const iconSize = 88

var boldFace font.Face

func init() {
	tt, err := opentype.Parse(gomonobold.TTF)
	if err != nil {
		panic(fmt.Sprintf("tray: parse gomonobold: %v", err))
	}
	boldFace, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    32,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic(fmt.Sprintf("tray: new face: %v", err))
	}
}

// renderIcon returns PNG bytes for the tray icon.
// count == 0: "NRF" centered. count > 0: "NRF" on top, count on bottom.
// Black text on transparent background — macOS template icon handles light/dark.
func renderIcon(count int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))

	col := color.Black

	if count == 0 {
		drawCentered(img, boldFace, col, "NRF", iconSize/2+16)
	} else {
		drawCentered(img, boldFace, col, "NRF", 36)
		drawCentered(img, boldFace, col, fmt.Sprintf("%d", count), 72)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// drawCentered draws text horizontally centered at the given y baseline.
func drawCentered(img *image.RGBA, face font.Face, col color.Color, text string, y int) {
	width := font.MeasureString(face, text)
	x := (fixed.I(iconSize) - width) / 2

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.Point26_6{X: x, Y: fixed.I(y)},
	}
	d.DrawString(text)
}
