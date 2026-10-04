<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { drawCityGrid } from '@/render/cityGrid'
import type { Building } from '@/types'

const props = defineProps<{ buildings: Building[] }>()

const canvas = ref<HTMLCanvasElement | null>(null)
let timer: number | null = null

function render(): void {
  if (canvas.value) {
    drawCityGrid(canvas.value, props.buildings)
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
    <canvas v-if="buildings.length" ref="canvas" />
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
}

.empty {
  margin: 24px 0;
  color: var(--text-dim);
  text-align: center;
}
</style>