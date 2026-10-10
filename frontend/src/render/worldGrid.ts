// 世界地图栅格 Canvas 渲染器（沿用 cityGrid.ts 的 high-DPI / 贴图缓存 / 命中测试范式）。
//
// 坐标体系（逐字移植 legacy utils.php cid2wid/wid2cid 与 getBlockData 的分块语义）：
//   地图为 100×100 格；x,y ∈ [0,99]。
//   wid(cid2wid)  = floor(y/10)*10000 + floor(x/10)*100 + (y%10)*10 + (x%10)
//   cid(wid2cid)  = ((floor(wid/10000)*10 + floor((wid%100)/10))*1000) + (floor((wid%10000)/100)*10 + wid%10)
//   block(x,y)    = floor(y/10)*100 + floor(x/10)  （getBlockData 以 block*100 为 blockstart）
//   wid - blockstart = (y%10)*10 + (x%10)          （group_concat 串首字段即此偏移）
import type { WorldCell } from '@/api/world'
import { img } from '@/assets/img'

/** 地图边长（格）。 */
export const WORLD_SIZE = 100
/** 单格像素尺寸（地形贴图原生 64×80，按 4:5 等比展示）。 */
export const CELL_W = 48
export const CELL_H = 60

export interface WorldView {
  x: number
  y: number
}

export interface WorldDrawOptions {
  cells: Map<number, WorldCell>
  view: WorldView
  cols: number
  rows: number
  selectedWid?: number
  hoverWid?: number
  /** 我的领地（getUserFields 的 wid）。 */
  ownWids?: Set<number>
  /** 特殊活动（getActionField 的 wid）。 */
  actionWids?: Set<number>
  /** 联盟标记（getBlockData marks 的 wid）。 */
  markWids?: Set<number>
}

const COLORS = {
  line: 'rgba(20,28,18,0.35)',
  war: 'rgba(196,32,32,0.34)',
  own: '#5fd06a',
  action: '#e0a020',
  mark: '#d9534f',
  selected: '#ffe066',
  hover: 'rgba(255,255,255,0.9)',
  empty: '#26301f',
}

// 地形贴图：mem_world.type → view_terrain_*（cfg_world_type 语义）。
const TERRAIN: Record<number, string> = {
  1: 'view_terrain_land.png', // 平原 / 可筑城平地
  2: 'view_terrain_forest.png', // 森林
  3: 'view_terrain_hill.png', // 山地
  4: 'view_terrain_lake.png', // 湖泊
  5: 'view_terrain_desert.png', // 沙漠
}
const TERRAIN_DEFAULT = 'view_terrain_grass.png'
const CITY_SPRITE = 'terrain_city.png'
const FLAG_SPRITE = 'flag_red.png'

const LEVEL_BADGE_W = 19
const LEVEL_BADGE_H = 14

// 贴图缓存：首次引用异步加载，加载完成后由 WorldMapPanel 的重绘拾取。
const imgCache = new Map<string, HTMLImageElement>()
function sprite(url: string): HTMLImageElement | undefined {
  const hit = imgCache.get(url)
  if (hit) {
    return hit.complete && hit.naturalWidth > 0 ? hit : undefined
  }
  const im = new Image()
  im.src = url
  imgCache.set(url, im)
  return undefined
}

// —— 坐标换算 ——

export function cidToWid(cid: number): number {
  const y = Math.floor(cid / 1000)
  const x = cid % 1000
  return Math.floor(y / 10) * 10000 + Math.floor(x / 10) * 100 + (y % 10) * 10 + (x % 10)
}

export function widToCid(wid: number): number {
  const y = Math.floor(wid / 10000) * 10 + Math.floor((wid % 100) / 10)
  const x = Math.floor((wid % 10000) / 100) * 10 + (wid % 10)
  return y * 1000 + x
}

/** wid → 地图格坐标。 */
export function cellOfWid(wid: number): WorldView {
  const cid = widToCid(wid)
  return { x: cid % 1000, y: Math.floor(cid / 1000) }
}

/** 地图格坐标 → wid。 */
export function widOf(x: number, y: number): number {
  return Math.floor(y / 10) * 10000 + Math.floor(x / 10) * 100 + (y % 10) * 10 + (x % 10)
}

/** 地图格坐标 → block 索引。 */
export function blockOf(x: number, y: number): number {
  return Math.floor(y / 10) * 100 + Math.floor(x / 10)
}

/** 视野覆盖的全部 block（去重、升序）。 */
export function blocksInView(view: WorldView, cols: number, rows: number): number[] {
  const set = new Set<number>()
  for (let cx = 0; cx < cols; cx += 1) {
    for (let cy = 0; cy < rows; cy += 1) {
      const x = view.x + cx
      const y = view.y + cy
      if (x < 0 || y < 0 || x >= WORLD_SIZE || y >= WORLD_SIZE) {
        continue
      }
      set.add(blockOf(x, y))
    }
  }
  return [...set].sort((a, b) => a - b)
}

/** 命中测试：画布像素坐标 → wid（越界返回 0）。 */
export function widAt(
  px: number,
  py: number,
  view: WorldView,
  cellW: number = CELL_W,
  cellH: number = CELL_H,
): number {
  const cx = Math.floor(px / cellW)
  const cy = Math.floor(py / cellH)
  const x = view.x + cx
  const y = view.y + cy
  if (x < 0 || y < 0 || x >= WORLD_SIZE || y >= WORLD_SIZE) {
    return 0
  }
  return widOf(x, y)
}

function terrainUrl(type: number): string {
  return img(TERRAIN[type] ?? TERRAIN_DEFAULT)
}

// —— 渲染 ——

export function drawWorldGrid(canvas: HTMLCanvasElement, options: WorldDrawOptions): void {
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    return
  }
  const { view, cols, rows, cells } = options
  const width = cols * CELL_W
  const height = rows * CELL_H

  const dpr = window.devicePixelRatio || 1
  canvas.width = Math.round(width * dpr)
  canvas.height = Math.round(height * dpr)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)

  // 底：空白地块色
  ctx.fillStyle = COLORS.empty
  ctx.fillRect(0, 0, width, height)

  const flag = sprite(img(FLAG_SPRITE))
  const city = sprite(img(CITY_SPRITE))

  for (let cy = 0; cy < rows; cy += 1) {
    for (let cx = 0; cx < cols; cx += 1) {
      const x = view.x + cx
      const y = view.y + cy
      if (x < 0 || y < 0 || x >= WORLD_SIZE || y >= WORLD_SIZE) {
        continue
      }
      const px = cx * CELL_W
      const py = cy * CELL_H
      const wid = widOf(x, y)
      const cell = cells.get(wid)

      // 地形 / 城池贴图
      const url = cell && cell.type === 0 ? img(CITY_SPRITE) : terrainUrl(cell?.type ?? 0)
      const sp = cell && cell.type === 0 ? city : sprite(url)
      if (sp) {
        ctx.drawImage(sp, px, py, CELL_W, CELL_H)
      }

      // 战乱态：红色蒙版
      if (cell && cell.state === 1) {
        ctx.fillStyle = COLORS.war
        ctx.fillRect(px, py, CELL_W, CELL_H)
      }

      // 归属旗：有主城池（type=0 且 ownercid>0）
      if (cell && cell.type === 0 && cell.ownercid > 0 && flag) {
        ctx.drawImage(flag, px + 2, py + 2, flag.naturalWidth, flag.naturalHeight)
      }

      // 等级角标（野地/城池）
      if (cell && cell.level > 0) {
        const lv = sprite(img(`level_${cell.level}.png`))
        if (lv) {
          ctx.drawImage(lv, px + CELL_W - LEVEL_BADGE_W - 2, py + CELL_H - LEVEL_BADGE_H - 2, LEVEL_BADGE_W, LEVEL_BADGE_H)
        }
      }

      // 高亮层
      if (options.ownWids?.has(wid)) {
        strokeCell(ctx, px, py, COLORS.own, 2)
      }
      if (options.actionWids?.has(wid)) {
        strokeCell(ctx, px, py, COLORS.action, 2)
      }
      if (options.markWids?.has(wid)) {
        strokeCell(ctx, px, py, COLORS.mark, 2)
      }
    }
  }

  // 网格线
  ctx.strokeStyle = COLORS.line
  ctx.lineWidth = 1
  for (let cx = 0; cx <= cols; cx += 1) {
    ctx.beginPath()
    ctx.moveTo(cx * CELL_W + 0.5, 0)
    ctx.lineTo(cx * CELL_W + 0.5, height)
    ctx.stroke()
  }
  for (let cy = 0; cy <= rows; cy += 1) {
    ctx.beginPath()
    ctx.moveTo(0, cy * CELL_H + 0.5)
    ctx.lineTo(width, cy * CELL_H + 0.5)
    ctx.stroke()
  }

  // 选中 / 悬停边框
  if (options.hoverWid) {
    const h = cellOfWid(options.hoverWid)
    strokeAt(ctx, h.x - view.x, h.y - view.y, COLORS.hover, 2)
  }
  if (options.selectedWid) {
    const s = cellOfWid(options.selectedWid)
    strokeAt(ctx, s.x - view.x, s.y - view.y, COLORS.selected, 3)
  }
}

function strokeCell(ctx: CanvasRenderingContext2D, px: number, py: number, color: string, lw: number): void {
  ctx.strokeStyle = color
  ctx.lineWidth = lw
  ctx.strokeRect(px + lw / 2, py + lw / 2, CELL_W - lw, CELL_H - lw)
}

function strokeAt(ctx: CanvasRenderingContext2D, cx: number, cy: number, color: string, lw: number): void {
  if (cx < 0 || cy < 0) {
    return
  }
  strokeCell(ctx, cx * CELL_W, cy * CELL_H, color, lw)
}
