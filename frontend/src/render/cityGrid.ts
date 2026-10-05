import type { Building } from '@/types'
import { buildingIntro, img } from '@/assets/img'

export interface RenderOptions {
  cellSize?: number
  /** 当前时间戳（秒），用于计算升级倒计时；默认取本地时间。 */
  now?: number
}

// 单格像素尺寸，BuildingGrid 点击命中测试与绘制保持一致。
export const CELL_SIZE = 56

const COLORS = {
  grid: '#1f2a22',
  idle: '#31413a',
  upgrading: '#b8860b',
  built: '#3f5f3f',
  countdown: '#ffd479',
}

// 等级角标贴图原生尺寸（原版 BuildingGrid.levelImage 为 19×14），见 tools/export/BloodWar BuildingGrid.as
const LEVEL_BADGE_W = 19
const LEVEL_BADGE_H = 14

const GROUND = img('block_ground.png')

// 贴图缓存：首次引用时异步加载，加载完成后由 BuildingGrid 的 1s 定时重绘拾取。
const imgCache = new Map<string, HTMLImageElement>()
function sprite(url: string): HTMLImageElement | undefined {
  const hit = imgCache.get(url)
  if (hit) return hit.complete && hit.naturalWidth > 0 ? hit : undefined
  const im = new Image()
  im.src = url
  imgCache.set(url, im)
  return undefined
}

// 原生 Canvas 2D 绘制城内网格：按 sys_building.x/y/bid/level/state 布局。
// 后续大地图/战斗再引入 PixiJS/WebGL。
export function drawCityGrid(
  canvas: HTMLCanvasElement,
  buildings: Building[],
  options: RenderOptions = {},
): void {
  const ctx = canvas.getContext('2d')
  if (!ctx) {
    return
  }

  const cell = options.cellSize ?? CELL_SIZE
  const now = options.now ?? Math.floor(Date.now() / 1000)

  const maxX = buildings.reduce((m, b) => Math.max(m, b.x), 0)
  const maxY = buildings.reduce((m, b) => Math.max(m, b.y), 0)
  const cols = Math.max(1, maxX + 1)
  const rows = Math.max(1, maxY + 1)
  const width = cols * cell
  const height = rows * cell

  const dpr = window.devicePixelRatio || 1
  canvas.width = Math.round(width * dpr)
  canvas.height = Math.round(height * dpr)
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  ctx.clearRect(0, 0, width, height)

  // 地面：有 ground 贴图则平铺，否则纯色
  const groundImg = sprite(GROUND)
  if (groundImg) {
    const pat = ctx.createPattern(groundImg, 'repeat')
    ctx.fillStyle = pat ?? '#2a3a2c'
  } else {
    ctx.fillStyle = '#2a3a2c'
  }
  ctx.fillRect(0, 0, width, height)

  // 网格线
  ctx.strokeStyle = COLORS.grid
  ctx.lineWidth = 1
  for (let x = 0; x <= cols; x += 1) {
    ctx.beginPath()
    ctx.moveTo(x * cell + 0.5, 0)
    ctx.lineTo(x * cell + 0.5, height)
    ctx.stroke()
  }
  for (let y = 0; y <= rows; y += 1) {
    ctx.beginPath()
    ctx.moveTo(0, y * cell + 0.5)
    ctx.lineTo(width, y * cell + 0.5)
    ctx.stroke()
  }

  for (const b of buildings) {
    const px = b.x * cell
    const py = b.y * cell
    const upgrading = b.state === 1 || b.state_timeleft > 0

    // 格底
    ctx.fillStyle = upgrading ? COLORS.upgrading : b.level > 0 ? COLORS.built : COLORS.idle
    ctx.fillRect(px + 3, py + 3, cell - 6, cell - 6)

    // 建筑贴图（building_intro_{bid}.png），缺失时回退为格底色块
    const sp = sprite(buildingIntro(b.bid))
    if (sp) {
      const pad = 4
      const iw = cell - pad * 2
      const ih = iw * (sp.naturalHeight / Math.max(1, sp.naturalWidth))
      const dispW = Math.min(iw, cell - pad)
      const dispH = Math.min(ih, cell - pad)
      const iy = py + cell - pad - dispH
      ctx.save()
      ctx.beginPath()
      ctx.roundRect(px + 3, py + 3, cell - 6, cell - 6, 4)
      ctx.clip()
      ctx.drawImage(sp, px + pad, iy, dispW, dispH)
      if (upgrading) {
        ctx.fillStyle = 'rgba(184,134,11,0.35)'
        ctx.fillRect(px + 3, py + 3, cell - 6, cell - 6)
      }
      ctx.restore()
    }

    ctx.strokeStyle = COLORS.grid
    ctx.strokeRect(px + 3.5, py + 3.5, cell - 7, cell - 7)

    // 等级角标：用原版 images/level_{n}.png 贴图（右下角），替代手绘 Lv 文本
    if (b.level > 0) {
      const lv = sprite(img(`level_${b.level}.png`))
      if (lv) {
        ctx.drawImage(lv, px + cell - LEVEL_BADGE_W - 3, py + cell - LEVEL_BADGE_H - 3, LEVEL_BADGE_W, LEVEL_BADGE_H)
      }
    }

    if (upgrading) {
      ctx.font = '9px "Microsoft YaHei", sans-serif'
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.fillStyle = 'rgba(0,0,0,0.75)'
      ctx.fillText(formatLeft(b.state_endtime - now), px + cell / 2 + 1, py + cell / 2 + 1)
      ctx.fillStyle = COLORS.countdown
      ctx.fillText(formatLeft(b.state_endtime - now), px + cell / 2, py + cell / 2)
    }
  }
}

// 命中测试：把画布内的像素坐标换算为格子并返回该格建筑。
export function buildingAt(
  buildings: Building[],
  px: number,
  py: number,
  cell: number = CELL_SIZE,
): Building | undefined {
  const x = Math.floor(px / cell)
  const y = Math.floor(py / cell)
  return buildings.find((b) => b.x === x && b.y === y)
}

// 把剩余秒数格式化为 mm:ss / hh:mm:ss
export function formatLeft(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  const pad = (n: number) => n.toString().padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(sec)}` : `${pad(m)}:${pad(sec)}`
}