package localaitests

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// fakeOllama は Launcher の fake。起動すると、環境変数の OLLAMA_HOST で指定された 127.0.0.1:<port> で HTTP サーバーを立てる。
type fakeOllama struct {
	// handler は /api/version 以外の要求を受ける（nil なら 404）。
	handler http.Handler
	// earlyExits は最初の N 回の起動を、待ち受けずにすぐ終了させる。
	earlyExits int
	// noListen は待ち受けずに動き続ける（準備が済まないプロセス）。
	noListen bool

	mu    sync.Mutex
	specs []localai.LaunchSpec
	procs []*fakeProc
}

func (f *fakeOllama) launcher() localai.Launcher { return f.launch }

func (f *fakeOllama) launch(spec localai.LaunchSpec) (localai.Process, error) {
	f.mu.Lock()
	f.specs = append(f.specs, spec)
	n := len(f.specs)
	f.mu.Unlock()

	p := &fakeProc{killed: make(chan struct{}), exited: make(chan struct{})}
	f.mu.Lock()
	f.procs = append(f.procs, p)
	f.mu.Unlock()
	if n <= f.earlyExits {
		p.err = errors.New("exit status 1")
		close(p.exited)
		return p, nil
	}
	if f.noListen {
		return p, nil
	}
	addr := ""
	for _, kv := range spec.Env {
		if v, ok := strings.CutPrefix(kv, "OLLAMA_HOST="); ok {
			addr = v
		}
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.0.0"}`))
	})
	if f.handler != nil {
		mux.Handle("/", f.handler)
	}
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: time.Minute}
	go func() { _ = p.srv.Serve(l) }()
	return p, nil
}

func (f *fakeOllama) launches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.specs)
}

func (f *fakeOllama) proc(t *testing.T, i int) *fakeProc {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.procs) {
		t.Fatalf("プロセス %d は起動されていません", i)
	}
	return f.procs[i]
}

func (f *fakeOllama) spec(t *testing.T, i int) localai.LaunchSpec {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.specs) {
		t.Fatalf("起動 %d はありません", i)
	}
	return f.specs[i]
}

// fakeProc は localai.Process の fake。
type fakeProc struct {
	srv    *http.Server
	killed chan struct{} // Kill で閉じる
	exited chan struct{} // 終了で閉じる（Wait が戻る）
	err    error
	once   sync.Once
}

func (p *fakeProc) Wait() error {
	<-p.exited
	return p.err
}

func (p *fakeProc) Kill() {
	p.once.Do(func() {
		close(p.killed)
		if p.srv != nil {
			_ = p.srv.Close()
		}
		select {
		case <-p.exited:
		default:
			close(p.exited)
		}
	})
}

// crash はプロセスを（Kill を経ずに）異常終了させる。
func (p *fakeProc) crash() {
	p.err = errors.New("exit status 2")
	if p.srv != nil {
		_ = p.srv.Close()
	}
	close(p.exited)
}

func (p *fakeProc) wasKilled() bool {
	select {
	case <-p.killed:
		return true
	default:
		return false
	}
}

// fakeManager は導入済みの状態で、f を Launcher にした Manager を作る。
func fakeManager(t *testing.T, f *fakeOllama, opts localai.Options) (*localai.Manager, *recorder) {
	t.Helper()
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	installFake(t, opts.Dir, "v9.9.9")
	opts.Launcher = f.launcher()
	opts.Pin = localai.RuntimePin{Version: "v9.9.9", URL: "https://127.0.0.1/x.zip", Size: 1, SHA256: "00", ExtractedSize: 1}
	return newManager(t, opts)
}
