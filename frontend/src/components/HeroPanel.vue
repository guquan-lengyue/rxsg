<script setup lang="ts">
import { computed, reactive } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { formatLeft } from '@/render/cityGrid'
import type { HeroInfo, HeroState } from '@/types'

const props = defineProps<{
  info: HeroInfo | null
  loading: boolean
  error: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'upgrade', hid: number): void
  (e: 'start-expr', hid: number, hours: number): void
  (e: 'cancel-expr', hid: number): void
  (e: 'close'): void
}>()

// 每武将的历练时长输入。
const hours = reactive<Record<number, number>>({})

const heroes = computed(() => props.info?.heroes ?? [])
const minHour = computed(() => props.info?.exprTypes[0]?.min_hour ?? 1)
const maxHour = computed(() => props.info?.exprTypes[0]?.max_hour ?? 12)

function stateText(s: number): string {
  switch (s) {
    case 0:
      return zhCN.hero.idle
    case 3:
      return zhCN.hero.inCity
    case 4:
      return zhCN.hero.out
    case 10:
      return zhCN.hero.expring
    case 11:
      return zhCN.hero.exprDone
    default:
      return String(s)
  }
}

// 当前等级内已获经验 / 升级所需经验。
function progressOf(h: HeroState): string {
  return `${h.exp - h.level_total_exp} / ${h.upgrade_exp}`
}

function onStartExpr(h: HeroState): void {
  emit('start-expr', h.hid, hours[h.hid] || minHour.value)
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal">
      <header class="modal-head">
        <strong>{{ zhCN.hero.title }}</strong>
        <button class="close" @click="emit('close')">×</button>
      </header>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>
      <p v-else-if="!heroes.length" class="hint">{{ zhCN.hero.empty }}</p>

      <ul v-else class="list">
        <li v-for="h in heroes" :key="h.hid" class="item">
          <div class="row">
            <strong>{{ h.name }}</strong>
            <span class="state">
              Lv{{ h.level }} · {{ stateText(h.state) }}
              <span v-if="h.expr"> · {{ zhCN.hero.expring }}</span>
            </span>
          </div>

          <div class="row dim small">
            <span>
              {{ zhCN.hero.exp }} {{ progressOf(h) }}
              <span v-if="!h.can_upgrade && h.no_upgrade_msg === '经验不足'">
                （{{ zhCN.hero.needExp }} {{ h.need_exp }}）
              </span>
            </span>
            <span>{{ zhCN.hero.loyalty }} {{ h.loyalty }}</span>
          </div>

          <div class="row dim small">
            <span>
              统{{ h.command_base + 0 }} 武{{ h.bravery_base + h.bravery_add }}
              智{{ h.wisdom_base + h.wisdom_add }} 政{{ h.affairs_base + h.affairs_add }}
            </span>
            <span>攻{{ h.attack_base + h.attack_add_on }} 防{{ h.defence_base + h.defence_add_on }}</span>
          </div>

          <!-- 历练进行中：显示倒计时与取消 -->
          <div v-if="h.expr" class="row">
            <span class="dim small">
              {{ zhCN.hero.expr }} {{ h.expr.hours }}h ·
              {{ formatLeft(h.expr.time_left) }} · +{{ h.expr.exp_gain }}
            </span>
            <button class="ghost" :disabled="busy" @click="emit('cancel-expr', h.hid)">
              {{ zhCN.hero.cancelExpr }}
            </button>
          </div>

          <!-- 空闲/在城：可升级与开始历练 -->
          <div v-else class="row">
            <div class="expr">
              <span class="dim small">{{ zhCN.hero.exprTime }}</span>
              <input
                v-model.number="hours[h.hid]"
                type="number"
                :min="minHour"
                :max="maxHour"
                class="num"
              />
            </div>
            <div class="btns">
              <button
                class="primary"
                :disabled="busy || !h.can_upgrade"
                :title="h.no_upgrade_msg"
                @click="emit('upgrade', h.hid)"
              >
                {{ h.level >= 0 && h.no_upgrade_msg === '已达最高等级'
                  ? zhCN.hero.maxLevel
                  : zhCN.hero.upgrade }}
              </button>
              <button
                class="ghost"
                :disabled="busy || h.state !== 0 && h.state !== 3"
                @click="onStartExpr(h)"
              >
                {{ zhCN.hero.startExpr }}
              </button>
            </div>
          </div>
        </li>
      </ul>
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

.list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  font-size: 13px;
}

.dim {
  color: var(--text-dim);
}

.small {
  font-size: 12px;
}

.state {
  color: var(--accent);
  font-size: 13px;
}

.expr {
  display: flex;
  align-items: center;
  gap: 6px;
}

.num {
  width: 64px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.btns {
  display: flex;
  gap: 8px;
}

.btns button {
  padding: 5px 14px;
  border-radius: 4px;
  cursor: pointer;
}

.primary {
  color: #10202e;
  background: var(--accent);
  border: 1px solid var(--accent);
}

.ghost {
  color: var(--text);
  background: transparent;
  border: 1px solid var(--panel-border);
}

button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.hint {
  color: var(--text-dim);
  font-size: 13px;
}

.hint.error {
  color: var(--danger);
}
</style>