package syncertests

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/domain/model"
	"github.com/ousiassllc/nefos-inbox/internal/gmail"
	"github.com/ousiassllc/nefos-inbox/internal/synctest"
)

const notifyFailedMsg = "notify failed"

// 件名・差出人を含むトースト基盤のエラーを返す Notifier で、新着 1 通（t1）の周期を回す。
func runCycleWithNotifyErr(t *testing.T, notifyErr error) (*harness, *lockedBuf) {
	t.Helper()
	h, buf := newLoggedHarness(t)
	h.account("a", true, synced("100"))
	knownLabels(h, "a")
	inboxUnread := []string{"INBOX", "UNREAD"}
	m := msg("m1", "t1", inboxUnread...)
	h.gm.Edit("a", func(a *synctest.Script) {
		a.History = gmail.HistoryPage{
			LatestHistoryID:  "105",
			ChangedThreadIDs: []string{"t1"},
			Added:            []gmail.HistoryAdded{{MessageID: "m1", ThreadID: "t1", LabelIDs: inboxUnread}},
		}
		a.Threads["t1"] = thread("t1", m)
	})
	h.nt.Err = notifyErr
	h.s.Start("a")
	h.waitActive("a", 1)
	return h, buf
}

// 通知の送信が失敗しても、ログには種別（kind）だけを残し、件名・差出人を出さない（NF-S10）。
func TestNotifyFailureLogsOnlyKindNotSubjectOrSender(t *testing.T) {
	const subject, fromName, fromEmail = "Subject t1", "Alice", "Alice@Example.com"
	toastErr := fmt.Errorf("toast: show %q from %s <%s>", subject, fromName, fromEmail)
	h, buf := runCycleWithNotifyErr(t, toastErr)

	require.Equal(t, []string{"t1"}, noticeThreads(h), "通知は試みられている")
	recs := buf.records(notifyFailedMsg)
	require.Len(t, recs, 1)
	assert.Equal(t, "WARN", recs[0]["level"])
	assert.Equal(t, "a", recs[0]["account"])
	assert.Equal(t, "other", recs[0]["kind"])
	assert.NotContains(t, recs[0], "err", "エラー本体は記録しない")

	out := buf.String()
	for _, secret := range []string{subject, fromName, fromEmail, "toast: show"} {
		assert.NotContains(t, out, secret, "ログのどこにも出さない")
	}
}

// 通知が失敗しても、同じ周期で連絡先の追加・トレイの未読・sync:updated は行われる（通知は at-most-once で、失敗しても同期を続ける）。
func TestNotifyFailureDoesNotStopRestOfFlush(t *testing.T) {
	h, _ := runCycleWithNotifyErr(t, errors.New("toast unavailable"))

	assert.Equal(t, "105", h.st.State("a").HistoryID, "同期は進む")
	h.st.Mu.Lock()
	contacts := append([]model.Address(nil), h.st.Contacts["a"]...)
	h.st.Mu.Unlock()
	assert.Contains(t, contacts, model.Address{Name: "Alice", Email: "alice@example.com"})
	unread, ok := h.nt.LastUnread()
	assert.True(t, ok, "トレイの未読を更新する")
	assert.True(t, unread)
	assert.Equal(t, [][]string{{"t1"}}, h.ev.Updated("a"))
}

// 通知エラーの種別は errKind で固定の語に丸められる（ラップされていても判別する）。
func TestNotifyFailureKind(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"canceled", fmt.Errorf("toast: %w", context.Canceled), "canceled"},
		{"timeout", fmt.Errorf("toast: %w", context.DeadlineExceeded), "timeout"},
		{"other", errors.New("toast: boom"), "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, buf := runCycleWithNotifyErr(t, tc.err)

			recs := buf.records(notifyFailedMsg)
			require.Len(t, recs, 1)
			assert.Equal(t, tc.want, recs[0]["kind"])
		})
	}
}
