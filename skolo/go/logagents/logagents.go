package logagents

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"go.skia.org/infra/go/fileutil"
	"go.skia.org/infra/go/util"
	"go.skia.org/infra/skolo/go/gcl"
)

// A LogScanner scans some log file(s), parses them, and reports any new logs to
// the specified CloudLogger.  ReportName() should be the name of the log as shared with GCL.
type LogScanner interface {
	Scan(logsclient gcl.CloudLogger) error
	ReportName() string
}

// persistenceDir is the folder in which persistence data regarding the logging
// progress will be kept.
var persistenceDir = ""

func SetPersistenceDir(dir string) error {
	if dir, err := fileutil.EnsureDirExists(dir); err != nil {
		return fmt.Errorf("Could not create persistence dir: %s", err)
	} else {
		persistenceDir = dir
		return nil
	}
}

// readAndHashFile opens a file, reads the contents, hashes them and returns the contents and hash.
// This is a package level var so it can be mocked out when unit testing
var readAndHashFile = _readAndHashFile

func _readAndHashFile(path string) (contents, hash string, err error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty string if file doesn't exist.  Same as "no logs".
			return "", "", nil
		}
		return "", "", fmt.Errorf("Problem opening log file %s: %s", path, err)
	}
	defer util.Close(f)
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return "", "", fmt.Errorf("Problem reading log file %s: %s", path, err)
	}

	return string(b), fmt.Sprintf("%x", sha1.Sum(b)), nil
}

// writeToPersistenceFile writes the given lines to a file in persistenceDir using reportName as the
// name of the file. It will be encoded as json. If the file already has content, it will be
// truncated.  This is a package level var so it can be mocked out when unit testing.
var writeToPersistenceFile = _writeToPersistenceFile

func _writeToPersistenceFile(reportName string, v interface{}) error {
	f, err := os.Create(filepath.Join(persistenceDir, reportName))
	if err != nil {
		return fmt.Errorf("Could not open %s for writing: %s", f.Name(), err)
	}
	defer util.Close(f)
	return json.NewEncoder(f).Encode(v)
}

// readFromPersistenceFile reads a file in persistenceDir using reportName as the
// name of the file. It expects the file to be encoded as JSON.
// This is a package level var so it can be mocked out when unit testing.
var readFromPersistenceFile = _readFromPersistenceFile

func _readFromPersistenceFile(reportName string, v interface{}) error {
	f, err := os.Open(filepath.Join(persistenceDir, reportName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer util.Close(f)
	return json.NewDecoder(f).Decode(v)
}
