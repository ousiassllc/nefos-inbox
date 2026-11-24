package localai

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// maxRedirects はリダイレクトの追従回数の上限。
const maxRedirects = 5

// checkURL は通信してよい URL か。https 以外・userinfo つき・許可していないホストは拒否する。メッセージに URL は含めない。
func (r resolved) checkURL(u *url.URL) error {
	if u.Scheme != "https" || u.User != nil || !r.allow(u) {
		return failure("許可していない接続先です。", nil)
	}
	return nil
}

func (r resolved) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return failure("リダイレクトが多すぎます。", nil)
	}
	return r.checkURL(req.URL)
}

// download は固定 URL の zip を downloads/*.part へ保存し、SHA-256（小文字の 16 進）を返す。サイズの照合もここで行う。
func (m *Manager) download(ctx context.Context) (string, error) {
	cfg := m.cfg
	u, err := url.Parse(cfg.pin.URL)
	if err != nil {
		return "", failure("ダウンロード先を解釈できませんでした。", err)
	}
	if err := cfg.checkURL(u); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", failure("ダウンロードの要求を作れませんでした。", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := cfg.hc.Do(req)
	if err != nil {
		var fe *failureError // CheckRedirect が拒否した（理由は日本語の 1 文。URL を含まない）
		if errors.As(err, &fe) {
			return "", fe
		}
		return "", failure("ダウンロードに失敗しました。", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", failure(fmt.Sprintf("ダウンロード先が HTTP %d を返しました。", resp.StatusCode), nil)
	}
	size := cfg.pin.Size
	if resp.ContentLength != size {
		return "", failure("ダウンロードするファイルのサイズが想定と違います。", nil)
	}

	f, err := os.OpenFile(cfg.zipPath()+".part", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", failure("ダウンロードの保存先を開けませんでした。", err)
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h, &progressWriter{report: m.setProgress}), io.LimitReader(resp.Body, size+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		return "", failure("ダウンロードに失敗しました。", err)
	case n > size:
		return "", failure("ダウンロードしたファイルが想定より大きいため止めました。", nil)
	case n < size:
		return "", failure("ダウンロードが途中で終わりました。", errors.New("short body"))
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// progressWriter は書いたバイト数の累計を report に渡す。
type progressWriter struct {
	n      int64
	report func(int64)
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	w.report(w.n)
	return len(p), nil
}
