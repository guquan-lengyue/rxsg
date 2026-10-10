import { defineStore } from 'pinia'
import { ref } from 'vue'

import * as taskApi from '@/api/task'
import type { Task, TaskDetail, TaskGroup } from '@/api/task'

// 任务 store：分组 / 列表 / 详情三元状态，均为用户级数据（与城池无关）。
export const useTaskStore = defineStore('task', () => {
  const groups = ref<TaskGroup[]>([])
  const tasks = ref<Task[]>([])
  const detail = ref<TaskDetail | null>(null)
  const sysCount = ref(0)
  const nobilityOk = ref(false)
  const activeType = ref(0)
  const activeGroup = ref(0)
  const detailTid = ref(0)
  const doneCount = ref(0)

  function reset(): void {
    groups.value = []
    tasks.value = []
    detail.value = null
    sysCount.value = 0
    nobilityOk.value = false
    activeType.value = 0
    activeGroup.value = 0
    detailTid.value = 0
    doneCount.value = 0
  }

  // 切换任务组类型：拉取该类型下的任务组并清空列表/详情。
  async function selectType(cid: number, type: number): Promise<void> {
    const r = await taskApi.getTaskGroups(cid, type)
    groups.value = r.groups
    sysCount.value = r.sysCount
    nobilityOk.value = r.nobilityOk
    activeType.value = type
    activeGroup.value = 0
    tasks.value = []
    doneCount.value = 0
    detail.value = null
    detailTid.value = 0
  }

  async function selectGroup(cid: number, group: number): Promise<void> {
    const r = await taskApi.getTaskList(cid, group)
    tasks.value = r.tasks
    doneCount.value = r.doneCount
    activeGroup.value = group
    detail.value = null
    detailTid.value = 0
  }

  async function loadDetail(cid: number, tid: number): Promise<void> {
    detail.value = await taskApi.getTaskDetail(cid, tid)
    detailTid.value = tid
  }

  function clearDetail(): void {
    detail.value = null
    detailTid.value = 0
  }

  // 领奖/放弃后，保持当前类型与分组选中，刷新分组计数与列表。
  async function refresh(cid: number): Promise<void> {
    const type = activeType.value
    const group = activeGroup.value
    const r = await taskApi.getTaskGroups(cid, type)
    groups.value = r.groups
    sysCount.value = r.sysCount
    nobilityOk.value = r.nobilityOk
    detail.value = null
    detailTid.value = 0
    if (group && r.groups.some((g) => g.id === group)) {
      await selectGroup(cid, group)
    } else {
      activeGroup.value = 0
      tasks.value = []
      doneCount.value = 0
    }
  }

  async function claim(cid: number, tid: number, selectId = 0, selectId2 = 0): Promise<void> {
    await taskApi.claimTaskReward(cid, tid, selectId, selectId2)
    await refresh(cid)
  }

  async function drop(cid: number, group: number): Promise<void> {
    await taskApi.dropTask(cid, group)
    await refresh(cid)
  }

  async function dropSys(cid: number, tid: number): Promise<void> {
    await taskApi.dropSysTask(cid, tid)
    await refresh(cid)
  }

  return {
    groups,
    tasks,
    detail,
    sysCount,
    nobilityOk,
    activeType,
    activeGroup,
    detailTid,
    doneCount,
    reset,
    selectType,
    selectGroup,
    loadDetail,
    clearDetail,
    refresh,
    claim,
    drop,
    dropSys,
  }
})
