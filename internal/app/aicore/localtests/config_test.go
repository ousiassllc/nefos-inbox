package localtests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
	"github.com/ousiassllc/nefos-inbox/internal/secrets"
)

// 既定は managed。導入済みでモデルが取得済みなら ready、同意は PC の外へ出ないので不要（consentedProviders に含める）。
func TestConfigDefaultsToManagedAndReady(t *testing.T) {
	f := newFixture(t)
	cfg, err := f.svc.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.AILocalConnection{Mode: "managed", Ready: true, Loopback: true}, cfg.Local)
	assert.Equal(t, []string{"local"}, cfg.ConsentedProviders)
	assert.Empty(t, cfg.APIKeyProviders)
}

func TestConfigManagedIsNotReadyUntilInstalledWithAModel(t *testing.T) {
	f := newFixture(t)
	st := f.local.Status()
	for i := range st.Models {
		st.Models[i].Installed = false
	}
	f.local.SetStatus(st)
	cfg, err := f.svc.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.False(t, cfg.Local.Ready, "モデルが 1 つも無い")

	st.Installed = false
	st.Models[0].Installed = true
	f.local.SetStatus(st)
	cfg, err = f.svc.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.False(t, cfg.Local.Ready, "未導入")
}

// capability が無い OS でも local を返す（ready は false。同意の欄にも含めない）。
func TestConfigOnUnsupportedOS(t *testing.T) {
	f := newFixture(t)
	f.setCaps(capability.Android())
	cfg, err := f.svc.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.Equal(t, dto.AILocalConnection{Mode: "managed", Loopback: true}, cfg.Local)
	assert.Empty(t, cfg.ConsentedProviders)

	f.setCaps(capability.Desktop("windows"))
	f.Deps.LocalAI = nil
	cfg, err = f.svc.GetAIConfig(t.Context())
	require.NoError(t, err)
	assert.False(t, cfg.Local.Ready, "managed は Runtime が無ければ使えない")
	assert.Empty(t, cfg.ConsentedProviders)
}

func TestSetAILocalConnection(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()

	cfg, err := f.svc.SetAILocalConnection(ctx, "external", " http://localhost:11434/v1 ")
	require.NoError(t, err)
	assert.Equal(t, dto.AILocalConnection{Mode: "external", BaseURL: "http://localhost:11434/v1", Ready: true, Loopback: true}, cfg.Local)
	assert.Equal(t, []string{"local"}, cfg.ConsentedProviders, "loopback は同意不要")
	assert.JSONEq(t, `{"localMode":"external","localBaseUrl":"http://localhost:11434/v1"}`, f.storedConnection(t))

	// managed へ戻すと Base URL は保存しない。
	cfg, err = f.svc.SetAILocalConnection(ctx, "managed", "http://ignored.example/v1")
	require.NoError(t, err)
	assert.Equal(t, "managed", cfg.Local.Mode)
	assert.Empty(t, cfg.Local.BaseURL)
	assert.NotContains(t, f.storedConnection(t), "localBaseUrl")
}

func TestSetAILocalConnectionValidation(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	for _, c := range []struct{ mode, url string }{
		{"", ""}, {"remote", "http://localhost:1/v1"}, {"external", ""}, {"external", "   "},
		{"external", "http://lm.example.com/v1"}, {"external", "https://u:p@lm.example.com/v1"}, {"external", "https://lm.example.com/v1?k=1"}, {"external", "ftp://lm"},
	} {
		_, err := f.svc.SetAILocalConnection(ctx, c.mode, c.url)
		ae := requireError(t, "VALIDATION", err)
		assert.NotContains(t, ae.Message, "u:p")
	}
	assert.Empty(t, f.storedConnection(t), "不正な値は保存しない")

	f.setCaps(capability.Android())
	_, err := f.svc.SetAILocalConnection(ctx, "managed", "")
	requireCode(t, "UNSUPPORTED", err)
}

// external で loopback 以外（https の別ホスト）は同意が要る。同意は送信先ごと。送信先を変えると無効になる。
func TestExternalNonLoopbackNeedsConsent(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cfg, err := f.svc.SetAILocalConnection(ctx, "external", "https://lm.example.com/v1")
	require.NoError(t, err)
	assert.True(t, cfg.Local.Ready)
	assert.False(t, cfg.Local.Loopback, "https の別ホストは loopback ではない（画面は同意の欄を出す）")
	assert.Empty(t, cfg.ConsentedProviders)

	_, err = f.svc.RecordAIConsent(ctx, "local", "")
	requireCode(t, "VALIDATION", err)
	cfg, err = f.svc.RecordAIConsent(ctx, "local", "https://lm.example.com/v1")
	require.NoError(t, err)
	assert.Equal(t, []string{"local"}, cfg.ConsentedProviders)

	cfg, err = f.svc.SetAILocalConnection(ctx, "external", "https://other.example.com/v1")
	require.NoError(t, err)
	assert.Empty(t, cfg.ConsentedProviders, "送信先が変わると同意は無効")
}

func TestLocalAPIKeyIsOptional(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	cfg, err := f.svc.SetAIApiKey(ctx, "local", "  "+testAPIKey+" ")
	require.NoError(t, err)
	assert.Equal(t, []string{"local"}, cfg.APIKeyProviders)
	got, err := f.Secrets.Get(secrets.AIAPIKeyKey("local"))
	require.NoError(t, err)
	assert.Equal(t, testAPIKey, got)

	cfg, err = f.svc.ClearAIApiKey(ctx, "local")
	require.NoError(t, err)
	assert.Empty(t, cfg.APIKeyProviders)
}

func TestSetAIFeatureLocal(t *testing.T) {
	f := newFixture(t)
	ctx := t.Context()
	for _, feature := range []string{dto.AIFeatureDraft, dto.AIFeatureThreadAI, dto.AIFeatureInboxZero} {
		cfg, err := f.svc.SetAIFeature(ctx, feature, "local", " gemma4:e2b ", "")
		require.NoError(t, err, feature)
		assert.Contains(t, []dto.AIFeatureSetting{cfg.Features.Draft, cfg.Features.ThreadAI, cfg.Features.InboxZero}, dto.AIFeatureSetting{Provider: "local", Model: "gemma4:e2b"})
	}
	_, err := f.svc.SetAIFeature(ctx, dto.AIFeatureDraft, "local", "gemma4:e2b", "low")
	requireCode(t, "VALIDATION", err) // エフォートは空のみ
	_, err = f.svc.SetAIFeature(ctx, dto.AIFeatureDraft, "local", "", "")
	requireCode(t, "VALIDATION", err)

	f.setCaps(capability.Android())
	_, err = f.svc.SetAIFeature(ctx, dto.AIFeatureDraft, "local", "gemma4:e2b", "")
	requireCode(t, "VALIDATION", err)
}

// loopback は Go の判定（厳密一致）と同じ。大文字・省略形・10 進・展開形は loopback ではない（http は保存できないので https で確かめる）。
func TestLoopbackFlagMatchesGo(t *testing.T) {
	f := newFixture(t)
	for url, want := range map[string]bool{
		"http://localhost:11434/v1": true, "http://127.0.0.1:1/v1": true, "http://[::1]:1/v1": true,
		"https://LOCALHOST:1/v1": false, "https://127.1:1/v1": false, "https://2130706433:1/v1": false, "https://[0:0:0:0:0:0:0:1]:1/v1": false,
	} {
		cfg, err := f.svc.SetAILocalConnection(t.Context(), "external", url)
		require.NoError(t, err, url)
		assert.Equal(t, want, cfg.Local.Loopback, url)
	}
}
