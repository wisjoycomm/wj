package main

import "testing"

func TestComposeOverlayKeepsTextOpaqueOverSeeThroughBackground(t *testing.T) {
	bg := [3]byte{100, 100, 100}
	// Three pixels in a row: background only, solid text, half covered text.
	colour := []byte{0, 0, 0, 0, 200, 200, 200, 0, 100, 100, 100, 0}
	mask := []byte{0, 0, 0, 0, 255, 255, 255, 0, 128, 128, 128, 0}
	composeOverlay(colour, mask, 3, 1, 0, bg, 128)
	if colour[3] != 128 || colour[0] != 50 {
		t.Errorf("background pixel = %v", colour[:4])
	}
	if colour[7] != 255 || colour[4] != 200 {
		t.Errorf("text pixel = %v", colour[4:8])
	}
	if colour[11] <= 128 || colour[8] > colour[11] {
		t.Errorf("edge pixel not valid premultiplied: %v", colour[8:12])
	}
}

func TestComposeBorderMarksEdgeAndRule(t *testing.T) {
	const w, h = 10, 8
	pix := make([]byte, w*h*4)
	composeBorder(pix, w, h, 0, [3]byte{200, 200, 200}, 255, 2, 4, 1)
	at := func(x, y int) []byte { i := (y*w + x) * 4; return pix[i : i+4] }
	if at(0, 3)[3] != 255 || at(5, 0)[3] != 255 || at(3, 4)[3] != 255 {
		t.Errorf("edge or rule missing: %v %v %v", at(0, 3), at(5, 0), at(3, 4))
	}
	if at(5, 2)[3] != 0 || at(1, 4)[3] != 0 {
		t.Errorf("interior touched: %v %v", at(5, 2), at(1, 4))
	}
}

func TestCornerCoverageRoundsOnlyTheCorners(t *testing.T) {
	if got := cornerCoverage(0, 0, 40, 20, 6); got != 0 {
		t.Errorf("corner = %v", got)
	}
	if got := cornerCoverage(39, 19, 40, 20, 6); got != 0 {
		t.Errorf("opposite corner = %v", got)
	}
	for _, p := range [][2]int{{20, 0}, {0, 10}, {20, 10}} {
		if got := cornerCoverage(p[0], p[1], 40, 20, 6); got != 1 {
			t.Errorf("%v = %v", p, got)
		}
	}
}
