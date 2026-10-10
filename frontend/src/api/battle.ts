// M7 战斗查询 REST 封装（战斗列表 / 逐回合战报 / 原版 HTML 战报）。
import http from './http'

// —— DTO（对齐后端 battle 包 handler.go）——

/** 进行中/已结束战斗摘要（BattleBrief）。 */
export interface BattleBrief {
  id: number
  type: number
  state: number
  result: number
  round: number
  attack_uid: number
  resist_uid: number
  attack_cid: number
  resist_cid: number
  nexttime: number
  time_left: number
}

/** 逐回合原始战报行（RoundReport）。 */
export interface RoundReport {
  round: number
  report: string
}

/** 战斗详情（GET /battles/:bid）。 */
export interface BattleDetail {
  battle: BattleBrief
  rounds: RoundReport[]
}

/** 原版 HTML 战报行（reports 表；content 为原版 HTML，直接 v-html 渲染）。 */
export interface BattleReportRow {
  id: number
  title: number
  type: number
  time: number
  battleid: number
  content: string
}

/**
 * 逐回合战报单行（逗号分号分隔的数字串，按引擎 engine.go L474 的 17 字段顺序解析）。
 * 字段顺序：isAttack,isCity,sid,action,speed,istarget,shanghai,targetType,targetSid,
 * targetStart,siwang,targetEnd,ableFanji,fanjiShanghai,targetStartD,fanjiSiwang,fanjiEnd。
 */
export interface RoundRow {
  isAttack: number
  isCity: number
  sid: number
  action: number
  speed: number
  isTarget: number
  damage: number
  targetType: number
  targetSid: number
  targetStart: number
  dead: number
  targetEnd: number
  ableCounter: number
  counterDamage: number
  targetStartD: number
  counterDead: number
  counterEnd: number
}

function num(raw: string): number {
  const n = Number.parseFloat(raw)
  return Number.isFinite(n) ? Math.trunc(n) : 0
}

/** 把一条回合 report 串解析为多行（';' 分句、',' 分字段）。 */
export function parseRounds(report: string): RoundRow[] {
  const out: RoundRow[] = []
  for (const seg of report.split(';')) {
    const parts = seg.split(',')
    if (parts.length < 17) {
      continue
    }
    out.push({
      isAttack: num(parts[0]),
      isCity: num(parts[1]),
      sid: num(parts[2]),
      action: num(parts[3]),
      speed: num(parts[4]),
      isTarget: num(parts[5]),
      damage: num(parts[6]),
      targetType: num(parts[7]),
      targetSid: num(parts[8]),
      targetStart: num(parts[9]),
      dead: num(parts[10]),
      targetEnd: num(parts[11]),
      ableCounter: num(parts[12]),
      counterDamage: num(parts[13]),
      targetStartD: num(parts[14]),
      counterDead: num(parts[15]),
      counterEnd: num(parts[16]),
    })
  }
  return out
}

// —— 端点 ——

export async function listBattles(cid: number): Promise<BattleBrief[]> {
  const { data } = await http.get<BattleBrief[]>(`/cities/${cid}/battles`)
  return data
}

export async function getBattleDetail(bid: number): Promise<BattleDetail> {
  const { data } = await http.get<BattleDetail>(`/battles/${bid}`)
  return data
}

export async function listBattleReports(cid: number): Promise<BattleReportRow[]> {
  const { data } = await http.get<BattleReportRow[]>(`/cities/${cid}/battles/reports`)
  return data
}
