package tray

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestRenderIcon_ValidPNG(t *testing.T) {
	data := renderIcon()
	if len(data) == 0 {
		t.Fatal("renderIcon() returned empty bytes")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("renderIcon() did not produce valid PNG: %v", err)
	}
}

func TestRenderIcon_Dimensions(t *testing.T) {
	data := renderIcon()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("image.Decode error: %v", err)
	}
	bounds := img.Bounds()
	if bounds.Dx() != iconSize || bounds.Dy() != iconSize {
		t.Errorf("dimensions = %dx%d, want %dx%d",
			bounds.Dx(), bounds.Dy(), iconSize, iconSize)
	}
}

func TestRenderIcon_HasNonTransparentPixels(t *testing.T) {
	data := renderIcon()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("image.Decode error: %v", err)
	}
	found := false
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y && !found; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("all pixels are transparent; expected rendered text")
	}
}

func TestRenderIcon_Deterministic(t *testing.T) {
	a := renderIcon()
	b := renderIcon()
	if !bytes.Equal(a, b) {
		t.Error("two calls to renderIcon() produced different bytes")
	}
}
