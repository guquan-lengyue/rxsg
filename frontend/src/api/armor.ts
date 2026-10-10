import http from './http'

import type {
  ArmorUpgradeResult,
  BagArmor,
  BarnGood,
  CombineResult,
  EmbedResult,
  EquipResult,
  HeroArmorItem,
  StrongResult,
  UnladeResult,
} from '@/types'

// —— 装备（对齐后端 armor 包 REST 端点）——

export async function listBag(): Promise<BagArmor[]> {
  const { data } = await http.get<BagArmor[]>('/armors')
  return data
}

export async function getHeroArmors(hid: number): Promise<HeroArmorItem[]> {
  const { data } = await http.get<HeroArmorItem[]>(`/heroes/${hid}/armors`)
  return data
}

export async function equipArmor(hid: number, sid: number, spart: number): Promise<EquipResult> {
  const { data } = await http.post<EquipResult>('/armors/equip', { hid, sid, spart })
  return data
}

export async function offloadArmor(hid: number, spart: number): Promise<EquipResult> {
  const { data } = await http.post<EquipResult>('/armors/offload', { hid, spart })
  return data
}

export async function repairArmor(
  cid: number,
  sid: number,
): Promise<{ sid: number; hp_max: number; gold: number }> {
  const { data } = await http.post<{ sid: number; hp_max: number; gold: number }>(
    '/armors/repair',
    { cid, sid },
  )
  return data
}

export async function repairAllArmor(
  cid: number,
  sids: number[],
): Promise<{ gold: number }> {
  const { data } = await http.post<{ gold: number }>('/armors/repair-all', { cid, sids })
  return data
}

export async function renovateArmor(sid: number): Promise<{ sid: number }> {
  const { data } = await http.post<{ sid: number }>('/armors/renovate', { sid })
  return data
}

export async function renovateAllArmor(sids: number[]): Promise<{ ok: boolean }> {
  const { data } = await http.post<{ ok: boolean }>('/armors/renovate-all', { sids })
  return data
}

export async function sellArmor(
  cid: number,
  sid: number,
): Promise<{ sid: number; cid: number; gold: number }> {
  const { data } = await http.post<{ sid: number; cid: number; gold: number }>('/armors/sell', {
    cid,
    sid,
  })
  return data
}

export async function chaijieArmor(sid: number): Promise<{ msg: string }> {
  const { data } = await http.post<{ msg: string }>('/armors/chaijie', { sid })
  return data
}

export interface StrongPayload {
  cid: number
  gid1: number
  good1_count: number
  gid2: number
  good2_count: number
  sid: number
  is_zuoji: number
}

export async function strongArmor(p: StrongPayload): Promise<StrongResult> {
  const { data } = await http.post<StrongResult>('/armors/strong', p)
  return data
}

export async function combineArmor(
  mainSid: number,
  mainFlag: number,
  subSid1: number,
  subSid2: number,
  goodsFlag: number,
): Promise<CombineResult> {
  const { data } = await http.post<CombineResult>('/armors/combine', {
    main_sid: mainSid,
    main_flag: mainFlag,
    sub_sid1: subSid1,
    sub_sid2: subSid2,
    goods_flag: goodsFlag,
  })
  return data
}

export async function initHoles(cid: number, sid: number): Promise<{ reduce_gold: number }> {
  const { data } = await http.post<{ reduce_gold: number }>('/armors/holes', { cid, sid })
  return data
}

// open-hole 返回结算后的装备行（map），仅取所需列。
export async function openHole(
  sid: number,
  gid: number,
  pos: number,
  useType: number,
  count: number,
): Promise<Partial<BagArmor>> {
  const { data } = await http.post<Partial<BagArmor>>('/armors/open-hole', {
    sid,
    gid,
    pos,
    use_type: useType,
    count,
  })
  return data
}

export async function embedPearl(
  sid: number,
  pos: number,
  gid: number,
  isZuoji: number,
): Promise<EmbedResult> {
  const { data } = await http.post<EmbedResult>('/armors/embed', {
    sid,
    pos,
    gid,
    is_zuoji: isZuoji,
  })
  return data
}

// —— 马厩/坐骑（对齐后端 armor 包 R11-3 端点；字段名逐字对齐 handler.go）——

/** 加载马厩道具（洗练符 3 选 1 + 强化/升级材料），POST /armors/barn/goods，xilianIndex∈{0,1,2}。 */
export async function loadBarnGoods(xilianIndex: number): Promise<BarnGood[]> {
  const { data } = await http.post<BarnGood[]>('/armors/barn/goods', { xilian_index: xilianIndex })
  return data
}

/** 按槽位号加载可镶嵌的坐骑装备（POST /armors/barn/zuoji-armors），zuojiType∈{1..5}。 */
export async function loadZuojiArmors(zuojiType: number, armorid: number): Promise<BarnGood[]> {
  const { data } = await http.post<BarnGood[]>('/armors/barn/zuoji-armors', {
    zuoji_type: zuojiType,
    armorid,
  })
  return data
}

/** 卸下坐骑某槽位装备（POST /armors/barn/unlade）。 */
export async function barnUnlade(sid: number, gid: number, pos: number): Promise<UnladeResult> {
  const { data } = await http.post<UnladeResult>('/armors/barn/unlade', { sid, gid, pos })
  return data
}

/** 坐骑/装备升级（POST /armors/upgrade），isProtected 决定是否使用升级保护符。 */
export async function upgradeArmor(sid: number, isProtected: boolean): Promise<ArmorUpgradeResult> {
  const { data } = await http.post<ArmorUpgradeResult>('/armors/upgrade', {
    sid,
    is_protected: isProtected,
  })
  return data
}

/** 按 embed_pearls 串逐位取宝珠道具行（POST /armors/barn/embed-pearls）。 */
export async function barnEmbedPearls(gidStr: string): Promise<Record<string, unknown>[]> {
  const { data } = await http.post<Record<string, unknown>[]>('/armors/barn/embed-pearls', {
    gid_str: gidStr,
  })
  return data
}

/**
 * 未激活坐骑列表（GET /armors/barn/unactive-horse）。
 * 注：后端仅返回装备列表段，原版 6 段属性聚合未实现。
 */
export async function loadUnactiveHorse(): Promise<BagArmor[]> {
  const { data } = await http.get<BagArmor[]>('/armors/barn/unactive-horse')
  return data
}
