import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as battleApi from '@/api/battle'
import type { BattleBrief, BattleDetail, BattleReportRow } from '@/api/battle'

// M7 战斗 store：战斗列表 / 战斗详情（含逐回合原始战报）/ 原版 HTML 战报。
export const useBattleStore = defineStore('battle', () => {
  const list = ref<BattleBrief[]>([])
  const detail = ref<BattleDetail | null>(null)
  const reports = ref<BattleReportRow[]>([])
  const selectedId = ref(0)

  async function loadList(cid: number): Promise<BattleBrief[]> {
    list.value = await battleApi.listBattles(cid)
    return list.value
  }

  async function loadDetail(bid: number): Promise<BattleDetail> {
    detail.value = await battleApi.getBattleDetail(bid)
    selectedId.value = bid
    return detail.value
  }

  async function loadReports(cid: number): Promise<BattleReportRow[]> {
    reports.value = await battleApi.listBattleReports(cid)
    return reports.value
  }

  function clearDetail(): void {
    detail.value = null
    selectedId.value = 0
  }

  return {
    list,
    detail,
    reports,
    selectedId,
    loadList,
    loadDetail,
    loadReports,
    clearDetail,
  }
})
