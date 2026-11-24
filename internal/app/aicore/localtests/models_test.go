package localtests

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
)

type listCall struct{ provider, baseURL, key string }

// listRecorder は Deps.AIModels を差し替え、呼び出しを記録する。
type listRecorder struct {
	mu    sync.Mutex
	calls []listCall
	err   error
}

func (f *fixture) recordList(models ...ai.ModelInfo) *listRecorder {
	r := &listRecorder{}
	f.Deps.AIModels = func(_ context.Context, provider, baseURL, key string) ([]ai.ModelInfo, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, listCall{provider, baseURL, key})
		return models, r.err
	}
	return r
}

// managed（baseURL が空）で未導入は、通信も起動もせず not_installed。
func TestListLocalModelsManagedNotInstalled(t *testing.T) {
	f := newFixture(t)
	r := f.recordList()
	st := f.local.Status()
	st.Installed = false
	f.local.SetStatus(st)

	list, err := f.svc.ListAIModels(t.Context(), "local", "")
	require.NoError(t, err)
	assert.Equal(t, "not_installed", list.Reason)
	assert.NotNil(t, list.Models)
	assert.Empty(t, list.Models)
	assert.NotEmpty(t, list.Message)
	assert.Empty(t, r.calls, "通信しない")
	assert.Zero(t, f.local.Acquires(), "起動しない")
}

// managed で導入済みは Acquire で起動して一覧を取り、release する。キーは付けない。
func TestListLocalModelsManagedAcquires(t *testing.T) {
	f := newFixture(t)
	r := f.recordList(ai.ModelInfo{ID: "gemma4:e2b"}, ai.ModelInfo{ID: "gemma4:e4b"})
	_, err := f.svc.SetAIApiKey(t.Context(), "local", testAPIKey)
	require.NoError(t, err)

	list, err := f.svc.ListAIModels(t.Context(), "local", "")
	require.NoError(t, err)
	assert.Empty(t, list.Reason)
	require.Len(t, list.Models, 2)
	assert.Equal(t, "gemma4:e2b", list.Models[0].ID)
	assert.Equal(t, []listCall{{"local", "http://127.0.0.1:50000/v1", ""}}, r.calls)
	assert.Equal(t, 1, f.local.Acquires())
	assert.Equal(t, 1, f.local.Releases())
}

func TestListLocalModelsManagedStartFailure(t *testing.T) {
	f := newFixture(t)
	f.recordList()
	f.local.AcquireErr = errors.New("start failed")
	list, err := f.svc.ListAIModels(t.Context(), "local", "")
	require.NoError(t, err)
	assert.NotEmpty(t, list.Reason)
	assert.Empty(t, list.Models)
	assert.Zero(t, f.local.Releases())
}

// external は渡された baseURL から取る（接続テスト）。キーがあれば付け、Ollama は起動しない。
func TestListLocalModelsExternal(t *testing.T) {
	f := newFixture(t)
	r := f.recordList(ai.ModelInfo{ID: "llama3"})
	list, err := f.svc.ListAIModels(t.Context(), "local", " http://localhost:1234/v1 ")
	require.NoError(t, err)
	require.Len(t, list.Models, 1)
	assert.Equal(t, []listCall{{"local", "http://localhost:1234/v1", ""}}, r.calls)

	_, err = f.svc.SetAIApiKey(t.Context(), "local", testAPIKey)
	require.NoError(t, err)
	_, err = f.svc.ListAIModels(t.Context(), "local", "http://localhost:1234/v1")
	require.NoError(t, err)
	assert.Equal(t, testAPIKey, r.calls[1].key)
	assert.Zero(t, f.local.Acquires())
}

// external で loopback 以外は、同意前は取得も送らず consent_required。不正な URL は invalid_base_url。
func TestListLocalModelsExternalConsentAndInvalid(t *testing.T) {
	f := newFixture(t)
	r := f.recordList(ai.ModelInfo{ID: "m"})
	list, err := f.svc.ListAIModels(t.Context(), "local", "https://lm.example.com/v1")
	require.NoError(t, err)
	assert.Equal(t, "consent_required", list.Reason)
	list, err = f.svc.ListAIModels(t.Context(), "local", "http://lm.example.com/v1")
	require.NoError(t, err)
	assert.Equal(t, "invalid_base_url", list.Reason)
	assert.Empty(t, r.calls)

	_, err = f.svc.RecordAIConsent(t.Context(), "local", "https://lm.example.com/v1")
	require.NoError(t, err)
	list, err = f.svc.ListAIModels(t.Context(), "local", "https://lm.example.com/v1")
	require.NoError(t, err)
	assert.Empty(t, list.Reason)
	assert.Len(t, list.Models, 1)
}

func TestListLocalModelsUnsupportedOS(t *testing.T) {
	f := newFixture(t)
	f.setCaps(capability.Android())
	_, err := f.svc.ListAIModels(t.Context(), "local", "")
	requireCode(t, "UNSUPPORTED", err)
}
