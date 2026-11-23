// Package callbackpage は、ブラウザの認可（OAuth・Sign in with ChatGPT）の 127.0.0.1 待受が返す完了ページを提供する。
// 外部リソースを使えない（CSP が default-src 'none'）ため、スタイルはページに直接埋め込む。
package callbackpage

import (
	"fmt"
	"html"
	"net/http"
	"runtime"
)

// AndroidReturnURL は Android の完了ページが「Nefos Inbox に戻る」リンクと自動遷移の行き先に使う URL。
// アプリの MainActivity にある BROWSABLE の intent-filter（scheme・host。build/android/app/src/main/AndroidManifest.xml）と
// 同じ値にする（一致は callbackpage_test.go の TestAndroidReturnURLMatchesManifest が検証する）。
// これはブラウザからアプリを前面に出すためだけの URL で、認可コード・state・トークンは載せない。
// Google への redirect_uri ではない（redirect_uri は今までどおり http://127.0.0.1:<ポート>/callback。docs/environment/android.md §14.1）。
const AndroidReturnURL = "jp.ousias.hitomado://auth-return"

// page は完了ページの HTML。色・フォント・サイズ・行間・角丸・余白は Ousiass デザインガイドのトークン（frontend/src/styles/tokens.css）の値の複製で、
// 同期は callbackpage_test.go の TestPageValuesMatchTokens が tokens.css を読んで検証する（トークンを変えたらここも直す）。
// 戻るリンク（Android のみ）は、ガイドの primary ボタン（--accent の地・--bone の文字・--r-md・--s-4 の左右の余白）の見た目で、押しやすさのため高さを 44px 以上にする（上下 12px）。
// 色は :root の変数に置き、ダークは prefers-color-scheme で切り替える。フォントは --font-sans の Sora + Noto Sans JP を端末のフォント名
// （"Sora Variable" は同梱した @font-face の名前なので使えない）で指定する。CSP が外部リソースを禁じるため同梱はしない。
// サイズ・行間はガイドのスケール（見出し 18px/1.7・本文 16px/1.8）。
// msg は呼び出し元次第で長くなりうるので、本文は overflow-wrap:anywhere で横あふれを止める。viewport のスクロールバーはガイド §10 の設計
// （8px・矢印なし・--mute の thumb・hover/active は --ink 35%/65%）で、frontend/src/styles/scrollbar.css と同じ構造にする。
// page は Fprintf の書式で、%s は順に head の追加（自動遷移の meta。無ければ空）・msg・戻るリンク（無ければ空）。% は %% と書く。
const page = `<!doctype html>
<html lang="ja"><head><meta charset="utf-8"><title>Nefos Inbox</title>%s
<style>
:root{--ink:#050505;--bone:#f5f5f4;--surface:#ffffff;--border:#e7e7e5;--mute:#6b6b6e;--accent:#050505}
@media (prefers-color-scheme:dark){:root{--ink:#ffffff;--bone:#050505;--surface:#141414;--border:#2a2a2a;--mute:#8b8b8e;--accent:#f5f5f4}}
body{font-family:Sora,"Noto Sans JP",sans-serif;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0;background:var(--bone);color:var(--ink)}
main{background:var(--surface);padding:24px;border-radius:12px;border:1px solid var(--border);max-width:28rem}
h1{font-size:18px;line-height:1.7;font-weight:600;margin:0 0 12px}
p{margin:0;font-size:16px;line-height:1.8;overflow-wrap:anywhere}
a.back{display:block;box-sizing:border-box;margin:16px 0 0;padding:12px 16px;border-radius:8px;background:var(--accent);color:var(--bone);font-size:16px;line-height:1.8;font-weight:600;text-align:center;text-decoration:none}
@media (forced-colors:none){
:root,:root *{scrollbar-width:thin;scrollbar-color:var(--mute) transparent}
@supports selector(::-webkit-scrollbar){
:root,:root *{scrollbar-width:auto;scrollbar-color:auto}
::-webkit-scrollbar{width:8px;height:8px}
::-webkit-scrollbar-track,::-webkit-scrollbar-corner{background:transparent}
::-webkit-scrollbar-button{display:none}
::-webkit-scrollbar-thumb{background:var(--mute);border-radius:999px}
::-webkit-scrollbar-thumb:hover{background:color-mix(in srgb,var(--ink) 35%%,var(--mute))}
::-webkit-scrollbar-thumb:active{background:color-mix(in srgb,var(--ink) 65%%,var(--mute))}
}}
</style>
</head><body><main><h1>Nefos Inbox</h1><p>%s</p>%s</main></body></html>
`

// Write は msg を載せた完了ページを w に書く。msg は HTML エスケープする。
// Android では、アプリへ戻るリンク（AndroidReturnURL）と、同じ行き先への自動遷移（meta refresh）を付ける。Android 以外（Windows など）は付けない。
func Write(w http.ResponseWriter, msg string) {
	write(w, msg, returnURL())
}

// returnURL は今の OS で完了ページに付ける「アプリへ戻る」URL。付けない OS は空。
func returnURL() string {
	if runtime.GOOS == "android" {
		return AndroidReturnURL
	}
	return ""
}

// write は Write の本体。backURL が空でなければ戻るリンクと自動遷移を付ける（テストが OS を問わず両方を確かめられるよう分けてある）。
//
// 戻るリンクが主で、自動遷移は補助（best-effort）。Chrome はユーザーの操作なしにアプリを起動する外部ナビゲーションを無視することがあり
// （docs/environment/android.md §14.2）、無視されたときはページがそのまま残るので、利用者はリンクをタップすればよい。
// meta refresh は CSP（default-src 'none'）の対象外なので、スクリプトを許可せずに済む。
func write(w http.ResponseWriter, msg, backURL string) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Connection", "close")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	var refresh, link string
	if backURL != "" {
		u := html.EscapeString(backURL)
		refresh = `<meta http-equiv="refresh" content="0;url=` + u + `">`
		link = `<a class="back" href="` + u + `">Nefos Inbox に戻る</a>`
	}
	// 書き込み失敗（クライアントの切断）は認可結果の受け取りに影響しない。
	_, _ = fmt.Fprintf(w, page, refresh, html.EscapeString(msg), link)
}
