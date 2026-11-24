package localai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// deleteTimeout はモデルの削除 1 回の上限。
const deleteTimeout = 2 * time.Minute

// DeleteModel はカタログのモデル name を消す（取得済みでなければ何もしない）。
// 取得済みのときだけ Acquire で起動して DELETE /api/delete する。ジョブ中は ErrBusy。
func (m *Manager) DeleteModel(ctx context.Context, name string) error {
	if _, err := m.catalogModel(name); err != nil {
		return err
	}
	if err := m.canStartJob(); err != nil {
		return err
	}
	if !m.installed() {
		return ErrNotInstalled
	}
	if !m.modelInstalled(name) {
		return nil
	}
	base, release, err := m.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	ctx, cancel := context.WithTimeout(ctx, deleteTimeout)
	defer cancel()
	body, err := json.Marshal(map[string]string{"model": name})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, ollamaRoot(base)+"/api/delete", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localClient.Do(req)
	if err != nil {
		return failure("モデルを削除できませんでした。", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return failure(fmt.Sprintf("モデルを削除できませんでした（Ollama が HTTP %d を返しました）。", resp.StatusCode), nil)
	}
	m.emit(true)
	return nil
}

// Remove は ollama を止めて、置き場所（Dir）を丸ごと消す。ジョブ中は ErrBusy。
func (m *Manager) Remove(ctx context.Context) error {
	m.mu.Lock()
	if err := m.precheck(); err != nil {
		m.mu.Unlock()
		return err
	}
	if m.job != nil {
		m.mu.Unlock()
		return ErrBusy
	}
	m.removing = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.removing = false
		m.lastError = ""
		m.mu.Unlock()
		m.emit(true)
	}()

	if err := ctx.Err(); err != nil {
		return err
	}
	m.stopProcess()
	if err := os.RemoveAll(m.cfg.dir); err != nil {
		return failure("ローカルAIを削除できませんでした。", err)
	}
	return nil
}
