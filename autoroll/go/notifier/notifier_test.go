package notifier

import (
	"context"
	"fmt"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"
	"go.skia.org/infra/go/notifier"
	"go.skia.org/infra/go/testutils"
)

type msg struct {
	subject string
	m       *notifier.Message
}

type testNotifier struct {
	msgs []*msg
}

func (n *testNotifier) Send(ctx context.Context, subject string, m *notifier.Message) error {
	n.msgs = append(n.msgs, &msg{
		subject: subject,
		m:       m,
	})
	return nil
}

func TestNotifier(t *testing.T) {
	testutils.SmallTest(t)

	ctx := context.Background()
	n, err := New(ctx, "childRepo", "parentRepo", nil, nil)
	assert.NoError(t, err)
	t1 := &testNotifier{}
	n.Router().Add(t1, notifier.FILTER_DEBUG, "")

	n.SendIssueUpdate(ctx, "123", "https://codereview/123", "uploaded a CL!")
	assert.Equal(t, 1, len(t1.msgs))
	assert.Equal(t, "The childRepo into parentRepo AutoRoller has uploaded issue 123", t1.msgs[0].subject)
	assert.Equal(t, "uploaded a CL!", t1.msgs[0].m.Body)
	assert.Equal(t, notifier.SEVERITY_INFO, t1.msgs[0].m.Severity)

	n.SendModeChange(ctx, "test@skia.org", "STOPPED", "<b>Staaahhp!</b>")
	assert.Equal(t, 2, len(t1.msgs))
	assert.Equal(t, "The childRepo into parentRepo AutoRoller mode was changed", t1.msgs[1].subject)
	assert.Equal(t, "test@skia.org changed the mode to \"STOPPED\" with message: &lt;b&gt;Staaahhp!&lt;/b&gt;", t1.msgs[1].m.Body)
	assert.Equal(t, notifier.SEVERITY_WARNING, t1.msgs[1].m.Severity)

	now := time.Now().Round(time.Millisecond)
	n.SendSafetyThrottled(ctx, now)
	assert.Equal(t, 3, len(t1.msgs))
	assert.Equal(t, "The childRepo into parentRepo AutoRoller is throttled", t1.msgs[2].subject)
	assert.Equal(t, fmt.Sprintf("The roller is throttled because it attempted to upload too many CLs in too short a time.  The roller will unthrottle at %s.", now.Format(time.RFC1123)), t1.msgs[2].m.Body)
	assert.Equal(t, notifier.SEVERITY_ERROR, t1.msgs[2].m.Severity)
}
