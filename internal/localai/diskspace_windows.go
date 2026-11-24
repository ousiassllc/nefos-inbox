//go:build windows

package localai

import (
	"math"

	"golang.org/x/sys/windows"
)

// diskFreeBytes は path のあるボリュームの、この利用者が使える空き（クォータを考慮する）。
func diskFreeBytes(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var avail uint64
	if err := windows.GetDiskFreeSpaceEx(p, &avail, nil, nil); err != nil {
		return 0, err
	}
	return int64(min(avail, uint64(math.MaxInt64))), nil //nolint:gosec // 上で MaxInt64 に丸めている
}
