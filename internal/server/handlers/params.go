package handlers

import "math"

// clampInt32 converts a client-supplied integer (pagination limit/offset) to
// int32 without wrapping: values outside the int32 range saturate instead of
// truncating, so a huge value can never turn into a small or negative one.
func clampInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < math.MinInt32 {
		return math.MinInt32
	}
	return int32(n) //nolint:gosec // G115: range checked above
}
