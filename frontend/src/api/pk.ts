import http from './http'

// —— M9 单机 PK 征战（对齐 backend/internal/pk）——
// 覆盖：关卡初始化、最大值刷新、首通奖励榜、玩家将领分页、领首通奖、单关战斗、购买军令。

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

/** 征战进度：normal/special 各以 battleId*100+levelId 编码。 */
export interface PkLevel {
  normal: number
  special: number
}

/** 关卡 NPC 将（cfg_pk_battle + cfg_pk_level + cfg_pk_hero 联合行）。 */
export interface PkHero {
  battleid: number
  levelid: number
  hero_level: number
  user_level: number
  hid: number
  face: number
  sex: number
  flag: number
  name: string
  area: number
  energy: number
  energy_add: number
  command_base: number
  command_add: number
  affair_base: number
  affair_add: number
  bravery_base: number
  bravery_add: number
  wisdom_base: number
  wisdom_add: number
  speed_base: number
  speed_add: number
  battle: number
}

export interface PkGoods {
  gid: number
  name: string
  level: number
}

export interface PkCampaignReward {
  battleid: number
  flag: number
  goods: PkGoods | null
}

export interface PkGoodsInfo {
  junling: number
  wuzhuqian: number
}

export interface PkBattleDesc {
  id: number
  description: string
}

export interface PkCampaign {
  level: PkLevel | null
  normalHeroes: PkHero[]
  specialHeroes: PkHero[]
  rewards: PkCampaignReward[]
  goods: PkGoodsInfo
  battles: PkBattleDesc[]
}

/** regetCampaignMaxData：[userBattleInfo, junlin, wuzhuqian]。 */
export interface PkMax {
  level: PkLevel | null
  junling: number
  wuzhuqian: number
}

/** 榜单奖品（道具 gid>0 / 装备 isArmor）。 */
export interface PkPrize {
  gid: number
  id: number
  name: string
  count: number
  isArmor: boolean
}

/** 首通榜一行（cfg_pk_first + 附加 userName/rewardGood 等）。 */
export interface PkRankRow {
  uid: number
  battleid: number
  type: number
  rankid: number
  passtime: number
  userName: string
  formatTime: string
  reward: string
  rewardType: number
  rewardCount: number
  rewardGood: PkPrize | null
}

/** 玩家将领（heroes + hero_blood 联合行，仅取所需列）。 */
export interface PkUserHero {
  hid: number
  name: string
  sex: number
  face: number
  level: number
  bravery: number
  hero_type: number
}

export interface PkHeroPage {
  heroes: PkUserHero[]
  page: number
  kingHero: PkUserHero | null
}

/** 单次攻击结算（battleComute 返回）。 */
export interface PkBattleHit {
  flag: number
  attack: number
  resist: number
  damage: number
  blood: number
  attackhid: number
  resisthid: number
  attacksex: number
  resistsex: number
  battleId: number
  attackStandIndex: number
  resistStandIndex: number
}

/** 单关战斗结果（startUserPK 返回 + reward）。 */
export interface PkBattle {
  report: PkBattleHit[][]
  totalNum: number
  endflag: number
  winer: number
  reward: PkPrize[]
}

function mapLevel(v: unknown): PkLevel | null {
  const r = asRow(v)
  return r ? { normal: asNum(r.normal), special: asNum(r.special) } : null
}

function mapHero(r: Row): PkHero {
  return {
    battleid: asNum(r.battleid),
    levelid: asNum(r.levelid),
    hero_level: asNum(r.hero_level),
    user_level: asNum(r.user_level),
    hid: asNum(r.hid),
    face: asNum(r.face),
    sex: asNum(r.sex),
    flag: asNum(r.flag),
    name: asStr(r.name),
    area: asNum(r.area),
    energy: asNum(r.energy),
    energy_add: asNum(r.energy_add),
    command_base: asNum(r.command_base),
    command_add: asNum(r.command_add),
    affair_base: asNum(r.affair_base),
    affair_add: asNum(r.affair_add),
    bravery_base: asNum(r.bravery_base),
    bravery_add: asNum(r.bravery_add),
    wisdom_base: asNum(r.wisdom_base),
    wisdom_add: asNum(r.wisdom_add),
    speed_base: asNum(r.speed_base),
    speed_add: asNum(r.speed_add),
    battle: asNum(r.battle),
  }
}

function mapGoods(v: unknown): PkGoods | null {
  const r = asRow(v)
  return r ? { gid: asNum(r.gid), name: asStr(r.name), level: asNum(r.level) } : null
}

// 奖励物品：有 gid → 道具；有 gtype=1 或仅有 id → 装备。
function mapPrize(v: unknown): PkPrize | null {
  const r = asRow(v)
  if (!r) {
    return null
  }
  const isArmor = asNum(r.gtype) === 1 || !Object.prototype.hasOwnProperty.call(r, 'gid')
  return {
    gid: isArmor ? 0 : asNum(r.gid),
    id: asNum(r.id),
    name: asStr(r.name),
    count: asNum(r.count),
    isArmor,
  }
}

function mapUserHero(r: Row): PkUserHero {
  return {
    hid: asNum(r.id),
    name: asStr(r.name),
    sex: asNum(r.sex),
    face: asNum(r.face),
    level: asNum(r.level),
    bravery: asNum(r.heroBravery),
    hero_type: asNum(r.hero_type),
  }
}

export async function loadCampaign(): Promise<PkCampaign> {
  const { data } = await http.get<unknown[]>('/pk/campaign')
  const goodsRow = asRow(data[4])
  return {
    level: mapLevel(data[0]),
    normalHeroes: asRows(data[1]).map(mapHero),
    specialHeroes: asRows(data[2]).map(mapHero),
    rewards: asRows(data[3]).map((r) => ({
      battleid: asNum(r.battleid),
      flag: asNum(r.flag),
      goods: mapGoods(r.goods),
    })),
    goods: { junling: asNum(goodsRow?.junling), wuzhuqian: asNum(goodsRow?.wuzhuqian) },
    battles: asRows(data[5]).map((r) => ({ id: asNum(r.id), description: asStr(r.description) })),
  }
}

export async function loadCampaignMax(): Promise<PkMax> {
  const { data } = await http.get<unknown[]>('/pk/campaign-max')
  return { level: mapLevel(data[0]), junling: asNum(data[1]), wuzhuqian: asNum(data[2]) }
}

/** 首通奖励榜（loadPKRewardRank）：[rows]。 */
export async function loadRewardRank(battleId: number, type: number): Promise<PkRankRow[]> {
  const { data } = await http.get<unknown[]>('/pk/reward-rank', {
    params: { battle_id: battleId, type },
  })
  return asRows(data[0]).map((r) => ({
    uid: asNum(r.uid),
    battleid: asNum(r.battleid),
    type: asNum(r.type),
    rankid: asNum(r.rankid),
    passtime: asNum(r.passtime),
    userName: asStr(r.userName),
    formatTime: asStr(r.formatTime),
    reward: asStr(r.reward),
    rewardType: asNum(r.rewardType),
    rewardCount: asNum(r.rewardCount),
    rewardGood: mapPrize(r.rewardGood),
  }))
}

/** 玩家将领分页（getAllHeroByUid）：[heroes, page] 或 [heroes, page, kingHero]。 */
export async function loadUserHeroes(page: number, hids: number[]): Promise<PkHeroPage> {
  const { data } = await http.get<unknown[]>('/pk/heroes', {
    params: { page, hids: hids.length ? hids.join(',') : '' },
  })
  const kingRow = data.length > 2 ? asRow(data[2]) : null
  return {
    heroes: asRows(data[0]).map(mapUserHero),
    page: asNum(data[1]),
    kingHero: kingRow ? mapUserHero(kingRow) : null,
  }
}

/** 领取首通榜首奖励（getPkFirstReward）：[parsedRewards]。 */
export async function claimFirstReward(
  battleId: number,
  flag: number,
  rankId: number,
): Promise<PkPrize[]> {
  const { data } = await http.post<unknown[]>('/pk/first-reward', {
    battle_id: battleId,
    flag,
    rank_id: rankId,
  })
  return asRows(data[0]).map((r) => mapPrize(r)).filter((p): p is PkPrize => p !== null)
}

/** 打一关（getOneBattleRet）：[battle]。 */
export async function startBattle(
  battleId: number,
  flag: number,
  level: number,
  hids: number[],
): Promise<PkBattle> {
  const { data } = await http.post<unknown[]>('/pk/battle', {
    battle_id: battleId,
    flag,
    level,
    hids,
  })
  const b = asRow(data[0])
  return {
    report: Array.isArray(b?.report)
      ? (b?.report as unknown[]).map((round) =>
          asRows(round).map((h) => ({
            flag: asNum(h.flag),
            attack: asNum(h.attack),
            resist: asNum(h.resist),
            damage: asNum(h.damage),
            blood: asNum(h.blood),
            attackhid: asNum(h.attackhid),
            resisthid: asNum(h.resisthid),
            attacksex: asNum(h.attacksex),
            resistsex: asNum(h.resistsex),
            battleId: asNum(h.battleId),
            attackStandIndex: asNum(h.attackStandIndex),
            resistStandIndex: asNum(h.resistStandIndex),
          })),
        )
      : [],
    totalNum: asNum(b?.totalNum),
    endflag: asNum(b?.endflag),
    winer: asNum(b?.winer),
    reward: asRows(b?.reward).map((r) => mapPrize(r)).filter((p): p is PkPrize => p !== null),
  }
}

/** 购买军令（buyJunlingFunc）：[type, newCount, money?]。 */
export async function buyJunling(type: number, count: number): Promise<{ newCount: number; money: number }> {
  const { data } = await http.post<unknown[]>('/pk/buy-junling', { type, count })
  return { newCount: asNum(data[1]), money: asNum(data[2]) }
}
