import http from './http'

// —— M8 成就（对齐 backend/internal/achievement）——
// getOverviewStat / getAchivementsByGroup / getAchivementDetail 三者均返回多元组数组。

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

export interface AchievementRecent {
  id: number
  name: string
  content: string
  point: number
  achieveGetTime: string
  image: string
}

export interface AchievementGroupStat {
  group: number
  group_name: string
  total_count: number
  finish_count: number
}

export interface AchievementOverview {
  point: number
  recent: AchievementRecent[]
  groups: AchievementGroupStat[]
}

export interface Achievement {
  id: number
  name: string
  content: string
  point: number
  image: string
  achieveGetTime: string
}

export interface AchievementProgress {
  targetValue: number
  userValue: number
  content: string
  isDone: boolean
}

export interface AchievementInfo {
  id: number
  name: string
  content: string
  todo: string
  image: string
  type: number
  target_value: number
}

export interface AchievementDetail {
  info: AchievementInfo | null
  progress: AchievementProgress[]
  finishCount: number
}

/** 成就筛选：0 全部 / 1 已完成 / 2 未完成。 */
export type AchievementFilter = 0 | 1 | 2

/** 后端 ACHIVE_PAGE_CPP：分组列表每页 5 条。 */
export const ACHIEVEMENT_PAGE_SIZE = 5

function mapRecent(r: Row): AchievementRecent {
  return {
    id: asNum(r.id),
    name: asStr(r.name),
    content: asStr(r.content),
    point: asNum(r.point),
    achieveGetTime: asStr(r.achieveGetTime),
    image: asStr(r.image),
  }
}

function mapGroupStat(r: Row): AchievementGroupStat {
  return {
    group: asNum(r.group),
    group_name: asStr(r.group_name),
    total_count: asNum(r.total_count),
    finish_count: asNum(r.finish_count),
  }
}

function mapAchievement(r: Row): Achievement {
  return {
    id: asNum(r.id),
    name: asStr(r.name),
    content: asStr(r.content),
    point: asNum(r.point),
    image: asStr(r.image),
    achieveGetTime: asStr(r.achieveGetTime),
  }
}

/** 分组列表结果：[总数, 当前页 5 条]。 */
export interface AchievementListResult {
  count: number
  rows: Achievement[]
}

export async function getAchievementOverview(): Promise<AchievementOverview> {
  const { data } = await http.get<unknown[]>('/achievements/overview')
  return {
    point: asNum(data[0]),
    recent: asRows(data[1]).map(mapRecent),
    groups: asRows(data[2]).map(mapGroupStat),
  }
}

export async function getAchievementsByGroup(
  group: number,
  subgroup: number,
  type: AchievementFilter,
  page: number,
): Promise<AchievementListResult> {
  const { data } = await http.get<unknown[]>('/achievements/group', {
    params: { group, subgroup, type, page },
  })
  return { count: asNum(data[0]), rows: asRows(data[1]).map(mapAchievement) }
}

export async function getAchievementDetail(aid: number): Promise<AchievementDetail> {
  const { data } = await http.get<unknown>('/achievements/detail', { params: { aid } })
  // 成就不存在时后端返回 null（非数组）。
  const arr: unknown[] = Array.isArray(data) ? (data as unknown[]) : []
  const infoRow = asRow(arr[0])
  const progress: AchievementProgress[] = asRows(arr[1]).map((r) => ({
    targetValue: asNum(r.targetValue),
    userValue: asNum(r.userValue),
    content: asStr(r.content),
    isDone: r.isDone === true || r.isDone === 1,
  }))
  return {
    info: infoRow
      ? {
          id: asNum(infoRow.id),
          name: asStr(infoRow.name),
          content: asStr(infoRow.content),
          todo: asStr(infoRow.todo),
          image: asStr(infoRow.image),
          type: asNum(infoRow.type),
          target_value: asNum(infoRow.target_value),
        }
      : null,
    progress,
    finishCount: asNum(arr[2]),
  }
}
