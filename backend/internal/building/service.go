package building

import (
	"context"
	"database/sql"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// Service 复刻 legacy BuildingFunc.php 的建筑升级逻辑（新库 buildings/cfg_building_levels 表）。
type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// UpgradeInfo 对齐 legacy BuildingInfo（interface.php:60）。
type UpgradeInfo struct {
	BID          int    `json:"bid"`
	Name         string `json:"name"`
	Level        int    `json:"level"`
	LevelDesc    string `json:"level_description"`
	WoodNeed     int64  `json:"woodNeed"`
	RockNeed     int64  `json:"rockNeed"`
	IronNeed     int64  `json:"ironNeed"`
	FoodNeed     int64  `json:"foodNeed"`
	GoldNeed     int64  `json:"goldNeed"`
	PeopleNeed   int64  `json:"peopleNeed"`
	UpgradeTime  int64  `json:"upgradeTime"`
	CanUpgrade   bool   `json:"canUpgrade"`
	NoUpgradeMsg string `json:"no_upgrade_msg"`
}

// Detail 是单个建筑的当前状态与下一级升级信息。
type Detail struct {
	BID           int          `json:"bid"`
	Name          string       `json:"name"`
	X             int          `json:"x"`
	Y             int          `json:"y"`
	Level         int          `json:"level"`
	State         int          `json:"state"`
	StateEndTime  int64        `json:"state_endtime"`
	StateTimeLeft int64        `json:"state_timeleft"`
	Next          *UpgradeInfo `json:"next"`
}

// xyFrom 把 0-based 坐标还原为建筑标签（ParseXY 的逆运算）。
func xyFrom(x, y int) string {
	return string(rune('a'+x)) + string(rune('1'+y))
}

// maxLevel 对齐 getBuildingMaxLevel（BuildingFunc.php:97），新库城池 type 恒为 0。
func maxLevel(cityType, bid int) int {
	max := 10
	if bid < 5 {
		switch cityType {
		case 0, 1:
			max = 12
		case 2, 5:
			max = 15
		case 3:
			max = 18
		case 4:
			max = 20
		}
	} else if cityType == 5 {
		max = 15
	}
	return max
}

// Settle 惰性结算：把已到期的升级落库（等价 legacy getBuildingTasks，utils.php:1964）。
// 新栈无 cron，故在每个读写建筑的请求入口调用。
func (s *Service) Settle(ctx context.Context, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx,
		"update buildings set level=level+1, state=0 where city_id=? and state=1 and state_end_at>0 and state_end_at<=?",
		cid, now)
	return err
}

// List 对齐 getCityBuildingInfo（utils.php:776）。
func (s *Service) List(ctx context.Context, cid int) ([]model.Building, error) {
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx,
		"select b.*, c.name as bname from buildings b left join cfg_buildings c on c.bid=b.building_id where b.city_id=? order by b.xy",
		cid)
	if err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Building, 0, len(rows))
	for _, r := range rows {
		b := model.BuildingFromMap(r)
		if b.State == 1 {
			b.StateTimeLeft = b.StateEndTime - now
		}
		out = append(out, b)
	}
	return out, nil
}

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

// findAt 取指定格子上的建筑行；不存在返回 sql.ErrNoRows。
func (s *Service) findAt(ctx context.Context, cid, x, y int) (map[string]any, error) {
	return s.db.FetchOne(ctx, "select * from buildings where city_id=? and xy=?", cid, xyFrom(x, y))
}

// Detail 对齐 doGetBuildingInfo（BuildingFunc.php:239）的单建筑信息。
func (s *Service) Detail(ctx context.Context, uid, cid, bid, x, y int) (*Detail, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	row, err := s.findAt(ctx, cid, x, y)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("nobuilding", "该位置没有建筑")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(row)
	if b.BID != bid {
		return nil, httpx.BadRequest("building_error", "该位置是其它建筑")
	}

	name, _ := s.db.FetchCellString(ctx, "select name from cfg_buildings where bid=?", bid)
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}

	d := &Detail{BID: b.BID, Name: name, X: b.X, Y: b.Y, Level: b.Level, State: b.State, StateEndTime: b.StateEndTime}
	if b.State == 1 {
		d.StateTimeLeft = b.StateEndTime - now
	}
	d.Next = s.nextInfo(ctx, cid, b, name)
	return d, nil
}

// nextInfo 计算下一级升级需求与可否升级的理由，不抛错（理由写入 NoUpgradeMsg）。
func (s *Service) nextInfo(ctx context.Context, cid int, b model.Building, name string) *UpgradeInfo {
	next := &UpgradeInfo{BID: b.BID, Name: name, Level: b.Level + 1, CanUpgrade: true}

	cfg, err := s.db.FetchOne(ctx,
		"select * from cfg_building_levels where bid=? and level=?", b.BID, b.Level+1)
	if err != nil || cfg == nil {
		next.CanUpgrade = false
		if has, _ := s.db.Exists(ctx, "select 1 from cfg_building_levels where bid=? limit 1", b.BID); has {
			next.NoUpgradeMsg = "已达最高等级"
		} else {
			next.NoUpgradeMsg = "暂无可升级配置"
		}
		return next
	}

	cityType, _ := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if b.Level+1 > maxLevel(int(cityType), b.BID) {
		next.CanUpgrade = false
		next.NoUpgradeMsg = "需要更高级的官府"
		return next
	}

	next.WoodNeed = model.Int64(cfg, "upgrade_wood")
	next.RockNeed = model.Int64(cfg, "upgrade_rock")
	next.IronNeed = model.Int64(cfg, "upgrade_iron")
	next.FoodNeed = model.Int64(cfg, "upgrade_food")
	next.GoldNeed = model.Int64(cfg, "upgrade_gold")
	next.UpgradeTime = model.Int64(cfg, "upgrade_time")

	if b.State == 1 {
		next.CanUpgrade = false
		next.NoUpgradeMsg = "正在升级中"
		return next
	}
	if b.State != 0 {
		next.CanUpgrade = false
		next.NoUpgradeMsg = "当前状态不可升级"
		return next
	}

	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil {
		next.CanUpgrade = false
		next.NoUpgradeMsg = "资源数据缺失"
		return next
	}
	if model.Int64(res, "wood") < next.WoodNeed ||
		model.Int64(res, "rock") < next.RockNeed ||
		model.Int64(res, "iron") < next.IronNeed ||
		model.Int64(res, "food") < next.FoodNeed ||
		model.Int64(res, "gold") < next.GoldNeed {
		next.CanUpgrade = false
		next.NoUpgradeMsg = "资源不足"
	}
	return next
}

// Upgrade 对齐 startUpgradeBuilding（BuildingFunc.php:355），仅支持已存在建筑的升级。
func (s *Service) Upgrade(ctx context.Context, uid, cid, bid, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	row, err := s.findAt(ctx, cid, x, y)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("nobuilding", "该位置没有建筑")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(row)
	if b.BID != bid {
		return nil, httpx.BadRequest("building_error", "该位置是其它建筑")
	}
	if b.State != 0 {
		return nil, httpx.BadRequest("upgrading", "建筑正在升级中")
	}

	cityType, err := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	dstLevel := b.Level + 1
	if dstLevel > maxLevel(int(cityType), bid) {
		return nil, httpx.BadRequest("no_advanced_construction_plan", "需要更高级的官府")
	}

	cfg, err := s.db.FetchOne(ctx, "select * from cfg_building_levels where bid=? and level=?", bid, dstLevel)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("nobuilding", "该建筑无升级配置")
	}
	if err != nil {
		return nil, err
	}

	wood := model.Int64(cfg, "upgrade_wood")
	rock := model.Int64(cfg, "upgrade_rock")
	iron := model.Int64(cfg, "upgrade_iron")
	food := model.Int64(cfg, "upgrade_food")
	gold := model.Int64(cfg, "upgrade_gold")
	upTime := model.Int64(cfg, "upgrade_time")

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
		return nil, httpx.BadRequest("resource_not_enough", "资源不足")
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
	if _, err := tx.ExecContext(ctx,
		"update buildings set state=1, state_start_at=?, state_end_at=? where city_id=? and xy=?",
		now, now+upTime, cid, xyFrom(x, y)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.List(ctx, cid)
}

// Stop 对齐 stopUpgradeBuilding（BuildingFunc.php:571）：取消升级并返还 66% 资源。
func (s *Service) Stop(ctx context.Context, uid, cid, bid, x, y int) ([]model.Building, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}

	row, err := s.findAt(ctx, cid, x, y)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("nobuilding", "该位置没有建筑")
	}
	if err != nil {
		return nil, err
	}
	b := model.BuildingFromMap(row)
	if b.BID != bid {
		return nil, httpx.BadRequest("building_error", "该位置是其它建筑")
	}
	if b.State != 1 {
		return nil, httpx.BadRequest("nobuilding", "该建筑没有正在进行的升级")
	}

	cfg, err := s.db.FetchOne(ctx, "select * from cfg_building_levels where bid=? and level=?", bid, b.Level+1)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("nobuilding", "该建筑无升级配置")
	}
	if err != nil {
		return nil, err
	}
	// 返还 66%，与 legacy floor/乘法语义一致（此处按整数截断）。
	wood := model.Int64(cfg, "upgrade_wood") * 66 / 100
	rock := model.Int64(cfg, "upgrade_rock") * 66 / 100
	iron := model.Int64(cfg, "upgrade_iron") * 66 / 100
	food := model.Int64(cfg, "upgrade_food") * 66 / 100
	gold := model.Int64(cfg, "upgrade_gold") * 66 / 100

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
	if _, err := tx.ExecContext(ctx,
		"update buildings set state=0, state_start_at=0, state_end_at=0 where city_id=? and xy=?",
		cid, xyFrom(x, y)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.List(ctx, cid)
}