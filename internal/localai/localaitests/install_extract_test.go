package localaitests

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func TestInstallRejectsUnsafeZips(t *testing.T) {
	good := zipEntry{name: binName(), data: "x"}
	many := []zipEntry{good}
	for i := range 2100 {
		many = append(many, zipEntry{name: fmt.Sprintf("f/%d", i)})
	}
	cases := []struct {
		name    string
		entries []zipEntry
		extract int64 // 0 なら 1 MiB
		want    string
	}{
		{"親へ出る", []zipEntry{good, {name: "../evil", data: "x"}}, 0, "安全でない"},
		{"途中で親へ出る", []zipEntry{good, {name: "a/../../evil", data: "x"}}, 0, "安全でない"},
		{"絶対パス", []zipEntry{good, {name: "/abs/evil", data: "x"}}, 0, "安全でない"},
		{"Windows の絶対パス", []zipEntry{good, {name: "C:/evil", data: "x"}}, 0, "安全でない"},
		{"ドライブ相対", []zipEntry{good, {name: "C:evil", data: "x"}}, 0, "安全でない"},
		{"バックスラッシュ", []zipEntry{good, {name: `a\..\..\evil`, data: "x"}}, 0, "安全でない"},
		{"シンボリックリンク", []zipEntry{good, {name: "link", data: "/etc/passwd", mode: os.ModeSymlink | 0o777}}, 0, "安全でない"},
		{"同じ名前が 2 つ", []zipEntry{good, good}, 0, "展開"},
		{"実行ファイルが無い", []zipEntry{{name: "other.txt", data: "x"}}, 0, "実行ファイル"},
		{"展開後の合計が大きすぎる", []zipEntry{good, {name: "big.bin", data: strings.Repeat("\x00", 20<<20)}}, 1024, "サイズ"},
		{"エントリが多すぎる", many, 0, "エントリ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := makeZip(t, c.entries...)
			u, hc := serveTLS(t, serveBody(body))
			pin := pinFor(body, u)
			if c.extract != 0 {
				pin.ExtractedSize = c.extract
			}
			st := failedInstall(t, localai.Options{HTTPClient: hc, Pin: pin})
			if !strings.Contains(st.LastError, c.want) {
				t.Fatalf("LastError = %q, want %q を含む", st.LastError, c.want)
			}
		})
	}
}

func TestInstallCancelMidDownload(t *testing.T) {
	body := goodZip(t)
	half := len(body) / 2
	started := make(chan struct{})
	var once sync.Once
	u, hc := serveTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body[:half])
		w.(http.Flusher).Flush()
		once.Do(func() { close(started) })
		<-r.Context().Done()
	}))
	dir := filepath.Join(t.TempDir(), "ollama")
	m, rec := newManager(t, localai.Options{Dir: dir, HTTPClient: hc, Pin: pinFor(body, u)})
	if err := m.StartInstall(); err != nil {
		t.Fatal(err)
	}
	<-started
	waitFor(t, "進捗", func() bool { j := m.Status().Job; return j != nil && j.Completed > 0 })
	if j := m.Status().Job; j.Kind != localai.JobInstall || j.Stage != localai.StageDownload || j.Total != int64(len(body)) {
		t.Fatalf("Job = %+v", j)
	}

	// ジョブ中はほかの操作を断る。
	if err := m.StartInstall(); !errors.Is(err, localai.ErrBusy) {
		t.Fatalf("StartInstall err = %v", err)
	}
	if err := m.StartPull(localai.Catalog[0].Name); !errors.Is(err, localai.ErrBusy) {
		t.Fatalf("StartPull err = %v", err)
	}
	if err := m.Remove(t.Context()); !errors.Is(err, localai.ErrBusy) {
		t.Fatalf("Remove err = %v", err)
	}

	m.CancelJob()
	st := waitJobEnd(t, m)
	if st.LastError != "" || st.Installed {
		t.Fatalf("中止の状態が違います: %+v", st)
	}
	if left := readDir(t, filepath.Join(dir, "downloads")); len(left) != 0 {
		t.Fatalf("途中のファイルが残っています: %v", left)
	}
	if last := rec.all()[len(rec.all())-1]; last.Job != nil {
		t.Fatalf("最後の通知が終了を示していません: %+v", last)
	}
	// 中止の後は、また始められる。
	if err := m.StartInstall(); err != nil {
		t.Fatalf("取り直せません: %v", err)
	}
	m.CancelJob()
	waitJobEnd(t, m)
}

func TestCloseCancelsInstall(t *testing.T) {
	body := goodZip(t)
	started := make(chan struct{})
	u, hc := serveTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		_, _ = w.Write(body[:10])
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	dir := filepath.Join(t.TempDir(), "ollama")
	m, _ := newManager(t, localai.Options{Dir: dir, HTTPClient: hc, Pin: pinFor(body, u)})
	if err := m.StartInstall(); err != nil {
		t.Fatal(err)
	}
	<-started
	m.Close()
	if m.Status().Job != nil {
		t.Fatal("Close の後もジョブが残っています")
	}
	if left := readDir(t, filepath.Join(dir, "downloads")); len(left) != 0 {
		t.Fatalf("途中のファイルが残っています: %v", left)
	}
	if err := m.StartInstall(); err == nil {
		t.Fatal("Close 後に導入を始められました")
	}
}
