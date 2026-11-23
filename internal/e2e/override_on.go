//go:build e2e && !production

package e2e

import (
	"context"
	"log/slog"

	"github.com/ousiassllc/nefos-inbox/internal/gmail"
	"github.com/ousiassllc/nefos-inbox/internal/oauth"
	"github.com/ousiassllc/nefos-inbox/internal/secrets"
	"github.com/ousiassllc/nefos-inbox/internal/store"
)

// Marker は E2E モードのバイナリにだけ入る文字列。リリース APK の検査（build/android/scripts/verify-apk.sh）が、
// libwails.so にこの文字列が無いことを確かめる。
const Marker = "hitomado-e2e-backend-v1"

// Enabled は E2E 用ビルドのとき true。
const Enabled = true

// Override は渡された secrets・gmail を捨て、メモリ上の資格情報と、シード済みのフェイク Gmail に差し替える。
// 初回（アカウントが無いとき）は OAuth クライアントとアカウントを 1 件ずつ DB に作る。
// 同期は通常どおりこのフェイクに対して走り、スレッド一覧が DB に入る。
func Override(ctx context.Context, dir string, st store.Store, _ secrets.Store, _ gmail.Client, logger *slog.Logger) (secrets.Store, gmail.Client, error) {
	sec := secrets.NewMemory()
	fake, err := Seed(ctx, st, sec, timeNow())
	if err != nil {
		return nil, nil, err
	}
	logger.Warn("E2E mode: Gmail is a seeded fake and credentials are in memory; never use with real accounts", "marker", Marker)
	return sec, NewRecordingGmail(fake, dir), nil
}

// OAuthOptions は opts の認可・トークン・userinfo・失効の URL を、アプリ内の偽の Google（IdP。idp.go）に差し替え、止める関数を返す。
// ブラウザを開く経路（OpenBrowser）は差し替えない: 本物の Intent で端末のブラウザを開き、ブラウザ→ループバックのコールバック→アプリへ戻る、までを実際に通す。
func OAuthOptions(opts oauth.Options, logger *slog.Logger) (oauth.Options, func(), error) {
	idp, err := StartIdP()
	if err != nil {
		return opts, func() {}, err
	}
	logger.Warn("E2E mode: OAuth endpoints are a fake IdP on loopback; never use with real accounts", "marker", Marker)
	return idp.Options(opts), idp.Close, nil
}
