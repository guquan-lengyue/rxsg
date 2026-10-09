package tavern

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/lock"
	"rxsg/backend/internal/model"
)

// service.go 1:1 复刻 legacy server/game/HotelFunc.php 的客栈招募主链：
//   getHotelInfo(970) / doGetRecruitHero(246) / generateRecruitHero(177) /
//   recruitHero(990) / resetRecruitHero(1067) + GoodsFunc.php useZhaoXinLin(1671) /
//   utils.php cityHasHeroPosition(757) / getBufferNobility(1534) / addCityResources(193) /
//   updateCityHeroChange(677) + BuildingFunc.php doGetBuildingInfo(239) HOTEL 分支。
//
// bid 映射：legacy ID_BUILDING_HOTEL=10 与重写版「校场=10」冲突，客栈改用扩展槽位 12；
// 官署沿用 legacy 11（office.go）。
//
// 原版怪癖 1:1 保留：
//   - getHotelInfo/recruitHero/resetRecruitHero 均不校验城池归属（无 checkCityOwner）——
//     任何 uid 可对任意 cid 的池子操作；
//   - recruitHero 池行不存在时静默无操作，仍返回酒店信息（非错误）；
//   - 野兵填充：select count from sys_recruit_hero where cid=0 <1000 时生成 1 个（每次请求至多 1 个）；
//   - 招贤榜：useZhaoXinLin 把 last_reset_recruit 置 0，下一次 doGetRecruitHero 读 empty(0)
//     重新置 now，但 $last_reset_recruit 变量仍为 0 → rate=活动rate 且 blockdelta 巨大 → 全池重建；
//   - 活动将领每日上限 countLimitPerDay=1、未用招贤榜保底 rate=5%。
//
// 缺表降级（cfg_act/cfg_recruit_hero/log_act 属 M9 活动批次，本库无表）：
//   - getAvailableActByType(2001) 恒 false → generateHeroFromConfig 不触发、活动将领概率分支跳过、
//     recruitHero 的 herotype>10 上限检查 cfg_recruit_hero 无行恒通过、
//     checkAndDoRecruitHeroAct 恒 false（retmsg 恒空）；
//   - completeTask(84/89)/finishAchivement(35) 属 M8，未接线；
//   - 姓名表 mem_cfg_* 原 data/name_*.txt 丢失 → cfg_names 为合成种子（对照表已声明）。

const (
	hotelBuildingID  = 12 // 客栈（legacy ID_BUILDING_HOTEL=10 与重写版校场冲突 → 扩展槽位）
	officeBuildingID = 11 // 官署（ID_BUILDING_OFFICE，容量位 cityHasHeroPosition）

	gameSpeedRate = 10 // common.php:29 单倍速段生效值（注释「10倍速」为原版错位）
	recruitBlock  = 10800

	zhaoXinLinGID = 23 // 招贤榜
)

// lang 文案（server/game/lang.php 逐字）。
const (
	msgNoHotelBuilt    = "该城池尚未建造客栈。"
	msgNoEnoughGold    = "本城池的黄金不足，不能招募该将领。"
	msgHotelLevelLow   = "招贤馆等级不够，不能容纳更多的将领。"
	msgNoZhaoXinLin    = "not_enough_goods23"
	msgNoLevelData     = "没有该级别数据"
	msgAlreadyHaveOne  = "你已经招募了%s名“%s”，需要解雇原来的将领才能重新招募！"
	actMsgTipFormat    = "MSG_ACTTIP|%s"
)

type Service struct {
	db *db.DB
	lk *lock.Locker
}

func NewService(d *db.DB) *Service {
	return &Service{db: d, lk: lock.New(d.DB)}
}

// WithUserLock 暴露用户级锁给 handler。
func (s *Service) WithUserLock(ctx context.Context, uid int, key string, fn func(context.Context) error) error {
	return s.lk.WithUserLock(ctx, uid, key, fn)
}

// errTavern 构造与 legacy throw new Exception 等价的错误响应。
func errTavern(msg string) error {
	return httpx.BadRequest("tavern_error", msg)
}

// Recruit 是招募池中的一个将领（recruit_heroes ← sys_recruit_hero）。
type Recruit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Sex         int    `json:"sex"`
	Face        int    `json:"face"`
	Level       int    `json:"level"`
	AffairsBase int    `json:"affairs_base"`
	BraveryBase int    `json:"bravery_base"`
	WisdomBase  int    `json:"wisdom_base"`
	CommandBase int    `json:"command_base"`
	AffairsAdd  int    `json:"affairs_add"`
	BraveryAdd  int    `json:"bravery_add"`
	WisdomAdd   int    `json:"wisdom_add"`
	CommandAdd  int    `json:"command_add"`
	Loyalty     int    `json:"loyalty"`
	GoldNeed    int64  `json:"gold_need"`
	HeroType    int    `json:"hero_type"`
	IsActHero   bool   `json:"is_act_hero"`
}

// Info 是客栈面板聚合（GET /hotel/info；对齐 doGetBuildingInfo HOTEL 分支返回结构）。
type Info struct {
	HotelLevel    int       `json:"hotel_level"`
	HotelName     string    `json:"hotel_name"`
	OfficePos     int       `json:"office_pos"`     // doGetOfficeValidPosition = 官署等级 - 城内将数
	CanJiejiao    bool      `json:"can_jiejiao"`    // getBufferNobility(uid) >= 5
	Recruits      []Recruit `json:"recruits"`
	Tip           string    `json:"tip,omitempty"`  // checkAndDoRecruitHeroAct 活动提示（无活动表恒空）
}

// Info 对齐 getHotelInfo：野兵填充 → 客栈建筑校验 → 招募池刷新 → 聚合返回。
func (s *Service) Info(ctx context.Context, uid, cid int) (*Info, error) {
	// 1. 野兵填充（getHotelInfo:973-978，怪癖：每次请求至多补 1 个）。
	npcCount, err := s.cellInt64Zero(ctx, "select count(*) from recruit_heroes where city_id=0")
	if err != nil {
		return nil, err
	}
	if npcCount < 1000 {
		if err := s.generateRecruitHero(ctx, 0, 5); err != nil {
			return nil, err
		}
	}

	// 2. 客栈建筑（getHotelInfo:981 order by level desc limit 1；无行 throw）。
	//    怪癖保留：legacy 不校验城池归属。
	hotel, err := s.db.FetchOne(ctx, `select b.*, c.name as bname from buildings b
		left join cfg_buildings c on c.bid=b.building_id
		where b.city_id=? and b.building_id=? order by b.level desc limit 1`, cid, hotelBuildingID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errTavern(msgNoHotelBuilt)
	}
	if err != nil {
		return nil, err
	}
	level := model.Int(hotel, "level")

	// 3. 招募池（doGetRecruitHero）。
	recruits, err := s.doGetRecruitHero(ctx, uid, cid, level)
	if err != nil {
		return nil, err
	}

	// 4. 官署空位 + 爵位（doGetBuildingInfo:261-269）。
	pos, err := s.officeValidPosition(ctx, uid, cid)
	if err != nil {
		return nil, err
	}
	nobility, err := s.bufferNobility(ctx, uid)
	if err != nil {
		return nil, err
	}

	return &Info{
		HotelLevel: level,
		HotelName:  model.Str(hotel, "bname"),
		OfficePos:  pos,
		CanJiejiao: nobility >= 5,
		Recruits:   recruits,
	}, nil
}

// RecruitHero 对齐 recruitHero(990)：池行存在才处理；扣城金 → 入 heroes(state=0) →
// hero_blood 上限 → 删池行 → updateCityHeroChange → 返回酒店信息。
// 怪癖：无城池归属校验；池行不存在静默成功；活动将领上限检查因 cfg_recruit_hero 无表恒通过。
func (s *Service) RecruitHero(ctx context.Context, uid, cid, id int) (*Info, error) {
	row, err := s.db.FetchOne(ctx, "select * from recruit_heroes where city_id=? and id=? limit 1", cid, id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if row != nil {
		ok, err := s.cityHasHeroPosition(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errTavern(msgHotelLevelLow)
		}
		goldNeed := model.Int64(row, "gold_need")
		gold, err := s.cellInt64Zero(ctx, "select gold from city_resources where city_id=?", cid)
		if err != nil {
			return nil, err
		}
		if goldNeed > gold {
			return nil, errTavern(msgNoEnoughGold)
		}
		// herotype>10 活动将领上限：cfg_recruit_hero 无表 → legacy $hero=false → userhavecnt null → 恒通过。

		// 扣城金（addCityResources 位序 wood,rock,iron,food,gold）。
		if _, err := s.db.Exec(ctx, "update city_resources set gold=gold-? where city_id=?", goldNeed, cid); err != nil {
			return nil, err
		}

		lv := model.Int(row, "level")
		braveryBase := model.Int(row, "bravery_base")
		braveryAdd := model.Int(row, "bravery_add")
		wisdomBase := model.Int(row, "wisdom_base")
		wisdomAdd := model.Int(row, "wisdom_add")
		hid, err := s.db.Insert(ctx, `insert into heroes
			(user_id, name, sex, face, city_id, state, level, exp,
			 affairs_base, bravery_base, wisdom_base, command_base,
			 affairs_add, bravery_add, wisdom_add, command_add_on, loyalty, hero_type)
			values (?,?,?,?,?,0,?,?,?,?,?,?,?,?,?,?,?,?)`,
			uid,
			model.Str(row, "name"), model.Int(row, "sex"), model.Int(row, "face"), cid,
			lv, model.Int64(row, "exp"),
			model.Int(row, "affairs_base"), braveryBase, wisdomBase, model.Int(row, "command_base"),
			model.Int(row, "affairs_add"), braveryAdd, wisdomAdd, model.Int(row, "command_add"),
			model.Int(row, "loyalty"), model.Int(row, "hero_type"))
		if err != nil {
			return nil, err
		}
		// mem_hero_blood → hero_blood（recruitHero:1017-1019）。
		forceMax := 100 + lv/5 + (braveryBase+braveryAdd)/3
		energyMax := 100 + lv/5 + (wisdomBase+wisdomAdd)/3
		if _, err := s.db.Exec(ctx,
			"insert into hero_blood (hero_id, `force`, force_max, energy, energy_max) values (?,?,?,100,?)",
			hid, 100, forceMax, energyMax); err != nil {
			return nil, err
		}
		// checkAndDoRecruitHeroAct：cfg_act/cfg_recruit_hero 无表 → 恒 false（retmsg 空）。
		if _, err := s.db.Exec(ctx, "delete from recruit_heroes where id=?", id); err != nil {
			return nil, err
		}
		if err := s.updateCityHeroChange(ctx, uid, cid); err != nil {
			return nil, err
		}
		// completeTask(84)/finishAchivement(35) 属 M8，未接线。
	}
	return s.Info(ctx, uid, cid)
}

// Reset 对齐 resetRecruitHero(1067) → useZhaoXinLin(1671)：
// 无招贤榜 throw "not_enough_goods23"（字面量）；置 last_reset_recruit=0（无行则插入）；扣 1。
func (s *Service) Reset(ctx context.Context, uid, cid int) (*Info, error) {
	ok, err := s.checkGoods(ctx, uid, zhaoXinLinGID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errTavern(msgNoZhaoXinLin)
	}
	if _, err := s.db.Exec(ctx,
		"insert into city_schedule (city_id, last_reset_recruit) values (?,0) on duplicate key update last_reset_recruit=0",
		cid); err != nil {
		return nil, err
	}
	// completeTask(89) 属 M8，未接线。
	if err := s.reduceGoods(ctx, uid, zhaoXinLinGID, 1); err != nil {
		return nil, err
	}
	return s.Info(ctx, uid, cid)
}

// cityHasHeroPosition 对齐 utils.php:757：官署等级为空→false；等级>城内将数。
func (s *Service) cityHasHeroPosition(ctx context.Context, uid, cid int) (bool, error) {
	officeLevel, err := s.cellInt64Zero(ctx,
		"select level from buildings where city_id=? and building_id=? limit 1", cid, officeBuildingID)
	if err != nil {
		return false, err
	}
	if officeLevel == 0 {
		return false, nil
	}
	heroCount, err := s.cellInt64Zero(ctx, "select count(*) from heroes where city_id=? and user_id=?", cid, uid)
	if err != nil {
		return false, err
	}
	return officeLevel > heroCount, nil
}

// officeValidPosition 对齐 OfficeFunc.php:12 doGetOfficeValidPosition。
func (s *Service) officeValidPosition(ctx context.Context, uid, cid int) (int, error) {
	officeLevel, err := s.cellInt64Zero(ctx,
		"select level from buildings where city_id=? and building_id=? limit 1", cid, officeBuildingID)
	if err != nil {
		return 0, err
	}
	heroCount, err := s.cellInt64Zero(ctx, "select count(*) from heroes where city_id=? and user_id=?", cid, uid)
	if err != nil {
		return 0, err
	}
	return int(officeLevel) - int(heroCount), nil
}

// bufferNobility 对齐 utils.php:1534 getBufferNobility（推恩后爵位）。
func (s *Service) bufferNobility(ctx context.Context, uid int) (float64, error) {
	real, err := s.cellFloat(ctx, "select nobility from users where id=?", uid)
	if err != nil {
		return 0, err
	}
	bufparam, err := s.cellInt64Zero(ctx,
		"select bufparam from user_buffers where user_id=? and (buftype=16 or buftype=18) order by bufparam desc limit 1", uid)
	if err != nil {
		return 0, err
	}
	if bufparam != 0 { // PHP !empty($bufparam)
		nb := real + float64(bufparam)
		if bufparam == 5 && nb > 19 {
			nb = 19
		}
		if bufparam == 2 && nb > 18 {
			nb = 18
		}
		if nb > real {
			return nb, nil
		}
	}
	return math.Trunc(real), nil // intval($realnobility)
}

// updateCityHeroChange 对齐 utils.php:677（与 hero 包同口径）。
func (s *Service) updateCityHeroChange(ctx context.Context, uid, cid int) error {
	fee, err := s.cellInt64Zero(ctx, `select coalesce(sum(level*20+(GREATEST(affairs_base+affairs_add-90,0)+GREATEST(bravery_base+bravery_add-90,0)+GREATEST(wisdom_base+wisdom_add-90,0))*50),0)+1-1
		from heroes where hero_type<>1000 and city_id=? and user_id=? and state<>5 and state<>6 and state<>9`, cid, uid)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, "update city_resources set hero_fee=? where city_id=?", fee, cid)
	return err
}
