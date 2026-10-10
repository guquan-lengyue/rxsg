<script setup lang="ts">
// TipBox —— 通用悬浮提示（原版 board_tip / board_hint 皮肤）。
// 渲染到 body 并用 (x, y) 定位（配合鼠标事件取 clientX/clientY）；
// 不引用任何 store，文案全部由 props 传入。
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    x: number
    y: number
    title?: string
    lines?: string[]
    kind?: 'tip' | 'hint'
  }>(),
  { kind: 'tip' },
)

const skinClass = computed(() => (props.kind === 'hint' ? 'u-hint-box' : 'u-tip'))
</script>

<template>
  <Teleport to="body">
    <div class="tip-box" :class="skinClass" :style="{ left: `${x}px`, top: `${y}px` }">
      <strong v-if="title" class="tip-title">{{ title }}</strong>
      <p v-for="(line, i) in lines" :key="i" class="tip-line">{{ line }}</p>
    </div>
  </Teleport>
</template>

<style scoped>
.tip-box {
  position: fixed;
  z-index: 1000;
  max-width: 260px;
  padding: 6px 8px;
  color: var(--text);
  font-size: 12px;
  line-height: 1.5;
  pointer-events: none;
}

.tip-title {
  display: block;
  margin-bottom: 2px;
  color: var(--accent);
  font-weight: 700;
}

.tip-line {
  margin: 0;
}
</style>
