package localaitests

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

const testModel = "gemma4:e2b"

// sampleLines は実際の ollama の /api/pull の出力（間引いたもの）。
func sampleLines(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "pull.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// sampleSums は lines[:n] を読んだあとの、digest ごとの completed の合計と、全 digest の total の合計。
func sampleSums(t *testing.T, lines []string, n int) (completed, total int64) {
	t.Helper()
	type rec struct {
		Digest           string
		Total, Completed int64
	}
	comp, tot := map[string]int64{}, map[string]int64{}
	for i, l := range lines {
		var r rec
		if err := json.Unmarshal([]byte(l), &r); err != nil {
			t.Fatal(err)
		}
		if r.Digest == "" {
			continue
		}
		tot[r.Digest] = r.Total
		if i < n {
			comp[r.Digest] = r.Completed
		}
	}
	for _, v := range comp {
		completed += v
	}
	for _, v := range tot {
		total += v
	}
	return completed, total
}

// pullServer は /api/pull・/api/delete の fake（Ollama の代わり）。
type pullServer struct {
	lines      []string
	status     int           // 0 なら 200
	pauseAfter int           // この行数を送ったら止まる（-1 なら止まらない）
	paused     chan struct{} // 止まったら閉じる
	resume     chan struct{} // 閉じると再開する
	manifest   string        // success の前に作るファイル（取得済みの印）
	deleteCode int

	mu         sync.Mutex
	pullBody   string
	deleteBody string
	cancelled  chan struct{} // 要求が切られたら閉じる
	once       sync.Once
}

func newPullServer(lines []string, manifest string) *pullServer {
	return &pullServer{lines: lines, pauseAfter: -1, manifest: manifest, paused: make(chan struct{}),
		resume: make(chan struct{}), cancelled: make(chan struct{})}
}

func (s *pullServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	switch r.URL.Path {
	case "/api/delete":
		s.mu.Lock()
		s.deleteBody = r.Method + " " + string(body)
		s.mu.Unlock()
		if s.deleteCode != 0 {
			w.WriteHeader(s.deleteCode)
			return
		}
		_ = os.Remove(s.manifest)
	case "/api/pull":
		s.mu.Lock()
		s.pullBody = string(body)
		s.mu.Unlock()
		if s.status != 0 {
			w.WriteHeader(s.status)
			return
		}
		s.stream(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *pullServer) stream(w http.ResponseWriter, r *http.Request) {
	for i, l := range s.lines {
		if i == s.pauseAfter {
			close(s.paused)
			select {
			case <-s.resume:
			case <-r.Context().Done():
				s.once.Do(func() { close(s.cancelled) })
				return
			}
		}
		if strings.Contains(l, `"success"`) && s.manifest != "" {
			_ = os.MkdirAll(filepath.Dir(s.manifest), 0o700)
			_ = os.WriteFile(s.manifest, []byte("{}"), 0o600)
		}
		_, _ = w.Write([]byte(l + "\n"))
		w.(http.Flusher).Flush()
	}
}

func manifestPath(dir string) string {
	return filepath.Join(dir, "models", "manifests", "registry.ollama.ai", "library", "gemma4", "e2b")
}

// pullManager は fake の ollama とカタログ（testModel 1 つ）つきの Manager。
func pullManager(t *testing.T, srv *pullServer, size int64, opts localai.Options) (*localai.Manager, *fakeOllama, *recorder, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ollama")
	f := &fakeOllama{handler: srv}
	opts.Dir = dir
	opts.Catalog = []localai.CatalogModel{{Name: testModel, DisplayName: "テスト", SizeBytes: size}}
	m, rec := fakeManager(t, f, opts)
	return m, f, rec, dir
}

func TestPullProgressAndSuccess(t *testing.T) {
	lines := sampleLines(t)
	pauseAt := len(lines) / 2
	wantDone, total := sampleSums(t, lines, pauseAt)
	if wantDone == 0 || wantDone >= total {
		t.Fatalf("testdata が想定と違います: %d / %d", wantDone, total)
	}
	srv := newPullServer(lines, "")
	srv.pauseAfter = pauseAt
	m, f, rec, dir := pullManager(t, srv, total, localai.Options{})
	srv.manifest = manifestPath(dir)

	if err := m.StartPull(testModel); err != nil {
		t.Fatal(err)
	}
	if j := m.Status().Job; j == nil || j.Kind != localai.JobPull || j.Model != testModel || j.Total != total {
		t.Fatalf("Job = %+v", j)
	}
	<-srv.paused
	waitFor(t, "途中の進捗", func() bool {
		j := m.Status().Job
		return j != nil && j.Stage == localai.StagePull && j.Completed == wantDone
	})
	if st := m.Status(); !st.Running || st.Models[0].Installed {
		t.Fatalf("取得中の状態が違います: %+v", st)
	}
	close(srv.resume)
	st := waitJobEnd(t, m)
	if st.LastError != "" || !st.Models[0].Installed || !st.Running {
		t.Fatalf("取得の結果が違います: %+v", st)
	}
	srv.mu.Lock()
	body := srv.pullBody
	srv.mu.Unlock()
	var req map[string]any
	if err := json.Unmarshal([]byte(body), &req); err != nil || req["model"] != testModel || req["stream"] != true || len(req) != 2 {
		t.Fatalf("pull の要求 = %q", body)
	}
	if f.launches() != 1 {
		t.Fatalf("起動 %d 回", f.launches())
	}
	var sawStart, sawPull bool
	for _, s := range rec.all() {
		if s.Job == nil {
			continue
		}
		sawStart = sawStart || s.Job.Stage == localai.StageStart
		sawPull = sawPull || s.Job.Stage == localai.StagePull
		if s.Job.Completed > s.Job.Total {
			t.Fatalf("Completed が Total を超えています: %+v", s.Job)
		}
	}
	if !sawStart || !sawPull {
		t.Fatalf("段階が通知されていません: start=%v pull=%v", sawStart, sawPull)
	}
}

func TestPullClampsProgress(t *testing.T) {
	lines := sampleLines(t)
	pauseAt := len(lines) / 2
	srv := newPullServer(lines, "")
	srv.pauseAfter = pauseAt
	m, _, _, _ := pullManager(t, srv, 1000, localai.Options{})
	if err := m.StartPull(testModel); err != nil {
		t.Fatal(err)
	}
	<-srv.paused
	waitFor(t, "上限まで", func() bool { j := m.Status().Job; return j != nil && j.Completed == 1000 })
	close(srv.resume)
	waitJobEnd(t, m)
}

func TestPullFailures(t *testing.T) {
	cases := []struct {
		name   string
		lines  []string
		status int
		want   string
	}{
		{"error の行", []string{`{"status":"pulling manifest"}`, `{"error":"pull model manifest: file does not exist"}`}, 0, "モデルの取得に失敗しました。"},
		{"success が無いまま終わる", []string{`{"status":"pulling manifest"}`}, 0, "途中で切れました"},
		{"HTTP エラー", nil, http.StatusInternalServerError, "500"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := newPullServer(c.lines, "")
			srv.status = c.status
			m, _, _, _ := pullManager(t, srv, 100, localai.Options{})
			if err := m.StartPull(testModel); err != nil {
				t.Fatal(err)
			}
			st := waitJobEnd(t, m)
			if !strings.Contains(st.LastError, c.want) || strings.Contains(st.LastError, "manifest") || st.Models[0].Installed {
				t.Fatalf("LastError = %q", st.LastError)
			}
			// 失敗の後は取り直せる（LastError は次のジョブで消える）。
			srv.lines, srv.status = []string{`{"status":"success"}`}, 0
			if err := m.StartPull(testModel); err != nil {
				t.Fatal(err)
			}
			if st := waitJobEnd(t, m); st.LastError != "" {
				t.Fatalf("LastError = %q", st.LastError)
			}
		})
	}
}

func TestPullCancel(t *testing.T) {
	srv := newPullServer(sampleLines(t), "")
	srv.pauseAfter = 3
	m, _, _, _ := pullManager(t, srv, 100, localai.Options{})
	if err := m.StartPull(testModel); err != nil {
		t.Fatal(err)
	}
	<-srv.paused
	m.CancelJob()
	<-srv.cancelled // 接続が閉じられた
	st := waitJobEnd(t, m)
	if st.LastError != "" || st.Models[0].Installed {
		t.Fatalf("%+v", st)
	}
}

// 前回の途中までの層（Ollama の blobs/sha256-*-partial）は、取り直すと続きから進むので、空き容量の見積もりから引く。
func TestStartPullNoSpaceCountsPartialBlobs(t *testing.T) {
	const size, partial = int64(5 << 30), int64(3 << 20)
	need := size + 1<<30 - partial // 途中の層を数えた必要量
	check := func(free int64) (*localai.Manager, string, error) {
		m, _, _, dir := pullManager(t, newPullServer(nil, ""), size, localai.Options{FreeSpace: func(string) (int64, error) { return free, nil }})
		blobs := filepath.Join(dir, "models", "blobs")
		if err := os.MkdirAll(blobs, 0o700); err != nil {
			t.Fatal(err)
		}
		files := map[string]int64{
			"sha256-aaaa-partial":   partial, // 数える
			"sha256-bbbb":           1 << 20, // 取得済みの層は数えない
			"sha256-cccc-partial-0": 1 << 20, // 層ごとの記録（-partial で終わらない）は数えない
		}
		for name, n := range files {
			if err := os.WriteFile(filepath.Join(blobs, name), make([]byte, n), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return m, dir, m.StartPull(testModel)
	}

	var ns *localai.NoSpaceError
	if _, _, err := check(need - 1); !errors.As(err, &ns) || ns.Required != need || ns.Free != need-1 {
		t.Fatalf("err = %v (%+v), want *NoSpaceError{Required: %d}", err, ns, need)
	}
	m, _, err := check(need)
	if err != nil {
		t.Fatalf("途中の層を数えれば足りるのに断られました: %v", err)
	}
	m.CancelJob()
	waitJobEnd(t, m)
}

func TestStartPullPreconditions(t *testing.T) {
	srv := newPullServer(nil, "")
	m, f, _, _ := pullManager(t, srv, 5<<30, localai.Options{FreeSpace: func(string) (int64, error) { return 1 << 30, nil }})
	if err := m.StartPull("llama3:8b"); !errors.Is(err, localai.ErrUnknownModel) {
		t.Fatalf("err = %v", err)
	}
	err := m.StartPull(testModel)
	var ns *localai.NoSpaceError
	if !errors.As(err, &ns) || ns.Required != 6<<30 || ns.Free != 1<<30 {
		t.Fatalf("err = %v", err)
	}
	if m.Status().Job != nil || f.launches() != 0 {
		t.Fatal("空き不足なのにジョブ・起動がありました")
	}

	bare, _ := newManager(t, localai.Options{Catalog: []localai.CatalogModel{{Name: testModel, SizeBytes: 1}}})
	if err := bare.StartPull(testModel); !errors.Is(err, localai.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
	un, _ := newManager(t, localai.Options{Support: func() (bool, string) { return false, "x" }})
	if err := un.StartPull(localai.Catalog[0].Name); !errors.Is(err, localai.ErrUnsupported) {
		t.Fatalf("err = %v", err)
	}
}
