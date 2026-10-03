// Package geo turns coordinates into #geo cells: the first six characters of
// the Open Location Code, lowercased (0.05° ≈ 5.5 km). A cell is just a
// string, so "near me" becomes a text search on any network.
package geo

import (
	"math"
	"strings"
)

const (
	alphabet = "23456789cfghjmpqrvwx"
	// CellDeg is the size of a 6-char cell in degrees (both axes).
	CellDeg = 0.05
)

// Cell returns the 6-char cell for a coordinate.
func Cell(lat, lon float64) string {
	lat = math.Min(math.Max(lat, -90), 90-1e-9) + 90
	lon = math.Mod(math.Mod(lon+180, 360)+360, 360)
	var b strings.Builder
	for _, res := range []float64{20, 1, CellDeg} {
		la := int(math.Floor(lat/res + 1e-9))
		lo := int(math.Floor(lon/res + 1e-9))
		la, lo = min(la, 19), min(lo, 19)
		b.WriteByte(alphabet[la])
		b.WriteByte(alphabet[lo])
		lat -= float64(la) * res
		lon -= float64(lo) * res
	}
	return b.String()
}

// Valid reports whether s is a well-formed 6-char cell.
func Valid(s string) bool {
	if len(s) != 6 {
		return false
	}
	for i := 0; i < 6; i++ {
		if strings.IndexByte(alphabet, s[i]) < 0 {
			return false
		}
	}
	// First latitude digit can't exceed 8 (90°+90° = 180° / 20° = 9 values).
	return strings.IndexByte(alphabet, s[0]) <= 8 && strings.IndexByte(alphabet, s[1]) <= 17
}

// Center returns the centre coordinate of a valid cell.
func Center(cell string) (lat, lon float64) {
	cell = strings.ToLower(cell)
	for i, res := range []float64{20, 1, CellDeg} {
		lat += float64(strings.IndexByte(alphabet, cell[2*i])) * res
		lon += float64(strings.IndexByte(alphabet, cell[2*i+1])) * res
	}
	return lat - 90 + CellDeg/2, lon - 180 + CellDeg/2
}

// Ring returns the cells at Chebyshev distance r from cell, in a stable
// order. Ring 0 is the cell itself. Cells past the poles are dropped and
// longitude wraps, so this replaces whenwhere's Ulam-spiral digit arithmetic.
func Ring(cell string, r int) []string {
	lat, lon := Center(cell)
	if r == 0 {
		return []string{strings.ToLower(cell)}
	}
	var out []string
	seen := map[string]bool{}
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if max(abs(dx), abs(dy)) != r {
				continue
			}
			la := lat + float64(dy)*CellDeg
			if la < -90 || la >= 90 {
				continue
			}
			c := Cell(la, lon+float64(dx)*CellDeg)
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// Rings returns rings 0..r concatenated, nearest first.
func Rings(cell string, r int) []string {
	var out []string
	for i := 0; i <= r; i++ {
		out = append(out, Ring(cell, i)...)
	}
	return out
}

// Tag is the public hashtag form of a cell, without the '#'.
func Tag(cell string) string { return "geo" + strings.ToLower(cell) }

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
