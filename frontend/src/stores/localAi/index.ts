import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  AIService,
  LocalAIService,
} from '../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app'
import type {
  AIModelList,
  LocalAIStatus,
} from '../../../bindings/github.com/ousiassllc/nefos-inbox/internal/app/dto'
import { normalizeAIConfig } from '../../lib/aiConfig'
import { call, list, parseBindingError } from '../../lib/api'
import { EVENTS, onEvent } from '../../lib/events'
import { settings as msg } from '../../lib/messages'
import { useSettingsStore } from '../settings'

/** Go は models・job を nil にし得るので、画面側は models を常に配列、job は無ければ undefined として扱う。 */
function normalize(s: LocalAIStatus): LocalAIStatus {
  return { ...s, models: list(s.models), job: s.job ?? undefined }
}

/**
 * ローカルAI（アプリが用意する Ollama。#458。Windows のみ）の状態。LocalAIService の呼び出しとイベント `localai:status` を受け、
 * 画面（設定 → AI → ローカルAI）が見る `status` を保つ。メモリのみ。ページを開いたとき `start()` で初期化して購読し、閉じたとき返された関数で解除する。
 * 操作の失敗（空き不足を含む）は `error` に Go の日本語の文面で入る（次の操作で消える）。
 */
export const useLocalAiStore = defineStore('localAi', () => {
  const settings = useSettingsStore()
  const status = ref<LocalAIStatus>()
  /** 直前の操作・状態の取得の失敗（日本語。`role="alert"` で出す）。 */
  const error = ref<string>()
  /** 直前の `error` が、モデルの取得を始められなかった空き容量不足か（画面が見出しと案内を変える）。 */
  const noSpacePull = ref(false)
  /** 操作（導入・取得・中止・停止・削除）を呼び出している間。 */
  const busy = ref(false)
  /** 状態を反映するたびに増える。取得・操作の結果より先に新しいイベントが届いたとき、古い結果で上書きしないための印。 */
  let revision = 0
  /** 購読している画面の数（設定ダイアログ・ローカルAIのページ）。0 → 1 で購読し、1 → 0 で解除する。 */
  let subscribers = 0
  let off: (() => void) | undefined

  const job = computed(() => status.value?.job ?? undefined)
  const models = computed(() => list(status.value?.models))

  /** 接続済み（`local.ready`）を左右する値。導入済みか・取得済みのモデルの数。 */
  const readyKey = (s?: LocalAIStatus) =>
    s ? `${s.installed}|${list(s.models).filter((m) => m.installed).length}` : ''

  function apply(s: LocalAIStatus): void {
    const before = readyKey(status.value)
    revision++
    status.value = normalize(s)
    // 導入・モデルの取得・削除が（どの画面にいても）終わったら、つなぎ先の一覧・機能の選択肢の「接続済み」を取り直す。最初の取得（before が空）では取り直さない。
    if (before !== '' && before !== readyKey(status.value)) void refreshConfig()
  }

  async function refresh(): Promise<void> {
    const rev = revision
    try {
      const s = await call(LocalAIService.GetLocalAIStatus())
      if (rev === revision) apply(s)
      error.value = undefined
    } catch (e) {
      error.value = parseBindingError(e).message || msg.ai.local.statusFailed
    }
  }

  /**
   * 状態を読み込み、`localai:status` を購読する。返す関数で購読を解除する（画面を閉じるとき。何度呼んでもよい）。
   * 設定ダイアログが開いている間とローカルAIのページが開いている間、それぞれ呼ぶ。購読は最後の 1 つが解除されるまで 1 つだけ。
   */
  function start(): () => void {
    error.value = undefined
    noSpacePull.value = false
    if (subscribers++ === 0) off = onEvent<LocalAIStatus>(EVENTS.LOCAL_AI_STATUS, apply)
    void refresh()
    let released = false
    return () => {
      if (released) return
      released = true
      if (--subscribers === 0) {
        off?.()
        off = undefined
      }
    }
  }

  /** 操作を 1 つ呼び、返った状態を反映する。失敗は `error` に入れる（投げない）。 */
  async function run(op: () => PromiseLike<LocalAIStatus>, pulling = false): Promise<void> {
    if (busy.value) return
    error.value = undefined
    noSpacePull.value = false
    busy.value = true
    const rev = revision
    try {
      const s = await call(op())
      if (rev === revision) apply(s) // 呼び出しの間に新しいイベントが届いていれば、そちらが新しい
    } catch (e) {
      const err = parseBindingError(e)
      error.value = err.message
      noSpacePull.value = pulling && err.details?.reason === 'no_space'
    } finally {
      busy.value = false
    }
  }

  const install = () => run(() => LocalAIService.InstallLocalAI())
  const pull = (name: string) => run(() => LocalAIService.PullLocalAIModel(name), true)
  const cancel = () => run(() => LocalAIService.CancelLocalAIJob())
  const stop = () => run(() => LocalAIService.StopLocalAI())
  const deleteModel = (name: string) => run(() => LocalAIService.DeleteLocalAIModel(name))
  const remove = () => run(() => LocalAIService.RemoveLocalAI())

  /** 設定（aiConfig）を取り直す。導入・モデルの取得で `local.ready` が変わったとき（と、つなぎ先・機能のタブを開いたとき）、つなぎ先の一覧・機能の選択肢へ反映する。失敗しても画面は今のまま。 */
  async function refreshConfig(): Promise<void> {
    try {
      settings.aiConfig = normalizeAIConfig(await call(AIService.GetAIConfig()))
    } catch {
      // 次に設定を開いたときに読み直される
    }
  }

  /** つなぎ方（managed／external と、external の Base URL）を保存する。設定（aiConfig）へ反映し、失敗は投げる（呼び出し側が項目の近くに出す）。 */
  async function setConnection(mode: 'managed' | 'external', baseUrl: string): Promise<void> {
    settings.aiConfig = normalizeAIConfig(await call(AIService.SetAILocalConnection(mode, baseUrl)))
  }

  /** 「接続テスト」。入力中の Base URL（空は managed）のモデル一覧を取る。取得できないときも例外にせず、reason・message つきの空の一覧が返る。 */
  async function testConnection(baseUrl: string): Promise<AIModelList> {
    const r = await call(AIService.ListAIModels('local', baseUrl))
    return { ...r, models: list(r.models) }
  }

  return {
    status,
    error,
    noSpacePull,
    busy,
    job,
    models,
    start,
    refresh,
    refreshConfig,
    install,
    pull,
    cancel,
    stop,
    deleteModel,
    remove,
    setConnection,
    testConnection,
  }
})
