package army

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strconv"

	"rxsg/backend/internal/battle"
	"rxsg/backend/internal/db"
	"rxsg/backend/internal/game"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// Service 复刻 legacy SoldierFunc.php / TroopFunc.php 的征兵与部队逻辑，并适配新库
// draft_queue/city_soldiers/troops/fields 表。
//
// 与 legacy 的差异（新库简化）：
//   - 无世界地图坐标，行军路程改用固定距离常量 marchDistance 折算，时间再按 GAME_SPEED_RATE 缩放。
//   - 无 cfg_book/sys_user_book/符/特殊兵种/器械表，相关加成省略。
//   - 战斗结算改用 battle 包的 OLdBattleCron 回合引擎（1:1 复刻），详见 battle 包。
type Service struct {
	db     *db.DB
	battle *battle.Service
}

func NewService(d *db.DB, b *battle.Service) *Service { return &Service{db: d, battle: b} }

// barracksBID 兵营建筑 ID（cfg_buildings: 8兵营）。
const barracksBID = 8

// marchDistance 无世界坐标时的固定行军距离（用于折算行程时间）。
const marchDistance = 3000

// 出征任务：3掠夺 / 4占领（对齐 troops.task 语义）。
const (
	TaskPlunder = 3
	TaskOccupy  = 4
)

// 兵种状态：0出征中 / 1返回中（对齐 troops.state 语义）。
const (
	MarchOutbound = 0
	MarchReturn   = 1
)

// DraftSoldier 对齐 legacy ArmySoldierState（interface.php:121）。
type DraftSoldier struct {
	SID        int    `json:"sid"`
	SName      string `json:"sname"`
	Count      int64  `json:"count"`
	HP         int64  `json:"hp"`
	AP         int64  `json:"ap"`
	DP         int64  `json:"dp"`
	Speed      int64  `json:"speed"`
	WoodNeed   int64  `json:"woodNeed"`
	RockNeed   int64  `json:"rockNeed"`
	IronNeed   int64  `json:"ironNeed"`
	FoodNeed   int64  `json:"foodNeed"`
	GoldNeed   int64  `json:"goldNeed"`
	PeopleNeed int64  `json:"peopleNeed"`
	DraftTime  int64  `json:"draft_time"`
	CanDraft   bool   `json:"can_draft"`
	NoDraftMsg string `json:"no_draft_msg"`
}

// Queue 对齐 legacy ArmyDraftState。
type Queue struct {
	QID      int    `json:"qid"`
	SID      int    `json:"sid"`
	SName    string `json:"sname"`
	Count    int64  `json:"count"`
	State    int    `json:"state"`
	TimeLeft int64  `json:"time_left"`
}

// ArmyInfo 是兵营面板聚合信息。
type ArmyInfo struct {
	X             int            `json:"x"`
	Y             int            `json:"y"`
	BarracksLevel int            `json:"barracksLevel"`
	Soldiers      []DraftSoldier `json:"soldiers"`
	Queues        []Queue        `json:"queues"`
	People        int64          `json:"people"`
	PeopleMax     int64          `json:"people_max"`
}

// March 是行军中/返程中的部队。
type March struct {
	ID         int           `json:"id"`
	HeroID     int           `json:"hero_id"`
	HeroName   string        `json:"hero_name"`
	TargetType int           `json:"target_type"`
	TargetID   int           `json:"target_id"`
	TargetName string        `json:"target_name"`
	Task       int           `json:"task"`
	State      int           `json:"state"`
	Soldiers   map[int]int64 `json:"soldiers"`
	StartAt    int64         `json:"start_at"`
	ArriveAt   int64         `json:"arrive_at"`
	BackAt     int64         `json:"back_at"`
	TimeLeft   int64         `json:"time_left"`
}

// Field 是野地/山寨目标。
type Field struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Level    int    `json:"level"`
	OwnerUID int    `json:"owner_uid"`
	Guard    int64  `json:"guard_power"`
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

// barracksXY 返回城内兵营坐标标签；未找到返回空串。
func (s *Service) barracksXY(ctx context.Context, cid int) string {
	v, err := s.db.FetchCellString(ctx,
		"select xy from buildings where city_id=? and building_id=? order by xy limit 1", cid, barracksBID)
	if err != nil {
		return ""
	}
	return v
}

// resolveXY 允许前端不传坐标，此时自动选用城内第一座兵营。
func (s *Service) resolveXY(ctx context.Context, cid int, x, y int, hasXY bool) (string, int, int, error) {
	if hasXY {
		return xyFrom(x, y), x, y, nil
	}
	xy := s.barracksXY(ctx, cid)
	if xy == "" {
		return "", 0, 0, httpx.BadRequest("no_barracks_built", "未建造兵营")
	}
	bx, by := parseXY(xy)
	return xy, bx, by, nil
}

func xyFrom(x, y int) string { return string(rune('a'+x)) + string(rune('1'+y)) }
func parseXY(xy string) (int, int) {
	return model.ParseXY(xy)
}

// speedRate 对齐 getSoldierSpeedRate（SoldierFunc.php:9）的勇武加速。
// 新库无练兵/制造科技与符表，仅取主将→城守→军师 bravery_base+bravery_add。
func (s *Service) speedRate(ctx context.Context, cid int) float64 {
	city, err := s.db.FetchOne(ctx, "select general_hero_id, chief_hero_id, counsellor_hero_id from cities where id=?", cid)
	if err != nil {
		return 1
	}
	hid := model.Int(city, "general_hero_id")
	if hid == 0 {
		hid = model.Int(city, "chief_hero_id")
	}
	if hid == 0 {
		hid = model.Int(city, "counsellor_hero_id")
	}
	if hid == 0 {
		return 1
	}
	hero, err := s.db.FetchOne(ctx, "select bravery_base, bravery_add from heroes where id=?", hid)
	if err != nil {
		return 1
	}
	add := float64(model.Int(hero, "bravery_base") + model.Int(hero, "bravery_add"))
	return 1.0 / (1.0 + 0.01*add)
}

// realDraftSeconds 对齐 startDraftQueue（SoldierFunc.php:342）：单兵真实耗时，按倍速缩放后向下取整、最小 1。
func realDraftSeconds(timeNeed int64, rate float64) int64 {
	t := math.Floor(float64(timeNeed) * rate / float64(game.SpeedRate))
	if t < 1 {
		t = 1
	}
	return int64(t)
}

// SettleDraft 惰性结算征兵队列：到期的入城、并推进同兵营的下一队（legacy mem_city_draft）。
func (s *Service) SettleDraft(ctx context.Context, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		row, err := s.db.FetchOne(ctx,
			"select * from draft_queue where city_id=? and state=1 and end_at>0 and end_at<=? order by queued_at limit 1",
			cid, now)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		qid := model.Int(row, "id")
		xy := model.Str(row, "xy")
		sid := model.Int(row, "soldier_id")
		count := model.Int64(row, "count")
		if _, err := s.db.Exec(ctx,
			"insert into city_soldiers (city_id, soldier_id, count) values (?,?,?) on duplicate key update count=count+values(count)",
			cid, sid, count); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, "delete from draft_queue where id=?", qid); err != nil {
			return err
		}
		// 同兵营下一队开始训练。
		next, err := s.db.FetchOne(ctx,
			"select * from draft_queue where city_id=? and xy=? and state=0 order by queued_at limit 1", cid, xy)
		if err == nil && next != nil {
			if _, err := s.db.Exec(ctx,
				"update draft_queue set state=1, started_at=?, end_at=? where id=?",
				now, now+model.Int64(next, "need_time"), model.Int(next, "id")); err != nil {
				return err
			}
		}
	}
	return nil
}

// Info 返回兵营面板信息（征兵列表 + 队列）。
func (s *Service) Info(ctx context.Context, uid, cid, x, y int, hasXY bool) (*ArmyInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleDraft(ctx, cid); err != nil {
		return nil, err
	}
	xy, bx, by, err := s.resolveXY(ctx, cid, x, y, hasXY)
	if err != nil {
		return nil, err
	}

	bLevel, err := s.db.FetchCellInt64(ctx,
		"select level from buildings where city_id=? and xy=? and building_id=?", cid, xy, barracksBID)
	if err != nil {
		return nil, httpx.BadRequest("no_barracks_info", "该位置没有兵营")
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	rate := s.speedRate(ctx, cid)

	res, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	owned, err := s.ownedSoldiers(ctx, cid)
	if err != nil {
		return nil, err
	}

	cfgs, err := s.db.FetchRows(ctx, "select * from cfg_soldiers where fromcity=1 order by sid")
	if err != nil {
		return nil, err
	}
	conds, err := s.soldierConditions(ctx)
	if err != nil {
		return nil, err
	}

	list := make([]DraftSoldier, 0, len(cfgs))
	for _, c := range cfgs {
		sid := model.Int(c, "sid")
		d := DraftSoldier{
			SID:        sid,
			SName:      model.Str(c, "name"),
			Count:      owned[sid],
			HP:         model.Int64(c, "hp"),
			AP:         model.Int64(c, "ap"),
			DP:         model.Int64(c, "dp"),
			Speed:      model.Int64(c, "speed"),
			WoodNeed:   model.Int64(c, "wood_need"),
			RockNeed:   model.Int64(c, "rock_need"),
			IronNeed:   model.Int64(c, "iron_need"),
			FoodNeed:   model.Int64(c, "food_need"),
			GoldNeed:   model.Int64(c, "gold_need"),
			PeopleNeed: model.Int64(c, "people_need"),
			DraftTime:  int64(math.Max(1, math.Floor(float64(model.Int64(c, "time_need"))*rate))),
			CanDraft:   true,
		}
		// 前置条件（新库 cfg_soldier_conditions 仅含兵营等级）。
		for _, cond := range conds[sid] {
			if cond.preType == 0 {
				cur := bLevel
				if cond.preID != barracksBID {
					cur, _ = s.db.FetchCellInt64(ctx,
						"select max(level) from buildings where city_id=? and building_id=?", cid, cond.preID)
				}
				if cur < cond.preLevel {
					d.CanDraft = false
					d.NoDraftMsg = "兵营等级不足"
				}
			}
		}
		if d.CanDraft && !s.enough(res, d.WoodNeed, d.RockNeed, d.IronNeed, d.FoodNeed, d.GoldNeed) {
			d.CanDraft = false
			d.NoDraftMsg = "资源不足"
		}
		list = append(list, d)
	}

	queues, err := s.queues(ctx, cid, xy, now)
	if err != nil {
		return nil, err
	}

	info := &ArmyInfo{X: bx, Y: by, BarracksLevel: int(bLevel), Soldiers: list, Queues: queues}
	info.People = model.Int64(res, "people")
	info.PeopleMax = model.Int64(res, "people_max")
	return info, nil
}

type cond struct {
	preType  int
	preID    int
	preLevel int64
}

// soldierConditions 读取 cfg_soldier_conditions，按 sid 分组。
func (s *Service) soldierConditions(ctx context.Context) (map[int][]cond, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_soldier_conditions")
	if err != nil {
		return nil, err
	}
	out := make(map[int][]cond)
	for _, r := range rows {
		sid := model.Int(r, "sid")
		out[sid] = append(out[sid], cond{
			preType:  model.Int(r, "pre_type"),
			preID:    model.Int(r, "pre_id"),
			preLevel: model.Int64(r, "pre_level"),
		})
	}
	return out, nil
}

func (s *Service) ownedSoldiers(ctx context.Context, cid int) (map[int]int64, error) {
	rows, err := s.db.FetchRows(ctx, "select soldier_id, count from city_soldiers where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	out := make(map[int]int64, len(rows))
	for _, r := range rows {
		out[model.Int(r, "soldier_id")] = model.Int64(r, "count")
	}
	return out, nil
}

func (s *Service) queues(ctx context.Context, cid int, xy string, now int64) ([]Queue, error) {
	rows, err := s.db.FetchRows(ctx,
		`select d.*, s.name as sname from draft_queue d
		 left join cfg_soldiers s on s.sid=d.soldier_id
		 where d.city_id=? and d.xy=? order by d.state desc, d.queued_at`, cid, xy)
	if err != nil {
		return nil, err
	}
	out := make([]Queue, 0, len(rows))
	for _, r := range rows {
		q := Queue{
			QID:   model.Int(r, "id"),
			SID:   model.Int(r, "soldier_id"),
			SName: model.Str(r, "sname"),
			Count: model.Int64(r, "count"),
			State: model.Int(r, "state"),
		}
		if q.State == 1 {
			q.TimeLeft = model.Int64(r, "end_at") - now
			if q.TimeLeft < 0 {
				q.TimeLeft = 0
			}
		} else {
			q.TimeLeft = model.Int64(r, "need_time")
		}
		out = append(out, q)
	}
	return out, nil
}

func (s *Service) enough(res map[string]any, wood, rock, iron, food, gold int64) bool {
	if res == nil {
		return false
	}
	return model.Int64(res, "wood") >= wood && model.Int64(res, "rock") >= rock &&
		model.Int64(res, "iron") >= iron && model.Int64(res, "food") >= food &&
		model.Int64(res, "gold") >= gold
}

// StartDraft 对齐 startDraftQueue（SoldierFunc.php:246）。
func (s *Service) StartDraft(ctx context.Context, uid, cid, x, y int, hasXY bool, sid, count int) (*ArmyInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleDraft(ctx, cid); err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, httpx.BadRequest("cant_recruit_zero", "征兵数量必须大于 0")
	}
	xy, bx, by, err := s.resolveXY(ctx, cid, x, y, hasXY)
	if err != nil {
		return nil, err
	}
	if ok, err := s.db.Exists(ctx,
		"select 1 from buildings where city_id=? and xy=? and building_id=? limit 1", cid, xy, barracksBID); err != nil {
		return nil, err
	} else if !ok {
		return nil, httpx.BadRequest("no_barracks_info", "该位置没有兵营")
	}
	si, err := s.db.FetchOne(ctx, "select * from cfg_soldiers where sid=? and fromcity=1", sid)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_army_branch_info", "没有该兵种")
	}
	if err != nil {
		return nil, err
	}

	rates, err := s.soldierConditions(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range rates[sid] {
		if c.preType == 0 {
			var cur int64
			if c.preID == barracksBID {
				cur, _ = s.db.FetchCellInt64(ctx,
					"select level from buildings where city_id=? and xy=? and building_id=?", cid, xy, barracksBID)
			} else {
				cur, _ = s.db.FetchCellInt64(ctx,
					"select max(level) from buildings where city_id=? and building_id=?", cid, c.preID)
			}
			if cur < c.preLevel {
				return nil, httpx.BadRequest("no_pre_building", "前置建筑等级不足")
			}
		}
	}

	// 该兵营队列上限 = 兵营等级 + 1。
	bLevel, err := s.db.FetchCellInt64(ctx, "select level from buildings where city_id=? and xy=?", cid, xy)
	if err != nil {
		return nil, err
	}
	qn, err := s.db.FetchCellInt64(ctx, "select count(*) from draft_queue where city_id=? and xy=?", cid, xy)
	if err != nil {
		return nil, err
	}
	if qn >= bLevel+1 {
		return nil, httpx.BadRequest("reach_queue_limit", "该兵营的征兵队列已满")
	}

	peopleNeed := model.Int64(si, "people_need") * int64(count)
	wood := model.Int64(si, "wood_need") * int64(count)
	rock := model.Int64(si, "rock_need") * int64(count)
	iron := model.Int64(si, "iron_need") * int64(count)
	food := model.Int64(si, "food_need") * int64(count)
	gold := model.Int64(si, "gold_need") * int64(count)

	perUnit := realDraftSeconds(model.Int64(si, "time_need"), s.speedRate(ctx, cid))
	needTime := perUnit * int64(count)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var res struct {
		Wood   int64 `db:"wood"`
		Rock   int64 `db:"rock"`
		Iron   int64 `db:"iron"`
		Food   int64 `db:"food"`
		Gold   int64 `db:"gold"`
		People int64 `db:"people"`
	}
	if err := tx.QueryRowxContext(ctx,
		"select wood,rock,iron,food,gold,people from city_resources where city_id=? for update", cid).
		StructScan(&res); err != nil {
		return nil, err
	}
	if res.Wood < wood || res.Rock < rock || res.Iron < iron || res.Food < food || res.Gold < gold {
		return nil, httpx.BadRequest("no_enough_resource", "资源不足")
	}
	if res.People < peopleNeed {
		return nil, httpx.BadRequest("lack_free_people", "空闲人口不足")
	}

	var now int64
	if err := tx.QueryRowxContext(ctx, "select unix_timestamp()").Scan(&now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"update city_resources set wood=wood-?,rock=rock-?,iron=iron-?,food=food-?,gold=gold-?,people=people-? where city_id=?",
		wood, rock, iron, food, gold, peopleNeed, cid); err != nil {
		return nil, err
	}

	// 该兵营若无训练中的队列，则本队列直接开训。
	var started int64
	_ = tx.QueryRowxContext(ctx,
		"select count(*) from draft_queue where city_id=? and xy=? and state=1", cid, xy).Scan(&started)
	state := 0
	var startedAt, endAt int64
	if started == 0 {
		state = 1
		startedAt = now
		endAt = now + needTime
	}
	if _, err := tx.ExecContext(ctx,
		`insert into draft_queue (city_id, xy, soldier_id, count, state, need_time, accmark, queued_at, started_at, end_at)
		 values (?,?,?,?,?,?,0,?,?,?)`,
		cid, xy, sid, count, state, needTime, now, startedAt, endAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return s.Info(ctx, uid, cid, bx, by, true)
}

// StopDraft 对齐 stopDraftQueue（SoldierFunc.php:395）：取消队列、还人、返还 66% 资源。
func (s *Service) StopDraft(ctx context.Context, uid, cid, x, y int, hasXY bool, qid int) (*ArmyInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleDraft(ctx, cid); err != nil {
		return nil, err
	}
	xy, bx, by, err := s.resolveXY(ctx, cid, x, y, hasXY)
	if err != nil {
		return nil, err
	}
	row, err := s.db.FetchOne(ctx, "select * from draft_queue where id=? and city_id=?", qid, cid)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_army_branch_info", "没有该征兵队列")
	}
	if err != nil {
		return nil, err
	}
	sid := model.Int(row, "soldier_id")
	count := model.Int64(row, "count")
	si, err := s.db.FetchOne(ctx, "select * from cfg_soldiers where sid=?", sid)
	if err != nil {
		return nil, err
	}
	people := model.Int64(si, "people_need") * count
	wood := model.Int64(si, "wood_need") * count * 66 / 100
	rock := model.Int64(si, "rock_need") * count * 66 / 100
	iron := model.Int64(si, "iron_need") * count * 66 / 100
	food := model.Int64(si, "food_need") * count * 66 / 100
	gold := model.Int64(si, "gold_need") * count * 66 / 100

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		"update city_resources set wood=wood+?,rock=rock+?,iron=iron+?,food=food+?,gold=gold+?,people=people+? where city_id=?",
		wood, rock, iron, food, gold, people, cid); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, "delete from draft_queue where id=?", qid); err != nil {
		return nil, err
	}
	// 若删的是训练中的队列，拉起同兵营下一队。
	if model.Int(row, "state") == 1 {
		var now int64
		if err := tx.QueryRowxContext(ctx, "select unix_timestamp()").Scan(&now); err != nil {
			return nil, err
		}
		next, err := s.db.FetchOne(ctx, "select * from draft_queue where city_id=? and xy=? and state=0 order by queued_at limit 1", cid, xy)
		if err == nil && next != nil {
			if _, err := tx.ExecContext(ctx,
				"update draft_queue set state=1, started_at=?, end_at=? where id=?",
				now, now+model.Int64(next, "need_time"), model.Int(next, "id")); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid, bx, by, true)
}

// Dissolve 对齐 dissolveSoldier（SoldierFunc.php:442）：解散士兵，还人并返还 33% 资源。
func (s *Service) Dissolve(ctx context.Context, uid, cid, sid, count int) (*ArmyInfo, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleDraft(ctx, cid); err != nil {
		return nil, err
	}
	if count <= 0 {
		return nil, httpx.BadRequest("cant_dismiss_zero", "解散数量必须大于 0")
	}
	si, err := s.db.FetchOne(ctx, "select * from cfg_soldiers where sid=? and fromcity=1", sid)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_army_branch_info", "没有该兵种")
	}
	if err != nil {
		return nil, err
	}
	cur, err := s.db.FetchCellInt64(ctx, "select count from city_soldiers where city_id=? and soldier_id=?", cid, sid)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if cur < int64(count) {
		return nil, httpx.BadRequest("cant_dismiss_exceed", "解散数量超过现有兵力")
	}
	people := model.Int64(si, "people_need") * int64(count)
	wood := model.Int64(si, "wood_need") * int64(count) * 33 / 100
	rock := model.Int64(si, "rock_need") * int64(count) * 33 / 100
	iron := model.Int64(si, "iron_need") * int64(count) * 33 / 100
	food := model.Int64(si, "food_need") * int64(count) * 33 / 100
	gold := model.Int64(si, "gold_need") * int64(count) * 33 / 100

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		"update city_resources set wood=wood+?,rock=rock+?,iron=iron+?,food=food+?,gold=gold+?,people=people+? where city_id=?",
		wood, rock, iron, food, gold, people, cid); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		"update city_soldiers set count=greatest(0,count-?) where city_id=? and soldier_id=?", count, cid, sid); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid, 0, 0, false)
}

// Fields 返回可出征的野地/山寨目标列表。
func (s *Service) Fields(ctx context.Context, uid, cid int) ([]Field, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx, "select * from fields order by level, id")
	if err != nil {
		return nil, err
	}
	out := make([]Field, 0, len(rows))
	for _, r := range rows {
		out = append(out, Field{
			ID:       model.Int(r, "id"),
			Name:     model.Str(r, "name"),
			Level:    model.Int(r, "level"),
			OwnerUID: model.Int(r, "owner_uid"),
			Guard:    s.fieldGuardPower(ctx, model.Str(r, "guard_soldiers")) + model.Int64(r, "guard_power"),
		})
	}
	return out, nil
}

func (s *Service) fieldGuardPower(ctx context.Context, soldiersJSON string) int64 {
	return s.soldiersPower(ctx, decodeSoldiers(soldiersJSON), "dp")
}

// soldiersPower 汇总一组兵力的战力：col 取 "ap"（进攻）或 "dp"（防守）。
func (s *Service) soldiersPower(ctx context.Context, soldiers map[int]int64, col string) int64 {
	if col != "ap" && col != "dp" {
		return 0
	}
	var power int64
	for sid, cnt := range soldiers {
		if cnt <= 0 {
			continue
		}
		v, _ := s.db.FetchCellInt64(ctx, "select "+col+" from cfg_soldiers where sid=?", sid)
		power += cnt * v
	}
	return power
}

// Marches 返回该城当前行军/返程中的部队。
func (s *Service) Marches(ctx context.Context, uid, cid int) ([]March, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleMarches(ctx, uid, cid); err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx, "select * from troops where city_id=? order by id", cid)
	if err != nil {
		return nil, err
	}
	out := make([]March, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.marchFromMap(ctx, r, now))
	}
	return out, nil
}

func (s *Service) marchFromMap(ctx context.Context, r map[string]any, now int64) March {
	m := March{
		ID:         model.Int(r, "id"),
		HeroID:     model.Int(r, "hero_id"),
		TargetType: model.Int(r, "target_type"),
		TargetID:   model.Int(r, "target_id"),
		Task:       model.Int(r, "task"),
		State:      model.Int(r, "state"),
		StartAt:    model.Int64(r, "start_at"),
		ArriveAt:   model.Int64(r, "arrive_at"),
		BackAt:     model.Int64(r, "back_at"),
		Soldiers:   decodeSoldiers(model.Str(r, "soldiers")),
	}
	if m.HeroID > 0 {
		m.HeroName, _ = s.db.FetchCellString(ctx, "select name from heroes where id=?", m.HeroID)
	}
	if m.TargetType == 1 {
		m.TargetName, _ = s.db.FetchCellString(ctx, "select name from fields where id=?", m.TargetID)
	} else {
		m.TargetName, _ = s.db.FetchCellString(ctx, "select name from cities where id=?", m.TargetID)
	}
	if m.State == MarchOutbound {
		m.TimeLeft = m.ArriveAt - now
	} else {
		m.TimeLeft = m.BackAt - now
	}
	if m.TimeLeft < 0 {
		m.TimeLeft = 0
	}
	return m
}

func decodeSoldiers(raw string) map[int]int64 {
	out := map[int]int64{}
	if raw == "" {
		return out
	}
	var m map[string]int64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return out
	}
	for k, v := range m {
		if sid, err := strconv.Atoi(k); err == nil {
			out[sid] = v
		}
	}
	return out
}

func encodeSoldiers(m map[int]int64) string {
	raw := make(map[string]int64, len(m))
	for k, v := range m {
		raw[strconv.Itoa(k)] = v
	}
	b, _ := json.Marshal(raw)
	return string(b)
}

// Dispatch 出征：把城内士兵（可选随行武将）派往野地或玩家城池。
// task=TaskPlunder 掠夺 / TaskOccupy 占领（占领仅对野地有效）。
func (s *Service) Dispatch(ctx context.Context, uid, cid, heroID, targetType, targetID, task int, soldiers map[int]int64) ([]March, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleDraft(ctx, cid); err != nil {
		return nil, err
	}
	if err := s.SettleMarches(ctx, uid, cid); err != nil {
		return nil, err
	}
	if targetType != 1 && targetType != 2 {
		return nil, httpx.BadRequest("invalid_target", "目标类型非法")
	}
	if task != TaskPlunder && task != TaskOccupy {
		return nil, httpx.BadRequest("invalid_task", "出征任务非法")
	}
	if task == TaskOccupy && targetType != 1 {
		return nil, httpx.BadRequest("occupy_only_field", "仅可占领野地")
	}
	total := int64(0)
	for _, c := range soldiers {
		total += c
	}
	if total <= 0 {
		return nil, httpx.BadRequest("no_soldier_selected", "请选择出征兵力")
	}
	// 目标校验。
	if targetType == 1 {
		if ok, err := s.db.Exists(ctx, "select 1 from fields where id=? limit 1", targetID); err != nil {
			return nil, err
		} else if !ok {
			return nil, httpx.NotFound("no_field", "目标野地不存在")
		}
	} else {
		ok, err := s.db.Exists(ctx, "select 1 from cities where id=? and user_id<>? limit 1", targetID, uid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, httpx.NotFound("no_target_city", "目标城池不存在或为己方城池")
		}
	}
	// 武将校验。
	if heroID > 0 {
		ok, err := s.db.Exists(ctx, "select 1 from heroes where id=? and user_id=? and city_id=? and state=0 limit 1", heroID, uid, cid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, httpx.BadRequest("invalid_hero", "武将不可出征")
		}
	}

	// 校验并扣减城内兵力。
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for sid, cnt := range soldiers {
		var cur int64
		if err := tx.QueryRowxContext(ctx,
			"select count from city_soldiers where city_id=? and soldier_id=? for update", cid, sid).Scan(&cur); err != nil {
			if err == sql.ErrNoRows {
				return nil, httpx.BadRequest("no_army_branch_info", "城内没有该兵种")
			}
			return nil, err
		}
		if cur < cnt {
			return nil, httpx.BadRequest("no_enough_soldier", "兵力不足")
		}
		if _, err := tx.ExecContext(ctx,
			"update city_soldiers set count=count-? where city_id=? and soldier_id=?", cnt, cid, sid); err != nil {
			return nil, err
		}
	}

	var now int64
	if err := tx.QueryRowxContext(ctx, "select unix_timestamp()").Scan(&now); err != nil {
		return nil, err
	}
	travel := s.marchSeconds(ctx, soldiers)
	if _, err := tx.ExecContext(ctx,
		`insert into troops (user_id, city_id, hero_id, target_type, target_id, task, state, soldiers, start_at, arrive_at, back_at, created_at)
		 values (?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, cid, heroID, targetType, targetID, task, MarchOutbound, encodeSoldiers(soldiers), now, now+travel, 0, now); err != nil {
		return nil, err
	}
	if heroID > 0 {
		if _, err := tx.ExecContext(ctx, "update heroes set state=4 where id=?", heroID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Marches(ctx, uid, cid)
}

// Recall 召回：出征途中立即折返；已在返程中的部队不可再次召回。
func (s *Service) Recall(ctx context.Context, uid, cid, troopID int) ([]March, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.SettleMarches(ctx, uid, cid); err != nil {
		return nil, err
	}
	row, err := s.db.FetchOne(ctx, "select * from troops where id=? and user_id=? and city_id=?", troopID, uid, cid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("invalid_army", "部队不存在")
	}
	if err != nil {
		return nil, err
	}
	switch model.Int(row, "state") {
	case MarchReturn:
		return nil, httpx.BadRequest("army_on_way_back", "部队正在返程中")
	default:
		now, err := s.db.Now(ctx)
		if err != nil {
			return nil, err
		}
		startAt := model.Int64(row, "start_at")
		// 回程时间 = 已行进的时间（对齐 legacy callBackTroop）。
		backAt := now + (now - startAt)
		if _, err := s.db.Exec(ctx,
			"update troops set state=?, start_at=?, back_at=? where id=?", MarchReturn, now, backAt, troopID); err != nil {
			return nil, err
		}
	}
	return s.Marches(ctx, uid, cid)
}

// marchSeconds 按最慢兵种速度折算行军耗时，再按倍速缩放（无世界地图，用固定距离）。
func (s *Service) marchSeconds(ctx context.Context, soldiers map[int]int64) int64 {
	minSpeed := int64(0)
	for sid := range soldiers {
		sp, err := s.db.FetchCellInt64(ctx, "select speed from cfg_soldiers where sid=?", sid)
		if err != nil || sp <= 0 {
			continue
		}
		if minSpeed == 0 || sp < minSpeed {
			minSpeed = sp
		}
	}
	if minSpeed <= 0 {
		minSpeed = 10
	}
	raw := int64(math.Ceil(float64(marchDistance) / float64(minSpeed)))
	t := int64(math.Ceil(float64(raw) / float64(game.SpeedRate)))
	if t < 1 {
		t = 1
	}
	return t
}

// SettleMarches 惰性结算：到达的出征执行战斗，返程到期的士兵回城。
// 无 cron，故在每个军队读写入口调用。
func (s *Service) SettleMarches(ctx context.Context, uid, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	// 1) 到达前线：建立战斗（legacy cfg_js.php 抵达分支 → mem_battle 落库）。
	for i := 0; i < 50; i++ {
		row, err := s.db.FetchOne(ctx,
			"select * from troops where city_id=? and state=? and arrive_at>0 and arrive_at<=? order by id limit 1",
			cid, MarchOutbound, now)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		troopID := model.Int(row, "id")
		bid, err := s.battle.StartBattleForTroop(ctx, troopID)
		if err != nil {
			return err
		}
		if bid == 0 {
			// 目标已消失：直接返程（对齐 legacy 目标不存在分支）。
			survivors := decodeSoldiers(model.Str(row, "soldiers"))
			if err := s.beginReturn(ctx, troopID, survivors, now, s.marchSeconds(ctx, survivors)); err != nil {
				return err
			}
		}
	}
	// 1b) 推进与 uid 相关的在战回合（legacy HandleBattle 惰性化）。
	if s.battle != nil {
		if _, err := s.battle.SettleForUser(ctx, uid); err != nil {
			return err
		}
	}
	// 2) 返程到期：存活士兵回城、删除部队、释放武将。
	for i := 0; i < 50; i++ {
		row, err := s.db.FetchOne(ctx,
			"select * from troops where city_id=? and state=? and back_at>0 and back_at<=? order by id limit 1",
			cid, MarchReturn, now)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		troopID := model.Int(row, "id")
		survivors := decodeSoldiers(model.Str(row, "soldiers"))
		if err := s.addSoldiersToCity(ctx, cid, survivors); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, "delete from troops where id=?", troopID); err != nil {
			return err
		}
		if hid := model.Int(row, "hero_id"); hid > 0 {
			if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", hid); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) addSoldiersToCity(ctx context.Context, cid int, soldiers map[int]int64) error {
	for sid, cnt := range soldiers {
		if cnt <= 0 {
			continue
		}
		if _, err := s.db.Exec(ctx,
			"insert into city_soldiers (city_id, soldier_id, count) values (?,?,?) on duplicate key update count=count+values(count)",
			cid, sid, cnt); err != nil {
			return err
		}
	}
	return nil
}

// beginReturn 战后处理：无幸存者则直接结束（释放武将），否则进入返程。
func (s *Service) beginReturn(ctx context.Context, troopID int, survivors map[int]int64, now, travel int64) error {
	heroID, _ := s.db.FetchCellInt64(ctx, "select hero_id from troops where id=?", troopID)
	if len(survivors) == 0 {
		if _, err := s.db.Exec(ctx, "delete from troops where id=?", troopID); err != nil {
			return err
		}
		if heroID > 0 {
			if _, err := s.db.Exec(ctx, "update heroes set state=0 where id=?", heroID); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := s.db.Exec(ctx,
		"update troops set state=?, soldiers=?, start_at=?, back_at=? where id=?",
		MarchReturn, encodeSoldiers(survivors), now, now+travel, troopID); err != nil {
		return err
	}
	return nil
}
