package hero

import (
	"context"
	"database/sql"
	"math"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// Service 复刻 legacy HeroFunc.php 的将领升级与历练（sys_hero_expr），并适配新库
// heroes / hero_exprs / cfg_hero_levels 三表。
//
// 与 legacy 的差异（新库简化）：
//   - 无 cfg_hero_expr_types（历练类型/金币消耗配置），仅保留「修身养性」单一类型且不消耗货币。
//   - 无奇遇/奖励/道具/装备等子系统，历练只产出经验。
//   - 无 mem_hero_blood（体力/精力），相关字段省略。
//   - 无 cron，历练到期改为惰性结算（见 Settle）。
type Service struct {
	db *db.DB
}

func NewService(d *db.DB) *Service { return &Service{db: d} }

// 武将状态（新库 heroes.state 语义）：
// 0 空闲 / 3 在城 / 4 出征 / 10 历练中 / 11 历练结束待结算。
const (
	StateIdle     = 0
	StateInCity   = 3
	StateOut      = 4
	StateExpring  = 10
	StateExprDone = 11
)

// 历练参数：新库无 cfg_hero_expr_types，取固定每小时经验与时长上限。
const (
	ExprTypeXiuShen = 1    // 修身养性
	ExprExpPerHour  = 40   // 每小时经验
	ExprMinHour     = 1    // 最短时长
	ExprMaxHour     = 12   // 最长时长
	exprUnitSecond  = 3600 // 一小时秒数
)

// Expr 是一次历练任务（hero_exprs）。
type Expr struct {
	ID        int   `json:"id"`
	ExprType  int   `json:"expr_type"`
	Hours     int   `json:"hours"`
	State     int   `json:"state"`
	StartedAt int64 `json:"started_at"`
	EndAt     int64 `json:"end_at"`
	TimeLeft  int64 `json:"time_left"`
	ExpGain   int64 `json:"exp_gain"`
}

// ExprType 是可供选择的历练类型（新库固定一种）。
type ExprType struct {
	Type       int   `json:"type"`
	Name       string `json:"name"`
	MinHour    int   `json:"min_hour"`
	MaxHour    int   `json:"max_hour"`
	ExpPerHour int64 `json:"exp_per_hour"`
}

// HeroState 是单个武将的完整状态（列表/详情共用）。
type HeroState struct {
	HID          int    `json:"hid"`
	Name         string `json:"name"`
	Sex          int    `json:"sex"`
	Face         int    `json:"face"`
	HeroType     int    `json:"hero_type"`
	Level        int    `json:"level"`
	Exp          int64  `json:"exp"`
	State        int    `json:"state"`
	Loyalty      int    `json:"loyalty"`
	HeroHealth   int    `json:"hero_health"`
	CommandBase  int    `json:"command_base"`
	BraveryBase  int    `json:"bravery_base"`
	BraveryAdd   int    `json:"bravery_add"`
	WisdomBase   int    `json:"wisdom_base"`
	WisdomAdd    int    `json:"wisdom_add"`
	AffairsBase  int    `json:"affairs_base"`
	AffairsAdd   int    `json:"affairs_add"`
	AttackBase   int    `json:"attack_base"`
	AttackAddOn  int    `json:"attack_add_on"`
	DefenceBase  int    `json:"defence_base"`
	DefenceAddOn int    `json:"defence_add_on"`
	LevelTotalExp int64 `json:"level_total_exp"`
	UpgradeExp    int64 `json:"upgrade_exp"`
	NeedExp       int64 `json:"need_exp"`
	CanUpgrade    bool  `json:"can_upgrade"`
	NoUpgradeMsg  string `json:"no_upgrade_msg"`
	Expr          *Expr `json:"expr"`
}

// Info 是武将面板聚合信息。
type Info struct {
	Heroes    []HeroState `json:"heroes"`
	ExprTypes []ExprType  `json:"exprTypes"`
}

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

// inCity 对齐 legacy isHeroInCity：空闲(0)与在城(3)视为可操作。
func inCity(state int) bool { return state == StateIdle || state == StateInCity }

// Settle 惰性结算到期的历练：写入经验、删除任务、复位武将状态。
// 经验口径 =（end_at - started_at）折算小时 × ExprExpPerHour。
// 正常结束 end_at-started_at = hours*3600；中途取消则按 legacy 的 endtime 回填规则折算。
func (s *Service) Settle(ctx context.Context, cid int) error {
	now, err := s.db.Now(ctx)
	if err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		row, err := s.db.FetchOne(ctx,
			"select * from hero_exprs where city_id=? and state in (0,1) and end_at>0 and end_at<=? order by end_at limit 1",
			cid, now)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		id := model.Int(row, "id")
		hid := model.Int(row, "hero_id")
		elapsed := model.Int64(row, "end_at") - model.Int64(row, "started_at")
		exp := int64(math.Floor(float64(elapsed) / float64(exprUnitSecond) * ExprExpPerHour))
		if exp < 0 {
			exp = 0
		}
		if _, err := s.db.Exec(ctx, "update heroes set exp=exp+? where id=?", exp, hid); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, "delete from hero_exprs where id=?", id); err != nil {
			return err
		}
		if _, err := s.db.Exec(ctx, "update heroes set state=? where id=? and state in (?,?)",
			StateIdle, hid, StateExpring, StateExprDone); err != nil {
			return err
		}
	}
	return nil
}

// levelConfig 读取 cfg_hero_levels 到 level 映射。
func (s *Service) levelConfig(ctx context.Context) (map[int]struct{ total, upgrade int64 }, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_hero_levels")
	if err != nil {
		return nil, err
	}
	out := make(map[int]struct{ total, upgrade int64 }, len(rows))
	for _, r := range rows {
		out[model.Int(r, "level")] = struct{ total, upgrade int64 }{
			total:   model.Int64(r, "total_exp"),
			upgrade: model.Int64(r, "upgrade_exp"),
		}
	}
	return out, nil
}

// Info 返回该城武将列表（含升级需求与当前历练任务）。
func (s *Service) Info(ctx context.Context, uid, cid int) (*Info, error) {
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
	cfgs, err := s.levelConfig(ctx)
	if err != nil {
		return nil, err
	}
	maxLevel := 0
	for lv := range cfgs {
		if lv > maxLevel {
			maxLevel = lv
		}
	}

	exprByHero, err := s.exprsByHero(ctx, cid, now)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx, "select * from heroes where city_id=? and user_id=? order by id", cid, uid)
	if err != nil {
		return nil, err
	}
	list := make([]HeroState, 0, len(rows))
	for _, r := range rows {
		h := s.heroState(ctx, r, cfgs, maxLevel, exprByHero)
		list = append(list, h)
	}
	return &Info{
		Heroes: list,
		ExprTypes: []ExprType{{
			Type:       ExprTypeXiuShen,
			Name:       "修身养性",
			MinHour:    ExprMinHour,
			MaxHour:    ExprMaxHour,
			ExpPerHour: ExprExpPerHour,
		}},
	}, nil
}

func (s *Service) exprsByHero(ctx context.Context, cid int, now int64) (map[int]*Expr, error) {
	rows, err := s.db.FetchRows(ctx, "select * from hero_exprs where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	out := make(map[int]*Expr, len(rows))
	for _, r := range rows {
		elapsed := model.Int64(r, "end_at") - model.Int64(r, "started_at")
		exp := int64(math.Floor(float64(elapsed) / float64(exprUnitSecond) * ExprExpPerHour))
		if exp < 0 {
			exp = 0
		}
		left := model.Int64(r, "end_at") - now
		if left < 0 {
			left = 0
		}
		out[model.Int(r, "hero_id")] = &Expr{
			ID:        model.Int(r, "id"),
			ExprType:  model.Int(r, "expr_type"),
			Hours:     model.Int(r, "hours"),
			State:     model.Int(r, "state"),
			StartedAt: model.Int64(r, "started_at"),
			EndAt:     model.Int64(r, "end_at"),
			TimeLeft:  left,
			ExpGain:   exp,
		}
	}
	return out, nil
}

func (s *Service) heroState(ctx context.Context, r map[string]any, cfgs map[int]struct{ total, upgrade int64 }, maxLevel int, exprs map[int]*Expr) HeroState {
	h := HeroState{
		HID:          model.Int(r, "id"),
		Name:         model.Str(r, "name"),
		Sex:          model.Int(r, "sex"),
		Face:         model.Int(r, "face"),
		HeroType:     model.Int(r, "hero_type"),
		Level:        model.Int(r, "level"),
		Exp:          model.Int64(r, "exp"),
		State:        model.Int(r, "state"),
		Loyalty:      model.Int(r, "loyalty"),
		HeroHealth:   model.Int(r, "hero_health"),
		CommandBase:  model.Int(r, "command_base"),
		BraveryBase:  model.Int(r, "bravery_base"),
		BraveryAdd:   model.Int(r, "bravery_add"),
		WisdomBase:   model.Int(r, "wisdom_base"),
		WisdomAdd:    model.Int(r, "wisdom_add"),
		AffairsBase:  model.Int(r, "affairs_base"),
		AffairsAdd:   model.Int(r, "affairs_add"),
		AttackBase:   model.Int(r, "attack_base"),
		AttackAddOn:  model.Int(r, "attack_add_on"),
		DefenceBase:  model.Int(r, "defence_base"),
		DefenceAddOn: model.Int(r, "defence_add_on"),
		Expr:         exprs[model.Int(r, "id")],
	}
	cur := cfgs[h.Level]
	next := cfgs[h.Level+1]
	h.LevelTotalExp = cur.total
	h.UpgradeExp = next.upgrade
	need := next.upgrade - (h.Exp - cur.total)
	if need < 0 {
		need = 0
	}
	h.NeedExp = need
	h.CanUpgrade = false
	switch {
	case !inCity(h.State):
		h.NoUpgradeMsg = "武将当前不在城中"
	case h.Level >= maxLevel:
		h.NoUpgradeMsg = "已达最高等级"
	case h.Exp-cur.total < next.upgrade:
		h.NoUpgradeMsg = "经验不足"
	default:
		h.CanUpgrade = true
	}
	return h
}

// Upgrade 消耗经验提升一级（对齐 legacy upgradeHero 的 exp 阈值判定）。
func (s *Service) Upgrade(ctx context.Context, uid, cid, hid int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}
	row, err := s.db.FetchOne(ctx, "select * from heroes where id=? and user_id=? and city_id=?", hid, uid, cid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("no_hero", "武将不存在")
	}
	if err != nil {
		return nil, err
	}
	state := model.Int(row, "state")
	if !inCity(state) {
		return nil, httpx.BadRequest("cant_upgrade_out_hero", "武将当前不在城中")
	}
	level := model.Int(row, "level")
	exp := model.Int64(row, "exp")
	cfgs, err := s.levelConfig(ctx)
	if err != nil {
		return nil, err
	}
	maxLevel := 0
	for lv := range cfgs {
		if lv > maxLevel {
			maxLevel = lv
		}
	}
	if level >= maxLevel {
		return nil, httpx.BadRequest("level_max", "已达最高等级")
	}
	cur := cfgs[level]
	next := cfgs[level+1]
	if exp-cur.total < next.upgrade {
		return nil, httpx.BadRequest("no_enough_exp", "经验不足，无法升级")
	}
	if _, err := s.db.Exec(ctx, "update heroes set level=level+1 where id=?", hid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// StartExpr 派遣武将历练（对齐 legacy beginExprHero 的精简版）。
func (s *Service) StartExpr(ctx context.Context, uid, cid, hid, hours int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}
	if hours < ExprMinHour || hours > ExprMaxHour {
		return nil, httpx.BadRequest("hero_expr_time_error", "历练时长超出范围")
	}
	row, err := s.db.FetchOne(ctx, "select * from heroes where id=? and user_id=? and city_id=?", hid, uid, cid)
	if err == sql.ErrNoRows {
		return nil, httpx.NotFound("no_hero", "武将不存在")
	}
	if err != nil {
		return nil, err
	}
	if !inCity(model.Int(row, "state")) {
		return nil, httpx.BadRequest("hero_expr_hero_not_kong", "该武将当前无法历练")
	}
	ok, err := s.db.Exists(ctx, "select 1 from hero_exprs where hero_id=? limit 1", hid)
	if err != nil {
		return nil, err
	}
	if ok {
		return nil, httpx.BadRequest("hero_expr_already", "该武将已在历练中")
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx,
		`insert into hero_exprs (user_id, city_id, hero_id, expr_type, hours, state, started_at, end_at)
		 values (?,?,?,?,?,?,?,?)`,
		uid, cid, hid, ExprTypeXiuShen, hours, 0, now, now+int64(hours)*exprUnitSecond); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update heroes set state=? where id=?", StateExpring, hid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// CancelExpr 取消历练（对齐 legacy cancelHeroExpr）：未过半则把 end_at 前移，过半则保留原样。
func (s *Service) CancelExpr(ctx context.Context, uid, cid, hid int) (*Info, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}
	if err := s.Settle(ctx, cid); err != nil {
		return nil, err
	}
	row, err := s.db.FetchOne(ctx,
		"select * from hero_exprs where hero_id=? and user_id=? and city_id=? and state=0", hid, uid, cid)
	if err == sql.ErrNoRows {
		return nil, httpx.BadRequest("no_hero_expr", "该武将没有进行中的历练")
	}
	if err != nil {
		return nil, err
	}
	now, err := s.db.Now(ctx)
	if err != nil {
		return nil, err
	}
	startedAt := model.Int64(row, "started_at")
	endAt := model.Int64(row, "end_at")
	hours := model.Int64(row, "hours")
	elapsed := now - startedAt
	if elapsed < hours*1800 {
		endAt = now + elapsed
	}
	if _, err := s.db.Exec(ctx, "update hero_exprs set state=1, end_at=? where id=?", endAt, model.Int(row, "id")); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, "update heroes set state=? where id=? and state=?",
		StateExprDone, hid, StateExpring); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}