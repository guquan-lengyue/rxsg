<script setup lang="ts">
// 官署面板：官署等级 / 空位 / 爵位 + 城内将领列表 + 任命（城守/主将/军师）。
// 对齐 legacy Office/OfficeHeroPanel.as（将领表）+ Office/BigSetChiefDialog.as（任命下拉与提交）。
// 面板只负责展示与发事件（props in / events out），写操作由 CityView 执行并回灌 notice/error。
// 文案取自原版 LocaleChinese（Office_OfficeHeroPanel_* / Office_SetChiefDialog_* / _Define_19..21）。
import { computed, ref, watch } from 'vue'

import BuildingOps from '@/components/BuildingOps.vue'
import SkinDialog from '@/components/SkinDialog.vue'
import type { BuildingDetail, HeroState, OfficeInfo } from '@/types'

const props = defineProps<{
  info: OfficeInfo | null
  detail: BuildingDetail | null
  loading: boolean
  error: string
  notice: string
  busy: boolean
}>()

const emit = defineEmits<{
  (e: 'set-office', sets: [number, number][]): void
  (e: 'upgrade'): void
  (e: 'stop'): void
  (e: 'destroy'): void
  (e: 'destroy-all'): void
  (e: 'cancel-destroy'): void
  (e: 'close'): void
}>()

// 任命类型（对齐 Define.CHIEF_TYPE_NEW：0/1/7/8；1 城守 / 7 主将 / 8 军师）。
const officeOptions: { value: number; label: string }[] = [
  { value: 0, label: '----' },
  { value: 1, label: '城守' },
  { value: 7, label: '主将' },
  { value: 8, label: '军师' },
]

// 状态列文案（对齐 BloodWar.HERO_STATE_ARRAY 的第 0/1/7/8 项）。
const stateLabels: Record<number, string> = { 0: '空闲', 1: '城守', 7: '主将', 8: '军师' }

const heroes = computed<HeroState[]>(() => props.info?.heroes ?? [])

// 每将的“任命类型”下拉当前值；随 info 变化重置为各将的当前 state。
const selections = ref<Record<number, number>>({})
watch(
  () => props.info,
  (info) => {
    const next: Record<number, number> = {}
    for (const h of info?.heroes ?? []) {
      next[h.hid] = h.state
    }
    selections.value = next
  },
  { immediate: true },
)

function nameOf(hid: number): string {
  if (hid <= 0) {
    return '----'
  }
  return heroes.value.find((h) => h.hid === hid)?.name ?? '----'
}

const chiefName = computed(() => nameOf(props.info?.chief_hero_id ?? 0))
const generalName = computed(() => nameOf(props.info?.general_hero_id ?? 0))
const counsellorName = computed(() => nameOf(props.info?.counsellor_hero_id ?? 0))

// 派生属性（对齐 OfficeHeroPanel 的 getCommand/getBravery/getWisdom/getAffairs；
// buf 加成依赖未迁移的 buff 数据，省略，其余按 base+add+add_on 求和）。
// 注：原版将领表的「体力/精力」两列依赖重写版 HeroState 未提供的字段，此处不渲染。
function commandOf(h: HeroState): number {
  return h.level + h.command_base + h.command_add_on
}
function braveryOf(h: HeroState): number {
  return h.bravery_base + h.bravery_add + h.bravery_add_on
}
function wisdomOf(h: HeroState): number {
  return h.wisdom_base + h.wisdom_add + h.wisdom_add_on
}
function affairsOf(h: HeroState): number {
  return h.affairs_base + h.affairs_add + h.affairs_add_on
}

// 仅提交发生变化的 [hid, 新职位]（对齐 BigSetChiefDialog.onSubmit）。
const pendingSets = computed<[number, number][]>(() => {
  const out: [number, number][] = []
  for (const h of heroes.value) {
    const next = selections.value[h.hid] ?? h.state
    if (next !== h.state) {
      out.push([h.hid, next])
    }
  }
  return out
})

function submit(): void {
  if (pendingSets.value.length === 0) {
    return
  }
  emit('set-office', pendingSets.value)
}
</script>

<template>
  <SkinDialog title="官署" wide @close="emit('close')">
    <p v-if="loading" class="u-hint">加载中…</p>
    <p v-else-if="error" class="u-hint is-error">{{ error }}</p>
    <p v-else-if="notice" class="u-hint is-notice">{{ notice }}</p>

    <!-- 建筑操作（升级 / 拆除），与通用建筑面板单一实现 -->
    <BuildingOps
      v-if="detail"
      :detail="detail"
      :busy="busy"
      @upgrade="emit('upgrade')"
      @stop="emit('stop')"
      @destroy="emit('destroy')"
      @destroy-all="emit('destroy-all')"
      @cancel-destroy="emit('cancel-destroy')"
    />

    <template v-if="info">
      <div class="u-row">
        <span class="label">官署等级</span>
        <span>Lv{{ info.office_level }}</span>
        <span class="label">官署空位</span>
        <span>{{ info.valid_position }}</span>
        <span class="label">爵位</span>
        <span>{{ info.nobility }}</span>
      </div>
      <div class="u-row">
        <span class="label">城守</span>
        <span>{{ chiefName }}</span>
        <span class="label">主将</span>
        <span>{{ generalName }}</span>
        <span class="label">军师</span>
        <span>{{ counsellorName }}</span>
      </div>
    </template>

    <p v-if="info && !heroes.length" class="u-hint">城中暂无武将</p>

    <table v-if="heroes.length" class="u-table u-table--lined office-table">
      <thead>
        <tr>
          <th>将领</th>
          <th>等级</th>
          <th>统率</th>
          <th>内政</th>
          <th>勇武</th>
          <th>智谋</th>
          <th>忠诚</th>
          <th>状态</th>
          <th>任命类型</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="h in heroes" :key="h.hid">
          <td>{{ h.name }}</td>
          <td>{{ h.level }}</td>
          <td>{{ commandOf(h) }}</td>
          <td>{{ affairsOf(h) }}</td>
          <td>{{ braveryOf(h) }}</td>
          <td>{{ wisdomOf(h) }}</td>
          <td>{{ h.loyalty }}</td>
          <td>{{ stateLabels[h.state] ?? '未知' }}</td>
          <td>
            <select v-model.number="selections[h.hid]" class="u-select pick">
              <option v-for="o in officeOptions" :key="o.value" :value="o.value">{{ o.label }}</option>
            </select>
          </td>
        </tr>
      </tbody>
    </table>

    <template #footer>
      <span class="u-hint">仅提交发生变化的任命</span>
      <button
        class="u-btn u-btn--confirm"
        type="button"
        :disabled="busy || pendingSets.length === 0"
        @click="submit"
      >
        任命
      </button>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.u-hint.is-notice {
  color: var(--accent);
}

.office-table {
  margin-top: 8px;
}

.pick {
  padding: 2px 4px;
  color: var(--text);
}
</style>
