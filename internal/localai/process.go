package localai

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

const (
	maxStartAttempts = 3                      // 起動してすぐ終了したとき（ポートの取り合いなど）の試行回数
	readyPoll        = 100 * time.Millisecond // 準備の確認の間隔
	readyRequestMax  = 2 * time.Second        // 準備の確認 1 回の上限
	killWait         = 5 * time.Second        // 止めてから終了を待つ上限
)

var (
	errEarlyExit = errors.New("ollama が準備の前に終了しました")
	errStopped   = failure("ローカルAIの起動が止められました。", nil)
)

// proc は起動した ollama 1 つ。
type proc struct {
	p    Process
	port int
	done chan struct{} // 終了したら閉じる
}

func (p *proc) root() string { return "http://127.0.0.1:" + strconv.Itoa(p.port) }

// startOp は進行中の起動。同時の Acquire は done を待つ。
type startOp struct{ done chan struct{} }

// Acquire は ollama が動いていなければ起動して、OpenAI 互換の Base URL（http://127.0.0.1:<port>/v1）を返す。
// release を呼ぶまで（呼ばなくても Close まで）アイドルで止めない。release は何度呼んでもよい。
func (m *Manager) Acquire(ctx context.Context) (string, func(), error) {
	for {
		m.mu.Lock()
		if err := m.precheck(); err != nil {
			m.mu.Unlock()
			return "", nil, err
		}
		if m.proc != nil {
			base := m.proc.root() + "/v1"
			m.refs++
			m.scheduleIdleLocked()
			m.mu.Unlock()
			return base, m.releaseOnce(), nil
		}
		if op := m.starting; op != nil {
			m.mu.Unlock()
			select {
			case <-op.done:
				continue
			case <-ctx.Done():
				return "", nil, ctx.Err()
			}
		}
		if !m.installed() {
			m.mu.Unlock()
			return "", nil, ErrNotInstalled
		}
		op := &startOp{done: make(chan struct{})}
		m.starting = op
		gen := m.stopGen
		m.mu.Unlock()

		// 起動は Close（baseCtx の取り消し）でも中止する。呼び出し元の ctx だけに任せると、Close のあとに起動してしまう。
		sctx, cancel := context.WithCancel(ctx)
		stopLink := context.AfterFunc(m.baseCtx, cancel)
		pr, err := m.startProcess(sctx)
		stopLink()
		cancel()

		m.mu.Lock()
		m.starting = nil
		if err == nil && (m.closed || m.stopGen != gen) {
			go killAndWait(pr)
			pr, err = nil, errStopped
		}
		if err == nil {
			m.proc = pr
			m.refs++
			m.scheduleIdleLocked()
		}
		close(op.done)
		m.mu.Unlock()
		if err != nil {
			return "", nil, err
		}
		m.emit(true)
		return pr.root() + "/v1", m.releaseOnce(), nil
	}
}

// releaseOnce は Acquire の release。最後の利用が終わったら IdleTimeout 後に止める。
func (m *Manager) releaseOnce() func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			m.refs--
			m.scheduleIdleLocked()
			m.mu.Unlock()
		})
	}
}

// scheduleIdleLocked は idle タイマーを張り直す（利用中・ジョブ中・止まっているときは張らない）。m.mu を持って呼ぶ。
func (m *Manager) scheduleIdleLocked() {
	if m.idle != nil {
		m.idle.Stop()
		m.idle = nil
	}
	m.idleSeq++
	if m.closed || m.proc == nil || m.refs > 0 || m.job != nil {
		return
	}
	seq := m.idleSeq
	m.idle = time.AfterFunc(m.cfg.idleTimeout, func() { m.idleStop(seq) })
}

func (m *Manager) idleStop(seq uint64) {
	m.mu.Lock()
	if seq != m.idleSeq || m.refs > 0 || m.job != nil {
		m.mu.Unlock()
		return
	}
	pr := m.takeProcLocked()
	m.mu.Unlock()
	if pr != nil {
		m.cfg.log.Info("ローカルAIを使っていないので止めます")
		killAndWait(pr)
		m.emit(true)
	}
}

// takeProcLocked は動いている ollama を外して返す（止めるのは呼び出し側）。m.mu を持って呼ぶ。
func (m *Manager) takeProcLocked() *proc {
	pr := m.proc
	m.proc = nil
	m.stopGen++
	m.scheduleIdleLocked()
	return pr
}

func killAndWait(pr *proc) {
	pr.p.Kill()
	select {
	case <-pr.done:
	case <-time.After(killWait):
	}
}

// stopProcess は動いている ollama を止める（通知はしない）。
func (m *Manager) stopProcess() {
	m.mu.Lock()
	pr := m.takeProcLocked()
	m.mu.Unlock()
	if pr != nil {
		killAndWait(pr)
	}
}

// Stop は今すぐ ollama を止める。導入・取得のジョブ中は ErrBusy（ジョブが使っている Ollama を止めない）。
// ジョブ以外で Acquire 中の利用者（生成など）がいても止める（その要求は失敗する。次の Acquire で起動し直す）。動いていなければ何もしない。
func (m *Manager) Stop() error {
	m.mu.Lock()
	busy := m.job != nil
	running := m.proc != nil
	m.mu.Unlock()
	if busy {
		return ErrBusy
	}
	if !running {
		return nil
	}
	m.stopProcess()
	m.emit(true)
	return nil
}

// onExit は ollama が終了したときの後始末（止められたものはすでに外れている）。
func (m *Manager) onExit(pr *proc, err error) {
	code := 0
	if err != nil {
		code = -1
		var ee interface{ ExitCode() int }
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
	}
	m.mu.Lock()
	unexpected := m.proc == pr
	if unexpected {
		m.proc = nil
		m.scheduleIdleLocked()
	}
	m.mu.Unlock()
	m.cfg.log.Info("ollama が終了しました", "exitCode", code, "unexpected", unexpected)
	if unexpected {
		m.emit(true)
	}
}
