//go:build e2e && !production

package e2e

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ousiassllc/nefos-inbox/internal/oauth"
)

// 偽の Google（IdP）が受け付ける・返す固定値。E2E スクリプト（build/android/e2e/run.sh）がこのメールアドレスをログで探す。
const (
	// IdPClientID・IdPClientSecret は、シード（seed.go）の OAuth クライアントの Client ID・Client Secret と同じ値。
	IdPClientID     = "e2e-client-id"
	IdPClientSecret = "e2e-client-secret" //nolint:gosec // 本物の資格情報ではない E2E 用の固定値
	// IdPEmail・IdPName は、偽の IdP が userinfo で返す利用者。シードのアカウント（AccountEmail）とは別のアドレス。
	IdPEmail = "e2e-login@example.com"
	IdPName  = "E2E Login"
)

// IdP は E2E 用の偽の Google。実フロー（internal/oauth の Authorize）が使う認可・トークン・userinfo・失効の 4 つのエンドポイントを、
// 127.0.0.1 の一時ポートで返す。本物のように PKCE（S256）・state・redirect_uri（127.0.0.1 のループバック）・Client Secret・認可コードの 1 回使いを検証するので、
// アプリの認可の流れ（ブラウザ→ループバックのコールバック→トークン交換→userinfo）が、本物の Google なしで通る。
// 認可コードの交換は、アプリが背面（ブラウザが前面）のまま行われても、ループバック宛なので OS の通信制限を受けない。つまり
// 「背面での名前解決の失敗」（docs/environment/android.md §20）は再現しない。ここで確かめるのは、ブラウザから戻るまでの流れと完了ページ・戻る導線である。
type IdP struct {
	ln  net.Listener
	srv *http.Server

	mu     sync.Mutex
	grants map[string]grant // 認可コード → 発行時の要求。交換で消える
	tokens map[string]bool  // 発行したアクセストークン
}

type grant struct {
	challenge   string
	redirectURI string
}

// StartIdP は 127.0.0.1 の一時ポートで偽の IdP を始める。Close で止める。
func StartIdP() (*IdP, error) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("e2e: fake IdP listen: %w", err)
	}
	p := &IdP{ln: ln, grants: map[string]grant{}, tokens: map[string]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	mux.HandleFunc("/userinfo", p.userinfo)
	mux.HandleFunc("/revoke", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

// Close は偽の IdP を止める。
func (p *IdP) Close() { _ = p.srv.Close() }

func (p *IdP) url(path string) string { return "http://" + p.ln.Addr().String() + path }

// Options は opts の認可・トークン・userinfo・失効の URL を、この IdP に差し替えたものを返す。
func (p *IdP) Options(opts oauth.Options) oauth.Options {
	opts.AuthURL, opts.TokenURL, opts.UserinfoURL, opts.RevokeURL = p.url("/authorize"), p.url("/token"), p.url("/userinfo"), p.url("/revoke")
	return opts
}

// authorize は認可の要求を検証し、同意済みとして redirect_uri へ code と state を付けて 302 で返す（同意画面は出さない）。
func (p *IdP) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	redirect, err := url.Parse(q.Get("redirect_uri"))
	switch {
	case q.Get("response_type") != "code", q.Get("client_id") != IdPClientID:
		http.Error(w, "invalid_request", http.StatusBadRequest)
	case q.Get("code_challenge_method") != "S256", q.Get("code_challenge") == "", q.Get("state") == "":
		http.Error(w, "invalid_request: pkce/state", http.StatusBadRequest)
	case !strings.Contains(q.Get("scope"), "https://www.googleapis.com/auth/gmail.modify"):
		http.Error(w, "invalid_scope", http.StatusBadRequest)
	case err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.Port() == "" || redirect.Path != "/callback":
		http.Error(w, "redirect_uri_mismatch", http.StatusBadRequest) // 本物の Google もデスクトップ種別ではループバックだけを許す
	default:
		code := randomToken()
		p.mu.Lock()
		p.grants[code] = grant{challenge: q.Get("code_challenge"), redirectURI: q.Get("redirect_uri")}
		p.mu.Unlock()
		cb := *redirect
		v := cb.Query()
		v.Set("code", code)
		v.Set("state", q.Get("state"))
		cb.RawQuery = v.Encode()
		http.Redirect(w, r, cb.String(), http.StatusFound) //nolint:gosec // redirect_uri は上の検査で 127.0.0.1 のループバックに限っている
	}
}

// token は認可コードをトークンに交換する。コードは 1 回だけ使え、code_verifier・redirect_uri・Client ID・Client Secret が合わなければ拒否する。
func (p *IdP) token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.ParseForm() != nil {
		http.Error(w, "invalid_request", http.StatusBadRequest)
		return
	}
	f := r.PostForm
	if f.Get("client_id") != IdPClientID || subtle.ConstantTimeCompare([]byte(f.Get("client_secret")), []byte(IdPClientSecret)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_client"})
		return
	}
	p.mu.Lock()
	g, ok := p.grants[f.Get("code")]
	delete(p.grants, f.Get("code")) // 成否にかかわらず 1 回で使い切る
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(f.Get("code_verifier")))
	if f.Get("grant_type") != "authorization_code" || !ok || g.redirectURI != f.Get("redirect_uri") ||
		subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(g.challenge)) != 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
		return
	}
	access := randomToken()
	p.mu.Lock()
	p.tokens[access] = true
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  access,
		"refresh_token": "e2e-login-refresh-" + randomToken(),
		"token_type":    "Bearer",
		"expires_in":    3600,
		"scope":         strings.Join(oauth.Scopes, " "),
	})
}

// userinfo は発行したアクセストークンに対して、固定の利用者（IdPEmail）を返す。
func (p *IdP) userinfo(w http.ResponseWriter, r *http.Request) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	p.mu.Lock()
	valid := ok && p.tokens[token]
	p.mu.Unlock()
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"email": IdPEmail, "name": IdPName})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func randomToken() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("e2e: random: %v", err)) // crypto/rand の失敗はテスト用モードでは続行できない
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
