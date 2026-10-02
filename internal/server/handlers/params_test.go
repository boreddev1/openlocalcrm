package handlers

import (
	"math"
	"testing"
)

func TestClampInt32(t *testing.T) {
	cases := []struct {
		in   int
		want int32
	}{
		{0, 0},
		{50, 50},
		{-7, -7},
		{math.MaxInt32, math.MaxInt32},
		{math.MaxInt32 + 1, math.MaxInt32},
		{1 << 40, math.MaxInt32},
		{math.MinInt32 - 1, math.MinInt32},
	}
	for _, c := range cases {
		if got := clampInt32(c.in); got != c.want {
			t.Errorf("clampInt32(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
