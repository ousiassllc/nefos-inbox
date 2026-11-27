package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// fakeWin は keeperWindow の fake。呼び出しを記録する。
type fakeWin struct {
	bounds      application.Rect
	maximised   bool
	minimised   bool
	centeredTo  *application.Rect // Center() 後のウィンドウ位置
	boundsCalls int
	centered    int
	maximise    int
}

func (w *fakeWin) Bounds() application.Rect { w.boundsCalls++; return w.bounds }
func (w *fakeWin) IsMaximised() bool        { return w.maximised }
func (w *fakeWin) IsMinimised() bool        { return w.minimised }
func (w *fakeWin) Maximise() application.Window {
	w.maximise++
	w.maximised = true
	return nil
}

func (w *fakeWin) Center() {
	w.centered++
	if w.centeredTo != nil {
		w.bounds = *w.centeredTo
	}
}

func newTestKeeper(win *fakeWin, saved WindowState, areas []Rect) (*WindowKeeper, *[]WindowState) {
	var saves []WindowState
	k := &WindowKeeper{
		win:       win,
		screens:   func() []Rect { return areas },
		saved:     saved,
		save:      func(s WindowState) { saves = append(saves, s) },
		maximized: saved.Maximized,
	}
	if r, ok := saved.Bounds(); ok {
		k.normal = r
	}
	return k, &saves
}

var primary = []Rect{{X: 0, Y: 0, W: 1920, H: 1040}}

func TestWindowKeeperRestoreOffScreenCenters(t *testing.T) {
	t.Parallel()
	win := &fakeWin{
		bounds:     application.Rect{X: 5000, Y: 5000, Width: 1100, Height: 700},
		centeredTo: &application.Rect{X: 410, Y: 170, Width: 1100, Height: 700},
	}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 5000, Y: 5000, W: 1100, H: 700}, false), primary)

	k.restore()

	assert.Equal(t, 1, win.centered, "画面外なら中央に戻す")
	assert.Equal(t, Rect{X: 410, Y: 170, W: 1100, H: 700}, k.normal, "中央に戻した後の位置を取り直す")
	assert.Zero(t, win.maximise)
}

func TestWindowKeeperRestoreKeepsOnScreenPosition(t *testing.T) {
	t.Parallel()
	win := &fakeWin{bounds: application.Rect{X: 100, Y: 100, Width: 1100, Height: 700}}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, false), primary)

	k.restore()

	assert.Zero(t, win.centered)
	assert.Equal(t, Rect{X: 100, Y: 100, W: 1100, H: 700}, k.normal)
}

func TestWindowKeeperRestoreWithoutScreensOrPositionDoesNotCenter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		saved WindowState
		areas []Rect
	}{
		{name: "モニター一覧が取れない", saved: stateOf(Rect{X: 5000, Y: 5000, W: 1100, H: 700}, false), areas: nil},
		{name: "位置が未保存", saved: WindowState{Width: 1100, Height: 700}, areas: primary},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			win := &fakeWin{}
			k, _ := newTestKeeper(win, tt.saved, tt.areas)
			k.restore()
			assert.Zero(t, win.centered)
		})
	}
}

func TestWindowKeeperRestoreMaximises(t *testing.T) {
	t.Parallel()
	win := &fakeWin{bounds: application.Rect{X: 100, Y: 100, Width: 1100, Height: 700}}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, true), primary)

	k.restore()

	assert.Equal(t, 1, win.maximise, "最大化して終了したなら最大化で戻す")
}

func TestWindowKeeperCaptureMinimisedKeepsState(t *testing.T) {
	t.Parallel()
	win := &fakeWin{minimised: true, maximised: true, bounds: application.Rect{X: -32000, Y: -32000, Width: 160, Height: 28}}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, false), primary)

	k.capture()

	assert.False(t, k.maximized)
	assert.Equal(t, Rect{X: 100, Y: 100, W: 1100, H: 700}, k.normal)
	assert.Zero(t, win.boundsCalls)
}

func TestWindowKeeperCaptureMaximisedKeepsNormal(t *testing.T) {
	t.Parallel()
	win := &fakeWin{maximised: true, bounds: application.Rect{X: 0, Y: 0, Width: 1920, Height: 1040}}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, false), primary)

	k.capture()

	assert.True(t, k.maximized)
	assert.Equal(t, Rect{X: 100, Y: 100, W: 1100, H: 700}, k.normal, "最大化中も戻したときの大きさを保つ")
	assert.Zero(t, win.boundsCalls)
}

func TestWindowKeeperCaptureIgnoresEmptyBounds(t *testing.T) {
	t.Parallel()
	win := &fakeWin{}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, false), primary)

	k.capture()

	assert.Equal(t, Rect{X: 100, Y: 100, W: 1100, H: 700}, k.normal)
}

func TestWindowKeeperCaptureTracksMove(t *testing.T) {
	t.Parallel()
	win := &fakeWin{bounds: application.Rect{X: 30, Y: 40, Width: 1000, Height: 650}}
	k, _ := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, true), primary)

	k.capture()

	assert.False(t, k.maximized)
	assert.Equal(t, Rect{X: 30, Y: 40, W: 1000, H: 650}, k.normal)
}

func TestWindowKeeperFlush(t *testing.T) {
	t.Parallel()
	t.Run("現在の状態を保存する", func(t *testing.T) {
		t.Parallel()
		win := &fakeWin{bounds: application.Rect{X: 30, Y: 40, Width: 1000, Height: 650}}
		k, saves := newTestKeeper(win, DefaultWindowState(), primary)

		k.Flush()

		require.Len(t, *saves, 1)
		assert.Equal(t, stateOf(Rect{X: 30, Y: 40, W: 1000, H: 650}, false), (*saves)[0])
	})
	t.Run("最大化中は normal と最大化を保存する", func(t *testing.T) {
		t.Parallel()
		win := &fakeWin{maximised: true}
		k, saves := newTestKeeper(win, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, false), primary)

		k.Flush()

		require.Len(t, *saves, 1)
		assert.Equal(t, stateOf(Rect{X: 100, Y: 100, W: 1100, H: 700}, true), (*saves)[0])
	})
	t.Run("矩形がまだ取れていなければ保存しない", func(t *testing.T) {
		t.Parallel()
		k, saves := newTestKeeper(&fakeWin{}, DefaultWindowState(), primary)

		k.Flush()

		assert.Empty(t, *saves)
	})
	t.Run("nil は何もしない", func(t *testing.T) {
		t.Parallel()
		var k *WindowKeeper
		assert.NotPanics(t, k.Flush)
	})
}
