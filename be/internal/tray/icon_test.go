package tray

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestRenderIcon_ValidPNG_CountZero(t *testing.T) {
	data := renderIcon(0)
	if len(data) == 0 {
		t.Fatal("renderIcon(0) returned empty bytes")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("renderIcon(0) did not produce valid PNG: %v", err)
	}
}

func TestRenderIcon_ValidPNG_CountPositive(t *testing.T) {
	data := renderIcon(5)
	if len(data) == 0 {
		t.Fatal("renderIcon(5) returned empty bytes")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("renderIcon(5) did not produce valid PNG: %v", err)
	}
}

func TestRenderIcon_Dimensions(t *testing.T) {
	cases := []struct {
		count int
	}{
		{0},
		{1},
		{99},
	}
	for _, tc := range cases {
		data := renderIcon(tc.count)
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("renderIcon(%d): image.Decode error: %v", tc.count, err)
		}
		bounds := img.Bounds()
		if bounds.Dx() != iconSize || bounds.Dy() != iconSize {
			t.Errorf("renderIcon(%d) dimensions = %dx%d, want %dx%d",
				tc.count, bounds.Dx(), bounds.Dy(), iconSize, iconSize)
		}
	}
}

func TestRenderIcon_DifferentOutputForDifferentCounts(t *testing.T) {
	idle := renderIcon(0)
	busy := renderIcon(5)
	if bytes.Equal(idle, busy) {
		t.Error("renderIcon(0) and renderIcon(5) produced identical bytes; expected different icons")
	}
}

func TestRenderIcon_MultiDigitCount(t *testing.T) {
	data := renderIcon(99)
	if len(data) == 0 {
		t.Fatal("renderIcon(99) returned empty bytes")
	}
	if _, err := png.Decode(bytes.NewReader(data)); err != nil {
		t.Errorf("renderIcon(99) did not produce valid PNG: %v", err)
	}
}

func TestRenderIcon_CountOneVsCountTen(t *testing.T) {
	// Different digit counts should produce different icons.
	one := renderIcon(1)
	ten := renderIcon(10)
	if bytes.Equal(one, ten) {
		t.Error("renderIcon(1) and renderIcon(10) produced identical bytes")
	}
}

func TestRenderIcon_HasNonTransparentPixels(t *testing.T) {
	// Verify the icon is not a blank/empty canvas — actual text must be drawn.
	for _, count := range []int{0, 1, 5, 99} {
		data := renderIcon(count)
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("renderIcon(%d): image.Decode error: %v", count, err)
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
			t.Errorf("renderIcon(%d): all pixels are transparent; expected rendered text", count)
		}
	}
}

func TestRenderIcon_ThreeDigitCount(t *testing.T) {
	// Three-digit count (999) must produce a valid PNG without panicking or clipping.
	data := renderIcon(999)
	if len(data) == 0 {
		t.Fatal("renderIcon(999) returned empty bytes")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Errorf("renderIcon(999) did not produce valid PNG: %v", err)
		return
	}
	bounds := img.Bounds()
	if bounds.Dx() != iconSize || bounds.Dy() != iconSize {
		t.Errorf("renderIcon(999) dimensions = %dx%d, want %dx%d",
			bounds.Dx(), bounds.Dy(), iconSize, iconSize)
	}
}

func TestRenderIcon_CountZeroVsCountOne(t *testing.T) {
	// count==0 (NRF-only layout) must differ from count==1 (NRF + digit layout).
	zero := renderIcon(0)
	one := renderIcon(1)
	if bytes.Equal(zero, one) {
		t.Error("renderIcon(0) and renderIcon(1) produced identical bytes; expected different layouts")
	}
}

func TestRenderIcon_AllCountsDifferFromIdle(t *testing.T) {
	idle := renderIcon(0)
	for _, count := range []int{1, 2, 9, 10, 42, 99, 100} {
		busy := renderIcon(count)
		if bytes.Equal(idle, busy) {
			t.Errorf("renderIcon(%d) produced same bytes as renderIcon(0)", count)
		}
	}
}
