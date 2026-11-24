// Package localconn はローカルAI（Ollama。provider "local"。#458）のつなぎ方と、その前提の検査・送信先の解決を扱う。
// aicore（Service）が膨らまないよう、Service に依らない部分をここに置く。
package localconn

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/capability"
	"github.com/ousiassllc/nefos-inbox/internal/localai"
)

// GenerateTimeout はローカルAIの生成（下書き・要約・質問・Inbox Zero の 1 件の判定）のタイムアウト。ほかのプロバイダは 60 秒。
const GenerateTimeout = 300 * time.Second

// installHint は managed で未導入のときの案内。
const installHint = "ローカルAI（Ollama）を設定画面の「AI のつなぎ先」で導入してください。"

// Conn はローカルAIのつなぎ方（保存する値）。
type Conn struct {
	Mode    string // dto.LocalModeManaged／dto.LocalModeExternal。空は managed
	BaseURL string // external の Base URL。managed は空
}

// Managed はアプリが用意する Ollama を使うか（Mode が空・不明も managed）。
func (c Conn) Managed() bool { return c.Mode != dto.LocalModeExternal }

// Loopback は送信先が PC の外へ出ないか（managed、または host が localhost／127.0.0.1／::1 の external）。
func (c Conn) Loopback() bool {
	return c.Managed() || (ai.IsLoopbackBaseURL(c.BaseURL) && ai.ValidateBaseURL(c.BaseURL) == nil)
}

// ValidMode は mode が保存できるつなぎ方か。
func ValidMode(mode string) bool {
	return mode == dto.LocalModeManaged || mode == dto.LocalModeExternal
}

// Usable は managed でこの OS の capability と Runtime が使えるか（external は capability だけ）。
func Usable(caps capability.Capabilities, rt localai.Runtime, c Conn) bool {
	return caps.SupportsLocalAI && (!c.Managed() || rt != nil)
}

// Config は AIConfig.local。ready は managed が導入済みでモデルが 1 つ以上取得済み、external が Base URL を持ち有効なとき（使えない OS は false）。
func Config(caps capability.Capabilities, rt localai.Runtime, c Conn) dto.AILocalConnection {
	out := dto.AILocalConnection{Mode: dto.LocalModeManaged, BaseURL: ""}
	if !c.Managed() {
		out.Mode, out.BaseURL = dto.LocalModeExternal, c.BaseURL
	}
	if !Usable(caps, rt, c) {
		return out
	}
	if !c.Managed() {
		out.Ready = c.BaseURL != "" && ai.ValidateBaseURL(c.BaseURL) == nil
		return out
	}
	st := rt.Status()
	out.Ready = st.Installed && slices.ContainsFunc(st.Models, func(m localai.Model) bool { return m.Installed })
	return out
}

// Check はローカルAIを使う前提（この OS で使える・managed は導入済み・external は Base URL が有効）を検査する。
// UNSUPPORTED／AI_NOT_CONFIGURED／VALIDATION を AppError で返す。同意・キー・モデルは見ない。
func Check(caps capability.Capabilities, rt localai.Runtime, c Conn) error {
	if !caps.SupportsLocalAI {
		return apperr.New(apperr.Unsupported, "ローカルAIはこの OS では使えません。")
	}
	if !c.Managed() {
		switch {
		case c.BaseURL == "":
			return apperr.New(apperr.AINotConfigured, "ローカルAIの Base URL を、設定画面の「AI のつなぎ先」で設定してください。")
		case ai.ValidateBaseURL(c.BaseURL) != nil:
			return apperr.New(apperr.Validation, "保存済みの Base URL は使えません。設定画面の「AI のつなぎ先」で https:// の URL（または localhost の http://）に直してください。")
		}
		return nil
	}
	if rt == nil {
		return apperr.New(apperr.Unsupported, "ローカルAIはこの OS では使えません。")
	}
	st := rt.Status()
	switch {
	case !st.Supported:
		return apperr.New(apperr.Unsupported, unsupportedMessage(st.UnsupportedReason))
	case !st.Installed:
		return apperr.New(apperr.AINotConfigured, installHint)
	}
	return nil
}

func unsupportedMessage(reason string) string {
	if reason == "" {
		return localai.ErrUnsupported.Error()
	}
	return reason
}

// Open は provider の送信先の Base URL と、使い終わったら呼ぶ release を返す。
// managed のローカルAIは Acquire で Ollama を起動して（アイドルで止めないよう保持して）その URL を返す。それ以外は baseURL をそのまま返す（release は何もしない）。
// 失敗は AppError（未導入 AI_NOT_CONFIGURED・使えない環境 UNSUPPORTED・起動できない AI_FAILED）。ctx の終了は ctx のエラーのまま返す。
func Open(ctx context.Context, rt localai.Runtime, provider, mode, baseURL string) (string, func(), error) {
	noop := func() {}
	if provider != ai.ProviderLocal || !(Conn{Mode: mode}).Managed() {
		return baseURL, noop, nil
	}
	if rt == nil {
		return "", noop, apperr.New(apperr.Unsupported, "ローカルAIはこの OS では使えません。")
	}
	base, release, err := rt.Acquire(ctx)
	switch {
	case err == nil:
		return base, release, nil
	case ctx.Err() != nil:
		return "", noop, err
	}
	return "", noop, MapError(err)
}

// MapError は localai のエラーを AppError にする（バインディングの LocalAIService と aicore で共通）。
// 未導入 AI_NOT_CONFIGURED・使えない環境 UNSUPPORTED・カタログ外のモデル VALIDATION・ジョブ中 VALIDATION（details.reason="busy"）・
// 空き不足 VALIDATION（details.reason="no_space"・requiredBytes・freeBytes）。それ以外は AI_FAILED（起動・取得に失敗した）。
func MapError(err error) *apperr.Error {
	var ae *apperr.Error
	var ns *localai.NoSpaceError
	switch {
	case errors.As(err, &ae):
		return ae
	case errors.Is(err, localai.ErrUnsupported):
		return apperr.Wrap(apperr.Unsupported, localai.ErrUnsupported.Error(), err)
	case errors.Is(err, localai.ErrUnknownModel):
		return apperr.Wrap(apperr.Validation, localai.ErrUnknownModel.Error(), err)
	case errors.Is(err, localai.ErrNotInstalled):
		return apperr.Wrap(apperr.AINotConfigured, installHint, err)
	case errors.Is(err, localai.ErrBusy):
		return apperr.Wrap(apperr.Validation, localai.ErrBusy.Error(), err).WithDetail(apperr.DetailReason, apperr.ReasonBusy)
	case errors.As(err, &ns):
		return apperr.Wrap(apperr.Validation, ns.Error(), err).
			WithDetail(apperr.DetailReason, apperr.ReasonNoSpace).
			WithDetail("requiredBytes", ns.Required).WithDetail("freeBytes", ns.Free)
	case errors.Is(err, context.Canceled):
		return apperr.Wrap(apperr.Cancelled, "キャンセルしました。", err)
	}
	return apperr.Wrap(apperr.AIFailed, "ローカルAI（Ollama）を使えませんでした。起動や取得に失敗しています。", err)
}
