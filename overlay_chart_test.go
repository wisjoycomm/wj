package main

import (
	"reflect"
	"testing"
)

func TestTrimLeadingEmptyKeepsAtLeastFour(t *testing.T) {
	if got := trimLeadingEmpty([]int64{0, 0, 0, 5, 0, 7}, 4); !reflect.DeepEqual(got, []int64{0, 5, 0, 7}) {
		t.Errorf("got %v", got)
	}
	if got := trimLeadingEmpty([]int64{0, 0, 0, 0, 0, 3}, 4); !reflect.DeepEqual(got, []int64{0, 0, 0, 3}) {
		t.Errorf("got %v", got)
	}
	if got := trimLeadingEmpty([]int64{1, 2}, 4); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("a short series stays: %v", got)
	}
}

func TestColumnHeightsScaleToTheTallest(t *testing.T) {
	if got := columnHeights([]int64{0, 1, 50, 100}, 20); !reflect.DeepEqual(got, []int{0, 1, 10, 20}) {
		t.Errorf("got %v", got)
	}
	if got := columnHeights([]int64{0, 0}, 20); !reflect.DeepEqual(got, []int{0, 0}) {
		t.Errorf("no calls, no columns: %v", got)
	}
}

func TestComposeColumnsColoursTheCurrentPeriod(t *testing.T) {
	const w, h = 40, 12
	pix := make([]byte, w*h*4)
	past, current, base := [3]byte{1, 1, 1}, [3]byte{2, 2, 2}, [3]byte{3, 3, 3}
	composeColumns(pix, w, h, 0, 0, w, h, 2, []int64{10, 0, 0, 20}, past, current, base)
	at := func(x, y int) byte { return pix[(y*w+x)*4] }
	if at(0, h-1) != 3 || at(25, h-1) != 3 {
		t.Error("the base line spans the chart")
	}
	if at(0, h-2) != 1 || at(0, 0) != 0 {
		t.Error("the first column is past-coloured and half as tall as the tallest")
	}
	if at(30, 0) != 2 || at(30, h-2) != 2 {
		t.Error("the last column is the current period, full height")
	}
	if at(12, h-2) != 0 {
		t.Error("an empty period draws nothing above the base line")
	}
}
