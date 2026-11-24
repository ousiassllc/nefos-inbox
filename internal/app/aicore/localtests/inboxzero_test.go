package localtests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
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
