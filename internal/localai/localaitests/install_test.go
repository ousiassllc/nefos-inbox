package localaitests

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func readDir(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}

func TestInstallSuccess(t *testing.T) {
	body := goodZip(t)
	u, hc := serveTLS(t, serveBody(body))
	dir := filepath.Join(t.TempDir(), "ollama")
	m, rec := newManager(t, localai.Options{Dir: dir, HTTPClient: hc, Pin: pinFor(body, u)})

	if st := m.Status(); st.Installed || !st.Supported || st.Version != "v9.9.9" || st.DownloadBytes != int64(len(body)) {
		t.Fatalf("導入前の状態が違います: %+v", st)
	}
	if err := m.StartInstall(); err != nil {
		t.Fatal(err)
	}
	st := waitJobEnd(t, m)
	if st.LastError != "" || !st.Installed {
		t.Fatalf("導入できていません: %+v", st)
	}
	v, _ := os.ReadFile(filepath.Join(dir, "runtime", "VERSION")) //nolint:gosec // テストの一時フォルダ
	if strings.TrimSpace(string(v)) != "v9.9.9" {
		t.Fatalf("VERSION = %q", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "runtime", "lib", "ollama", "runner.dll")); err != nil {
		t.Fatal(err)
	}
	if left := readDir(t, filepath.Join(dir, "downloads")); len(left) != 0 {
		t.Fatalf("downloads/ に残っています: %v", left)
	}
	if got := readDir(t, dir); !slices.Equal(got, []string{"downloads", "runtime"}) {
		t.Fatalf("置き場所の中身が違います: %v", got)
	}
	want := []string{localai.StageDownload, localai.StageVerify, localai.StageExtract}
	if got := rec.stages(); !slices.Equal(got, want) {
		t.Fatalf("段階 = %v, want %v", got, want)
	}
	if last := rec.all()[len(rec.all())-1]; last.Job != nil || !last.Installed {
		t.Fatalf("最後の通知が終了を示していません: %+v", last)
	}
	// 導入済みなら何もしない。
	if err := m.StartInstall(); err != nil || m.Status().Job != nil {
		t.Fatalf("導入済みで再導入しようとしました: %v", err)
	}
}

func TestInstallReplacesOlderVersion(t *testing.T) {
	body := goodZip(t)
	u, hc := serveTLS(t, serveBody(body))
	dir := filepath.Join(t.TempDir(), "ollama")
	installFake(t, dir, "v0.0.1")
	if err := os.WriteFile(filepath.Join(dir, "runtime", "stale.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := newManager(t, localai.Options{Dir: dir, HTTPClient: hc, Pin: pinFor(body, u)})
	if m.Status().Installed {
		t.Fatal("古い版が導入済みと見なされました")
	}
	if err := m.StartInstall(); err != nil {
		t.Fatal(err)
	}
	if st := waitJobEnd(t, m); !st.Installed || st.LastError != "" {
		t.Fatalf("%+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, "runtime", "stale.txt")); !os.IsNotExist(err) {
		t.Fatal("古い runtime/ の中身が残っています")
	}
}

// failedInstall は導入を走らせて、失敗した状態を返す（途中のファイルが何も残らないことも確かめる）。
func failedInstall(t *testing.T, opts localai.Options) localai.Status {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ollama")
	opts.Dir = dir
	m, _ := newManager(t, opts)
	if err := m.StartInstall(); err != nil {
		t.Fatal(err)
	}
	st := waitJobEnd(t, m)
	if st.Installed || st.LastError == "" {
		t.Fatalf("失敗するはずの導入が失敗していません: %+v", st)
	}
	if left := readDir(t, filepath.Join(dir, "downloads")); len(left) != 0 {
		t.Fatalf("downloads/ に残っています: %v", left)
	}
	for _, n := range readDir(t, dir) {
		if n != "downloads" {
			t.Fatalf("置き場所に残っています: %v", n)
		}
	}
	return st
}

func TestInstallRejectsBadDownloads(t *testing.T) {
	body := goodZip(t)
	t.Run("サイズが違う", func(t *testing.T) {
		u, hc := serveTLS(t, serveBody(body))
		pin := pinFor(body, u)
		pin.Size++
		st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pin})
		if !strings.Contains(st.LastError, "サイズ") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("SHA-256 が違う", func(t *testing.T) {
		u, hc := serveTLS(t, serveBody(body))
		pin := pinFor(body, u)
		pin.SHA256 = strings.Repeat("0", 64)
		st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pin})
		if !strings.Contains(st.LastError, "SHA-256") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("HTTP エラー", func(t *testing.T) {
		u, hc := serveTLS(t, http.NotFoundHandler())
		st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u)})
		if !strings.Contains(st.LastError, "404") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("Content-Length が無い", func(t *testing.T) {
		u, hc := serveTLS(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(body)
			w.(http.Flusher).Flush()
		}))
		failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u)})
	})
	t.Run("宣言より多く返す", func(t *testing.T) {
		pin := pinFor(body, "https://127.0.0.1/ollama.zip")
		rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200, ContentLength: pin.Size,
				Body: readCloser{strings.NewReader(string(body) + strings.Repeat("y", 4096))},
			}, nil
		})
		st := failedInstall(t, localai.Options{HTTPClient: &http.Client{Transport: rt}, Pin: pin})
		if !strings.Contains(st.LastError, "大きい") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
}

func TestInstallRejectsDisallowedHosts(t *testing.T) {
	body := goodZip(t)
	t.Run("リダイレクト先が許可外", func(t *testing.T) {
		u, hc := serveTLS(t, http.RedirectHandler("https://evil.invalid/x.zip?sig=secret", http.StatusFound))
		st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u)})
		if !strings.Contains(st.LastError, "許可していない接続先") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("リダイレクトが多すぎる", func(t *testing.T) {
		u, hc := serveTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, r.URL.Path+"x", http.StatusFound) //nolint:gosec // テストの fake サーバー（自分のパスへ戻すだけ）
		}))
		st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u)})
		if !strings.Contains(st.LastError, "リダイレクト") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("http は拒否", func(t *testing.T) {
		st := failedInstall(t, localai.Options{Pin: pinFor(body, "http://127.0.0.1:1/ollama.zip")})
		if !strings.Contains(st.LastError, "許可していない接続先") {
			t.Fatalf("LastError = %q", st.LastError)
		}
	})
	t.Run("AllowURL が拒否する", func(t *testing.T) {
		u, hc := serveTLS(t, serveBody(body))
		m, _ := newManager(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u),
			AllowURL: func(*url.URL) bool { return false }})
		if err := m.StartInstall(); err != nil {
			t.Fatal(err)
		}
		if st := waitJobEnd(t, m); st.Installed || st.LastError == "" {
			t.Fatalf("%+v", st)
		}
	})
}

func TestInstallLastErrorHasNoURLOrPath(t *testing.T) {
	body := goodZip(t)
	u, hc := serveTLS(t, http.RedirectHandler("https://evil.invalid/x.zip?sig=secret", http.StatusFound))
	st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pinFor(body, u)})
	for _, bad := range []string{"127.0.0.1", "evil", "secret", string(filepath.Separator)} {
		if strings.Contains(st.LastError, bad) {
			t.Fatalf("LastError に %q が含まれています: %q", bad, st.LastError)
		}
	}
}

func TestInstallNoSpace(t *testing.T) {
	body := goodZip(t)
	u, hc := serveTLS(t, serveBody(body))
	pin := pinFor(body, u)
	m, _ := newManager(t, localai.Options{HTTPClient: hc, Pin: pin, FreeSpace: func(string) (int64, error) { return 123, nil }})
	err := m.StartInstall()
	var ns *localai.NoSpaceError
	if !errors.As(err, &ns) {
		t.Fatalf("err = %v, want *NoSpaceError", err)
	}
	if want := pin.Size + pin.ExtractedSize + 1<<30; ns.Required != want || ns.Free != 123 {
		t.Fatalf("NoSpaceError = %+v, want Required=%d", ns, want)
	}
	if m.Status().Job != nil {
		t.Fatal("空き不足なのにジョブが始まりました")
	}
}

func TestInstallUnsupported(t *testing.T) {
	m, _ := newManager(t, localai.Options{Support: func() (bool, string) { return false, "テスト用の理由です。" }})
	if err := m.StartInstall(); !errors.Is(err, localai.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
	if st := m.Status(); st.Supported || st.UnsupportedReason != "テスト用の理由です。" || st.Installed {
		t.Fatalf("%+v", st)
	}
	if _, _, err := m.Acquire(t.Context()); !errors.Is(err, localai.ErrUnsupported) {
		t.Fatalf("Acquire err = %v", err)
	}
}
