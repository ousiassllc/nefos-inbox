package app

import (
	"context"

	"github.com/ousiassllc/nefos-inbox/internal/app/deps"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/app/localaicore"
)

// LocalAIService はアプリが用意するローカルAI（Ollama。Windows のみ。#458）の導入・モデル取得・停止・削除（バインディング仕様 §6）。
// 使える OS（capability.SupportsLocalAI）でだけ登録する。実装は localaicore に委譲する。進捗はイベント localai:status。
// エラー: 使えない環境 UNSUPPORTED・カタログ外のモデル VALIDATION・未導入 AI_NOT_CONFIGURED・ジョブ中 VALIDATION（details.reason="busy"）・
// 空き不足 VALIDATION（details.reason="no_space"・requiredBytes・freeBytes）。
type LocalAIService struct{ core *localaicore.Service }

// NewLocalAIService は LocalAIService を作る。
func NewLocalAIService(d *Deps) *LocalAIService {
	return &LocalAIService{core: localaicore.New((*deps.Deps)(d))}
}

// GetLocalAIStatus はローカルAIの状態（使えるか・導入済みか・起動中か・ジョブ・モデル）を返す。
func (s *LocalAIService) GetLocalAIStatus() (dto.LocalAIStatus, error) { return s.core.Status() }

// InstallLocalAI は Ollama の導入を始める（ジョブを始めて戻る。進捗はイベント）。
func (s *LocalAIService) InstallLocalAI() (dto.LocalAIStatus, error) { return s.core.Install() }

// PullLocalAIModel はカタログのモデルの取得を始める（ジョブを始めて戻る）。
func (s *LocalAIService) PullLocalAIModel(name string) (dto.LocalAIStatus, error) {
	return s.core.Pull(name)
}

// CancelLocalAIJob は実行中の導入・取得を中止する。
func (s *LocalAIService) CancelLocalAIJob() (dto.LocalAIStatus, error) { return s.core.Cancel() }

// StopLocalAI は起動中の Ollama を止める。
func (s *LocalAIService) StopLocalAI() (dto.LocalAIStatus, error) { return s.core.Stop() }

// DeleteLocalAIModel は取得済みのモデルを削除する。
func (s *LocalAIService) DeleteLocalAIModel(ctx context.Context, name string) (dto.LocalAIStatus, error) {
	return s.core.DeleteModel(ctx, name)
}

// RemoveLocalAI は Ollama とモデルをすべて削除する。
func (s *LocalAIService) RemoveLocalAI(ctx context.Context) (dto.LocalAIStatus, error) {
	return s.core.Remove(ctx)
}
