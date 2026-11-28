// Package aiinput は AI へ渡すメールの内容（スレッドの本文・差出人・添付の手がかり）と、Inbox Zero の判定の選択肢を作る純粋な補助関数を持つ。
// aicore（Service）が大きくなりすぎないよう、Service に依らないものをここに置く。
package aiinput

import (
	"context"
	"errors"
	"net"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ousiassllc/nefos-inbox/internal/ai"
	"github.com/ousiassllc/nefos-inbox/internal/app/apperr"
	"github.com/ousiassllc/nefos-inbox/internal/app/dto"
	"github.com/ousiassllc/nefos-inbox/internal/domain/model"
	"github.com/ousiassllc/nefos-inbox/internal/domain/textconv"
	"github.com/ousiassllc/nefos-inbox/internal/gmail"
)

// BodyRunes は Inbox Zero の判定で AI へ送る本文の長さ（先頭からの文字数）。
const BodyRunes = 500

// Gmail のシステムラベル。
const (
	LabelDraft  = "DRAFT" // 下書きメッセージに付く
	LabelSent   = "SENT"
	LabelUnread = "UNREAD"
)

// Criteria は判定の AI へ渡す選択肢（この順。Jev でもチャット系のモデルでも同じ）。無効にした理由も含めて常に全部を渡す
// （無効な理由のメールを「その他」へ寄せず、その理由として選ばせたうえで候補から外すため）。
// カレンダーの招待・通知も AI が判定する（端末内の MIME・送信元の判定はしない。#377）。通知と重ならないよう、通知の説明でカレンダーを除く。
var Criteria = []ai.Criterion{
	{Name: dto.InboxZeroReasonAd, Description: "商品・サービス・イベントなどを宣伝・販売促進する広告のメール（セール、キャンペーン、クーポン、新商品の案内など）。"},
	{Name: dto.InboxZeroReasonCalendar, Description: "Google カレンダー・Outlook などの予定の招待・変更・取り消し・返信・リマインダー・予定表のまとめの通知。"},
	{Name: dto.InboxZeroReasonNotification, Description: "サービスやシステムが自動で送る通知だけのメール（ログイン通知、配送状況、利用明細、SNS の更新通知など）で、返信も対応も求めないもの。カレンダーの招待・通知は除く。"},
	{Name: dto.InboxZeroReasonNewsletter, Description: "定期的に配信されるニュースレター・メールマガジン・記事やブログの更新まとめ。"},
	{Name: ChoiceOther, Description: "通常のメール・個人宛て・重要なもの・判断できない。"},
}

// ChoiceOther は「どれにも当てはまらない」選択肢の名前。候補にならない。
const ChoiceOther = "other"

// Instructions は判定の AI への指示。
const Instructions = "このメールの種類として、最も当てはまるものを 1 つ選んでください。"

// ContextMessages はスレッドの DRAFT でないメッセージを、古い順のままプレーンテキスト化して返す（添付はファイル名とサイズだけ）。
func ContextMessages(thread gmail.ThreadFull) []ai.ContextMessage {
	msgs := make([]ai.ContextMessage, 0, len(thread.Messages))
	for _, m := range thread.Messages {
		if slices.Contains(m.LabelIDs, LabelDraft) {
			continue
		}
		msgs = append(msgs, ai.ContextMessage{From: SenderLabel(m), Date: m.InternalDate, Text: MessageText(m), Attachments: AttachmentRefs(m.Attachments)})
	}
	return msgs
}

func AttachmentRefs(atts []model.AttachmentInfo) []ai.AttachmentRef {
	refs := make([]ai.AttachmentRef, len(atts))
	for i, a := range atts {
		refs[i] = ai.AttachmentRef{Filename: a.Filename, Size: a.Size}
	}
	return refs
}

func SenderLabel(m gmail.MessageFull) string {
	if m.From.Name != "" {
		return m.From.Name
	}
	return m.From.Email
}

func MessageText(m gmail.MessageFull) string {
	if m.BodyHTML != "" {
		return strings.TrimSpace(textconv.HTMLToText(m.BodyHTML))
	}
	return strings.TrimSpace(m.BodyText)
}

// ClassifyState は判定の AI へ送る内容（差出人・件名・メールの形の手がかり・本文の先頭 500 文字だけ）。Jev でもチャット系のモデルでも同じ。
// 手がかり（Content-Types・Attachments）は事実を並べるだけで、カレンダーかどうかは AI が決める（端末内では決めない）。
func ClassifyState(m gmail.MessageFull) string {
	var b strings.Builder
	b.WriteString("From: " + Sender(m) + "\nSubject: " + m.Subject + "\n")
	if len(m.PartTypes) > 0 {
		b.WriteString("Content-Types: " + strings.Join(m.PartTypes, ", ") + "\n")
	}
	if a := attachmentHints(m); a != "" {
		b.WriteString("Attachments: " + a + "\n")
	}
	b.WriteString("\n" + FirstRunes(MessageText(m), BodyRunes))
	return b.String()
}

// attachmentHints は添付の種類と拡張子（".ics" など）をカンマ区切りで返す。ファイル名・サイズは AI へ送らない。
func attachmentHints(m gmail.MessageFull) string {
	hints := make([]string, 0, len(m.Attachments))
	for _, a := range m.Attachments {
		h := strings.ToLower(a.MimeType)
		if ext := strings.ToLower(path.Ext(a.Filename)); ext != "" {
			h += " (" + ext + ")"
		}
		hints = append(hints, h)
	}
	return strings.Join(hints, ", ")
}

// Sender は差出人を「表示名 <アドレス>」（表示名が無ければアドレス）にする。
func Sender(m gmail.MessageFull) string {
	if m.From.Name == "" {
		return m.From.Email
	}
	return m.From.Name + " <" + m.From.Email + ">"
}

// FirstRunes は s の先頭 n 文字（rune）。
func FirstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// NonDraftMessages はスレッドの下書きでないメッセージ（古い順）。
func NonDraftMessages(thread gmail.ThreadFull) []gmail.MessageFull {
	return slices.DeleteFunc(slices.Clone(thread.Messages), func(m gmail.MessageFull) bool { return slices.Contains(m.LabelIDs, LabelDraft) })
}

// IsOwnMessage は m が自分の送信（SENT ラベル、または差出人が自分のアドレス。大文字小文字を区別しない）か。
func IsOwnMessage(m gmail.MessageFull, email string) bool {
	return slices.Contains(m.LabelIDs, LabelSent) || (email != "" && strings.EqualFold(strings.TrimSpace(m.From.Email), email))
}

// ValidateHistory は会話の形と長さを検証して ai.ThreadTurn にする。nil・空は空の会話。
// 形: user から始まり assistant で終わる交互の並びで、text は空でない。長さ: 20 件（10 往復）以内・text の合計 20,000 文字以内。
func ValidateHistory(history []dto.ThreadAITurn) ([]ai.ThreadTurn, error) {
	bad := func(msg string) error { return apperr.New(apperr.Validation, msg) }
	if len(history) > ai.MaxHistoryTurns {
		return nil, bad("会話が長すぎます（最新の 10 往復までです）。")
	}
	if len(history)%2 != 0 {
		return nil, bad("会話の形が正しくありません（質問と回答が対になっている必要があります）。")
	}
	turns := make([]ai.ThreadTurn, len(history))
	total := 0
	for i, t := range history {
		want := ai.RoleUser
		if i%2 == 1 {
			want = ai.RoleAssistant
		}
		if t.Role != want {
			return nil, bad("会話の形が正しくありません（質問と回答が交互である必要があります）。")
		}
		if strings.TrimSpace(t.Text) == "" {
			return nil, bad("会話の形が正しくありません（空の発言があります）。")
		}
		total += utf8.RuneCountInString(t.Text)
		turns[i] = ai.ThreadTurn{Role: t.Role, Text: t.Text}
	}
	if total > ai.MaxHistoryRunes {
		return nil, bad("会話が長すぎます（合計 20,000 文字までです）。")
	}
	return turns, nil
}

// ValidateBaseURL は Base URL を検証する（ai.ValidateBaseURL と同じ規則。URL はメッセージに含めない）。
func ValidateBaseURL(raw string) error {
	err := ai.ValidateBaseURL(raw)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ai.ErrInsecureBaseURL):
		return apperr.Wrap(apperr.Validation, "http:// は localhost／127.0.0.1／::1 のときだけ使えます。", err)
	default:
		return apperr.Wrap(apperr.Validation, "Base URL は http:// または https:// で始まる、認証情報・クエリ・フラグメントを含まない URL を指定してください。", err)
	}
}

// ModelsFailure は一覧の取得エラーを理由コードと短い表示にする。メッセージにキー・応答本文・URL は含めない。
func ModelsFailure(err error) (reason, msg string) {
	var netErr net.Error
	switch {
	case errors.Is(err, ai.ErrInvalidBaseURL):
		return dto.AIModelsInvalidBaseURL, "Base URL が使えないため、モデル一覧を取得できません。モデル名を入力してください。"
	case errors.Is(err, ai.ErrModelsUnsupported):
		return dto.AIModelsUnsupported, "このサービスにはモデル一覧の取得機能がないようです。モデル名を入力してください。"
	case errors.Is(err, ai.ErrUnauthorized):
		return dto.AIModelsUnauthorized, "認証または権限がないため、モデル一覧を取得できません。キー（またはログイン）を確認するか、モデル名を入力してください。"
	case errors.Is(err, ai.ErrRateLimited):
		return dto.AIModelsRateLimited, "利用制限のため、モデル一覧を取得できません。しばらくしてから更新するか、モデル名を入力してください。"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr):
		return dto.AIModelsNetwork, "通信できないため、モデル一覧を取得できません。モデル名を入力してください。"
	}
	return dto.AIModelsFailed, "モデル一覧を取得できませんでした。モデル名を入力してください。"
}
