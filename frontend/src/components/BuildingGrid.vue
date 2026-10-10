<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { buildingAt, CELL_SIZE, drawCityGrid } from '@/render/cityGrid'
import type { Building } from '@/types'

const props = defineProps<{ buildings: Building[]; selected?: Building | null }>()
const emit = defineEmits<{
  (e: 'select', b: Building): void
  (e: 'select-empty', pos: { x: number; y: number }): void
}>()

const canvas = ref<HTMLCanvasElement | null>(null)
let timer: number | null = null

function render(): void {
  if (canvas.value) {
    drawCityGrid(canvas.value, props.buildings)
  }
}

function onClick(event: MouseEvent): void {
  const el = canvas.value
  if (!el) {
    return
  }
  const hit = buildingAt(props.buildings, event.offsetX, event.offsetY, CELL_SIZE)
  if (hit) {
    emit('select', hit)
    return
  }
  // 空地格：把像素坐标换算为格子（画布尺寸 = 列/行数 × CELL_SIZE，offsetX/Y 恒在画布内）。
  const x = Math.floor(event.offsetX / CELL_SIZE)
  const y = Math.floor(event.offsetY / CELL_SIZE)
  if (x >= 0 && y >= 0) {
    emit('select-empty', { x, y })
  }
}

onMounted(() => {
  render()
  // 每秒重绘以刷新升级倒计时
  timer = window.setInterval(render, 1000)
})

onBeforeUnmount(() => {
  if (timer !== null) {
    window.clearInterval(timer)
  }
})

watch(() => props.buildings, render, { flush: 'post' })
</script>

<template>
  <div class="grid-wrap">
    <canvas v-if="buildings.length" ref="canvas" @click="onClick" />
    <p v-else class="empty">暂无建筑数据</p>
  </div>
</template>

<style scoped>
.grid-wrap {
  overflow: auto;
  padding: 16px;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.grid-wrap canvas {
  display: block;
  cursor: pointer;
}

.empty {
  margin: 24px 0;
  color: var(--text-dim);
  text-align: center;
}
</style>