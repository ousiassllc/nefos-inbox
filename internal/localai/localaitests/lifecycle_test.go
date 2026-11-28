package localaitests

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// ジョブ（導入・取得）の最中の Stop は ErrBusy（Ollama を止めない）。
func TestStopBusyDuringJob(t *testing.T) {
	srv := newPullServer(sampleLines(t), "")
	srv.pauseAfter = 2
	m, f, _, _ := pullManager(t, srv, 100, localai.Options{})
	if err := m.StartPull(testModel); err != nil {
		t.Fatal(err)
	}
	<-srv.paused
	if err := m.Stop(); !errors.Is(err, localai.ErrBusy) {
		t.Fatalf("err = %v", err)
	}
	if f.proc(t, 0).wasKilled() || !m.Status().Running {
		t.Fatal("ジョブ中なのに止めました")
	}
	m.CancelJob()
	waitJobEnd(t, m)
}

// 起動の最中に Close すると、起動は中止され、Close のあとに Ollama が残らない。
func TestCloseDuringStart(t *testing.T) {
	f := &fakeOllama{noListen: true}
	m, _ := fakeManager(t, f, localai.Options{})
	done := make(chan error, 1)
	go func() { _, _, err := m.Acquire(context.Background()); done <- err }() // 呼び出し側の ctx は切らない
	waitFor(t, "起動", func() bool { return f.launches() == 1 })
	m.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close のあとに Acquire が成功しました")
		}
	case <-time.After(waitLimit):
		t.Fatal("Acquire が戻りません")
	}
	if !f.proc(t, 0).wasKilled() || m.Status().Running {
		t.Fatal("起動中のプロセスを止めていません")
	}
	if f.launches() != 1 {
		t.Fatalf("起動 %d 回", f.launches())
	}
}

// Close のあとは、起動の再試行でも子プロセスを作らない（起動の直前に Close 済みを確かめる）。
func TestNoLaunchAfterClose(t *testing.T) {
	f := &fakeOllama{earlyExits: 3}
	m, _ := fakeManager(t, f, localai.Options{})
	m.Close()
	if _, _, err := m.Acquire(context.Background()); err == nil {
		t.Fatal("Close のあとに Acquire できました")
	}
	if f.launches() != 0 {
		t.Fatalf("起動 %d 回", f.launches())
	}
}

// ダウンロードのバイトが届かないまま StallTimeout を超えたら、止めて理由を残す。
func TestDownloadStall(t *testing.T) {
	body := goodZip(t)
	u, hc := serveTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body[:10])
		w.(http.Flusher).Flush()
		<-r.Context().Done() // 続きを送らない
	}))
	st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u), StallTimeout: 100 * time.Millisecond})
	if st.LastError != "ダウンロードが進まなくなったため止めました。" {
		t.Fatalf("LastError = %q", st.LastError)
	}
}

func TestUserMessage(t *testing.T) {
	srv := newPullServer(nil, "")
	srv.deleteCode = http.StatusInternalServerError
	m, _, _, dir := pullManager(t, srv, 100, localai.Options{})
	mf := manifestPath(dir)
	if err := os.MkdirAll(filepath.Dir(mf), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mf, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := m.DeleteModel(t.Context(), testModel)
	msg, ok := localai.UserMessage(err)
	if !ok || msg != "モデルを削除できませんでした（Ollama が HTTP 500 を返しました）。" {
		t.Fatalf("UserMessage(%v) = %q, %v", err, msg, ok)
	}
	if _, ok := localai.UserMessage(localai.ErrBusy); ok {
		t.Fatal("ErrBusy に画面向けの失敗理由はありません")
	}
}
