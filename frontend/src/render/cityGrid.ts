import type { Building } from '@/types'

export interface RenderOptions {
  cellSize?: number
  /** 当前时间戳（秒），用于计算升级倒计时；默认取本地时间。 */
  now?: number
}

const COLORS = {
  grid: '#2b3a4a',
  idle: '#3a5a7a',
  upgrading: '#b8860b',
  built: '#4a7a4a',
  text: '#eaf2fb',
  countdown: '#ffd479',
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

  const cell = options.cellSize ?? 56
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

  // 背景网格
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
    const upgrading = b.state_timeleft > 0

    ctx.fillStyle = upgrading ? COLORS.upgrading : b.level > 0 ? COLORS.built : COLORS.idle
    ctx.fillRect(px + 4, py + 4, cell - 8, cell - 8)

    ctx.strokeStyle = COLORS.grid
    ctx.strokeRect(px + 4.5, py + 4.5, cell - 9, cell - 9)

    ctx.fillStyle = COLORS.text
    ctx.font = '12px "Microsoft YaHei", sans-serif'
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.fillText(`${b.bid}`, px + cell / 2, py + cell / 2 - 8)
    ctx.fillText(`Lv${b.level}`, px + cell / 2, py + cell / 2 + 8)

    if (upgrading) {
      ctx.fillStyle = COLORS.countdown
      ctx.font = '10px "Microsoft YaHei", sans-serif'
      // state_endtime 为服务端绝对时间戳，用本地时间实时倒计时
      ctx.fillText(formatLeft(b.state_endtime - now), px + cell / 2, py + cell - 8)
    }
  }
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