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

func TestBarFill(t *testing.T) {
	cases := []struct {
		total, limit int64
		want         int
	}{{0, 100, 0}, {50, 100, 50}, {100, 100, 100}, {250, 100, 100}, {10, 0, 0}}
	for _, c := range cases {
		if got := barFill(c.total, c.limit, 100); got != c.want {
			t.Errorf("barFill(%d,%d) = %d, want %d", c.total, c.limit, got, c.want)
		}
	}
}

func TestComposeBarAndRule(t *testing.T) {
	pix := make([]byte, 4*4*2)
	composeBar(pix, 4, 2, 0, 0, 4, 1, 2, [3]byte{1, 1, 1}, [3]byte{9, 9, 9})
	if pix[0] != 9 || pix[4*2] != 1 || pix[3] != 255 {
		t.Errorf("bar row = %v", pix[:16])
	}
	composeRule(pix, 4, 2, 1, 3, 1, 1, [3]byte{5, 5, 5})
	if pix[16] != 0 || pix[20] != 5 || pix[28] != 0 {
		t.Errorf("rule row = %v", pix[16:])
	}
	composeRule(pix, 4, 2, -5, 99, 7, 1, [3]byte{5, 5, 5}) // off image: no panic
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
