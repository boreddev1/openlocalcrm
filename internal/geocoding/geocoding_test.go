package geocoding_test

import (
	"testing"

	"github.com/openlocalcrm/openlocalcrm/internal/geocoding"
	"github.com/stretchr/testify/require"
)

func TestLookupPLZ_ValidGermanPLZ(t *testing.T) {
	// Berlin Mitte
	lat, lon, ok := geocoding.LookupPLZ("10115")
	require.True(t, ok)
	require.InDelta(t, 52.53, lat, 0.1)
	require.InDelta(t, 13.38, lon, 0.1)

	// Frankfurt am Main
	lat, lon, ok = geocoding.LookupPLZ("60311")
	require.True(t, ok)
	require.InDelta(t, 50.11, lat, 0.1)
	require.InDelta(t, 8.68, lon, 0.1)

	// Munich
	lat, lon, ok = geocoding.LookupPLZ("80331")
	require.True(t, ok)
	require.InDelta(t, 48.13, lat, 0.1)
	require.InDelta(t, 11.57, lon, 0.1)
}

func TestLookupPLZ_InvalidOrUnknownPLZ(t *testing.T) {
	_, _, ok := geocoding.LookupPLZ("")
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("9999999") // too long
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("12") // 2 digits: NO fallback
	require.False(t, ok)

	_, _, ok = geocoding.LookupPLZ("ABCDE")
	require.False(t, ok)
}

func TestLookupPLZ_TrimsWhitespace(t *testing.T) {
	lat, lon, ok := geocoding.LookupPLZ("  10115 \n")
	require.True(t, ok)
	require.InDelta(t, 52.53, lat, 0.1)
	require.InDelta(t, 13.38, lon, 0.1)
}
