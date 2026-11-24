//go:build !windows

package localai

// checkSupport は Windows 以外では使えないと返す（Linux の開発・テストは Options.Support で差し替える）。
func checkSupport() (bool, string) {
	return false, "Windows 以外では使えません。"
}
