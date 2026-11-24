package localaitests

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// TestRealOllama は実物の ollama（Linux）で起動・準備確認・停止を確かめる。
// NEFOS_OLLAMA_BIN に ollama の実行ファイルのパスを入れたときだけ走る。
func TestRealOllama(t *testing.T) {
	bin := os.Getenv("NEFOS_OLLAMA_BIN")
	if bin == "" {
		t.Skip("NEFOS_OLLAMA_BIN が未設定です")
	}
	t.Setenv("HOME", t.TempDir()) // ollama は $HOME/.ollama に鍵を作る
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	dir := filepath.Join(t.TempDir(), "ollama")
	rt := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(rt, 0o700); err != nil {
		t.Fatal(err)
	}
	// symlink にして、ollama が自分の場所から lib を探せるようにする。
	if err := os.Symlink(bin, filepath.Join(rt, binName())); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rt, "VERSION"), []byte("v9.9.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ := newManager(t, localai.Options{Dir: dir, ReadyTimeout: 60 * time.Second, Pin: localai.RuntimePin{
		Version: "v9.9.9", URL: "https://127.0.0.1/x.zip", Size: 1, SHA256: "00", ExtractedSize: 1}})
	base, rel, err := m.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(base + "/models")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(base, "http://127.0.0.1:") || !m.Status().Running {
		t.Fatalf("status = %d, base = %s", resp.StatusCode, base)
	}
	rel()
	if err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if m.Status().Running {
		t.Fatal("Stop の後も動いています")
	}
	if resp, err := http.Get(base + "/models"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("止めたのに応答があります")
	}
}
