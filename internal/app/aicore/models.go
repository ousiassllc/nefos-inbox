package aicore

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/app/aicore/aiinput"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
)

// aiModelsTimeout はモデル一覧の取得のタイムアウト。
const aiModelsTimeout = 20 * time.Second

// ListAIModels は provider のモデル一覧を、渡された送信先（openai は baseURL。空は既定）から取得する（#76・#105）。
// 保存済みの設定には依らないので、設定を保存する前でも、選んだプロバイダ（と Base URL）の一覧を取得できる。
// baseURL は openai と local のみ有効（それ以外のプロバイダでは無視する）。local は空が managed（アプリが用意する Ollama。未導入は reason not_installed で通信しない）で、
// 指定すれば external（localhost 以外は同意が要る）。実装は listLocalModels。
// キーは資格情報ストアから読み、呼び出し元（フロント）には渡さない。呼ぶのは、プロバイダ・Base URL を選んだとき、API キーの登録・変更後、
// ChatGPT のログイン後、利用者が「更新」を押したときだけで、定期的には呼ばない（呼び出し側の責務）。
//
// 取得できないとき（キー未登録・同意前・不正な Base URL・Models API が無い・通信できない・権限がない）は、エラーではなく
// Models が空で Reason と短い Message を持つ結果を返し、画面は手入力へ切り替える。
// 同意前の送信先（プロバイダ＋送信先ごとに判定する）・使えない Base URL には、一覧の取得も送らない（画面は consent_required を同意の画面を出す契機にする）。
// ログには件数と結果だけを残す（キー・応答の中身は残さない）。
func (s *Service) ListAIModels(ctx context.Context, provider, baseURL string) (dto.AIModelList, error) {
	if !validProvider(provider) {
		return dto.AIModelList{}, errBadProvider()
	}
	baseURL = strings.TrimSpace(baseURL)
	if provider == providerLocal {
		return s.listLocalModels(ctx, baseURL)
	}
	if provider != providerOpenAI {
		baseURL = ""
	}
	if baseURL != "" {
		if err := aiinput.ValidateBaseURL(baseURL); err != nil {
			return s.noModels(provider, dto.AIModelsInvalidBaseURL, "Base URL が使えないため、モデル一覧を取得できません。モデル名を入力してください。"), nil
		}
	}
	consented, err := s.hasConsent(ctx, provider, baseURL)
	if err != nil {
		return dto.AIModelList{}, err
	}
	if !consented {
		return s.noModels(provider, dto.AIModelsConsentRequired, "送信先への同意前は、モデル一覧を取得しません。モデル名を入力してください。"), nil
	}
	cred, ok, err := s.credential(ctx, provider)
	if err != nil {
		var ae *apperr.Error
		if errors.As(err, &ae) && ae.Code == apperr.AINotConfigured {
			return s.noModels(provider, dto.AIModelsNoCredentials, noCredentialsMessage(provider)), nil
		}
		if errors.As(err, &ae) && ae.Code == apperr.Offline {
			reason, msg := aiinput.ModelsFailure(err)
			return s.noModels(provider, reason, msg), nil
		}
		return dto.AIModelList{}, err
	}
	if !ok {
		return s.noModels(provider, dto.AIModelsNoCredentials, noCredentialsMessage(provider)), nil
	}

	lctx, cancel := context.WithTimeout(ctx, aiModelsTimeout)
	defer cancel()
	return s.fetchModels(lctx, ctx, provider, baseURL, cred)
}

// fetchModels は provider のモデル一覧を取得する（lctx は期限つき、ctx は呼び出し元。ctx の終了は CANCELLED）。取得できないときは Reason つきの空の一覧。
func (s *Service) fetchModels(lctx, ctx context.Context, provider, baseURL, cred string) (dto.AIModelList, error) {
	list, err := s.d.AIModels(lctx, provider, baseURL, cred)
	if err != nil {
		if ctx.Err() != nil {
			return dto.AIModelList{}, apperr.Wrap(apperr.Cancelled, "モデル一覧の取得を中止しました。", err)
		}
		reason, msg := aiinput.ModelsFailure(err)
		s.d.Logger.Warn("ai models list failed", "provider", provider, "reason", reason)
		return dto.AIModelList{Provider: provider, Models: []dto.AIModel{}, Reason: reason, Message: msg}, nil
	}
	out := make([]dto.AIModel, len(list))
	for i, m := range list {
		out[i] = dto.AIModel{ID: m.ID, DisplayName: m.DisplayName, Efforts: m.Efforts}
	}
	s.d.Logger.Info("ai models listed", "provider", provider, "count", len(out))
	return dto.AIModelList{Provider: provider, Models: out}, nil
}

func (s *Service) noModels(provider, reason, msg string) dto.AIModelList {
	return dto.AIModelList{Provider: provider, Models: []dto.AIModel{}, Reason: reason, Message: msg}
}

func noCredentialsMessage(provider string) string {
	if provider == providerChatGPT {
		return "ChatGPT にログインすると、使えるモデルを取得できます。モデル名を入力することもできます。"
	}
	return "API キーを登録すると、使えるモデルを取得できます。モデル名を入力することもできます。"
}
