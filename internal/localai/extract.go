package localai

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// extractMargin は展開後の合計サイズの上限（固定の値）に足す余裕。
	extractMargin = int64(16) << 20
	// maxZipEntries は zip のエントリ数の上限（実物は 80 ファイル）。
	maxZipEntries = 2000
)

// extractZip は zipPath を dest の中へ展開する。名前が dest の外へ出るもの（絶対パス・..・ドライブ名・\ や :）、
// シンボリックリンクなどの通常でないエントリ、エントリ数・合計サイズの超過は拒否する。report には展開したバイト数の累計を渡す。
func extractZip(ctx context.Context, zipPath, dest string, maxTotal int64, report func(int64)) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return failure("ダウンロードしたファイルを開けませんでした。", err)
	}
	defer func() { _ = zr.Close() }()
	if len(zr.File) > maxZipEntries {
		return failure("ダウンロードしたファイルのエントリが多すぎます。", nil)
	}
	var declared uint64
	for _, f := range zr.File {
		declared += f.UncompressedSize64
		if declared > uint64(maxTotal) { //nolint:gosec // maxTotal は正の値
			return failure("ダウンロードしたファイルの展開後のサイズが想定を超えています。", nil)
		}
	}
	var written int64
	for _, f := range zr.File {
		target, err := entryPath(dest, f)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return failure("展開先のフォルダを作れませんでした。", err)
			}
			continue
		}
		n, err := extractFile(ctx, f, target, maxTotal-written, &progressWriter{n: written, report: report})
		written += n
		if err != nil {
			return err
		}
	}
	return nil
}

// entryPath は f の展開先（dest の中）。安全でない名前・種類は拒否する。
func entryPath(dest string, f *zip.File) (string, error) {
	name := f.Name
	bad := failure("ダウンロードしたファイルに安全でない項目が含まれていたため止めました。", nil)
	if name == "" || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "/") {
		return "", bad
	}
	rel := filepath.FromSlash(name)
	if !filepath.IsLocal(rel) {
		return "", bad
	}
	if m := f.Mode(); !m.IsRegular() && !m.IsDir() {
		return "", bad
	}
	return filepath.Join(dest, rel), nil
}

// extractFile は f を target へ書く（pw に進捗を渡す）。budget を超えて書きそうなら失敗する。書いたバイト数を返す。
func extractFile(ctx context.Context, f *zip.File, target string, budget int64, pw *progressWriter) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return 0, failure("展開先のフォルダを作れませんでした。", err)
	}
	rc, err := f.Open()
	if err != nil {
		return 0, failure("ダウンロードしたファイルの中身を読めませんでした。", err)
	}
	defer func() { _ = rc.Close() }()
	perm := (f.Mode().Perm() | 0o600) & 0o755
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) //nolint:gosec // 展開先は entryPath で dest の中に限っている
	if err != nil {
		return 0, failure("展開先のファイルを作れませんでした。", err)
	}
	n, err := io.Copy(io.MultiWriter(out, pw), io.LimitReader(&ctxReader{ctx: ctx, r: rc}, budget+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, failure("ファイルの展開に失敗しました。", err)
	}
	if n > budget {
		return n, failure("ダウンロードしたファイルの展開後のサイズが想定を超えています。", nil)
	}
	return n, nil
}

// ctxReader は読むたびに ctx を確かめる（展開の中止を、大きなファイルの途中でも効かせる）。
type ctxReader struct {
	ctx context.Context //nolint:containedctx // io.Reader に ctx を持たせるための薄いラッパ
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
