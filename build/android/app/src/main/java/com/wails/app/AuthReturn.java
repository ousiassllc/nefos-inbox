package com.wails.app;

import android.content.Intent;
import android.net.Uri;

/**
 * ブラウザの認可の完了ページ（Go が 127.0.0.1 で返す）の「Nefos Inbox に戻る」リンクから来た Intent の判定（#368）。
 *
 * <p>リンクの URL は {@code jp.ousias.hitomado://auth-return} だけで、認可コード・state・トークンは載せない（AndroidManifest.xml の
 * BROWSABLE の intent-filter と、Go の {@code callbackpage.AndroidReturnURL} が同じ値）。この Intent はアプリを前面に出すだけで、
 * データは読まず、{@link OpenRequest} にも入れない。singleTask の MainActivity は起動済みなら onNewIntent で受け、Go を起動し直さない。
 */
final class AuthReturn {
    static final String SCHEME = "jp.ousias.hitomado";
    static final String HOST = "auth-return";

    private AuthReturn() {}

    /** Intent がこのリンクからの起動なら true（ACTION_VIEW で、データの scheme・host が一致する）。 */
    static boolean matches(Intent intent) {
        if (intent == null) {
            return false;
        }
        Uri data = intent.getData();
        return data != null && matches(intent.getAction(), data.getScheme(), data.getHost());
    }

    /** Android に依存しない判定（JVM の単体テストで使う）。 */
    static boolean matches(String action, String scheme, String host) {
        return Intent.ACTION_VIEW.equals(action) && SCHEME.equals(scheme) && HOST.equals(host);
    }
}
