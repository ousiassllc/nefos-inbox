// ローカルAI（Ollama。#458）の画面の数値の表示。サイズは 10 進（1 GB = 10^9 B。Go の「空き容量が足りません（必要: 約 X GB…）」と同じ）。

/** バイト数を「1.6」のような GB の数字にする（小数 1 桁。単位は付けない）。 */
export function gbNumber(bytes: number): string {
  return (Math.max(bytes, 0) / 1e9).toFixed(1)
}

/** 進み（0〜100 の整数）。全体が不明（0 以下）のときは undefined（不定の進捗）。 */
export function percentOf(completed: number, total: number): number | undefined {
  if (!(total > 0)) return undefined
  return Math.min(100, Math.max(0, Math.floor((completed / total) * 100)))
}
