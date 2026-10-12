//go:build integration

package economy

// economy_r5_integration_test.go R5 经济缺陷回归测试（真库）：
//   缺陷1 getUserBuyList 有挂单时 join users 取名称列（u.name 不存在 → u.nickname）；
//   缺陷2 buyGoods 传 gid 时 select * 单列 Scan 列数不匹配（改为 select id）。

import (
	"context"
	"fmt"
	"testing"

	"rxsg/backend/internal/game"
)

// 缺陷1：有挂单时 buylist 不再 500，且 sellername 为卖家昵称（users.nickname），sellName 过滤生效。
func TestUserBuyListWithTrades(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	_, f2 := newSvc(t)
	addBuilding(t, f2, game.BidMarket, "a4", 5)

	nick, err := f2.DB.FetchCellString(ctx, "select nickname from users where id=?", f2.UID)
	if err != nil {
		t.Fatal(err)
	}
	// f2 城 state=0 挂单；distance 取大值以通过距离过滤。
	id, err := f2.DB.Insert(ctx,
		"insert into city_trades (cid,state,restype,`count`,price,gold,distance,unionid,limittime,endtime) "+
			"values (?,0,0,100,0.1,10,1000000000000,0,0,0)", f2.CID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f2.DB.Exec(ctx, "delete from city_trades where id=?", id) })

	// 有数据分支：修复前此处 union 报 Unknown column 'u.name' → 500。
	out, err := s.GetUserBuyList(ctx, f.UID, f.CID, 0, 0, 0, "")
	if err != nil {
		t.Fatalf("buylist: %v", err)
	}
	list := out.([]any)
	rows, ok := list[4].([]map[string]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows=%v want 1", list[4])
	}
	if got := fmt.Sprint(rows[0]["sellername"]); got != nick {
		t.Fatalf("sellername=%q want %q", got, nick)
	}
	if asInt(rows[0]["id"]) != id {
		t.Fatalf("row id=%v want %d", rows[0]["id"], id)
	}

	// sellName 分支（where 用 u.nickname=?）命中。
	out, err = s.GetUserBuyList(ctx, f.UID, f.CID, 0, 0, 0, nick)
	if err != nil {
		t.Fatalf("buylist by name: %v", err)
	}
	if rows2 := out.([]any)[4].([]map[string]any); len(rows2) != 1 {
		t.Fatalf("rows by name=%d want 1", len(rows2))
	}
	// 未命中 → 0 行（不再报错）。
	out, err = s.GetUserBuyList(ctx, f.UID, f.CID, 0, 0, 0, "无此人_NoSuchName")
	if err != nil {
		t.Fatalf("buylist bogus name: %v", err)
	}
	if rows3 := out.([]any)[4].([]map[string]any); len(rows3) != 0 {
		t.Fatalf("rows bogus=%d want 0", len(rows3))
	}
}

// 缺陷2：buyGoods 传 gid（走 select id 取首列分支）不再 Scan 列数不匹配，购买成功。
func TestBuyGoodsByGid(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	exec(t, f, "update users set money=1000 where id=?", f.UID)

	// cfg_shops 种子：gid=12 → id=2（price=5 pack=1）。cnt=2 → 扣 10 元宝、得 2 件 gid12。
	ret, err := s.BuyGoods(ctx, f.UID, 0, 2, 0, 12)
	if err != nil {
		t.Fatalf("buy by gid: %v", err)
	}
	if asInt(ret[0]) != 0 || asInt(ret[1]) != 990 {
		t.Fatalf("ret=%v want [0,990]", ret)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=12", f.UID); v != 2 {
		t.Fatalf("gid12=%d want 2", v)
	}
	if v := cell(t, f, "select money from users where id=?", f.UID); v != 990 {
		t.Fatalf("money=%d want 990", v)
	}
}

// TestGetGoodsHeroAttrDataDriven：cfg_hero 补建（0021）后 GetGoodsHeroAttr 恢复 legacy 数据驱动语义：
// 命中返回 [该行]（select * from cfg_hero where id=?），空行 → 原文案「没有属性提示」。
func TestGetGoodsHeroAttrDataDriven(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()

	out, err := s.GetGoodsHeroAttr(ctx, 1)
	if err != nil {
		t.Fatalf("hero attr: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len=%d want 1", len(out))
	}
	row, ok := out[0].(map[string]any)
	if !ok {
		t.Fatalf("row type %T", out[0])
	}
	if got := fmt.Sprint(row["name"]); got != "赵云" {
		t.Fatalf("name=%q want 赵云", got)
	}
	// 四维/等级口径对齐 0004 演示武将（0021 合成种子）。
	if fmt.Sprint(row["level"]) != "10" || fmt.Sprint(row["command_base"]) != "80" ||
		fmt.Sprint(row["affairs_base"]) != "40" || fmt.Sprint(row["bravery_base"]) != "90" ||
		fmt.Sprint(row["wisdom_base"]) != "60" || fmt.Sprint(row["loyalty"]) != "100" {
		t.Fatalf("hero row=%v", row)
	}

	if _, err := s.GetGoodsHeroAttr(ctx, 999999); errMsg(err) != "没有属性提示" {
		t.Fatalf("missing err=%q", errMsg(err))
	}
}

// TestGetGoodsArmorAttrRow：装备属性提示命中 cfg_armor（id=10001 铁剑，对齐 ShopFunc.php:36）。
func TestGetGoodsArmorAttrRow(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()

	out, err := s.GetGoodsArmorAttr(ctx, 10001)
	if err != nil {
		t.Fatalf("armor attr: %v", err)
	}
	row, ok := out[0].(map[string]any)
	if !ok || fmt.Sprint(row["name"]) != "铁剑" {
		t.Fatalf("armor=%v want [铁剑行]", out)
	}
	if _, err := s.GetGoodsArmorAttr(ctx, 999999); errMsg(err) != "没有属性提示" {
		t.Fatalf("missing err=%q", errMsg(err))
	}
}
