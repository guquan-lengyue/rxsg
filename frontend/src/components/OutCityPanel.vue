<script setup lang="ts">
// 外城 / 城防面板：驻军 + 城防器械 + 城墙耐久说明。
// 驻军走 GET /cities/:cid/troops；城防器械走 GET /cities/:cid/defences（后端当前恒返回空数组）。
// 城墙耐久后端未暴露（cities 表无 wallhp 列、city handler 响应无城墙字段），此面板不伪造数值。
import SkinDialog from '@/components/SkinDialog.vue'
import { soldierIcon } from '@/assets/img'
import type { CityDefence, CitySoldier } from '@/types'

defineProps<{
  troops: CitySoldier[]
  defences: CityDefence[]
  loading: boolean
  error: string
}>()

const emit = defineEmits<{ (e: 'close'): void }>()

// 城防器械名对齐 cfg_defence（did 1..5）。
const DEFENCE_NAMES: Record<number, string> = {
  1: '陷阱',
  2: '拒马',
  3: '箭塔',
  4: '滚木',
  5: '擂石',
}

function defenceName(did: number): string {
  return DEFENCE_NAMES[did] ?? `器械${did}`
}
</script>

<template>
  <SkinDialog title="外城城防" wide @close="emit('close')">
    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <template v-else>
      <h4 class="block-title">驻军</h4>
      <p v-if="!troops.length" class="u-hint">暂无驻军</p>
      <ul v-else class="grid">
        <li v-for="t in troops" :key="t.sid" class="cell">
          <img class="soldier" :src="soldierIcon(t.sid)" :alt="`兵种${t.sid}`" />
          <span class="name">兵种 {{ t.sid }}</span>
          <strong class="count">{{ t.count }}</strong>
        </li>
      </ul>

      <h4 class="block-title">城防器械</h4>
      <p v-if="!defences.length" class="u-hint">暂无城防器械</p>
      <ul v-else class="grid">
        <li v-for="d in defences" :key="d.did" class="cell">
          <span class="name">{{ defenceName(d.did) }}</span>
          <strong class="count">{{ d.count }}</strong>
        </li>
      </ul>

      <h4 class="block-title">城墙耐久</h4>
      <p class="u-hint">城墙耐久暂未由后端提供。</p>
    </template>

    <template #footer>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.block-title {
  margin: 12px 0 4px;
  color: var(--accent);
  font-size: 13px;
  font-weight: normal;
  border-bottom: 1px solid var(--panel-border);
  padding-bottom: 4px;
}

.block-title:first-child {
  margin-top: 0;
}

.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
  gap: 8px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.cell {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  font-size: 13px;
}

.soldier {
  width: 32px;
  height: 32px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--panel);
}

.name {
  flex: 1 1 auto;
  min-width: 0;
}

.count {
  color: var(--accent);
}
</style>
