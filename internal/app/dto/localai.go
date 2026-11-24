package dto

// AILocalConnection はローカルAI（Ollama。provider "local"）のつなぎ方（#458）。capability が無い OS でも返す（Ready は false）。
type AILocalConnection struct {
	// Mode は "managed"（既定。アプリが用意する Ollama）／"external"（すでに動いている Ollama・LM Studio など）。
	Mode string `json:"mode"`
	// BaseURL は external のときの Base URL。managed は空。
	BaseURL string `json:"baseUrl"`
	// Ready はローカルAIを選べる状態か。managed は導入済みでモデルが 1 つ以上取得済み、external は Base URL が有効。
	Ready bool `json:"ready"`
	// Loopback は送信先が PC の外へ出ないか（managed、または BaseURL の host が localhost／127.0.0.1／::1 の external）。
	// 同意の要否（loopback 以外は同意が要る）の判定と同じ関数で求める。画面はこの値で同意の欄を出す。
	Loopback bool `json:"loopback"`
}

// ローカルAIのつなぎ方（AILocalConnection.Mode）。
const (
	LocalModeManaged  = "managed"
	LocalModeExternal = "external"
)

// ローカルAIのジョブの種類（LocalAIJob.Kind）と段階（LocalAIJob.Stage）。
const (
	LocalAIJobInstall = "install"
	LocalAIJobPull    = "pull"

	LocalAIStageDownload = "download"
	LocalAIStageVerify   = "verify"
	LocalAIStageExtract  = "extract"
	LocalAIStageStart    = "start"
	LocalAIStagePull     = "pull"
)

// LocalAIStatus はアプリが用意するローカルAI（Ollama）の状態（LocalAIService・イベント localai:status。#458）。
type LocalAIStatus struct {
	Supported bool `json:"supported"`
	// UnsupportedReason は使えない理由（日本語 1 文）。使えるときは省略。
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
	Installed         bool   `json:"installed"`
	Running           bool   `json:"running"`
	// Version は固定した Ollama の版（v0.40.17 など）。
	Version string `json:"version"`
	// DownloadBytes は導入でダウンロードする量（バイト）。
	DownloadBytes int64 `json:"downloadBytes"`
	// Job は実行中の導入・モデル取得。無ければ省略。
	Job *LocalAIJob `json:"job,omitempty"`
	// Models は取得できるモデル（カタログ順）。null にしない。
	Models []LocalAIModel `json:"models"`
	// LastError は直前に失敗したジョブの理由（日本語。次のジョブを始めると消える）。
	LastError string `json:"lastError,omitempty"`
}

// LocalAIJob は実行中のジョブ（同時に 1 つ）。
type LocalAIJob struct {
	Kind           string `json:"kind"`
	Model          string `json:"model,omitempty"`
	Stage          string `json:"stage"`
	CompletedBytes int64  `json:"completedBytes"`
	TotalBytes     int64  `json:"totalBytes"`
}

// LocalAIModel は取得できるモデル 1 つと、取得済みか。
type LocalAIModel struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	License     string `json:"license"`
	SizeBytes   int64  `json:"sizeBytes"`
	Installed   bool   `json:"installed"`
}
