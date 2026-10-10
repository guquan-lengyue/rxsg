package building

// valid.go —— R11-2 项①：1:1 复刻 legacy server/game/BuildingFunc.php 的
//   getAllValidBuilding:315（建造候选列表）及其依赖的 doGetSimpleBuildingInfo:113。
//
// 1:1 一致之处：候选筛选顺序（inner 0/1/2 三分支）、doGetSimpleBuildingInfo 的成本/耗时/资源/前置
//   条件判定、以及中文文案原文（"前提建筑"/"前提科技"/"需要物品"/"等级"/"数量"）。
//
// 与 legacy 的差异（不可 1:1 之处，汇报同步列出）：
//   - bid 空间：新库 cfg_buildings 为重写映射（见 legacy_bid.go），候选返回【新库 building_id】；
//     legacy 的 {5,9,17}（多座建筑白名单）经 legacyBidOf 翻译为 {6,8,9}。
//   - cfg_buildings 无 `description` 列 → 候选 description 恒为空串（缺列如实汇报，未改数据）；
//     levelDescription 取自 cfg_building_levels.description（存在）。
//   - inner==2（城墙）：legacy 返回 cfg_building.bid=20；新库无城墙建筑（legacy 20 未映射）→ 返回空列表。
//   - 君主将等级表 sys_user_level 新库无 → checkHeroLevel 的 userLevel 取 0（相关加成不触发）。
//   - getAllValidBuilding 原版未校验城池归属，此处追加 ensureOwner（安全加固，非语义变更）。
//
// ⚠ 原版怪癖（1:1 保留，汇报列出）：
//   1) upgradeTime = ceil(upgrade_time × speedRate)，【未】除以 GAME_SPEED_RATE —— 与 startUpgradeBuilding
//      实际开建耗时（得除以 GAME_SPEED_RATE）不一致。
//   2) 资源行缺失时取 $GLOBALS['doGetSimpleBuildingInfo']['no_resource']，而 lang.php 只定义了
//      ['noresource'] → 抛出的异常消息为空串（此处沿用空消息）。
//   3) 资源不足时仅置 canUpgrade=false，不产生任何原因文案（原因只体现在 conditions）。

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strconv"

	"rxsg/backend/internal/httpx"
	"rxsg/backend/internal/model"
)

// UpgradeCond 对齐 legacy interface.php:51 的 UpgradeCondition。
type UpgradeCond struct {
	Type        string `json:"type"`
	UpgradeNeed string `json:"upgradeNeed"`
	CurrentOwn  string `json:"currentOwn"`
	CanUpgrade  bool   `json:"canUpgrade"`
}

// BuildingCandidate 对齐 legacy interface.php:60 的 BuildingInfo（字段名逐字保留）。
type BuildingCandidate struct {
	BID              int           `json:"bid"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	LevelDescription string        `json:"levelDescription"`
	Level            int           `json:"level"`
	WoodNeed         int64         `json:"woodNeed"`
	RockNeed         int64         `json:"rockNeed"`
	IronNeed         int64         `json:"ironNeed"`
	FoodNeed         int64         `json:"foodNeed"`
	GoldNeed         int64         `json:"goldNeed"`
	PeopleNeed       int64         `json:"peopleNeed"`
	UpgradeTime      int64         `json:"upgradeTime"`
	CanUpgrade       bool          `json:"canUpgrade"`
	Conditions       []UpgradeCond `json:"conditions"`
}

// ValidBuildings 对齐 getAllValidBuilding（BuildingFunc.php:315）。
// inner：0 城外建筑 / 1 城内建筑 / 2 城墙；其余值按 0 处理（对齐原版 `else` 分支）。
func (s *Service) ValidBuildings(ctx context.Context, uid, cid, inner int) ([]BuildingCandidate, error) {
	if err := s.ensureOwner(ctx, uid, cid); err != nil {
		return nil, err
	}

	var (
		rows []map[string]any
		err  error
	)
	switch inner {
	case 2: // 城墙：新库无城墙建筑 → 空列表
		wallNew, ok := newBidOf(IDBuildingWall)
		if !ok {
			return []BuildingCandidate{}, nil
		}
		rows, err = s.db.FetchRows(ctx, "select * from cfg_buildings where bid=?", wallNew)
	case 1: // 其它城内建筑
		rows, err = s.db.FetchRows(ctx, "select * from cfg_buildings where `inner`=1 order by bid")
	default: // 城外建筑
		rows, err = s.db.FetchRows(ctx, "select * from cfg_buildings where `inner`=0 order by bid")
	}
	if err != nil {
		return nil, err
	}

	haskaogongji, err := s.isUsingKaoGongJi(ctx, uid, cid, 1)
	if err != nil {
		return nil, err
	}
	cityType, err := s.db.FetchCellInt64(ctx, "select type from cities where id=?", cid)
	if err != nil {
		return nil, err
	}

	resRow, err := s.db.FetchOne(ctx, "select * from city_resources where city_id=?", cid)
	if errors.Is(err, sql.ErrNoRows) {
		// 原版怪癖 2：未定义的 lang 键 → 空消息。
		return nil, httpx.BadRequest("no_resource", "")
	}
	if err != nil {
		return nil, err
	}

	out := make([]BuildingCandidate, 0, len(rows))
	for _, r := range rows {
		newBid := model.Int(r, "bid")
		if inner == 1 {
			// legacy: `bid<>20 and (bid in {5,9,17} or 该城尚未建造该 bid)`
			if legacy, ok := legacyBidOf(newBid); ok && legacy == IDBuildingWall {
				continue
			}
			if !multiBuildingAllowed(newBid) {
				built, err := s.db.Exists(ctx,
					"select 1 from buildings where city_id=? and building_id=? limit 1", cid, newBid)
				if err != nil {
					return nil, err
				}
				if built {
					continue
				}
			}
		}
		c, err := s.simpleBuildingInfo(ctx, uid, cid, int(cityType), r, 1, haskaogongji, resRow)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// simpleBuildingInfo 对齐 doGetSimpleBuildingInfo（BuildingFunc.php:113）。
func (s *Service) simpleBuildingInfo(ctx context.Context, uid, cid, cityType int,
	row map[string]any, dstlevel int, haskaogongji bool, resRow map[string]any) (*BuildingCandidate, error) {

	newBid := model.Int(row, "bid")
	c := &BuildingCandidate{
		BID:         newBid,
		Name:        model.Str(row, "name"),
		Description: model.Str(row, "description"), // 新库 cfg_buildings 无此列 → ""
		Level:       dstlevel,
		CanUpgrade:  true,
		Conditions:  []UpgradeCond{},
	}

	maxlevel := maxLevelFor(newBid, cityType)
	if cityType == 0 && isResourceField(newBid) { // 活动开放普通城池资源田等级上限至 12
		maxlevel = 12
	}
	if cityType == 0 && isMonarchBonusBid(newBid) { // 君主将修为等级（新库无等级表 → 取 0）
		if ok, err := s.checkHeroLevel(ctx, uid, 4, 30); err == nil && ok {
			maxlevel = 15
		}
	}

	rate := 1.0
	if haskaogongji {
		rate = 0.7
	}

	var resNeed map[string]any
	if dstlevel <= maxlevel {
		m, err := s.db.FetchOne(ctx, "select * from cfg_building_levels where bid=? and level=?", newBid, dstlevel)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		resNeed = m
	}
	if resNeed != nil {
		speedRate, err := s.buildingSpeedRate(ctx, uid, cid)
		if err != nil {
			return nil, err
		}
		c.WoodNeed = int64(float64(model.Int64(resNeed, "upgrade_wood")) * rate)
		c.RockNeed = int64(float64(model.Int64(resNeed, "upgrade_rock")) * rate)
		c.IronNeed = int64(float64(model.Int64(resNeed, "upgrade_iron")) * rate)
		c.FoodNeed = int64(float64(model.Int64(resNeed, "upgrade_food")) * rate)
		c.GoldNeed = int64(float64(model.Int64(resNeed, "upgrade_gold")) * rate)
		c.PeopleNeed = model.Int64(resNeed, "upgrade_people")
		// 原版怪癖 1：未除以 GAME_SPEED_RATE。
		c.UpgradeTime = int64(math.Ceil(float64(model.Int64(resNeed, "upgrade_time")) * speedRate))
		c.LevelDescription = model.Str(resNeed, "description")
	} else if dstlevel > 0 {
		c.CanUpgrade = false
	}

	// 判断资源够不够（对齐原版 5 资源比较，不含人口）
	if model.Int64(resRow, "wood") < c.WoodNeed ||
		model.Int64(resRow, "rock") < c.RockNeed ||
		model.Int64(resRow, "iron") < c.IronNeed ||
		model.Int64(resRow, "food") < c.FoodNeed ||
		model.Int64(resRow, "gold") < c.GoldNeed {
		c.CanUpgrade = false
	}

	conds, err := s.buildingConditions(ctx, uid, cid, cityType, newBid, dstlevel)
	if err != nil {
		return nil, err
	}
	for _, cd := range conds {
		if !cd.CanUpgrade {
			c.CanUpgrade = false
		}
		c.Conditions = append(c.Conditions, cd)
	}
	return c, nil
}

// buildingConditions 对齐 doGetSimpleBuildingInfo 的前置条件段（BuildingFunc.php:176-235）。
// 只读版：不扣减物品（startUpgradeBuilding 的 checkConditions 才扣减）。
func (s *Service) buildingConditions(ctx context.Context, uid, cid, cityType, dstbid, dstlevel int) ([]UpgradeCond, error) {
	rows, err := s.db.FetchRows(ctx,
		"select * from cfg_building_conditions where bid=? and levelid=? order by pre_type", dstbid, dstlevel)
	if err != nil {
		return nil, err
	}
	if cityType == 5 && dstlevel >= 11 { // 玩家主城建筑 11 级以上需官府同等级
		if govNew, ok := newBidOf(IDBuildingGoverment); !ok || dstbid != govNew {
			rows = append(rows, map[string]any{
				"pre_type": int64(0), "pre_id": int64(IDBuildingGoverment), "pre_level": int64(dstlevel),
			})
		}
	}

	out := make([]UpgradeCond, 0, len(rows))
	for _, cond := range rows {
		cd := UpgradeCond{CanUpgrade: true}
		switch model.Int(cond, "pre_type") {
		case 0: // 建筑
			cd.Type = "前提建筑"
			preNew, ok := newBidOf(model.Int(cond, "pre_id"))
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
			cd.UpgradeNeed = fmt.Sprintf("%s(等级%d)", name, preLevel)
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
			cd.UpgradeNeed = fmt.Sprintf("%s(等级%d)", name, preLevel)
			cd.CurrentOwn = "等级" + strconv.FormatInt(curr, 10)
			if curr < int64(preLevel) {
				cd.CanUpgrade = false
			}
		case 2: // 物品
			cd.Type = "需要物品"
			preID := model.Int(cond, "pre_id")
			cnt, err := s.goodsCount(ctx, uid, preID)
			if err != nil {
				return nil, err
			}
			name, _ := s.db.FetchCellString(ctx, "select name from cfg_goods where gid=?", preID)
			preLevel := model.Int(cond, "pre_level")
			cd.UpgradeNeed = fmt.Sprintf("%s(数量%d)", name, preLevel)
			cd.CurrentOwn = "数量" + strconv.FormatInt(cnt, 10)
			if cnt < int64(preLevel) {
				cd.CanUpgrade = false
			}
		}
		out = append(out, cd)
	}
	return out, nil
}
