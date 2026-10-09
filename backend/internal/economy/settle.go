package economy

// settle.go 复刻惰性产出结算与交易结算。
// 新栈无 cron（legacy 由 cfg_js.php 定时器 + global.php 进城触发），故在进城/市场请求入口惰性执行。
// 进城顺序（global.php:77-82）：SetCityBaseProduce → refreshFoodArmyUsers → UpdateUsersCityResource。
// 交易结算（原 ReportCron 定时器）：HandleAutoTrans → HandleTrade。

import (
	"context"
	"database/sql"
	"errors"
	"math"
)

// cityProducts 对齐 getMyCityProducts 返回结构（utils.php:2188）。
type cityProducts struct {
	FoodRate, WoodRate, RockRate, IronRate int

	FoodPeople, WoodPeople, RockPeople, IronPeople float64
	FoodBase, WoodBase, RockBase, IronBase         float64
	FoodTech, WoodTech, RockTech, IronTech         float64

	FoodArmyUse float64
	ChiefAdd    float64

	FieldFoodAdd, FieldWoodAdd, FieldRockAdd, FieldIronAdd int
	GoodsFoodAdd, GoodsWoodAdd, GoodsRockAdd, GoodsIronAdd int
	SkillFoodAdd, SkillWoodAdd, SkillRockAdd, SkillIronAdd int

	GoodsFoodEndtime, GoodsWoodEndtime, GoodsRockEndtime, GoodsIronEndtime int64
}

// setCityBaseProduce 对齐 utils.php:2145（资源/人口/黄金上限；bid 已按新库映射）。
func (s *Service) setCityBaseProduce(ctx context.Context, cid int) error {
	type bRow struct {
		bid, level, usingPeople int
	}
	rows, err := s.db.FetchRows(ctx,
		"select b.building_id as bid, b.level, l.using_people "+
			"from buildings b join cfg_building_levels l on b.building_id=l.bid and b.level=l.level "+
			"where b.city_id=? and b.building_id<7 order by b.building_id desc, b.level desc", cid)
	if err != nil {
		return err
	}
	if len(rows) == 0 { // legacy empty($cityresource) → 不写
		return nil
	}
	var foodMax, woodMax, rockMax, ironMax, goldMax, peopleMax float64
	for _, r := range rows {
		bid := modelInt(r, "bid")
		level := modelInt(r, "level")
		resMax := float64(modelInt(r, "using_people"))
		switch bid {
		case 2: // 农田（legacy bid1）
			foodMax += resMax * 1000
		case 3: // 伐木（legacy bid2）
			woodMax += resMax * 1000
		case 4: // 采石（legacy bid3）
			rockMax += resMax * 500
		case 5: // 铁矿（legacy bid4）
			ironMax += resMax * 400
		case 6: // 民居（legacy bid5）
			peopleMax += 100*float64(level) + float64(level)*float64(level-1)*100/2
		case 1: // 官府（legacy bid6）
			goldMax = resMax * 100000
		}
	}
	// 仓库（新 bid9 ← legacy bid17）
	cktlevels, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=9 limit 1", cid)
	if err != nil {
		return err
	}
	if cktlevels != 0 {
		tlevels, err := s.cellInt(ctx, "select level from city_technics where technic_id=15 and city_id=?", cid)
		if err != nil {
			return err
		}
		if tlevels > cktlevels {
			tlevels = cktlevels
		}
		tlevel := 1 + float64(tlevels)*0.1
		cktUsingPeople, err := s.cellInt(ctx,
			"select using_people from cfg_building_levels where bid=9 and level=? limit 1", cktlevels)
		if err != nil {
			return err
		}
		cktlevel := 100 * tlevel * float64(cktUsingPeople)
		store, err := s.db.FetchOne(ctx,
			"select food_store,wood_store,rock_store,iron_store from city_res_add where city_id=?", cid)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				store = map[string]any{}
			} else {
				return err
			}
		}
		foodMax = 10000 + cktlevel*float64(modelInt(store, "food_store")) + foodMax*tlevel
		woodMax = 10000 + cktlevel*float64(modelInt(store, "wood_store")) + woodMax*tlevel
		rockMax = 10000 + cktlevel*float64(modelInt(store, "rock_store")) + rockMax*tlevel
		ironMax = 10000 + cktlevel*float64(modelInt(store, "iron_store")) + ironMax*tlevel
	}
	_, err = s.db.Exec(ctx,
		"update city_resources set wood_max=?, food_max=?, rock_max=?, iron_max=?, gold_max=?, people_max=?, changing=1 where city_id=?",
		int64(math.Floor(woodMax)), int64(math.Floor(foodMax)), int64(math.Floor(rockMax)),
		int64(math.Floor(ironMax)), int64(math.Floor(goldMax)), int64(math.Floor(peopleMax)), cid)
	return err
}

// refreshFoodArmyUsers 对齐 utils.php:2104（野地 troops 缺失 → 仅计城内驻军；×2/3）。
func (s *Service) refreshFoodArmyUsers(ctx context.Context, cid int) error {
	fau, err := s.cellFloat(ctx,
		"select coalesce(sum(c.food_use*sc.`count`),0) from city_soldiers sc join cfg_soldiers c on sc.soldier_id=c.sid where sc.city_id=?", cid)
	if err != nil {
		return err
	}
	fau = fau * 2 / 3
	_, err = s.db.Exec(ctx, "update city_resources set food_army_use=? where city_id=?", int64(math.Floor(fau)), cid)
	return err
}

// getMyCityProducts 对齐 utils.php:2188。
func (s *Service) getMyCityProducts(ctx context.Context, uid, cid int) (*cityProducts, error) {
	if _, err := s.db.Exec(ctx, "insert ignore into city_res_add (city_id) values (?)", cid); err != nil {
		return nil, err
	}
	addRow, err := s.db.FetchOne(ctx, "select * from city_res_add where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	p := &cityProducts{
		FoodRate: modelInt(addRow, "food_rate"), WoodRate: modelInt(addRow, "wood_rate"),
		RockRate: modelInt(addRow, "rock_rate"), IronRate: modelInt(addRow, "iron_rate"),

		FieldFoodAdd: modelInt(addRow, "field_food_add"), FieldWoodAdd: modelInt(addRow, "field_wood_add"),
		FieldRockAdd: modelInt(addRow, "field_rock_add"), FieldIronAdd: modelInt(addRow, "field_iron_add"),

		GoodsFoodAdd: modelInt(addRow, "goods_food_add"), GoodsWoodAdd: modelInt(addRow, "goods_wood_add"),
		GoodsRockAdd: modelInt(addRow, "goods_rock_add"), GoodsIronAdd: modelInt(addRow, "goods_iron_add"),

		SkillFoodAdd: modelInt(addRow, "skill_food_add"), SkillWoodAdd: modelInt(addRow, "skill_wood_add"),
		SkillRockAdd: modelInt(addRow, "skill_rock_add"), SkillIronAdd: modelInt(addRow, "skill_iron_add"),
	}
	peopleOf := func(bid int) (float64, error) {
		v, err := s.cellFloat(ctx, `select coalesce(sum(l.using_people),0)
			from buildings b join cfg_building_levels l on b.building_id=l.bid and b.level=l.level
			where b.city_id=? and b.building_id=?`, cid, bid)
		return v, err
	}
	if p.FoodPeople, err = peopleOf(2); err != nil {
		return nil, err
	}
	if p.WoodPeople, err = peopleOf(3); err != nil {
		return nil, err
	}
	if p.RockPeople, err = peopleOf(4); err != nil {
		return nil, err
	}
	if p.IronPeople, err = peopleOf(5); err != nil {
		return nil, err
	}
	spd := 10.0 // GAME_SPEED_RATE
	p.FoodBase = 10 * p.FoodPeople * spd
	p.WoodBase = 10 * p.WoodPeople * spd
	p.RockBase = 5 * p.RockPeople * spd
	p.IronBase = 4 * p.IronPeople * spd

	techOf := func(tid int) (float64, error) {
		return s.cellFloat(ctx, "select level*10 from city_technics where technic_id=? and city_id=?", tid, cid)
	}
	if p.FoodTech, err = techOf(1); err != nil {
		return nil, err
	}
	if p.WoodTech, err = techOf(2); err != nil {
		return nil, err
	}
	if p.RockTech, err = techOf(3); err != nil {
		return nil, err
	}
	if p.IronTech, err = techOf(4); err != nil {
		return nil, err
	}

	// 城守加成
	chiefhid, err := s.cellInt(ctx, "select chief_hero_id from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	if chiefhid > 0 {
		h, err := s.db.FetchOne(ctx,
			"select level, affairs_base, affairs_add, affairs_add_on, command_base, command_add_on from heroes where id=? and city_id=?",
			chiefhid, cid)
		if err == nil {
			chiefAdd := float64(modelInt(h, "affairs_add") + modelInt(h, "affairs_base") + modelInt(h, "affairs_add_on"))
			heroCommand := float64(modelInt(h, "level") + modelInt(h, "command_base") + modelInt(h, "command_add_on"))
			cityPeopleMax, err := s.cellInt(ctx, "select people_max from city_resources where city_id=?", cid)
			if err != nil {
				return nil, err
			}
			hufu := 1.0 // mem_hero_buffer buftype=1 缺失
			leaderTech, err := s.cellInt(ctx, "select level from city_technics where city_id=? and technic_id=6", cid)
			if err != nil {
				return nil, err
			}
			peoplerate := heroCommand * 10.0 * (100*hufu + float64(leaderTech)*10) / float64(cityPeopleMax+1)
			if peoplerate > 1.0 {
				peoplerate = 1.0
			}
			chiefAdd *= peoplerate
			// 文曲星（buftype=2）缺失 → 不 ×1.25
			p.ChiefAdd = chiefAdd
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	p.FoodArmyUse, err = s.cellFloat(ctx, "select food_army_use from city_resources where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	bufEnd := func(buftype int) (int64, error) {
		return s.cellInt(ctx, "select endtime from user_buffers where user_id=? and buftype=?", uid, buftype)
	}
	if p.GoodsFoodAdd > 0 {
		if p.GoodsFoodEndtime, err = bufEnd(1); err != nil {
			return nil, err
		}
	}
	if p.GoodsWoodAdd > 0 {
		if p.GoodsWoodEndtime, err = bufEnd(2); err != nil {
			return nil, err
		}
	}
	if p.GoodsRockAdd > 0 {
		if p.GoodsRockEndtime, err = bufEnd(3); err != nil {
			return nil, err
		}
	}
	if p.GoodsIronAdd > 0 {
		if p.GoodsIronEndtime, err = bufEnd(4); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// updateUsersCityResource 对齐 utils.php:2329。
// 原版怪癖 1:1 保留：
//   - skill_wood/rock/iron_add 均乘 food_add_base（2360-2363 全用 $food_add_base）。
//   - $skill_iorn_add 拼写错误 → iron 的 empty 守卫失效（DB 值为 0 时无影响）。
//   - $ciyt['hero_fee'] 拼写错误（$city→$ciyt）→ 黄金扣减恒按 0。
//   - wood/rock/iron_add 不含 skill 项（仅 food_add 含 skill_food_add）。
func (s *Service) updateUsersCityResource(ctx context.Context, uid, cid int) error {
	info, err := s.getMyCityProducts(ctx, uid, cid)
	if err != nil {
		return err
	}
	// skill_*_add 全部以 food_add_base 计算（原版 bug）。
	skillFoodAdd := info.FoodBase * float64(info.SkillFoodAdd) / 100
	skillWoodAdd := info.FoodBase * float64(info.SkillWoodAdd) / 100
	skillRockAdd := info.FoodBase * float64(info.SkillRockAdd) / 100
	skillIronAdd := info.FoodBase * float64(info.SkillIronAdd) / 100

	peopleWorking := info.FoodPeople*float64(info.FoodRate)/100 +
		info.WoodPeople*float64(info.WoodRate)/100 +
		info.RockPeople*float64(info.RockRate)/100 +
		info.IronPeople*float64(info.IronRate)/100
	if _, err := s.db.Exec(ctx, "update city_resources set people_working=? where city_id=?", peopleWorking, cid); err != nil {
		return err
	}
	city, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil {
		return err
	}
	pw := modelFloat(city, "people_working")
	if pw == 0 {
		pw = 1
	}
	productRate := modelFloat(city, "people") / pw
	if productRate > 1 {
		productRate = 1
	}
	if _, err := s.db.Exec(ctx, "update city_res_add set chief_add=? where city_id=?", info.ChiefAdd, cid); err != nil {
		return err
	}
	tlevels, err := s.cellInt(ctx, "select level from city_technics where technic_id=15 and city_id=?", cid)
	if err != nil {
		return err
	}
	tlevel := 1 + float64(tlevels)*0.1

	foodAdd := math.Floor(100 + skillFoodAdd + ((info.FoodBase*productRate)*(1+(info.FoodTech+float64(info.FieldFoodAdd)+float64(info.GoodsFoodAdd)+info.ChiefAdd)/100)*float64(info.FoodRate))/100)
	foodMax := math.Floor(10000 + (info.FoodBase*tlevel)*100)
	woodAdd := math.Floor(100 + ((info.WoodBase*productRate)*(1+(info.WoodTech+float64(info.FieldWoodAdd)+float64(info.GoodsWoodAdd)+info.ChiefAdd)/100)*float64(info.WoodRate))/100)
	woodMax := math.Floor(10000 + (info.WoodBase*tlevel)*100)
	rockAdd := math.Floor(100 + ((info.RockBase*productRate)*(1+(info.RockTech+float64(info.FieldRockAdd)+float64(info.GoodsRockAdd)+info.ChiefAdd)/100)*float64(info.RockRate))/100)
	rockMax := math.Floor(10000 + (info.RockBase*tlevel)*100)
	ironAdd := math.Floor(100 + ((info.IronBase*productRate)*(1+(info.IronTech+float64(info.FieldIronAdd)+float64(info.GoodsIronAdd)+info.ChiefAdd)/100)*float64(info.IronRate))/100)
	ironMax := math.Floor(10000 + (info.IronBase*tlevel)*100)

	_ = skillWoodAdd
	_ = skillRockAdd
	_ = skillIronAdd

	curFood := modelFloat(city, "food")
	curWood := modelFloat(city, "wood")
	curRock := modelFloat(city, "rock")
	curIron := modelFloat(city, "iron")
	curGold := modelFloat(city, "gold")
	people := modelFloat(city, "people")
	goldRate := modelFloat(city, "gold_rate")
	tax := modelFloat(city, "tax")
	// hero_fee 因 $ciyt 拼写错误恒 0（原版 bug）。

	foods := curFood + (foodAdd-info.FoodArmyUse)/225
	if foods <= 0 {
		foods = 0
	} else {
		foods = math.Floor(foods)
	}
	woods := curWood + woodAdd/225
	if woods > woodMax {
		woods = math.Floor(curWood)
	} else {
		woods = math.Floor(woods)
	}
	rocks := curRock + rockAdd/255
	if rocks > rockMax {
		rocks = math.Floor(curRock)
	} else {
		rocks = math.Floor(rocks)
	}
	irons := curIron + ironAdd/255
	if irons > ironMax {
		irons = math.Floor(curIron)
	} else {
		irons = math.Floor(irons)
	}
	golds := (people*goldRate/10000*tax - 0) / 225
	if golds+curGold > 0 {
		golds = math.Floor(golds + curGold)
	} else {
		golds = 0
	}

	if _, err := s.db.Exec(ctx, `update city_resources set
		food=?, wood=?, rock=?, iron=?, gold=?,
		food_add=?, food_max=?, wood_add=?, wood_max=?, rock_add=?, rock_max=?, iron_add=?, iron_max=?, changing=1
		where city_id=?`,
		int64(foods), int64(woods), int64(rocks), int64(irons), int64(golds),
		int64(foodAdd), int64(foodMax), int64(woodAdd), int64(woodMax), int64(rockAdd), int64(rockMax),
		int64(ironAdd), int64(ironMax), cid); err != nil {
		return err
	}
	if err := s.updateCityPeopleMax(ctx, cid); err != nil {
		return err
	}
	return s.updateCityGoldMax(ctx, cid)
}

// updateCityPeopleMax 对齐 utils.php:882（sum(level*(level+1)*50)，bid=民居6）。
func (s *Service) updateCityPeopleMax(ctx context.Context, cid int) error {
	peopleMax, err := s.cellFloat(ctx,
		"select coalesce(sum(level*(level+1)*50),0) from buildings where city_id=? and building_id=6", cid)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "update city_resources set people_max=? where city_id=?", int64(math.Floor(peopleMax)), cid); err != nil {
		return err
	}
	return s.updateCityPeopleStable(ctx, cid)
}

// updateCityPeopleStable 对齐 utils.php:874。
func (s *Service) updateCityPeopleStable(ctx context.Context, cid int) error {
	peopleMax, err := s.cellFloat(ctx, "select people_max from city_resources where city_id=?", cid)
	if err != nil {
		return err
	}
	morale, err := s.cellFloat(ctx, "select morale from city_resources where city_id=?", cid)
	if err != nil {
		return err
	}
	peopleStable := peopleMax * morale * 0.01
	_, err = s.db.Exec(ctx, "update city_resources set people_stable=? where city_id=?", peopleStable, cid)
	return err
}

// updateCityGoldMax 对齐 utils.php:889（bid=官府1）。
func (s *Service) updateCityGoldMax(ctx context.Context, cid int) error {
	level, err := s.cellInt(ctx, "select level from buildings where city_id=? and building_id=1 limit 1", cid)
	if err != nil {
		return err
	}
	goldMax, err := s.cellFloat(ctx,
		"select coalesce(sum(level*(level+1)*500000),0) from buildings where city_id=? and building_id=1", cid)
	if err != nil {
		return err
	}
	switch level {
	case 11:
		goldMax = 6600 * 10000
	case 12:
		goldMax = 7800 * 10000
	case 13:
		goldMax = 9100 * 10000
	case 14:
		goldMax = 10500 * 10000
	case 15:
		goldMax = 12000 * 10000
	}
	_, err = s.db.Exec(ctx, "update city_resources set gold_max=? where city_id=?", int64(goldMax), cid)
	return err
}

// settleCity 进城三件套（global.php:77-82）。
func (s *Service) settleCity(ctx context.Context, uid, cid int) error {
	if err := s.setCityBaseProduce(ctx, cid); err != nil {
		return err
	}
	if err := s.refreshFoodArmyUsers(ctx, cid); err != nil {
		return err
	}
	return s.updateUsersCityResource(ctx, uid, cid)
}

// tradeResourceNames 对齐 lang.php:265 report.resources（restype++ 后的展示索引）。
var tradeResourceNames = []string{"黄金", "粮食", "木材", "石料", "铁锭", "人口", "民心"}

// handleTrade 对齐 ReportCron.php:729（惰性结算已到期成交单）。
func (s *Service) handleTrade(ctx context.Context) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	rows, err := s.db.FetchRows(ctx,
		"select id,cid,buycid,gold,restype,`count` from city_trades where endtime>1 and endtime<=?", now)
	if err != nil {
		return err
	}
	for _, row := range rows {
		id := modelInt(row, "id")
		cid := modelInt(row, "cid")
		buycid := modelInt(row, "buycid")
		gold := modelInt64(row, "gold")
		restype := modelInt(row, "restype")
		count := modelInt64(row, "count")
		// 卖家城收黄金
		if _, err := s.db.Exec(ctx, "update city_resources set gold=gold+? where city_id=?", gold, cid); err != nil {
			return err
		}
		switch restype {
		case 0:
			if err := s.addCityResources(ctx, buycid, 0, 0, 0, count, 0); err != nil {
				return err
			}
			restype++
		case 1:
			if err := s.addCityResources(ctx, buycid, count, 0, 0, 0, 0); err != nil {
				return err
			}
			restype++
		case 2:
			if err := s.addCityResources(ctx, buycid, 0, count, 0, 0, 0); err != nil {
				return err
			}
			restype++
		case 3:
			if err := s.addCityResources(ctx, buycid, 0, 0, count, 0, 0); err != nil {
				return err
			}
			restype++
		case 4:
			if err := s.addCityResources(ctx, buycid, 0, 0, 0, 0, count); err != nil {
				return err
			}
			restype = 0
		}
		if _, err := s.db.Exec(ctx, "delete from city_trades where id=?", id); err != nil {
			return err
		}
		uid, err := s.cellInt(ctx, "select user_id from cities where id=?", cid)
		if err != nil {
			return err
		}
		youruid, err := s.cellInt(ctx, "select user_id from cities where id=?", buycid)
		if err != nil {
			return err
		}
		mycityname, err := s.cityNamePosition(ctx, cid)
		if err != nil {
			return err
		}
		buycityname, err := s.cityNamePosition(ctx, buycid)
		if err != nil {
			return err
		}
		resourcename := ""
		if restype >= 0 && restype < len(tradeResourceNames) {
			resourcename = tradeResourceNames[restype]
		}
		content := "与" + buycityname + "的买卖已达成，商队已经返回" + mycityname + "。" +
			"</br>出售：" + resourcename + " " + itoa(count) + "</br>获得：黄金 " + itoa(gold)
		if err := s.sendReport(ctx, int(uid), cid, buycid, 15, content); err != nil {
			return err
		}
		content = "与" + mycityname + "的买卖已达成，商队已经返回" + buycityname + "。" +
			"</br>购买：" + resourcename + " " + itoa(count) + "</br>支付：黄金 " + itoa(gold)
		if err := s.sendReport(ctx, int(youruid), cid, buycid, 15, content); err != nil {
			return err
		}
	}
	return nil
}

// handleAutoTrans 对齐 ReportCron.php:782。
// 原版怪癖 1:1 保留：switch(${$v['res_type']}) 是可变变量误用 → 恒取 $0（未定义=null）
// → loose 匹配 case 0 → 无论 res_type 为何值，都只从出发城扣"粮食"；且粮食不足时 return（中断整批）。
func (s *Service) handleAutoTrans(ctx context.Context) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	rows, err := s.db.FetchRows(ctx,
		"select id,fromcid,tocid,trans_type,res_type,`count`,distance,end_time from city_autotrans where end_time-60<=?", now)
	if err != nil {
		return err
	}
	for _, row := range rows {
		id := modelInt(row, "id")
		fromcid := modelInt(row, "fromcid")
		tocid := modelInt(row, "tocid")
		transType := modelInt(row, "trans_type")
		resType := modelInt(row, "res_type")
		count := modelInt64(row, "count")
		distance := modelFloat(row, "distance")
		endTime := modelInt64(row, "end_time")

		if transType == 0 {
			if _, err := s.db.Exec(ctx, "delete from city_autotrans where id=?", id); err != nil {
				return err
			}
		} else {
			if _, err := s.db.Exec(ctx,
				"update city_autotrans set start_time=start_time+86400, end_time=end_time+86400 where id=?", id); err != nil {
				return err
			}
		}
		// 恒按 case 0（food）扣减出发城粮食（原版 bug）。
		food, err := s.cellInt(ctx, "select food from city_resources where city_id=?", fromcid)
		if err != nil {
			return err
		}
		if food < count {
			return nil // 原版 return：中断整批
		}
		if _, err := s.db.Exec(ctx,
			"update city_resources set food=GREATEST(0,food-?) where city_id=?", count, fromcid); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx,
			"insert into city_trades (cid,state,restype,`count`,price,gold,distance,endtime,buycid,unionid,limittime) "+
				"values (?,1,?,?,0,0,?,?,?,0,0)",
			fromcid, resType, count, distance, endTime, tocid); err != nil {
			return err
		}
	}
	return nil
}

// settleTrades 惰性交易结算（原 cfg_js.php 定时器：HandleAutoTrans → HandleTrade）。
func (s *Service) settleTrades(ctx context.Context) error {
	if err := s.handleAutoTrans(ctx); err != nil {
		return err
	}
	return s.handleTrade(ctx)
}
