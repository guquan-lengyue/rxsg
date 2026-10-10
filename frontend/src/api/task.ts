import http from './http'

// —— M8 任务（对齐 backend/internal/task）——
// legacy 经 AMF 网关按函数名分派，这里按 REST 端点逐一映射；
// 后端多数接口返回「多元组数组」（结构体非定长），故此处把 unknown[] 收窄为显式 DTO。

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

function asBool(v: unknown): boolean {
  return v === true || v === 1
}

function asRows(v: unknown): Row[] {
  return Array.isArray(v) ? (v as Row[]) : []
}

function asRow(v: unknown): Row | null {
  return v !== null && typeof v === 'object' && !Array.isArray(v) ? (v as Row) : null
}

export interface TaskGroup {
  id: number
  name: string
  type: number
  priority: number
  count: number
}

export interface Task {
  id: number
  group: number
  name: string
  todo: string
  state: boolean
}

export interface TaskGoal {
  id: number
  tid: number
  sort: number
  type: number
  count: number
  content: string
  currentcount: number
  state: boolean
}

export interface TaskReward {
  id: number
  tid: number
  sort: number
  type: number
  count: number
}

// getTaskTypeGroupList：type=7 → [groups, sysCount]；type∈{0,1,2,3,5,6} → [groups, 0, 0, nobilityOk]。
export interface TaskGroupList {
  groups: TaskGroup[]
  /** 系统随机任务当日剩余领取数（仅 type=7 有意义）。 */
  sysCount: number
  /** 爵位是否达到使用条件（type∈{0,1,2,3,5,6} 的第 4 项）。 */
  nobilityOk: boolean
}

// getTaskList：[tasklist, 已完成数]。
export interface TaskListResult {
  tasks: Task[]
  doneCount: number
}

// getTaskDetail：[cfg_task, goals(带 state), rewards]。
export interface TaskDetail {
  task: Task | null
  goals: TaskGoal[]
  rewards: TaskReward[]
}

function mapGroup(r: Row): TaskGroup {
  return {
    id: asNum(r.id),
    name: asStr(r.name),
    type: asNum(r.type),
    priority: asNum(r.priority),
    count: asNum(r.count),
  }
}

function mapTask(r: Row): Task {
  return {
    id: asNum(r.id),
    group: asNum(r.group),
    name: asStr(r.name),
    todo: asStr(r.todo),
    state: asBool(r.state),
  }
}

function mapGoal(r: Row): TaskGoal {
  return {
    id: asNum(r.id),
    tid: asNum(r.tid),
    sort: asNum(r.sort),
    type: asNum(r.type),
    count: asNum(r.count),
    content: asStr(r.content),
    currentcount: asNum(r.currentcount),
    state: asBool(r.state),
  }
}

function mapReward(r: Row): TaskReward {
  return {
    id: asNum(r.id),
    tid: asNum(r.tid),
    sort: asNum(r.sort),
    type: asNum(r.type),
    count: asNum(r.count),
  }
}

export async function getTaskGroups(cid: number, type: number): Promise<TaskGroupList> {
  const { data } = await http.get<unknown[]>(`/cities/${cid}/tasks/groups`, { params: { type } })
  const groups = asRows(data[0]).map(mapGroup)
  const sysCount = type === 7 ? asNum(data[1]) : 0
  const nobilityOk = type !== 7 && data.length > 3 ? asBool(data[3]) : false
  return { groups, sysCount, nobilityOk }
}

export async function getTaskList(cid: number, group: number): Promise<TaskListResult> {
  const { data } = await http.get<unknown[]>(`/cities/${cid}/tasks/list`, { params: { group } })
  return { tasks: asRows(data[0]).map(mapTask), doneCount: asNum(data[1]) }
}

export async function getTaskDetail(cid: number, tid: number): Promise<TaskDetail> {
  const { data } = await http.get<unknown[]>(`/cities/${cid}/tasks/detail`, { params: { tid } })
  const taskRow = asRow(data[0])
  return {
    task: taskRow ? mapTask(taskRow) : null,
    goals: asRows(data[1]).map(mapGoal),
    rewards: asRows(data[2]).map(mapReward),
  }
}

/** 领奖（getReward）：selectId/selectId2 对应 legacy 装备/武将选择；成功返回 [1]。 */
export async function claimTaskReward(
  cid: number,
  tid: number,
  selectId = 0,
  selectId2 = 0,
): Promise<void> {
  await http.post(`/cities/${cid}/tasks/reward`, { tid, select_id: selectId, select_id2: selectId2 })
}

/** 放弃整组任务（dropTask）。 */
export async function dropTask(cid: number, taskgroup: number): Promise<void> {
  await http.post(`/cities/${cid}/tasks/drop`, { taskgroup })
}

/** 放弃随机系统任务（dropSysTask）。 */
export async function dropSysTask(cid: number, tid: number): Promise<void> {
  await http.post(`/cities/${cid}/tasks/sys/drop`, { tid })
}
