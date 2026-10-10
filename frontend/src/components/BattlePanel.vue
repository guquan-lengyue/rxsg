<script setup lang="ts">
import { computed, ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { parseRounds } from '@/api/battle'
import type { BattleBrief, BattleDetail, BattleReportRow } from '@/api/battle'
import { formatLeft } from '@/render/cityGrid'

const props = defineProps<{
  battles: BattleBrief[]
  detail: BattleDetail | null
  reports: BattleReportRow[]
  selectedId: number
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'select', bid: number): void
  (e: 'load-reports'): void
  (e: 'close'): void
}>()

type Tab = 'list' | 'detail' | 'reports'
const tab = ref<Tab>('list')

function switchTab(t: Tab): void {
  tab.value = t
  if (t === 'reports') {
    emit('load-reports')
  }
}

function resultText(r: number): string {
  return ['胜利', '失败', '平局'][r] ?? '进行中'
}

function stateText(s: number): string {
  return s === 0 ? '进行中' : '已结束'
}

function selectBattle(b: BattleBrief): void {
  tab.value = 'detail'
  emit('select', b.id)
}

// 逐回合原始战报解析（17 字段）。
const roundGroups = computed(() => {
  const d = props.detail
  if (!d) {
    return []
  }
  return d.rounds.map((r) => ({ round: r.round, rows: parseRounds(r.report) }))
})

function sideText(isAttack: number): string {
  return isAttack === 1 ? '攻' : '守'
}

function fmtTime(ts: number): string {
  if (!ts) {
    return '—'
  }
  return new Date(ts * 1000).toLocaleString()
}
</script>

<template>
  <SkinDialog title="战斗战报" wide @close="emit('close')">
    <div class="u-tabs">
      <button class="u-tab" :class="{ 'is-active': tab === 'list' }" type="button" @click="tab = 'list'">
        战斗列表
      </button>
      <button
        class="u-tab"
        :class="{ 'is-active': tab === 'detail' }"
        type="button"
        :disabled="!detail"
        @click="tab = 'detail'"
      >
        逐回合
      </button>
      <button
        class="u-tab"
        :class="{ 'is-active': tab === 'reports' }"
        type="button"
        @click="switchTab('reports')"
      >
        战报
      </button>
    </div>

    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <!-- 战斗列表 -->
    <template v-if="tab === 'list'">
      <p v-if="!battles.length" class="u-hint">暂无战斗记录</p>
      <table v-else class="bt-table">
        <thead>
          <tr>
            <th>ID</th>
            <th>类型</th>
            <th>状态</th>
            <th>结果</th>
            <th>回合</th>
            <th>剩余</th>
            <th>攻方城</th>
            <th>守方城</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="b in battles"
            :key="b.id"
            class="row"
            :class="{ picked: b.id === selectedId }"
            @click="selectBattle(b)"
          >
            <td>{{ b.id }}</td>
            <td>{{ b.type }}</td>
            <td>{{ stateText(b.state) }}</td>
            <td>{{ resultText(b.result) }}</td>
            <td>{{ b.round }}</td>
            <td>{{ b.time_left > 0 ? formatLeft(b.time_left) : '—' }}</td>
            <td>{{ b.attack_cid }}</td>
            <td>{{ b.resist_cid }}</td>
          </tr>
        </tbody>
      </table>
      <p class="u-hint">点击一行查看逐回合战报</p>
    </template>

    <!-- 逐回合 -->
    <template v-else-if="tab === 'detail'">
      <template v-if="detail">
        <div class="u-row">
          <span class="label">战斗 #{{ detail.battle.id }}</span>
          <span>结果 {{ resultText(detail.battle.result) }}</span>
          <span>回合 {{ detail.battle.round }}</span>
          <span v-if="detail.battle.time_left > 0">剩余 {{ formatLeft(detail.battle.time_left) }}</span>
        </div>
        <p v-if="!roundGroups.length" class="u-hint">暂无逐回合记录</p>
        <template v-else>
          <div v-for="g in roundGroups" :key="g.round" class="round">
            <h4 class="block-title">第 {{ g.round }} 回合</h4>
            <p v-if="!g.rows.length" class="u-hint">本回合无明细</p>
            <table v-else class="bt-table">
              <thead>
                <tr>
                  <th>方</th>
                  <th>兵种</th>
                  <th>伤害</th>
                  <th>目标兵种</th>
                  <th>目标损失</th>
                  <th>反击伤害</th>
                  <th>反击损失</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="(r, i) in g.rows" :key="i">
                  <td>{{ sideText(r.isAttack) }}</td>
                  <td>{{ r.sid }}</td>
                  <td>{{ r.damage }}</td>
                  <td>{{ r.targetType }} / {{ r.targetSid }}</td>
                  <td>{{ r.dead }}（{{ r.targetStart }}→{{ r.targetEnd }}）</td>
                  <td>{{ r.counterDamage }}</td>
                  <td>{{ r.counterDead }}（{{ r.targetStartD }}→{{ r.counterEnd }}）</td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </template>
      <p v-else class="u-hint">请先在列表中选一场战斗</p>
    </template>

    <!-- 原版战报 -->
    <template v-else>
      <p v-if="!reports.length" class="u-hint">暂无战报</p>
      <ul v-else class="report-list">
        <li v-for="r in reports" :key="r.id" class="report">
          <div class="u-row report-head">
            <strong>战报 #{{ r.id }}</strong>
            <span class="dim">战斗 {{ r.battleid }}</span>
            <span class="dim">{{ fmtTime(r.time) }}</span>
          </div>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <div class="report-body" v-html="r.content"></div>
        </li>
      </ul>
    </template>

    <template #footer>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.u-tabs {
  margin-bottom: 8px;
}

.bt-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12px;
}

.bt-table th,
.bt-table td {
  padding: 4px 6px;
  text-align: left;
  border: 1px solid var(--panel-border);
}

.bt-table th {
  color: var(--text-dim);
  font-weight: normal;
  background: var(--bg);
}

.row {
  cursor: pointer;
}

.row:hover {
  background: var(--bg);
}

.row.picked {
  color: var(--accent);
}

.round {
  margin-top: 10px;
}

.block-title {
  margin: 0 0 4px;
  color: var(--accent);
  font-size: 13px;
  font-weight: normal;
}

.report-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.report {
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.report-head {
  border-bottom: 1px solid var(--panel-border);
  padding-bottom: 4px;
  margin-bottom: 6px;
}

.report-body {
  color: var(--text);
  font-size: 13px;
  line-height: 1.6;
  word-break: break-word;
}

.dim {
  color: var(--text-dim);
}
</style>
