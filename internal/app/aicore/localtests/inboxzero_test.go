package localtests

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/apptest"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
)

// Inbox Zero の判定もローカルAIを使える。1 件ごとに Acquire・release し、期限は 300 秒、managed ではキーを使わない。
func TestInboxZeroJudgesWithManagedLocalAI(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "local", "gemma4:e2b", "")
	require.NoError(t, err)
	_, err = f.svc.SetInboxZeroReasons(t.Context(), dto.InboxZeroReasons{Ad: true})
	require.NoError(t, err)

	var remaining time.Duration
	f.Classifier.Fn = func(ctx context.Context, _ ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		d, _ := ctx.Deadline()
		remaining = time.Until(d)
		return ai.ClassifyAnswer{Choice: "ad", Probabilities: map[string]float64{"ad": 0.95}}, nil
	}
	res, err := f.svc.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{f.ref}, "iz-1")
	require.NoError(t, err)
	require.Len(t, res.Candidates, 1)
	assert.Equal(t, "ad", res.Candidates[0].Reason)
	assert.Equal(t, 1, f.local.Acquires())
	assert.Equal(t, 1, f.local.Releases())
	assert.InDelta(t, (300 * time.Second).Seconds(), remaining.Seconds(), 5)
	require.Len(t, f.Classifier.Made, 1)
	assert.Equal(t, "local", f.Classifier.Made[0].Provider)
	assert.Equal(t, "http://127.0.0.1:50000/v1", f.Classifier.Made[0].BaseURL)
	assert.Equal(t, []string{""}, f.Classifier.Keys)
}

// 前提（未導入）は通信・起動の前に止める。
func TestInboxZeroLocalNotInstalled(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "local", "gemma4:e2b", "")
	require.NoError(t, err)
	_, err = f.svc.SetInboxZeroReasons(t.Context(), dto.InboxZeroReasons{Ad: true})
	require.NoError(t, err)
	st := f.local.Status()
	st.Installed = false
	f.local.SetStatus(st)
	_, err = f.svc.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{f.ref}, "iz-1")
	requireCode(t, "AI_NOT_CONFIGURED", err)
	assert.Zero(t, f.local.Acquires())
	assert.Zero(t, f.Classifier.Calls())
}

// 判定の途中で導入が外れた（Acquire が ErrNotInstalled）ときは、どのスレッドでも同じ結果なので全体を止める。
func TestInboxZeroLocalAcquireFailureAborts(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "local", "gemma4:e2b", "")
	require.NoError(t, err)
	_, err = f.svc.SetInboxZeroReasons(t.Context(), dto.InboxZeroReasons{Ad: true})
	require.NoError(t, err)
	f.local.AcquireErr = ErrStart
	_, err = f.svc.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{f.ref}, "iz-1")
	requireCode(t, "AI_FAILED", err)
	assert.Zero(t, f.Classifier.Calls())
}

// ローカルAIの判定は 1 件ずつ（ほかのプロバイダは同時に 4 件まで）。
func TestInboxZeroLocalJudgesOneAtATime(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "local", "gemma4:e2b", "")
	require.NoError(t, err)
	_, err = f.svc.SetInboxZeroReasons(t.Context(), dto.InboxZeroReasons{Ad: true})
	require.NoError(t, err)
	refs := []dto.ThreadRef{f.ref}
	for i := range 7 {
		id := fmt.Sprintf("x%d", i)
		f.PutThread(t, f.ref.AccountID, id, apptest.Msg("m"+id, id, apptest.Epoch, "INBOX"))
		refs = append(refs, dto.ThreadRef{AccountID: f.ref.AccountID, ThreadID: id})
	}

	var cur, peak atomic.Int32
	f.Classifier.Fn = func(context.Context, ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		n := cur.Add(1)
		defer cur.Add(-1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		for range 200 { // 重なる機会を与える（固定時間の待ちは使わない）
			runtime.Gosched()
		}
		return ai.ClassifyAnswer{Choice: "other", Probabilities: map[string]float64{"other": 0.9}}, nil
	}
	_, err = f.svc.FindInboxZeroCandidates(t.Context(), refs, "iz-1")
	require.NoError(t, err)
	assert.Equal(t, 8, f.Classifier.Calls())
	assert.Equal(t, int32(1), peak.Load(), "ローカルAIは同時に 1 件")
}

// サーバーに接続できない（net.Error）ときは、どのスレッドでも同じ結果なので全体を止める。
func TestInboxZeroLocalConnectionRefusedAborts(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureInboxZero, "local", "gemma4:e2b", "")
	require.NoError(t, err)
	_, err = f.svc.SetInboxZeroReasons(t.Context(), dto.InboxZeroReasons{Ad: true})
	require.NoError(t, err)
	f.Classifier.Fn = func(context.Context, ai.ClassifyInput) (ai.ClassifyAnswer, error) {
		return ai.ClassifyAnswer{}, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	}
	_, err = f.svc.FindInboxZeroCandidates(t.Context(), []dto.ThreadRef{f.ref}, "iz-1")
	ae := requireError(t, "AI_FAILED", err)
	assert.Equal(t, "ローカルAIのサーバーに接続できませんでした。起動しているか確かめてください。", ae.Message)
}
