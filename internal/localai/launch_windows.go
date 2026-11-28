//go:build windows

package localai

import (
	"errors"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// winProcess は Job Object（KILL_ON_JOB_CLOSE）に入れた ollama。アプリの終了・異常終了でハンドルが閉じると、子孫ごと止まる。
type winProcess struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	job windows.Handle // 閉じたら 0
}

// launchProcess は コンソールを出さずに ollama を、サスペンド（CREATE_SUSPENDED）で起動し、Job Object に入れてから再開する。
// 入れる前に動き出すと、その間に作った子が Job の外に出てしまうため。標準入出力は捨てる（NUL）。
func launchProcess(spec LaunchSpec) (Process, error) {
	job, err := newKillOnCloseJob()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(spec.Path, spec.Args...) //nolint:gosec // 固定の実行ファイルと固定の引数（serve）だけ
	cmd.Env = spec.Env
	cmd.Dir = spec.Dir
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_SUSPENDED}
	if err := cmd.Start(); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	pid := uint32(cmd.Process.Pid) //nolint:gosec // pid は正の小さな値
	if err := assignToJob(job, pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = windows.CloseHandle(job)
		return nil, err
	}
	if err := resumeMainThread(pid); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = windows.CloseHandle(job)
		return nil, err
	}
	return &winProcess{cmd: cmd, job: job}, nil
}

func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil { //nolint:gosec // Win32 API へ構造体を渡す
		_ = windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func assignToJob(job windows.Handle, pid uint32) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	return windows.AssignProcessToJobObject(job, h)
}

// resumeMainThread はサスペンドで作ったプロセスの唯一のスレッド（メインスレッド）を再開する。
// os/exec はスレッドのハンドルを返さないので、スレッドの一覧から pid の持ち主のものを探す（再開前は、そのプロセスのスレッドは 1 つだけ）。
func resumeMainThread(pid uint32) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))} //nolint:gosec // 構造体の大きさは小さい
	for err = windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		th, oerr := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if oerr != nil {
			return oerr
		}
		_, rerr := windows.ResumeThread(th)
		_ = windows.CloseHandle(th)
		return rerr
	}
	return errors.New("起動したプロセスのスレッドが見つかりません")
}

// Wait は終了まで待ち、Job を閉じる（残った子孫があれば KILL_ON_JOB_CLOSE で止まる）。
func (p *winProcess) Wait() error {
	err := p.cmd.Wait()
	p.closeJob(false)
	return err
}

// Kill は Job ごと（子孫を含めて）終了させる。
func (p *winProcess) Kill() {
	p.closeJob(true)
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
}

func (p *winProcess) closeJob(terminate bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.job == 0 {
		return
	}
	if terminate {
		_ = windows.TerminateJobObject(p.job, 1)
	}
	_ = windows.CloseHandle(p.job)
	p.job = 0
}
