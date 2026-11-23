//go:build e2e && !production

package e2etests

import (
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/e2e"
	"github.com/ousiassllc/nefos-inbox/internal/oauth"
)

func startIdP(t *testing.T) *e2e.IdP {
	t.Helper()
	idp, err := e2e.StartIdP()
	require.NoError(t, err)
	t.Cleanup(idp.Close)
	return idp
}

// browser は OpenBrowser の代わりに、認可 URL を GET して 302 に従い、127.0.0.1 のコールバックまで届ける（本物の端末ではブラウザがやること）。
// 最終ページの本文を pages へ送る。
func browser(t *testing.T, pages chan<- string) func(string) error {
	t.Helper()
	return func(authURL string) error {
		resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(authURL)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		pages <- string(b)
		return nil
	}
}

// 実フロー（oauth.Authorize）が、偽の IdP に対して、認可 → ループバックのコールバック → トークン交換 → userinfo まで通る。
// これが Android の E2E（build/android/e2e/run.sh）でアプリの中を通る経路と同じで、token_exchange で止まらないことの Go 側の回帰テスト（#368）。
func TestAuthorizeCompletesAgainstFakeIdP(t *testing.T) {
	idp := startIdP(t)
	pages := make(chan string, 1)
	flow := oauth.NewFlow(idp.Options(oauth.Options{OpenBrowser: browser(t, pages), Timeout: 20 * time.Second}))

	res, err := flow.Authorize(t.Context(), oauth.ClientConfig{ClientID: e2e.IdPClientID, ClientSecret: e2e.IdPClientSecret})
	require.NoError(t, err)
	assert.Equal(t, e2e.IdPEmail, res.Email)
	assert.Equal(t, e2e.IdPName, res.Name)
	assert.NotEmpty(t, res.RefreshToken)

	// ブラウザに出た完了ページ（Android 以外では戻るリンクを付けない。付くのは GOOS=android のビルドだけ）。
	page := <-pages
	assert.Contains(t, page, "認可の操作が完了しました")
	assert.NotContains(t, page, "戻る</a>")
}

// Client Secret が違えばトークン交換は拒否され、Authorize は失敗する（偽の IdP が検証している＝緑が「何でも通る」ではないことの確認）。
func TestAuthorizeFailsWithWrongClientSecret(t *testing.T) {
	idp := startIdP(t)
	pages := make(chan string, 1)
	flow := oauth.NewFlow(idp.Options(oauth.Options{OpenBrowser: browser(t, pages), Timeout: 20 * time.Second}))

	_, err := flow.Authorize(t.Context(), oauth.ClientConfig{ClientID: e2e.IdPClientID, ClientSecret: "wrong"})
	require.ErrorIs(t, err, oauth.ErrClientRejected)
}

// reply は GET の応答のうち、確かめるものだけ（本文は閉じてから返す）。
type reply struct {
	status   int
	location string
}

func get(t *testing.T, client *http.Client, rawURL string) reply {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return reply{status: resp.StatusCode, location: resp.Header.Get("Location")}
}

func authorizeQuery(redirect, state, challenge string) url.Values {
	return url.Values{
		"client_id": {e2e.IdPClientID}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {strings.Join(oauth.Scopes, " ")}, "state": {state},
		"code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
}

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// 偽の IdP は、本物の Google と同じく、ループバック以外の redirect_uri・PKCE の無い要求を拒否し、認可コードを 1 回しか使わせず、
// code_verifier が合わない交換を拒否する。
func TestFakeIdPEnforcesRedirectPKCEAndSingleUse(t *testing.T) {
	idp := startIdP(t)
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	base := idp.Options(oauth.Options{})

	// ループバック以外の redirect_uri・PKCE の無い要求は 400。
	q := authorizeQuery("https://example.com/callback", "s", challengeOf("v"))
	assert.Equal(t, http.StatusBadRequest, get(t, noFollow, base.AuthURL+"?"+q.Encode()).status)
	q = authorizeQuery("http://127.0.0.1:1234/callback", "s", "")
	assert.Equal(t, http.StatusBadRequest, get(t, noFollow, base.AuthURL+"?"+q.Encode()).status)

	// 正しい要求は 302 で redirect_uri へ code と state を返す。
	redirect := "http://127.0.0.1:1234/callback"
	q = authorizeQuery(redirect, "state-1", challengeOf("verifier-1"))
	resp := get(t, noFollow, base.AuthURL+"?"+q.Encode())
	require.Equal(t, http.StatusFound, resp.status)
	loc, err := url.Parse(resp.location)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:1234", loc.Host)
	assert.Equal(t, "state-1", loc.Query().Get("state"))
	code := loc.Query().Get("code")
	require.NotEmpty(t, code)

	exchange := func(code, verifier string) int {
		form := url.Values{
			"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirect},
			"client_id": {e2e.IdPClientID}, "client_secret": {e2e.IdPClientSecret},
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, base.TokenURL, strings.NewReader(form.Encode()))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = r.Body.Close()
		return r.StatusCode
	}
	// 違う code_verifier は拒否され、その時点でコードは使い切られる（総当たりさせない）。
	assert.Equal(t, http.StatusBadRequest, exchange(code, "wrong-verifier"))
	assert.Equal(t, http.StatusBadRequest, exchange(code, "verifier-1"))

	// 新しいコードは、正しい verifier で 1 回だけ交換できる。
	resp = get(t, noFollow, base.AuthURL+"?"+q.Encode())
	loc, err = url.Parse(resp.location)
	require.NoError(t, err)
	code = loc.Query().Get("code")
	assert.Equal(t, http.StatusOK, exchange(code, "verifier-1"))
	assert.Equal(t, http.StatusBadRequest, exchange(code, "verifier-1"))
}
