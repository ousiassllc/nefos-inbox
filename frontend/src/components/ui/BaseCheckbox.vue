<script setup lang="ts">
import { Check } from '@iconoir/vue'

// 属性（data-testid など）は外側の span ではなく <input> へ渡す
defineOptions({ inheritAttrs: false })
defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>

<template>
  <span class="checkbox">
    <input
      v-bind="$attrs"
      type="checkbox"
      class="box"
      :checked="modelValue"
      @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
    />
    <Check class="icon icon-sm mark" aria-hidden="true" />
  </span>
</template>

<style scoped>
/* ガイドのチェックボックス（O8）。ブラウザ標準にせず、ChatGPTAccountRow の .radio と同じ作りにする。
   外枠 --mute・18px・--r-sm・選択時は --accent 地に --bone のチェック（Iconoir の Check を重ねる）・フォーカスは --accent の outline（3px・offset 2px）・無効は opacity .4。 */
.checkbox {
  position: relative;
  flex: none;
  display: grid;
}

.box {
  appearance: none;
  grid-area: 1 / 1;
  width: 18px;
  height: 18px;
  margin: 0;
  border: 1px solid var(--mute);
  border-radius: var(--r-sm);
  background: var(--surface);
  cursor: pointer;
}

.box:checked {
  border-color: var(--accent);
  background: var(--accent);
}

/* チェックはクリックを素通しにして <input> を押させる。選択中だけ出す */
.mark {
  grid-area: 1 / 1;
  place-self: center;
  color: var(--bone);
  pointer-events: none;
}

.box:not(:checked) + .mark {
  display: none;
}

.box:disabled + .mark {
  opacity: 0.4;
}

.box:focus-visible {
  outline: 3px solid var(--accent);
  outline-offset: 2px;
}

.box:disabled {
  opacity: 0.4;
  cursor: default;
}
</style>
