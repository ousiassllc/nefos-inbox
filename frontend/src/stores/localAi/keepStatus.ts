import { type Ref, watch } from 'vue'
import { useLocalAiStore } from './index'

/**
 * 設定ダイアログが開いている間、ローカルAI（Windows のみ）の `localai:status` の購読を保つ。導入・モデルの取得は、ほかのタブ・ページにいる間も進み、
 * 終わったときに AI の設定（local.ready）を取り直せるようにするため。閉じたとき・コンポーネントが外れたとき（watch の後始末）に解除する。
 */
export function keepLocalAiStatus(open: Ref<boolean>, supported: () => boolean): void {
  const local = useLocalAiStore()
  watch(
    [open, supported],
    ([isOpen, ok], _prev, onCleanup) => {
      if (isOpen && ok) onCleanup(local.start())
    },
    { immediate: true },
  )
}
