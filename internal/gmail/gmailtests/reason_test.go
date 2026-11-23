package gmailtests

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/gmail"
	"github.com/ousiassllc/nefos-inbox/internal/secrets"
)

// 資格情報ストアの Get を key だけ err で失敗させる Store（key が空なら全部）。
type failingStore struct {
	secrets.Store
	key string
	err error
}

func (s failingStore) Get(key string) (string, error) {
	if s.key == "" || key == s.key {
		return "", s.err
	}
	return s.Store.Get(key)
}

// 再接続が必要になった理由（ReauthReason）が、失敗の経路ごとに区別されて返る（#369）。ログから原因を切り分けるためのもの。
func TestReauthReasonPerFailurePath(t *testing.T) {
	token := func(status int, code string) func(*testEnv) {
		return func(e *testEnv) {
			e.f.Token = func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(w, status, map[string]any{"error": code, "error_description": leakedToken})
			}
		}
	}
	api := func(status int, reason string) func(*testEnv) {
		return func(e *testEnv) {
			e.f.HandleJSON("GET "+apiBase+"/profile", status, apiError(status, reason, leakedToken))
		}
	}
	for _, tc := range []struct {
		name     string
		setup    func(*testEnv)
		store    secrets.Store // 非 nil なら資格情報ストアを差し替える
		sentinel error
		want     string
	}{
		{"invalid_grant", token(400, "invalid_grant"), nil, gmail.ErrNeedsReauth, gmail.ReasonInvalidGrant},
		{"invalid_client", token(401, "invalid_client"), nil, gmail.ErrClientRejected, gmail.ReasonClientRejected},
		{"api 401 after retry", api(401, "authError"), nil, gmail.ErrNeedsReauth, gmail.ReasonAPIUnauthorized},
		{"api 403 scope", api(403, "insufficientPermissions"), nil, gmail.ErrScopeInsufficient, gmail.ReasonScopeInsufficient},
		{"refresh token missing", nil, failingStore{err: secrets.ErrNotFound}, gmail.ErrNeedsReauth, gmail.ReasonSecretNotFound},
		{"secret store key lost", nil, failingStore{err: secrets.ErrKeyLost}, gmail.ErrNeedsReauth, gmail.ReasonSecretKeyLost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEnvWith(t, func(o *gmail.Options) {
				if tc.store != nil {
					o.Secrets = tc.store
				}
			})
			if tc.setup != nil {
				tc.setup(e)
			}
			_, err := e.c.Profile(context.Background(), testAccount)
			require.ErrorIs(t, err, tc.sentinel)
			assert.Equal(t, tc.want, gmail.ReauthReason(err))
			assert.NotContains(t, err.Error(), leakedToken)
		})
	}
}

// 再接続が必要でない失敗には理由が付かない。
func TestReauthReasonEmptyForOtherFailures(t *testing.T) {
	assert.Empty(t, gmail.ReauthReason(nil))
	assert.Empty(t, gmail.ReauthReason(gmail.ErrOffline))
	assert.Empty(t, gmail.ReauthReason(gmail.ErrNeedsReauth), "sentinel だけ（理由なし）")
	e := newTestEnv(t)
	e.f.HandleJSON("GET "+apiBase+"/profile", 500, apiError(500, "backendError", "x"))
	_, err := e.c.Profile(context.Background(), testAccount)
	require.ErrorIs(t, err, gmail.ErrGeneric)
	assert.Empty(t, gmail.ReauthReason(err))
}

// 資格情報ストアが読み出しに失敗した（鍵の喪失でも「無い」でもない）ときは再接続待ちにしない。一時的な失敗を
// 「トークンが無い」と取り違えて再ログインを強いない。原因（ストアのエラー）は呼び出し側が追えるよう残す。
func TestSecretStoreReadFailureIsNotNeedsReauth(t *testing.T) {
	boom := errors.New("secure storage: failed:android.security.KeyStoreException")
	for name, key := range map[string]string{
		"refresh token": secrets.RefreshTokenKey(testAccount), "client secret": secrets.OAuthClientSecretKey(testRowID),
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnvWith(t, func(o *gmail.Options) { o.Secrets = failingStore{Store: o.Secrets, key: key, err: boom} })
			_, err := e.c.Profile(context.Background(), testAccount)
			require.ErrorIs(t, err, gmail.ErrGeneric)
			assert.NotErrorIs(t, err, gmail.ErrNeedsReauth)
			assert.NotErrorIs(t, err, gmail.ErrClientRejected)
			assert.Empty(t, gmail.ReauthReason(err))
			assert.ErrorIs(t, err, boom, "ストアの失敗の原因を残す")
		})
	}
}

// トークン更新エンドポイントの一時的・想定外の失敗（5xx・429・invalid_grant 以外の 400・HTML や空の応答・access_token の無い 200）は、
// 再接続待ちにしない（#369）。再接続待ちは再試行では戻らない（利用者が再接続するまで同期が止まる）ので、一時的な失敗で入ってはならない。
func TestTokenEndpointTransientFailuresAreNotNeedsReauth(t *testing.T) {
	raw := func(status int, contentType, body string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}
	}
	for name, handler := range map[string]func(http.ResponseWriter, *http.Request){
		"500 json":                      raw(500, "application/json", `{"error":"internal_failure"}`),
		"503 html":                      raw(503, "text/html", "<html>unavailable</html>"),
		"429":                           raw(429, "application/json", `{"error":"rate_limit_exceeded"}`),
		"400 invalid_request":           raw(400, "application/json", `{"error":"invalid_request"}`),
		"403 html (proxy)":              raw(403, "text/html", "<html>forbidden</html>"),
		"400 empty body":                raw(400, "application/json", ""),
		"200 captive portal html":       raw(200, "text/html", "<html>login</html>"),
		"200 json without access_token": raw(200, "application/json", `{"token_type":"Bearer","expires_in":3600}`),
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.f.Token = handler
			_, err := e.c.Profile(context.Background(), testAccount)
			require.Error(t, err)
			assert.NotErrorIs(t, err, gmail.ErrNeedsReauth)
			assert.NotErrorIs(t, err, gmail.ErrClientRejected)
			assert.Empty(t, gmail.ReauthReason(err))
		})
	}
}

// 401 でアクセストークンを取り直して再試行したことを、再接続待ちの前触れとして Info で残す（アカウントは ID だけ）。
func TestUnauthorizedRetryIsLogged(t *testing.T) {
	var buf bytes.Buffer
	e := newTestEnvWith(t, func(o *gmail.Options) {
		o.Logger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	})
	var calls atomic.Int32
	e.f.Handle("GET "+apiBase+"/profile", func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			writeJSON(w, 401, apiError(401, "authError", leakedToken))
			return
		}
		writeJSON(w, 200, map[string]any{"emailAddress": "a@example.com", "historyId": "7"})
	})
	_, err := e.c.Profile(context.Background(), testAccount)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "401")
	assert.Contains(t, buf.String(), "account_id="+testAccount)
	assert.NotContains(t, buf.String(), leakedToken)
	assert.NotContains(t, buf.String(), "a@example.com")
}
