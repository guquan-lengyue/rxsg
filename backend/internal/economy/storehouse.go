package economy

// storehouse.go 1:1 复刻 server/game/StoreFunc.php（仓库/打包）。
// bid 映射：legacy 农田1/伐木2/采石3/铁矿4→新 2/3/4/5；legacy 仓库17→新 9。
// 原版怪癖保留：payToPack 铜钱不足文案 "您的元宝数量不够"（not_enough_moeny 实指铜钱）。

import (
	"context"
	"database/sql"
	"errors"
	"math"

	"rxsg/backend/internal/game"
)

// doGetStoreInfo 对齐 StoreFunc.php:5。
// 返回 [foodbase,woodbase,rockbase,ironbase,storageTech,仓库数,仓库容量基,存放比例行,铜钱数]。
func (s *Service) DoGetStoreInfo(ctx context.Context, uid, cid int) ([]any, error) {
	baseOf := func(bid int) (float64, error) {
		return s.cellFloat(ctx, "select sum(level*(level+1)*50) from buildings where city_id=? and building_id=?", cid, bid)
	}
	foodBase, err := baseOf(game.BidFarmland) // legacy bid1
	if err != nil {
		return nil, err
	}
	woodBase, err := baseOf(game.BidWood) // legacy bid2
	if err != nil {
		return nil, err
	}
	rockBase, err := baseOf(game.BidRock) // legacy bid3
	if err != nil {
		return nil, err
	}
	ironBase, err := baseOf(game.BidIron) // legacy bid4
	if err != nil {
		return nil, err
	}
	spd := float64(game.SpeedRate)
	foodBase *= 100 * spd
	woodBase *= 100 * spd
	rockBase *= 100 * spd
	ironBase *= 100 * spd

	storageTech, err := s.cellFloat(ctx, "select level from city_technics where city_id=? and technic_id=15", cid)
	if err != nil {
		return nil, err
	}
	if storageTech == 0 { // legacy empty()
		storageTech = 1
	} else if storageTech > 0 {
		storageTech = 1.0 + 0.1*storageTech
	}
	storeCnt, err := s.cellInt(ctx, "select count(*) from buildings where city_id=? and building_id=?", cid, game.BidStore)
	if err != nil {
		return nil, err
	}
	storeSum, err := s.cellFloat(ctx, "select sum(level*(level+1)*5000) from buildings where city_id=? and building_id=?", cid, game.BidStore)
	if err != nil {
		return nil, err
	}
	rateRow, err := s.db.FetchOne(ctx,
		"select food_store,wood_store,rock_store,iron_store from city_res_add where city_id=?", cid)
	if errors.Is(err, sql.ErrNoRows) {
		rateRow = map[string]any{}
	} else if err != nil {
		return nil, err
	}
	copper, err := s.goodsCount(ctx, uid, game.GidCopper)
	if err != nil {
		return nil, err
	}
	return []any{foodBase, woodBase, rockBase, ironBase, storageTech,
		storeCnt, spd * storeSum, rateRow, copper}, nil
}

// ModifyStoreRate 对齐 StoreFunc.php:39（成功也经 throw 返回文案）。
func (s *Service) ModifyStoreRate(ctx context.Context, cid int, foodRate, woodRate, rockRate, ironRate int64) error {
	if foodRate < 0 || woodRate < 0 || rockRate < 0 || ironRate < 0 {
		return errLegacy("存放比例不能为负数！")
	} else if foodRate+woodRate+rockRate+ironRate > 100 {
		return errLegacy("四项资源存放比例之和不能超过100。")
	}
	if _, err := s.db.Exec(ctx,
		"update city_res_add set food_store=?,wood_store=?,rock_store=?,iron_store=?,resource_changing=1 where city_id=?",
		foodRate, woodRate, rockRate, ironRate, cid); err != nil {
		return err
	}
	return errLegacy("修改仓库存放比例成功！") // succ_change_rate
}

// PackItem 对齐 payToPack 的 $param 元素：[type, packCount, resCount, copper]。
type PackItem struct {
	Type      string  `json:"type"`
	PackCount float64 `json:"pack_count"`
	ResCount  float64 `json:"res_count"`
	Copper    float64 `json:"copper"`
}

// payToPackTypeToGid 对齐 StoreFunc.php:74。
var payToPackTypeToGid = map[string]int{
	"gold1": 86, "gold2": 85, "food1": 91, "food2": 87, "iron1": 94,
	"iron2": 90, "rock1": 93, "rock2": 89, "wood1": 92, "wood2": 88,
}

// payToPackTypeToCol 对齐 StoreFunc.php:76。
var payToPackTypeToCol = map[string]string{
	"gold1": "gold", "gold2": "gold", "food1": "food", "food2": "food", "iron1": "iron",
	"iron2": "iron", "rock1": "rock", "rock2": "rock", "wood1": "wood", "wood2": "wood",
}

// PayToPack 对齐 StoreFunc.php:68。
// 原版怪癖：铜钱不足文案为 "您的元宝数量不够"（not_enough_moeny），1:1 保留。
func (s *Service) PayToPack(ctx context.Context, uid, cid int, items []PackItem) ([]int, error) {
	if uid == 0 || cid == 0 || len(items) == 0 { // legacy empty() 直接 return（无值）
		return nil, nil
	}
	totalCopper := 0.0
	for _, it := range items {
		col, ok := payToPackTypeToCol[it.Type]
		if !ok {
			continue // 非法 type → $type=null → select  报错，原版未防护；Go 跳过（无对应列）
		}
		resource, err := s.cellFloat(ctx, "select "+col+" from city_resources where city_id=?", cid)
		if err != nil {
			return nil, err
		}
		if resource < it.ResCount {
			return nil, errLegacy("您的资源数目不足") // not_enough_res
		}
		totalCopper += it.Copper
	}
	totalCopper = math.Ceil(totalCopper)
	curCopper, err := s.goodsCount(ctx, uid, game.GidCopper)
	if err != nil {
		return nil, err
	}
	if float64(curCopper) < totalCopper {
		return nil, errLegacy("您的元宝数量不够") // not_enough_moeny（原文案，实指铜钱）
	}
	for _, it := range items {
		col, ok := payToPackTypeToCol[it.Type]
		if !ok {
			continue
		}
		_ = col
		// 扣钱（addGoods 152 负数 → reduceGoods 语义：cnt<0 时 addGoods 原样 upsert 负增量）。
		if err := s.addGoods(ctx, uid, game.GidCopper, -int64(math.Ceil(it.Copper)), 2); err != nil {
			return nil, err
		}
		gid := payToPackTypeToGid[it.Type]
		if err := s.addGoods(ctx, uid, gid, int64(math.Floor(it.PackCount)), 2); err != nil {
			return nil, err
		}
		switch col {
		case "wood":
			err = s.addCityResources(ctx, cid, int64(-it.ResCount), 0, 0, 0, 0)
		case "rock":
			err = s.addCityResources(ctx, cid, 0, int64(-it.ResCount), 0, 0, 0)
		case "iron":
			err = s.addCityResources(ctx, cid, 0, 0, int64(-it.ResCount), 0, 0)
		case "food":
			err = s.addCityResources(ctx, cid, 0, 0, 0, int64(-it.ResCount), 0)
		case "gold":
			err = s.addCityResources(ctx, cid, 0, 0, 0, 0, int64(-it.ResCount))
		}
		if err != nil {
			return nil, err
		}
	}
	return []int{0, 0}, nil
}
