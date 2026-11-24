<script setup lang="ts">
import { computed, onMounted, ref, useId, watch } from 'vue'
import { parseBindingError } from '../../../../lib/api'
import { settings as msg } from '../../../../lib/messages'
import { useLocalAiStore } from '../../../../stores/localAi'
import { useSettingsStore } from '../../../../stores/settings'
import { useUiStore } from '../../../../stores/ui'
import AILocalExternalSection from './AILocalExternalSection.vue'
import AILocalManagedSection from './AILocalManagedSection.vue'

// ローカルAI（Ollama。#458。Windows のみ）のページ（「AI のつなぎ先」の一覧の行から開く）。ここで行うのは、つなぎ方の選択と、そのつなぎ方の準備
// （managed = Ollama の導入・モデルの取得、external = Base URL・接続テスト・任意の API キー・同意）。使うモデルは「機能ごとに使う AI」で選ぶ。
// つなぎ方のラジオ: managed を選ぶとすぐ保存する。external は Base URL が要るので、選ぶと入力欄を出し、「保存」で SetAILocalConnection を呼ぶ。
const store = useSettingsStore()
const local = useLocalAiStore()
const ui = useUiStore()
const t = msg.ai.local

const savedMode = computed<'managed' | 'external'>(() => (store.aiConfig?.local?.mode === 'external' ? 'external' : 'managed'))
const mode = ref<'managed' | 'external'>(savedMode.value)
const error = ref<string>()
// 設定の読み込みが遅れて保存済みのつなぎ方が分かったとき・保存で変わったときは、それに合わせる。
watch(savedMode, (m) => (mode.value = m))
const labelId = useId()

async function choose(next: 'managed' | 'external'): Promise<void> {
  error.value = undefined
  mode.value = next
  if (next === 'external' || next === savedMode.value) return
  try {
    await local.setConnection('managed', '')
  } catch (e) {
    mode.value = savedMode.value
    error.value = parseBindingError(e).message
  }
}

// 「ログイン」「API キーを追加」から来た印（takeAiConnectStart）は、このページでは使わない。消しておく（次に開くページで効かないように）。
onMounted(() => {
  ui.takeAiConnectStart()
})
</script>

<template>
  <section class="ai" :aria-label="t.name" data-testid="ai-local-page">
    <div class="group">
      <h3 class="heading">{{ t.name }}</h3>
      <p class="intro">{{ t.pageIntro }}</p>
    </div>

    <div class="modes">
      <h4 :id="labelId" class="sub">{{ t.modeLabel }}</h4>
      <div class="radios" role="radiogroup" :aria-labelledby="labelId" data-testid="local-ai-mode">
        <label class="radio">
          <input
            type="radio"
            name="local-ai-mode"
            value="managed"
            :checked="mode === 'managed'"
            data-testid="local-ai-mode-managed"
            @change="choose('managed')"
          />
          <span>{{ t.modeManaged }}</span>
        </label>
        <label class="radio">
          <input
            type="radio"
            name="local-ai-mode"
            value="external"
            :checked="mode === 'external'"
            data-testid="local-ai-mode-external"
            @change="choose('external')"
          />
          <span>{{ t.modeExternal }}</span>
        </label>
      </div>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
    </div>

    <template v-if="mode === 'managed'">
      <p class="note" data-testid="local-ai-note">{{ t.noKeyNote }}</p>
      <AILocalManagedSection />
    </template>
    <AILocalExternalSection v-else />
  </section>
</template>

<style scoped>
.ai {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.group,
.modes {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.heading {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-base);
}

.sub {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.intro,
.note {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.radios {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.radio {
  display: flex;
  align-items: flex-start;
  gap: var(--s-2);
  min-height: var(--control-h);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
  cursor: pointer;
}

.radio input {
  flex: none;
  margin: 0;
  width: 16px;
  height: 16px;
  margin-top: 2px;
  accent-color: var(--accent);
}

.error {
  margin: 0;
  color: var(--danger);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}
</style>
