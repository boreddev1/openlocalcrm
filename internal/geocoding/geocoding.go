// Package geocoding provides offline German postal code (PLZ) centroid lookups.
//
// Dataset: German Postal Codes (PLZ) Centroids
// License: CC0 1.0 Universal (Public Domain Dedication)
package geocoding

import (
	"strings"
)

// LookupPLZ returns the latitude and longitude centroid for an exact 5-digit German postal code.
// Returns ok = false if the postal code is invalid or not found.
func LookupPLZ(plz string) (lat, lon float64, ok bool) {
	clean := strings.TrimSpace(plz)
	if len(clean) != 5 {
		return 0, 0, false
	}
	coords, found := plzCentroids[clean]
	if !found {
		return 0, 0, false
	}
	return coords[0], coords[1], true
}
