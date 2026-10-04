<script setup lang="ts">
import { computed } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { formatLeft } from '@/render/cityGrid'
import { techIcon } from '@/assets/img'
import type { TechnicInfo, TechnicState } from '@/types'

const props = defineProps<{
  info: TechnicInfo | null
  loading: boolean
  error: string
  busyTid: number
}>()
const emit = defineEmits<{
  (e: 'upgrade', tid: number): void
  (e: 'stop', tid: number): void
  (e: 'close'): void
}>()

const list = computed<TechnicState[]>(() => props.info?.technics ?? [])

function costs(t: TechnicState): { key: string; label: string; value: number }[] {
  return [
    { key: 'wood', label: zhCN.resource.wood, value: t.woodNeed },
    { key: 'rock', label: zhCN.resource.rock, value: t.rockNeed },
    { key: 'iron', label: zhCN.resource.iron, value: t.ironNeed },
    { key: 'food', label: zhCN.resource.food, value: t.foodNeed },
    { key: 'gold', label: zhCN.resource.gold, value: t.goldNeed },
  ].filter((c) => c.value > 0)
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal">
      <header class="modal-head">
        <strong>{{ zhCN.technic.title }}</strong>
        <button class="close" @click="emit('close')">×</button>
      </header>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>

      <template v-else>
        <p v-if="info && info.collegeCount === 0" class="hint error">
          {{ zhCN.technic.noCollege }}
        </p>
        <p v-if="!list.length" class="hint">{{ zhCN.technic.empty }}</p>

        <ul class="tech-list">
          <li v-for="t in list" :key="t.tid" class="tech">
            <div class="tech-head">
              <span class="tech-title">
                <img class="ticon" :src="techIcon(t.tid)" :alt="t.tname" />
                <strong>{{ t.tname }}</strong>
              </span>
              <span class="lv">
                Lv{{ t.level }}
                <span v-if="t.sharelevel" class="share">
                  {{ zhCN.technic.share }} {{ t.sharelevel }}
                </span>
              </span>
            </div>
            <p v-if="t.description" class="desc">{{ t.description }}</p>

            <div v-if="t.state === 1" class="row">
              <span class="label">{{ zhCN.technic.researching }}</span>
              <span class="countdown">{{ formatLeft(t.state_timeleft) }}</span>
            </div>
            <template v-else>
              <div class="row">
                <span class="label">{{ zhCN.technic.nextLevel }}</span>
                <span>Lv{{ t.level + 1 }}</span>
              </div>
              <div class="row">
                <span class="label">{{ zhCN.technic.time }}</span>
                <span>{{ formatLeft(t.upgrade_time) }}</span>
              </div>
              <ul class="costs">
                <li v-for="c in costs(t)" :key="c.key">
                  <span>{{ c.label }}</span>
                  <span>{{ c.value }}</span>
                </li>
              </ul>
            </template>

            <p v-if="t.state !== 1 && t.no_upgrade_msg" class="hint error">
              {{ t.no_upgrade_msg }}
            </p>

            <div class="actions">
              <button
                v-if="t.state === 1"
                class="stop"
                :disabled="busyTid === t.tid"
                @click="emit('stop', t.tid)"
              >
                {{ zhCN.technic.cancel }}
              </button>
              <button
                v-else
                class="upgrade"
                :disabled="busyTid === t.tid || !t.can_upgrade"
                @click="emit('upgrade', t.tid)"
              >
                {{ zhCN.technic.research }}
              </button>
            </div>
          </li>
        </ul>
      </template>
    </div>
  </div>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: rgba(0, 0, 0, 0.55);
}

.modal {
  display: flex;
  flex-direction: column;
  gap: 10px;
  width: 100%;
  max-width: 560px;
  max-height: 86vh;
  padding: 16px;
  overflow-y: auto;
  background: var(--panel);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.modal-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.modal-head strong {
  color: var(--accent);
  font-size: 16px;
}

.close {
  color: var(--text-dim);
  background: transparent;
  border: none;
  font-size: 20px;
  line-height: 1;
  cursor: pointer;
}

.tech-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.tech {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.tech-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.tech-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.tech-head strong {
  color: var(--text);
}

.ticon {
  width: 36px;
  height: 36px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--panel);
}

.lv {
  color: var(--accent);
  font-size: 13px;
}

.share {
  margin-left: 8px;
  color: var(--text-dim);
  font-size: 12px;
}

.desc {
  margin: 0;
  color: var(--text-dim);
  font-size: 13px;
}

.row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
}

.label {
  color: var(--text-dim);
}

.countdown {
  color: var(--accent);
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

.actions {
  display: flex;
  justify-content: flex-end;
}

.actions button {
  padding: 5px 16px;
  color: var(--text);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.upgrade {
  background: var(--accent);
  color: #10202e;
  border-color: var(--accent);
}

.upgrade:disabled,
.stop:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.stop {
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