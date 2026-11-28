package apptest

import (
	"context"
	"sync"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// FakeLocalAI は localai.Runtime のフェイク。Acquire・release・各操作の呼び出しを記録し、Err～で失敗を差し込める。実際の Ollama は起動しない。
type FakeLocalAI struct {
	mu sync.Mutex

	// St は Status が返す状態。BaseURL は Acquire が返す送信先（OpenAI 互換。テストサーバーの URL を入れる）。
	St      localai.Status
	BaseURL string
	// Err は各操作が返すエラー（nil なら成功）。Acquire は AcquireErr。
	StartInstallErr, StartPullErr, DeleteErr, StopErr, RemoveErr, AcquireErr error

	acquires, releases int
	deadline           time.Time // 最後の Acquire に渡された ctx の期限（無ければゼロ）
	Pulled, Deleted    []string
	installs, cancels  int
}

var _ localai.Runtime = (*FakeLocalAI)(nil)

// NewFakeLocalAI は使える・導入済み・モデル 1 つ取得済みの状態のフェイクを返す。
func NewFakeLocalAI(baseURL string) *FakeLocalAI {
	models := make([]localai.Model, len(localai.Catalog))
	for i, m := range localai.Catalog {
		models[i] = localai.Model{CatalogModel: m, Installed: i == 0}
	}
	return &FakeLocalAI{BaseURL: baseURL, St: localai.Status{
		Supported: true, Installed: true, Version: localai.RuntimeVersion, DownloadBytes: localai.RuntimeSize, Models: models,
	}}
}

// Status は St を返す。
func (f *FakeLocalAI) Status() localai.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.St
}

// SetStatus は Status が返す状態を差し替える。
func (f *FakeLocalAI) SetStatus(st localai.Status) {
	f.mu.Lock()
	f.St = st
	f.mu.Unlock()
}

func (f *FakeLocalAI) StartInstall() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.installs++
	return f.StartInstallErr
}

func (f *FakeLocalAI) StartPull(model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Pulled = append(f.Pulled, model)
	return f.StartPullErr
}

func (f *FakeLocalAI) CancelJob() {
	f.mu.Lock()
	f.cancels++
	f.mu.Unlock()
}

func (f *FakeLocalAI) DeleteModel(_ context.Context, model string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Deleted = append(f.Deleted, model)
	return f.DeleteErr
}

func (f *FakeLocalAI) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.StopErr
}

func (f *FakeLocalAI) Remove(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.RemoveErr
}

// Acquire は BaseURL を返し、呼び出しと ctx の期限を記録する。release は 1 回ごとに数える。
func (f *FakeLocalAI) Acquire(ctx context.Context) (string, func(), error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deadline, _ = ctx.Deadline()
	if f.AcquireErr != nil {
		return "", nil, f.AcquireErr
	}
	f.acquires++
	return f.BaseURL, func() {
		f.mu.Lock()
		f.releases++
		f.mu.Unlock()
	}, nil
}

// Acquires・Releases は Acquire が成功した回数と、release が呼ばれた回数。
func (f *FakeLocalAI) Acquires() int { f.mu.Lock(); defer f.mu.Unlock(); return f.acquires }
func (f *FakeLocalAI) Releases() int { f.mu.Lock(); defer f.mu.Unlock(); return f.releases }

// AcquireDeadline は最後の Acquire に渡された ctx の期限までの残り時間（期限が無ければ 0）。
func (f *FakeLocalAI) AcquireDeadline() time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deadline.IsZero() {
		return 0
	}
	return time.Until(f.deadline)
}

// Installs・Cancels は StartInstall・CancelJob が呼ばれた回数。
func (f *FakeLocalAI) Installs() int { f.mu.Lock(); defer f.mu.Unlock(); return f.installs }
func (f *FakeLocalAI) Cancels() int  { f.mu.Lock(); defer f.mu.Unlock(); return f.cancels }
