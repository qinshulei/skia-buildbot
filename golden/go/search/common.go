// search contains the core functionality for searching for digests across a tile.
package search

import (
	"go.skia.org/infra/go/paramtools"
	"go.skia.org/infra/go/sklog"
	"go.skia.org/infra/go/tiling"
	"go.skia.org/infra/golden/go/indexer"
	"go.skia.org/infra/golden/go/types"
)

// AcceptFn is the callback function used by iterTile to determine whether to
// include a trace into the result or not. The second return value is an
// intermediate result that will be passed to addFn to avoid redundant computation.
// The second return value is application dependent since it will be passed
// into the call to the corresponding AddFn. Determining whether to accept a
// result might require an expensive computation and we want to avoid repeating
// that computation in the 'add' step. So we can return it here and it will
// be passed into the instance of AddFn.
type AcceptFn func(params paramtools.Params, digests []string) (bool, interface{})

// AddFn is the callback function used by iterTile to add a digest and it's
// trace to the result. acceptResult is the same value returned by the AcceptFn.
type AddFn func(test, digest, traceID string, trace *types.GoldenTrace, acceptResult interface{})

// iterTile is a generic function to extract information from a tile.
// It iterates over the tile and filters against the given query. If calls
// acceptFn to determine whether to keep a trace (after it has already been
// tested against the query) and calls addFn to add a digest and its trace.
// acceptFn == nil equals unconditional acceptance.
func iterTile(query *Query, addFn AddFn, acceptFn AcceptFn, exp ExpSlice, idx *indexer.SearchIndex) error {
	tile := idx.GetTile(query.IncludeIgnores)

	if acceptFn == nil {
		acceptFn = func(params paramtools.Params, digests []string) (bool, interface{}) { return true, nil }
	}

	traceTally := idx.TalliesByTrace(query.IncludeIgnores)
	lastTraceIdx, traceView, err := getTraceViewFn(tile, query.FCommitBegin, query.FCommitEnd)
	if err != nil {
		return err
	}

	// Iterate through the tile.
	for id, trace := range tile.Traces {
		// Check if the query matches.
		if tiling.Matches(trace, query.Query) {
			fullTr := trace.(*types.GoldenTrace)
			params := fullTr.Params_
			reducedTr := traceView(fullTr)
			digests := digestsFromTrace(id, reducedTr, query.Head, lastTraceIdx, traceTally)

			// If there is an acceptFn defined then check whether
			// we should include this trace.
			ok, acceptRet := acceptFn(params, digests)
			if !ok {
				continue
			}

			// Iterate over the digess and filter them.
			test := params[types.PRIMARY_KEY_FIELD]
			for _, digest := range digests {
				cl := exp.Classification(test, digest)
				if query.excludeClassification(cl) {
					continue
				}

				// Fix blamer to make this easier.
				if query.BlameGroupID != "" {
					if cl == types.UNTRIAGED {
						b := idx.GetBlame(test, digest, tile.Commits)
						if query.BlameGroupID != blameGroupID(b, tile.Commits) {
							continue
						}
					} else {
						continue
					}
				}

				// Add the digest to the results
				addFn(test, digest, id, fullTr, acceptRet)
			}
		}
	}
	return nil
}

// traceViewFn returns a view of a trace that contains a subset of values but the same params.
type traceViewFn func(*types.GoldenTrace) *types.GoldenTrace

// traceViewIdentity is a no-op traceViewFn that returns the exact trace that it
// receives. This is used when no commit range is provided in the query.
func traceViewIdentity(tr *types.GoldenTrace) *types.GoldenTrace {
	return tr
}

// getTraceViewFn returns a traceViewFn for the given Git hashes as well as the
// index of the last value in the resulting traces.
// If startHash occurs after endHash in the tile, an error is returned.
func getTraceViewFn(tile *tiling.Tile, startHash, endHash string) (int, traceViewFn, error) {
	if startHash == "" && endHash == "" {
		return tile.LastCommitIndex(), traceViewIdentity, nil
	}

	// Find the indices to slice the values of the trace.
	startIdx, _ := tiling.FindCommit(tile.Commits, startHash)
	endIdx, _ := tiling.FindCommit(tile.Commits, endHash)
	if (startIdx == -1) && (endIdx == -1) {
		return tile.LastCommitIndex(), traceViewIdentity, nil
	}

	// If either was not found set it to the beginning/end.
	if startIdx == -1 {
		startIdx = 0
	} else if endIdx == -1 {
		endIdx = tile.LastCommitIndex()
	}

	// Increment the last index for the slice operation in the function below.
	endIdx++
	if startIdx >= endIdx {
		return 0, nil, sklog.FmtErrorf("Start commit occurs later than end commit.")
	}

	ret := func(trace *types.GoldenTrace) *types.GoldenTrace {
		return &types.GoldenTrace{
			Values:  trace.Values[startIdx:endIdx],
			Params_: trace.Params_,
		}
	}

	return (endIdx - startIdx) - 1, ret, nil
}
