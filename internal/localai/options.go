package localai

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultIdleTimeout は最後の利用（release）からこの時間使われなければ Ollama を止める。
	DefaultIdleTimeout = 10 * time.Minute
	// DefaultReadyTimeout は起動してから /api/version が 200 を返すまで待つ最長の時間。
	DefaultReadyTimeout = 60 * time.Second

	// progressInterval は OnChange をバイト進捗で呼ぶ最短の間隔（段階・状態の変化は即時）。
	progressInterval = 250 * time.Millisecond

	// 空き容量の確認で、必要量に足す余裕。
	spaceMargin = int64(1) << 30

	// OLLAMA_KEEP_ALIVE と OLLAMA_CONTEXT_LENGTH（設計 §2）。
	keepAlive     = "5m"
	contextLength = "16384"
)

// RuntimePin は導入する Ollama の固定版と配布物。ゼロ値の Options.Pin は types.go の固定値になる。
type RuntimePin struct {
	Version       string
	URL           string
	Size          int64
	SHA256        string
	ExtractedSize int64 // 展開後の合計。空き容量の見積もりと展開の上限に使う
}

// defaultPin は types.go の固定値。
func defaultPin() RuntimePin {
	return RuntimePin{
		Version:       RuntimeVersion,
		URL:           RuntimeURL,
		Size:          RuntimeSize,
		SHA256:        RuntimeSHA256,
		ExtractedSize: RuntimeExtractedSize,
	}
}

// Options は Manager の構成。ゼロ値のフィールドは既定値になる。
// 「テスト用」と書いたものは、実機以外（Linux の開発・単体テスト）で差し替えるための入り口で、アプリは設定しない。
type Options struct {
	// Dir は置き場所（DefaultDir）。downloads/・runtime/・models/ をこの下に作る。
	Dir string
	// HTTPClient は Ollama の zip のダウンロードに使う。リダイレクトの制限は渡された値に関係なく掛ける（コピーして差し替える）。
	HTTPClient *http.Client
	Logger     *slog.Logger
	// OnChange は状態が変わるたびに呼ぶ（進捗は 250ms に 1 回まで）。Manager の鍵を持たずに呼ぶ。
	OnChange func(Status)
	// IdleTimeout・ReadyTimeout は DefaultIdleTimeout・DefaultReadyTimeout。
	IdleTimeout  time.Duration
	ReadyTimeout time.Duration

	// Pin は固定版（テスト用。ゼロ値なら types.go の値）。
	Pin RuntimePin
	// Catalog は取得できるモデル（テスト用。nil なら types.go の Catalog）。
	Catalog []CatalogModel
	// AllowURL は通信してよい URL（テスト用。https 以外は常に拒否する。既定は github.com と *.githubusercontent.com）。
	AllowURL func(*url.URL) bool
	// Launcher はプロセスの起動（テスト用。既定は実際に ollama を起動する）。
	Launcher Launcher
	// FreeSpace は path のあるボリュームの空きバイト数（テスト用。既定は OS の問い合わせ）。
	FreeSpace func(path string) (int64, error)
	// Support は使える環境か（テスト用。既定は OS の検査。使えないときは理由を日本語 1 文で返す）。
	Support func() (ok bool, reason string)
}

// DefaultDir は既定の置き場所 <os.UserCacheDir()>/Nefos Inbox/localai/ollama。
// Windows では %LOCALAPPDATA% の下（数 GB になるので Roaming には置かない）。
func DefaultDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "Nefos Inbox", "localai", "ollama"), nil
}

// resolved は既定値を埋めた構成。
type resolved struct {
	dir          string
	hc           *http.Client
	log          *slog.Logger
	onChange     func(Status)
	idleTimeout  time.Duration
	readyTimeout time.Duration
	pin          RuntimePin
	catalog      []CatalogModel
	allow        func(*url.URL) bool
	launch       Launcher
	freeSpace    func(string) (int64, error)
	support      func() (bool, string)
}

func resolve(o Options) resolved {
	r := resolved{
		dir: o.Dir, log: o.Logger, onChange: o.OnChange, idleTimeout: o.IdleTimeout, readyTimeout: o.ReadyTimeout,
		pin: o.Pin, catalog: o.Catalog, allow: o.AllowURL, launch: o.Launcher, freeSpace: o.FreeSpace, support: o.Support,
	}
	if r.log == nil {
		r.log = slog.New(slog.DiscardHandler)
	}
	if r.idleTimeout <= 0 {
		r.idleTimeout = DefaultIdleTimeout
	}
	if r.readyTimeout <= 0 {
		r.readyTimeout = DefaultReadyTimeout
	}
	if r.pin.URL == "" {
		r.pin = defaultPin()
	}
	if r.catalog == nil {
		r.catalog = Catalog
	}
	if r.allow == nil {
		r.allow = defaultAllowURL
	}
	if r.launch == nil {
		r.launch = launchProcess
	}
	if r.freeSpace == nil {
		r.freeSpace = diskFreeBytes
	}
	if r.support == nil {
		r.support = checkSupport
	}
	// 管理するのは Ollama の zip だけ。リダイレクトの制限は渡されたクライアントの値に関係なく掛ける。
	client := http.Client{}
	if o.HTTPClient != nil {
		client = *o.HTTPClient
	}
	client.CheckRedirect = r.checkRedirect
	r.hc = &client
	return r
}

// defaultAllowURL は GitHub のリリースの配信元。github.com から *.githubusercontent.com へリダイレクトされる。
func defaultAllowURL(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com")
}

// binaryName は runtime/ の中の実行ファイル名。Windows 以外は Linux の開発・テストで動かすため ollama。
func binaryName() string {
	if runtime.GOOS == "windows" {
		return "ollama.exe"
	}
	return "ollama"
}

// 置き場所の中の構成。
func (r resolved) downloadsDir() string { return filepath.Join(r.dir, "downloads") }
func (r resolved) zipPath() string {
	return filepath.Join(r.downloadsDir(), "ollama-windows-amd64.zip")
}
func (r resolved) runtimeDir() string  { return filepath.Join(r.dir, "runtime") }
func (r resolved) modelsDir() string   { return filepath.Join(r.dir, "models") }
func (r resolved) versionFile() string { return filepath.Join(r.runtimeDir(), "VERSION") }
func (r resolved) binaryPath() string  { return filepath.Join(r.runtimeDir(), binaryName()) }

// freeBytes は path のあるボリュームの空き。path が無ければ、あるところまで遡る。
func (r resolved) freeBytes(path string) (int64, error) {
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return r.freeSpace(path)
}

// checkSpace は path のボリュームに need が空いているか。足りなければ *NoSpaceError。
func (r resolved) checkSpace(path string, need int64) error {
	free, err := r.freeBytes(path)
	if err != nil {
		return failure("空き容量を確認できませんでした。", err)
	}
	if free < need {
		return &NoSpaceError{Required: need, Free: free}
	}
	return nil
}

// Launcher は ollama のプロセスを起動する。
type Launcher func(spec LaunchSpec) (Process, error)

// LaunchSpec は起動する内容。
type LaunchSpec struct {
	Path string   // 実行ファイル
	Args []string // 引数（serve だけ）
	Env  []string // 環境変数（KEY=VALUE）
	Dir  string   // 作業ディレクトリ
}

// Process は起動した ollama。
type Process interface {
	// Wait は終了するまで待つ。終了コードが 0 以外なら error を返す。
	Wait() error
	// Kill は子孫のプロセスごと止める。何度呼んでもよい。
	Kill()
}
