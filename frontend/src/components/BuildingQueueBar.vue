<script setup lang="ts">
// BuildingQueueBar —— 建筑队列条（对齐原版 TopPanel.buildingListPanel + BuildingItem）。
// 数据来源 GET /buildings/queue（后端返回中文任务名「正在建造/正在升级/正在拆除」）。
// 每秒本地推进倒计时（不额外请求），组件卸载时清理定时器；队列为空时不渲染。
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { formatLeft } from '@/render/cityGrid'
import type { BuildingQueueItem } from '@/types'

defineProps<{ items: BuildingQueueItem[] }>()

// 本地时钟（秒）：用于把 state_endtime 换算为实时剩余时间。
const nowSec = ref(Math.floor(Date.now() / 1000))
let timer: number | null = null

onMounted(() => {
  timer = window.setInterval(() => {
    nowSec.value = Math.floor(Date.now() / 1000)
  }, 1000)
})

onBeforeUnmount(() => {
  if (timer !== null) {
    window.clearInterval(timer)
  }
})
</script>

<template>
  <div v-if="items.length" class="queue-bar">
    <div v-for="item in items" :key="`${item.x}-${item.y}`" class="queue-item">
      <span class="task">{{ item.task }}</span>
      <span class="name">{{ item.bname }}</span>
      <span class="lv">{{ item.current_level }}→{{ item.target_level }}</span>
      <span class="left">{{ formatLeft(item.state_endtime - nowSec) }}</span>
    </div>
  </div>
</template>

<style scoped>
.queue-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 16px;
  padding: 8px 16px;
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  background: var(--panel);
}

.queue-item {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.task {
  color: var(--accent);
}

.name {
  color: var(--text);
}

.lv {
  color: var(--text-dim);
}

.left {
  color: #ffd479;
}
</style>
