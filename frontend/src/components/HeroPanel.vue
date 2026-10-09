<script setup lang="ts">
import { computed, reactive, watch } from 'vue'

import { zhCN } from '@/lang/zh-CN'
import { formatLeft } from '@/render/cityGrid'
import { heroFace } from '@/assets/img'
import type { HeroExprType, HeroInfo, HeroState } from '@/types'

const props = defineProps<{
  info: HeroInfo | null
  loading: boolean
  error: string
  busy: boolean
}>()
const emit = defineEmits<{
  (e: 'upgrade', hid: number): void
  (e: 'add-point', hid: number, affairs: number, bravery: number, wisdom: number): void
  (e: 'clear-point', hid: number): void
  (e: 'start-expr', hid: number, exprType: number, hours: number, carrymoney: number): void
  (e: 'cancel-expr', hid: number): void
  (e: 'faster-expr', hid: number): void
  (e: 'close'): void
}>()

// 每武将的历练表单（类型/时长/随带元宝）。
const exprForm = reactive<Record<number, { exprType: number; hours: number; carrymoney: number }>>({})
// 每武将的加点目标值（legacy addHeroPoint 传目标总值）。
const pointForm = reactive<Record<number, { affairs: number; bravery: number; wisdom: number }>>({})

const heroes = computed(() => props.info?.heroes ?? [])
const exprTypes = computed(() => props.info?.exprTypes ?? [])
const firstType = computed<HeroExprType>(
  () =>
    exprTypes.value[0] ?? { type: 1, name: '', min_hour: 1, max_hour: 24, hour_money: 0, hour_gold: 0 },
)

// 首次出现某武将时初始化其表单（不覆盖用户已输入值）。
watch(
  () => props.info,
  (info) => {
    if (!info) {
      return
    }
    for (const h of info.heroes) {
      if (!exprForm[h.hid]) {
        exprForm[h.hid] = {
          exprType: firstType.value.type,
          hours: firstType.value.min_hour,
          carrymoney: 0,
        }
      }
      if (!pointForm[h.hid]) {
        pointForm[h.hid] = {
          affairs: h.affairs_base + h.affairs_add,
          bravery: h.bravery_base + h.bravery_add,
          wisdom: h.wisdom_base + h.wisdom_add,
        }
      }
    }
  },
  { immediate: true },
)

// stateText 对齐 legacy sys_city_hero.state 值集 {0,1,4,7,8,10,11}。
function stateText(s: number): string {
  switch (s) {
    case 0:
      return zhCN.hero.idle
    case 1:
      return zhCN.hero.cityChief
    case 4:
      return zhCN.hero.out
    case 7:
      return zhCN.hero.general
    case 8:
      return zhCN.hero.counsellor
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
  const f = exprForm[h.hid]
  emit('start-expr', h.hid, f.exprType, f.hours || firstType.value.min_hour, f.carrymoney || 0)
}

function onAddPoint(h: HeroState): void {
  const p = pointForm[h.hid]
  emit('add-point', h.hid, p.affairs, p.bravery, p.wisdom)
}

function inCity(h: HeroState): boolean {
  return h.state === 0 || h.state === 1 || h.state === 7 || h.state === 8
}
</script>

<template>
  <div class="overlay" @click.self="emit('close')">
    <div class="modal u-modal">
      <header class="u-title">
        <strong>{{ zhCN.hero.title }}</strong>
        <button class="u-close" title="关闭" @click="emit('close')"></button>
      </header>

      <p v-if="loading" class="hint">{{ zhCN.city.loading }}</p>
      <p v-else-if="error" class="hint error">{{ error }}</p>
      <p v-else-if="info?.toomany" class="hint warn">{{ info.toomany }}</p>
      <p v-else-if="!heroes.length" class="hint">{{ zhCN.hero.empty }}</p>

      <ul v-else class="list">
        <li v-for="h in heroes" :key="h.hid" class="item">
          <div class="row">
            <img class="face" :src="heroFace(h.sex, h.face)" :alt="h.name" />
            <div class="head-main">
              <div class="row">
                <strong>{{ h.name }}</strong>
                <span class="state">
                  Lv{{ h.level }} · {{ stateText(h.state) }}
                  <span v-if="h.expr"> · {{ h.expr.expr_name }}{{ h.expr.acc_times ? `×${h.expr.acc_times}` : '' }}</span>
                </span>
              </div>

              <div class="row dim small">
                <span>
                  {{ zhCN.hero.exp }} {{ progressOf(h) }}
                  <span v-if="!h.can_upgrade && h.need_exp > 0">
                    （{{ zhCN.hero.needExp }} {{ h.need_exp }}）
                  </span>
                </span>
                <span>{{ zhCN.hero.loyalty }} {{ h.loyalty }}</span>
              </div>

              <div class="row dim small">
                <span>
                  统{{ h.command_base + h.command_add_on }}
                  武{{ h.bravery_base + h.bravery_add }}
                  智{{ h.wisdom_base + h.wisdom_add }}
                  政{{ h.affairs_base + h.affairs_add }}
                </span>
                <span>攻{{ h.attack_base + h.attack_add_on }} 防{{ h.defence_base + h.defence_add_on }}</span>
              </div>
            </div>
          </div>

          <!-- 加点/洗点（在城可用） -->
          <div v-if="inCity(h) && !h.expr" class="row form">
            <span class="dim small">{{ zhCN.hero.affairs }}</span>
            <input v-model.number="pointForm[h.hid].affairs" type="number" class="num" />
            <span class="dim small">{{ zhCN.hero.bravery }}</span>
            <input v-model.number="pointForm[h.hid].bravery" type="number" class="num" />
            <span class="dim small">{{ zhCN.hero.wisdom }}</span>
            <input v-model.number="pointForm[h.hid].wisdom" type="number" class="num" />
            <button class="ghost" :disabled="busy" @click="onAddPoint(h)">加点</button>
            <button class="ghost" :disabled="busy" @click="emit('clear-point', h.hid)">洗点</button>
          </div>

          <!-- 历练进行中/待结算：倒计时、取消、加速 -->
          <div v-if="h.expr" class="row">
            <span class="dim small">
              {{ zhCN.hero.expr }} {{ h.expr.hours }}h · {{ formatLeft(h.expr.time_left) }} ·
              {{ zhCN.hero.exprCarry }} {{ h.expr.carrymoney }}
            </span>
            <div class="btns">
              <button class="ghost" :disabled="busy" @click="emit('faster-expr', h.hid)">
                {{ zhCN.hero.faster }}
              </button>
              <button v-if="h.expr.state === 0" class="ghost" :disabled="busy" @click="emit('cancel-expr', h.hid)">
                {{ zhCN.hero.cancelExpr }}
              </button>
            </div>
          </div>

          <!-- 空闲：升级 + 开始历练 -->
          <div v-else class="row form">
            <div class="expr">
              <select v-if="inCity(h)" v-model.number="exprForm[h.hid].exprType" class="num sel">
                <option v-for="t in exprTypes" :key="t.type" :value="t.type">
                  {{ t.name }}（{{ t.hour_money }}{{ zhCN.hero.exprHourMoney }}）
                </option>
              </select>
              <span class="dim small">{{ zhCN.hero.exprTime }}</span>
              <input
                v-model.number="exprForm[h.hid].hours"
                type="number"
                :min="firstType.min_hour"
                :max="firstType.max_hour"
                class="num"
              />
              <span class="dim small">{{ zhCN.hero.exprCarry }}</span>
              <input v-model.number="exprForm[h.hid].carrymoney" type="number" min="0" class="num" />
            </div>
            <div class="btns">
              <button
                class="primary"
                :disabled="busy || !h.can_upgrade"
                :title="h.no_upgrade_msg"
                @click="emit('upgrade', h.hid)"
              >
                {{ zhCN.hero.upgrade }}
              </button>
              <button class="ghost" :disabled="busy || h.state !== 0" @click="onStartExpr(h)">
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
  max-width: 620px;
  max-height: 86vh;
  padding: 16px;
  overflow-y: auto;
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

.row.form {
  justify-content: flex-start;
  flex-wrap: wrap;
}

.face {
  flex: 0 0 auto;
  width: 48px;
  height: 60px;
  object-fit: cover;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.head-main {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
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
  flex-wrap: wrap;
}

.num {
  width: 64px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.sel {
  width: auto;
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

.hint.warn {
  color: var(--accent);
}
</style>
