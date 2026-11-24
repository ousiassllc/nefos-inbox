package main

import (
	"log/slog"

	"github.com/ousiassllc/nefos-inbox/internal/app/localaicore"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
	appevents "github.com/ousiassllc/nefos-inbox/internal/events"
	"github.com/ousiassllc/nefos-inbox/internal/localai"
	"github.com/ousiassllc/nefos-inbox/internal/logging"
)

// newLocalAI はアプリが用意するローカルAI（Ollama。Windows のみ。#458）の管理を作る。何も起動・作成しない（Ollama は Acquire のときだけ起動する）。
// capability が無い OS・置き場所を決められないときは nil（ローカルAIは使えない扱い）。戻りの close はアプリの終了で呼び、起動中の Ollama を止める。
func newLocalAI(caps capability.Capabilities, dir func() (string, error), events appevents.Emitter, logger *slog.Logger) (localai.Runtime, func()) {
	if !caps.SupportsLocalAI {
		return nil, func() {}
	}
	d, err := dir()
	if err != nil {
		logger.Warn("local ai disabled: resolve dir failed", "error", err)
		return nil, func() {}
	}
	m := localai.New(localai.Options{
		Dir:    d,
		Logger: logging.WithOrigin(logger, logging.OriginApp),
		OnChange: func(st localai.Status) {
			events.Emit(appevents.LocalAIStatus, localaicore.ToDTO(st))
		},
	})
	return m, m.Close
}
