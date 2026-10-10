package building

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// build.go —— R11-1：1:1 移植 legacy server/game/BuildingFunc.php 的
//   startUpgradeBuilding:355 / stopUpgradeBuilding:571 / startDestroyBuildingAll:625 /
//   startDestroyBuilding:661 / stopDestroyBuilding:733 / startChangeBuilding:748
// 以及 getBuildingSpeedRate:15 / isUsingKaoGongJi:69 / getBuildingMaxLevel:97。
//
// 与 legacy 的差异（不可 1:1 之处，均逐条注释；汇报中同步列出）：
//   - bid 空间：见 legacy_bid.go（legacy 20 bid ↔ 重写 14 building_id 显式翻译）。
//   - 建筑坐标：legacy 用 encodeBuildingPosition=inner*100+x*10+y 且校验 VALID_GRID_ARRAY/buildingxy
//     白名单；新库 buildings.xy 为字母+数字标签（model.xyFrom），无法逐值映射 → 未移植位置白名单与外挂文案
//     （“郑重提醒你：再用外挂…”）与官府空地校验（government_not_enough）。
//   - 结算 side-effect：legacy getBuildingTasks 附带 completeTask/UpdateUsersCityResource/logUserAction，
//     属 M8/任务/产出模块，未接线（既有 building 包亦未接线）。
//   - 鬼斧神工技能（cfg_book/sys_user_book）与文曲星符（mem_hero_buffer）新库无表 → 分别取 0 / false。
//   - 君主将等级表（sys_user_level）新库无 → checkHeroLevel 的 userLevel 取 0（相关加成不触发）。

// Build 对齐 startUpgradeBuilding（BuildingFunc.php:355）：建造新建筑与升级同一入口。
func (s *Service) Build(ctx context.Context, uid, cid, bid, inner, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	xy := xyTag(x, y)

	// cfg_building where bid and inner（校验建筑类型与内外城一致）
	hasCfg, err := s.db.Exists(ctx, "select 1 from cfg_buildings where bid=? and `inner`=? limit 1", bid, inner)
	if err != nil {
		return nil, err
	}
	if !hasCfg {
		return nil, httpx.NotFound("nobuilding", "不存在该建筑")
	}

	dstbid := bid
	dstlevel := 1

	exist, err := s.db.FetchOne(ctx, "select * from buildings where city_id=? and xy=? and building_id=?", cid, xy, bid)
	switch {
	case err == nil:
		b := model.BuildingFromMap(exist)
		if b.State != 0 {
			return nil, httpx.BadRequest("upgrading", "建筑正在升级中")
		}
		dstbid = b.BID
		dstlevel = b.Level + 1
	case errors.Is(err, sql.ErrNoRows):
		// 不是该建筑但格子被占用 → building_error
		occupied, err2 := s.db.Exists(ctx, "select 1 from buildings where city_id=? and xy=? limit 1", cid, xy)
		if err2 != nil {
			return nil, err2
		}
		if occupied {
			return nil, httpx.BadRequest("building_error", "建造建筑错误")
		}
	default:
		return nil, err
	}

	cityType, err := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}

	if dstlevel == 1 {
		// 刚开始建造：非多座建筑不得重复建造
		if !multiBuildingAllowed(bid) {
			dup, err := s.db.Exists(ctx, "select 1 from buildings where city_id=? and building_id=? limit 1", cid, bid)
			if err != nil {
				return nil, err
			}
			if dup {
				return nil, httpx.BadRequest("same_building_has_build", "相同的建筑已经建造。")
			}
		}
	} else {
		maxlevel := maxLevelFor(bid, int(cityType))
		if int(cityType) == 0 && isResourceField(bid) {
			// 高级建筑图纸（mem_city_buffer buftype=10001）开放普通城池资源田至 12 级
			active, err := s.db.Exists(ctx,
				"select 1 from city_buffers where city_id=? and buftype=10001 and endtime>=unix_timestamp() limit 1", cid)
			if err != nil {
				return nil, err
			}
			if active {
				maxlevel = 12
			}
		}
		if int(cityType) == 0 && isMonarchBonusBid(bid) {
			if ok, err := s.checkHeroLevel(ctx, uid, 4, 30); err == nil && ok {
				maxlevel = 15
			}
		}
		if dstlevel > maxlevel {
			return nil, httpx.BadRequest("no_advanced_construction_plan",
				"升级失败！普通城池需要使用“高级建筑图纸”之后，才能将城外资源建筑升至10级以上。")
		}
	}

	upgradeNeed, err := s.db.FetchOne(ctx,
		"select * from cfg_building_levels where bid=? and level=?", dstbid, dstlevel)
	if errors.Is(err, sql.ErrNoRows) {
		upgradeNeed = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}

	// legacy：仅当 upgrade_need 存在且 upgrade_time 非 0 才真正开建，否则静默返回建筑列表。
	if upgradeNeed != nil && model.Int64(upgradeNeed, "upgrade_time") != 0 {
		wood := model.Int64(upgradeNeed, "upgrade_wood")
		rock := model.Int64(upgradeNeed, "upgrade_rock")
		iron := model.Int64(upgradeNeed, "upgrade_iron")
		food := model.Int64(upgradeNeed, "upgrade_food")
		gold := model.Int64(upgradeNeed, "upgrade_gold")
		peopleNeed := model.Int64(upgradeNeed, "upgrade_people")

		haskaogongji, err := s.isUsingKaoGongJi(ctx, uid, cid, dstlevel)
		if err != nil {
			return nil, err
		}
		if haskaogongji {
			// 原版仅对木/石/铁/粮打 0.7 折（floor），金/人口不打折。
			wood = int64(math.Floor(float64(wood) * 0.7))
			rock = int64(math.Floor(float64(rock) * 0.7))
			iron = int64(math.Floor(float64(iron) * 0.7))
			food = int64(math.Floor(float64(food) * 0.7))
		}

		enough, err := s.checkCityResource(ctx, cid, wood, rock, iron, food, gold)
		if err != nil {
			return nil, err
		}
		if !enough {
			return nil, httpx.BadRequest("resource_not_enough", "资源不足")
		}
		cityPeople, err := s.db.FetchCellInt64(ctx, "select people from city_resources where city_id=?", cid)
		if err != nil {
			return nil, err
		}
		if cityPeople < peopleNeed {
			return nil, httpx.BadRequest("people_not_enough", "人口不足，不能建造此建筑。")
		}

		// 同时建造/拆除的建筑数量上限（官府等级在 legacy 亦用固定值，不随等级变化）
		upgradingCount, err := s.db.FetchCellInt64(ctx, "select count(*) from buildings where city_id=? and state>0", cid)
		if err != nil {
			return nil, err
		}
		limitCount, err := s.buildQueueLimit(ctx, uid)
		if err != nil {
			return nil, err
		}
		has166, err := s.bufferActive(ctx, uid, 166)
		if err != nil {
			return nil, err
		}
		if upgradingCount >= limitCount {
			switch {
			case has166:
				return nil, httpx.BadRequest("upgrading_queue_full", "现在建造列表已满，不能再建造新的建筑了。")
			case limitCount == 2:
				return nil, httpx.BadRequest("ask_to_use_yaoyiling", "ask_to_use_yaoyiling")
			default: // limitCount==5
				return nil, httpx.BadRequest("ask_to_use_yaoyiling2", "ask_to_use_yaoyiling2")
			}
		}

		// 前置条件（建筑/科技/物品）
		if err := s.checkConditions(ctx, uid, cid, int(cityType), dstbid, dstlevel, true); err != nil {
			return nil, err
		}

		now, err := s.db.Now(ctx)
		if err != nil {
			return nil, err
		}
		speed, err := s.buildingSpeedRate(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		realTime := game.ScaledSeconds(model.Int64(upgradeNeed, "upgrade_time"), speed)

		if _, err := s.db.Exec(ctx,
			"update city_resources set wood=wood-?,rock=rock-?,iron=iron-?,food=food-?,gold=gold-? where city_id=?",
			wood, rock, iron, food, gold, cid); err != nil {
			return nil, err
		}

		var lastid int
		if exist != nil {
			if _, err := s.db.Exec(ctx,
				"update buildings set building_id=?, state=1, state_start_at=?, state_end_at=? where city_id=? and xy=?",
				bid, now, now+realTime, cid, xy); err != nil {
				return nil, err
			}
			lastid = model.Int(exist, "id")
		} else {
			id, err := s.db.Insert(ctx,
				"insert into buildings (city_id, xy, building_id, level, state, state_start_at, state_end_at) values (?,?,?,0,1,?,?)",
				cid, xy, bid, now, now+realTime)
			if err != nil {
				return nil, err
			}
			lastid = int(id)
		}
		if _, err := s.db.Exec(ctx,
			"insert into building_upgrading (id,cid,xy,bid,level,state_endtime) values (?,?,?,?,?,?) "+
				"on duplicate key update state_endtime=values(state_endtime)",
			lastid, cid, xy, bid, dstlevel, now+realTime); err != nil {
			return nil, err
		}
	}
	return s.listAfter(ctx, cid)
}

// Destroy 对齐 startDestroyBuilding（BuildingFunc.php:661）：拆除一级（state=2，耗时 1% 升级时间）。
func (s *Service) Destroy(ctx context.Context, uid, cid, bid, inner, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	// checkBigCityDestroy：玩家主城（type=5）的官府不可拆除
	cityType, err := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	if int(cityType) == 5 && isGoverment(bid) {
		return nil, httpx.BadRequest("bigcity_destrotybigcity", "主城无法拆除官府！")
	}
	xy := xyTag(x, y)

	row, err := s.db.FetchOne(ctx, "select * from buildings where city_id=? and xy=? and building_id=?", cid, xy, bid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("nobuilding", "不存在该建筑")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(row)
	if b.State == 1 {
		return nil, httpx.BadRequest("upgrading", "建筑正在升级中")
	}
	if b.State == 2 {
		return nil, httpx.BadRequest("destroying", "建筑正在拆除中")
	}

	upgradingCount, err := s.db.FetchCellInt64(ctx, "select count(*) from buildings where city_id=? and state>0", cid)
	if err != nil {
		return nil, err
	}
	limitCount, err := s.buildQueueLimit(ctx, uid)
	if err != nil {
		return nil, err
	}
	has166, err := s.bufferActive(ctx, uid, 166)
	if err != nil {
		return nil, err
	}
	if upgradingCount >= limitCount {
		// 原版：非 166 分支 limitCount==5 与 else 文案相同（upgrading_queue_full2）
		if has166 {
			return nil, httpx.BadRequest("upgrading_queue_full", "现在建造列表已满，不能再建造新的建筑了。")
		}
		return nil, httpx.BadRequest("upgrading_queue_full2",
			"现在建造列表已满，不能再建造新的建筑了。使用“徭役令”或者“高级徭役令”可以增加建造队列。")
	}

	dstbid := b.BID
	dstlevel := b.Level
	if isGoverment(dstbid) && dstlevel == 1 {
		return nil, httpx.BadRequest("govenment_1_destroy", "1级官府不能拆除。")
	}

	upgradeNeed, err := s.db.FetchOne(ctx,
		"select * from cfg_building_levels where bid=? and level=?", dstbid, dstlevel)
	if errors.Is(err, sql.ErrNoRows) {
		upgradeNeed = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}
	if upgradeNeed == nil {
		return nil, httpx.NotFound("nobuilding", "不存在该建筑")
	}

	speed, err := s.buildingSpeedRate(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	// legacy：real_time = floor(upgrade_time*0.01 * speedRate / GAME_SPEED_RATE)
	realTime := int64(math.Floor(float64(model.Int64(upgradeNeed, "upgrade_time")) * 0.01 * speed / float64(game.SpeedRate)))

	if _, err := s.db.Exec(ctx,
		"update buildings set state=2, state_start_at=?, state_end_at=? where city_id=? and xy=?",
		now, now+realTime, cid, xy); err != nil {
		return nil, err
	}
	targetLevel := dstlevel - 1
	if _, err := s.db.Exec(ctx,
		"insert into building_destroying (id,cid,xy,bid,level,state_endtime) values (?,?,?,?,?,?) "+
			"on duplicate key update state_endtime=values(state_endtime)",
		model.Int(row, "id"), cid, xy, dstbid, targetLevel, now+realTime); err != nil {
		return nil, err
	}
	return s.listAfter(ctx, cid)
}

// DestroyAll 对齐 startDestroyBuildingAll（BuildingFunc.php:625）：彻底拆除（消耗 火油桶 gid=83，即时）。
//
// ⚠ 原版怪癖（1:1 保留）：useFireBarrel 只把 sys_building 置 state=2、state_endtime=now（不改 level），
// 拆除完成由 getCityBuildingInfo 的 `level=level-1` 结算 → 事实上对 level>1 的建筑仅降一级（level=1 归零后
// 被删除）。mem_building_destroying.level 虽写 0，但结算 SQL 未使用该值。
func (s *Service) DestroyAll(ctx context.Context, uid, cid, bid, inner, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if isGoverment(bid) {
		return nil, httpx.BadRequest("govenment_all_destroy", "官府不能彻底拆除。")
	}
	xy := xyTag(x, y)

	row, err := s.db.FetchOne(ctx, "select * from buildings where city_id=? and xy=? and building_id=?", cid, xy, bid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.NotFound("nobuilding", "不存在该建筑")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(row)
	if b.State == 1 {
		return nil, httpx.BadRequest("upgrading", "建筑正在升级中")
	}
	if b.State == 2 {
		return nil, httpx.BadRequest("destroying", "建筑正在拆除中")
	}

	if err := s.useFireBarrel(ctx, uid, cid, xy, b.BID, model.Int(row, "id")); err != nil {
		return nil, err
	}
	// legacy：bid==ID_BUILDING_HONGLU(12) 时召回鸿胪寺驻军（sys_city_hero/sys_troops）。
	// 新库重写映射无鸿胪寺建筑，且 troops 模块口径不同 → 此分支惰性化（见 legacy_bid.go）。
	return s.listAfter(ctx, cid)
}

// CancelDestroy 对齐 stopDestroyBuilding（BuildingFunc.php:733）：取消拆除，恢复 state=0。
func (s *Service) CancelDestroy(ctx context.Context, uid, cid, bid, inner, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	xy := xyTag(x, y)
	// legacy：delete from mem_building_destroying where id=(select id from sys_building where cid,xy,state=2)
	if _, err := s.db.Exec(ctx,
		"delete from building_destroying where id in (select id from buildings where city_id=? and xy=? and state=2)",
		cid, xy); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		"update buildings set state=0 where city_id=? and xy=? and state=2", cid, xy); err != nil {
		return nil, err
	}
	return s.listAfter(ctx, cid)
}

// Exchange 对齐 startChangeBuilding（BuildingFunc.php:748）：资源地转换（消耗 资源地转换令 gid=161504）。
// 注意：原版命令名为 startChangeBuilding，语义为“资源地转换”而非换位置。
func (s *Service) Exchange(ctx context.Context, uid, cid, bid, targetbid, inner, x, y int) ([]model.Building, error) {
	const biandifugid = 161504
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	xy := xyTag(x, y)

	exist, err := s.db.FetchOne(ctx, "select * from buildings where city_id=? and xy=? and building_id=?", cid, xy, bid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, httpx.BadRequest("building_error", "建造建筑错误")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(exist)
	if b.State != 0 {
		return nil, httpx.BadRequest("upgrading", "建筑正在升级中")
	}

	ok, err := s.checkGoods(ctx, uid, biandifugid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.BadRequest("not_enough_goods161504", "not_enough_goods161504")
	}

	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	const realTime = 5
	if _, err := s.db.Exec(ctx,
		"update buildings set building_id=?, state=1, state_start_at=?, state_end_at=? where city_id=? and xy=? and building_id=?",
		targetbid, now, now+realTime, cid, xy, bid); err != nil {
		return nil, err
	}
	dstlevel := b.Level
	if dstlevel > 20 {
		dstlevel = 20
	}
	if _, err := s.db.Exec(ctx,
		"insert into building_upgrading (id,cid,xy,bid,level,state_endtime) values (?,?,?,?,?,?) "+
			"on duplicate key update state_endtime=values(state_endtime)",
		model.Int(exist, "id"), cid, xy, targetbid, dstlevel, now+realTime); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update city_res_add set resource_changing=1 where city_id=?", cid); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update city_resources set changing=1 where city_id=?", cid); err != nil {
		return nil, err
	}
	if err := s.reduceGoods(ctx, uid, biandifugid, 1); err != nil {
		return nil, err
	}
	return s.listAfter(ctx, cid)
}

// QueueItem 建筑队列行（state>0 的在建/在拆建筑）。
type QueueItem struct {
	CID           int    `json:"cid"`
	BID           int    `json:"bid"`
	Name          string `json:"bname"`
	X             int    `json:"x"`
	Y             int    `json:"y"`
	State         int    `json:"state"`
	Task          string `json:"task"`
	CurrentLevel  int    `json:"current_level"`
	TargetLevel   int    `json:"target_level"`
	StateEndTime  int64  `json:"state_endtime"`
	StateTimeLeft int64  `json:"state_timeleft"`
}

// Queue 建筑队列查询。legacy 无独立命令：队列即 getCityBuildingInfo 中 state>0 的建筑（GovernmentPanel/
// TopPanel 亦复用该回包），此处按 state 拼装任务名（正在建造/正在升级/正在拆除，对齐 GovernmentPanelSrc）。
func (s *Service) Queue(ctx context.Context, uid, cid int) ([]QueueItem, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.settleAll(ctx, cid); err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx,
		"select b.*, c.name as bname from buildings b left join cfg_buildings c on c.bid=b.building_id "+
			"where b.city_id=? and b.state>0 order by b.state_end_at", cid)
	if err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	// 队列表的目标等级（upgrading.level = 目标级；destroying.level = 目标剩余级）
	upTarget := map[int]int{}
	if up, err := s.db.FetchRows(ctx, "select id, level from building_upgrading where cid=?", cid); err == nil {
		for _, r := range up {
			upTarget[model.Int(r, "id")] = model.Int(r, "level")
		}
	}
	downTarget := map[int]int{}
	if dn, err := s.db.FetchRows(ctx, "select id, level from building_destroying where cid=?", cid); err == nil {
		for _, r := range dn {
			downTarget[model.Int(r, "id")] = model.Int(r, "level")
		}
	}
	out := make([]QueueItem, 0, len(rows))
	for _, r := range rows {
		b := model.BuildingFromMap(r)
		rid := model.Int(r, "id")
		item := QueueItem{
			CID: b.CID, BID: b.BID, Name: b.Name, X: b.X, Y: b.Y,
			State: b.State, CurrentLevel: b.Level,
			StateEndTime: b.StateEndTime, StateTimeLeft: b.StateEndTime - now,
		}
		if b.State == 1 {
			item.TargetLevel = upTarget[rid]
			if item.TargetLevel <= 1 {
				item.Task = "正在建造"
			} else {
				item.Task = "正在升级"
			}
		} else { // state==2
			item.TargetLevel = downTarget[rid]
			item.Task = "正在拆除"
		}
		out = append(out, item)
	}
	return out, nil
}

// ── 内部工具 ────────────────────────────────────────────────────────────────

// xyTag 把 0-based 坐标编码为新库建筑标签（model.xyFrom 的等价实现）。
func xyTag(x, y int) string {
	return string(rune('a'+x)) + string(rune('1'+y))
}

// settleAll 对齐 getCityBuildingInfo+getBuildingTasks 的惰性结算（utils.php:776-782 / 1964-1968）。
func (s *Service) settleAll(ctx context.Context, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	// 升级完成：level+1
	if _, err := s.db.Exec(ctx,
		"update buildings set state=0, level=level+1 where state=1 and city_id=? and state_end_at<=?", cid, now); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "delete from building_upgrading where cid=? and state_endtime<=?", cid, now); err != nil {
		return err
	}
	// 拆除完成：level-1
	if _, err := s.db.Exec(ctx,
		"update buildings set state=0, level=level-1 where state=2 and city_id=? and state_end_at<=?", cid, now); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx, "delete from building_destroying where cid=? and state_endtime<=?", cid, now); err != nil {
		return err
	}
	// 归零建筑清除
	_, err = s.db.Exec(ctx, "delete from buildings where city_id=? and level=0 and state=0", cid)
	return err
}

// listAfter 结算后返回建筑列表（= legacy 各函数末尾的 return getCityBuildingInfo 再 order 后的列表）。
func (s *Service) listAfter(ctx context.Context, cid int) ([]model.Building, error) {
	if err := s.settleAll(ctx, cid); err != nil {
		return nil, err
	}
	return s.List(ctx, cid)
}

// maxLevelFor 对齐 getBuildingMaxLevel（BuildingFunc.php:97），入参为【新库 building_id】。
func maxLevelFor(newBid, cityType int) int {
	legacy, ok := legacyBidOf(newBid)
	maxlevel := 10
	if ok && legacy < IDBuildingHouse { // legacy bid<5 → 资源田
		switch cityType {
		case 1:
			maxlevel = 12
		case 2, 5:
			maxlevel = 15
		case 3:
			maxlevel = 18
		case 4:
			maxlevel = 20
		}
	} else if !ok || legacy >= IDBuildingHouse {
		if cityType == 5 {
			maxlevel = 15
		}
	}
	return maxlevel
}

// isUsingKaoGongJi 对齐 BuildingFunc.php:69（考工记，user_buffers buftype 12/13/14，按目标等级分档）。
func (s *Service) isUsingKaoGongJi(ctx context.Context, uid, cid, dstlevel int) (bool, error) {
	var q string
	switch {
	case dstlevel <= 5:
		q = "select 1 from user_buffers where user_id=? and buftype in (12,13,14) and endtime>unix_timestamp() limit 1"
	case dstlevel <= 8:
		q = "select 1 from user_buffers where user_id=? and buftype in (13,14) and endtime>unix_timestamp() limit 1"
	default:
		q = "select 1 from user_buffers where user_id=? and buftype=14 and endtime>unix_timestamp() limit 1"
	}
	return s.db.Exists(ctx, q, uid)
}

// bufferActive user_buffers 中该 buftype 是否存在有效（不判 endtime，对齐 legacy `!empty` 语义由调用方判）。
func (s *Service) bufferActive(ctx context.Context, uid, buftype int) (bool, error) {
	return s.db.Exists(ctx, "select 1 from user_buffers where user_id=? and buftype=? limit 1", uid, buftype)
}

// buildQueueLimit 对齐 startUpgradeBuilding 队列上限：默认 2 / 徭役令(11) 5 / 高级徭役令(166) 7。
// 注意 legacy 中 166 的 endtime 用 `endtime>unix_timestamp()` 判定，11 亦同；此处以有效 buff 计。
func (s *Service) buildQueueLimit(ctx context.Context, uid int) (int64, error) {
	limit := int64(2)
	if ok, err := s.db.Exists(ctx,
		"select 1 from user_buffers where user_id=? and buftype=11 and endtime>unix_timestamp() limit 1", uid); err != nil {
		return 0, err
	} else if ok {
		limit = 5
	}
	if ok, err := s.db.Exists(ctx,
		"select 1 from user_buffers where user_id=? and buftype=166 and endtime>unix_timestamp() limit 1", uid); err != nil {
		return 0, err
	} else if ok {
		limit = 7
	}
	return limit, nil
}

// checkHeroLevel 对齐 HeroFunc.php:2247。新库无 sys_user_level（君主将等级）→ userLevel 取 0。
func (s *Service) checkHeroLevel(ctx context.Context, uid, userLevel, heroLevel int) (bool, error) {
	mUserLevel := 0
	monarch, err := s.db.FetchCellInt64(ctx,
		"select level from heroes where user_id=? and hero_type=1000 limit 1", uid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return int(monarch) >= heroLevel && mUserLevel >= userLevel, nil
}

// buildingSpeedRate 对齐 getBuildingSpeedRate（BuildingFunc.php:15）。
// 衰减项：建筑技术17、城守内政、鬼斧神工技能（新库无 book 表→0）、文曲星符（无 mem_hero_buffer→false）、
// 君主将加成（无 sys_user_level→不触发）。
//
// ⚠ 原版怪癖（1:1 保留）：$cityhids 的三级回退（城守→军师→主将）结果被随后的
// `select chiefhid` 覆盖 → 实际只用城守（chief_hero_id）。
func (s *Service) buildingSpeedRate(ctx context.Context, uid, cid int) (float64, error) {
	speedAdd := 0.0

	// 建筑技术(17)：每级 +100%
	techLevel, err := s.db.FetchCellInt64(ctx, "select level from city_technics where city_id=? and technic_id=17", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if techLevel > 0 {
		speedAdd += float64(techLevel) * 100
	}

	// 城守内政
	chiefHid, err := s.db.FetchCellInt64(ctx, "select chief_hero_id from cities where id=?", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if chiefHid > 0 {
		chief, err := s.db.FetchOne(ctx, "select * from heroes where id=?", chiefHid)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if chief != nil {
			bufadd := 1.0 // isHeroHasBuffer(chiefhid,2)「文曲星符」：新库无 mem_hero_buffer → false
			speedAdd += (float64(model.Int(chief, "affairs_base"))+float64(model.Int(chief, "affairs_add")))*bufadd +
				float64(model.Int(chief, "affairs_add_on"))
		}
	}

	skillRate := 0.0 // 鬼斧神工（cfg_book/sys_user_book bid=15）：新库无表 → 0
	finalSpeed := (1.0 / (1.0 + 0.01*speedAdd)) * (1 - skillRate)

	if ok, err := s.checkHeroLevel(ctx, uid, 2, 10); err != nil {
		return 0, err
	} else if ok {
		finalSpeed *= 0.8
	}
	return finalSpeed, nil
}

// checkCityResource 对齐 utils.php:197：五资源是否充足。
func (s *Service) checkCityResource(ctx context.Context, cid int, wood, rock, iron, food, gold int64) (bool, error) {
	res, err := s.db.FetchOne(ctx, "select wood,rock,iron,food,gold from city_resources where city_id=?", cid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return model.Int64(res, "wood") >= wood && model.Int64(res, "rock") >= rock &&
		model.Int64(res, "iron") >= iron && model.Int64(res, "food") >= food &&
		model.Int64(res, "gold") >= gold, nil
}

// checkConditions 对齐 startUpgradeBuilding 的前置条件校验（BuildingFunc.php:497-532）。
// pre_type：0 建筑 / 1 科技 / 2 物品（物品条件在满足时立即扣减，与 legacy 一致）。
func (s *Service) checkConditions(ctx context.Context, uid, cid, cityType, dstbid, dstlevel int, deductGoods bool) error {
	conditions, err := s.db.FetchRows(ctx,
		"select * from cfg_building_conditions where bid=? and levelid=? order by pre_type", dstbid, dstlevel)
	if err != nil {
		return err
	}
	govNew, _ := newBidOf(IDBuildingGoverment)
	if cityType == 5 && dstlevel >= 11 && dstbid != govNew {
		conditions = append(conditions, map[string]any{
			"pre_type": int64(0), "pre_id": int64(IDBuildingGoverment), "pre_level": int64(dstlevel),
		})
	}
	for _, cond := range conditions {
		switch model.Int(cond, "pre_type") {
		case 0: // 建筑
			preNew, ok := newBidOf(model.Int(cond, "pre_id"))
			if !ok {
				return httpx.BadRequest("no_pre_building", "前提建筑没有建好。")
			}
			lv, err := s.db.FetchCellInt64(ctx,
				"select max(level) from buildings where city_id=? and building_id=?", cid, preNew)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if lv < int64(model.Int(cond, "pre_level")) {
				return httpx.BadRequest("no_pre_building", "前提建筑没有建好。")
			}
		case 1: // 科技
			lv, err := s.db.FetchCellInt64(ctx,
				"select max(level) from city_technics where city_id=? and technic_id=?", cid, model.Int(cond, "pre_id"))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if lv < int64(model.Int(cond, "pre_level")) {
				return httpx.BadRequest("no_pre_technic", "前提科技没有研究好。")
			}
		case 2: // 物品
			need := int64(model.Int(cond, "pre_level"))
			cnt, err := s.goodsCount(ctx, uid, model.Int(cond, "pre_id"))
			if err != nil {
				return err
			}
			if cnt < need {
				return httpx.BadRequest("no_pre_thing", "你没有相应的任务物品。")
			}
			if deductGoods {
				if err := s.addGoodsDelta(ctx, uid, model.Int(cond, "pre_id"), -need, 0); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// useFireBarrel 对齐 GoodsFunc.php:3200：彻底拆除消耗 火油桶(gid=83)，即时置 state=2。
func (s *Service) useFireBarrel(ctx context.Context, uid, cid int, xy string, bid, buildingID int) error {
	ok, err := s.checkGoods(ctx, uid, 83)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.BadRequest("not_enough_goods83", "not_enough_goods83")
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	// legacy：real_time_need=0
	if _, err := s.db.Exec(ctx,
		"update buildings set state=2, state_start_at=?, state_end_at=? where city_id=? and xy=?",
		now, now, cid, xy); err != nil {
		return err
	}
	if _, err := s.db.Exec(ctx,
		"insert into building_destroying (id,cid,xy,bid,level,state_endtime) values (?,?,?,?,0,?) "+
			"on duplicate key update state_endtime=values(state_endtime)",
		buildingID, cid, xy, bid, now); err != nil {
		return err
	}
	// legacy：bid==20（城墙）删除城防/加固队列；新库重写映射无城墙建筑 → 惰性化。
	return s.reduceGoods(ctx, uid, 83, 1)
}

// goodsCount 读取 user_goods.count（无行 0），对齐 goods 包同名实现。
func (s *Service) goodsCount(ctx context.Context, uid, gid int) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, "select `count` from user_goods where user_id=? and gid=?", uid, gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// checkGoods 对齐 utils.php:933：拥有数量 >=1。
func (s *Service) checkGoods(ctx context.Context, uid, gid int) (bool, error) {
	cnt, err := s.goodsCount(ctx, uid, gid)
	if err != nil {
		return false, err
	}
	return cnt >= 1, nil
}

// reduceGoods 对齐 utils.php:938：写 log_goods(-cnt)，count=GREATEST(0,count-cnt)。
func (s *Service) reduceGoods(ctx context.Context, uid, gid int, cnt int64) error {
	if cnt <= 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),0)",
		uid, gid, -cnt); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"update user_goods set `count`=GREATEST(0,`count`-?) where user_id=? and gid=?", cnt, uid, gid)
	return err
}

// addGoodsDelta 对齐 utils.php:971 addGoods 的普通分支（upsert count+=delta + log_goods）。
func (s *Service) addGoodsDelta(ctx context.Context, uid, gid int, delta int64, typ int) error {
	if delta == 0 {
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"insert into user_goods (user_id, gid, `count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		uid, gid, delta, delta); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx,
		"insert into log_goods (user_id, gid, `count`, `time`, `type`) values (?,?,?,unix_timestamp(),?)",
		uid, gid, delta, typ)
	return err
}
