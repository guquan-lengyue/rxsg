import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as lotteryApi from '@/api/lottery'
import type { LotteryBoard, LotteryItem, LotteryWin } from '@/api/lottery'

// 抽奖 store：状态（今日次数/元宝）+ 8 格盘面 + 中奖档位 + 重开次数。
export const useLotteryStore = defineStore('lottery', () => {
  const todayCount = ref(0)
  const moneyOk = ref(false)
  const board = ref<LotteryBoard>([])
  const winInfo = ref<LotteryWin>({ type: -1, id: 0, count: 0 })
  const restartCount = ref(0)
  const lastWin = ref<LotteryItem | null>(null)

  function applyRound(round: lotteryApi.LotteryRound): void {
    board.value = round.board
    winInfo.value = round.win
    restartCount.value = round.restartCount
    todayCount.value = round.todayCount
  }

  async function loadStatus(): Promise<void> {
    const [count, ok] = await Promise.all([lotteryApi.getTodayCount(), lotteryApi.checkMoney()])
    todayCount.value = count
    moneyOk.value = ok
  }

  async function startDraw(): Promise<void> {
    const r = await lotteryApi.startLottery()
    winInfo.value = r.win
    todayCount.value = r.todayCount
  }

  // addCount：领取当前盘面奖励（返回 winObj）。
  async function claimWin(): Promise<LotteryItem | null> {
    lastWin.value = await lotteryApi.getWin()
    return lastWin.value
  }

  // getLotteryReward：领奖并重开盘面。
  async function claimAndRestart(): Promise<void> {
    applyRound(await lotteryApi.getLotteryReward())
  }

  // autoGetReward：领奖 + 重开（一并返回中奖对象）。
  async function autoClaim(): Promise<LotteryItem | null> {
    const r = await lotteryApi.autoGetReward()
    lastWin.value = r.win
    applyRound(r.round)
    return r.win
  }

  async function restartDraw(winId: number, winType: number): Promise<void> {
    const w = await lotteryApi.restartLottery(winId, winType)
    winInfo.value = w
    restartCount.value += 1
  }

  async function payMoney(count: number): Promise<boolean> {
    return lotteryApi.useMoney(count)
  }

  function reset(): void {
    todayCount.value = 0
    moneyOk.value = false
    board.value = []
    winInfo.value = { type: -1, id: 0, count: 0 }
    restartCount.value = 0
    lastWin.value = null
  }

  return {
    todayCount,
    moneyOk,
    board,
    winInfo,
    restartCount,
    lastWin,
    loadStatus,
    startDraw,
    claimWin,
    claimAndRestart,
    autoClaim,
    restartDraw,
    payMoney,
    reset,
  }
})
