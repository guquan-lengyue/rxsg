import http from './http'

// —— M9 抽奖（幸运宝匣，对齐 backend/internal/lottery）——
// 默认行为复刻原版：checkLottoryTime 恒抛 not_available_time，
// 且未达公士爵位时 start 返回「你爵位未达到公士，不能使用幸运宝匣。」（HTTP 400）。

type Row = Record<string, unknown>

function asNum(v: unknown): number {
  if (typeof v === 'number') {
    return v
  }
  if (typeof v === 'string') {
    const n = Number(v)
    return Number.isFinite(n) ? n : 0
  }
  return 0
}

function asStr(v: unknown): string {
  return typeof v === 'string' ? v : ''
}

function asRows(v: unknown): Row[] {
  return Array.isArray(v) ? (v as Row[]) : []
}

function asRow(v: unknown): Row | null {
  return v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Row) : null
}

/** 盘面格子物品：gid>0 为道具（item_{gid}.png），否则为装备（armor/{id}.png）。 */
export interface LotteryItem {
  gid: number
  id: number
  name: string
  count: number
  level: number
  isArmor: boolean
}

/** 盘面 8 档（index 0 为 1 等 … index 7 为 8 等）。 */
export type LotteryBoard = LotteryItem[][]

/** 当前中奖档位：type 0=道具/礼金 1=装备；未抽奖时为 {type:-1,id:0,count:0}。 */
export interface LotteryWin {
  type: number
  id: number
  count: number
}

/** getGoods 返回：盘面 + 中奖 + 重开次数 + 今日次数。 */
export interface LotteryRound {
  board: LotteryBoard
  win: LotteryWin
  restartCount: number
  todayCount: number
}

export interface LotteryStartResult {
  win: LotteryWin
  todayCount: number
}

function mapItem(r: Row): LotteryItem {
  const hasGid = Object.prototype.hasOwnProperty.call(r, 'gid')
  return {
    gid: hasGid ? asNum(r.gid) : 0,
    id: asNum(r.id),
    name: asStr(r.name),
    count: asNum(r.count),
    level: asNum(r.level),
    isArmor: !hasGid,
  }
}

function mapBoard(v: unknown): LotteryBoard {
  return (Array.isArray(v) ? (v as unknown[]) : []).map((tier) => asRows(tier).map(mapItem))
}

// getGoods 元组：[records, winType, winId, winCount, restartCount, todayCount]
function mapRound(data: unknown[]): LotteryRound {
  return {
    board: mapBoard(data[0]),
    win: { type: asNum(data[1]), id: asNum(data[2]), count: asNum(data[3]) },
    restartCount: asNum(data[4]),
    todayCount: asNum(data[5]),
  }
}

export async function getTodayCount(): Promise<number> {
  const { data } = await http.get<unknown>('/lottery/today-count')
  return asNum(data)
}

export async function checkMoney(): Promise<boolean> {
  const { data } = await http.get<unknown>('/lottery/check-money')
  return data === true
}

/** startLottery：返回 [winType, winId, winCount, todayCount]；次数达上限时为 [-1,-2,-1,count]。 */
export async function startLottery(): Promise<LotteryStartResult> {
  const { data } = await http.post<unknown[]>('/lottery/start')
  return { win: { type: asNum(data[0]), id: asNum(data[1]), count: asNum(data[2]) }, todayCount: asNum(data[3]) }
}

/** getLotteryReward：领奖并重开盘面，返回 getGoods 结果。 */
export async function getLotteryReward(): Promise<LotteryRound> {
  const { data } = await http.post<unknown[]>('/lottery/reward')
  return mapRound(data)
}

/** autoGetReward：返回 [winObj, newRound]。 */
export async function autoGetReward(): Promise<{ win: LotteryItem | null; round: LotteryRound }> {
  const { data } = await http.post<unknown[]>('/lottery/auto-reward')
  const winRow = asRow(data[0])
  return { win: winRow ? mapItem(winRow) : null, round: mapRound(asRows(data[1])) }
}

/** addCount：领取当前盘面奖励，返回 winObj。 */
export async function getWin(): Promise<LotteryItem | null> {
  const { data } = await http.post<unknown>('/lottery/win')
  const row = asRow(data)
  return row ? mapItem(row) : null
}

/** restart：重开一次（每日仅一次），返回 [winType, winId, winCount]。 */
export async function restartLottery(winId: number, winType: number): Promise<LotteryWin> {
  const { data } = await http.post<unknown[]>('/lottery/restart', { win_id: winId, win_type: winType })
  return { type: asNum(data[0]), id: asNum(data[1]), count: asNum(data[2]) }
}

/** useMoney：count>1 才扣 6 元宝；返回 [1] 成功 / [0] 元宝不足。 */
export async function useMoney(count: number): Promise<boolean> {
  const { data } = await http.post<unknown[]>('/lottery/use-money', { count })
  return asNum(data[0]) === 1
}
