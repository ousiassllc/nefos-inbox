<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useLocalAiStore } from '../../../../stores/localAi'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import BaseButton from '../../../ui/BaseButton.vue'
import SettingsInput from '../../SettingsInput.vue'
import AIApiKey from '../AIApiKey.vue'
import AIConsentSection from '../AIConsentSection.vue'
import AIDestinationBox from '../AIDestinationBox.vue'
import { type AiBusy, useAiConnect } from '../useAiConnect'

// 「すでに動いているサーバーにつなぐ」（external。Ollama・LM Studio など。#458）。Base URL の保存・接続テスト・任意の API キー・
// 送信先が PC の外（loopback 以外）のときだけ送信への同意。接続テストは保存前の入力中の Base URL でも押せる（モデル一覧の取得で確かめる）。
const store = useSettingsStore()
const local = useLocalAiStore()
const ui = useUiStore()
const t = msg.ai.local

const errors = ref<{ key?: string; consent?: string; baseUrl?: string }>({})
const keyForm = ref<InstanceType<typeof AIApiKey>>()
const busy = ref<AiBusy>()
const consentOpen = ref(false)
const provider = ref('local')

/** 入力中の Base URL。親が持つ（managed に切り替えて戻っても消さない）。undefined は、まだ開いていない。 */
const draft = defineModel<string | undefined>('draft')

const c = useAiConnect({ provider, busy, errors, consentOpen, keyForm, chatgptLogin: ref(undefined) })

/** 保存済みの Base URL（つなぎ方が external のときだけ。managed は空）。 */
const saved = computed(() => c.baseUrl.value)
if (draft.value === undefined) draft.value = saved.value // 初めて開いたときは保存済みの値から始める
const baseUrlInput = computed({
  get: () => draft.value ?? '',
  set: (v: string) => (draft.value = v),
})
const trimmed = computed(() => baseUrlInput.value.trim())
const dirty = computed(() => trimmed.value !== saved.value)
const hasKey = computed(() => (store.aiConfig?.apiKeyProviders ?? []).includes('local'))
/** 送信先が PC の外の保存済みの Base URL のときだけ、送信への同意を求める（loopback は同意の欄を出さない）。 */
const needsConsentSection = computed(() => saved.value !== '' && !(store.aiConfig?.local?.loopback ?? false))

/** 接続テストの結果。warn は接続できたがモデルが 0 件（成功の緑にしない）。 */
type TestResult = { kind: 'ok' | 'warn' | 'ng'; text: string }
const testing = ref(false)
const result = ref<TestResult>()
// 入力を変えたら、前の入力への結果は消す。
watch(trimmed, () => (result.value = undefined))

async function save(): Promise<void> {
  if (busy.value || !dirty.value) return
  errors.value = {}
  if (trimmed.value === '') {
    errors.value = { baseUrl: t.baseUrlRequired }
    return
  }
  busy.value = 'baseUrl'
  try {
    await local.setConnection('external', trimmed.value)
    baseUrlInput.value = trimmed.value
    result.value = undefined
    ui.pushToast('success', t.baseUrlSaved)
  } catch (e) {
    errors.value = { baseUrl: parseBindingError(e).message }
  } finally {
    busy.value = undefined
  }
}

/** 「接続テスト」。入力中の Base URL のモデル一覧を取り、件数か、取れなかった理由（Go の message）を出す。 */
async function test(): Promise<void> {
  if (testing.value) return
  if (trimmed.value === '') {
    result.value = { kind: 'ng', text: t.baseUrlRequired }
    return
  }
  const asked = trimmed.value
  testing.value = true
  result.value = undefined
  try {
    const r = await local.testConnection(asked)
    if (trimmed.value !== asked) return // テスト中に入力が変わった。結果は前の入力のものなので捨てる
    const n = r.models?.length ?? 0
    if (n > 0) result.value = { kind: 'ok', text: t.testOk(n) }
    else if (r.message !== undefined || r.reason !== undefined) result.value = { kind: 'ng', text: r.message ?? t.testFailed }
    else result.value = { kind: 'warn', text: t.testNone }
  } catch (e) {
    if (trimmed.value !== asked) return
    result.value = { kind: 'ng', text: parseBindingError(e).message || t.testFailed }
  } finally {
    testing.value = false
  }
}
</script>

<template>
  <div class="external" data-testid="local-ai-external">
    <p class="intro">{{ t.externalIntro }}</p>

    <AIDestinationBox
      provider="local"
      :base-url="saved"
      :loopback="store.aiConfig?.local?.loopback"
      draft-provider="local"
      :draft-base-url="trimmed"
    />

    <div class="form">
      <SettingsInput
        v-model="baseUrlInput"
        mono
        type="url"
        :label="t.baseUrl"
        :placeholder="t.baseUrlPlaceholder"
        :hint="t.baseUrlHint"
        :error="errors.baseUrl"
        @enter="save"
      />
      <div class="actions">
        <BaseButton variant="secondary" :loading="testing" data-testid="local-ai-test" @click="test">{{ t.test }}</BaseButton>
        <BaseButton variant="primary" :disabled="!dirty" :loading="busy === 'baseUrl'" data-testid="local-ai-base-url-save" @click="save">
          {{ t.baseUrlSave }}
        </BaseButton>
      </div>
      <p
        v-if="result"
        class="result"
        :class="result.kind"
        :role="result.kind === 'ng' ? 'alert' : 'status'"
        data-testid="local-ai-test-result"
      >
        {{ result.text }}
      </p>
    </div>

    <AIApiKey
      ref="keyForm"
      :provider-label="t.name"
      :has-key="hasKey"
      :busy="busy"
      :error="errors.key"
      :heading="t.apiKeyHeading"
      :hint="t.apiKeyOptional"
      @save="c.saveKey"
      @clear="c.clearKey"
    />

    <AIConsentSection
      v-if="needsConsentSection"
      :text="c.consentText.value"
      :error="errors.consent"
      :needed="!c.consented.value"
      :disabled="busy !== undefined || dirty"
      :destination="c.destination.value"
      :open="consentOpen"
      :saving="busy === 'consent'"
      @ask="consentOpen = true"
      @agree="c.agree"
      @decline="c.declineConsent"
    />
  </div>
</template>

<style scoped>
.external {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.intro {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.form {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}

.result {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.ok {
  color: var(--success);
}

.warn {
  color: var(--warning);
}

.ng {
  color: var(--danger);
}
</style>
