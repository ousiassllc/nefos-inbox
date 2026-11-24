<script setup lang="ts">
import { WarningCircle } from '@iconoir/vue'
import { computed, onBeforeUnmount, watch } from 'vue'
import { gbNumber } from '../../../../lib/localAi/format'
import { settings as msg } from '../../../../lib/messages'
import { useLocalAiStore } from '../../../../stores/localAi'
import BaseButton from '../../../ui/BaseButton.vue'
import AILocalInstalled from './AILocalInstalled.vue'
import AILocalUnsupported from './AILocalUnsupported.vue'
import LocalAIProgress from './LocalAIProgress.vue'

// 「このアプリが用意する Ollama」（managed。#458）。状態は LocalAIStatus（開いたとき取得し、`localai:status` で更新する。閉じたら購読を解除）。
//   使えない PC → 理由とクラウドの案内 ／ 未導入 → 版・ダウンロード量・「Ollama を導入」 ／ 導入中 → 段階＋進捗＋「中止」 ／ 導入済み → AILocalInstalled。
// 失敗（直前のジョブの lastError・操作の失敗・空き不足）は role="alert" で出す。
const store = useLocalAiStore()
const t = msg.ai.local

const status = computed(() => store.status)
const installing = computed(() => (status.value?.job?.kind === 'install' ? status.value.job : undefined))
const alerts = computed(() => [store.error, status.value?.lastError].filter((m): m is string => !!m))

const stop = store.start()
onBeforeUnmount(stop)

// 導入・モデルの取得・削除でつなぎ先の「接続済み」（local.ready。導入済みかつモデルが 1 つ以上取得済み）が変わるので、設定（aiConfig）を取り直す。
// 最初の状態の取得（before が空）では取り直さない。
const readyKey = computed(() =>
  status.value ? `${status.value.installed}|${(status.value.models ?? []).filter((m) => m.installed).length}` : '',
)
watch(readyKey, (_, before) => {
  if (before !== '') void store.refreshConfig()
})
</script>

<template>
  <div class="managed" data-testid="local-ai-managed">
    <p v-if="alerts.length > 0" class="alert" role="alert" data-testid="local-ai-alert">
      <WarningCircle class="icon alert-icon" aria-hidden="true" />
      <span class="alert-text">
        <strong>{{ t.failedHeading }}</strong>
        <template v-for="m in alerts" :key="m"><br />{{ m }}</template>
      </span>
    </p>
    <p v-if="!status" class="loading" role="status">{{ t.statusLoading }}</p>
    <AILocalUnsupported v-else-if="!status.supported" :reason="status.unsupportedReason" />
    <div v-else-if="installing" class="install" data-testid="local-ai-installing">
      <LocalAIProgress :job="installing" />
      <div class="actions">
        <BaseButton variant="secondary" :disabled="store.busy" data-testid="local-ai-cancel" @click="store.cancel()">
          {{ t.cancel }}
        </BaseButton>
      </div>
    </div>
    <div v-else-if="!status.installed" class="install" data-testid="local-ai-not-installed">
      <h4 class="sub">{{ t.notInstalledHeading }}</h4>
      <p class="detail">{{ t.notInstalledDetail(status.version, `${gbNumber(status.downloadBytes)} GB`) }}</p>
      <div class="actions">
        <BaseButton variant="primary" :disabled="store.busy || status.job != null" data-testid="local-ai-install" @click="store.install()">
          {{ t.install }}
        </BaseButton>
      </div>
    </div>
    <AILocalInstalled v-else :status="status" />
  </div>
</template>

<style scoped>
.managed {
  display: flex;
  flex-direction: column;
  gap: var(--gap-field);
}

.install {
  display: flex;
  flex-direction: column;
  gap: var(--s-2);
}

.sub {
  margin: 0;
  font-weight: 600;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.detail,
.loading {
  margin: 0;
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.loading {
  color: var(--mute);
}

.actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--s-2);
}

.alert {
  display: flex;
  align-items: flex-start;
  gap: var(--s-3);
  margin: 0;
  padding: var(--s-3) var(--s-4);
  border: 1px solid var(--danger-border);
  border-radius: var(--r-md);
  background: var(--danger-soft);
  color: var(--ink);
  font-size: var(--font-size-sm);
  line-height: var(--line-height-sm);
}

.alert-icon {
  flex: none;
  margin-top: var(--s-1);
  color: var(--danger);
}

.alert-text {
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
