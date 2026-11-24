package localai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// startProcess は ollama を起動して準備ができるまで待つ。起動してすぐ終了したときは別のポートでやり直す（最大 3 回）。
func (m *Manager) startProcess(ctx context.Context) (*proc, error) {
	if err := os.MkdirAll(m.cfg.modelsDir(), 0o700); err != nil {
		return nil, failure("ローカルAIの保存先を作れませんでした。", err)
	}
	var last error
	for range maxStartAttempts {
		pr, err := m.launchOnce(ctx)
		if err == nil {
			return pr, nil
		}
		if !errors.Is(err, errEarlyExit) {
			return nil, err
		}
		last = err
	}
	return nil, failure("ローカルAIを起動できませんでした。", last)
}

func (m *Manager) launchOnce(ctx context.Context) (*proc, error) {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	switch { // 起動の直前に確かめる（Close のあとに子プロセスを作らない）
	case closed:
		return nil, errClosed
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}
	port, err := freePort()
	if err != nil {
		return nil, failure("ローカルAIの待ち受けポートを確保できませんでした。", err)
	}
	p, err := m.cfg.launch(LaunchSpec{
		Path: m.cfg.binaryPath(),
		Args: []string{"serve"},
		Env:  launchEnv(os.Environ(), m.cfg.modelsDir(), port),
		Dir:  m.cfg.runtimeDir(),
	})
	if err != nil {
		return nil, failure("ローカルAI（Ollama）を起動できませんでした。", err)
	}
	pr := &proc{p: p, port: port, done: make(chan struct{})}
	go func() {
		err := p.Wait()
		close(pr.done)
		m.onExit(pr, err)
	}()
	if err := m.waitReady(ctx, pr); err != nil {
		killAndWait(pr)
		return nil, err
	}
	return pr, nil
}

// waitReady は GET /api/version が 200 を返すまで、ReadyTimeout・ctx・プロセスの終了を上限に待つ。
func (m *Manager) waitReady(ctx context.Context, pr *proc) error {
	deadline := time.NewTimer(m.cfg.readyTimeout)
	defer deadline.Stop()
	tick := time.NewTimer(0)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-pr.done:
			return errEarlyExit
		case <-deadline.C:
			return failure("ローカルAIの起動が間に合いませんでした。", nil)
		case <-tick.C:
		}
		if m.versionOK(ctx, pr.root()) {
			return nil
		}
		tick.Reset(readyPoll)
	}
}

func (m *Manager) versionOK(ctx context.Context, root string) bool {
	ctx, cancel := context.WithTimeout(ctx, readyRequestMax)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/api/version", nil)
	if err != nil {
		return false
	}
	resp, err := localClient.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// localClient は 127.0.0.1 の ollama への通信用（プロキシを通さない。Timeout は掛けず、呼び出し側の ctx で打ち切る）。
var localClient = &http.Client{Transport: &http.Transport{Proxy: nil}}

// freePort は 127.0.0.1 の空きポートを 1 つ返す（確保してすぐ閉じるので、起動までに取られ得る。そのときは再試行する）。
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// launchEnv は ollama に渡す環境変数。親の OLLAMA_* は引き継がず（設定の持ち込みを防ぐ）、必要なものを足す。
func launchEnv(parent []string, modelsDir string, port int) []string {
	env := make([]string, 0, len(parent)+5)
	for _, kv := range parent {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(key), "OLLAMA_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		fmt.Sprintf("OLLAMA_HOST=127.0.0.1:%d", port),
		"OLLAMA_MODELS="+modelsDir,
		"OLLAMA_KEEP_ALIVE="+keepAlive,
		"OLLAMA_CONTEXT_LENGTH="+contextLength,
		"OLLAMA_NO_CLOUD=1",
	)
}
