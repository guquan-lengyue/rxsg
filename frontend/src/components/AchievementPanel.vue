<script setup lang="ts">
import { computed, ref } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { img } from '@/assets/img'
import { ACHIEVEMENT_PAGE_SIZE } from '@/api/achievement'
import type {
  Achievement,
  AchievementDetail,
  AchievementFilter,
  AchievementOverview,
} from '@/api/achievement'

const props = defineProps<{
  overview: AchievementOverview | null
  list: Achievement[]
  count: number
  detail: AchievementDetail | null
  activeGroup: number
  filter: AchievementFilter
  page: number
  loading: boolean
  error: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'select-group', group: number): void
  (e: 'select-filter', filter: AchievementFilter): void
  (e: 'select-page', page: number): void
  (e: 'open-detail', aid: number): void
  (e: 'back'): void
  (e: 'close'): void
}>()

const FILTERS: { value: AchievementFilter; label: string }[] = [
  { value: 0, label: '全部' },
  { value: 1, label: '已完成' },
  { value: 2, label: '未完成' },
]

const failed = ref<Record<number, boolean>>({})

const groups = computed(() => props.overview?.groups ?? [])
const recent = computed(() => props.overview?.recent ?? [])
const totalPages = computed(() => Math.max(1, Math.ceil(props.count / ACHIEVEMENT_PAGE_SIZE)))
const showDetail = computed(() => props.detail !== null)

function iconUrl(id: number, image: string): string {
  return img(`achievement/${image || id}.png`)
}

function onImgError(id: number): void {
  failed.value = { ...failed.value, [id]: true }
}

function progressPercent(): number {
  const p = props.detail?.progress?.[0]
  if (!p || !p.targetValue) {
    return 0
  }
  return Math.min(100, Math.round((p.userValue / p.targetValue) * 100))
}
</script>

<template>
  <SkinDialog title="成就" wide @close="emit('close')">
    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>

    <template v-else>
      <section class="head">
        <div class="point">
          <span class="label">成就点数</span>
          <strong>{{ overview?.point ?? 0 }}</strong>
        </div>
        <ul v-if="recent.length" class="recent">
          <li v-for="r in recent" :key="r.id">
            <span class="rname">{{ r.name }}</span>
            <span class="rtime">{{ r.achieveGetTime }}</span>
          </li>
        </ul>
      </section>

      <div class="u-tabs">
        <button
          v-for="g in groups"
          :key="g.group"
          class="u-tab"
          :class="{ 'is-active': g.group === activeGroup }"
          type="button"
          @click="emit('select-group', g.group)"
        >
          {{ g.group_name }}（{{ g.finish_count }}/{{ g.total_count }}）
        </button>
      </div>

      <div class="u-tabs">
        <button
          v-for="f in FILTERS"
          :key="f.value"
          class="u-tab"
          :class="{ 'is-active': f.value === filter }"
          type="button"
          @click="emit('select-filter', f.value)"
        >
          {{ f.label }}
        </button>
      </div>

      <!-- 成就详情 -->
      <section v-if="showDetail && detail" class="detail">
        <header class="d-head">
          <div class="d-title">
            <img
              v-if="detail.info && !failed[detail.info.id]"
              class="d-icon"
              :src="iconUrl(detail.info.id, detail.info.image)"
              :alt="detail.info.name"
              @error="onImgError(detail.info.id)"
            />
            <strong>{{ detail.info?.name || '成就详情' }}</strong>
          </div>
          <button class="u-btn u-btn--cancel" type="button" @click="emit('back')">返回</button>
        </header>

        <p v-if="detail.info?.content" class="u-hint">{{ detail.info.content }}</p>
        <p v-if="detail.info?.todo" class="u-hint">达成条件：{{ detail.info.todo }}</p>

        <!-- type=2：数值型进度 -->
        <div v-if="detail.info?.type === 2 && detail.progress.length" class="prog">
          <div class="bar">
            <div class="fill" :style="{ width: progressPercent() + '%' }"></div>
          </div>
          <span class="prog-text">
            {{ detail.progress[0].userValue }} / {{ detail.progress[0].targetValue }}
          </span>
        </div>

        <!-- type=3：目标型清单 -->
        <ul v-else-if="detail.info?.type === 3" class="goals">
          <li v-for="(p, idx) in detail.progress" :key="idx" :class="{ done: p.isDone }">
            <span class="mark">{{ p.isDone ? '✔' : '✘' }}</span>
            <span>{{ p.content }}</span>
          </li>
          <li v-if="!detail.progress.length" class="u-hint">暂无目标</li>
        </ul>

        <p class="u-hint">全服已有 {{ detail.finishCount }} 人达成</p>
      </section>

      <!-- 成就列表 -->
      <ul v-else class="list">
        <li v-for="a in list" :key="a.id" class="item" @click="emit('open-detail', a.id)">
          <img
            v-if="!failed[a.id]"
            class="icon"
            :src="iconUrl(a.id, a.image)"
            :alt="a.name"
            @error="onImgError(a.id)"
          />
          <div class="info">
            <div class="row">
              <span class="name">{{ a.name }}</span>
              <span class="pt">{{ a.point }}点</span>
            </div>
            <p class="content">{{ a.content }}</p>
            <span class="time">{{ a.achieveGetTime }}</span>
          </div>
        </li>
        <li v-if="!list.length" class="u-hint">该分组暂无成就</li>
      </ul>

      <div v-if="!showDetail && totalPages > 1" class="pager">
        <button
          class="u-btn u-btn--green"
          type="button"
          :disabled="page <= 0"
          @click="emit('select-page', page - 1)"
        >
          上一页
        </button>
        <span class="page">{{ page + 1 }} / {{ totalPages }}</span>
        <button
          class="u-btn u-btn--green"
          type="button"
          :disabled="page >= totalPages - 1"
          @click="emit('select-page', page + 1)"
        >
          下一页
        </button>
      </div>
    </template>
  </SkinDialog>
</template>

<style scoped>
.head {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 8px;
}

.point {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.point .label {
  color: var(--text-dim);
  font-size: 13px;
}

.point strong {
  color: var(--accent);
  font-size: 20px;
}

.recent {
  display: flex;
  gap: 16px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 12px;
  color: var(--text-dim);
}

.rname {
  color: var(--text);
}

.list {
  margin: 8px 0 0;
  padding: 0;
  list-style: none;
}

.item {
  display: flex;
  gap: 10px;
  padding: 8px 10px;
  margin-bottom: 6px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  cursor: pointer;
}

.icon {
  width: 48px;
  height: 48px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
  background: var(--panel);
}

.info {
  flex: 1;
  min-width: 0;
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.name {
  color: var(--text);
  font-size: 14px;
}

.pt {
  color: var(--accent);
  font-size: 12px;
}

.content {
  margin: 2px 0;
  color: var(--text-dim);
  font-size: 12px;
}

.time {
  color: var(--text-dim);
  font-size: 11px;
}

.detail {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-top: 8px;
}

.d-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.d-title {
  display: flex;
  align-items: center;
  gap: 8px;
}

.d-icon {
  width: 40px;
  height: 40px;
  object-fit: contain;
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.prog {
  display: flex;
  align-items: center;
  gap: 8px;
}

.bar {
  flex: 1;
  height: 12px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
  overflow: hidden;
}

.fill {
  height: 100%;
  background: var(--accent);
}

.prog-text {
  color: var(--accent);
  font-size: 12px;
}

.goals {
  margin: 0;
  padding: 0;
  list-style: none;
}

.goals li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 3px 0;
  font-size: 13px;
  color: var(--text-dim);
}

.goals li.done {
  color: var(--text);
}

.mark {
  color: var(--danger);
}

.goals li.done .mark {
  color: #6fbf73;
}

.pager {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-top: 8px;
}

.page {
  color: var(--text-dim);
  font-size: 13px;
}
</style>
