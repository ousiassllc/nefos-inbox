package localtests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/aicore"
	"github.com/ousiassllc/nefos-inbox/internal/app/apptest"
	"github.com/ousiassllc/nefos-inbox/internal/app/composecore/composetest"
	"github.com/ousiassllc/nefos-inbox/internal/app/deps"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
	"github.com/ousiassllc/nefos-inbox/internal/store"
)

var (
	requireCode  = composetest.RequireCode
	requireError = composetest.RequireError
)

const testAPIKey = "sk-local-secret" //nolint:gosec // テスト用のダミー値

// recProvider は ai.Provider のフェイク。呼び出しの送信先・モデル・キーと、呼び出しの ctx の期限までの残りを記録する。
type recProvider struct {
	mu        sync.Mutex
	made      []made
	remaining []time.Duration
	fn        func(ctx context.Context) (string, error)
}

type made struct{ provider, model, baseURL, key string }

func (p *recProvider) record(ctx context.Context) (string, error) {
	p.mu.Lock()
	d, ok := ctx.Deadline()
	if ok {
		p.remaining = append(p.remaining, time.Until(d))
	}
	fn := p.fn
	p.mu.Unlock()
	if fn != nil {
		return fn(ctx)
	}
	return "ok", nil
}

func (p *recProvider) GenerateReply(ctx context.Context, _ ai.ReplyInput) (string, error) {
	return p.record(ctx)
}

func (p *recProvider) SummarizeThread(ctx context.Context, _ ai.ThreadInput) (string, error) {
	return p.record(ctx)
}

func (p *recProvider) AskAboutThread(ctx context.Context, _ ai.ThreadQuestionInput) (string, error) {
	return p.record(ctx)
}

// fixture は Windows 相当（SupportsLocalAI）の Deps、フェイクの Runtime、アカウント 1 つとスレッド t1 を持つ環境。
type fixture struct {
	*apptest.Env
	svc      *aicore.Service
	local    *apptest.FakeLocalAI
	provider *recProvider
	ref      dto.ThreadRef
}

func newFixture(t *testing.T, opts ...aicore.Option) *fixture {
	t.Helper()
	e := apptest.NewEnv(t)
	a := e.AddAccount(t, "me@a.example")
	e.PutThread(t, a.ID, "t1", apptest.Msg("m1", "t1", apptest.Epoch, "INBOX"))
	f := &fixture{Env: e, local: apptest.NewFakeLocalAI("http://127.0.0.1:50000/v1"), provider: &recProvider{}, ref: dto.ThreadRef{AccountID: a.ID, ThreadID: "t1"}}
	e.Deps.LocalAI = f.local
	e.Deps.AI = func(provider, model, baseURL, key string) (ai.Provider, error) {
		f.provider.mu.Lock()
		f.provider.made = append(f.provider.made, made{provider, model, baseURL, key})
		f.provider.mu.Unlock()
		return f.provider, nil
	}
	f.svc = aicore.New((*deps.Deps)(e.Deps), nil, opts...)
	return f
}

// selectLocal は「要約・質問」（threadAi）にローカルAIを選ぶ。
func (f *fixture) selectLocal(t *testing.T) {
	t.Helper()
	_, err := f.svc.SetAIFeature(t.Context(), dto.AIFeatureThreadAI, "local", "gemma4:e2b", "")
	require.NoError(t, err)
}

func (f *fixture) summarize(t *testing.T) error {
	t.Helper()
	_, err := f.svc.SummarizeThread(t.Context(), f.ref, "r1")
	return err
}

func (f *fixture) setCaps(c capability.Capabilities) { f.Deps.Capabilities = c }

func (f *fixture) storedConnection(t *testing.T) string {
	t.Helper()
	v, _, err := f.Store.GetSetting(t.Context(), store.SettingAIConnection)
	require.NoError(t, err)
	return v
}

func aicoreWithLocalTimeout(d time.Duration) aicore.Option { return aicore.WithLocalTimeout(d) }

// ErrStart は Ollama を起動できなかったエラー（テスト用）。
var ErrStart = errors.New("start failed")
