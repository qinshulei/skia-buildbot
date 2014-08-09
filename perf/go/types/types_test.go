package types

import (
	"fmt"
	"testing"

	"skia.googlesource.com/buildbot.git/perf/go/config"
)

func TestMerge(t *testing.T) {
	t1 := NewTile()
	t1.Scale = 1
	t1.TileIndex = 20
	t1.Commits[1].Hash = "hash1"

	t2 := NewTile()
	t2.Scale = 1
	t2.TileIndex = 21
	t2.Commits[1].Hash = "hash33"
	t2.Commits[2].Hash = "hash34"

	// Create a Trace that exists in both tile1 and tile2.
	tr := NewTrace()
	tr.Params["p1"] = "v1"
	tr.Params["p2"] = "v2"
	tr.Values[0] = 0.1
	tr.Values[1] = 0.2

	t1.Traces["foo"] = tr

	tr = NewTrace()
	tr.Params["p1"] = "v1"
	tr.Params["p2"] = "v2"
	tr.Values[0] = 0.3
	tr.Values[1] = 0.4

	t2.Traces["foo"] = tr

	// Add a trace that only appears in tile2.
	tr = NewTrace()
	tr.Params["p1"] = "v1"
	tr.Params["p3"] = "v3"
	tr.Values[0] = 0.5
	tr.Values[1] = 0.6

	t2.Traces["bar"] = tr

	// Merge the two tiles.
	merged := Merge(t1, t2)

	if got, want := merged.Scale, 1; got != want {
		fmt.Errorf("Wrong scale: Got %v Want %v", got, want)
	}
	if got, want := merged.TileIndex, t1.TileIndex; got != want {
		fmt.Errorf("TileIndex is wrong: Got %v Want %v", got, want)
	}
	if got, want := len(merged.Traces), 2; got != want {
		fmt.Errorf("Number of traces: Got %v Want %v", got, want)
	}
	if got, want := len(merged.Traces["foo"].Values), 2*config.TILE_SIZE; got != want {
		fmt.Errorf("Number of values: Got %v Want %v", got, want)
	}
	if got, want := len(merged.ParamSet), 3; got != want {
		fmt.Errorf("ParamSet length: Got %v Want %v", got, want)
	}

	// Test the "foo" trace.
	tr = merged.Traces["foo"]
	testCases := []struct {
		N int
		V float64
	}{
		{31, 1e100},
		{32, 0.3},
		{33, 0.4},
		{34, 1e100},
		{0, 1e100},
		{1, 0.1},
		{2, 0.2},
		{3, 1e100},
	}
	for _, tc := range testCases {
		if got, want := tr.Values[tc.N], tc.V; got != want {
			fmt.Errorf("Error copying trace values: Got %v Want %v at %d", got, want, tc.N)
		}
	}
	if got, want := tr.Params["p1"], "v1"; got != want {
		fmt.Errorf("Wrong params for trace: Got %v Want %v", got, want)
	}

	// Test the "bar" trace.
	tr = merged.Traces["foo"]
	testCases = []struct {
		N int
		V float64
	}{
		{31, 1e100},
		{32, 0.5},
		{33, 0.6},
		{34, 1e100},
	}
	for _, tc := range testCases {
		if got, want := tr.Values[tc.N], tc.V; got != want {
			fmt.Errorf("Error copying trace values: Got %v Want %v at %d", got, want, tc.N)
		}
	}
	if got, want := tr.Params["p3"], "v3"; got != want {
		fmt.Errorf("Wrong params for trace: Got %v Want %v", got, want)
	}
}
