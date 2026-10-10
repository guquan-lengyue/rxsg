//go:build integration

package building

import (
	"context"
	"math"
	"testing"
)

// valid_integration_test.go —— R11-2 项①：1:1 复刻 getAllValidBuilding（BuildingFunc.php:315）。
//
// 覆盖：
//   - inner==0（城外资源田）候选数/成本/耗时（含 upgradeTime 的“未除 GAME_SPEED_RATE”原版怪癖）；
//   - 前置条件不满足的候选（canUpgrade=false + 原文案 前提建筑/等级/数量）；
//   - inner==1（城内）唯一性排除（已建的唯一建筑不再出现在候选中；多座建筑仍出现）；
//   - inner==2（城墙）：新库无城墙建筑 → 空列表。
//
// bid 空间：新库重写映射（1官府 2农田 3伐木 4采石 5铁矿 6民居 7书院 8兵营 9仓库 10校场 11官署
// 12客栈 13市场 14工匠作坊），见 legacy_bid.go。

func hasBID(list []BuildingCandidate, bid int) *BuildingCandidate {
	for i := range list {
		if list[i].BID == bid {
			return &list[i]
		}
	}
	return nil
}

func TestValidBuildingsOuter(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()

	// 注入一条前置条件：铁矿(new bid=5) 1 级需 legacy 官府(6→new 1) 1 级，而本城无官府 → 不可建。
	exec(t, f, "insert into cfg_building_conditions (bid,levelid,pre_type,pre_id,pre_level) values (5,1,0,6,1)")
	t.Cleanup(func() {
		_, _ = f.DB.Exec(context.Background(),
			"delete from cfg_building_conditions where bid=5 and levelid=1")
	})

	list, err := svc.ValidBuildings(ctx, f.UID, f.CID, 0)
	if err != nil {
		t.Fatalf("ValidBuildings(0): %v", err)
	}
	// inner==0：cfg_buildings 只有 2/3/4/5 四块资源田
	if len(list) != 4 {
		t.Fatalf("outer candidates want 4, got %d", len(list))
	}

	// 农田(new 2) 1 级成本（rate=1，无考工记）：木300 石200 铁150 粮50 金0 人口0
	field := hasBID(list, 2)
	if field == nil {
		t.Fatal("缺 bid=2 农田候选")
	}
	if field.Name != "农田" || field.Level != 1 {
		t.Fatalf("bid2 name/level=%q/%d", field.Name, field.Level)
	}
	if field.WoodNeed != 300 || field.RockNeed != 200 || field.IronNeed != 150 ||
		field.FoodNeed != 50 || field.GoldNeed != 0 || field.PeopleNeed != 0 {
		t.Fatalf("bid2 成本不符: wood=%d rock=%d iron=%d food=%d gold=%d people=%d",
			field.WoodNeed, field.RockNeed, field.IronNeed, field.FoodNeed, field.GoldNeed, field.PeopleNeed)
	}
	// 原版怪癖：upgradeTime = ceil(upgrade_time × speedRate)，未除 GAME_SPEED_RATE
	rate, err := svc.buildingSpeedRate(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("buildingSpeedRate: %v", err)
	}
	wantTime := int64(math.Ceil(60 * rate))
	if field.UpgradeTime != wantTime {
		t.Fatalf("bid2 upgradeTime=%d want %d (ceil(60*%v))", field.UpgradeTime, wantTime, rate)
	}
	if !field.CanUpgrade {
		t.Fatal("bid2 应可建")
	}

	// 至少 2 个可建候选
	okCount := 0
	for _, c := range list {
		if c.CanUpgrade {
			okCount++
		}
	}
	if okCount < 2 {
		t.Fatalf("可建候选 want >=2, got %d", okCount)
	}

	// 铁矿(new 5) 因前置（无官府）不可建：断言原文案
	iron := hasBID(list, 5)
	if iron == nil {
		t.Fatal("缺 bid=5 铁矿候选")
	}
	if iron.CanUpgrade {
		t.Fatal("bid5 应因前置条件不可建")
	}
	if len(iron.Conditions) != 1 {
		t.Fatalf("bid5 conditions want 1, got %d", len(iron.Conditions))
	}
	cond := iron.Conditions[0]
	if cond.Type != "前提建筑" || cond.UpgradeNeed != "官府(等级1)" || cond.CurrentOwn != "等级0" || cond.CanUpgrade {
		t.Fatalf("bid5 条件文案不符: %#v", cond)
	}
}

func TestValidBuildingsInnerUniqueness(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()

	// 书院(new 7) 为唯一建筑（不可多座）；已建后应从候选中排除。
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,7,'a1',1,0,0,0)", f.CID)

	list, err := svc.ValidBuildings(ctx, f.UID, f.CID, 1)
	if err != nil {
		t.Fatalf("ValidBuildings(1): %v", err)
	}
	if hasBID(list, 7) != nil {
		t.Fatal("已建的书院(7) 不应作为候选")
	}
	// 多座建筑（民居6/兵营8/仓库9）始终出现
	for _, bid := range []int{6, 8, 9} {
		if hasBID(list, bid) == nil {
			t.Fatalf("多座建筑 bid=%d 应始终在候选内", bid)
		}
	}
}

func TestValidBuildingsWallEmpty(t *testing.T) {
	svc, f := newSvc(t)
	list, err := svc.ValidBuildings(bgCtx(), f.UID, f.CID, 2)
	if err != nil {
		t.Fatalf("ValidBuildings(2): %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("新库无城墙建筑 → 城墙候选应为空, got %d", len(list))
	}
}
