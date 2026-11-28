// Package localai は、アプリが用意して動かすローカル AI（Ollama。Windows のみ）を管理する（#458）。
// Ollama の導入（固定版・固定 URL・サイズと SHA-256 の照合）、起動と停止（127.0.0.1 だけで待ち受け、使わなければ止める）、
// 決めたモデルの取得・削除を行う。生成そのものは OpenAI 互換の Chat Completions で internal/ai が行う。
package localai

import (
	"context"
	"errors"
	"fmt"
)

// 固定する Ollama の版と配布物（GitHub の公式リリース。2026-11-24 にダウンロードして測り直した値）。
const (
	RuntimeVersion = "v0.40.17"
	RuntimeURL     = "https://github.com/ollama/ollama/releases/download/v0.40.17/ollama-windows-amd64.zip"
	RuntimeSize    = int64(1618522644)
	RuntimeSHA256  = "3dd80f6e3a9699f2eadf4ac418f00529cee594df9557541806eda1f1f3f5588a"
	// RuntimeExtractedSize は zip の中身の合計（80 ファイル）。空き容量の見積もりと、展開の上限に使う。
	RuntimeExtractedSize = int64(2267742223)
)

// CatalogModel は取得できるモデル 1 つ（決めたものだけ。自由入力の取得はしない）。
type CatalogModel struct {
	Name        string // Ollama の <名前>:<タグ>
	DisplayName string
	Description string
	License     string
	SizeBytes   int64 // 取得の合計（manifest の layers と config の合計）
}

// Catalog は取得できるモデル（画面の表示順）。サイズは registry.ollama.ai の manifest（2026-11-24 確認）。タグは更新され得る。
var Catalog = []CatalogModel{
	{Name: "gemma4:e2b", DisplayName: "Gemma 4 E2B", Description: "軽い（メモリ 8 GB の PC 向け）", License: "Apache-2.0", SizeBytes: 4587156505},
	{Name: "gemma4:e4b", DisplayName: "Gemma 4 E4B", Description: "標準（おすすめ）", License: "Apache-2.0", SizeBytes: 6583656505},
	{Name: "gemma4:12b", DisplayName: "Gemma 4 12B", Description: "高品質（メモリ 16 GB 以上・GPU 推奨）", License: "Apache-2.0", SizeBytes: 8021618941},
}

// ジョブの種類（Job.Kind）。
const (
	JobInstall = "install"
	JobPull    = "pull"
)

// ジョブの段階（Job.Stage）。
const (
	StageDownload = "download" // Ollama の zip をダウンロード中
	StageVerify   = "verify"   // サイズと SHA-256 の照合中
	StageExtract  = "extract"  // 展開中
	StageStart    = "start"    // モデルの取得のために Ollama を起動中
	StagePull     = "pull"     // モデルを取得中
)

// Status は管理している Ollama の状態（画面に出す）。
type Status struct {
	Supported         bool
	UnsupportedReason string // 使えない理由（日本語 1 文）。Supported のときは空
	Installed         bool
	Running           bool
	Version           string // RuntimeVersion
	DownloadBytes     int64  // RuntimeSize
	Job               *Job   // 実行中のジョブ。無ければ nil
	Models            []Model
	LastError         string // 直前に失敗したジョブの理由（日本語。次のジョブを始めると消える）
}

// Job は実行中のジョブ（同時に 1 つ）。
type Job struct {
	Kind      string
	Model     string // JobPull のときのモデル名
	Stage     string
	Completed int64
	Total     int64
}

// Model はカタログのモデルと、取得済みか。
type Model struct {
	CatalogModel
	Installed bool
}

// エラー。メッセージは画面に出してよい日本語（パス・URL を含めない）。
var (
	ErrUnsupported  = errors.New("この PC では、アプリが用意する Ollama を動かせません。")
	ErrNotInstalled = errors.New("ローカルAI（Ollama）がまだ導入されていません。")
	ErrUnknownModel = errors.New("取得できないモデルです。")
	ErrBusy         = errors.New("ほかの導入・取得が終わるまでお待ちください。")
)

// NoSpaceError は空き容量が足りないこと。
type NoSpaceError struct {
	Required int64
	Free     int64
}

func (e *NoSpaceError) Error() string {
	return fmt.Sprintf("空き容量が足りません（必要: 約 %s・空き: 約 %s）。", formatGB(e.Required), formatGB(e.Free))
}

// formatGB はバイト数を「12.3 GB」（10 進の GB。小数 1 桁）にする。
func formatGB(n int64) string {
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// Runtime は aicore・バインディングが使う管理の面（*Manager が実装する。テストではフェイク）。
type Runtime interface {
	Status() Status
	StartInstall() error
	StartPull(model string) error
	CancelJob()
	DeleteModel(ctx context.Context, model string) error
	Stop() error
	Remove(ctx context.Context) error
	// Acquire は Ollama が動いていなければ起動して、OpenAI 互換の Base URL（http://127.0.0.1:<port>/v1）を返す。
	// release を呼ぶまでアイドルで止めない。未導入は ErrNotInstalled、使えない環境は ErrUnsupported。
	Acquire(ctx context.Context) (baseURL string, release func(), err error)
}
