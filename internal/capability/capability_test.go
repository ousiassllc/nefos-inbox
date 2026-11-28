package capability_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ousiassllc/nefos-inbox/internal/capability"
)

func TestDesktopEnablesEverythingButTouchUI(t *testing.T) {
	assert.Equal(t, capability.Capabilities{
		OS: "windows", SupportsMCP: true, SupportsTray: true, SupportsAutoUpdate: true, SupportsWindowState: true, SingleInstance: true,
		SupportsLocalAI: true,
	}, capability.Desktop("windows"))
	assert.False(t, capability.Desktop("windows").TouchUI, "デスクトップは 3 ペイン固定（タッチ UI なし）")
}

// ローカル AI（Ollama）は Windows だけ。ほかのデスクトップ OS・Android では無効。
func TestLocalAIIsWindowsOnly(t *testing.T) {
	assert.True(t, capability.Desktop("windows").SupportsLocalAI)
	assert.False(t, capability.Desktop("darwin").SupportsLocalAI)
	assert.False(t, capability.Desktop("linux").SupportsLocalAI)
	assert.False(t, capability.Android().SupportsLocalAI)
}

func TestAndroidKeepsUpdateCheckAndTouchUIOnly(t *testing.T) {
	assert.Equal(t, capability.Capabilities{OS: "android", SupportsAutoUpdate: true, TouchUI: true}, capability.Android())
}

func TestZeroValueDisablesEverything(t *testing.T) {
	c := capability.Capabilities{}
	assert.False(t, c.SupportsMCP || c.SupportsTray || c.SupportsAutoUpdate || c.SupportsWindowState || c.SingleInstance || c.TouchUI || c.SupportsLocalAI)
}

// このテストを実行するホスト（Android ではない）の Current はデスクトップの機能になる。
func TestCurrentOnHostIsDesktop(t *testing.T) {
	assert.Equal(t, capability.Desktop(capability.Current().OS), capability.Current())
}

func TestPlatformServiceReturnsGivenCapabilities(t *testing.T) {
	assert.Equal(t, capability.Android(), capability.NewPlatformService(capability.Android()).GetCapabilities())
	assert.Equal(t, capability.Desktop("windows"), capability.NewPlatformService(capability.Desktop("windows")).GetCapabilities())
}

// フロントの型（frontend/bindings）と store が読む JSON のキー。
func TestCapabilitiesJSONKeys(t *testing.T) {
	b, err := json.Marshal(capability.Android())
	require.NoError(t, err)
	assert.JSONEq(t, `{"os":"android","supportsMcp":false,"supportsTray":false,"supportsAutoUpdate":true,"supportsWindowState":false,"singleInstance":false,"touchUi":true,"supportsLocalAi":false}`, string(b))
}
