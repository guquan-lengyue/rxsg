package game

// 全局数值常量，1:1 对齐 legacy server/game/common.php:73-99。
const (
	GlobalTaxRate  = 1.0  // GLOBAL_TAX_RATE
	GlobalGoldRate = 1.0  // GLOBAL_GOLD_RATE
	GlobalFoodRate = 10.0 // GLOBAL_FOOD_RATE
	GlobalWoodRate = 10.0 // GLOBAL_WOOD_RATE
	GlobalRockRate = 5.0  // GLOBAL_ROCK_RATE
	GlobalIronRate = 4.0  // GLOBAL_IRON_RATE

	FoodPrice   = 0.1  // FOOD_PRICE
	WoodPrice   = 0.1  // WOOD_PRICE
	RockPrice   = 0.2  // ROCK_PRICE
	IronPrice   = 0.25 // IRON_PRICE
	HeroExpRate = 0.1  // HERO_EXP_RATE

	MerchantMoveSpeed = 360  // MERCHANT_MOVE_SPEED（common.php:44）
	GridDistance      = 6000 // GRID_DISTANCE（common.php:30；注意旧文档误记 60000，以代码为准）

	MerchantInitFood = 100 // MERCHANT_INIT_FOOD
	MerchantInitWood = 100
	MerchantInitRock = 100
	MerchantInitIron = 100
	MerchantInitGold = 10000

	MarketListCPP = 10 // MARKET_LIST_CPP 每页交易数
)

// 科技 ID（common.php:68-71，legacy ID_TECHNIC_*；新库 cfg_technics.tid 同值）。
const (
	TechnicFood   = 1
	TechnicWood   = 2
	TechnicRock   = 3
	TechnicIron   = 4
	TechnicLeader = 6 // 统帅能力，影响城守产出加成
)

// 建筑 bid（重写版映射，见 0004/0010 种子注释；legacy→新：
// 农田1→2、伐木2→3、采石3→4、铁矿4→5、民居5→6、官府6→1、市场13→13、
// 工匠作坊15→14、仓库17→9；新库另有 11官署/12客栈，M6 不涉及）。
const (
	BidGoverment = 1  // legacy ID_BUILDING_GOVERMENT=6
	BidFarmland  = 2  // legacy ID_BUILDING_FARMLAND=1
	BidWood      = 3  // legacy ID_BUILDING_WOOD=2
	BidRock      = 4  // legacy ID_BUILDING_ROCK=3
	BidIron      = 5  // legacy ID_BUILDING_IRON=4
	BidHouse     = 6  // legacy ID_BUILDING_HOUSE=5
	BidStore     = 9  // legacy ID_BUILDING_STORE=17
	BidMarket    = 13 // legacy ID_BUILDING_MARKET=13
	BidWorkshop  = 14 // legacy 工匠作坊 bid=15 → 新库 14
)

// M6 官市/商城相关 gid 与费用常量（legacy 硬编码值）。
const (
	GidCopper            = 152    // 铜钱
	GidWuzhuqian         = 10960  // 五铢钱
	GidPoint             = 888888 // 积分
	GidShangduiQiyue     = 120    // 商队契约
	GidMuniu             = 11     // 木牛流马（加速交易）
	MerchantServiceFee   = 5      // buyFromMerchant/sellToMerchant 每次交易 5 元宝/礼金
	WorkshopRefreshLimit = 50     // 每日刷新上限
	WorkshopCooldown     = 7200
	AutoTransMax         = 10
)
