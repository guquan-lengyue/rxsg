import { defineStore } from 'pinia'
import { ref, shallowRef } from 'vue'

import * as worldApi from '@/api/world'
import type {
  FavouritesResult,
  GovernInfo,
  GovernPayload,
  InvadeCheck,
  WorldCell,
  WorldCityInfo,
  WorldFieldInfo,
  WorldMark,
} from '@/api/world'
import { blocksInView, cellOfWid, WORLD_SIZE } from '@/render/worldGrid'

// 世界地图 store：视野 / 地格（按 wid 索引）/ 选中目标 / 收藏 / 治理信息。
// 视野与地格按 block 分段懒加载（getBlockData 一次可传多个 block）。
export const useWorldStore = defineStore('world', () => {
  const cells = shallowRef<Map<number, WorldCell>>(new Map())
  const view = ref({ x: 0, y: 0 })
  const cols = ref(12)
  const rows = ref(8)
  const marks = shallowRef<WorldMark[]>([])
  const selectedWid = ref(0)
  const selectedCity = ref<WorldCityInfo | null>(null)
  const selectedField = ref<WorldFieldInfo | null>(null)
  const invade = ref<InvadeCheck | null>(null)
  const governInfo = ref<GovernInfo | null>(null)
  const favourites = ref<FavouritesResult>({ cities: [], favourites: [] })
  const userFields = shallowRef<number[]>([])
  const actionFields = shallowRef<number[]>([])

  // 已加载 block（非响应式，仅作去重缓存）。
  const loadedBlocks = new Set<number>()

  function maxOrigin(): { x: number; y: number } {
    return { x: Math.max(0, WORLD_SIZE - cols.value), y: Math.max(0, WORLD_SIZE - rows.value) }
  }

  function clampView(x: number, y: number): { x: number; y: number } {
    const max = maxOrigin()
    return { x: Math.min(Math.max(0, x), max.x), y: Math.min(Math.max(0, y), max.y) }
  }

  /** 调整可视行数（cols 不变）：视野钳制回界内，并预取新增 block，避免 resizing 后出现空白。 */
  function setRows(n: number): void {
    const r = Math.max(1, Math.min(WORLD_SIZE, Math.round(n)))
    if (r === rows.value) {
      return
    }
    rows.value = r
    view.value = clampView(view.value.x, view.value.y)
    void ensureArea(view.value.x, view.value.y, cols.value, rows.value)
  }

  /** 加载若干 block（已加载的跳过），合并进 cells 并回写 marks。 */
  async function loadBlocks(blocks: number[]): Promise<void> {
    const need = blocks.filter((b) => !loadedBlocks.has(b))
    if (need.length === 0) {
      return
    }
    const data = await worldApi.getBlockData(need)
    const next = new Map(cells.value)
    for (const chunk of data.chunks) {
      for (const cell of chunk.cells) {
        next.set(cell.wid, cell)
      }
    }
    for (const b of need) {
      loadedBlocks.add(b)
    }
    cells.value = next
    marks.value = data.marks
  }

  /** 确保当前视野覆盖的 block 已加载。 */
  async function ensureArea(x: number, y: number, c: number, r: number): Promise<void> {
    await loadBlocks(blocksInView({ x, y }, c, r))
  }

  /** 平移视野并按需加载新 block。 */
  async function moveView(x: number, y: number): Promise<void> {
    view.value = clampView(x, y)
    await ensureArea(view.value.x, view.value.y, cols.value, rows.value)
  }

  /** 将视野居中到某 wid。 */
  async function centerOnWid(wid: number): Promise<void> {
    const c = cellOfWid(wid)
    await moveView(c.x - Math.floor(cols.value / 2), c.y - Math.floor(rows.value / 2))
  }

  /** 清空选中态。 */
  function clearSelection(): void {
    selectedWid.value = 0
    selectedCity.value = null
    selectedField.value = null
    invade.value = null
    governInfo.value = null
  }

  /** 选中地格：拉取野地/城池详情（城池再取入侵判定与治理信息）。 */
  async function selectCell(wid: number, myCid: number): Promise<void> {
    selectedWid.value = wid
    selectedCity.value = null
    selectedField.value = null
    invade.value = null
    governInfo.value = null
    const rowsInfo = await worldApi.getWorldFieldInfo(wid)
    const field = rowsInfo[0] ?? null
    selectedField.value = field
    if (field && field.type === 0 && field.ownercid > 0) {
      const cities = await worldApi.getWorldCityInfo([field.ownercid])
      selectedCity.value = cities[0] ?? null
      invade.value = await worldApi.checkCanInvade(field.ownercid)
      if (myCid > 0 && selectedCity.value && selectedCity.value.citytype > 0 && selectedCity.value.citytype < 5) {
        governInfo.value = await worldApi.getGovernInfo(myCid)
      }
    }
  }

  async function reloadSelected(myCid: number): Promise<void> {
    if (selectedWid.value > 0) {
      await selectCell(selectedWid.value, myCid)
    }
  }

  async function loadFavourites(): Promise<void> {
    favourites.value = await worldApi.getFavouritesList()
  }

  async function addFavourite(cid: number): Promise<void> {
    await worldApi.addFavourites(cid)
  }

  async function removeFavourite(id: number): Promise<void> {
    favourites.value = await worldApi.deleteFavourites(id)
  }

  async function setFavComments(id: number, comments: string): Promise<void> {
    await worldApi.setFavouritesComments(id, comments)
  }

  async function loadMyFields(): Promise<void> {
    const [uf, af] = await Promise.all([worldApi.getUserFields(), worldApi.getActionField(0)])
    userFields.value = uf
    actionFields.value = af.wids.filter((w) => w > 0)
  }

  async function loadGovern(myCid: number): Promise<void> {
    if (myCid > 0) {
      governInfo.value = await worldApi.getGovernInfo(myCid)
    }
  }

  async function buildCity(wid: number): Promise<void> {
    await worldApi.createCityFromLand(wid)
  }

  async function mark(cid: number): Promise<void> {
    await worldApi.markCity(cid)
  }

  async function war(uid: number, cid: number): Promise<void> {
    await worldApi.startWar(uid, cid)
  }

  async function govern(payload: GovernPayload): Promise<void> {
    await worldApi.governOthers(payload)
  }

  return {
    cells,
    view,
    cols,
    rows,
    setRows,
    marks,
    selectedWid,
    selectedCity,
    selectedField,
    invade,
    governInfo,
    favourites,
    userFields,
    actionFields,
    loadBlocks,
    ensureArea,
    moveView,
    centerOnWid,
    clearSelection,
    selectCell,
    reloadSelected,
    loadFavourites,
    addFavourite,
    removeFavourite,
    setFavComments,
    loadMyFields,
    loadGovern,
    buildCity,
    mark,
    war,
    govern,
  }
})
