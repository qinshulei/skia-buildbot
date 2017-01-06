// poprepo is a package for populating a git repo with commits that associate
// a git commit with a buildid, a monotonically increasing number maintained
// by a external build system. This is needed because Perf only knows how
// to associate measurement values with git commits.
package poprepo

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"

	"go.skia.org/infra/go/sklog"

	"go.skia.org/infra/go/exec"
	"go.skia.org/infra/go/git"
)

const (
	BUILDID_FILENAME = "BUILDID"
)

// PopRepoI is the interface that PopRepo supports.
//
// It supports adding and reading BuildIDs from a git repo.
type PopRepoI interface {
	// GetLast returns the last committed buildid, its timestamp, and git hash.
	GetLast() (int64, int64, string, error)

	// Add a new buildid to the repo.
	Add(buildid, ts int64) error
}

// PopRepo implements PopRepoI.
type PopRepo struct {
	checkout *git.Checkout
	workdir  string
	local    bool
}

// NewPopRepo returns a *PopRepo that writes and reads BuildIds into the
// 'checkout'.
//
// If not 'local' then the HOME environment variable is set for running on the
// server.
func NewPopRepo(checkout *git.Checkout, local bool) *PopRepo {
	return &PopRepo{
		checkout: checkout,
		workdir:  checkout.Dir(),
		local:    local,
	}
}

// GetLast returns the last buildid, the timestamp of when that buildid was
// added, and the git hash.
func (p *PopRepo) GetLast() (int64, int64, string, error) {
	fullpath := filepath.Join(p.checkout.Dir(), BUILDID_FILENAME)
	b, err := ioutil.ReadFile(fullpath)
	if err != nil {
		return 0, 0, "", fmt.Errorf("Unable to read file %q: %s", fullpath, err)
	}
	s := strings.TrimSpace(string(b))
	parts := strings.Split(s, " ")
	if len(parts) != 2 {
		return 0, 0, "", fmt.Errorf("Unable to find just buildid and timestamp in: %q", s)
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("Timestamp is invalid in %q: %s", s, err)
	}
	buildid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, "", fmt.Errorf("BuildID is invalid in %q: %s", s, err)
	}
	hash, err := p.checkout.RevParse("HEAD")
	if err != nil {
		return 0, 0, "", fmt.Errorf("Unable to retrieve git hash: %s", err)
	}
	return buildid, ts, hash, nil
}

// Add a new buildid and its assocatied Unix timestamp to the repo.
//
func (p *PopRepo) Add(buildid int64, ts int64) error {
	rollback := false
	defer func() {
		if !rollback {
			return
		}
		if err := p.checkout.Update(); err != nil {
			sklog.Errorf("While rolling back failed Add(): Unable to update the checkout at %q: %s", p.checkout.Dir(), err)
		}
	}()

	// Need to set GIT_COMMITTER_DATE with commit call.
	output := bytes.Buffer{}
	cmd := exec.Command{
		Name:           "git",
		Args:           []string{"commit", "-m", fmt.Sprintf("https://android-ingest.skia.org/r/%d", buildid), fmt.Sprintf("--date=%d", ts)},
		Env:            []string{fmt.Sprintf("GIT_COMMITTER_DATE=%d", ts)},
		Dir:            p.checkout.Dir(),
		CombinedOutput: &output,
	}
	if !p.local {
		cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=/home/default"))
	}

	// Also needs to confirm that the buildids are ascending, which means they should be ints.
	lastBuildID, _, _, err := p.GetLast()
	if err != nil {
		return fmt.Errorf("Couldn't get last buildid: %s", err)
	}
	if buildid <= lastBuildID {
		return fmt.Errorf("Error: buildid=%d <= lastBuildID=%d, buildid added in wrong order.", buildid, lastBuildID)
	}
	if err := ioutil.WriteFile(filepath.Join(p.checkout.Dir(), BUILDID_FILENAME), []byte(fmt.Sprintf("%d %d", buildid, ts)), 0644); err != nil {
		rollback = true
		return fmt.Errorf("Failed to write updated buildid: %s", err)
	}
	if msg, err := p.checkout.Git("add", BUILDID_FILENAME); err != nil {
		rollback = true
		return fmt.Errorf("Failed to add updated file %q: %s", msg, err)
	}
	if err := cmd.Run(); err != nil {
		rollback = true
		return fmt.Errorf("Failed to commit updated file %q: %s", output.String(), err)
	}
	fmt.Printf("git commit: %q", output.String())
	if msg, err := p.checkout.Git("push", "origin", "master"); err != nil {
		rollback = true
		return fmt.Errorf("Failed to push updated checkout %q: %s", msg, err)
	}

	return nil
}

// Verify that PopRepo implements PopRepoI.
var _ PopRepoI = (*PopRepo)(nil)