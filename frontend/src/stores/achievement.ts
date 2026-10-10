import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as achievementApi from '@/api/achievement'
import type { Achievement, AchievementDetail, AchievementFilter, AchievementOverview } from '@/api/achievement'

// 成就 store：总览 + 分组列表（分页/筛选）+ 成就详情。
export const useAchievementStore = defineStore('achievement', () => {
  const overview = ref<AchievementOverview | null>(null)
  const list = ref<Achievement[]>([])
  const count = ref(0)
  const detail = ref<AchievementDetail | null>(null)
  const activeGroup = ref(0)
  const activeSubgroup = ref(0)
  const filter = ref<AchievementFilter>(0)
  const page = ref(0)

  function reset(): void {
    overview.value = null
    list.value = []
    count.value = 0
    detail.value = null
    activeGroup.value = 0
    activeSubgroup.value = 0
    filter.value = 0
    page.value = 0
  }

  async function loadOverview(): Promise<AchievementOverview> {
    overview.value = await achievementApi.getAchievementOverview()
    return overview.value
  }

  async function loadList(
    group: number,
    subgroup: number,
    type: AchievementFilter,
    pageNo: number,
  ): Promise<void> {
    const r = await achievementApi.getAchievementsByGroup(group, subgroup, type, pageNo)
    list.value = r.rows
    count.value = r.count
    activeGroup.value = group
    activeSubgroup.value = subgroup
    filter.value = type
    page.value = pageNo
    detail.value = null
  }

  async function loadDetail(aid: number): Promise<void> {
    detail.value = await achievementApi.getAchievementDetail(aid)
  }

  function clearDetail(): void {
    detail.value = null
  }

  return {
    overview,
    list,
    count,
    detail,
    activeGroup,
    activeSubgroup,
    filter,
    page,
    reset,
    loadOverview,
    loadList,
    loadDetail,
    clearDetail,
  }
})
