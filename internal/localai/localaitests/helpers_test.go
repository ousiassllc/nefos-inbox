package localaitests

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

const waitLimit = 10 * time.Second

// binName は runtime/ の中の実行ファイル名（localai の binaryName と同じ規則）。
func binName() string {
	if runtime.GOOS == "windows" {
		return "ollama.exe"
	}
	return "ollama"
}

// waitFor は cond が真になるまで、上限つきで繰り返し確かめる。
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitLimit)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("待ちきれませんでした: %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// waitJobEnd はジョブが終わる（Status().Job が nil になる）まで待つ。
func waitJobEnd(t *testing.T, m *localai.Manager) localai.Status {
	t.Helper()
	waitFor(t, "ジョブの終了", func() bool { return m.Status().Job == nil })
	return m.Status()
}

// zipEntry は作る zip の 1 項目。
type zipEntry struct {
	name string
	data string
	mode os.FileMode // 0 なら 0o755
}

func makeZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		if mode == 0 {
			mode = 0o755
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(e.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// goodZip は ollama の実行ファイルと lib/ の 1 ファイルを持つ zip。
func goodZip(t *testing.T) []byte {
	return makeZip(t,
		zipEntry{name: binName(), data: "#!/bin/sh\nexit 0\n"},
		zipEntry{name: "lib/ollama/runner.dll", data: strings.Repeat("x", 4096)},
	)
}

// pinFor は body に合わせた固定版（URL は u）。
func pinFor(body []byte, u string) localai.RuntimePin {
	sum := sha256.Sum256(body)
	return localai.RuntimePin{
		Version: "v9.9.9", URL: u, Size: int64(len(body)),
		SHA256: hex.EncodeToString(sum[:]), ExtractedSize: 1 << 20,
	}
}

// loopbackOnly は 127.0.0.1 だけを許す AllowURL。
func loopbackOnly(u *url.URL) bool { return u.Hostname() == "127.0.0.1" }

// roomy は十分な空きを返す FreeSpace。
func roomy(string) (int64, error) { return 1 << 50, nil }

func supported() (bool, string) { return true, "" }

// recorder は OnChange で受けた Status を貯める。
type recorder struct {
	mu   sync.Mutex
	list []localai.Status
}

func (r *recorder) add(s localai.Status) {
	r.mu.Lock()
	r.list = append(r.list, s)
	r.mu.Unlock()
}

func (r *recorder) all() []localai.Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]localai.Status(nil), r.list...)
}

// stages は受けた Status の Job.Stage を順に（重複を除いて）返す。
func (r *recorder) stages() []string {
	var out []string
	for _, s := range r.all() {
		if s.Job == nil {
			continue
		}
		if n := len(out); n == 0 || out[n-1] != s.Job.Stage {
			out = append(out, s.Job.Stage)
		}
	}
	return out
}

// newManager は Dir を t.TempDir() にした Manager を作る。opts の未設定の seam は「使える・十分な空き・loopback 許可」にする。
func newManager(t *testing.T, opts localai.Options) (*localai.Manager, *recorder) {
	t.Helper()
	rec := &recorder{}
	if opts.Dir == "" {
		opts.Dir = filepath.Join(t.TempDir(), "ollama")
	}
	opts.OnChange = rec.add
	if opts.Support == nil {
		opts.Support = supported
	}
	if opts.FreeSpace == nil {
		opts.FreeSpace = roomy
	}
	if opts.AllowURL == nil {
		opts.AllowURL = loopbackOnly
	}
	m := localai.New(opts)
	t.Cleanup(m.Close)
	return m, rec
}

// serveTLS は https の fake サーバーを立てて、その URL とクライアントを返す。
func serveTLS(t *testing.T, h http.Handler) (string, *http.Client) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	return srv.URL + "/ollama-windows-amd64.zip", srv.Client()
}

// serveBody は body を Content-Length つきで返す。
func serveBody(body []byte) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	})
}

// installFake は導入済みの状態（runtime/<binary> と VERSION）を直接作る。
func installFake(t *testing.T, dir, version string) {
	t.Helper()
	rt := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(rt, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, binName()), []byte("fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "VERSION"), []byte(version+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// roundTripFunc は関数を http.RoundTripper にする。
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// readCloser は io.Reader に Close を足す。
type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

func asNoSpace(err error, target **localai.NoSpaceError) bool { return errors.As(err, target) }
