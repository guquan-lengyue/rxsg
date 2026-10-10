package building

// legacy_bid.go —— legacy server/game/common.php:47-66 的 20 建筑 bid 权威常量（1:1 摘录）。
//
// ⚠ bid 映射差异（R11-1 已核实，见 docs/SWF解包-脚本与美术资源关联.md §9.1 与后端 0004 头注）：
//   原版 20 建筑 bid = {1农田 2伐木 3采石 4铁矿 5民房 6官府 7书院 8校场 9军营 10客栈
//                       11官署 12鸿胪寺 13市场 14铁匠铺 15工坊 16粮仓 17仓库 18驿站 19烽火台 20城墙}
//   重写版 DB（migrations/0004 + 0008 + 0010）使用另一套【重写 14 建筑映射】：
//   1官府 2农田 3伐木场 4采石场 5铁矿 6民居 7书院 8兵营 9仓库 10校场 11官署 12客栈 13市场 14工匠作坊。
//   两套映射不同源。本次逻辑语义（校验顺序/公式/文案）以 legacy 为准，但落库/查询统一走【重写映射】的
//   building_id 空间（与既有 building.Service.Upgrade、cfg_buildings、前端一致），由 legacyToNew 显式翻译。
//   legacy 12/14/16/18/19/20 在新库无对应建筑 → 引用这些 bid 的分支（鸿胪寺/铁匠铺/粮仓/驿站/烽火台/城墙）
//   惰性化（见 startDestroyBuildingAll 中注释）。
//
// 结论与建议见汇报：如需严格对齐原版 20 bid，须整体切换 cfg_buildings/building_id 并重导数据（本次未动数据）。

const (
	IDBuildingFarmland   = 1
	IDBuildingWood       = 2
	IDBuildingRock       = 3
	IDBuildingIron       = 4
	IDBuildingHouse      = 5
	IDBuildingGoverment  = 6
	IDBuildingCollege    = 7
	IDBuildingGround     = 8
	IDBuildingArmy       = 9
	IDBuildingHotel      = 10
	IDBuildingOffice     = 11
	IDBuildingHonglu     = 12
	IDBuildingMarket     = 13
	IDBuildingBlacksmith = 14
	IDBuildingWorkshop   = 15
	IDBuildingBarn       = 16
	IDBuildingStore      = 17
	IDBuildingDak        = 18
	IDBuildingBalefire   = 19
	IDBuildingWall       = 20
)

// legacyToNew：legacy bid → 新库 building_id（重写映射）。
// 未落库的 legacy 建筑（鸿胪寺/铁匠铺/粮仓/驿站/烽火台/城墙）不出现在此表。
var legacyToNew = map[int]int{
	IDBuildingGoverment: 1,  // 官府
	IDBuildingFarmland:  2,  // 农田
	IDBuildingWood:      3,  // 伐木场
	IDBuildingRock:      4,  // 采石场
	IDBuildingIron:      5,  // 铁矿
	IDBuildingHouse:     6,  // 民居
	IDBuildingCollege:   7,  // 书院
	IDBuildingArmy:      8,  // 兵营
	IDBuildingStore:     9,  // 仓库
	IDBuildingGround:    10, // 校场
	IDBuildingOffice:    11, // 官署
	IDBuildingHotel:     12, // 客栈
	IDBuildingMarket:    13, // 市场
	IDBuildingWorkshop:  14, // 工匠作坊
}

// newBidOf legacy bid → 新库 building_id；无对应返回 (0,false)。
func newBidOf(legacy int) (int, bool) { n, ok := legacyToNew[legacy]; return n, ok }

// LegacyToNewBid 供包外使用（city 包的城防前置条件 bid 翻译）：legacy bid → 新库 building_id。
func LegacyToNewBid(legacy int) (int, bool) { return newBidOf(legacy) }

// legacyBidOf 新库 building_id → legacy bid；无对应返回 (0,false)。
func legacyBidOf(newBid int) (int, bool) {
	for l, n := range legacyToNew {
		if n == newBid {
			return l, true
		}
	}
	return 0, false
}

// isGoverment 新库 building_id 是否对应 legacy ID_BUILDING_GOVERMENT（官府）。
func isGoverment(newBid int) bool {
	l, ok := legacyBidOf(newBid)
	return ok && l == IDBuildingGoverment
}

// isResourceField 是否资源田（legacy getBuildingMaxLevel 的 `buildingid<5`，即 legacy 1..4）。
func isResourceField(newBid int) bool {
	l, ok := legacyBidOf(newBid)
	return ok && l >= IDBuildingFarmland && l < IDBuildingHouse
}

// multiBuildingAllowed 是否可多座共存（legacy startUpgradeBuilding 唯一性排除集 {1,2,3,4,5,9,17}：
// 四种资源田 + 民房 + 军营 + 仓库）。
func multiBuildingAllowed(newBid int) bool {
	l, ok := legacyBidOf(newBid)
	if !ok {
		return false
	}
	switch l {
	case IDBuildingFarmland, IDBuildingWood, IDBuildingRock, IDBuildingIron, IDBuildingHouse, IDBuildingArmy, IDBuildingStore:
		return true
	}
	return false
}

// isMonarchBonusBid legacy `bid==8||bid==20||bid==15`（校场/城墙/工坊，君主将修为等级加成）。
// legacy 20（城墙）在新库无对应 → 仅 8/15 生效。
func isMonarchBonusBid(newBid int) bool {
	l, ok := legacyBidOf(newBid)
	if !ok {
		return false
	}
	return l == IDBuildingGround || l == IDBuildingWall || l == IDBuildingWorkshop
}
