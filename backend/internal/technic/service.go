package technic

import (
	"context"
	"database/sql"
	"math"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// Service 复刻 legacy TechnicFunc.php 的科技研究逻辑（新库 technics/cfg_technic_levels 表）。
// 说明：新库 cfg_technic_conditions 为空表，前置条件校验（建筑/科技/全局任务）不适用，故省略。
type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// Item 对齐 legacy TechnicState（interface.php:97）。
type Item struct {
	TID                  int    `json:"tid"`
	TName                string `json:"tname"`
	CID                  int    `json:"cid"`
	Description          string `json:"description"`
	Level                int    `json:"level"`
	ShareLevel           int    `json:"sharelevel"`
	State                int    `json:"state"`
	StateEndTime         int64  `json:"state_endtime"`
	StateTimeLeft        int64  `json:"state_timeleft"`
	CanUpgrade           bool   `json:"can_upgrade"`
	NoUpgradeMsg         string `json:"no_upgrade_msg"`
	LevelDescription     string `json:"levelDescription"`
	NextLevelDescription string `json:"nextLevelDescription"`
	WoodNeed             int64  `json:"woodNeed"`
	RockNeed             int64  `json:"rockNeed"`
	IronNeed             int64  `json:"ironNeed"`
	FoodNeed             int64  `json:"foodNeed"`
	GoldNeed             int64  `json:"goldNeed"`
	UpgradeTime          int64  `json:"upgrade_time"`
}

// Info 对齐 doGetTechnicInfo 的 [techlist, collegeCount] 返回。
type Info struct {
	Technics     []Item `json:"technics"`
	CollegeCount int    `json:"collegeCount"`
}

// collegeBID 书院建筑 ID（cfg_buildings: 7书院）。
const collegeBID = 7

// researchTID 研究技巧，每级使其它科技研究时间 -3%（TechnicFunc.php:279）。
const researchTID = 24

// ensureOwner 对齐 checkCityExist/checkCityOwner。
func (s *Service) ensureOwner(ctx context.Context, uid, cid int) error {
	ok, err := s.db.Exists(ctx, "select 1 from cities where id=? and user_id=? limit 1", cid, uid)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.Forbidden("not_user_city", "该城池不属于当前用户")
	}
	return nil
}

// Settle 惰性结算到期科技（等价 legacy ReportCron.php:347 UpdateUsersTechnic）。
// 新栈无 cron，故在每个读写科技的请求入口调用；并同步城共享科技等级（legacy checkUsersTechnic）。
func (s *Service) Settle(ctx context.Context, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	affected, err := s.db.Exec(ctx,
		"update technics set level=level+1, state=0 where city_id=? and state=1 and state_end_at>0 and state_end_at<=?",
		cid, now)
	if err != nil {
		return err
	}
	if affected == 0 {
		return nil
	}
	rows, err := s.db.FetchRows(ctx, "select technic_id, level from technics where city_id=? and state=0", cid)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := s.db.Exec(ctx,
			"replace into city_technics (city_id, technic_id, level) values (?,?,?)",
			cid, model.Int(r, "technic_id"), model.Int64(r, "level")); err != nil {
			return err
		}
	}
	return nil
}

// speedRate 对齐 getTechnicSpeedRate（TechnicFunc.php:5）的智谋加速。
// 新库无符/墨家真传表，仅取军师→主将→城守的 wisdom_base+wisdom_add。
func (s *Service) speedRate(ctx context.Context, cid int) float64 {
	city, err := s.db.FetchOne(ctx, "select counsellor_hero_id, general_hero_id, chief_hero_id from cities where id=?", cid)
	if err != nil {
		return 1
	}
	hid := model.Int(city, "counsellor_hero_id")
	if hid == 0 {
		hid = model.Int(city, "general_hero_id")
	}
	if hid == 0 {
		hid = model.Int(city, "chief_hero_id")
	}
	if hid == 0 {
		return 1
	}
	hero, err := s.db.FetchOne(ctx, "select wisdom_base, wisdom_add from heroes where id=?", hid)
	if err != nil {
		return 1
	}
	add := float64(model.Int(hero, "wisdom_base") + model.Int(hero, "wisdom_add"))
	return 1.0 / (1.0 + 0.01*add)
}

// realSeconds 对齐 getRealTime（TechnicFunc.php:275）：基准时间 × 速度系数 ÷ 倍速，研究技巧再加速，向下取整。
func realSeconds(raw int64, speedRate float64, researchLevel int) int64 {
	if raw <= 0 {
		return 0
	}
	t := float64(raw) * speedRate / float64(game.SpeedRate)
	if researchLevel > 0 {
		t = t / 100 * (100 - float64(researchLevel)*3)
	}
	if t < 1 {
		t = 1
	}
	return int64(math.Floor(t))
}

// ownRow 玩家（跨城共享）的科技等级行。
func (s *Service) ownRow(ctx context.Context, uid, tid int) (map[string]any, error) {
	return s.db.FetchOne(ctx, "select * from technics where user_id=? and technic_id=?", uid, tid)
}

// List 对齐 doGetTechnicInfo（TechnicFunc.php:49）。
func (s *Service) List(ctx context.Context, uid, cid int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	rate := s.speedRate(ctx, cid)

	// 研究技巧当前等级（用户级）。
	researchLevel := 0
	if row, err := s.ownRow(ctx, uid, researchTID); err == nil {
		researchLevel = model.Int(row, "level")
	}

	hasUpgrading, err := s.db.Exists(ctx, "select 1 from technics where city_id=? and state=1", cid)
	if err != nil {
		return nil, err
	}

	cfgs, err := s.db.FetchRows(ctx, "select * from cfg_technics order by tid")
	if err != nil {
		return nil, err
	}
	shareRows, err := s.db.FetchRows(ctx, "select technic_id, level from city_technics where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	shareLevel := make(map[int]int, len(shareRows))
	for _, r := range shareRows {
		shareLevel[model.Int(r, "technic_id")] = model.Int(r, "level")
	}

	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	items := make([]Item, 0, len(cfgs))
	for _, cfg := range cfgs {
		tid := model.Int(cfg, "tid")
		it := Item{
			TID:        tid,
			TName:      model.Str(cfg, "name"),
			CID:        cid,
			Description: model.Str(cfg, "description"),
			ShareLevel: shareLevel[tid],
			CanUpgrade: true,
		}
		if own, err := s.ownRow(ctx, uid, tid); err == nil {
			it.Level = model.Int(own, "level")
			it.State = model.Int(own, "state")
			it.StateEndTime = model.Int64(own, "state_end_at")
			if it.State == 1 {
				it.StateTimeLeft = it.StateEndTime - now
				if it.StateTimeLeft < 0 {
					it.StateTimeLeft = 0
				}
			}
		}
		if it.Level > 0 {
			it.LevelDescription, _ = s.db.FetchCellString(ctx,
				"select description from cfg_technic_levels where tid=? and level=?", tid, it.Level)
		}

		dstLevel := it.Level + 1
		if dstLevel > 10 {
			it.CanUpgrade = false
			it.NoUpgradeMsg = "科技等级已达上限"
		}
		need, err := s.db.FetchOne(ctx, "select * from cfg_technic_levels where tid=? and level=?", tid, dstLevel)
		if err == nil && need != nil {
			it.WoodNeed = model.Int64(need, "upgrade_wood")
			it.RockNeed = model.Int64(need, "upgrade_rock")
			it.IronNeed = model.Int64(need, "upgrade_iron")
			it.FoodNeed = model.Int64(need, "upgrade_food")
			it.GoldNeed = model.Int64(need, "upgrade_gold")
			it.UpgradeTime = realSeconds(model.Int64(need, "upgrade_time"), rate, researchLevel)
			it.NextLevelDescription = model.Str(need, "description")
			it.CanUpgrade = enough(res, it)
			if !it.CanUpgrade {
				it.NoUpgradeMsg = "资源不足"
			}
		} else if it.CanUpgrade {
			it.CanUpgrade = false
			it.NoUpgradeMsg = "暂无可升级配置"
		}
		if it.State == 1 {
			it.CanUpgrade = false
			it.NoUpgradeMsg = "正在研究中"
		}
		if hasUpgrading {
			it.CanUpgrade = false
			if it.State != 1 {
				it.NoUpgradeMsg = "本城已有科技正在研究"
			}
		}
		items = append(items, it)
	}

	college, err := s.db.FetchCellInt64(ctx,
		"select count(*) from buildings b join cities c on c.id=b.city_id where c.user_id=? and b.building_id=?",
		uid, collegeBID)
	if err != nil {
		return nil, err
	}

	return &Info{Technics: items, CollegeCount: int(college)}, nil
}

// enough 判断城市资源是否满足需求（对齐 checkCityResource）。
func enough(res map[string]any, it Item) bool {
	if res == nil {
		return false
	}
	return model.Int64(res, "wood") >= it.WoodNeed &&
		model.Int64(res, "rock") >= it.RockNeed &&
		model.Int64(res, "iron") >= it.IronNeed &&
		model.Int64(res, "food") >= it.FoodNeed &&
		model.Int64(res, "gold") >= it.GoldNeed
}

// Upgrade 对齐 startUpgradeTechnic（TechnicFunc.php:180）。
func (s *Service) Upgrade(ctx context.Context, uid, cid, tid int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	ok, err := s.db.Exists(ctx, "select 1 from cfg_technics where tid=? limit 1", tid)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.BadRequest("no_technic_info", "没有该科技")
	}
	hasUpgrading, err := s.db.Exists(ctx, "select 1 from technics where city_id=? and state=1", cid)
	if err != nil {
		return nil, err
	}
	if hasUpgrading {
		return nil, httpx.BadRequest("only_analysis_1_tech", "本城已有科技正在研究")
	}

	own, err := s.ownRow(ctx, uid, tid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	dstLevel := 1
	if own != nil {
		dstLevel = model.Int(own, "level") + 1
	}
	if dstLevel > 10 {
		return nil, httpx.BadRequest("technic_full", "科技等级已达上限")
	}

	need, err := s.db.FetchOne(ctx, "select * from cfg_technic_levels where tid=? and level=?", tid, dstLevel)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_technic_info", "暂无可升级配置")
	}
	if err != nil {
		return nil, err
	}
	wood := model.Int64(need, "upgrade_wood")
	rock := model.Int64(need, "upgrade_rock")
	iron := model.Int64(need, "upgrade_iron")
	food := model.Int64(need, "upgrade_food")
	gold := model.Int64(need, "upgrade_gold")

	researchLevel := 0
	if row, err := s.ownRow(ctx, uid, researchTID); err == nil {
		researchLevel = model.Int(row, "level")
	}
	upTime := realSeconds(model.Int64(need, "upgrade_time"), s.speedRate(ctx, cid), researchLevel)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var res struct {
		Wood int64 `db:"wood"`
		Rock int64 `db:"rock"`
		Iron int64 `db:"iron"`
		Food int64 `db:"food"`
		Gold int64 `db:"gold"`
	}
	if err := tx.QueryRowxContext(ctx, "select wood,rock,iron,food,gold from city_resources where city_id=? for update", cid).
		StructScan(&res); err != nil {
		if err == sql.ErrNoRows {
			return nil, httpx.NotFound("no_city_info", "城市资源不存在")
		}
		return nil, err
	}
	if res.Wood < wood || res.Rock < rock || res.Iron < iron || res.Food < food || res.Gold < gold {
		return nil, httpx.BadRequest("no_enough_resource", "资源不足")
	}

	var now int64
	if err := tx.QueryRowxContext(ctx, "select unix_timestamp()").Scan(&now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"update city_resources set wood=wood-?,rock=rock-?,iron=iron-?,food=food-?,gold=gold-? where city_id=?",
		wood, rock, iron, food, gold, cid); err != nil {
		return nil, err
	}
	if own != nil {
		if _, err := tx.ExecContext(ctx,
			"update technics set city_id=?, state=1, state_start_at=?, state_end_at=? where user_id=? and technic_id=?",
			cid, now, now+upTime, uid, tid); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			"insert into technics (user_id, city_id, technic_id, level, state, state_start_at, state_end_at) values (?,?,?,0,1,?,?)",
			uid, cid, tid, now, now+upTime); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.List(ctx, uid, cid)
}

// Stop 对齐 stopUpgradeTechnic（TechnicFunc.php:287）：取消研究并返还 66% 资源。
func (s *Service) Stop(ctx context.Context, uid, cid, tid int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	row, err := s.db.FetchOne(ctx, "select * from technics where city_id=? and technic_id=? and state=1", cid, tid)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_upgrading_tech_info", "没有正在研究的科技")
	}
	if err != nil {
		return nil, err
	}
	level := model.Int(row, "level")
	need, err := s.db.FetchOne(ctx, "select * from cfg_technic_levels where tid=? and level=?", tid, level+1)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_technic_info", "暂无可升级配置")
	}
	if err != nil {
		return nil, err
	}
	wood := model.Int64(need, "upgrade_wood") * 66 / 100
	rock := model.Int64(need, "upgrade_rock") * 66 / 100
	iron := model.Int64(need, "upgrade_iron") * 66 / 100
	food := model.Int64(need, "upgrade_food") * 66 / 100
	gold := model.Int64(need, "upgrade_gold") * 66 / 100

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		"update city_resources set wood=wood+?,rock=rock+?,iron=iron+?,food=food+?,gold=gold+? where city_id=?",
		wood, rock, iron, food, gold, cid); err != nil {
		return nil, err
	}
	if level == 0 {
		if _, err := tx.ExecContext(ctx, "delete from technics where user_id=? and technic_id=?", uid, tid); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.ExecContext(ctx,
			"update technics set state=0, state_start_at=0, state_end_at=0 where user_id=? and technic_id=?",
			uid, tid); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.List(ctx, uid, cid)
}