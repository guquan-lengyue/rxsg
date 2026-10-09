package economy

// shop.go 1:1 复刻 server/game/ShopFunc.php（商城/礼券）+ GoodsFunc.sellGoods +
// CityFunc.setCityProductRate + utils.parseAndAddReward。
// 表映射：cfg_shop→cfg_shops、cfg_goods_copper→cfg_goods_copper、log_shop→log_shops、
// log_shop_buy_cnt→log_shop_buy_cnts、sys_ticket→tickets、sys_ticket_content→ticket_contents、
// sys_user→users、sys_goods→user_goods。
// 降级（新库缺表/外部服务）：
//   - cfg_things 无 → getShopInfo 的五铢/积分"物品"列表恒空、parseAndAddReward type2 返回空行、
//     isChiBiGoods 恒 false（cfg_things 联查）。
//   - cfg_hero 无 → getGoodsHeroAttr 恒抛"没有属性提示"。
//   - checkAndDoShopAct（M9 商城活动）→ 恒 false；addChibiGoods/addMenghuoGood → 走普通道具路径。
//   - completeTask（M8）省略。
// 原版怪癖保留：
//   - buyGoods 传 gid 时 `select *` 赋给 cell 变量 → 实际取首列 id，且不过滤时间。
//   - buyGoods 的 reach_remain_amount_todayLimit 第二参数误用 $buycnt（个人限购计数）而非 $todaybuycnt。
//   - buyGoods 重复 update last_pay 两次。
//   - exchangeLiquan 的 preg_match "/[a-zA-Z0-9]{10}/" 无锚定 → 任意位置含 10 位字母数字即通过。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"rxsg/backend/internal/game"
)

// GetShopInfo 对齐 ShopFunc.php:7。
// 返回 [五铢钱数, 积分数, 常规商品(commend 0/2), 推荐商品(commend 1/2), 五铢钱商品, 积分商品]。
func (s *Service) GetShopInfo(ctx context.Context, uid int) ([]any, error) {
	wuzhu, err := s.goodsCount(ctx, uid, game.GidWuzhuqian)
	if err != nil {
		return nil, err
	}
	point, err := s.goodsCount(ctx, uid, game.GidPoint)
	if err != nil {
		return nil, err
	}
	baseWhere := "onsale=1 and starttime<=unix_timestamp() and endtime>unix_timestamp() and `group` not in (6,7) and (gid<160001 or gid>160051)"
	goods0, err := s.db.FetchRows(ctx, "SELECT * FROM cfg_shops WHERE "+baseWhere+" and (commend='0' or commend='2') ORDER BY position,id")
	if err != nil {
		return nil, err
	}
	goods1, err := s.db.FetchRows(ctx, "SELECT * FROM cfg_shops WHERE "+baseWhere+" and (commend='1' or commend='2') ORDER BY position,id")
	if err != nil {
		return nil, err
	}
	// 五铢钱/积分商城：cfg_goods_copper（type0=cfg_goods / type1=cfg_things 缺失→空）。
	copper := func(commend string) ([]map[string]any, error) {
		g, err := s.db.FetchRows(ctx,
			"select g.*,c.price,c.type from cfg_goods_copper c left join cfg_goods g on c.gid=g.gid "+
				"where c.type='0' and c.onsale='1' and (c.commend='"+commend+"' or c.commend=2)")
		if err != nil {
			return nil, err
		}
		// cfg_things 缺失 → wuZhuThings/copperThings 恒空（array_merge 无追加）。
		return g, nil
	}
	wuzhuGoods, err := copper("1")
	if err != nil {
		return nil, err
	}
	copperGoods, err := copper("0")
	if err != nil {
		return nil, err
	}
	// completeTask($uid,541)：M8 省略。
	return []any{wuzhu, point, goods0, goods1, wuzhuGoods, copperGoods}, nil
}

// GetGoodsHeroAttr 对齐 ShopFunc.php:25。cfg_hero 表缺失 → 恒抛"没有属性提示"。
func (s *Service) GetGoodsHeroAttr(ctx context.Context, hid int) error {
	return errLegacy("没有属性提示") // buyGoods.no_tip（原版 empty($hinfo) 分支）
}

// GetGoodsArmorAttr 对齐 ShopFunc.php:36。
func (s *Service) GetGoodsArmorAttr(ctx context.Context, aid int) ([]any, error) {
	info, err := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", aid)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(info) == 0) {
		return nil, errLegacy("没有属性提示")
	} else if err != nil {
		return nil, err
	}
	return []any{info}, nil
}

// isMenghuoGoods 对齐 ShopFunc.php:147。
func isMenghuoGoods(id int) bool { return id >= 41001 && id <= 41009 }

// isChiBiGoods 对齐 ShopFunc.php:131。cfg_things 缺失 → 联查恒空 → false。
func (s *Service) isChiBiGoods(ctx context.Context, gid int) bool { return false }

// BuyGoods 对齐 ShopFunc.php:155。
func (s *Service) BuyGoods(ctx context.Context, uid, id, cnt, paytype, gidArg int) ([]any, error) {
	if paytype != 0 && paytype != 1 {
		return nil, errLegacy("") // buyGoods.invalid_pay_type：lang 键未定义 → 空消息
	}
	if cnt < 1 {
		return nil, errLegacy("购买数量无效。")
	}
	var goods map[string]any
	var err error
	if gidArg != 0 {
		// 原版怪癖：sql_fetch_one_cell("select * ...") 取首列 id。
		idCell, e := s.cellInt(ctx, "select * from cfg_shops where gid=? and onsale=1", gidArg)
		if e != nil {
			return nil, e
		}
		if idCell != 0 {
			id = int(idCell)
		}
		goods, e = s.db.FetchOne(ctx,
			"select * from cfg_shops where gid=? and onsale=1 and starttime<=unix_timestamp() and endtime>unix_timestamp()", gidArg)
		err = e
	} else {
		goods, err = s.db.FetchOne(ctx,
			"select * from cfg_shops where id=? and onsale=1 and starttime<=unix_timestamp() and endtime>unix_timestamp()", id)
	}
	if errors.Is(err, sql.ErrNoRows) {
		goods = nil
	} else if err != nil {
		return nil, err
	}
	if len(goods) == 0 {
		return nil, errLegacy("此商品已经停售。")
	}
	if modelInt(goods, "rebate") == 1 && paytype == 1 {
		return nil, errLegacy("特价商品不可用礼金购买")
	}
	moneyNeed := int64(cnt) * modelInt64(goods, "price")
	userInfo, err := s.db.FetchOne(ctx, "select nobility, money, gift from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	// 推恩（getBufferNobility）
	nobility, err := s.getBufNobility(ctx, uid, modelFloat(userInfo, "nobility"))
	if err != nil {
		return nil, err
	}
	userMoney := modelInt64(userInfo, "money")
	userGift := modelInt64(userInfo, "gift")
	if paytype == 0 && userMoney < moneyNeed {
		return nil, errLegacy("你的元宝不足，请充值。")
	}
	if paytype == 1 && userGift < moneyNeed {
		return nil, errLegacy("你的礼金不足。")
	}
	if id == 121 && nobility < 1 {
		return nil, errLegacy("只有爵位达到“公士”才能购买和使用“聚贤包”。")
	}
	totalCount := modelInt(goods, "totalCount")
	if totalCount < 2000000000 && totalCount == 0 {
		return nil, errLegacy("此商品已经卖光了。")
	}
	userbuycnt := modelInt(goods, "userbuycnt")
	daybuycnt := modelInt(goods, "daybuycnt")
	if userbuycnt > 0 || daybuycnt > 0 || totalCount < 2000000000 { // 限制商品
		rebate := false
		rebateStartTime := int64(0)
		if modelInt(goods, "rebate") > 0 {
			rebate = true
			rebateStartTime = modelInt64(goods, "starttime")
		}
		var totalBuycnt int64
		if rebate {
			totalBuycnt, err = s.cellInt(ctx, "select sum(`count`) from log_shops where shopid=? and time>=?", id, rebateStartTime)
		} else {
			totalBuycnt, err = s.cellInt(ctx, "select sum(`count`) from log_shop_buy_cnts where sid=?", id)
		}
		if err != nil {
			return nil, err
		}
		if totalCount < 2000000000 && totalBuycnt+int64(cnt) > int64(totalCount) {
			validnum := int64(totalCount) - totalBuycnt
			if validnum > 0 {
				return nil, errLegacy(fmt.Sprintf("购买数量无效。此商品全服限量出售%d个，已经售出%d个，您只能再购买%d个此商品。",
					totalCount, totalBuycnt, validnum))
			}
			return nil, errLegacy("此商品已经卖光了。")
		}
		var buycnt int64
		if rebate {
			buycnt, err = s.cellInt(ctx, "select sum(`count`) from log_shops where user_id=? and shopid=? and time>=?", uid, id, rebateStartTime)
		} else {
			buycnt, err = s.cellInt(ctx, "select `count` from log_shop_buy_cnts where user_id=? and sid=?", uid, id)
		}
		if err != nil {
			return nil, err
		}
		if userbuycnt > 0 && buycnt+int64(cnt) > int64(userbuycnt) {
			if int64(userbuycnt) > buycnt {
				remain := int64(userbuycnt) - buycnt
				return nil, errLegacy(fmt.Sprintf("购买数量无效。此商品每人限购%d个，你已经购买%d个，只能再购买%d个此商品。",
					userbuycnt, buycnt, remain))
			}
			return nil, errLegacy(fmt.Sprintf("此商品每人限购%d个，你已经达到购买限制，不能再购买更多此商品。", userbuycnt))
		}
		todaybuycnt, err := s.cellInt(ctx,
			"select sum(`count`) from log_shops where user_id=? and shopid=? and date(now())=date(from_unixtime(time))", uid, id)
		if err != nil {
			return nil, err
		}
		if daybuycnt > 0 && todaybuycnt+int64(cnt) > int64(daybuycnt) {
			if int64(daybuycnt) > todaybuycnt {
				remain := int64(daybuycnt) - todaybuycnt
				// 原版怪癖：第二参数误用 $buycnt（个人累计）而非 $todaybuycnt。
				return nil, errLegacy(fmt.Sprintf("购买数量无效。此商品每人每天限购%d个，你今天已经购买%d个，只能再购买%d个此商品。",
					daybuycnt, buycnt, remain))
			}
			return nil, errLegacy(fmt.Sprintf("此商品每人每天限购%d个，你今天已经达到购买限制，请明天再来购买此商品。", daybuycnt))
		}
		if _, err := s.db.Exec(ctx,
			"insert into log_shop_buy_cnts (user_id, sid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
			uid, id, cnt, cnt); err != nil {
			return nil, err
		}
	}
	logGoodType := 2
	if paytype == 0 {
		logGoodType = 61
	} else {
		logGoodType = 62
	}
	if s.isChiBiGoods(ctx, id) {
		// addChibiGoods：M9 远程赤壁 → 省略（降级）。
	} else if isMenghuoGoods(id) {
		// addMenghuoGood：M9 孟获 → 省略（降级）。
	} else {
		if err := s.addGoods(ctx, uid, modelInt(goods, "gid"), int64(modelInt(goods, "pack")*cnt), logGoodType); err != nil {
			return nil, err
		}
	}
	if paytype == 0 {
		if err := s.addMoney(ctx, uid, -moneyNeed, 10); err != nil {
			return nil, err
		}
	} else if paytype == 1 {
		if err := s.addGift(ctx, uid, -moneyNeed, 10); err != nil {
			return nil, err
		}
	}
	if _, err := s.db.Exec(ctx, "update users set last_pay=? where id=?", paytype, uid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_shops (user_id, shopid, `count`, price, time, paytype) values (?,?,?,?,unix_timestamp(),?)",
		uid, id, cnt, modelInt64(goods, "price"), paytype); err != nil {
		return nil, err
	}
	// completeTask 366/531、completeTaskWithTaskid 294：M8 省略。
	ret := []any{paytype}
	if paytype == 0 {
		ret = append(ret, userMoney-moneyNeed)
		// checkAndDoShopAct（M9 商城活动）→ 恒 false，不追加 actMsg。
	} else if paytype == 1 {
		ret = append(ret, userGift-moneyNeed)
	}
	// 原版第二次 update last_pay（重复语句）。
	if _, err := s.db.Exec(ctx, "update users set last_pay=? where id=?", paytype, uid); err != nil {
		return nil, err
	}
	return ret, nil
}

// BuyGoodsBeforeUse 对齐 ShopFunc.php:360。
func (s *Service) BuyGoodsBeforeUse(ctx context.Context, uid, id, cnt, paytype int) ([]any, error) {
	if paytype != 0 && paytype != 1 {
		return nil, errLegacy("")
	}
	if cnt < 1 {
		return nil, errLegacy("购买数量无效。")
	}
	commend := "(commend='0' or commend=2)"
	if paytype != 0 {
		commend = "(commend='1' or commend=2)"
	}
	goods, err := s.db.FetchOne(ctx,
		"select * from cfg_shops where id=? and onsale=1 and starttime<=unix_timestamp() and endtime>unix_timestamp() and "+commend, id)
	if errors.Is(err, sql.ErrNoRows) {
		goods = nil
	} else if err != nil {
		return nil, err
	}
	if len(goods) == 0 {
		return nil, errLegacy("此商品已经停售。")
	}
	moneyNeed := int64(cnt) * modelInt64(goods, "price")
	userInfo, err := s.db.FetchOne(ctx, "select money, gift from users where id=?", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	userMoney := modelInt64(userInfo, "money")
	userGift := modelInt64(userInfo, "gift")
	if paytype == 0 && userMoney < moneyNeed {
		return nil, errLegacy("你的元宝不足，请充值。")
	}
	if paytype == 1 && userGift < moneyNeed {
		return nil, errLegacy("你的礼金不足。")
	}
	totalCount := modelInt(goods, "totalCount")
	if totalCount < 2000000000 && totalCount == 0 {
		return nil, errLegacy("此商品已经卖光了。")
	}
	userbuycnt := modelInt(goods, "userbuycnt")
	daybuycnt := modelInt(goods, "daybuycnt")
	if userbuycnt > 0 || daybuycnt > 0 { // 限制商品（原版此处无 totalCount<2e9 条件）
		buycnt, err := s.cellInt(ctx, "select `count` from log_shop_buy_cnts where user_id=? and sid=?", uid, id)
		if err != nil {
			return nil, err
		}
		todaybuycnt, err := s.cellInt(ctx,
			"select sum(`count`) from log_shops where user_id=? and shopid=? and date(now())=date(from_unixtime(time))", uid, id)
		if err != nil {
			return nil, err
		}
		if userbuycnt > 0 && buycnt+int64(cnt) > int64(userbuycnt) {
			if int64(userbuycnt) > buycnt {
				remain := int64(userbuycnt) - buycnt
				return nil, errLegacy(fmt.Sprintf("购买数量无效。此商品每人限购%d个，你已经购买%d个，只能再购买%d个此商品。",
					userbuycnt, buycnt, remain))
			}
			return nil, errLegacy(fmt.Sprintf("此商品每人限购%d个，你已经达到购买限制，不能再购买更多此商品。", userbuycnt))
		}
		if daybuycnt > 0 && todaybuycnt+int64(cnt) > int64(daybuycnt) {
			if int64(daybuycnt) > todaybuycnt {
				remain := int64(daybuycnt) - todaybuycnt
				return nil, errLegacy(fmt.Sprintf("购买数量无效。此商品每人每天限购%d个，你今天已经购买%d个，只能再购买%d个此商品。",
					daybuycnt, buycnt, remain)) // 原版同样用 $buycnt
			}
			return nil, errLegacy(fmt.Sprintf("此商品每人每天限购%d个，你今天已经达到购买限制，请明天再来购买此商品。", daybuycnt))
		}
		if _, err := s.db.Exec(ctx,
			"insert into log_shop_buy_cnts (user_id, sid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
			uid, id, cnt, cnt); err != nil {
			return nil, err
		}
	}
	if paytype == 0 {
		if err := s.addMoney(ctx, uid, -moneyNeed, 10); err != nil {
			return nil, err
		}
	} else if paytype == 1 {
		if err := s.addGift(ctx, uid, -moneyNeed, 10); err != nil {
			return nil, err
		}
	}
	logGoodType := 222
	if paytype == 0 {
		logGoodType = 63
	} else {
		logGoodType = 64
	}
	if err := s.addGoods(ctx, uid, modelInt(goods, "gid"), int64(modelInt(goods, "pack")*cnt), logGoodType); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update users set last_pay=? where id=?", paytype, uid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_shops (user_id, shopid, `count`, price, time, paytype) values (?,?,?,?,unix_timestamp(),?)",
		uid, id, cnt, modelInt64(goods, "price"), paytype); err != nil {
		return nil, err
	}
	ret := []any{modelInt(goods, "gid"), paytype}
	if paytype == 0 {
		ret = append(ret, userMoney-moneyNeed)
	} else if paytype == 1 {
		ret = append(ret, userGift-moneyNeed)
	}
	if _, err := s.db.Exec(ctx, "update users set last_pay=? where id=?", paytype, uid); err != nil {
		return nil, err
	}
	return ret, nil
}

// liquanPattern 对齐 "/[a-zA-Z0-9]{10}/"（无锚定）。
var liquanPattern = regexp.MustCompile(`[a-zA-Z0-9]{10}`)

// ExchangeLiquan 对齐 ShopFunc.php:305。
func (s *Service) ExchangeLiquan(ctx context.Context, uid int, rawCode string) ([]any, error) {
	code := strings.TrimSpace(rawCode)
	if code == "" {
		return nil, errLegacy("礼券码不能为空。")
	}
	if !liquanPattern.MatchString(code) {
		return nil, errLegacy("礼券码无效。请重新输入正确的礼券码。")
	}
	item, err := s.db.FetchOne(ctx, "select * from tickets where code=? limit 1", code)
	if errors.Is(err, sql.ErrNoRows) {
		item = nil
	} else if err != nil {
		return nil, err
	}
	if len(item) == 0 {
		return nil, errLegacy("礼券码无效。请重新输入正确的礼券码。")
	} else if modelInt(item, "user_id") > 0 {
		return nil, errLegacy("该礼券码已被使用。")
	} else if modelInt(item, "binduid") > 0 && modelInt(item, "binduid") != uid {
		return nil, errLegacy("该礼券码已和另外的玩家绑定，你无法使用。")
	}
	limit := 0
	pattern := code
	if len(pattern) >= 3 {
		pattern = code[:3]
	}
	if pattern == "LQA" {
		limit = 1
	} else {
		for i := 2; i < 10; i++ {
			if pattern == "LQ"+strconv.Itoa(i) {
				limit = i
				break
			}
		}
	}
	if limit > 0 {
		used, err := s.cellInt(ctx,
			"select count(1) from tickets where user_id=? and contentid=? and code like ?",
			uid, modelInt(item, "contentid"), pattern+"%")
		if err != nil {
			return nil, err
		}
		if used > 0 && used >= int64(limit) {
			return nil, errLegacy(fmt.Sprintf("该类型礼券一个账号只能使用%d个，您已经使用了%d个，不能再使用了。", limit, used))
		}
	}
	content, err := s.db.FetchOne(ctx, "select * from ticket_contents where id=?", modelInt(item, "contentid"))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update tickets set user_id=?, time=unix_timestamp() where id=?", uid, modelInt(item, "id")); err != nil {
		return nil, err
	}
	contentStr := ""
	if content != nil {
		contentStr = fmt.Sprint(content["content"])
	}
	ret, err := s.parseAndAddReward(ctx, uid, contentStr, 8, 3, 8, 3)
	if err != nil {
		return nil, err
	}
	return []any{ret}, nil
}

// parseAndAddReward 对齐 utils.php:7。
// reward 格式：n,type,gid,cnt,type,gid,cnt,...（type0 道具/货币、1 装备、2 物品）。
func (s *Service) parseAndAddReward(ctx context.Context, uid int, reward string,
	logGoodsType, logArmorType, logThingType, logMoneyType int) ([]map[string]any, error) {
	goods := strings.Split(reward, ",")
	if len(goods) == 0 {
		return []map[string]any{}, nil
	}
	goodcnt, _ := strconv.Atoi(goods[0])
	money := int64(0)
	ret := []map[string]any{}
	isYuanBao := false
	for i := 1; i < goodcnt*3 && i+2 < len(goods); i += 3 {
		typ := goods[i]
		gid, _ := strconv.Atoi(goods[i+1])
		cnt, _ := strconv.ParseInt(goods[i+2], 10, 64)
		switch typ {
		case "0":
			if gid == 0 {
				money += cnt
			} else if gid == -100 {
				money += cnt
				isYuanBao = true
			} else {
				if err := s.addGoods(ctx, uid, gid, cnt, logGoodsType); err != nil {
					return nil, err
				}
			}
			good, err := s.db.FetchOne(ctx, "select *,? as `count`,'0' as gype from cfg_goods where gid=?", cnt, gid)
			if errors.Is(err, sql.ErrNoRows) {
				good = map[string]any{}
			} else if err != nil {
				return nil, err
			}
			good["count"] = cnt
			good["gtype"] = 0
			ret = append(ret, good)
		case "1":
			armor, err := s.db.FetchOne(ctx, "select * from cfg_armor where id=?", gid)
			if errors.Is(err, sql.ErrNoRows) {
				armor = map[string]any{}
			} else if err != nil {
				return nil, err
			}
			armor["count"] = cnt
			armor["gtype"] = 1
			oriHP := modelInt(armor, "ori_hp_max")
			armor["hp"] = oriHP
			armor["hp_max"] = oriHP
			ret = append(ret, armor)
			if err := s.addArmor(ctx, uid, armor, cnt, logArmorType); err != nil {
				return nil, err
			}
		case "2":
			// cfg_things 缺失 → 空行（原版 select 无行返回 false，PHP 数组赋值仍成功）。
			thing := map[string]any{}
			thing["count"] = cnt
			thing["gtype"] = 2
			ret = append(ret, thing)
			if err := s.addThings(ctx, uid, gid, cnt, logThingType); err != nil {
				return nil, err
			}
		}
	}
	if money > 0 {
		if isYuanBao {
			if err := s.addMoney(ctx, uid, money, logMoneyType); err != nil {
				return nil, err
			}
		} else {
			if err := s.addGift(ctx, uid, money, logMoneyType); err != nil {
				return nil, err
			}
		}
	}
	return ret, nil
}

// SellGoods 对齐 GoodsFunc.php:632（出售宝物换黄金）。
func (s *Service) SellGoods(ctx context.Context, uid, cid, gid, goodsCount int) ([]any, error) {
	if gid == 160052 { // checkForSell
		return nil, errLegacy("无法回收")
	}
	level, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=? limit 1", cid, game.BidMarket)
	if err != nil {
		return nil, err
	}
	if level < 5 {
		return nil, errLegacy("市场等级达到5级才能出售宝物。")
	}
	realNobility, err := s.cellFloat(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return nil, err
	}
	nobility, err := s.getBufNobility(ctx, uid, realNobility)
	if err != nil {
		return nil, err
	}
	if nobility < 1 {
		return nil, errLegacy("爵位达到“公士”才能出售宝物。")
	}
	goldAdd, err := s.cellInt(ctx, "select value from cfg_goods where gid=?", gid)
	if err != nil {
		return nil, err
	}
	goldAdd = goldAdd * 500 * int64(goodsCount)
	myCount, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return nil, err
	}
	if goodsCount <= 0 || myCount < int64(goodsCount) {
		return nil, errLegacy("你没有那么多道具，请正确填写道具数量。")
	}
	if _, err := s.db.Exec(ctx, "update city_resources set gold=gold+? where city_id=?", goldAdd, cid); err != nil {
		return nil, err
	}
	if err := s.reduceGoods(ctx, uid, gid, int64(goodsCount), 9); err != nil {
		return nil, err
	}
	joined, err := s.db.FetchOne(ctx,
		"select * from user_goods g left join cfg_goods f on f.gid=g.gid where g.user_id=? and g.gid=?", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		joined = map[string]any{}
	} else if err != nil {
		return nil, err
	}
	gold, err := s.cellInt(ctx, "select gold from city_resources where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	return []any{joined, cid, gold}, nil
}

// SetCityProductRate 对齐 CityFunc.php:236（设置四资源劳力比例）。
func (s *Service) SetCityProductRate(ctx context.Context, uid, cid int, foodRate, woodRate, rockRate, ironRate int64) ([]any, error) {
	ok, err := s.db.Exists(ctx, "select 1 from cities where user_id=? and id=? limit 1", uid, cid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errLegacy("你没有权限进行该项操作。") // MarkCity.No_Permission
	}
	if _, err := s.db.Exec(ctx,
		"update city_res_add set food_rate=?,wood_rate=?,rock_rate=?,iron_rate=?,resource_changing=1 where city_id=?",
		foodRate, woodRate, rockRate, ironRate, cid); err != nil {
		return nil, err
	}
	// completeTask 219-222（各比例=100 触发）：M8 省略。
	return []any{}, nil
}
