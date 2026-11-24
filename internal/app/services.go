package app

import (
	"github.com/ousiassllc/nefos-inbox/internal/app/deps"
	"github.com/ousiassllc/nefos-inbox/internal/app/mcpcore"
	"github.com/ousiassllc/nefos-inbox/internal/app/threadcache"
)

// Services は Wails に登録するバインディングサービスの組。
type Services struct {
	Account *AccountService
	Inbox   *InboxService
	Thread  *ThreadService
	Compose *ComposeService
	AI      *AIService
	// LocalAI はローカルAI（Ollama。#458）。Deps.LocalAI が nil なら全メソッドが UNSUPPORTED。登録は capability.SupportsLocalAI の OS だけ。
	LocalAI *LocalAIService
	App     *AppService
	Log     *LogService
	MCP     *MCPService
}

// NewServices は各サービスを組み立てる。共有のスレッド本文 LRU（d.Threads）が未設定なら作る
// （同期のイベントで無効化するため、main は先に作って Emitter を包んでから渡す）。
func NewServices(d *Deps) *Services {
	if d.Threads == nil {
		d.Threads = threadcache.New(d.Gmail, threadcache.DefaultCapacity)
	}
	compose := NewComposeService(d)
	inbox := NewInboxService(d)
	return &Services{
		Account: NewAccountService(d),
		Inbox:   inbox,
		Thread:  NewThreadService(d),
		Compose: compose,
		AI:      NewAIService(d, compose),
		LocalAI: NewLocalAIService(d),
		App:     NewAppService(d),
		Log:     NewLogService(d),
		MCP:     NewMCPService(d, mcpcore.New((*deps.Deps)(d), inbox)),
	}
}
