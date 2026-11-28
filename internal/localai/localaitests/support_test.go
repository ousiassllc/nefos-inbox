package localaitests

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

func TestDefaultSupportOutsideWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows の実機の検査は実機でだけ確かめる")
	}
	m := localai.New(localai.Options{Dir: t.TempDir()})
	t.Cleanup(m.Close)
	st := m.Status()
	if st.Supported || st.UnsupportedReason != "Windows 以外では使えません。" || st.Installed {
		t.Fatalf("%+v", st)
	}
}

func TestDefaultDir(t *testing.T) {
	d, err := localai.DefaultDir()
	if err != nil {
		t.Skip("UserCacheDir が使えません")
	}
	if filepath.Base(d) != "ollama" || filepath.Base(filepath.Dir(d)) != "localai" || filepath.Base(filepath.Dir(filepath.Dir(d))) != "Nefos Inbox" {
		t.Fatalf("DefaultDir = %q", d)
	}
}

func TestDefaultFreeSpace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("実機で確かめる")
	}
	// 既定の FreeSpace（OS の問い合わせ）で、巨大な要求は空き不足になり、存在しない下位フォルダも親から求める。
	m := localai.New(localai.Options{
		Dir: filepath.Join(t.TempDir(), "a", "b"), Support: supported,
		Pin: localai.RuntimePin{Version: "v1", URL: "https://127.0.0.1/x.zip", Size: 1 << 60, SHA256: "00", ExtractedSize: 1 << 60},
	})
	t.Cleanup(m.Close)
	var ns *localai.NoSpaceError
	if err := m.StartInstall(); !asNoSpace(err, &ns) || ns.Free <= 0 {
		t.Fatalf("err = %v", err)
	}
}
