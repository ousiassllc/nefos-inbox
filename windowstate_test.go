package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/platform"
	"github.com/ousiassllc/nefos-inbox/internal/store"
)

// failingStore は設定の読み書きを失敗させる Store（埋め込みの nil Store は他のメソッドを呼ぶと panic する）。
type failingStore struct {
	store.Store
	err error
}

func (f failingStore) GetSetting(context.Context, string) (string, bool, error) {
	return "", false, f.err
}
func (f failingStore) SetSetting(context.Context, string, string) error { return f.err }

func openTestStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(t.Context(), filepath.Join(t.TempDir(), "h.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func logRecorder() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestLoadWindowState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		raw  *string // nil なら値なし
		want platform.WindowState
	}{
		{name: "値なしは既定値", raw: nil, want: platform.DefaultWindowState()},
		// app_settings の value は json_valid の CHECK があるため、構文は正しいが構造体として読めない値で「壊れた値」を作る。
		{name: "構造体として読めない値は既定値", raw: ptr(`"oops"`), want: platform.DefaultWindowState()},
		{name: "寸法のない値は既定値", raw: ptr(`{}`), want: platform.DefaultWindowState()},
		{name: "幅が 0 は既定値", raw: ptr(`{"width":0,"height":600}`), want: platform.DefaultWindowState()},
		{name: "上限超えの高さは既定値", raw: ptr(`{"width":800,"height":999999}`), want: platform.DefaultWindowState()},
		{
			name: "最小サイズ未満は最小サイズに直す",
			raw:  ptr(`{"width":10,"height":10}`),
			want: platform.WindowState{Width: platform.MinWidth, Height: platform.MinHeight},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			st := openTestStore(t)
			if tt.raw != nil {
				require.NoError(t, st.SetSetting(t.Context(), store.SettingWindowState, *tt.raw))
			}
			log, buf := logRecorder()
			assert.Equal(t, tt.want, loadWindowState(t.Context(), st, log))
			assert.Empty(t, buf.String(), "既定値に戻すだけで警告は出さない")
		})
	}
}

func TestLoadWindowStateReadFailureFallsBackToDefault(t *testing.T) {
	t.Parallel()
	log, buf := logRecorder()
	got := loadWindowState(t.Context(), failingStore{err: errors.New("db down")}, log)
	assert.Equal(t, platform.DefaultWindowState(), got, "読み取りに失敗しても起動を止めず既定値で開く")
	assert.Contains(t, buf.String(), "load window state failed")
}

func TestWindowStateSaverRoundTrip(t *testing.T) {
	t.Parallel()
	st := openTestStore(t)
	log, buf := logRecorder()
	x, y := -1200, 40
	in := platform.WindowState{X: &x, Y: &y, Width: 1100, Height: 700, Maximized: true}

	windowStateSaver(st, log)(in)

	assert.Equal(t, in, loadWindowState(t.Context(), st, log))
	assert.Empty(t, buf.String())
}

func TestWindowStateSaverFailureOnlyLogs(t *testing.T) {
	t.Parallel()
	log, buf := logRecorder()
	save := windowStateSaver(failingStore{err: errors.New("disk full")}, log)

	assert.NotPanics(t, func() { save(platform.DefaultWindowState()) })
	assert.Contains(t, buf.String(), "save window state failed")
}

func ptr[T any](v T) *T { return &v }
