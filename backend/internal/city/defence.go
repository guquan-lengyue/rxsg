package city

// defence.go —— R11-2 项③：1:1 复刻 legacy server/game/DefenceFunc.php:40 的 doGetDefenceInfo
//（城墙面板的“城防器械信息”）及 :4 的 getDefenceSpeedRate。
//
// 数据来源（legacy）：
//   - cfg_defence(cfg_defence)：器械配置（did/name/hp/ap/dp/range/speed/carry/*_need/area_need/time_need/description）
//   - sys_city_defence         ：城池器械存量（cid/did/count），新库同名表 city_defences
//   - cfg_defence_condition    ：器械前置条件，新库同名表 cfg_defence_conditions（本次建空表）
//
// 与 legacy 的差异（不可 1:1 之处，汇报同步列出）：
//   - cfg_defence 原始 dump 已丢失（0011 已声明为合成值 hp/ap/dp/range）。doGetDefenceInfo 读取的
//     description/speed/carry/*_need/area_need/time_need 在新库原缺失，本次 0016 迁移按 legacy 列名补齐
//     （结构对齐），但原配置数值无来源 → 全部保持 0/空串，【非复刻数值】。因此 reinforce_time = max(1, 0*…)=1。
//   - 城防加固/解散/加速队列（startReinforceQueue/stopReinforceQueue/dissolveDefence/accReinforceQueue）
//     依赖 sys_city_reinforcequeue/mem_city_reinforce，本项只读，不移植。
//   - 鬼斧神工/文曲星符（cfg_book/sys_user_book、mem_hero_buffer）新库无表 → 相关系数取 0/false。

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"rxsg/backend/internal/building"
	"rxsg/backend/internal/model"
)

// WallDefenceCondition 对齐 legacy interface.php:51 UpgradeCondition（doGetDefenceInfo 只产出 0/1 两类）。
type WallDefenceCondition struct {
	Type        string `json:"type"`
	UpgradeNeed string `json:"upgradeNeed"`
	CurrentOwn  string `json:"currentOwn"`
	CanUpgrade  bool   `json:"canUpgrade"`
}

// WallDefence 对齐 legacy interface.php:159 WallDefenceState。
// 附加 cid 字段以对齐既有 model.Defence / 前端 CityDefence（legacy WallDefenceState 无 cid）。
type WallDefence struct {
	CID           int                    `json:"cid"`
	DID           int                    `json:"did"`
	DName         string                 `json:"dname"`
	Count         int64                  `json:"count"`
	Description   string                 `json:"description"`
	Hp            int64                  `json:"hp"`
	Ap            int64                  `json:"ap"`
	Dp            int64                  `json:"dp"`
	Range         int64                  `json:"range"`
	Speed         int64                  `json:"speed"`
	Carry         int64                  `json:"carry"`
	CanReinforce  bool                   `json:"can_reinforce"`
	WoodNeed      int64                  `json:"woodNeed"`
	RockNeed      int64                  `json:"rockNeed"`
	IronNeed      int64                  `json:"ironNeed"`
	FoodNeed      int64                  `json:"foodNeed"`
	GoldNeed      int64                  `json:"goldNeed"`
	AreaNeed      int64                  `json:"areaNeed"`
	ReinforceTime int64                  `json:"reinforce_time"`
	Conditions    []WallDefenceCondition `json:"conditions"`
}

// defenceSpeedRate 对齐 getDefenceSpeedRate（DefenceFunc.php:4）。
// 建筑技术17 + 城守内政（注意：legacy 此处不含 affairs_add_on）；鬼斧神工技能新库无表 → 0。
func (s *Service) defenceSpeedRate(ctx context.Context, cid int) (float64, error) {
	speedAdd := 0.0

	techLevel, err := s.db.FetchCellInt64(ctx, "select level from city_technics where city_id=? and technic_id=17", cid)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if techLevel > 0 {
		speedAdd += float64(techLevel) * 100
	}

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
			speedAdd += float64(model.Int(chief, "affairs_base") + model.Int(chief, "affairs_add"))
		}
	}

	skillRate := 0.0 // 城防加固技能（cfg_book bid=17）：新库无表 → 0
	return (1.0 / (1.0 + 0.01*speedAdd)) * (1 - skillRate), nil
}

// GetDefences 对齐 doGetDefenceInfo（DefenceFunc.php:40）：cfg_defence LEFT JOIN city_defences。
func (s *Service) GetDefences(ctx context.Context, uid, cid int) ([]WallDefence, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}

	rows, err := s.db.FetchRows(ctx,
		"select d.*, c.count from cfg_defence d left join city_defences c on c.city_id=? and c.did=d.did order by d.did", cid)
	if err != nil {
		return nil, err
	}

	speedAdd, err := s.defenceSpeedRate(ctx, cid)
	if err != nil {
		return nil, err
	}

	out := make([]WallDefence, 0, len(rows))
	for _, r := range rows {
		did := model.Int(r, "did")
		wd := WallDefence{
			CID:          cid,
			DID:          did,
			DName:        model.Str(r, "name"),
			Count:        model.Int64(r, "count"),
			Description:  model.Str(r, "description"),
			Hp:           model.Int64(r, "hp"),
			Ap:           model.Int64(r, "ap"),
			Dp:           model.Int64(r, "dp"),
			Range:        model.Int64(r, "g_range"),
			Speed:        model.Int64(r, "speed"),
			Carry:        model.Int64(r, "carry"),
			WoodNeed:     model.Int64(r, "wood_need"),
			RockNeed:     model.Int64(r, "rock_need"),
			IronNeed:     model.Int64(r, "iron_need"),
			FoodNeed:     model.Int64(r, "food_need"),
			GoldNeed:     model.Int64(r, "gold_need"),
			AreaNeed:     model.Int64(r, "area_need"),
			CanReinforce: true,
			Conditions:   []WallDefenceCondition{},
		}
		// legacy: max(1, time_need * speedAdd)
		rt := int64(float64(model.Int64(r, "time_need")) * speedAdd)
		if rt < 1 {
			rt = 1
		}
		wd.ReinforceTime = rt

		conds, err := s.defenceConditions(ctx, uid, cid, did)
		if err != nil {
			return nil, err
		}
		for _, cd := range conds {
			if !cd.CanUpgrade {
				wd.CanReinforce = false
			}
			wd.Conditions = append(wd.Conditions, cd)
		}
		out = append(out, wd)
	}
	return out, nil
}

// defenceConditions 对齐 doGetDefenceInfo 的前置条件段（DefenceFunc.php:68-104），只处理 pre_type 0/1。
func (s *Service) defenceConditions(ctx context.Context, uid, cid, did int) ([]WallDefenceCondition, error) {
	rows, err := s.db.FetchRows(ctx,
		"select * from cfg_defence_conditions where did=? order by pre_type", did)
	if err != nil {
		return nil, err
	}
	out := make([]WallDefenceCondition, 0, len(rows))
	for _, cond := range rows {
		cd := WallDefenceCondition{CanUpgrade: true}
		switch model.Int(cond, "pre_type") {
		case 0: // 建筑
			cd.Type = "前提建筑"
			preNew, ok := building.LegacyToNewBid(model.Int(cond, "pre_id"))
			var (
				curr int64
				name string
			)
			if ok {
				curr, err = s.db.FetchCellInt64(ctx,
					"select max(level) from buildings where city_id=? and building_id=?", cid, preNew)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return nil, err
				}
				name, _ = s.db.FetchCellString(ctx, "select name from cfg_buildings where bid=?", preNew)
			}
			preLevel := model.Int(cond, "pre_level")
			cd.UpgradeNeed = name + "(等级" + strconv.Itoa(preLevel) + ")"
			cd.CurrentOwn = "等级" + strconv.FormatInt(curr, 10)
			if curr < int64(preLevel) {
				cd.CanUpgrade = false
			}
		case 1: // 科技
			cd.Type = "前提科技"
			preID := model.Int(cond, "pre_id")
			curr, err := s.db.FetchCellInt64(ctx,
				"select max(level) from city_technics where city_id=? and technic_id=?", cid, preID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			name, _ := s.db.FetchCellString(ctx, "select name from cfg_technics where tid=?", preID)
			preLevel := model.Int(cond, "pre_level")
			cd.UpgradeNeed = name + "(等级" + strconv.Itoa(preLevel) + ")"
			cd.CurrentOwn = "等级" + strconv.FormatInt(curr, 10)
			if curr < int64(preLevel) {
				cd.CanUpgrade = false
			}
		}
		out = append(out, cd)
	}
	return out, nil
}
