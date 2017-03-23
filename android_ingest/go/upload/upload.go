package upload

import (
	"crypto/md5"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.skia.org/infra/perf/go/ingestcommon"
)

// ObjectPath returns the Google Cloud Storage path where the JSON serialization
// of benchData should be stored.
//
// gcsPath will be the root of the path.
// now is the time which will be encoded in the path.
// b is the source bytes of the incoming file.
func ObjectPath(benchData *ingestcommon.BenchData, gcsPath string, now time.Time, b []byte) string {
	hash := fmt.Sprintf("%x", md5.Sum(b))
	keyparts := []string{}
	if benchData.Key != nil {
		for k, v := range benchData.Key {
			keyparts = append(keyparts, k, v)
		}
	}
	filename := fmt.Sprintf("%s_%s_%s.json", benchData.Hash, strings.Join(keyparts, "_"), hash)
	return filepath.Join(gcsPath, now.Format("2006/01/02/15/"), filename)
}
