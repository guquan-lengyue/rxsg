<script setup lang="ts">
// BuildDialog —— 建造建筑候选列表（对齐原版 CreateBuildingDialog / getAllValidBuilding）。
// 仅接收 props、向外 emit（不直接访问 store）；候选数据与接口调用由 CityView 编排。
// 文案取原版 locale：Building_CreateBuildingDialog_1_1=建造建筑、Building_CreateBuildingItem_1_1=建造、
// _CreateBuildingItemSrc_1=建造:、_3..8=当前人口/消耗粮食/木材/石料/铁锭/黄金。
import { ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { buildingIntro } from '@/assets/img'
import { zhCN } from '@/lang/zh-CN'
import { formatLeft } from '@/render/cityGrid'
import type { BuildingCandidate } from '@/types'

const props = defineProps<{
  candidates: BuildingCandidate[]
  loading: boolean
  error: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'build', bid: number): void
  (e: 'close'): void
}>()

// 二次确认：记录待建造的候选（点击「建造」后先弹确认，再由 CityView 调接口）。
const confirming = ref<BuildingCandidate | null>(null)

function askBuild(c: BuildingCandidate): void {
  if (props.busy || !c.canUpgrade) {
    return
  }
  confirming.value = c
}

function cancelConfirm(): void {
  confirming.value = null
}

function confirmBuild(): void {
  const c = confirming.value
  if (!c) {
    return
  }
  emit('build', c.bid)
}

function costs(c: BuildingCandidate): { key: string; label: string; value: number }[] {
  return [
    { key: 'wood', label: zhCN.resource.wood, value: c.woodNeed },
    { key: 'rock', label: zhCN.resource.rock, value: c.rockNeed },
    { key: 'iron', label: zhCN.resource.iron, value: c.ironNeed },
    { key: 'food', label: zhCN.resource.food, value: c.foodNeed },
    { key: 'gold', label: zhCN.resource.gold, value: c.goldNeed },
  ].filter((it) => it.value > 0)
}
</script>

<template>
  <SkinDialog :title="zhCN.city.buildTitle" @close="emit('close')">
    <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
    <p v-else-if="error" class="hint error">{{ error }}</p>

    <template v-else>
      <p v-if="!candidates.length" class="hint">{{ zhCN.city.buildEmpty }}</p>

      <ul class="cand-list">
        <li v-for="c in candidates" :key="c.bid" class="cand" :class="{ disabled: !c.canUpgrade }">
          <div class="cand-head">
            <img class="thumb" :src="buildingIntro(c.bid)" :alt="c.name" />
            <div class="meta">
              <strong>{{ c.name }}</strong>
              <span class="lv">({{ zhCN.city.buildLevel }} {{ c.level }})</span>
              <p v-if="c.levelDescription" class="desc">{{ c.levelDescription }}</p>
            </div>
            <span class="time">{{ zhCN.city.buildTime }} {{ formatLeft(c.upgradeTime) }}</span>
          </div>

          <ul class="costs">
            <li v-for="it in costs(c)" :key="it.key">
              <span>{{ it.label }}</span>
              <span>{{ it.value }}</span>
            </li>
            <li v-if="c.peopleNeed > 0">
              <span>{{ zhCN.resource.people }}</span>
              <span>{{ c.peopleNeed }}</span>
            </li>
          </ul>

          <ul v-if="c.conditions.length" class="conds">
            <li v-for="(cond, i) in c.conditions" :key="i" :class="{ unmet: !cond.canUpgrade }">
              <span class="ctype">{{ cond.type }}</span>
              <span>{{ cond.upgradeNeed }}</span>
              <span class="own">（{{ cond.currentOwn }}）</span>
            </li>
          </ul>

          <div class="actions">
            <button class="build" :disabled="busy || !c.canUpgrade" @click="askBuild(c)">
              {{ zhCN.city.build }}
            </button>
          </div>
        </li>
      </ul>
    </template>

    <!-- 建造二次确认 -->
    <div v-if="confirming" class="confirm-mask">
      <div class="confirm u-modal">
        <p class="confirm-text">{{ zhCN.city.buildConfirm(confirming?.name ?? '') }}</p>
        <div class="confirm-actions">
          <button class="ok" :disabled="busy" @click="confirmBuild">{{ zhCN.city.ok }}</button>
          <button class="cancel" :disabled="busy" @click="cancelConfirm">
            {{ zhCN.city.cancel }}
          </button>
        </div>
      </div>
    </div>
  </SkinDialog>
</template>

<style scoped>
.cand-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.cand {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.cand.disabled {
  opacity: 0.6;
}

.cand-head {
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.thumb {
  width: 44px;
  height: 44px;
  flex: 0 0 auto;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--panel);
}

.meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.meta strong {
  color: var(--text);
}

.meta .lv {
  color: var(--text-dim);
  font-size: 12px;
}

.desc {
  margin: 0;
  color: var(--text-dim);
  font-size: 12px;
}

.time {
  flex: 0 0 auto;
  color: var(--text-dim);
  font-size: 12px;
}

.costs {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 16px;
  margin: 0;
  padding: 6px 0;
  list-style: none;
  border-top: 1px solid var(--panel-border);
}

.costs li {
  display: flex;
  gap: 6px;
  font-size: 13px;
  color: var(--text-dim);
}

.conds {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
  color: var(--text-dim);
}

.conds .ctype {
  margin-right: 6px;
}

.conds .own {
  margin-left: 4px;
}

.conds li.unmet {
  color: var(--danger);
}

.actions {
  display: flex;
  justify-content: flex-end;
}

.actions button {
  padding: 5px 16px;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.build {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.build:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.confirm-mask {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.5);
}

.confirm {
  display: flex;
  flex-direction: column;
  gap: 14px;
  padding: 18px 20px;
  background: var(--panel);
}

.confirm-text {
  margin: 0;
  color: var(--text);
  font-size: 14px;
}

.confirm-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.confirm-actions button {
  min-width: 64px;
  padding: 5px 14px;
  color: var(--text);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.confirm-actions .ok {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.confirm-actions .cancel {
  background: transparent;
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}
</style>
