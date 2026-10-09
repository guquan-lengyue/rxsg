package hero

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// service.go 1:1 复刻 legacy server/game/HeroFunc.php 的武将升级/加点/洗点
// （upgradeHero:121 / insertHeroBaseAdds:210 / isMyInBigHero:216 / addHeroPoint:225 /
// clearHeroPoint:274 + GoodsFunc.php useXiShuiDan:1657 / getMaxHeroLevel:1490 /
// isInBigHero:2120 / checkHeroLevel:2247 + utils.php updateCityHeroChange:677）。
// 历练（beginExprHero/cancelHeroExpr/fasterHeroExpr + HeroExpr.php 结算）见 expr.go，
// 任命三职（OfficeFunc.php setCityChief:30）见 office.go，道具/元宝底层见 store.go。
//
// 原版怪癖 1:1 保留：
//   - upgradeHero 119 级且 hid∈isMyInBigHero → +6 级并四维 floor(base×0.05)、attack/defence_add_on=125；
//     124 级无条件同处理（level=125 绝对赋值）；
//   - addHeroPoint 的 xidian_unvalid 判定「新 add 不得小于旧 add」按 PHP 松散比较；
//   - beginExprHero 查武将只按 hid 不校验 uid/cid——不存在时按 null 继续（state null!=0 为 false），
//     照样插入历练行；
//   - cancelHeroExpr/fasterHeroExpr 的 update/delete 按 hid 全量匹配，无 uid 条件。
//
// 缺表降级（不建表，注释逐处说明）：
//   - mem_state state=197（yysType 签到服）无表 → 恒 0，走 legacy 默认服分支；
//   - sys_user_level（君主等级）无表 → checkHeroLevel 的 userLevel 条件恒 false，
//     君主将升级上限恒 100、加点恒无 +50；
//   - mem_assemble / sys_hero_attribute / mem_hero_buffer(文曲星符) / cfg_book 技能无表 →
//     regenerateHeroAttri 省略（新库无派生属性表，装备加成属 M5）；
//   - completeTask/completeTaskWithTaskid/logUserAction/logActionCountback 属 M8，未接线；
//   - sys_report / sys_inform 广播无表 → 历练结算战报与全服公告省略；
//   - cfg_hero_expr_reward / sys_hero_expr_reward 奇遇奖励表无种子（原 dump 丢失）→ 奇遇分支省略，
//     仅保留闭关突破（不依赖奖励表）；sys_king_expr 君主历练（MoranchHeroExpr）无表 → 省略。

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// WithUserLock 暴露用户级锁给 handler（对齐 legacy lockUser/unlockUser 文件锁）。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	return s.lk.WithUserLock(ctx, uid, key, fn)
}

// 武将状态（legacy sys_city_hero.state 语义，新库 heroes.state 同值）。
const (
	StateIdle       = 0  // 空闲
	StateChief      = 1  // 城守
	StateOut        = 4  // 出征
	StateGeneral    = 7  // 主将
	StateCounsellor = 8  // 军师
	StateExpring    = 10 // 历练中
	StateExprDone   = 11 // 历练取消待结算
)

// isHeroInCity 对齐 HeroFunc.php:1949：state∈{0,1,7,8} 视为在城可操作。
func isHeroInCity(state int) bool {
	return state == 0 || state == 1 || state == 7 || state == 8
}

// errHero 构造与 legacy throw new Exception 等价的错误响应。
func errHero(msg string) error {
	return httpx.BadRequest("hero_error", msg)
}

// lang 文案（server/game/lang.php 逐字）。
const (
	msgCantUpgradeThis    = "不能升级该将领。"
	msgCantUpgradeOut     = "不能升级不在城池内的将领。"
	msgNoEnoughExp        = "将领的经验不足，不能升级。"
	msgLevelMax           = "将领已经升到顶级。"
	msgCantFindHero       = "找不到该将领。"
	msgCantAddOut         = "不能给不在城池内的将领加属性点。"
	msgNoExtraPotential   = "没有多余的潜力。"
	msgXidianUnvalid      = "使用外挂将导致账号数据异常，后果自负！"
	msgCantCleanOut       = "不能给不在城池里的将领洗点。"
	msgCannotExpr         = "君主将不能历练。"
	msgHeroNotKong        = "该将领不是空闲状态"
	msgExprCountMax       = "本城同时进行历练的将领数已达到最大人数5名"
	msgToomanyHeroExpr    = "同时进行历练的将领数为2名，是否使用道具“巡查令”增加将领数到5名?"
	msgExprNotEnoughGold  = "本城拥有的黄金数不够"
	msgExprNotEnoughMoney = "你的元宝数目不够"
	msgExprTimeError      = "请设置历练时间,“%s”时间在“%d”到“%d”之间"
	msgWaiguaInvalid      = "数据异常"
	msgExpUpdateAlert     = "历练系统更新，暂停将领历练，将于3月17日17点重开"
	msgSetChiefFail       = "任命将领失败。"
	msgSetChiefFail2      = "数据出错，任命将领失败。"
	msgSetChiefBusy       = "将领出征中，不能任命。"
)

// bighidsForMaxLevel 对齐 HeroFunc.php:2120 isInBigHero（按 npcid 判 125 上限）。
var bighidsForMaxLevel = map[int]bool{36: true, 102: true, 107: true, 114: true, 118: true, 177: true, 186: true, 241: true, 255: true, 285: true, 320: true, 321: true, 340: true, 347: true, 357: true, 360: true, 362: true, 381: true, 382: true, 409: true, 456: true, 484: true, 518: true, 541: true, 562: true, 580: true, 620: true, 677: true, 699: true, 725: true, 780: true, 801: true, 856: true, 861: true, 870: true}

// myBigHids 对齐 HeroFunc.php:216 isMyInBigHero（119 级突破名单，按 hid）。
var myBigHids = map[int]bool{156: true, 200: true, 222: true, 261: true, 441: true, 455: true, 497: true, 549: true, 563: true, 577: true, 791: true, 832: true, 856: true, 863: true, 1006: true, 1011: true, 1015: true}

// exprBigHids 对齐 HeroExpr.php:92 isInBigHero（历练闭关突破名单，按 hid；与上面两份列表不同，原版如此）。
var exprBigHids = map[int]bool{10: true, 36: true, 102: true, 107: true, 114: true, 118: true, 177: true, 186: true, 241: true, 255: true, 285: true, 320: true, 321: true, 340: true, 347: true, 357: true, 360: true, 362: true, 381: true, 382: true, 409: true, 456: true, 484: true, 518: true, 541: true, 562: true, 580: true, 620: true, 677: true, 699: true, 725: true, 780: true, 801: true, 861: true, 870: true, 156: true, 200: true, 222: true, 261: true, 441: true, 455: true, 497: true, 549: true, 563: true, 577: true, 791: true, 832: true, 856: true, 863: true, 1006: true, 1011: true, 1015: true}

// Expr 是一次历练任务（hero_exprs ← sys_hero_expr）。
type Expr struct {
	ID         int    `json:"id"`
	ExprType   int    `json:"expr_type"`
	ExprName   string `json:"expr_name"`
	Hours      int    `json:"hours"`
	State      int    `json:"state"`
	StartedAt  int64  `json:"started_at"`
	EndAt      int64  `json:"end_at"`
	TimeLeft   int64  `json:"time_left"`
	Carrymoney int64  `json:"carrymoney"`
	AccTimes   int    `json:"acc_times"`
}

// ExprType 是历练类型配置行（cfg_hero_expr_types，原 dump 丢失、种子为合成值）。
type ExprType struct {
	Type      int    `json:"type"`
	Name      string `json:"name"`
	MinHour   int    `json:"min_hour"`
	MaxHour   int    `json:"max_hour"`
	HourMoney int64  `json:"hour_money"`
	HourGold  int64  `json:"hour_gold"`
}

// HeroState 是单个武将的完整状态（列表/详情共用）。
type HeroState struct {
	HID           int    `json:"hid"`
	Name          string `json:"name"`
	Sex           int    `json:"sex"`
	Face          int    `json:"face"`
	HeroType      int    `json:"hero_type"`
	NpcID         int    `json:"npc_id"`
	Level         int    `json:"level"`
	Exp           int64  `json:"exp"`
	State         int    `json:"state"`
	Loyalty       int    `json:"loyalty"`
	HeroHealth    int    `json:"hero_health"`
	CommandBase   int    `json:"command_base"`
	CommandAddOn  int    `json:"command_add_on"`
	BraveryBase   int    `json:"bravery_base"`
	BraveryAdd    int    `json:"bravery_add"`
	BraveryAddOn  int    `json:"bravery_add_on"`
	WisdomBase    int    `json:"wisdom_base"`
	WisdomAdd     int    `json:"wisdom_add"`
	WisdomAddOn   int    `json:"wisdom_add_on"`
	AffairsBase   int    `json:"affairs_base"`
	AffairsAdd    int    `json:"affairs_add"`
	AffairsAddOn  int    `json:"affairs_add_on"`
	AttackBase    int    `json:"attack_base"`
	AttackAddOn   int    `json:"attack_add_on"`
	DefenceBase   int    `json:"defence_base"`
	DefenceAddOn  int    `json:"defence_add_on"`
	LevelTotalExp int64  `json:"level_total_exp"`
	UpgradeExp    int64  `json:"upgrade_exp"`
	NeedExp       int64  `json:"need_exp"`
	CanUpgrade    bool   `json:"can_upgrade"`
	NoUpgradeMsg  string `json:"no_upgrade_msg"`
	Expr          *Expr  `json:"expr"`
}

// Info 是武将面板聚合信息（GET /heroes/info）。
// Toomany 对齐 legacy beginExprHero 的 return [1,toomany] 分支（非错误提示）。
type Info struct {
	Heroes    []HeroState `json:"heroes"`
	ExprTypes []ExprType  `json:"exprTypes"`
	Toomany   string      `json:"toomany,omitempty"`
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

// levelRow 是 cfg_hero_levels 一行；无行时 legacy sql_fetch_one_cell 返回 false→0。
type levelRow struct {
	total, upgrade int64
}

// cellInt64Zero 读取单列，无行→0（对齐 legacy sql_fetch_one_cell false 语义）。
func (s *Service) cellInt64Zero(ctx context.Context, query string, args ...any) (int64, error) {
	v, err := s.db.FetchCellInt64(ctx, query, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return v, err
}

// levelConfig 读取 cfg_hero_levels 到 level 映射。
func (s *Service) levelConfig(ctx context.Context) (map[int]levelRow, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_hero_levels")
	if err != nil {
		return nil, err
	}
	out := make(map[int]levelRow, len(rows))
	for _, r := range rows {
		out[model.Int(r, "level")] = levelRow{
			total:   model.Int64(r, "total_exp"),
			upgrade: model.Int64(r, "upgrade_exp"),
		}
	}
	return out, nil
}

// getMaxHeroLevel 对齐 GoodsFunc.php:1490 getMaxHeroLevel(npcid) + isInBigHero(HeroFunc:2120)。
func getMaxHeroLevel(npcid int) int {
	if bighidsForMaxLevel[npcid] {
		return 125
	}
	return 120
}

// checkHeroLevel 对齐 HeroFunc.php:2247。
// 降级：sys_user_level 无表 → mUserLevel 恒 0（userLevel≥1 的调用恒 false）。
func (s *Service) checkHeroLevel(ctx context.Context, uid, userLevel, heroLevel int) (bool, error) {
	mHeroLevel, err := s.cellInt64Zero(ctx, "select level from heroes where user_id=? and hero_type=1000 limit 1", uid)
	if err != nil {
		return false, err
	}
	mUserLevel := int64(0) // sys_user_level 无表
	return mHeroLevel >= int64(heroLevel) && mUserLevel >= int64(userLevel), nil
}

// heroMaxLevel 计算某武将的等级上限（upgradeHero:136-142）。
func (s *Service) heroMaxLevel(ctx context.Context, uid, npcID, heroType int) (int, error) {
	maxLevel := getMaxHeroLevel(npcID)
	if heroType == 1000 {
		maxLevel = 100
		ok, err := s.checkHeroLevel(ctx, uid, 10, 100)
		if err != nil {
			return 0, err
		}
		if ok {
			maxLevel = 125
		}
	}
	return maxLevel, nil
}

// updateCityHeroChange 对齐 utils.php:677：重算俸禄 hero_fee 写 city_resources。
func (s *Service) updateCityHeroChange(ctx context.Context, uid, cid int) error {
	fee, err := s.cellInt64Zero(ctx, `select coalesce(sum(level*20+(GREATEST(affairs_base+affairs_add-90,0)+GREATEST(bravery_base+bravery_add-90,0)+GREATEST(wisdom_base+wisdom_add-90,0))*50),0)+1-1
		from heroes where hero_type<>1000 and city_id=? and user_id=? and state<>5 and state<>6 and state<>9`, cid, uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "update city_resources set hero_fee=? where city_id=?", fee, cid)
	return err
}

// insertHeroBaseAdds 对齐 HeroFunc.php:210（GREATEST(0,…) 夹底）。
func (s *Service) insertHeroBaseAdds(ctx context.Context, hid, uid, braveryAdd, wisdomAdd, affairsAdd, commandAdd, attackAdd, defenceAdd int) error {
	if _, err := s.db.Exec(ctx, `insert into hero_base_add
		(user_id, hero_id, bravery_base_add_on, wisdom_base_add_on, affairs_base_add_on, command_base_add_on, type)
		values (?,?,?,?,?,?,1)`, uid, hid, braveryAdd, wisdomAdd, affairsAdd, commandAdd); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `update heroes set
		bravery_base=GREATEST(0,bravery_base+?), wisdom_base=GREATEST(0,wisdom_base+?),
		affairs_base=GREATEST(0,affairs_base+?), command_base=GREATEST(0,command_base+?),
		attack_add_on=GREATEST(0,attack_add_on+?), defence_add_on=GREATEST(0,defence_add_on+?)
		where id=?`, braveryAdd, wisdomAdd, affairsAdd, commandAdd, attackAdd, defenceAdd, hid)
	return err
}

// floorPct 对齐 PHP floor($base*$attValue)。
func floorPct(base int, rate float64) int {
	return int(math.Floor(float64(base) * rate))
}

// Upgrade 对齐 HeroFunc.php:121 upgradeHero。
func (s *Service) Upgrade(ctx context.Context, uid, cid, hid int) (*Info, error) {
	hero, err := s.db.FetchOne(ctx, "select * from heroes where city_id=? and user_id=? and id=?", cid, uid, hid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errHero(msgCantUpgradeThis)
	}
	if err != nil {
		return nil, err
	}
	if !isHeroInCity(model.Int(hero, "state")) {
		return nil, errHero(msgCantUpgradeOut)
	}
	// mem_state state=197 无表 → yysType 恒 0（legacy 默认服分支）。
	npcID := model.Int(hero, "npc_id")
	heroType := model.Int(hero, "hero_type")
	level := model.Int(hero, "level")
	maxLevel, err := s.heroMaxLevel(ctx, uid, npcID, heroType)
	if err != nil {
		return nil, err
	}
	if level >= maxLevel {
		if level > maxLevel {
			if _, err := s.db.Exec(ctx, "update heroes set level=? where id=?", maxLevel, hid); err != nil {
				return nil, err
			}
		}
		return nil, errHero(msgLevelMax)
	}
	// legacy 重读一行（update 后）。
	cur, err := s.cellInt64Zero(ctx, "select total_exp from cfg_hero_levels where level=?", level)
	if err != nil {
		return nil, err
	}
	upgradeExp, err := s.cellInt64Zero(ctx, "select upgrade_exp from cfg_hero_levels where level=?", level+1)
	if err != nil {
		return nil, err
	}
	exp := model.Int64(hero, "exp")
	if exp-cur >= upgradeExp {
		switch {
		case level == 119 && myBigHids[hid]:
			// 119 级突破名单：+6 级并四维 floor(base×0.05)、攻/防 add_on=125。
			ba := floorPct(model.Int(hero, "bravery_base"), 0.05)
			wi := floorPct(model.Int(hero, "wisdom_base"), 0.05)
			af := floorPct(model.Int(hero, "affairs_base"), 0.05)
			co := floorPct(model.Int(hero, "command_base"), 0.05)
			if _, err := s.db.Exec(ctx, "update heroes set level=level+6 where id=?", hid); err != nil {
				return nil, err
			}
			if err := s.insertHeroBaseAdds(ctx, hid, uid, ba, wi, af, co, 125, 125); err != nil {
				return nil, err
			}
		case level == 124:
			// 124 级无条件突破（level=125 绝对赋值）。
			ba := floorPct(model.Int(hero, "bravery_base"), 0.05)
			wi := floorPct(model.Int(hero, "wisdom_base"), 0.05)
			af := floorPct(model.Int(hero, "affairs_base"), 0.05)
			co := floorPct(model.Int(hero, "command_base"), 0.05)
			if _, err := s.db.Exec(ctx, "update heroes set level=125 where id=?", hid); err != nil {
				return nil, err
			}
			if err := s.insertHeroBaseAdds(ctx, hid, uid, ba, wi, af, co, 125, 125); err != nil {
				return nil, err
			}
		default:
			if _, err := s.db.Exec(ctx, "update heroes set level=level+1 where id=?", hid); err != nil {
				return nil, err
			}
		}
		// mem_assemble 无表 → 君主集结等级同步省略。
	} else {
		return nil, errHero(msgNoEnoughExp)
	}
	// regenerateHeroAttri 依赖装备/派生属性表（新库无）→ 省略（M5 装备批次处理属性口径）。
	// completeTask(86) 属 M8，未接线。
	if err := s.updateCityHeroChange(ctx, uid, cid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// AddPoint 对齐 HeroFunc.php:225 addHeroPoint（param: affairs/bravery/wisdom 为目标总值）。
func (s *Service) AddPoint(ctx context.Context, uid, cid, hid, affairs, bravery, wisdom int) (*Info, error) {
	hero, err := s.db.FetchOne(ctx, "select * from heroes where city_id=? and user_id=? and id=?", cid, uid, hid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errHero(msgCantFindHero)
	}
	if err != nil {
		return nil, err
	}
	if !isHeroInCity(model.Int(hero, "state")) {
		return nil, errHero(msgCantAddOut)
	}
	level := model.Int(hero, "level")
	levelAdd := int64(level)
	if model.Int(hero, "hero_type") == 1000 {
		levelAdd = 3 * int64(level)
		// checkHeroLevel(uid,8,70) 且君主将 → +50；sys_user_level 无表恒 false → 省略（降级）。
	}
	baseSum := int64(model.Int(hero, "affairs_base") + model.Int(hero, "bravery_base") + model.Int(hero, "wisdom_base"))
	if int64(affairs+bravery+wisdom) > baseSum+levelAdd {
		return nil, errHero(msgNoExtraPotential)
	}
	affairsAdd := affairs - model.Int(hero, "affairs_base")
	braveryAdd := bravery - model.Int(hero, "bravery_base")
	wisdomAdd := wisdom - model.Int(hero, "wisdom_base")
	if model.Int(hero, "affairs_add") > affairsAdd || model.Int(hero, "bravery_add") > braveryAdd || model.Int(hero, "wisdom_add") > wisdomAdd {
		// legacy 此处 echo >> /waigua.uid 记录后抛错；文件写入省略。
		return nil, errHero(msgXidianUnvalid)
	}
	if _, err := s.db.Exec(ctx, "update heroes set affairs_add=?, bravery_add=?, wisdom_add=? where id=?",
		affairsAdd, braveryAdd, wisdomAdd, hid); err != nil {
		return nil, err
	}
	if err := s.updateCityHeroChange(ctx, uid, cid); err != nil {
		return nil, err
	}
	chiefhid, err := s.cellInt64Zero(ctx, "select chief_hero_id from cities where id=?", cid)
	if err != nil {
		return nil, err
	}
	if int(chiefhid) == hid {
		// updateCityChiefResAdd → updateCityResourceAdd：mem_world 无表 → ownercid=cid，
		// 即置 city_res_add.resource_changing=1。
		if err := s.markResChanging(ctx, cid); err != nil {
			return nil, err
		}
	}
	// completeTask(87) 属 M8，未接线。
	return s.Info(ctx, uid, cid)
}

// ClearPoint 对齐 HeroFunc.php:274 clearHeroPoint → GoodsFunc.php:1657 useXiShuiDan。
func (s *Service) ClearPoint(ctx context.Context, uid, cid, hid int) (*Info, error) {
	hero, err := s.db.FetchOne(ctx, "select * from heroes where city_id=? and user_id=? and id=?", cid, uid, hid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errHero(msgCantFindHero)
	}
	if err != nil {
		return nil, err
	}
	if !isHeroInCity(model.Int(hero, "state")) {
		return nil, errHero(msgCantCleanOut)
	}
	// useXiShuiDan：洗髓丹(22) 消耗 ceil(level/10)。
	goodsNeed := int64(math.Ceil(float64(model.Int(hero, "level")) / 10))
	ok, err := s.checkGoodsCount(ctx, uid, 22, goodsNeed)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errHero(fmt.Sprintf("not_enough_goods22#%d", goodsNeed))
	}
	if _, err := s.db.Exec(ctx, "update heroes set affairs_add=0, bravery_add=0, wisdom_add=0 where id=?", hid); err != nil {
		return nil, err
	}
	if err := s.reduceGoods(ctx, uid, 22, goodsNeed); err != nil {
		return nil, err
	}
	if err := s.updateCityHeroChange(ctx, uid, cid); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// markResChanging 保证 city_res_add 有行并置 resource_changing=1。
func (s *Service) markResChanging(ctx context.Context, cid int) error {
	_, err := s.db.Exec(ctx, `insert into city_res_add (city_id, resource_changing) values (?,1)
		on duplicate key update resource_changing=1`, cid)
	return err
}

// exprTypes 读取历练类型配置。
func (s *Service) exprTypes(ctx context.Context) ([]ExprType, error) {
	rows, err := s.db.FetchRows(ctx, "select * from cfg_hero_expr_types order by type")
	if err != nil {
		return nil, err
	}
	out := make([]ExprType, 0, len(rows))
	for _, r := range rows {
		out = append(out, ExprType{
			Type:      model.Int(r, "type"),
			Name:      model.Str(r, "name"),
			MinHour:   model.Int(r, "min_hour"),
			MaxHour:   model.Int(r, "max_hour"),
			HourMoney: model.Int64(r, "hour_money"),
			HourGold:  model.Int64(r, "hour_gold"),
		})
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
	types, err := s.exprTypes(ctx)
	if err != nil {
		return nil, err
	}
	nameByType := map[int]string{}
	for _, t := range types {
		nameByType[t.Type] = t.Name
	}
	exprByHero, err := s.exprsByHero(ctx, cid, now, nameByType)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.FetchRows(ctx, "select * from heroes where city_id=? and user_id=? order by id", cid, uid)
	if err != nil {
		return nil, err
	}
	list := make([]HeroState, 0, len(rows))
	for _, r := range rows {
		h, err := s.heroState(ctx, uid, r, cfgs, exprByHero)
		if err != nil {
			return nil, err
		}
		list = append(list, h)
	}
	return &Info{Heroes: list, ExprTypes: types}, nil
}

func (s *Service) exprsByHero(ctx context.Context, cid int, now int64, nameByType map[int]string) (map[int]*Expr, error) {
	rows, err := s.db.FetchRows(ctx, "select * from hero_exprs where city_id=?", cid)
	if err != nil {
		return nil, err
	}
	out := make(map[int]*Expr, len(rows))
	for _, r := range rows {
		typ := model.Int(r, "expr_type")
		left := model.Int64(r, "end_at") - now
		if left < 0 {
			left = 0
		}
		out[model.Int(r, "hero_id")] = &Expr{
			ID:         model.Int(r, "id"),
			ExprType:   typ,
			ExprName:   nameByType[typ],
			Hours:      model.Int(r, "hours"),
			State:      model.Int(r, "state"),
			StartedAt:  model.Int64(r, "started_at"),
			EndAt:      model.Int64(r, "end_at"),
			TimeLeft:   left,
			Carrymoney: model.Int64(r, "carrymoney"),
			AccTimes:   model.Int(r, "acc_times"),
		}
	}
	return out, nil
}

func (s *Service) heroState(ctx context.Context, uid int, r map[string]any, cfgs map[int]levelRow, exprs map[int]*Expr) (HeroState, error) {
	hid := model.Int(r, "id")
	level := model.Int(r, "level")
	h := HeroState{
		HID:          hid,
		Name:         model.Str(r, "name"),
		Sex:          model.Int(r, "sex"),
		Face:         model.Int(r, "face"),
		HeroType:     model.Int(r, "hero_type"),
		NpcID:        model.Int(r, "npc_id"),
		Level:        level,
		Exp:          model.Int64(r, "exp"),
		State:        model.Int(r, "state"),
		Loyalty:      model.Int(r, "loyalty"),
		HeroHealth:   model.Int(r, "hero_health"),
		CommandBase:  model.Int(r, "command_base"),
		CommandAddOn: model.Int(r, "command_add_on"),
		BraveryBase:  model.Int(r, "bravery_base"),
		BraveryAdd:   model.Int(r, "bravery_add"),
		BraveryAddOn: model.Int(r, "bravery_add_on"),
		WisdomBase:   model.Int(r, "wisdom_base"),
		WisdomAdd:    model.Int(r, "wisdom_add"),
		WisdomAddOn:  model.Int(r, "wisdom_add_on"),
		AffairsBase:  model.Int(r, "affairs_base"),
		AffairsAdd:   model.Int(r, "affairs_add"),
		AffairsAddOn: model.Int(r, "affairs_add_on"),
		AttackBase:   model.Int(r, "attack_base"),
		AttackAddOn:  model.Int(r, "attack_add_on"),
		DefenceBase:  model.Int(r, "defence_base"),
		DefenceAddOn: model.Int(r, "defence_add_on"),
		Expr:         exprs[hid],
	}
	cur := cfgs[level]
	next := cfgs[level+1]
	h.LevelTotalExp = cur.total
	h.UpgradeExp = next.upgrade
	need := next.upgrade - (h.Exp - cur.total)
	if need < 0 {
		need = 0
	}
	h.NeedExp = need
	h.CanUpgrade = false
	maxLevel, err := s.heroMaxLevel(ctx, uid, h.NpcID, h.HeroType)
	if err != nil {
		return h, err
	}
	switch {
	case !isHeroInCity(h.State):
		h.NoUpgradeMsg = msgCantUpgradeOut
	case h.Level >= maxLevel:
		h.NoUpgradeMsg = msgLevelMax
	case h.Exp-cur.total < next.upgrade:
		h.NoUpgradeMsg = msgNoEnoughExp
	default:
		h.CanUpgrade = true
	}
	return h, nil
}
