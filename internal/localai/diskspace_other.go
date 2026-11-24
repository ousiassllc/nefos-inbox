//go:build !windows

package localai

import (
	"math"

	"golang.org/x/sys/unix"
)

// diskFreeBytes は path のあるボリュームの空き（Linux の開発・テスト用）。
func diskFreeBytes(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	free := uint64(st.Bavail) * uint64(st.Bsize)        //nolint:gosec // 空きブロック数・ブロックサイズは負にならない
	return int64(min(free, uint64(math.MaxInt64))), nil //nolint:gosec // 上で MaxInt64 に丸めている
}
