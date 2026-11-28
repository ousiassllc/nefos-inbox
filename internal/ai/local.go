package ai

import (
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProviderLocal はローカルの AI サーバー（アプリが用意する Ollama、または利用者が動かしている Ollama・LM Studio など。#458）。
// OpenAI 互換の Chat Completions で呼ぶ。API キーは任意で、空なら Authorization ヘッダを付けない。Base URL は必須。
const ProviderLocal = "local"

// localHTTPTimeout は ProviderLocal の http.Client のタイムアウト。ローカルの生成は数分かかることがあるので、既定の 2 分にしない
// （実際の期限は呼び出し側の ctx。これは ctx が無期限だった場合の保険）。
const localHTTPTimeout = 10 * time.Minute

// httpClientFor は provider 用の既定の http.Client（リダイレクトは追従しない）。
func httpClientFor(provider string) *http.Client {
	hc := NewHTTPClient()
	if provider == ProviderLocal {
		hc.Timeout = localHTTPTimeout
	}
	return hc
}

// IsLoopbackBaseURL は raw のホストが localhost／127.0.0.1／::1 に厳密一致するか（loopback＝PC の外へ出ない送信先）。解釈できない URL は false。
func IsLoopbackBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && isLoopbackHost(u.Hostname())
}

// bearer は Authorization ヘッダ。apiKey が空なら付けない（ローカルの AI サーバーはキーが任意）。
func bearer(apiKey string) map[string]string {
	if apiKey == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + apiKey}
}

// requireLocalBase は ProviderLocal の Base URL が空でないことを確かめる（空の既定の送信先は無い）。
func requireLocalBase(provider, baseURL string) error {
	if provider == ProviderLocal && strings.TrimSpace(baseURL) == "" {
		return ErrInvalidBaseURL
	}
	return nil
}
