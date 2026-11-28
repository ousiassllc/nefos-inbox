<script setup lang="ts">
import { WarningCircle } from '@iconoir/vue'
import { computed, onBeforeUnmount } from 'vue'
import { gbNumber } from '../../../../lib/localAi/format'
import { settings as msg } from '../../../../lib/messages'
import { useLocalAiStore } from '../../../../stores/localAi'
import BaseButton from '../../../ui/BaseButton.vue'
import AILocalInstalled from './AILocalInstalled.vue'
import AILocalUnsupported from './AILocalUnsupported.vue'
import LocalAIProgress from './LocalAIProgress.vue'

// 「このアプリが用意する Ollama」（managed。#458）。状態は LocalAIStatus（開いたとき取得し、`localai:status` で更新する。閉じたら購読を解除）。
//   使えない PC → 理由と「すでに動いているサーバーにつなぐ」・クラウドの案内 ／ 未導入 → 版・ダウンロード量・「Ollama を導入」 ／ 導入中 → 段階＋進捗＋「中止」 ／ 導入済み → AILocalInstalled。
// 失敗（直前のジョブの lastError・操作の失敗・空き不足）は role="alert" で出す。つなぎ先の「接続済み」の取り直しは、ストアが状態の変化で行う（設定ダイアログが開いている間は購読を保つ）。
const emit = defineEmits<{ 'connect-external': [] }>()
const store = useLocalAiStore()
const t = msg.ai.local

const status = computed(() => store.status)
const installing = computed(() => (status.value?.job?.kind === 'install' ? status.value.job : undefined))
/** 同じ文面は 1 つだけ（操作の失敗と直前のジョブの失敗が同じ理由のとき）。 */
const alerts = computed(() => [
  ...new Set([store.error, status.value?.lastError].filter((m): m is string => !!m)),
])

const stop = store.start()
onBeforeUnmount(stop)
</script>

<template>
  <div class="managed" data-testid="local-ai-managed">
    <p v-if="alerts.length > 0" class="alert" role="alert" data-testid="local-ai-alert">
      <WarningCircle class="icon alert-icon" aria-hidden="true" />
      <span class="alert-text">
        <strong>{{ store.noSpacePull ? t.pullFailedHeading : t.failedHeading }}</strong>
        <template v-for="m in alerts" :key="m"><br />{{ m }}</template>
        <template v-if="store.noSpacePull"><br />{{ t.noSpacePullHint }}</template>
      </span>
    </p>
    <div v-if="!status">
      <p v-if="!store.error" class="loading" role="status">{{ t.statusLoading }}</p>
      <div v-else class="actions">
        <BaseButton variant="secondary" data-testid="local-ai-reload" @click="store.refresh()">{{ t.reload }}</BaseButton>
      </div>
    </div>
    <AILocalUnsupported
      v-else-if="!status.supported"
      :reason="status.unsupportedReason"
      @connect-external="emit('connect-external')"
    />
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
