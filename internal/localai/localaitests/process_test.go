package localaitests

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func TestAcquireStartsOnceAndShares(t *testing.T) {
	f := &fakeOllama{}
	m, _ := fakeManager(t, f, localai.Options{})
	if m.Status().Running {
		t.Fatal("Acquire の前に起動しています")
	}
	var wg sync.WaitGroup
	bases := make([]string, 8)
	releases := make([]func(), 8)
	for i := range bases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, rel, err := m.Acquire(t.Context())
			if err != nil {
				t.Error(err)
				return
			}
			bases[i], releases[i] = b, rel
		}()
	}
	wg.Wait()
	if t.Failed() {
		return
	}
	if n := f.launches(); n != 1 {
		t.Fatalf("起動が %d 回です", n)
	}
	spec := f.spec(t, 0)
	if !strings.HasPrefix(bases[0], "http://127.0.0.1:") || !strings.HasSuffix(bases[0], "/v1") {
		t.Fatalf("base = %q", bases[0])
	}
	for _, b := range bases {
		if b != bases[0] {
			t.Fatalf("base が揃いません: %v", bases)
		}
	}
	if !slices.Equal(spec.Args, []string{"serve"}) || !strings.HasSuffix(spec.Path, binName()) {
		t.Fatalf("spec = %+v", spec)
	}
	if !m.Status().Running {
		t.Fatal("Running が false です")
	}
	// 起動した ollama は /v1 の根で答える。
	resp, err := http.Get(strings.TrimSuffix(bases[0], "/v1") + "/api/version")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("version: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	for _, rel := range releases {
		rel()
		rel() // 何度呼んでもよい
	}
}

func TestLaunchEnv(t *testing.T) {
	t.Setenv("OLLAMA_HOST", "0.0.0.0:11434")
	t.Setenv("OLLAMA_ORIGINS", "*")
	t.Setenv("ollama_debug", "1")
	t.Setenv("NEFOS_KEEP_ME", "yes")
	f := &fakeOllama{}
	m, _ := fakeManager(t, f, localai.Options{})
	base, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	env := f.spec(t, 0).Env
	count := map[string]int{}
	var host string
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(strings.ToUpper(k), "OLLAMA_") {
			count[k]++
		}
		if k == "OLLAMA_HOST" {
			host = v
		}
	}
	for _, k := range []string{"OLLAMA_HOST", "OLLAMA_MODELS", "OLLAMA_KEEP_ALIVE", "OLLAMA_CONTEXT_LENGTH", "OLLAMA_NO_CLOUD"} {
		if count[k] != 1 {
			t.Errorf("%s が %d 個あります", k, count[k])
		}
	}
	if len(count) != 5 {
		t.Errorf("OLLAMA_* が余分にあります: %v", count)
	}
	if want := strings.TrimSuffix(strings.TrimPrefix(base, "http://"), "/v1"); host != want || !strings.HasPrefix(host, "127.0.0.1:") {
		t.Errorf("OLLAMA_HOST = %q, base = %q", host, base)
	}
	for _, want := range []string{"OLLAMA_KEEP_ALIVE=5m", "OLLAMA_CONTEXT_LENGTH=16384", "OLLAMA_NO_CLOUD=1", "NEFOS_KEEP_ME=yes"} {
		if !slices.Contains(env, want) {
			t.Errorf("%s がありません", want)
		}
	}
	var models string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "OLLAMA_MODELS="); ok {
			models = v
		}
	}
	if !strings.HasSuffix(models, "models") {
		t.Errorf("OLLAMA_MODELS = %q", models)
	}
}

func TestIdleStop(t *testing.T) {
	f := &fakeOllama{}
	m, _ := fakeManager(t, f, localai.Options{IdleTimeout: 30 * time.Millisecond})
	_, rel1, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, rel2, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p := f.proc(t, 0)
	rel1()
	// 1 つ残っているうちは、IdleTimeout を過ぎても止めない。
	select {
	case <-p.killed:
		t.Fatal("利用中なのに止めました")
	case <-time.After(150 * time.Millisecond):
	}
	rel2()
	waitFor(t, "アイドルでの停止", func() bool { return p.wasKilled() && !m.Status().Running })

	// 止まった後の Acquire は起動し直す。
	_, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if f.launches() != 2 || !m.Status().Running {
		t.Fatalf("起動し直していません: %d", f.launches())
	}
}

func TestStopKillsEvenWhenAcquired(t *testing.T) {
	f := &fakeOllama{}
	m, rec := fakeManager(t, f, localai.Options{})
	_, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if !f.proc(t, 0).wasKilled() || m.Status().Running {
		t.Fatal("Stop で止まっていません")
	}
	if last := rec.all()[len(rec.all())-1]; last.Running {
		t.Fatal("Stop の通知がありません")
	}
	if err := m.Stop(); err != nil { // 止まっていても何もしない
		t.Fatal(err)
	}
}

func TestAcquireNotInstalledAndClosed(t *testing.T) {
	f := &fakeOllama{}
	m, _ := newManager(t, localai.Options{Launcher: f.launcher()})
	if _, _, err := m.Acquire(t.Context()); !errors.Is(err, localai.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
	if f.launches() != 0 {
		t.Fatal("未導入なのに起動しようとしました")
	}

	other, _ := fakeManager(t, f, localai.Options{})
	if _, _, err := other.Acquire(t.Context()); err != nil {
		t.Fatal(err)
	}
	other.Close()
	if !f.proc(t, 0).wasKilled() {
		t.Fatal("Close で止まっていません")
	}
	if _, _, err := other.Acquire(t.Context()); err == nil {
		t.Fatal("Close 後に Acquire できました")
	}
}

func TestStartRetriesEarlyExit(t *testing.T) {
	f := &fakeOllama{earlyExits: 2}
	m, _ := fakeManager(t, f, localai.Options{})
	_, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if f.launches() != 3 {
		t.Fatalf("起動 %d 回", f.launches())
	}
	// 毎回ポートを取り直す（env の OLLAMA_HOST は 3 回とも 127.0.0.1）。
	for i := range 3 {
		if !slices.ContainsFunc(f.spec(t, i).Env, func(s string) bool { return strings.HasPrefix(s, "OLLAMA_HOST=127.0.0.1:") }) {
			t.Fatalf("起動 %d の OLLAMA_HOST", i)
		}
	}

	f2 := &fakeOllama{earlyExits: 3}
	m3, _ := fakeManager(t, f2, localai.Options{})
	if _, _, err := m3.Acquire(t.Context()); err == nil || m3.Status().Running {
		t.Fatalf("3 回とも終了したのに成功しました: %v", err)
	}
	if f2.launches() != 3 {
		t.Fatalf("起動 %d 回", f2.launches())
	}
}

func TestReadyTimeoutAndCancel(t *testing.T) {
	f := &fakeOllama{noListen: true}
	m, _ := fakeManager(t, f, localai.Options{ReadyTimeout: 50 * time.Millisecond})
	_, _, err := m.Acquire(t.Context())
	if err == nil || !strings.Contains(err.Error(), "間に合いません") {
		t.Fatalf("err = %v", err)
	}
	if !f.proc(t, 0).wasKilled() || m.Status().Running {
		t.Fatal("準備が済まないプロセスを止めていません")
	}

	// ctx を切ると待ちをやめて、プロセスも止める。
	f2 := &fakeOllama{noListen: true}
	m3, _ := fakeManager(t, f2, localai.Options{})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, _, err := m3.Acquire(ctx); done <- err }()
	waitFor(t, "起動", func() bool { return f2.launches() == 1 })
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if !f2.proc(t, 0).wasKilled() {
		t.Fatal("取り消した起動のプロセスを止めていません")
	}
}

func TestUnexpectedExit(t *testing.T) {
	f := &fakeOllama{}
	m, _ := fakeManager(t, f, localai.Options{})
	_, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	f.proc(t, 0).crash()
	waitFor(t, "異常終了の反映", func() bool { return !m.Status().Running })
	rel()
	if _, rel2, err := m.Acquire(t.Context()); err != nil || f.launches() != 2 {
		t.Fatalf("起動し直せません: %v", err)
	} else {
		rel2()
	}
}
