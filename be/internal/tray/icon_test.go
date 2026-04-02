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
