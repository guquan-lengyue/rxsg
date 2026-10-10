import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as pkApi from '@/api/pk'
import type { PkBattle, PkCampaign, PkHeroPage, PkMax, PkPrize, PkRankRow } from '@/api/pk'

// 单机 PK 征战 store：关卡数据 / 进度最大值 / 首通榜 / 玩家将领分页 / 战斗结果。
export const usePkStore = defineStore('pk', () => {
  const campaign = ref<PkCampaign | null>(null)
  const max = ref<PkMax | null>(null)
  const rank = ref<PkRankRow[]>([])
  const heroPage = ref<PkHeroPage | null>(null)
  const battle = ref<PkBattle | null>(null)
  const firstRewards = ref<PkPrize[]>([])

  async function loadCampaign(): Promise<PkCampaign> {
    campaign.value = await pkApi.loadCampaign()
    return campaign.value
  }

  async function refreshMax(): Promise<PkMax> {
    max.value = await pkApi.loadCampaignMax()
    return max.value
  }

  async function loadRank(battleId: number, type: number): Promise<PkRankRow[]> {
    rank.value = await pkApi.loadRewardRank(battleId, type)
    return rank.value
  }

  async function loadHeroes(page: number, hids: number[]): Promise<PkHeroPage> {
    heroPage.value = await pkApi.loadUserHeroes(page, hids)
    return heroPage.value
  }

  async function claimFirst(battleId: number, flag: number, rankId: number): Promise<PkPrize[]> {
    firstRewards.value = await pkApi.claimFirstReward(battleId, flag, rankId)
    return firstRewards.value
  }

  async function fight(battleId: number, flag: number, level: number, hids: number[]): Promise<PkBattle> {
    battle.value = await pkApi.startBattle(battleId, flag, level, hids)
    return battle.value
  }

  function clearBattle(): void {
    battle.value = null
    firstRewards.value = []
  }

  async function buy(type: number, count: number): Promise<void> {
    await pkApi.buyJunling(type, count)
    await refreshMax()
  }

  function reset(): void {
    campaign.value = null
    max.value = null
    rank.value = []
    heroPage.value = null
    battle.value = null
    firstRewards.value = []
  }

  return {
    campaign,
    max,
    rank,
    heroPage,
    battle,
    firstRewards,
    loadCampaign,
    refreshMax,
    loadRank,
    loadHeroes,
    claimFirst,
    fight,
    clearBattle,
    buy,
    reset,
  }
})
