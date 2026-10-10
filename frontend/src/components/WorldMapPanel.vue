<script setup lang="ts">
// 世界地图面板：Canvas 栅格（worldGrid.ts）+ 选中目标侧栏 + 收藏/我的领地页签。
// props in / events out：业务写操作由父级（CityView）经 runX 执行，面板内不直接调用 store；
// 例外：视口自适应行数（fitRows → world.setRows）属布局，直接写 store 的 rows 以保证单一来源。
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import type { CSSProperties } from 'vue'

import SkinDialog from '@/components/SkinDialog.vue'
import { img } from '@/assets/img'
import { CELL_H, CELL_W, WORLD_SIZE, drawWorldGrid, widAt, widToCid } from '@/render/worldGrid'
import { useWorldStore } from '@/stores/world'
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
import type { DispatchPayload, DraftSoldier, Hero } from '@/types'

const props = defineProps<{
  cells: Map<number, WorldCell>
  viewX: number
  viewY: number
  cols: number
  rows: number
  marks: WorldMark[]
  selectedWid: number
  city: WorldCityInfo | null
  field: WorldFieldInfo | null
  invade: InvadeCheck | null
  governInfo: GovernInfo | null
  favourites: FavouritesResult
  userFields: number[]
  actionFields: number[]
  selfUid: number
  myCid: number
  heroes: Hero[]
  soldiers: DraftSoldier[]
  loading: boolean
  busy: boolean
  error: string
  notice: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'view', x: number, y: number): void
  (e: 'select', wid: number): void
  (e: 'refresh'): void
  (e: 'build-city', wid: number): void
  (e: 'mark', cid: number): void
  (e: 'war', uid: number, cid: number): void
  (e: 'favourite', cid: number): void
  (e: 'unfavourite', id: number): void
  (e: 'fav-comment', id: number, comments: string): void
  (e: 'govern', payload: GovernPayload): void
  (e: 'dispatch', payload: DispatchPayload): void
  (e: 'goto-wid', wid: number): void
  (e: 'goto-cid', cid: number): void
}>()

const canvas = ref<HTMLCanvasElement | null>(null)
const tab = ref<'target' | 'fav' | 'fields'>('target')
const gotoInput = ref('')
const hoverWid = ref(0)
const hoverPos = ref({ x: 0, y: 0 })

const world = useWorldStore()

// 按视口可用高度推导可视行数（列数保持 12），每格仍为 CELL_W×CELL_H 像素，不做 CSS 缩放
// （命中测试依赖 e.offsetX/offsetY 与常量，缩放会使选格/拖拽失真）。
// 预留高度 = 弹窗外边距 24×2 + 对话框边框/内边距 + 标题栏 + 底部按钮栏 + 正文内边距
//          + 视野提示行 + 画布下的小地图/提示行（实测合计 351px）。
const MAP_ROWS_MIN = 4
const MAP_ROWS_MAX = 8
const MAP_RESERVED_H = 351

/** 依视口高度计算并写入行数（cols/rows 仍是 store 的单一来源，绘制/分块/翻页共用）。 */
function fitRows(): void {
  const next = Math.max(
    MAP_ROWS_MIN,
    Math.min(MAP_ROWS_MAX, Math.floor((window.innerHeight - MAP_RESERVED_H) / CELL_H)),
  )
  world.setRows(next)
}

const minimapUrl = img('minimap.jpg')

// —— 原版地图控件贴图（public/images 实存；对应 AS images/map_*、images/ditu_*、images/move_to_*）——
/** 三态按钮贴图：常态 / 悬停 / 按下。 */
interface BtnSkin {
  up: string
  hl: string
  dn: string
}

interface DirButton extends BtnSkin {
  key: string
  title: string
  dx: number
  dy: number
}

// 8 向方向按钮：up 的按下态用 _pr，其余用 _down（与 AS 内嵌类一致）。
const DIR_BUTTONS: DirButton[] = [
  { key: 'up', title: '上移', dx: 0, dy: -1, up: img('map_up_up.png'), hl: img('map_up_hl.png'), dn: img('map_up_pr.png') },
  { key: 'down', title: '下移', dx: 0, dy: 1, up: img('map_down_up.png'), hl: img('map_down_hl.png'), dn: img('map_down_down.png') },
  { key: 'left', title: '左移', dx: -1, dy: 0, up: img('map_left_up.png'), hl: img('map_left_hl.png'), dn: img('map_left_down.png') },
  { key: 'right', title: '右移', dx: 1, dy: 0, up: img('map_right_up.png'), hl: img('map_right_hl.png'), dn: img('map_right_down.png') },
  { key: 'upleft', title: '左上移', dx: -1, dy: -1, up: img('map_upleft_up.png'), hl: img('map_upleft_hl.png'), dn: img('map_upleft_down.png') },
  { key: 'upright', title: '右上移', dx: 1, dy: -1, up: img('map_upright_up.png'), hl: img('map_upright_hl.png'), dn: img('map_upright_down.png') },
  { key: 'downleft', title: '左下移', dx: -1, dy: 1, up: img('map_downleft_up.png'), hl: img('map_downleft_hl.png'), dn: img('map_downleft_down.png') },
  { key: 'downright', title: '右下移', dx: 1, dy: 1, up: img('map_downright_up.png'), hl: img('map_downright_hl.png'), dn: img('map_downright_down.png') },
]

const controllerBg = img('map_controller.png')
const controllerBarBg = img('map_controller1_bg.png')

/** 三态贴图 → CSS 自定义属性（style 绑定用，避免 any）。 */
function btnStyle(skin: BtnSkin): CSSProperties {
  return { '--btn-up': `url(${skin.up})`, '--btn-hl': `url(${skin.hl})`, '--btn-dn': `url(${skin.dn})` }
}

// 定位我的城池：复用既有 goto-cid（父级 centerOnWid + selectCell），不新增业务。
const MYCITY_SKIN: BtnSkin = { up: img('map_mycity_up.png'), hl: img('map_mycity_hl.png'), dn: img('map_mycity_down.png') }

function onMyCity(): void {
  if (props.myCid > 0) {
    emit('goto-cid', props.myCid)
  }
}

// 移动模式：现版拖拽平移常驻、无模式开关，故仅做视觉按钮并禁用。
const MOVE_SKIN: BtnSkin = { up: img('map_move_up.png'), hl: img('map_move_hl.png'), dn: img('map_move_down.png') }

// 左右翻页：复用 view 事件按一屏平移；禁用态用 *_disbaled 贴图。
interface PageSkin {
  up: string
  over: string
  down: string
  disabled: string
}

const PAGE_LEFT_SKIN: PageSkin = {
  up: img('move_to_left.png'),
  over: img('move_to_left_over.png'),
  down: img('move_to_left_down.png'),
  disabled: img('move_to_left_disbaled.png'),
}
const PAGE_RIGHT_SKIN: PageSkin = {
  up: img('move_to_right.png'),
  over: img('move_to_right_over.png'),
  down: img('move_to_right_down.png'),
  disabled: img('move_to_right_disbaled.png'),
}

const canPageLeft = computed(() => props.viewX > 0)
const canPageRight = computed(() => props.viewX + props.cols < WORLD_SIZE)

function pageLeft(): void {
  pan(-props.cols, 0)
}

function pageRight(): void {
  pan(props.cols, 0)
}

function pageStyle(skin: PageSkin, enabled: boolean): CSSProperties {
  if (!enabled) {
    const d = `url(${skin.disabled})`
    return { '--btn-up': d, '--btn-hl': d, '--btn-dn': d }
  }
  return { '--btn-up': `url(${skin.up})`, '--btn-hl': `url(${skin.over})`, '--btn-dn': `url(${skin.down})` }
}

// 地图容器边框/侧边/底部/四角/关闭页签（装饰层均 pointer-events:none，不遮挡 canvas 交互）。
const FRAME_IMGS = {
  side: img('ditu_side.png'),
  bottom: img('ditu_bottom.png'),
  corner1: img('ditu_b1.png'),
  corner2: img('ditu_b2.png'),
  corner3: img('ditu_b3.png'),
  corner4: img('ditu_b4.png'),
}
const CLOSE_SKIN: BtnSkin = { up: img('ditu_close1.png'), hl: img('ditu_close2.png'), dn: img('ditu_close2.png') }

// —— 高亮集合 ——
const ownWids = computed(() => new Set(props.userFields))
const actionWids = computed(() => new Set(props.actionFields))
const markWids = computed(() => new Set(props.marks.map((m) => m.wid)))

// —— 选中/悬停地格 ——
const hoverCell = computed(() => (hoverWid.value ? props.cells.get(hoverWid.value) ?? null : null))
const selectedCell = computed(() =>
  props.selectedWid ? props.cells.get(props.selectedWid) ?? null : null,
)
const focusCell = computed(() => hoverCell.value ?? selectedCell.value)

function coordOfWid(wid: number): { x: number; y: number } {
  if (!wid) {
    return { x: props.viewX, y: props.viewY }
  }
  const cid = widToCid(wid)
  return { x: cid % 1000, y: Math.floor(cid / 1000) }
}

const focusCoord = computed(() => coordOfWid(props.selectedWid || hoverWid.value))
const selectedCoord = computed(() => coordOfWid(props.selectedWid))
const hoverCoord = computed(() => coordOfWid(hoverWid.value))

const selectedCid = computed(() => props.city?.cid ?? (props.field ? widToCid(props.field.wid) : 0))
const favEntry = computed(() =>
  selectedCid.value > 0
    ? props.favourites.favourites.find((f) => f.cid === selectedCid.value) ?? null
    : null,
)

// 司隶：mem_world.province == '1'（checkIsInSili 无独立端点，按地格 province 判定）。
const inSili = computed(() => {
  const p = selectedCell.value?.province ?? props.field?.province ?? ''
  return p === '1'
})

const canInvadeText = computed(() => {
  const inv = props.invade
  if (!inv) {
    return '—'
  }
  if (inv.can) {
    return '可以入侵'
  }
  return `尚未满足（${inv.invaded}/${inv.total}）`
})

function terrainName(type: number): string {
  switch (type) {
    case 0:
      return '城池'
    case 1:
      return '平原'
    case 2:
      return '森林'
    case 3:
      return '山地'
    case 4:
      return '湖泊'
    case 5:
      return '沙漠'
    default:
      return '野地'
  }
}

function cityTypeName(type: number): string {
  switch (type) {
    case 1:
      return '县'
    case 2:
      return '郡'
    case 3:
      return '州'
    case 4:
      return '都'
    default:
      return '普通'
  }
}

// —— 渲染 ——
function render(): void {
  const el = canvas.value
  if (!el) {
    return
  }
  drawWorldGrid(el, {
    cells: props.cells,
    view: { x: props.viewX, y: props.viewY },
    cols: props.cols,
    rows: props.rows,
    selectedWid: props.selectedWid || undefined,
    hoverWid: hoverWid.value || undefined,
    ownWids: ownWids.value,
    actionWids: actionWids.value,
    markWids: markWids.value,
  })
}

onMounted(() => {
  fitRows()
  render()
  window.addEventListener('resize', fitRows)
})
onBeforeUnmount(() => {
  window.removeEventListener('resize', fitRows)
})
watch(
  [
    () => props.cells,
    () => props.viewX,
    () => props.viewY,
    () => props.cols,
    () => props.rows,
    () => props.selectedWid,
    hoverWid,
    ownWids,
    actionWids,
    markWids,
  ],
  render,
  { flush: 'post' },
)

// —— 平移交互：拖拽 / 方向键 / map_up 按钮 ——
const drag = ref<{ clientX: number; clientY: number; originX: number; originY: number; moved: boolean } | null>(null)

function pan(dx: number, dy: number): void {
  emit('view', props.viewX + dx, props.viewY + dy)
}

function onMouseDown(e: MouseEvent): void {
  drag.value = { clientX: e.clientX, clientY: e.clientY, originX: props.viewX, originY: props.viewY, moved: false }
  canvas.value?.focus()
}

function onMouseMove(e: MouseEvent): void {
  const el = canvas.value
  if (!el) {
    return
  }
  hoverWid.value = widAt(e.offsetX, e.offsetY, { x: props.viewX, y: props.viewY })
  hoverPos.value = { x: e.offsetX, y: e.offsetY }
  const d = drag.value
  if (d) {
    if (Math.abs(e.clientX - d.clientX) > 4 || Math.abs(e.clientY - d.clientY) > 4) {
      d.moved = true
    }
    const dx = Math.round((e.clientX - d.clientX) / CELL_W)
    const dy = Math.round((e.clientY - d.clientY) / CELL_H)
    if (d.moved && (dx !== 0 || dy !== 0)) {
      emit('view', d.originX - dx, d.originY - dy)
    }
  }
}

function onMouseUp(): void {
  const d = drag.value
  if (d && !d.moved && hoverWid.value > 0) {
    emit('select', hoverWid.value)
  }
  drag.value = null
}

function onMouseLeave(): void {
  drag.value = null
  hoverWid.value = 0
}

function onKeydown(e: KeyboardEvent): void {
  switch (e.key) {
    case 'ArrowUp':
      pan(0, -1)
      break
    case 'ArrowDown':
      pan(0, 1)
      break
    case 'ArrowLeft':
      pan(-1, 0)
      break
    case 'ArrowRight':
      pan(1, 0)
      break
    default:
      return
  }
  e.preventDefault()
}

function doGoto(): void {
  const parts = gotoInput.value.split(/[,\s]+/).filter((p) => p.length > 0)
  if (parts.length < 2) {
    return
  }
  const x = Number(parts[0])
  const y = Number(parts[1])
  if (!Number.isFinite(x) || !Number.isFinite(y)) {
    return
  }
  emit('view', Math.round(x), Math.round(y))
}

// 悬停/选中地格的气泡文本
const bubbleText = computed(() => {
  const c = hoverCell.value
  if (!c) {
    return ''
  }
  const coord = hoverCoord.value
  return `(坐标 ${coord.x + 1},${coord.y + 1})　${c.province}·${c.jun}　${terrainName(c.type)}`
})

// —— 出征任务枚举（原版 Ground_CampaignDialog_14..18：0运输/1派遣/2侦察/3掠夺/4占领）——
// 后端 army.dispatch 仅接受 task∈{3掠夺,4占领}（service.go:789），且占领要求 target_type=1
// （service.go:792 occupy_only_field）→ 城池目标（target_type=2）仅 掠夺(3) 可用，故只渲染该项。
const CITY_TASKS: { task: number; name: string }[] = [{ task: 3, name: '掠夺' }]

// —— 出征（复用 army dispatch：城池目标 target_type=2）——
const showDispatch = ref(false)
const showGovern = ref(false)
const heroId = ref(0)
const selectedTask = ref(0)
const dispatchCounts = reactive<Record<number, number>>({})

watch(
  () => props.soldiers,
  (list) => {
    for (const s of list) {
      if (dispatchCounts[s.sid] === undefined) {
        dispatchCounts[s.sid] = 0
      }
    }
  },
  { immediate: true },
)

function openDispatch(task: number): void {
  heroId.value = 0
  selectedTask.value = task
  for (const s of props.soldiers) {
    dispatchCounts[s.sid] = 0
  }
  showDispatch.value = true
}

function submitDispatch(): void {
  const cid = props.city?.cid ?? 0
  if (cid <= 0) {
    return
  }
  const soldiers: Record<string, number> = {}
  for (const s of props.soldiers) {
    const c = dispatchCounts[s.sid] ?? 0
    if (c > 0) {
      soldiers[String(s.sid)] = c
    }
  }
  if (Object.keys(soldiers).length === 0) {
    return
  }
  emit('dispatch', { hero_id: heroId.value, target_type: 2, target_id: cid, task: selectedTask.value, soldiers })
  showDispatch.value = false
}

function onWar(): void {
  if (props.city) {
    emit('war', props.city.uid, props.city.cid)
  }
}

function onBuild(): void {
  if (selectedCell.value) {
    emit('build-city', selectedCell.value.wid)
  } else {
    emit('build-city', props.selectedWid)
  }
}

function canBuild(): boolean {
  const c = selectedCell.value
  return !!c && c.type === 1 && c.ownercid === props.myCid && c.state === 0 && props.myCid > 0
}

// 目标操作可用性：原版 WorldActionDialog 按 flag 选 ViewStack 页（WorldActionDialog.as:2933-3014）。
// flag 语义见 docs/SWF前端逻辑还原/08-世界地图.md §3.5。
const cityOps = computed(() => {
  const flag = props.city?.flag ?? -1
  const citytype = props.city?.citytype ?? 0
  return {
    // 掠夺：flag4敌对盟/flag5 NPC可占/flag6个人宣战中/flag9；flag8联盟宣战等待中仅名城可攻（page5）。
    canPlunder:
      flag === 4 || flag === 5 || flag === 6 || flag === 9 || (flag === 8 && citytype > 0 && citytype !== 5),
    // 宣战：仅「无小旗」flag7（page8）；敌对/已宣战状态下原版不提供宣战入口。
    canWar: flag === 7,
    // 治理：govern.visible=true 于 flag2..8（flag0/1/9 隐藏，WorldActionDialog.as:3019）；
    // 且治理仅对名城（县城及以上）有效，故叠加 citytype 1..4。
    canGovern: flag >= 2 && flag <= 8 && citytype > 0 && citytype < 5,
    // 标记：原版 canMark()=User.canMark() && mCityType>0（WorldActionDialog.as:2427）；
    // User.canMark()（联盟官职）前端无对应字段，故仅按 citytype>0 判定。
    canMark: citytype > 0,
  }
})

// 城池可出征任务：仅渲染后端 army.dispatch 确实支持的项（当前仅 掠夺）。
const cityTaskButtons = computed(() => (cityOps.value.canPlunder ? CITY_TASKS : []))

function onFavouriteToggle(): void {
  if (favEntry.value) {
    emit('unfavourite', favEntry.value.id)
  } else if (selectedCid.value > 0) {
    emit('favourite', selectedCid.value)
  }
}

// —— 收藏备注编辑 ——
const editingId = ref(0)
const commentText = ref('')

function startEditFav(id: number, comments: string): void {
  editingId.value = id
  commentText.value = comments
}

function saveComment(): void {
  if (editingId.value > 0) {
    emit('fav-comment', editingId.value, commentText.value)
  }
  editingId.value = 0
}

function onGovern(type: number): void {
  if (!props.city) {
    return
  }
  emit('govern', {
    type,
    tcid: props.city.cid,
    tuid: props.city.uid,
    cid: props.myCid,
    cityname: props.city.cityname,
  })
}

function cellTypeName(wid: number): string {
  const c = props.cells.get(wid)
  return c ? terrainName(c.type) : '—'
}

function widCoord(wid: number): string {
  const cid = widToCid(wid)
  return `(${cid % 1000},${Math.floor(cid / 1000)})`
}

const governTypes: { type: number; name: string }[] = [
  { type: 0, name: '收税' },
  { type: 1, name: '抽丁' },
  { type: 2, name: '征粮' },
  { type: 3, name: '收编' },
  { type: 4, name: '裁军' },
]
</script>

<template>
  <SkinDialog title="世界地图" wide grow :mask-closable="false" @close="emit('close')">
    <div class="world-bar">
      <span class="label">视野</span>
      <span>{{ viewX }},{{ viewY }}</span>
      <span v-if="focusCell" class="province">
        {{ focusCoord.x + 1 }},{{ focusCoord.y + 1 }} · {{ focusCell.province }}·{{ focusCell.jun }} ·
        {{ terrainName(focusCell.type) }}
      </span>
      <span class="spacer"></span>
      <span class="label">前往</span>
      <input v-model="gotoInput" class="num" placeholder="x,y" @keyup.enter="doGoto" />
      <button class="u-btn u-btn--gold" type="button" @click="doGoto">前往</button>
      <button class="u-btn u-btn--green" type="button" :disabled="busy" @click="emit('refresh')">刷新</button>
      <button
        class="u-btn u-btn--yellow"
        type="button"
        :disabled="busy || selectedCid <= 0"
        @click="onFavouriteToggle"
      >
        {{ favEntry ? '取消收藏' : '收藏' }}
      </button>
    </div>

    <div class="world-main">
      <div class="map-area">
        <div class="map-wrap">
          <canvas
            ref="canvas"
            tabindex="0"
            @mousedown="onMouseDown"
            @mousemove="onMouseMove"
            @mouseup="onMouseUp"
            @mouseleave="onMouseLeave"
            @keydown="onKeydown"
          ></canvas>

          <!-- 原版地图边框/侧边/底部装饰 + 四角（ditu_side / ditu_bottom / ditu_b1~b4），不拦截指针 -->
          <span class="frame-edge frame-side-left" :style="{ backgroundImage: `url(${FRAME_IMGS.side})` }"></span>
          <span class="frame-edge frame-side-right" :style="{ backgroundImage: `url(${FRAME_IMGS.side})` }"></span>
          <span class="frame-edge frame-bottom" :style="{ backgroundImage: `url(${FRAME_IMGS.bottom})` }"></span>
          <span class="frame-corner tl" :style="{ backgroundImage: `url(${FRAME_IMGS.corner1})` }"></span>
          <span class="frame-corner tr" :style="{ backgroundImage: `url(${FRAME_IMGS.corner2})` }"></span>
          <span class="frame-corner br" :style="{ backgroundImage: `url(${FRAME_IMGS.corner3})` }"></span>
          <span class="frame-corner bl" :style="{ backgroundImage: `url(${FRAME_IMGS.corner4})` }"></span>

          <!-- 关闭页签（ditu_close1/2）：复用既有 close 行为 -->
          <button
            class="frame-close"
            type="button"
            title="关闭地图"
            :style="btnStyle(CLOSE_SKIN)"
            @click="emit('close')"
          ></button>

          <!-- 原版控制盘（map_controller 底图）+ 8 向三态方向按钮，点击沿用既有平移逻辑 -->
          <div class="control-pad" :style="{ backgroundImage: `url(${controllerBg})` }">
            <button
              v-for="d in DIR_BUTTONS"
              :key="d.key"
              class="dir-btn"
              :class="`dir-${d.key}`"
              type="button"
              :title="d.title"
              :style="btnStyle(d)"
              @click="pan(d.dx, d.dy)"
            ></button>
          </div>

          <!-- 功能条（map_controller1_bg 底图）：定位我的城池（复用 goto-cid）/ 移动模式（无该模式，禁用） -->
          <div class="controller-bar" :style="{ backgroundImage: `url(${controllerBarBg})` }">
            <button
              class="bar-btn mycity"
              type="button"
              title="定位我的城池"
              :disabled="myCid <= 0"
              :style="btnStyle(MYCITY_SKIN)"
              @click="onMyCity"
            ></button>
            <button
              class="bar-btn move"
              type="button"
              title="移动模式（当前版本未提供）"
              disabled
              :style="btnStyle(MOVE_SKIN)"
            ></button>
          </div>

          <!-- 左右翻页（move_to_left/right 三态）：复用 view 事件，按一屏平移 -->
          <button
            class="page-btn page-left"
            type="button"
            title="左翻一屏"
            :disabled="!canPageLeft"
            :style="pageStyle(PAGE_LEFT_SKIN, canPageLeft)"
            @click="pageLeft"
          ></button>
          <button
            class="page-btn page-right"
            type="button"
            title="右翻一屏"
            :disabled="!canPageRight"
            :style="pageStyle(PAGE_RIGHT_SKIN, canPageRight)"
            @click="pageRight"
          ></button>

          <div v-if="hoverCell" class="bubble" :style="{ left: `${hoverPos.x + 12}px`, top: `${hoverPos.y + 12}px` }">
            {{ bubbleText }}
          </div>
        </div>
        <div class="minimap">
          <img :src="minimapUrl" alt="小地图" />
          <span
            class="mini-view"
            :style="{
              left: `${(viewX / 100) * 100}%`,
              top: `${(viewY / 100) * 100}%`,
              width: `${(cols / 100) * 100}%`,
              height: `${(rows / 100) * 100}%`,
            }"
            title="当前视野"
          ></span>
        </div>
        <p class="u-hint">拖拽 / 方向键 / 上移按钮平移地图；点击地格查看目标。</p>
      </div>

      <aside class="side">
        <div class="u-tabs">
          <button class="u-tab" :class="{ 'is-active': tab === 'target' }" type="button" @click="tab = 'target'">
            目标
          </button>
          <button class="u-tab" :class="{ 'is-active': tab === 'fav' }" type="button" @click="tab = 'fav'">
            收藏
          </button>
          <button class="u-tab" :class="{ 'is-active': tab === 'fields' }" type="button" @click="tab = 'fields'">
            我的领地
          </button>
        </div>

        <p v-if="loading" class="u-hint">加载中…</p>
        <p v-else-if="error" class="u-hint is-error">{{ error }}</p>
        <p v-else-if="notice" class="u-hint is-notice">{{ notice }}</p>

        <div class="side-body u-scroll">
          <!-- 目标 -->
          <template v-if="tab === 'target'">
            <template v-if="city || field">
              <div class="u-row">
                <span class="label">坐标</span>
                <span>{{ selectedCoord.x + 1 }},{{ selectedCoord.y + 1 }}</span>
                <span class="label">{{ selectedCell?.province }}·{{ selectedCell?.jun }}</span>
              </div>

              <!-- 城池 -->
              <template v-if="city">
                <div class="u-row"><span class="label">城池</span><strong>{{ city.cityname }}</strong></div>
                <div class="u-row"><span class="label">君主</span><span>{{ city.username || '无主' }}</span></div>
                <div class="u-row">
                  <span class="label">等级</span><span>{{ cityTypeName(city.citytype) }}</span>
                  <span class="label">声望</span><span>{{ city.prestige }}</span>
                </div>
                <div class="u-row"><span class="label">所属</span><span>{{ city.unionname || '—' }}</span></div>
                <div class="u-row"><span class="label">司隶</span><span>{{ inSili ? '在司隶' : '不在司隶' }}</span></div>
                <div class="u-row"><span class="label">可入侵</span><span>{{ canInvadeText }}</span></div>

                <div class="actions">
                  <button
                    v-for="t in cityTaskButtons"
                    :key="t.task"
                    class="u-btn u-btn--red"
                    type="button"
                    :disabled="busy"
                    @click="openDispatch(t.task)"
                  >
                    {{ t.name }}
                  </button>
                  <button v-if="cityOps.canWar" class="u-btn u-btn--red" type="button" :disabled="busy" @click="onWar">
                    宣战
                  </button>
                  <button
                    v-if="cityOps.canMark"
                    class="u-btn u-btn--gold"
                    type="button"
                    :disabled="busy"
                    @click="emit('mark', city.cid)"
                  >
                    标记城池
                  </button>
                  <button
                    v-if="cityOps.canGovern"
                    class="u-btn u-btn--green"
                    type="button"
                    :disabled="busy"
                    @click="showGovern = !showGovern"
                  >
                    治理
                  </button>
                </div>

                <!-- 治理：先展示次数/上限（getGovernInfo）-->
                <div v-if="cityOps.canGovern && showGovern" class="govern">
                  <p class="u-hint">
                    {{ governInfo?.officename || '—' }} 今日已下达 {{ governInfo?.todayCount ?? 0 }} /
                    {{ governInfo?.maxCount ?? 0 }} 次
                  </p>
                  <div class="actions">
                    <button
                      v-for="g in governTypes"
                      :key="g.type"
                      class="u-btn u-btn--gold"
                      type="button"
                      :disabled="busy"
                      @click="onGovern(g.type)"
                    >
                      {{ g.name }}
                    </button>
                  </div>
                </div>

                <!-- 出征：选择武将 + 兵力（复用 army dispatch；task 由上方任务按钮决定）-->
                <div v-if="showDispatch" class="dispatch">
                  <div class="u-row">
                    <span class="label">随行武将</span>
                    <select v-model.number="heroId">
                      <option :value="0">不派遣</option>
                      <option v-for="h in heroes" :key="h.hid" :value="h.hid">
                        {{ h.name }}（Lv{{ h.level }}）
                      </option>
                    </select>
                  </div>
                  <div v-for="s in soldiers" :key="s.sid" class="u-row">
                    <span class="label soldier">{{ s.sname }}</span>
                    <input v-model.number="dispatchCounts[s.sid]" type="number" min="0" :max="s.count" class="num" />
                    <span class="u-hint">/ {{ s.count }}</span>
                  </div>
                  <div class="actions">
                    <button class="u-btn u-btn--confirm" type="button" :disabled="busy" @click="submitDispatch">
                      确认出征
                    </button>
                    <button class="u-btn u-btn--cancel" type="button" @click="showDispatch = false">取消</button>
                  </div>
                </div>
              </template>

              <!-- 野地 -->
              <template v-else-if="field">
                <div class="u-row"><span class="label">类型</span><span>{{ terrainName(field.type) }}</span></div>
                <div class="u-row"><span class="label">等级</span><span>{{ field.level }}</span></div>
                <div class="u-row"><span class="label">守备</span><span>—</span></div>
                <div class="u-row">
                  <span class="label">归属</span>
                  <span>{{ field.cityname || '无主' }}{{ field.username ? ' · ' + field.username : '' }}</span>
                </div>
                <div class="u-row"><span class="label">司隶</span><span>{{ inSili ? '在司隶' : '不在司隶' }}</span></div>
                <div class="actions">
                  <button
                    class="u-btn u-btn--gold"
                    type="button"
                    :disabled="busy || !canBuild()"
                    @click="onBuild"
                  >
                    占领/建城
                  </button>
                </div>
                <p class="u-hint">野地出征需在校场（军事）界面选择目标；此处建城仅对己方附属平地且无战乱时可用。</p>
              </template>
            </template>
            <p v-else class="u-hint">点击地图上的地格查看目标详情。</p>
          </template>

          <!-- 收藏列表 -->
          <template v-else-if="tab === 'fav'">
            <p v-if="favourites.favourites.length === 0" class="u-hint">暂无收藏目标。</p>
            <ul class="list">
              <li v-for="f in favourites.favourites" :key="f.id" class="item">
                <div class="u-row between">
                  <strong>{{ f.name }}</strong>
                  <span class="u-hint">{{ widCoord(widToCid(f.cid)) }}</span>
                </div>
                <div v-if="editingId === f.id" class="u-row">
                  <input v-model="commentText" class="grow" type="text" maxlength="60" />
                  <button class="u-btn u-btn--confirm" type="button" :disabled="busy" @click="saveComment">
                    保存
                  </button>
                </div>
                <p v-else class="u-hint">{{ f.comments || '（无备注）' }}</p>
                <div class="actions">
                  <button class="u-btn u-btn--green" type="button" @click="emit('goto-cid', f.cid)">定位</button>
                  <button class="u-btn u-btn--yellow" type="button" @click="startEditFav(f.id, f.comments)">
                    备注
                  </button>
                  <button class="u-btn u-btn--cancel" type="button" :disabled="busy" @click="emit('unfavourite', f.id)">
                    删除
                  </button>
                </div>
              </li>
            </ul>
          </template>

          <!-- 我的领地 -->
          <template v-else>
            <p v-if="userFields.length === 0" class="u-hint">暂无附属领地。</p>
            <ul v-else class="list">
              <li v-for="wid in userFields" :key="wid" class="item">
                <div class="u-row between">
                  <strong>{{ widCoord(wid) }}</strong>
                  <span class="u-hint">{{ cellTypeName(wid) }}</span>
                  <button class="u-btn u-btn--green" type="button" @click="emit('goto-wid', wid)">定位</button>
                </div>
              </li>
            </ul>
          </template>
        </div>
      </aside>
    </div>

    <template #footer>
      <button class="u-btn u-btn--cancel" type="button" @click="emit('close')">关闭</button>
    </template>
  </SkinDialog>
</template>

<style scoped>
.world-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 6px;
  font-size: 13px;
}

.world-bar .spacer {
  flex: 1;
}

.world-bar .label {
  color: var(--text-dim);
}

.world-bar .province {
  color: var(--accent);
}

.world-main {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  overflow-x: auto;
}

.map-area {
  flex: 0 0 auto;
  width: 576px;
}

.map-wrap {
  position: relative;
  width: 576px;
  border: 1px solid var(--panel-border);
  background: #141a10;
}

.map-wrap canvas {
  display: block;
  cursor: crosshair;
  outline: none;
}

.map-wrap canvas:focus {
  box-shadow: inset 0 0 0 2px var(--accent);
}

/* —— 原版地图边框（ditu_side / ditu_bottom / ditu_b1~b4）：装饰层不拦截指针 —— */
.frame-edge,
.frame-corner {
  position: absolute;
  z-index: 3;
  pointer-events: none;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
}

.frame-side-left,
.frame-side-right {
  top: 0;
  bottom: 0;
  width: 6px;
  background-repeat: repeat-y;
  background-size: 100% auto;
}

.frame-side-left {
  left: 0;
}

.frame-side-right {
  right: 0;
}

.frame-bottom {
  left: 0;
  right: 0;
  bottom: 0;
  height: 6px;
  background-repeat: repeat-x;
  background-size: auto 100%;
}

.frame-corner {
  width: 26px;
  height: 26px;
}

.frame-corner.tl {
  left: 0;
  top: 0;
}

.frame-corner.tr {
  right: 0;
  top: 0;
}

.frame-corner.bl {
  left: 0;
  bottom: 0;
}

.frame-corner.br {
  right: 0;
  bottom: 0;
}

/* 关闭页签（ditu_close1/2）：复用面板关闭行为 */
.frame-close {
  position: absolute;
  top: 6px;
  right: 10px;
  z-index: 4;
  width: 26px;
  height: 48px;
  padding: 0;
  border: none;
  background-color: transparent;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
  background-image: var(--btn-up);
  cursor: pointer;
}

.frame-close:hover {
  background-image: var(--btn-hl);
}

.frame-close:active {
  background-image: var(--btn-dn);
}

/* —— 原版控制盘（map_controller 底图）+ 8 向三态方向按钮 —— */
.control-pad {
  position: absolute;
  left: 10px;
  bottom: 10px;
  z-index: 4;
  width: 182px;
  height: 132px;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
}

.dir-btn {
  position: absolute;
  padding: 0;
  border: none;
  background-color: transparent;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
  background-image: var(--btn-up);
  cursor: pointer;
}

.dir-btn:hover {
  background-image: var(--btn-hl);
}

.dir-btn:active {
  background-image: var(--btn-dn);
}

.dir-up {
  top: 2px;
  left: 50%;
  width: 43px;
  height: 28px;
  transform: translateX(-50%);
}

.dir-down {
  bottom: 2px;
  left: 50%;
  width: 43px;
  height: 28px;
  transform: translateX(-50%);
}

.dir-left {
  left: 2px;
  top: 50%;
  width: 28px;
  height: 43px;
  transform: translateY(-50%);
}

.dir-right {
  right: 2px;
  top: 50%;
  width: 28px;
  height: 43px;
  transform: translateY(-50%);
}

.dir-upleft {
  top: 6px;
  left: 6px;
  width: 41px;
  height: 40px;
}

.dir-upright {
  top: 6px;
  right: 6px;
  width: 41px;
  height: 40px;
}

.dir-downleft {
  bottom: 6px;
  left: 6px;
  width: 41px;
  height: 40px;
}

.dir-downright {
  bottom: 6px;
  right: 6px;
  width: 41px;
  height: 40px;
}

/* —— 功能条（map_controller1_bg 底图）：我的城池 / 移动模式 —— */
.controller-bar {
  position: absolute;
  left: 10px;
  bottom: 148px;
  z-index: 4;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 150px;
  height: 28px;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
}

.bar-btn {
  padding: 0;
  border: none;
  background-color: transparent;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
  background-image: var(--btn-up);
  cursor: pointer;
}

.bar-btn:hover:enabled {
  background-image: var(--btn-hl);
}

.bar-btn:active:enabled {
  background-image: var(--btn-dn);
}

.bar-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.bar-btn.mycity {
  width: 34px;
  height: 28px;
}

.bar-btn.move {
  width: 42px;
  height: 23px;
}

/* —— 左右翻页（move_to_left/right 三态，禁用态用 *_disbaled）—— */
.page-btn {
  position: absolute;
  top: 50%;
  z-index: 4;
  width: 29px;
  height: 117px;
  padding: 0;
  border: none;
  background-color: transparent;
  background-repeat: no-repeat;
  background-position: center;
  background-size: 100% 100%;
  background-image: var(--btn-up);
  cursor: pointer;
  transform: translateY(-50%);
}

.page-btn:hover:enabled {
  background-image: var(--btn-hl);
}

.page-btn:active:enabled {
  background-image: var(--btn-dn);
}

.page-btn:disabled {
  cursor: not-allowed;
}

.page-left {
  left: 6px;
}

.page-right {
  right: 6px;
}

.bubble {
  position: absolute;
  z-index: 5;
  padding: 2px 6px;
  white-space: nowrap;
  font-size: 12px;
  color: #f5edda;
  background: rgba(12, 16, 10, 0.9);
  border: 1px solid var(--panel-border);
  border-radius: 3px;
  pointer-events: none;
}

.minimap {
  position: relative;
  width: 200px;
  height: 105px;
  margin-top: 6px;
}

.minimap img {
  display: block;
  width: 200px;
  height: 105px;
}

.mini-view {
  position: absolute;
  border: 1px solid var(--accent);
  background: rgba(224, 180, 87, 0.22);
  pointer-events: none;
}

.side {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.side-body {
  min-height: 300px;
  max-height: 420px;
}

.u-row {
  margin: 4px 0;
}

.u-row.between {
  justify-content: space-between;
}

.u-tabs {
  margin-bottom: 4px;
}

.u-hint {
  margin: 4px 0;
}

.u-hint.is-notice {
  color: var(--accent);
}

.num {
  width: 72px;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.grow {
  flex: 1 1 auto;
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.soldier {
  width: 88px;
}

select {
  padding: 4px 6px;
  color: var(--text);
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 4px;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  margin: 6px 0;
}

.govern,
.dispatch {
  margin-top: 6px;
  padding: 6px 8px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}

.list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 4px 0 0;
  padding: 0;
  list-style: none;
}

.item {
  padding: 6px 8px;
  background: var(--bg);
  border: 1px solid var(--panel-border);
  border-radius: 6px;
}
</style>
