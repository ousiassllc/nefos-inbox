package localai

import (
	"errors"
	"net/url"
)

// failureError は画面に出す日本語の理由（msg）と、ログにだけ残す原因（cause）を持つ。
// Error() は msg だけを返すので、パス・URL・Ollama の出力が画面に出ない。
type failureError struct {
	msg   string
	cause error
}

func (e *failureError) Error() string { return e.msg }
func (e *failureError) Unwrap() error { return e.cause }

// failure は msg を画面に出す理由とする error を作る。cause の URL からはクエリ・フラグメントを除く（署名つきトークンを残さない）。
func failure(msg string, cause error) error {
	return &failureError{msg: msg, cause: scrubURL(cause)}
}

// scrubURL は *url.Error の URL からクエリ（リダイレクト先の一時的な署名つきトークン）を除く。
func scrubURL(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	if u, perr := url.Parse(ue.URL); perr == nil {
		u.RawQuery, u.Fragment = "", ""
		return &url.Error{Op: ue.Op, URL: u.String(), Err: ue.Err}
	}
	return &url.Error{Op: ue.Op, URL: "(invalid)", Err: ue.Err}
}

// UserMessage は err が画面に出してよい日本語の理由を持つなら、それを返す（localai が failure で作った失敗。パス・URL を含まない）。
func UserMessage(err error) (string, bool) {
	var fe *failureError
	if errors.As(err, &fe) {
		return fe.msg, true
	}
	return "", false
}
