package aicore

import (
	"context"
	"strings"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/app/aicore/aiinput"
	"github.com/ousiassllc/nefos-inbox/internal/app/aicore/localconn"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
)

// aiLocalModelsTimeout はローカルAI（managed）のモデル一覧の取得のタイムアウト（Ollama の起動を含む）。
const aiLocalModelsTimeout = 90 * time.Second

// SetAILocalConnection はローカルAI（Ollama。#458）のつなぎ方を保存する。mode は managed（既定。baseURL は保存しない）／external
// （baseURL は必須。ValidateBaseURL の規則）。この OS で使えない（capability が無い）ときは UNSUPPORTED、mode・Base URL が不正なら VALIDATION。
// 送信先が変わると、そのローカルAIの同意は無効になる。API キー（任意）は維持する。
func (s *Service) SetAILocalConnection(ctx context.Context, mode, baseURL string) (dto.AIConfig, error) {
	if !s.d.Capabilities.SupportsLocalAI {
		return dto.AIConfig{}, apperr.New(apperr.Unsupported, "ローカルAIはこの OS では使えません。")
	}
	mode, baseURL = strings.TrimSpace(mode), strings.TrimSpace(baseURL)
	switch {
	case !localconn.ValidMode(mode):
		return dto.AIConfig{}, apperr.New(apperr.Validation, "ローカルAIのつなぎ方が正しくありません（managed か external）。")
	case mode == dto.LocalModeManaged:
		baseURL = ""
	case baseURL == "":
		return dto.AIConfig{}, apperr.New(apperr.Validation, "ローカルAIの Base URL を入力してください。")
	default:
		if err := aiinput.ValidateBaseURL(baseURL); err != nil {
			return dto.AIConfig{}, err
		}
	}
	conn, err := s.loadConnection(ctx)
	if err != nil {
		return dto.AIConfig{}, err
	}
	conn.LocalMode, conn.LocalBaseURL = mode, baseURL
	return s.saveConnection(ctx, conn)
}

// precheckLocal はローカルAIの前提（この OS で使える→導入済み・Base URL→モデル→loopback 以外は同意済み）を検査し、任意の API キー（無ければ空）を返す。
func (s *Service) precheckLocal(ctx context.Context, label string, sel SelectedAI) (SelectedAI, string, error) {
	conn := localconn.Conn{Mode: sel.LocalMode, BaseURL: sel.BaseURL}
	if err := localconn.Check(s.d.Capabilities, s.d.LocalAI, conn); err != nil {
		return sel, "", err
	}
	if strings.TrimSpace(sel.Model) == "" {
		return sel, "", errNoModel(label)
	}
	if !conn.Loopback() {
		consented, err := s.hasConsent(ctx, providerLocal, sel.BaseURL)
		if err != nil {
			return sel, "", err
		}
		if !consented {
			return sel, "", apperr.New(apperr.AIConsentRequired, "スレッドの内容が AI プロバイダへ送信されることへの同意が必要です。")
		}
	}
	if conn.Managed() {
		return sel, "", nil // アプリが用意する Ollama はキーを使わない
	}
	key, _, err := s.apiKey(providerLocal)
	return sel, key, err
}

// listLocalModels はローカルAIのモデル一覧（接続テストを兼ねる）。baseURL が空は managed（未導入は通信せず not_installed。導入済みは起動して取得）、
// それ以外は external の baseURL から取得する（loopback 以外で未同意は consent_required）。取得できないときは Reason つきの空の一覧。
func (s *Service) listLocalModels(ctx context.Context, baseURL string) (dto.AIModelList, error) {
	if !s.d.Capabilities.SupportsLocalAI {
		return dto.AIModelList{}, apperr.New(apperr.Unsupported, "ローカルAIはこの OS では使えません。")
	}
	if baseURL != "" {
		key, _, err := s.apiKey(providerLocal)
		if err != nil {
			return dto.AIModelList{}, err
		}
		return s.listExternalLocalModels(ctx, baseURL, key)
	}
	if err := localconn.Check(s.d.Capabilities, s.d.LocalAI, localconn.Conn{}); err != nil {
		if apperr.FromInfra(err, "").Code == apperr.AINotConfigured {
			return s.noModels(providerLocal, dto.AIModelsNotInstalled, "ローカルAI（Ollama）をまだ導入していません。設定画面の「AI のつなぎ先」で導入してください。"), nil
		}
		return dto.AIModelList{}, err
	}
	lctx, cancel := context.WithTimeout(ctx, aiLocalModelsTimeout)
	defer cancel()
	base, release, err := localconn.Open(lctx, s.d.LocalAI, providerLocal, dto.LocalModeManaged, "")
	if err != nil {
		if ctx.Err() != nil {
			return dto.AIModelList{}, apperr.Wrap(apperr.Cancelled, "モデル一覧の取得を中止しました。", err)
		}
		reason, msg := aiinput.ModelsFailure(err)
		s.d.Logger.Warn("ai models list failed", "provider", providerLocal, "reason", reason)
		return s.noModels(providerLocal, reason, msg), nil
	}
	defer release()
	return s.fetchModels(lctx, ctx, providerLocal, base, "")
}

func (s *Service) listExternalLocalModels(ctx context.Context, baseURL, key string) (dto.AIModelList, error) {
	if err := aiinput.ValidateBaseURL(baseURL); err != nil {
		return s.noModels(providerLocal, dto.AIModelsInvalidBaseURL, "Base URL が使えないため、モデル一覧を取得できません。モデル名を入力してください。"), nil
	}
	if !(localconn.Conn{Mode: dto.LocalModeExternal, BaseURL: baseURL}).Loopback() {
		consented, err := s.hasConsent(ctx, providerLocal, baseURL)
		if err != nil {
			return dto.AIModelList{}, err
		}
		if !consented {
			return s.noModels(providerLocal, dto.AIModelsConsentRequired, "送信先への同意前は、モデル一覧を取得しません。モデル名を入力してください。"), nil
		}
	}
	lctx, cancel := context.WithTimeout(ctx, aiModelsTimeout)
	defer cancel()
	return s.fetchModels(lctx, ctx, providerLocal, baseURL, key)
}
