//go:build windows

package localai

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	// minBuild は Windows 10 22H2 のビルド番号。これより古い Windows は使えない。
	minBuild = 19045
	// minPhysicalRAM は使える物理メモリの下限。「8 GB」の PC でも、統合 GPU などの予約で GlobalMemoryStatusEx は
	// 7.x GiB を返すので、8 GiB ちょうどではなく 7 GiB で判定する（8 GB の PC を使えないとしないため）。
	minPhysicalRAM = uint64(7) << 30

	imageFileMachineARM64 = 0xAA64
)

// memoryStatusEx は MEMORYSTATUSEX。
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

var procGlobalMemoryStatusEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")

// checkSupport は ARM64 の Windows（x64 エミュレーションで動いていても）・Windows 10 22H2 より古い・物理メモリ不足を断る。
func checkSupport() (bool, string) {
	if nativeARM64() {
		return false, "ARM 版の Windows（Snapdragon などを使った PC）では、アプリが用意する Ollama を動かせません。"
	}
	if windows.RtlGetVersion().BuildNumber < minBuild {
		return false, "Windows 10 22H2（ビルド 19045）より古い Windows ではローカルAIを使えません。"
	}
	if total, ok := totalPhysicalMemory(); ok && total < minPhysicalRAM {
		return false, "メモリ（RAM）が 8 GB 未満の PC ではローカルAIを使えません。"
	}
	return true, ""
}

// nativeARM64 は本物のハードウェアが ARM64 か（エミュレーションで x64 として動いていても true）。
func nativeARM64() bool {
	var process, native uint16
	if err := windows.IsWow64Process2(windows.CurrentProcess(), &process, &native); err != nil {
		return runtime.GOARCH == "arm64"
	}
	return native == imageFileMachineARM64
}

// totalPhysicalMemory は物理メモリの合計。取得できなければ ok=false（判定しない）。
func totalPhysicalMemory() (uint64, bool) {
	ms := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	r, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))) //nolint:gosec // Win32 API へ構造体を渡す
	if r == 0 {
		return 0, false
	}
	return ms.TotalPhys, true
}
