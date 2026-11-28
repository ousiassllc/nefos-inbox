package aitests

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
)

func localProvider(t *testing.T, base, key string) ai.Provider {
	t.Helper()
	p, err := ai.NewWithClient(ai.ProviderLocal, "gemma4:e2b", base, key, nil)
	require.NoError(t, err)
	return p
}

// local は OpenAI 互換の Chat Completions で呼ぶ。キーが空なら Authorization ヘッダを付けない。
func TestLocalWithoutKeyOmitsAuthorization(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":" 了解です。 "},"finish_reason":"stop"}],"reasoning":"考え中"}`)
	got, err := localProvider(t, srv.URL+"/v1/", "").GenerateReply(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "了解です。", got)
	assert.Equal(t, http.MethodPost, rec.method)
	assert.Equal(t, "/v1/chat/completions", rec.path)
	assert.Equal(t, "gemma4:e2b", rec.body["model"])
	_, hasEffort := rec.body["reasoning_effort"]
	assert.False(t, hasEffort, "local はエフォートを載せない")
	_, hasAuth := rec.header["Authorization"]
	assert.False(t, hasAuth, "キーが空なら Authorization を付けない")
}

func TestLocalWithKeySendsBearer(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	_, err := localProvider(t, srv.URL, apiKey).SummarizeThread(context.Background(), ai.ThreadInput{Messages: input.Messages})
	require.NoError(t, err)
	assert.Equal(t, "/chat/completions", rec.path)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
}

// local は Base URL が必須（空の既定の送信先は無い）。openai と同じ検証（http は loopback のみ）。
func TestLocalRequiresValidBaseURL(t *testing.T) {
	_, err := ai.New(ai.ProviderLocal, "m", "", "")
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
	_, err = ai.New(ai.ProviderLocal, "m", "   ", "")
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
	_, err = ai.New(ai.ProviderLocal, "m", "http://lm.example.com/v1", "")
	require.ErrorIs(t, err, ai.ErrInsecureBaseURL)
	_, err = ai.New(ai.ProviderLocal, "", "http://localhost:11434/v1", "")
	require.Error(t, err, "モデルは必須")
	for _, base := range []string{"http://localhost:11434/v1", "http://127.0.0.1:1234/v1", "http://[::1]:11434/v1", "https://lm.example.com/v1"} {
		_, err = ai.New(ai.ProviderLocal, "m", base, "")
		require.NoError(t, err, base)
	}
}

func TestLocalEffortScaleIsEmpty(t *testing.T) {
	assert.Empty(t, ai.EffortScale(ai.ProviderLocal))
	assert.False(t, ai.ValidEffort(ai.ProviderLocal, ai.EffortLow))
}

func TestListModelsLocal(t *testing.T) {
	srv, rec := serve(t, 200, `{"object":"list","data":[{"id":"gemma4:e2b"},{"id":"gemma4:e4b"}]}`)
	got, err := ai.ListModels(context.Background(), ai.ProviderLocal, srv.URL+"/v1", "", nil)
	require.NoError(t, err)
	assert.Equal(t, []ai.ModelInfo{{ID: "gemma4:e2b"}, {ID: "gemma4:e4b"}}, got)
	assert.Equal(t, "/v1/models", rec.path)
	_, hasAuth := rec.header["Authorization"]
	assert.False(t, hasAuth, "キーが空なら Authorization を付けない")

	_, err = ai.ListModels(context.Background(), ai.ProviderLocal, srv.URL, apiKey, nil)
	require.NoError(t, err)
	assert.Equal(t, "Bearer "+apiKey, rec.header.Get("Authorization"))
}

func TestListModelsLocalNeedsBaseURL(t *testing.T) {
	_, err := ai.ListModels(context.Background(), ai.ProviderLocal, "", "", nil)
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
	_, err = ai.ListModels(context.Background(), ai.ProviderLocal, "http://lm.example.com/v1", "", nil)
	require.ErrorIs(t, err, ai.ErrInsecureBaseURL)
}

// Inbox Zero の判定（chat 分類器）も local で作れる。
func TestNewClassifierForLocal(t *testing.T) {
	srv, rec := serve(t, 200, `{"choices":[{"message":{"content":"{\"choice\":\"other\",\"confidence\":0.9}"}}]}`)
	cl, err := ai.NewClassifierFor(ai.ProviderLocal, "gemma4:e2b", srv.URL, "")
	require.NoError(t, err)
	ans, err := cl.Classify(context.Background(), ai.ClassifyInput{State: "x", Instructions: "i", Criteria: []ai.Criterion{{Name: "other", Description: "d"}}})
	require.NoError(t, err)
	assert.Equal(t, "other", ans.Choice)
	_, hasAuth := rec.header["Authorization"]
	assert.False(t, hasAuth)
	_, err = ai.NewClassifierFor(ai.ProviderLocal, "gemma4:e2b", "", "")
	require.ErrorIs(t, err, ai.ErrInvalidBaseURL)
}

func TestIsLoopbackBaseURL(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://localhost:11434/v1": true, "http://127.0.0.1:1234/v1": true, "http://[::1]:11434/v1": true, "https://localhost/v1": true,
		"https://lm.example.com/v1": false, "http://localhost.evil.com/v1": false, "http://0.0.0.0:1/v1": false, "": false, "::bad::": false,
	} {
		assert.Equal(t, want, ai.IsLoopbackBaseURL(raw), raw)
	}
}
