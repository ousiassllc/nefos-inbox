// Package localaicore はローカルAI（アプリが用意する Ollama。Windows のみ。#458）の導入・モデル取得・起動停止の操作と、状態の変換を担う
// （LocalAIService の本体）。管理そのものは internal/localai（Runtime）が行う。
package localaicore

import (
	"context"

	"github.com/ousiassllc/nefos-inbox/internal/app/aicore/localconn"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/deps"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// ToDTO は管理している Ollama の状態を、バインディング・イベント（localai:status）のペイロードにする。Models は nil にしない。
func ToDTO(st localai.Status) dto.LocalAIStatus {
	out := dto.LocalAIStatus{
		Supported: st.Supported, UnsupportedReason: st.UnsupportedReason, Installed: st.Installed, Running: st.Running,
		Version: st.Version, DownloadBytes: st.DownloadBytes, LastError: st.LastError,
		Models: make([]dto.LocalAIModel, len(st.Models)),
	}
	if j := st.Job; j != nil {
		out.Job = &dto.LocalAIJob{Kind: j.Kind, Model: j.Model, Stage: j.Stage, CompletedBytes: j.Completed, TotalBytes: j.Total}
	}
	for i, m := range st.Models {
		out.Models[i] = dto.LocalAIModel{
			Name: m.Name, DisplayName: m.DisplayName, Description: m.Description, License: m.License, SizeBytes: m.SizeBytes, Installed: m.Installed,
		}
	}
	return out
}

// Service はローカルAIの操作（どれも操作後の状態を返す）。Deps.LocalAI が nil（使えない OS・未配線）なら UNSUPPORTED。
type Service struct{ d *deps.Deps }

// New は Service を作る。
func New(d *deps.Deps) *Service { return &Service{d: d} }

// runtime は使える Runtime。nil は UNSUPPORTED。
func (s *Service) runtime() (localai.Runtime, error) {
	if s.d.LocalAI == nil {
		return nil, apperr.New(apperr.Unsupported, localai.ErrUnsupported.Error())
	}
	return s.d.LocalAI, nil
}

// do は Runtime を取り、op の結果（エラーは AppError にする）と操作後の状態を返す。
func (s *Service) do(op func(rt localai.Runtime) error) (dto.LocalAIStatus, error) {
	rt, err := s.runtime()
	if err != nil {
		return dto.LocalAIStatus{}, err
	}
	if err := op(rt); err != nil {
		return dto.LocalAIStatus{}, localconn.MapError(err)
	}
	return ToDTO(rt.Status()), nil
}

// Status は状態を返す。
func (s *Service) Status() (dto.LocalAIStatus, error) {
	return s.do(func(localai.Runtime) error { return nil })
}

// Install は Ollama の導入を始める（ジョブを始めて戻る。進捗はイベント localai:status）。
func (s *Service) Install() (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { return rt.StartInstall() })
}

// Pull はモデルの取得を始める（ジョブを始めて戻る）。
func (s *Service) Pull(name string) (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { return rt.StartPull(name) })
}

// Cancel は実行中の導入・取得を中止する（無ければ何もしない）。
func (s *Service) Cancel() (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { rt.CancelJob(); return nil })
}

// Stop は起動中の Ollama を止める。
func (s *Service) Stop() (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { return rt.Stop() })
}

// DeleteModel は取得済みのモデルを削除する。
func (s *Service) DeleteModel(ctx context.Context, name string) (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { return rt.DeleteModel(ctx, name) })
}

// Remove は Ollama とモデルをすべて削除する（ジョブ中は VALIDATION）。
func (s *Service) Remove(ctx context.Context) (dto.LocalAIStatus, error) {
	return s.do(func(rt localai.Runtime) error { return rt.Remove(ctx) })
}
