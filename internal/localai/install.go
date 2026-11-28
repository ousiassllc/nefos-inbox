package localai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// StartInstall は固定版の Ollama の導入を始めて、すぐ戻る（進捗は OnChange・Status の Job）。
// すでに導入済みなら何もしない。ほかのジョブが走っていれば ErrBusy、空き容量が足りなければ *NoSpaceError。
// 失敗は Status.LastError に残る。中止（CancelJob・Close）では LastError を残さない（利用者が止めたため）。
func (m *Manager) StartInstall() error {
	if err := m.canStartJob(); err != nil {
		return err
	}
	if m.installed() {
		return nil
	}
	p := m.cfg.pin
	// 前回の途中までの .part は、ダウンロードの始めに作り直す（上書きする）ので、そのぶんは新しく要らない。
	part := m.cfg.zipPath() + ".part"
	held := partialBytes(filepath.Dir(part), func(name string) bool { return name == filepath.Base(part) })
	if err := m.cfg.checkSpace(m.cfg.downloadsDir(), p.Size+p.ExtractedSize+spaceMargin, min(held, p.Size)); err != nil {
		return err
	}
	ctx, err := m.beginJob(JobInstall, "", StageDownload, p.Size)
	if err != nil {
		return err
	}
	m.goJob(ctx, m.runInstall)
	return nil
}

// canStartJob はジョブを始められる状態か（使える環境・ジョブ中でない）。空き容量の確認の前に、早く断るために使う。
func (m *Manager) canStartJob() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.precheck(); err != nil {
		return err
	}
	if m.job != nil {
		return ErrBusy
	}
	return nil
}

// runInstall は zip のダウンロード → SHA-256 の照合 → 展開 → runtime/ への入れ替え。失敗・中止したら途中のファイルを消す。
func (m *Manager) runInstall(ctx context.Context) (err error) {
	cfg := m.cfg
	var tmp string
	defer func() {
		if err != nil {
			_ = os.Remove(cfg.zipPath() + ".part")
			_ = os.Remove(cfg.zipPath())
			if tmp != "" {
				_ = os.RemoveAll(tmp)
			}
		}
	}()
	if err = os.MkdirAll(cfg.downloadsDir(), 0o700); err != nil {
		return failure("導入の保存先を作れませんでした。", err)
	}
	m.removeStaleTemp()

	sum, err := m.download(ctx)
	if err != nil {
		return err
	}
	m.setStage(StageVerify, cfg.pin.Size, cfg.pin.Size)
	if sum != strings.ToLower(cfg.pin.SHA256) {
		return failure("ダウンロードしたファイルが正しくありません（SHA-256 が一致しません）。", nil)
	}
	if err = os.Rename(cfg.zipPath()+".part", cfg.zipPath()); err != nil {
		return failure("ダウンロードしたファイルを保存できませんでした。", err)
	}

	m.setStage(StageExtract, 0, cfg.pin.ExtractedSize)
	if tmp, err = os.MkdirTemp(cfg.dir, "runtime.tmp-"); err != nil {
		return failure("展開先を作れませんでした。", err)
	}
	if err = extractZip(ctx, cfg.zipPath(), tmp, cfg.pin.ExtractedSize+extractMargin, m.setProgress); err != nil {
		return err
	}
	if fi, serr := os.Stat(filepath.Join(tmp, binaryName())); serr != nil || !fi.Mode().IsRegular() {
		return failure("ダウンロードしたファイルに Ollama の実行ファイルがありませんでした。", serr)
	}
	if err = os.WriteFile(filepath.Join(tmp, "VERSION"), []byte(cfg.pin.Version+"\n"), 0o600); err != nil {
		return failure("導入の記録を書けませんでした。", err)
	}

	m.stopProcess() // 古い版が動いていれば止める
	if err = os.RemoveAll(cfg.runtimeDir()); err != nil {
		return failure("古い Ollama を消せませんでした。", err)
	}
	if err = os.Rename(tmp, cfg.runtimeDir()); err != nil {
		return failure("Ollama を配置できませんでした。", err)
	}
	tmp = ""
	_ = os.Remove(cfg.zipPath())
	return nil
}

// removeStaleTemp は以前の導入が途中で終わって残した runtime.tmp-* を消す。
func (m *Manager) removeStaleTemp() {
	old, _ := filepath.Glob(filepath.Join(m.cfg.dir, "runtime.tmp-*"))
	for _, p := range old {
		_ = os.RemoveAll(p)
	}
}
