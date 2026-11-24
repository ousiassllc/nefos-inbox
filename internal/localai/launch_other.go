//go:build !windows

package localai

import (
	"os/exec"
	"syscall"
)

// execProcess は Windows 以外（Linux の開発・テスト）で動かす ollama。
type execProcess struct{ cmd *exec.Cmd }

// launchProcess は ollama を起動する。標準入出力は捨てる（/dev/null。メモリにもファイルにも残さない）。
// 子孫ごと止められるように、専用のプロセスグループに入れる。
func launchProcess(spec LaunchSpec) (Process, error) {
	cmd := exec.Command(spec.Path, spec.Args...) //nolint:gosec // 固定の実行ファイルと固定の引数（serve）だけ
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcess{cmd: cmd}, nil
}

func (p *execProcess) Wait() error { return p.cmd.Wait() }

func (p *execProcess) Kill() {
	if p.cmd.Process == nil {
		return
	}
	// プロセスグループごと止める（負の pid）。失敗したら本体だけでも止める。
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		_ = p.cmd.Process.Kill()
	}
}
