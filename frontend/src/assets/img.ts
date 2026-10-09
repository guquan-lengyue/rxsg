// 美术资源访问工具。
// 原版 resources 已整体拷入 frontend/public/images/，Vite 以 /images/... 原路径发布。
// 约定：所有资源一律走本文件构造 URL，禁止散落拼接，便于统一 base 与缺失核对。

export const IMAGES_BASE = '/images'

/** 拼接资源相对路径 => 绝对 URL；避免重复 /。 */
export function img(rel: string): string {
  return rel.replace(/^\/+/, '') ? `${IMAGES_BASE}/${rel.replace(/^\/+/, '')}` : IMAGES_BASE
}

/** 武将头像：images/hero/hero_{sex}_{face}.jpg（sex: 1=男 2=女）。face<=0 为未设置占位，回退到 1。 */
export function heroFace(sex: number, face: number): string {
  const gender = sex === 2 ? 'girl' : 'boy'
  const fid = face > 0 ? face : 1
  return img(`hero/hero_${gender}_${fid}.jpg`)
}

/** 兵种图标：images/army_{sid}.png。 */
export function soldierIcon(sid: number): string {
  return img(`army_${sid}.png`)
}

/** 兵种大图（面板卡/出征展示）：images/army_{sid}_big.png；无该图时回退到 soldierIcon。 */
export function soldierBig(sid: number): string {
  return img(`army_${sid}_big.png`)
}

/** 建筑贴图：按 bid 映射到原版建筑贴图（inbuilding_/outbuilding_/building_）。
    rxsg_test 的 cfg_buildings 仅含 1-10 号核心建筑，其编号与 building_intro_N 并不一一对应；
    故改用语义明确的原版建筑贴图。未知 bid 回退到 building_intro_{bid}。 */
const BUILDING_TEXTURES: Record<number, string> = {
  1: 'building_cityhall.png', // 官府
  2: 'outbuilding_farm.png', // 农田
  3: 'outbuilding_logcamp.png', // 伐木场
  4: 'outbuilding_stonepit.png', // 采石场
  5: 'outbuilding_mine.png', // 铁矿
  6: 'inbuilding_house.png', // 民居
  7: 'inbuilding_institute.png', // 书院
  8: 'inbuilding_barrack.png', // 兵营
  9: 'inbuilding_barn.png', // 仓库（谷仓）
  10: 'inbuilding_forceyard.png', // 校场
  12: 'inbuilding_hotel.png', // 客栈（扩展槽位 12，legacy HOTEL=10 与校场冲突）
}
export function buildingIntro(bid: number): string {
  return img(BUILDING_TEXTURES[bid] ?? `building_intro_${bid}.png`)
}

/** 装备图标：images/armor/{id}.png。 */
export function armor(id: number): string {
  return img(`armor/${id}.png`)
}

/** 地形图：images/view_terrain_{id}.png。 */
export function terrain(id: number | string): string {
  return img(`view_terrain_${id}.png`)
}

/** 科技图标：images/tech_{tid}.png（tid 1..29）。 */
export function techIcon(tid: number): string {
  return img(`tech_${tid}.png`)
}

/** 资源图标：粮/木/石/铁用 images/resource_*.png；金银与人口用替代图。 */
export function resIcon(key: string): string {
  const map: Record<string, string> = {
    wood: 'resource_wood.png',
    rock: 'resource_rock.png',
    iron: 'resource_iron.png',
    food: 'resource_food.png',
    gold: 'city_gold.png',
    people: 'city_population.png',
    morale: 'city_popularity.png',
  }
  return img(map[key] ?? 'resource_wood.png')
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