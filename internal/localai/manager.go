package localai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// errClosed は Close 済みの Manager に仕事を頼んだとき。
var errClosed = errors.New("ローカルAIの管理は終了しています。")

// Manager は管理する Ollama（導入・起動・停止・モデルの取得と削除）。Runtime を実装する。
// すべてのメソッドは複数の goroutine から呼んでよい。
type Manager struct {
	cfg       resolved
	supported bool
	reason    string

	baseCtx    context.Context
	baseCancel context.CancelFunc

	emitMu   sync.Mutex // OnChange の呼び出しを直列にする（状態の新旧が入れ替わらないように）
	lastEmit atomic.Int64

	mu        sync.Mutex
	job       *Job // 実行中のジョブ（同時に 1 つ）
	jobCancel context.CancelFunc
	jobDone   chan struct{}
	lastError string
	removing  bool // Remove の最中（ジョブ・起動を受け付けない）
	closed    bool

	proc     *proc    // 準備が済んで動いている ollama
	starting *startOp // 起動中（同時の Acquire は同じ起動を待つ）
	refs     int      // Acquire して release していない数
	idle     *time.Timer
	idleSeq  uint64 // idle を張り直すたびに増やす（古いタイマーの発火を無視する）
	stopGen  uint64 // 止めるたびに増やす（起動中に止められたかを知る）
}

var _ Runtime = (*Manager)(nil)

// New は Manager を作る。何も起動・作成しない。
func New(opts Options) *Manager {
	cfg := resolve(opts)
	m := &Manager{cfg: cfg}
	m.supported, m.reason = cfg.support()
	m.baseCtx, m.baseCancel = context.WithCancel(context.Background())
	return m
}

// Status は今の状態（ファイルの有無から求めるので、起動していなくても正しい）。
func (m *Manager) Status() Status {
	m.mu.Lock()
	st := Status{
		Supported:     m.supported,
		Version:       m.cfg.pin.Version,
		DownloadBytes: m.cfg.pin.Size,
		Running:       m.proc != nil,
		LastError:     m.lastError,
	}
	if !m.supported {
		st.UnsupportedReason = m.reason
	}
	if m.job != nil {
		j := *m.job
		st.Job = &j
	}
	m.mu.Unlock()

	st.Installed = m.supported && m.installed()
	st.Models = make([]Model, 0, len(m.cfg.catalog))
	for _, c := range m.cfg.catalog {
		st.Models = append(st.Models, Model{CatalogModel: c, Installed: m.supported && m.modelInstalled(c.Name)})
	}
	return st
}

// installed は runtime/ に固定版の ollama があるか。
func (m *Manager) installed() bool {
	v, err := os.ReadFile(m.cfg.versionFile())
	if err != nil || strings.TrimSpace(string(v)) != m.cfg.pin.Version {
		return false
	}
	fi, err := os.Stat(m.cfg.binaryPath())
	return err == nil && fi.Mode().IsRegular()
}

// modelInstalled は Ollama を起動せずに、models/manifests/registry.ollama.ai/library/<名前>/<タグ> の有無で判定する。
func (m *Manager) modelInstalled(name string) bool {
	repo, tag, ok := strings.Cut(name, ":")
	if !ok || repo == "" || tag == "" {
		return false
	}
	p := filepath.Join(m.cfg.modelsDir(), "manifests", "registry.ollama.ai", "library", repo, tag)
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// catalogModel は name のカタログ項目。無ければ ErrUnknownModel。
func (m *Manager) catalogModel(name string) (CatalogModel, error) {
	for _, c := range m.cfg.catalog {
		if c.Name == name {
			return c, nil
		}
	}
	return CatalogModel{}, ErrUnknownModel
}

// emit は OnChange を呼ぶ。force でなければ 250ms に 1 回まで（バイト進捗用）。
func (m *Manager) emit(force bool) {
	if m.cfg.onChange == nil {
		return
	}
	if !force && time.Since(time.Unix(0, m.lastEmit.Load())) < progressInterval {
		return
	}
	m.emitMu.Lock()
	defer m.emitMu.Unlock()
	if !force && time.Since(time.Unix(0, m.lastEmit.Load())) < progressInterval {
		return
	}
	m.lastEmit.Store(time.Now().UnixNano())
	m.cfg.onChange(m.Status())
}

// precheck はジョブ・起動の前提（Close 済みでない・使える環境・Remove の最中でない）。m.mu を持って呼ぶ。
func (m *Manager) precheck() error {
	switch {
	case m.closed:
		return errClosed
	case !m.supported:
		return ErrUnsupported
	case m.removing:
		return ErrBusy
	}
	return nil
}

// beginJob は同時に 1 つのジョブを登録する。ほかのジョブが走っていれば ErrBusy。
func (m *Manager) beginJob(kind, model, stage string, total int64) (context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.precheck(); err != nil {
		return nil, err
	}
	if m.job != nil {
		return nil, ErrBusy
	}
	ctx, cancel := context.WithCancel(m.baseCtx)
	m.job = &Job{Kind: kind, Model: model, Stage: stage, Total: total}
	m.jobCancel = cancel
	m.jobDone = make(chan struct{})
	m.lastError = ""
	return ctx, nil
}

// goJob は fn をジョブとして走らせ、終わったら結果を lastError と状態に反映する。beginJob の直後に呼ぶ。
func (m *Manager) goJob(ctx context.Context, fn func(context.Context) error) {
	m.emit(true)
	done := m.jobDone
	go func() {
		defer close(done)
		m.endJob(ctx, fn(ctx))
	}()
}

// endJob はジョブの結果を記録する。中止（ctx が切れた）ときは理由を残さない（利用者が止めたため）。
func (m *Manager) endJob(ctx context.Context, err error) {
	msg := ""
	if err != nil && ctx.Err() == nil {
		msg = "処理に失敗しました。"
		cause := err
		var fe *failureError
		if errors.As(err, &fe) {
			msg, cause = fe.msg, fe.cause
		}
		m.cfg.log.Warn("ローカルAIのジョブが失敗しました", "reason", msg, "cause", cause)
	}
	m.mu.Lock()
	m.job = nil
	m.lastError = msg
	m.jobCancel()
	m.scheduleIdleLocked()
	m.mu.Unlock()
	m.emit(true)
}

// setStage はジョブの段階を変える（即時に通知する）。
func (m *Manager) setStage(stage string, completed, total int64) {
	m.mu.Lock()
	if m.job != nil {
		m.job.Stage, m.job.Completed, m.job.Total = stage, completed, total
	}
	m.mu.Unlock()
	m.emit(true)
}

// setProgress はバイト進捗を更新する（通知は 250ms に 1 回まで）。
func (m *Manager) setProgress(completed int64) {
	m.mu.Lock()
	if m.job != nil {
		m.job.Completed = completed
	}
	m.mu.Unlock()
	m.emit(false)
}

// CancelJob は実行中のジョブを中止する（無ければ何もしない）。ファイルの後始末は非同期に終わる。
func (m *Manager) CancelJob() {
	m.mu.Lock()
	cancel := m.jobCancel
	busy := m.job != nil
	m.mu.Unlock()
	if busy && cancel != nil {
		cancel()
	}
}

// closeWait は Close が、ジョブ・起動中の準備・ollama の終了を待つ合計の上限（アプリの終了の猶予 5 秒より短くする）。
const closeWait = 3 * time.Second

// Close はジョブを中止して ollama を止める。アプリの終了で呼ぶ。何度呼んでもよい。
// 起動の途中だった Ollama はここで中止され（起動の ctx は baseCtx につながっている）、起動後に残らない。待ちの合計は closeWait まで。
func (m *Manager) Close() {
	m.mu.Lock()
	already := m.closed
	m.closed = true
	var waits []chan struct{}
	if m.job != nil && m.jobDone != nil {
		waits = append(waits, m.jobDone)
	}
	if m.starting != nil {
		waits = append(waits, m.starting.done)
	}
	m.mu.Unlock()
	if already {
		return
	}
	m.baseCancel()
	deadline := time.NewTimer(closeWait)
	defer deadline.Stop()
	for _, ch := range waits {
		select {
		case <-ch:
		case <-deadline.C:
			deadline.Reset(0) // 以降の待ちはすぐ打ち切る
		}
	}
	m.mu.Lock()
	pr := m.takeProcLocked()
	m.mu.Unlock()
	if pr != nil {
		pr.p.Kill()
		select {
		case <-pr.done:
		case <-deadline.C:
		}
	}
}
