package localai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// maxPullLine は取得の進捗 1 行（NDJSON）の上限。
const maxPullLine = 1 << 20

// StartPull はカタログのモデル name の取得を始めて、すぐ戻る（進捗は OnChange・Status の Job）。
// カタログ外は ErrUnknownModel、Ollama が未導入なら ErrNotInstalled、ほかのジョブ中は ErrBusy、
// 空き容量（models のボリュームに、取得済みの途中の層を除いた SizeBytes + 1 GiB）が足りなければ *NoSpaceError。
// 取得の間は Acquire を持ち続ける（途中で止めない）。中止すると Ollama が途中の層を残すので、取り直すと続きから進む。
func (m *Manager) StartPull(name string) error {
	cm, err := m.catalogModel(name)
	if err != nil {
		return err
	}
	if err := m.canStartJob(); err != nil {
		return err
	}
	if !m.installed() {
		return ErrNotInstalled
	}
	// 前回の途中までの層（models/blobs/sha256-*-partial）は、取り直すと続きから進むので、そのぶんは新しく要らない。
	// 別のモデルの途中のものも数えるので、足りるかどうかの見積もりは少し甘くなり得る（足りなければ Ollama がジョブの失敗で知らせる）。
	held := partialBytes(filepath.Join(m.cfg.modelsDir(), "blobs"), func(name string) bool {
		return strings.HasPrefix(name, "sha256-") && strings.HasSuffix(name, "-partial")
	})
	if err := m.cfg.checkSpace(m.cfg.modelsDir(), cm.SizeBytes+spaceMargin, min(held, cm.SizeBytes)); err != nil {
		return err
	}
	ctx, err := m.beginJob(JobPull, name, StageStart, cm.SizeBytes)
	if err != nil {
		return err
	}
	m.goJob(ctx, func(ctx context.Context) error { return m.runPull(ctx, cm) })
	return nil
}

// pullLine は /api/pull の NDJSON の 1 行。
type pullLine struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

func (m *Manager) runPull(ctx context.Context, cm CatalogModel) error {
	base, release, err := m.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	m.setStage(StagePull, 0, cm.SizeBytes)

	body, err := json.Marshal(map[string]any{"model": cm.Name, "stream": true})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ollamaRoot(base)+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := localClient.Do(req)
	if err != nil {
		return failure("モデルの取得を始められませんでした。", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return failure(fmt.Sprintf("モデルの取得に失敗しました（Ollama が HTTP %d を返しました）。", resp.StatusCode), nil)
	}

	completed := map[string]int64{} // digest ごとの取得済み
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), maxPullLine)
	for sc.Scan() {
		var l pullLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		switch {
		case l.Error != "":
			return failure("モデルの取得に失敗しました。", errors.New(l.Error))
		case l.Status == "success":
			m.setProgress(cm.SizeBytes)
			return nil
		case l.Digest != "":
			completed[l.Digest] = l.Completed
			var sum int64
			for _, c := range completed {
				sum += c
			}
			m.setProgress(min(sum, cm.SizeBytes))
		}
	}
	return failure("モデルの取得が途中で切れました。", sc.Err())
}

// ollamaRoot は Acquire の Base URL（.../v1）から ollama の API の根（.../api の親）を求める。
func ollamaRoot(base string) string { return strings.TrimSuffix(base, "/v1") }
