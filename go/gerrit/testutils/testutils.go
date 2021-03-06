package testutils

import (
	"encoding/json"
	"fmt"
	"path"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/buildbucket"
	"go.skia.org/infra/go/gerrit"
	"go.skia.org/infra/go/mockhttpclient"
	"go.skia.org/infra/go/testutils"
)

const (
	FAKE_GERRIT_URL = "https://fake-skia-review.googlesource.com"
	FAKE_GITCOOKIES = ".googlesource.com\tTRUE\t/\tTRUE\t123\to\tgit-user.google.com=abc123"
)

// MockGerrit is a GerritInterface implementation which mocks out requests to
// the server.
type MockGerrit struct {
	Gerrit    *gerrit.Gerrit
	Mock      *mockhttpclient.URLMock
	isAndroid bool
	t         testutils.TestingT
}

// NewGerrit returns a mocked Gerrit instance.
func NewGerrit(t testutils.TestingT, workdir string, isAndroid bool) *MockGerrit {
	gitcookies := path.Join(workdir, "gitcookies_fake")
	testutils.WriteFile(t, gitcookies, FAKE_GITCOOKIES)

	mock := mockhttpclient.NewURLMock()
	g, err := gerrit.NewGerrit(FAKE_GERRIT_URL, gitcookies, mock.Client())
	assert.NoError(t, err)
	return &MockGerrit{
		Gerrit:    g,
		Mock:      mock,
		isAndroid: isAndroid,
		t:         t,
	}
}

func (g *MockGerrit) AssertEmpty() {
	assert.True(g.t, g.Mock.Empty())
}

func (g *MockGerrit) MockGetIssueProperties(ci *gerrit.ChangeInfo) {
	url := fmt.Sprintf(gerrit.URL_TMPL_CHANGE, ci.Issue)
	// TODO(borenet): How do we decide whether we should be using
	// authenticated GETs?
	if false {
		url = "/a" + url
	}
	url = FAKE_GERRIT_URL + url

	serialized, err := json.Marshal(ci)
	assert.NoError(g.t, err)
	serialized = append([]byte(")]}'\n"), serialized...)
	g.Mock.MockOnce(url, mockhttpclient.MockGetDialogue(serialized))
}

func (g *MockGerrit) MockGetTrybotResults(ci *gerrit.ChangeInfo, results []*buildbucket.Build) {
	url := fmt.Sprintf("https://cr-buildbucket.appspot.com/api/buildbucket/v1/search?tag=buildset%%3Apatch%%2Fgerrit%%2Ffake-skia-review.googlesource.com%%2F%d%%2F1", ci.Issue)
	serialized, err := json.Marshal(struct {
		Builds []*buildbucket.Build
	}{
		Builds: results,
	})
	assert.NoError(g.t, err)
	g.Mock.MockOnce(url, mockhttpclient.MockGetDialogue(serialized))
}

func (g *MockGerrit) MakePostRequest(ci *gerrit.ChangeInfo, msg string, labels map[string]int) (string, []byte) {
	url := fmt.Sprintf("%s/a/changes/%d/revisions/%d/review", FAKE_GERRIT_URL, ci.Issue, len(ci.Revisions))
	if labels == nil {
		labels = map[string]int{}
	}
	req := struct {
		Labels  map[string]int `json:"labels"`
		Message string         `json:"message"`
	}{
		Labels:  labels,
		Message: msg,
	}
	reqBytes := testutils.MarshalJSON(g.t, req)
	return url, []byte(reqBytes)
}

func (g *MockGerrit) MockPost(ci *gerrit.ChangeInfo, msg string, labels map[string]int) {
	url, reqBytes := g.MakePostRequest(ci, msg, labels)
	g.Mock.MockOnce(url, mockhttpclient.MockPostDialogue("application/json", reqBytes, []byte("")))
}

func (g *MockGerrit) MockAddComment(ci *gerrit.ChangeInfo, msg string) {
	g.MockPost(ci, msg, nil)
}

func (g *MockGerrit) MockSetDryRun(ci *gerrit.ChangeInfo, msg string) {
	if g.isAndroid {
		g.MockPost(ci, msg, map[string]int{
			gerrit.AUTOSUBMIT_LABEL: gerrit.AUTOSUBMIT_LABEL_NONE,
		})
	} else {
		g.MockPost(ci, msg, map[string]int{
			gerrit.COMMITQUEUE_LABEL: gerrit.COMMITQUEUE_LABEL_DRY_RUN,
		})
	}
}

func (g *MockGerrit) MockSetCQ(ci *gerrit.ChangeInfo, msg string) {
	if g.isAndroid {
		g.MockPost(ci, msg, map[string]int{
			gerrit.AUTOSUBMIT_LABEL: gerrit.AUTOSUBMIT_LABEL_SUBMIT,
		})
	} else {
		g.MockPost(ci, msg, map[string]int{
			gerrit.COMMITQUEUE_LABEL: gerrit.COMMITQUEUE_LABEL_SUBMIT,
		})
	}
}

func (g *MockGerrit) Abandon(ci *gerrit.ChangeInfo, msg string) {
	url := fmt.Sprintf("%s/a/changes/%d/abandon", FAKE_GERRIT_URL, ci.Issue)
	req := struct {
		Message string `json:"message"`
	}{
		Message: msg,
	}
	reqBytes := testutils.MarshalJSON(g.t, req)
	g.Mock.MockOnce(url, mockhttpclient.MockPostDialogue("application/json", []byte(reqBytes), []byte("")))
}
