// 美术资源访问工具。
// 原版 resources 已整体拷入 frontend/public/images/，Vite 以 /images/... 原路径发布。
// 约定：所有资源一律走本文件构造 URL，禁止散落拼接，便于统一 base 与缺失核对。

export const IMAGES_BASE = '/images'

/** 拼接资源相对路径 => 绝对 URL；避免重复 /。 */
export function img(rel: string): string {
  return rel.replace(/^\/+/, '') ? `${IMAGES_BASE}/${rel.replace(/^\/+/, '')}` : IMAGES_BASE
}

/** 武将头像：images/hero/hero_{sex}_{face}.jpg（sex: 1=男 2=女，与原版 trade 拼接一致）。 */
export function heroFace(sex: number, face: number): string {
  const gender = sex === 2 ? 'girl' : 'boy'
  return img(`hero/hero_${gender}_${face}.jpg`)
}

/** 兵种图标：images/army_{sid}.png。 */
export function soldierIcon(sid: number): string {
  return img(`army_${sid}.png`)
}

/** 兵种大图（面板卡/出征展示）：images/army_{sid}_big.png；无该图时回退到 soldierIcon。 */
export function soldierBig(sid: number): string {
  return img(`army_${sid}_big.png`)
}

/** 建筑介绍小图（内政面板用）：images/building_intro_{bid}.png。 */
export function buildingIntro(bid: number): string {
  return img(`building_intro_${bid}.png`)
}

/** 装备图标：images/armor/{id}.png。 */
export function armor(id: number): string {
  return img(`armor/${id}.png`)
}

/** 地形图：images/view_terrain_{id}.png。 */
export function terrain(id: number | string): string {
  return img(`view_terrain_${id}.png`)
}

/** 预取一组图片，先把像素/缓存加载好再绘制，避免闪烁。 */
export function preloadPaths(urls: string[], onProgress?: (loaded: number, total: number) => void): void {
  let loaded = 0
  const total = urls.length
  for (const url of urls) {
    const im = new Image()
    im.onload = im.onerror = () => {
      loaded += 1
      onProgress?.(loaded, total)
    }
    im.src = url
  }
}