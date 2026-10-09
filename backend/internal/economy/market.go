package economy

// market.go 1:1 复刻 server/game/MarketFunc.php（19 函数）。
// 表映射：sys_city_trade→city_trades、sys_city_merchant→city_merchants、
// mem_city_autotrans→city_autotrans、sys_building→buildings（bid13 市场）、
// mem_city_resource→city_resources、sys_city→cities、sys_user→users、
// mem_user_buffer→user_buffers、log_merchant→log_merchants。
// 降级：mem_city_trade 镜像（cron 缓存）省略；sys_union→unionname 恒空串（M8）；
// completeTask（M8）省略。
// 原版怪癖 1:1 保留：
//   - getMerchantInfo 的 MERCHANT_*_BUY/SELL_PRICE 常量全库未 define → PHP 松散求值为常量名字符串，
//     且后四行 SELL 覆盖同名 BUY 键 → food_buy_price="MERCHANT_FOOD_SELL_PRICE" 等。
//   - logMerchantAction 形参序 ($food,$wood,$iron,$rock) 与插入列序 (wood,food,iron,rock) 不一致：
//     buyFromMerchant 实参传 (food,wood,rock,iron) → iron 列记 rock、rock 列记 iron（错位保留）。
//   - sellToUser 先算 price=$gold/$rescount 再校验 rescount<=0（除零在 PHP7 仅警告）。
//   - accelerateSell 的 endtime=(endtime-now)/10+now 为 SQL 浮点除法后落 bigint（四舍五入）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"rxsg/backend/internal/game"
	"rxsg/backend/internal/model"
)

// MarketInfo 对齐 getMarketInfo 返回（doGetBuildingInfo 建筑信息 + doGetCityTrade 挂单列表）。
type MarketInfo struct {
	Building any              `json:"building"`
	Trades   []map[string]any `json:"trades"`
}

// doGetCityTrade 对齐 MarketFunc.php:13。
func (s *Service) doGetCityTrade(ctx context.Context, cid int) ([]map[string]any, error) {
	return s.db.FetchRows(ctx, "select * from city_trades where cid=? or buycid=?", cid, cid)
}

// marketBuildingXY 取市场建筑行（legacy order by level desc limit 1）。
func (s *Service) marketBuildingXY(ctx context.Context, cid int) (x, y, level int, found bool, err error) {
	row, e := s.db.FetchOne(ctx,
		"select xy, level from buildings where city_id=? and building_id=? order by level desc limit 1",
		cid, game.BidMarket)
	if errors.Is(e, sql.ErrNoRows) {
		return 0, 0, 0, false, nil
	}
	if e != nil {
		return 0, 0, 0, false, e
	}
	x, y = model.ParseXY(row["xy"].(string))
	return x, y, modelInt(row, "level"), true, nil
}

// getMarketInfo 对齐 MarketFunc.php:18。入口先做惰性结算（进城三件套 + 交易结算）。
func (s *Service) getMarketInfo(ctx context.Context, uid, cid int) (*MarketInfo, error) {
	if err := s.settleCity(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.settleTrades(ctx); err != nil {
		return nil, err
	}
	x, y, _, found, err := s.marketBuildingXY(ctx, cid)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errLegacy("该城池尚未建造市场。") // getMarketInfo.no_market_built
	}
	d, err := s.bld.Detail(ctx, uid, cid, game.BidMarket, x, y)
	if err != nil {
		return nil, err
	}
	trades, err := s.doGetCityTrade(ctx, cid)
	if err != nil {
		return nil, err
	}
	return &MarketInfo{Building: d, Trades: trades}, nil
}

// GetMarketInfo 对外入口。
func (s *Service) GetMarketInfo(ctx context.Context, uid, cid int) (*MarketInfo, error) {
	return s.getMarketInfo(ctx, uid, cid)
}

// cancelSell 对齐 MarketFunc.php:27（restype 无 4 分支：黄金挂单不能取消）。
func (s *Service) CancelSell(ctx context.Context, uid, cid, tradeid int) (*MarketInfo, error) {
	info, err := s.db.FetchOne(ctx, "select * from city_trades where id=? and cid=? and state=0", tradeid, cid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy("已经达成的交易不能取消。") // cancelSell.cant_cancel
	} else if err != nil {
		return nil, err
	}
	count := modelInt64(info, "count")
	switch modelInt(info, "restype") {
	case 0:
		err = s.addCityResources(ctx, cid, 0, 0, 0, count, 0)
	case 1:
		err = s.addCityResources(ctx, cid, count, 0, 0, 0, 0)
	case 2:
		err = s.addCityResources(ctx, cid, 0, count, 0, 0, 0)
	case 3:
		err = s.addCityResources(ctx, cid, 0, 0, count, 0, 0)
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "delete from city_trades where id=?", tradeid); err != nil {
		return nil, err
	}
	return s.getMarketInfo(ctx, uid, cid)
}

// cancelAutoTrans 对齐 MarketFunc.php:54（state=2 的自动运输单，restype 含 4→黄金）。
func (s *Service) CancelAutoTrans(ctx context.Context, uid, cid, tradeid int) (*MarketInfo, error) {
	info, err := s.db.FetchOne(ctx, "select * from city_trades where id=? and cid=? and state=2", tradeid, cid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy("已经达成的交易不能取消。")
	} else if err != nil {
		return nil, err
	}
	count := modelInt64(info, "count")
	switch modelInt(info, "restype") {
	case 0:
		err = s.addCityResources(ctx, cid, 0, 0, 0, count, 0)
	case 1:
		err = s.addCityResources(ctx, cid, count, 0, 0, 0, 0)
	case 2:
		err = s.addCityResources(ctx, cid, 0, count, 0, 0, 0)
	case 3:
		err = s.addCityResources(ctx, cid, 0, 0, count, 0, 0)
	case 4:
		err = s.addCityResources(ctx, cid, 0, 0, 0, 0, count)
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "delete from city_trades where id=?", tradeid); err != nil {
		return nil, err
	}
	return s.getMarketInfo(ctx, uid, cid)
}

// accelerateSell 对齐 MarketFunc.php:85（木牛流马 gid=11 加速，10 倍速压缩剩余时间）。
func (s *Service) AccelerateSell(ctx context.Context, uid, cid, tradeid int) (*MarketInfo, error) {
	ok, err := s.checkGoods(ctx, uid, game.GidMuniu)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("not_enough_goods11")
	}
	trade, err := s.db.FetchOne(ctx, "select cid,buycid from city_trades where id=? and (state=1 or state=2)", tradeid)
	if errors.Is(err, sql.ErrNoRows) {
		trade = nil
	} else if err != nil {
		return nil, err
	}
	if trade == nil || (modelInt(trade, "cid") != cid && modelInt(trade, "buycid") != cid) {
		return nil, errLegacy("指定的交易不存在，不能进行加速。") // accelerateSell.trade_not_exist
	}
	if _, err := s.db.Exec(ctx,
		"update city_trades set endtime=(endtime-unix_timestamp())/10+unix_timestamp() where id=?", tradeid); err != nil {
		return nil, err
	}
	if err := s.reduceGoods(ctx, uid, game.GidMuniu, 1, 0); err != nil {
		return nil, err
	}
	return s.getMarketInfo(ctx, uid, cid)
}

// getMerchantInfo 对齐 MarketFunc.php:103。
// 原版怪癖：MERCHANT_*_BUY_PRICE / *_SELL_PRICE 常量未 define → 值为常量名字符串本身，
// 且后四行把 SELL 名赋进 *_buy_price 键（BUY 四行被覆盖）。1:1 保留。
func (s *Service) GetMerchantInfo(ctx context.Context, cid int) (map[string]any, error) {
	now := time.Now()
	today := now.Year()*10000 + int(now.Month())*100 + now.Day() // intval(date("Ymd"))
	merchant, err := s.db.FetchOne(ctx, "select * from city_merchants where city_id=? and trade_day=?", cid, today)
	if errors.Is(err, sql.ErrNoRows) {
		if _, err := s.db.Exec(ctx, `insert into city_merchants
			(city_id,food,wood,rock,iron,gold,trade_day) values (?,?,?,?,?,?,?)
			on duplicate key update food=?,wood=?,rock=?,iron=?,gold=?,trade_day=?`,
			cid, game.MerchantInitFood, game.MerchantInitWood, game.MerchantInitRock,
			game.MerchantInitIron, game.MerchantInitGold, today,
			game.MerchantInitFood, game.MerchantInitWood, game.MerchantInitRock,
			game.MerchantInitIron, game.MerchantInitGold, today); err != nil {
			return nil, err
		}
		merchant = map[string]any{
			"city_id": int64(cid), "food": int64(game.MerchantInitFood), "wood": int64(game.MerchantInitWood),
			"rock": int64(game.MerchantInitRock), "iron": int64(game.MerchantInitIron),
			"gold": int64(game.MerchantInitGold), "trade_day": int64(today),
		}
	} else if err != nil {
		return nil, err
	}
	merchant["food_buy_price"] = "MERCHANT_FOOD_SELL_PRICE"
	merchant["wood_buy_price"] = "MERCHANT_WOOD_SELL_PRICE"
	merchant["rock_buy_price"] = "MERCHANT_ROCK_SELL_PRICE"
	merchant["iron_buy_price"] = "MERCHANT_IRON_SELL_PRICE"
	return merchant, nil
}

// buyFromMerchant 对齐 MarketFunc.php:129。
func (s *Service) BuyFromMerchant(ctx context.Context, uid, cid int, food, wood, rock, iron, buyTimes, paytype int64) error {
	if paytype != 0 && paytype != 1 {
		return errLegacy("") // buyGoods.invalid_pay_type：lang 键全库未定义 → 空消息（原版行为）
	}
	if food < 0 || wood < 0 || rock < 0 || iron < 0 || (food == 0 && wood == 0 && rock == 0 && iron == 0) || buyTimes < 1 {
		return errLegacy("请输入正常的购买数量。")
	}
	addFood, addWood, addRock, addIron := food*buyTimes, wood*buyTimes, rock*buyTimes, iron*buyTimes
	goldTotal := game.FoodPrice*float64(addFood) + game.WoodPrice*float64(addWood) +
		game.RockPrice*float64(addRock) + game.IronPrice*float64(addIron)
	gold := game.FoodPrice*float64(food) + game.WoodPrice*float64(wood) +
		game.RockPrice*float64(rock) + game.IronPrice*float64(iron) // 单倍价，用于上限判断
	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if goldTotal > modelFloat(res, "gold") {
		return errLegacy("本城的黄金不够。")
	}
	level, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=?", cid, game.BidMarket)
	if err != nil {
		return err
	}
	if gold > float64(level)*100000 {
		return errLegacy(fmt.Sprintf("%d级市场和商人交易的单笔交易上限为%d00000黄金。", level, level))
	}
	need := 5 * buyTimes // MerchantServiceFee
	money, err := s.cellInt(ctx, "select money from users where id=? limit 1", uid)
	if err != nil {
		return err
	}
	gift, err := s.cellInt(ctx, "select gift from users where id=? limit 1", uid)
	if err != nil {
		return err
	}
	if paytype == 0 && money < need {
		return errLegacy("你的元宝数量不足，不能完成交易。\n请充值后再来支付。")
	}
	if paytype == 1 && gift < need {
		return errLegacy("你的礼金数量不足，不能完成交易。\n请充值后再来支付。")
	}
	if paytype == 0 {
		if err := s.addMoney(ctx, uid, -need, 50); err != nil {
			return err
		}
	} else if paytype == 1 {
		if err := s.addGift(ctx, uid, -need, 50); err != nil {
			return err
		}
	}
	if err := s.logUserAction(ctx, uid, 19); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources set gold=gold-?, food=food+?, wood=wood+?, rock=rock+?, iron=iron+? where city_id=?",
		goldTotal, addFood, addWood, addRock, addIron, cid); err != nil {
		return err
	}
	// 原版实参 (food,wood,rock,iron) 与形参 (food,wood,iron,rock) 错位 → rock/iron 流水互换，保留。
	if err := s.logMerchantAction(ctx, uid, addFood, addWood, addRock, addIron, goldTotal, 1); err != nil {
		return err
	}
	// completeTask($uid,24)：M8 省略。
	return errLegacy("**购买成功！") // 原版经 throw 返回成功文案
}

// sellToMerchant 对齐 MarketFunc.php:199。
func (s *Service) SellToMerchant(ctx context.Context, uid, cid int, food, wood, rock, iron, sellTimes, paytype int64) error {
	if paytype != 0 && paytype != 1 {
		return errLegacy("")
	}
	if food < 0 || wood < 0 || rock < 0 || iron < 0 || (food == 0 && wood == 0 && rock == 0 && iron == 0) || sellTimes < 1 {
		return errLegacy("请输入正常的出售数量。")
	}
	money, err := s.cellInt(ctx, "select money from users where id=? limit 1", uid)
	if err != nil {
		return err
	}
	gift, err := s.cellInt(ctx, "select gift from users where id=? limit 1", uid)
	if err != nil {
		return err
	}
	need := 5 * sellTimes
	// 原版顺序：元宝/礼金检查先于资源检查。
	if paytype == 0 && money < need {
		return errLegacy("你的元宝数量不足，不能完成交易。\n请充值后再来支付。")
	}
	if paytype == 1 && gift < need {
		return errLegacy("你的礼金数量不足，不能完成交易。\n请充值后再来支付。")
	}
	needFood, needWood, needRock, needIron := food*sellTimes, wood*sellTimes, rock*sellTimes, iron*sellTimes
	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if needFood > modelInt64(res, "food") {
		return errLegacy("本城的粮食不足，不能完成交易。")
	}
	if needWood > modelInt64(res, "wood") {
		return errLegacy("本城的木材不足，不能完成交易。")
	}
	if needRock > modelInt64(res, "rock") {
		return errLegacy("本城的石料不足，不能完成交易。")
	}
	if needIron > modelInt64(res, "iron") {
		return errLegacy("本城的铁锭不足，不能完成交易。")
	}
	gold := game.FoodPrice*float64(food) + game.WoodPrice*float64(wood) +
		game.RockPrice*float64(rock) + game.IronPrice*float64(iron)
	level, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=?", cid, game.BidMarket)
	if err != nil {
		return err
	}
	if gold > float64(level)*100000 {
		return errLegacy(fmt.Sprintf("%d级市场和商人交易的单笔交易上限为%d00000黄金。", level, level))
	}
	goldTotal := game.FoodPrice*float64(needFood) + game.WoodPrice*float64(needWood) +
		game.RockPrice*float64(needRock) + game.IronPrice*float64(needIron)
	if paytype == 0 {
		if err := s.addMoney(ctx, uid, -need, 51); err != nil {
			return err
		}
	} else if paytype == 1 {
		if err := s.addGift(ctx, uid, -need, 51); err != nil {
			return err
		}
	}
	if err := s.logUserAction(ctx, uid, 19); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"update city_resources set gold=gold+?, food=food-?, wood=wood-?, rock=rock-?, iron=iron-? where city_id=?",
		goldTotal, needFood, needWood, needRock, needIron, cid); err != nil {
		return err
	}
	// 原版实参 (needFood,needWood,needIron,needRock)：形参序 (food,wood,iron,rock) → 列序正确。
	if err := s.logMerchantAction(ctx, uid, needFood, needWood, needIron, needRock, goldTotal, 0); err != nil {
		return err
	}
	// completeTask($uid,23)：M8 省略。
	return errLegacy("**出售成功！")
}

// logMerchantAction 对齐 MarketFunc.php:594。
// 形参序 ($food,$wood,$iron,$rock) 与插入列序 (wood,food,iron,rock) 按下标直插：
// 列 wood←$wood、food←$food、iron←$iron、rock←$rock（调用方实参错位即由此暴露）。
func (s *Service) logMerchantAction(ctx context.Context, uid int, food, wood, iron, rock int64, gold float64, typ int) error {
	_, err := s.db.Exec(ctx,
		"insert into log_merchants (user_id, time, wood, food, iron, rock, gold) values (?,unix_timestamp(),?,?,?,?,?)",
		uid, wood, food, iron, rock, gold)
	return err
}

// getCityTradeUsing 对齐 MarketFunc.php:267。
func (s *Service) getCityTradeUsing(ctx context.Context, cid int) (int64, error) {
	return s.cellInt(ctx,
		"select count(*) from city_trades where cid=? or (buycid=? and state!=2)", cid, cid)
}

// getCityMarketLevel 对齐 MarketFunc.php:271。
func (s *Service) getCityMarketLevel(ctx context.Context, cid int) (int64, error) {
	return s.cellInt(ctx, "select level from buildings where city_id=? and building_id=? limit 1", cid, game.BidMarket)
}

// sellToUser 对齐 MarketFunc.php:275。
func (s *Service) SellToUser(ctx context.Context, uid, cid int, resType int64, rescount, gold, hour, unionsel int64) (*MarketInfo, error) {
	// 原版先算 price 再校验 rescount（除零仅警告）→ Go 浮点除 Inf，随后被 rescount<=0 拒绝。
	price := math.Round(float64(gold)/float64(rescount)*100) / 100
	if rescount <= 0 {
		return nil, errLegacy("出售数量不正确。")
	}
	if hour < 1 {
		return nil, errLegacy("交易时限不能少于1小时。")
	} else if hour > 10000 {
		hour = 10000
	}
	count, err := s.getCityTradeUsing(ctx, cid)
	if err != nil {
		return nil, err
	}
	marketLevel, err := s.getCityMarketLevel(ctx, cid)
	if err != nil {
		return nil, err
	}
	if count >= marketLevel {
		return nil, errLegacy("城内已经没有空闲商队了。")
	}
	if rescount > marketLevel*100000 {
		return nil, errLegacy(fmt.Sprintf("%d级市场的单笔交易上限为%d00000。", marketLevel, marketLevel))
	}
	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var basePrice float64
	var col, noEnough string
	switch resType {
	case 0:
		basePrice, col, noEnough = game.FoodPrice, "food", "本城的粮食不够。"
	case 1:
		basePrice, col, noEnough = game.WoodPrice, "wood", "本城的木材不够。"
	case 2:
		basePrice, col, noEnough = game.RockPrice, "rock", "本城的石料不够。"
	case 3:
		basePrice, col, noEnough = game.IronPrice, "iron", "本城的铁锭不够。"
	default:
		return nil, errLegacy("出售数量不正确。") // 原版无 else：落到不扣资源，但 restype 仅 0-3 入参
	}
	if rescount > modelInt64(res, col) {
		return nil, errLegacy(noEnough)
	}
	if price < basePrice*0.79 || price > basePrice*1.21 {
		return nil, errLegacy("出售价格超出规定范围，不能出售。")
	}
	if _, err := s.db.Exec(ctx,
		fmt.Sprintf("update city_resources set %s=%s-? where city_id=?", col, col), rescount, cid); err != nil {
		return nil, err
	}
	unionid := int64(0)
	if unionsel > 0 {
		if unionid, err = s.cellInt(ctx, "select union_id from users where id=?", uid); err != nil {
			return nil, err
		}
	}
	second := hour * 3600
	gridtime := game.GridDistance / (1.0 * game.MerchantMoveSpeed)
	distance := (float64(second) / gridtime) * (float64(second) / gridtime)
	if _, err := s.db.Exec(ctx,
		"insert into city_trades (cid,state,restype,`count`,price,gold,distance,unionid,limittime) values (?,0,?,?,?,?,?,?,?)",
		cid, resType, rescount, price, gold, distance, unionid, second); err != nil {
		return nil, err
	}
	return s.getMarketInfo(ctx, uid, cid)
}

// buyFromUser 对齐 MarketFunc.php:362。
func (s *Service) BuyFromUser(ctx context.Context, uid, cid, tradeid int) (*MarketInfo, error) {
	trade, err := s.db.FetchOne(ctx, "select * from city_trades where id=?", tradeid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errLegacy("该交易不存在。")
	} else if err != nil {
		return nil, err
	}
	if modelInt(trade, "state") == 1 {
		return nil, errLegacy("该资源已经被其他玩家抢先购买。")
	}
	tradecid := modelInt(trade, "cid")
	tradeuid, err := s.cellInt(ctx, "select user_id from cities where id=?", tradecid)
	if err != nil {
		return nil, err
	}
	if tradeuid == int64(uid) {
		return nil, errLegacy("你不能购买自己其它城池的资源。")
	}
	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if modelInt64(res, "gold") < modelInt64(trade, "gold") {
		return nil, errLegacy("本城的黄金不足。")
	}
	count, err := s.getCityTradeUsing(ctx, cid)
	if err != nil {
		return nil, err
	}
	marketLevel, err := s.getCityMarketLevel(ctx, cid)
	if err != nil {
		return nil, err
	}
	if count >= marketLevel {
		return nil, errLegacy("城内已经没有空闲商队了，请升级市场。")
	}
	currx, curry := cid%1000, cid/1000
	targx, targy := tradecid%1000, tradecid/1000
	dx2dy2 := float64((currx-targx)*(currx-targx) + (curry-targy)*(curry-targy))
	if dx2dy2 > modelFloat(trade, "distance") {
		return nil, errLegacy("城池之间的距离过远，交易失败。")
	}
	if modelInt(trade, "unionid") > 0 {
		unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
		if err != nil {
			return nil, err
		}
		if int64(modelInt(trade, "unionid")) != unionid {
			return nil, errLegacy("对方只卖给同一联盟的人，你和对方已经不在同一联盟内，不能购买。")
		}
	}
	if _, err := s.db.Exec(ctx, "update city_trades set state=1,buycid=? where id=? and state=0", cid, tradeid); err != nil {
		return nil, err
	}
	buycid, err := s.cellInt(ctx, "select buycid from city_trades where id=?", tradeid)
	if err != nil {
		return nil, err
	}
	if buycid == int64(cid) {
		if _, err := s.db.Exec(ctx, "update city_resources set gold=gold-? where city_id=?", modelInt64(trade, "gold"), cid); err != nil {
			return nil, err
		}
		needtime := cityDistance(tradecid, cid) * game.GridDistance / game.MerchantMoveSpeed
		if _, err := s.db.Exec(ctx, "update city_trades set endtime=unix_timestamp()+? where id=?", needtime, tradeid); err != nil {
			return nil, err
		}
		// completeTask(tradeuid,25)/completeTask(uid,26)：M8 省略。
	} else {
		return nil, errLegacy("该资源已经被其他玩家抢先购买。")
	}
	if err := s.logUserAction(ctx, uid, 20); err != nil {
		return nil, err
	}
	return s.getMarketInfo(ctx, uid, cid)
}

// cityDistance 对齐 utils.php:669 getCityDistance（格距离，欧氏）。
func cityDistance(cid1, cid2 int) float64 {
	x1, y1 := cid1%1000, cid1/1000
	x2, y2 := cid2%1000, cid2/1000
	return math.Sqrt(float64((x1-x2)*(x1-x2) + (y1-y2)*(y1-y2)))
}

// getUserBuyList 对齐 MarketFunc.php:436。
// 返回 [占用商队数, 市场等级, pageCount, page, rows]（原版无数据时后三项为 0,0,[]）。
func (s *Service) GetUserBuyList(ctx context.Context, uid, cid int, page, filter, unionOnly int, sellName string) (any, error) {
	using, err := s.getCityTradeUsing(ctx, cid)
	if err != nil {
		return nil, err
	}
	level, err := s.getCityMarketLevel(ctx, cid)
	if err != nil {
		return nil, err
	}
	unionid, err := s.cellInt(ctx, "select union_id from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	filterResource := ""
	if filter > 0 {
		filterResource = fmt.Sprintf("and restype = %d", filter-1)
	}
	x := cid % 1000
	y := cid / 1000
	itemCount, err := s.cellInt(ctx,
		"select count(*) from city_trades where state=0 and (unionid=0 or unionid=?) "+filterResource+
			" and distance > ((cid%1000-?)*(cid%1000-?)+(floor(cid/1000)-?)*(floor(cid/1000)-?))",
		unionid, x, x, y, y)
	if err != nil {
		return nil, err
	}
	pageCount := int64(math.Ceil(float64(itemCount) / game.MarketListCPP))
	if page >= int(pageCount) {
		page = int(pageCount) - 1
	}
	if page < 0 {
		page = 0
		pageCount = 0
	}
	ret := []any{using, level}
	if itemCount > 0 {
		pagestart := page * game.MarketListCPP
		distExpr := fmt.Sprintf("((t.cid%%1000-%d)*(t.cid%%1000-%d)+(floor(t.cid/1000)-%d)*(floor(t.cid/1000)-%d))", x, x, y, y)
		where := fmt.Sprintf("t.cid<>%d and t.state=0 and (t.unionid=0 or t.unionid=%d) %s and t.distance >= %s",
			cid, unionid, filterResource2(filter), distExpr)
		if unionOnly != 0 {
			where = fmt.Sprintf("t.cid<>%d and t.state=0 and (t.unionid=%d) %s and t.distance >= %s",
				cid, unionid, filterResource2(filter), distExpr)
		}
		if strings.TrimSpace(sellName) != "" {
			where = fmt.Sprintf("t.cid<>%d and t.state=0 and u.name=? and (t.unionid=0 or t.unionid=%d) %s and t.distance >= %s",
				cid, unionid, filterResource2(filter), distExpr)
		}
		q := "select t.*, u.name as sellername from city_trades t " +
			"left join cities c on c.id=t.cid left join users u on u.id=c.user_id " +
			"where " + where + " order by " + distExpr + fmt.Sprintf(" limit %d,%d", pagestart, game.MarketListCPP)
		var rows []map[string]any
		if strings.TrimSpace(sellName) != "" {
			rows, err = s.db.FetchRows(ctx, q, sellName)
		} else {
			rows, err = s.db.FetchRows(ctx, q)
		}
		if err != nil {
			return nil, err
		}
		// unionname（sys_union）降级：M8 不实现 → 恒空串。
		for i := range rows {
			rows[i]["unionname"] = ""
		}
		ret = append(ret, pageCount, page, rows)
	} else {
		ret = append(ret, int64(0), int64(0), []map[string]any{})
	}
	return ret, nil
}

func filterResource2(filter int) string {
	if filter > 0 {
		return fmt.Sprintf("and t.restype = %d", filter-1)
	}
	return ""
}

// GetUserSellData 对齐 MarketFunc.php:490：[占用, 等级, 官价粮木石铁]。
func (s *Service) GetUserSellData(ctx context.Context, cid int) ([]any, error) {
	using, err := s.getCityTradeUsing(ctx, cid)
	if err != nil {
		return nil, err
	}
	level, err := s.getCityMarketLevel(ctx, cid)
	if err != nil {
		return nil, err
	}
	return []any{using, level, game.FoodPrice, game.WoodPrice, game.RockPrice, game.IronPrice}, nil
}

// RemoveAutoTrans 对齐 MarketFunc.php:504。
func (s *Service) RemoveAutoTrans(ctx context.Context, id int) ([]int, error) {
	if _, err := s.db.Exec(ctx, "delete from city_autotrans where id=?", id); err != nil {
		return nil, err
	}
	return []int{id}, nil
}

// GetAutoTrans 对齐 MarketFunc.php:512。
func (s *Service) GetAutoTrans(ctx context.Context, uid int) ([]map[string]any, error) {
	return s.db.FetchRows(ctx,
		"select a.id,a.fromcid,c1.name as fromcity,a.tocid,c2.name as tocity,a.res_type,a.`count`,a.start_time "+
			"from city_autotrans a left join cities c1 on a.fromcid=c1.id left join cities c2 on a.tocid=c2.id "+
			"where a.user_id=?", uid)
}

// AddAutoTrans 对齐 MarketFunc.php:518。startMilli 为客户端毫秒时间戳。
func (s *Service) AddAutoTrans(ctx context.Context, uid int, fromcid, tocid int64, resType, count, transType int64, startMilli float64) ([]map[string]any, error) {
	startTime := int64(math.Floor(startMilli / 1000))
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	if startTime < now {
		return nil, errLegacy("时间设置错误")
	}
	ok, err := s.db.Exists(ctx, "select 1 from cities where id=? and user_id=? limit 1", tocid, uid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("目标城池必须是我方城池")
	}
	fromMaxCount, err := s.getCityMarketLevel(ctx, int(fromcid))
	if err != nil {
		return nil, err
	}
	if count > fromMaxCount*100000 {
		return nil, errLegacy("运输数量不能超过出发城池市场等级的限制")
	}
	buf, err := s.db.FetchOne(ctx, "select * from user_buffers where user_id=? and buftype=17", uid)
	if errors.Is(err, sql.ErrNoRows) {
		buf = nil
	} else if err != nil {
		return nil, err
	}
	if buf == nil {
		has, err := s.checkGoods(ctx, uid, game.GidShangduiQiyue)
		if err != nil {
			return nil, err
		}
		if !has {
			return nil, errLegacy("not_enough_goods120")
		}
		if _, err := s.db.Exec(ctx,
			"insert into user_buffers (user_id,buftype,bufparam,endtime) values (?,17,0,unix_timestamp()+259200) "+
				"on duplicate key update endtime=endtime+259200", uid); err != nil {
			return nil, err
		}
		if err := s.reduceGoods(ctx, uid, game.GidShangduiQiyue, 1, 0); err != nil {
			return nil, err
		}
	}
	valid, err := s.db.Exists(ctx,
		"select buftype from user_buffers where user_id=? and buftype=17 and endtime>=?", uid, startTime)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errLegacy("开始时间不能大于商队契约失效时间")
	}
	currx, curry := int(fromcid)%1000, int(fromcid)/1000
	targx, targy := int(tocid)%1000, int(tocid)/1000
	distance := math.Sqrt(float64((currx-targx)*(currx-targx) + (curry-targy)*(curry-targy)))
	gridtime := game.GridDistance / (1.0 * game.MerchantMoveSpeed)
	costTime := int64(math.Floor(distance * gridtime))
	endTime := startTime + costTime
	if _, err := s.db.Exec(ctx,
		"insert into city_autotrans (user_id,fromcid,tocid,state,trans_type,start_time,distance,cost_time,res_type,`count`,end_time) "+
			"values (?,?,?,0,?,?,?,?,?,?,?)",
		uid, fromcid, tocid, transType, startTime, distance, costTime, resType, count, endTime); err != nil {
		return nil, err
	}
	// completeTaskWithTaskid($uid,314)：M8 省略。
	return s.GetAutoTrans(ctx, uid)
}

// HasAutoTrans 对齐 MarketFunc.php:578：[是否有商队契约 buff]；≥10 支抛错。
func (s *Service) HasAutoTrans(ctx context.Context, uid int) ([]any, error) {
	ret := []any{}
	ok, err := s.db.Exists(ctx, "select 1 from user_buffers where user_id=? and buftype=17 limit 1", uid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return append(ret, false), nil
	}
	ret = append(ret, true)
	mcount, err := s.cellInt(ctx, "select count(*) from city_autotrans where user_id=?", uid)
	if err != nil {
		return nil, err
	}
	if mcount >= game.AutoTransMax {
		return nil, errLegacy("自动运输商队不能超过10支。")
	}
	return ret, nil
}
