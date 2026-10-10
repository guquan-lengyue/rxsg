<script setup lang="ts">
import { computed, ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { armor, itemIcon } from '@/assets/img'
import type { Task, TaskDetail, TaskGroup, TaskReward } from '@/api/task'

const props = defineProps<{
  groups: TaskGroup[]
  tasks: Task[]
  detail: TaskDetail | null
  activeType: number
  activeGroup: number
  detailTid: number
  sysCount: number
  nobilityOk: boolean
  doneCount: number
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'select-type', type: number): void
  (e: 'select-group', group: number): void
  (e: 'open-detail', tid: number): void
  (e: 'back'): void
  (e: 'claim', tid: number, selectId: number, selectId2: number): void
  (e: 'drop', group: number): void
  (e: 'sys-drop', tid: number): void
  (e: 'close'): void
}>()

// 任务组类型分页（legacy cfg_task_group.type；类型枚举见 migrations 0012 注释）。
const TASK_TYPES: { type: number; label: string }[] = [
  { type: 0, label: '新手任务' },
  { type: 1, label: '日常任务' },
  { type: 2, label: '成长任务' },
  { type: 3, label: '支线任务' },
  { type: 5, label: '活动任务' },
  { type: 6, label: '其他任务' },
  { type: 7, label: '系统任务' },
]

const RESOURCE_NAMES: Record<number, string> = {
  1: '黄金',
  2: '粮食',
  3: '木材',
  4: '石料',
  5: '铁锭',
  6: '人口',
  7: '民心',
  8: '民怨',
  9: '声望',
  17: '官职',
  18: '爵位',
  19: '礼金',
  20: '铜钱',
  22: '元宝',
  30: '荣誉',
}

const showDetail = computed(() => props.detail !== null)
const allGoalsDone = computed(() => {
  const goals = props.detail?.goals ?? []
  return goals.length > 0 && goals.every((g) => g.state)
})
const goalProgress = computed(() => {
  const goals = props.detail?.goals ?? []
  return `${goals.filter((g) => g.state).length}/${goals.length}`
})

function resourceName(type: number): string {
  return RESOURCE_NAMES[type] ?? `资源${type}`
}

// 奖励项无 name 字段，按 sort/type 拼可读文案（对齐 reward.go giveReward 的类别语义）。
function rewardText(r: TaskReward): string {
  switch (r.sort) {
    case 1:
      return `${resourceName(r.type)}×${r.count}`
    case 2:
      return r.type === 0 ? `礼金×${r.count}` : `道具${r.type}×${r.count}`
    case 3:
      return `兵力${r.type}×${r.count}`
    case 4:
      return `城防${r.type}×${r.count}`
    case 5:
      return `任务物品${r.type}×${r.count}`
    case 6:
      return `装备${r.type}×${r.count}`
    case 10:
      return `开启任务${r.type}`
    case 11:
      return `开启任务组${r.type}`
    default:
      return `奖励(${r.sort})×${r.count}`
  }
}

function onClaim(tid: number): void {
  emit('claim', tid, 0, 0)
}

// 奖励图标（仅视觉）：仅对有可靠映射的类别出图，其余保持纯文字。
//   sort=2 道具：type 即 gid（backend reward.go GiveReward case 2 → giveGoods）→ item_{gid}.png；type=0 为礼金 → item_0.png。
//   sort=6 装备：type 即 armorid（GiveReward case 6 → giveArmor）→ armor/{id}.png。
// 资源、兵力、城防、任务物品等无对应图标资源，返回空串不加图标。
function rewardIcon(r: TaskReward): string {
  if (r.sort === 2) {
    return itemIcon(r.type)
  }
  if (r.sort === 6 && r.type > 0) {
    return armor(r.type)
  }
  return ''
}

// 图标加载失败时直接隐藏（原版风格：缺图不报错、不留破图）。
const failedIcons = ref<Record<number, boolean>>({})
function onRewardIconError(id: number): void {
  failedIcons.value = { ...failedIcons.value, [id]: true }
}
</script>

<template>
  <SkinDialog title="任务" wide @close="emit('close')">
    <div class="u-tabs">
      <button
        v-for="t in TASK_TYPES"
        :key="t.type"
        class="u-tab"
        :class="{ 'is-active': t.type === activeType }"
        type="button"
        @click="emit('select-type', t.type)"
      >
        {{ t.label }}
      </button>
    </div>

    <p v-if="activeType === 7" class="u-hint">今日可领取系统任务：{{ sysCount }}</p>
    <p v-else-if="activeType === 1 && !nobilityOk" class="u-hint">爵位未达到公士，部分日常任务不可用。</p>

    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <template v-else>
      <!-- 任务组列表 -->
      <div class="groups">
        <div
          v-for="g in groups"
          :key="g.id"
          class="group"
          :class="{ 'is-active': g.id === activeGroup }"
          @click="emit('select-group', g.id)"
        >
          <span class="gname">{{ g.name }}</span>
          <span v-if="g.count > 0" class="gcount">{{ g.count }}</span>
          <button
            v-if="activeType !== 7"
            class="u-btn u-btn--red drop"
            type="button"
            :disabled="busy"
            @click.stop="emit('drop', g.id)"
          >
            放弃
          </button>
        </div>
        <p v-if="!groups.length" class="u-hint">暂无任务组</p>
      </div>

      <!-- 任务详情（优先展示） -->
      <section v-if="showDetail && detail" class="detail">
        <header class="d-head">
          <strong>{{ detail.task?.name || '任务详情' }}</strong>
          <button class="u-btn u-btn--cancel" type="button" @click="emit('back')">返回</button>
        </header>
        <p v-if="detail.task?.todo" class="u-hint">{{ detail.task.todo }}</p>

        <div class="d-block">
          <span class="d-label">目标（{{ goalProgress }}）</span>
          <ul class="goals">
            <li v-for="goal in detail.goals" :key="goal.id" :class="{ done: goal.state }">
              <span class="mark">{{ goal.state ? '✔' : '✘' }}</span>
              <span class="content">{{ goal.content || `目标${goal.id}` }}</span>
              <span v-if="goal.sort === 50 || goal.sort === 80" class="prog">
                {{ goal.currentcount }}/{{ goal.count }}
              </span>
            </li>
            <li v-if="!detail.goals.length" class="u-hint">暂无目标</li>
          </ul>
        </div>

        <div class="d-block">
          <span class="d-label">奖励</span>
          <ul class="rewards">
            <li v-for="r in detail.rewards" :key="r.id">
              <img
                v-if="rewardIcon(r) && !failedIcons[r.id]"
                class="r-icon"
                :src="rewardIcon(r)"
                alt=""
                @error="onRewardIconError(r.id)"
              />
              <span>{{ rewardText(r) }}</span>
            </li>
            <li v-if="!detail.rewards.length" class="u-hint">无奖励</li>
          </ul>
        </div>

        <div class="d-actions">
          <button
            class="u-btn u-btn--gold"
            type="button"
            :disabled="busy || !allGoalsDone"
            @click="onClaim(detailTid)"
          >
            领取奖励
          </button>
        </div>
      </section>

      <!-- 任务列表 -->
      <ul v-else class="tasks">
        <li
          v-for="task in tasks"
          :key="task.id"
          class="task"
          :class="{ complete: task.state }"
          @click="emit('open-detail', task.id)"
        >
          <div class="t-main">
            <span class="t-name">{{ task.name }}</span>
            <span v-if="task.state" class="t-tag">可领奖</span>
          </div>
          <p v-if="task.todo" class="t-todo">{{ task.todo }}</p>
          <button
            v-if="activeType === 7"
            class="u-btn u-btn--red drop"
            type="button"
            :disabled="busy"
            @click.stop="emit('sys-drop', task.id)"
          >
            放弃
          </button>
        </li>
        <li v-if="!tasks.length" class="u-hint">该任务组暂无进行中的任务</li>
      </ul>

      <p v-if="activeGroup && activeType !== 7" class="u-hint">
        本组可领奖：{{ doneCount }}
      </p>
    </template>
  </SkinDialog>
</template>

<style scoped>
.groups {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 8px 0;
}

.group {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 8px;
  font-size: 13px;
  color: var(--text-dim);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  cursor: pointer;
}

.group.is-active {
  color: var(--accent);
  border-color: var(--accent);
}

.gcount {
  min-width: 16px;
  padding: 0 4px;
  color: #10202e;
  font-size: 11px;
  text-align: center;
  background: var(--accent);
  border-radius: 8px;
}

.drop {
  min-width: 48px;
  height: 22px;
  padding: 0 6px;
  font-size: 12px;
}

.tasks,
.goals,
.rewards {
  margin: 0;
  padding: 0;
  list-style: none;
}

.task {
  position: relative;
  padding: 8px 10px;
  margin-bottom: 6px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  cursor: pointer;
}

.task.complete {
  border-color: var(--accent);
}

.t-main {
  display: flex;
  align-items: center;
  gap: 8px;
}

.t-name {
  color: var(--text);
  font-size: 14px;
}

.t-tag {
  padding: 0 6px;
  color: #10202e;
  font-size: 11px;
  background: var(--accent);
  border-radius: 3px;
}

.t-todo {
  margin: 4px 0 0;
  color: var(--text-dim);
  font-size: 12px;
}

.detail {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.d-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.d-block {
  padding: 8px 10px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.d-label {
  display: block;
  margin-bottom: 6px;
  color: var(--accent);
  font-size: 13px;
}

.goals li,
.rewards li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 2px 0;
  font-size: 13px;
  color: var(--text-dim);
}

.r-icon {
  width: 20px;
  height: 20px;
  object-fit: contain;
}

.goals li.done .content {
  color: var(--text);
}

.mark {
  color: var(--danger);
}

.goals li.done .mark {
  color: #6fbf73;
}

.prog {
  margin-left: auto;
  color: var(--accent);
}

.d-actions {
  display: flex;
  justify-content: flex-end;
}
</style>
