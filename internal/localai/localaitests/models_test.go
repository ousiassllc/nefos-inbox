package localaitests

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func TestModelInstalledByManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ollama")
	f := &fakeOllama{}
	m, _ := fakeManager(t, f, localai.Options{Dir: dir}) // 既定のカタログ（gemma4:e2b・e4b・12b）
	for _, mod := range m.Status().Models {
		if mod.Installed {
			t.Fatalf("取得していないのに取得済みです: %s", mod.Name)
		}
	}
	p := filepath.Join(dir, "models", "manifests", "registry.ollama.ai", "library", "gemma4", "e4b")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	st := m.Status()
	if len(st.Models) != len(localai.Catalog) {
		t.Fatalf("models = %d", len(st.Models))
	}
	for i, mod := range st.Models {
		if mod.Name != localai.Catalog[i].Name {
			t.Fatalf("カタログ順ではありません: %v", st.Models)
		}
		if want := mod.Name == "gemma4:e4b"; mod.Installed != want {
			t.Fatalf("%s: Installed = %v", mod.Name, mod.Installed)
		}
	}
	if f.launches() != 0 {
		t.Fatal("判定のために Ollama を起動しました")
	}
}

func TestStatusModelsNeverNil(t *testing.T) {
	m, _ := newManager(t, localai.Options{Support: func() (bool, string) { return false, "x" }})
	if st := m.Status(); st.Models == nil || len(st.Models) != len(localai.Catalog) || st.Models[0].Installed {
		t.Fatalf("%+v", st)
	}
}

func TestDeleteModel(t *testing.T) {
	srv := newPullServer(nil, "")
	m, f, _, dir := pullManager(t, srv, 100, localai.Options{})
	srv.manifest = manifestPath(dir)

	// 取得していなければ何もせず、Ollama も起動しない。
	if err := m.DeleteModel(t.Context(), testModel); err != nil || f.launches() != 0 {
		t.Fatalf("err = %v, launches = %d", err, f.launches())
	}
	if err := m.DeleteModel(t.Context(), "llama3:8b"); !errors.Is(err, localai.ErrUnknownModel) {
		t.Fatalf("err = %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(srv.manifest), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srv.manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteModel(t.Context(), testModel); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	got := srv.deleteBody
	srv.mu.Unlock()
	if got != `DELETE {"model":"gemma4:e2b"}` {
		t.Fatalf("delete の要求 = %q", got)
	}
	if m.Status().Models[0].Installed {
		t.Fatal("削除したのに取得済みです")
	}

	// Ollama の失敗は日本語の理由で返す。
	if err := os.WriteFile(srv.manifest, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv.deleteCode = http.StatusInternalServerError
	if err := m.DeleteModel(t.Context(), testModel); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("err = %v", err)
	}

	bare, _ := newManager(t, localai.Options{Catalog: []localai.CatalogModel{{Name: testModel, SizeBytes: 1}}})
	if err := bare.DeleteModel(t.Context(), testModel); !errors.Is(err, localai.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteModelBusyDuringJob(t *testing.T) {
	srv := newPullServer(sampleLines(t), "")
	srv.pauseAfter = 2
	m, _, _, _ := pullManager(t, srv, 100, localai.Options{})
	if err := m.StartPull(testModel); err != nil {
		t.Fatal(err)
	}
	<-srv.paused
	if err := m.DeleteModel(t.Context(), testModel); !errors.Is(err, localai.ErrBusy) {
		t.Fatalf("err = %v", err)
	}
	m.CancelJob()
	waitJobEnd(t, m)
}

func TestRemove(t *testing.T) {
	srv := newPullServer(nil, "")
	m, f, rec, dir := pullManager(t, srv, 100, localai.Options{})
	_, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	if err := os.MkdirAll(filepath.Dir(manifestPath(dir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !f.proc(t, 0).wasKilled() {
		t.Fatal("Remove で止まっていません")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("置き場所が残っています: %v", err)
	}
	if st := m.Status(); st.Installed || st.Running || st.Job != nil {
		t.Fatalf("%+v", st)
	}
	if last := rec.all()[len(rec.all())-1]; last.Installed {
		t.Fatal("Remove の通知がありません")
	}
	if err := m.StartPull(testModel); !errors.Is(err, localai.ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
}
