package silence

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.skia.org/infra/am/go/note"
	"go.skia.org/infra/go/ds"
	"go.skia.org/infra/go/ds/testutil"
	"go.skia.org/infra/go/paramtools"
	"go.skia.org/infra/go/testutils"
)

func TestStore(t *testing.T) {
	testutils.LargeTest(t)

	cleanup := testutil.InitDatastore(t, ds.SILENCE_AM)
	defer cleanup()

	st := NewStore(ds.DS)
	s := &Silence{
		User: "fred@example.org",
		ParamSet: paramtools.ParamSet{
			"alertname": []string{"BotQuarantined"},
			"bot":       []string{"skia-rpi-104", "skia-rpi-114"},
		},
		Created:  time.Now().Unix(),
		Duration: "2h",
	}

	// Add a Silence.
	var err error
	s, err = st.Put(s)
	assert.NoError(t, err)
	assert.True(t, s.Active)
	assert.NotEqual(t, "", s.Key)

	all, err := st.GetAll()
	assert.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "fred@example.org", all[0].User)

	// Add a note.
	s, err = st.AddNote(s.Key, note.Note{
		Text:   "Stuff happened.",
		Author: "fred@example.com",
		TS:     time.Now().Unix(),
	})
	assert.NoError(t, err)
	assert.Equal(t, "Stuff happened.", s.Notes[0].Text)

	// Fail to add note, bad key.
	_, err = st.AddNote("badkey", note.Note{})
	assert.Error(t, err)

	// Delete note, bad index.
	_, err = st.DeleteNote(s.Key, 1)
	assert.Error(t, err)

	// Delete note.
	s, err = st.DeleteNote(s.Key, 0)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(s.Notes))

	archived, err := st.GetRecentlyArchived()
	assert.NoError(t, err)
	assert.Len(t, archived, 0)

	s, err = st.Archive(s.Key)
	assert.NoError(t, err)
	assert.False(t, s.Active)

	all, err = st.GetAll()
	assert.NoError(t, err)
	assert.Len(t, all, 0)

	archived, err = st.GetRecentlyArchived()
	assert.NoError(t, err)
	assert.Len(t, archived, 1)
	assert.Equal(t, "fred@example.org", archived[0].User)

	reactivated, err := st.Reactivate(archived[0].Key, "wilma@example.org")
	assert.NoError(t, err)
	assert.True(t, reactivated.Active)
	assert.Len(t, reactivated.Notes, 1)
	assert.Equal(t, "wilma@example.org", reactivated.Notes[0].Author)
}
