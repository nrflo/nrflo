package tray

import (
	"bytes"
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
		panic("tray: parse gomonobold: " + err.Error())
	}
	boldFace, err = opentype.NewFace(tt, &opentype.FaceOptions{
		Size:    44,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		panic("tray: new face: " + err.Error())
	}
}

// renderIcon returns PNG bytes for the tray icon.
// Shows "NRF" centered. Agent count is displayed via systray.SetTitle().
// Black text on transparent background — macOS template icon handles light/dark.
func renderIcon() []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	drawCentered(img, boldFace, color.Black, "NRF", iconSize/2+16)

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
