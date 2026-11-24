package localtests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
	"github.com/ousiassllc/nefos-inbox/internal/localai"
	"github.com/ousiassllc/nefos-inbox/internal/store"
)

// managed は呼び出しの前に Acquire で Ollama の送信先を得て、API キーは使わず、終わったら release する。タイムアウトは 300 秒。
func TestManagedGenerationAcquiresAndReleases(t *testing.T) {
	f := newFixture(t)
	f.selectLocal(t)
	_, err := f.svc.SetAIApiKey(t.Context(), "local", testAPIKey) // external 用の任意キー。managed では送らない
	require.NoError(t, err)

	require.NoError(t, f.summarize(t))
	assert.Equal(t, []made{{"local", "gemma4:e2b", "http://127.0.0.1:50000/v1", ""}}, f.provider.made)
	assert.Equal(t, 1, f.local.Acquires())
	assert.Equal(t, 1, f.local.Releases())
	require.Len(t, f.provider.remaining, 1)
	assert.InDelta(t, (300 * time.Second).Seconds(), f.provider.remaining[0].Seconds(), 5, "ローカルAIは 300 秒")
	assert.InDelta(t, (300 * time.Second).Seconds(), f.local.AcquireDeadline().Seconds(), 5, "起動の待ちも同じ期限の中")
}

// ほかのプロバイダは 60 秒のまま（ローカルAIだけ長い）。
func TestOtherProvidersKeepSixtySeconds(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureThreadAI, "anthropic", "claude-x", "")
	require.NoError(t, err)
	_, err = f.svc.SetAIApiKey(t.Context(), "anthropic", testAPIKey)
	require.NoError(t, err)
	_, err = f.svc.RecordAIConsent(t.Context(), "anthropic", "")
	require.NoError(t, err)

	require.NoError(t, f.summarize(t))
	require.Len(t, f.provider.remaining, 1)
	assert.InDelta(t, (60 * time.Second).Seconds(), f.provider.remaining[0].Seconds(), 5)
	assert.Zero(t, f.local.Acquires(), "ローカルAIは起動しない")
}

// 生成が失敗しても release は必ず呼ぶ。
func TestReleaseIsCalledWhenGenerationFails(t *testing.T) {
	f := newFixture(t)
	f.selectLocal(t)
	f.provider.fn = func(context.Context) (string, error) { return "", ai.ErrFailed }
	requireCode(t, "AI_FAILED", f.summarize(t))
	assert.Equal(t, 1, f.local.Acquires())
	assert.Equal(t, 1, f.local.Releases())
}

// 期限（ローカルAI用に短縮）を過ぎたら AI_FAILED（再試行可）で、release は呼ぶ。
func TestLocalTimeoutIsConfigurable(t *testing.T) {
	f := newFixture(t, aicoreWithLocalTimeout(20*time.Millisecond))
	f.selectLocal(t)
	f.provider.fn = func(ctx context.Context) (string, error) { <-ctx.Done(); return "", ctx.Err() }
	ae := requireError(t, "AI_FAILED", f.summarize(t))
	assert.True(t, ae.Retryable)
	assert.Equal(t, 1, f.local.Releases())
}

// 起動できない（Acquire の失敗）は生成へ進まず、AppError にする。
func TestAcquireFailures(t *testing.T) {
	for _, c := range []struct {
		err  error
		code string
	}{
		{localai.ErrNotInstalled, "AI_NOT_CONFIGURED"},
		{localai.ErrUnsupported, "UNSUPPORTED"},
		{errors.New("start failed"), "AI_FAILED"},
	} {
		f := newFixture(t)
		f.selectLocal(t)
		f.local.AcquireErr = c.err
		requireCode(t, c.code, f.summarize(t))
		assert.Empty(t, f.provider.made, "生成へ進まない")
		assert.Zero(t, f.local.Releases())
	}
}

func TestExternalGenerationUsesBaseURLAndOptionalKey(t *testing.T) {
	f := newFixture(t)
	f.selectLocal(t)
	_, err := f.svc.SetAILocalConnection(t.Context(), "external", "http://localhost:1234/v1")
	require.NoError(t, err)

	require.NoError(t, f.summarize(t)) // キーなし・同意なし（loopback）
	assert.Equal(t, made{"local", "gemma4:e2b", "http://localhost:1234/v1", ""}, f.provider.made[0])
	assert.Zero(t, f.local.Acquires(), "external は Ollama を起動しない")

	_, err = f.svc.SetAIApiKey(t.Context(), "local", testAPIKey)
	require.NoError(t, err)
	require.NoError(t, f.summarize(t))
	assert.Equal(t, testAPIKey, f.provider.made[1].key)
}

// 前提の検査（通信・起動の前）。
func TestLocalPrecheckMatrix(t *testing.T) {
	type tc struct {
		name  string
		setup func(f *fixture)
		code  string
		msg   string
	}
	notInstalled := func(f *fixture) {
		st := f.local.Status()
		st.Installed = false
		f.local.SetStatus(st)
	}
	external := func(url string) func(*fixture) {
		return func(f *fixture) {
			_, err := f.svc.SetAILocalConnection(context.Background(), "external", url)
			require.NoError(t, err)
		}
	}
	for _, c := range []tc{
		{"managed 未導入", notInstalled, "AI_NOT_CONFIGURED", "ローカルAI（Ollama）を設定画面の「AI のつなぎ先」で導入してください。"},
		{"この OS では使えない", func(f *fixture) { f.setCaps(capability.Android()) }, "UNSUPPORTED", ""},
		{"Runtime が無い", func(f *fixture) { f.Deps.LocalAI = nil }, "UNSUPPORTED", ""},
		{"Runtime が使えない環境", func(f *fixture) {
			f.local.SetStatus(localai.Status{Supported: false, UnsupportedReason: "メモリが足りません。"})
		}, "UNSUPPORTED", "メモリが足りません。"},
		{"external の Base URL が無い", func(f *fixture) {
			f.setCaps(capability.Desktop("windows"))
			require.NoError(t, f.Store.SetSetting(context.Background(), store.SettingAIConnection, `{"localMode":"external"}`))
		}, "AI_NOT_CONFIGURED", ""},
		{"external の保存済み Base URL が不正", func(f *fixture) {
			require.NoError(t, f.Store.SetSetting(context.Background(), store.SettingAIConnection, `{"localMode":"external","localBaseUrl":"http://lm.example.com/v1"}`))
		}, "VALIDATION", ""},
		{"external で loopback 以外は未同意", external("https://lm.example.com/v1"), "AI_CONSENT_REQUIRED", ""},
	} {
		f := newFixture(t)
		f.selectLocal(t)
		c.setup(f)
		ae := requireError(t, c.code, f.summarize(t))
		if c.msg != "" {
			assert.Equal(t, c.msg, ae.Message, c.name)
		}
		assert.Empty(t, f.provider.made, c.name)
		assert.Zero(t, f.local.Acquires(), c.name)
	}
}

func TestLocalWithoutModelIsNotConfigured(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.Store.SetSetting(t.Context(), store.SettingAIFeatures, `{"threadAi":{"provider":"local"}}`))
	ae := requireError(t, "AI_NOT_CONFIGURED", f.summarize(t))
	assert.Contains(t, ae.Message, "モデル")
}

func TestExternalNonLoopbackWithConsentGenerates(t *testing.T) {
	f := newFixture(t)
	f.selectLocal(t)
	_, err := f.svc.SetAILocalConnection(t.Context(), "external", "https://lm.example.com/v1")
	require.NoError(t, err)
	_, err = f.svc.RecordAIConsent(t.Context(), "local", "https://lm.example.com/v1")
	require.NoError(t, err)
	require.NoError(t, f.summarize(t))
	assert.Equal(t, "https://lm.example.com/v1", f.provider.made[0].baseURL)
}
