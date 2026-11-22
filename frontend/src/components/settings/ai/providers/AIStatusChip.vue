<script setup lang="ts">
// 設定の AI の状態チップ（接続済み・未接続・つなぎ直しが必要・準備中）。色だけでなく文言で伝える。
// ガイド C4 のステータスのピル: 文字は状態色・枠は 40%・地は 10%・--r-full（ChatGPTAccountRow の .badge と同じ定形）。
// off は中立（--mute）で同じ形にする。
defineProps<{ kind: 'ok' | 'off' | 'warn' | 'wait' }>()
</script>

<template>
  <span class="status" :class="`status-${kind}`" :data-status="kind"><slot /></span>
</template>

<style scoped>
.status {
  flex: none;
  display: inline-flex;
  align-items: center;
  height: 20px;
  padding: 0 var(--s-2);
  border: 1px solid var(--status-border);
  border-radius: var(--r-full);
  background: var(--status-soft);
  color: var(--status-color);
  font-size: var(--font-size-xs);
  line-height: 1;
  white-space: nowrap;
}

.status-ok {
  --status-color: var(--success);
  --status-border: var(--success-border);
  --status-soft: var(--success-soft);
}

.status-warn {
  --status-color: var(--danger);
  --status-border: var(--danger-border);
  --status-soft: var(--danger-soft);
}

.status-wait {
  --status-color: var(--info);
  --status-border: var(--info-border);
  --status-soft: var(--info-soft);
}

.status-off {
  --status-color: var(--mute);
  --status-border: color-mix(in srgb, var(--mute) 40%, transparent);
  --status-soft: color-mix(in srgb, var(--mute) 10%, transparent);
}
</style>
