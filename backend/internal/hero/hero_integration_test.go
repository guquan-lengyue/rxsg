//go:build integration

package hero

import (
	"context"
	"strings"
	"testing"

	"rxsg/backend/internal/model"
	"rxsg/backend/internal/testutil"
)

// M3 武将真库集成测试：与 HeroFunc.php / HeroExpr.php / OfficeFunc.php 1:1 对照。
// 基线（testutil.SetupCity）：gold=10000、money=0；
//   hid1: level10 type1 command80 affairs40 bravery90 wisdom60（城守+主将位）
//   hid2: level10 type2 command60 affairs95 bravery30 wisdom100
// cfg_hero_levels: upgrade(L)=(L-1)^2*100 → total(10)=28500、upgrade(11)=10000。
// cfg_hero_expr_types: type1 修身 min1 max24 hour_money100 hour_gold1。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

func cell(t *testing.T, f *testutil.Fixture, query string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("query %q: %v", query, err)
	}
	return v
}

func setHero(t *testing.T, f *testutil.Fixture, hid int, level int64, exp int64, state int64, npcID int64) {
	t.Helper()
	_, err := f.DB.Exec(context.Background(),
		"update heroes set level=?, exp=?, state=?, npc_id=? where id=?", level, exp, state, npcID, hid)
	if err != nil {
		t.Fatalf("set hero: %v", err)
	}
}

// TestUpgradeFlow 升级经验阈值、俸禄重算、124 超上限夹平、124→125 突破。
func TestUpgradeFlow(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 经验差 1 → 拒绝
	setHero(t, f, f.HID1, 10, 28500+9999, 0, 0)
	if _, err := s.Upgrade(ctx, f.UID, f.CID, f.HID1); err == nil ||
		!strings.Contains(err.Error(), msgNoEnoughExp) {
		t.Fatalf("经验不足应拒绝, got %v", err)
	}

	// 恰好达标 → 10→11
	setHero(t, f, f.HID1, 10, 28500+10000, 0, 0)
	if _, err := s.Upgrade(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if lv := cell(t, f, "select level from heroes where id=?", f.HID1); lv != 11 {
		t.Fatalf("level=%d want 11", lv)
	}
	// hero_fee = (11*20) + (10*20+(5+0+10)*50) = 220+950 = 1170
	if fee := cell(t, f, "select hero_fee from city_resources where city_id=?", f.CID); fee != 1170 {
		t.Fatalf("hero_fee=%d want 1170", fee)
	}

	// 不在城（state=4）→ 拒绝
	setHero(t, f, f.HID1, 11, 0, 4, 0)
	if _, err := s.Upgrade(ctx, f.UID, f.CID, f.HID1); err == nil ||
		!strings.Contains(err.Error(), msgCantUpgradeOut) {
		t.Fatalf("出征将应拒绝, got %v", err)
	}
	setHero(t, f, f.HID1, 11, 0, 0, 0)

	// npc_id=0 → 上限 120；level=124 → 夹平到 120 并报顶级（原版怪癖）
	setHero(t, f, f.HID1, 124, 0, 0, 0)
	if _, err := s.Upgrade(ctx, f.UID, f.CID, f.HID1); err == nil ||
		!strings.Contains(err.Error(), msgLevelMax) {
		t.Fatalf("超上限应报顶级, got %v", err)
	}
	if lv := cell(t, f, "select level from heroes where id=?", f.HID1); lv != 120 {
		t.Fatalf("夹平后 level=%d want 120", lv)
	}

	// npc_id=36（bighidsForMaxLevel）→ 上限 125；124 级无条件突破 → level=125、四维+5%、攻防 add_on=125
	total124 := cell(t, f, "select total_exp from cfg_hero_levels where level=124")
	setHero(t, f, f.HID1, 124, total124+124*124*100, 0, 36)
	if _, err := s.Upgrade(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatalf("124 突破: %v", err)
	}
	row, err := f.DB.FetchOne(ctx, "select * from heroes where id=?", f.HID1)
	if err != nil {
		t.Fatal(err)
	}
	if v := model.Int(row, "level"); v != 125 {
		t.Fatalf("突破后 level=%d want 125", v)
	}
	// bravery 90→floor(4.5)=4→94；wisdom 60→3→63；affairs 40→2→42；command 80→4→84
	for col, want := range map[string]int{
		"bravery_base": 94, "wisdom_base": 63, "affairs_base": 42,
		"command_base": 84, "attack_add_on": 125, "defence_add_on": 125,
	} {
		if got := model.Int(row, col); got != want {
			t.Fatalf("%s=%d want %d", col, got, want)
		}
	}
	if n := cell(t, f, "select count(1) from hero_base_add where hero_id=?", f.HID1); n != 1 {
		t.Fatalf("hero_base_add=%d want 1", n)
	}
}

// TestAddPoint 加点上限（levelAdd=level）、潜力不足、洗点回退（xidian 外挂判定）。
func TestAddPoint(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// hid1: base 和=190、level10 → 上限 200。目标 191 ≤ 200 → 允许
	if _, err := s.AddPoint(ctx, f.UID, f.CID, f.HID1, 41, 90, 60); err != nil {
		t.Fatalf("AddPoint: %v", err)
	}
	if v := cell(t, f, "select affairs_add from heroes where id=?", f.HID1); v != 1 {
		t.Fatalf("affairs_add=%d want 1", v)
	}
	// chiefhid==hid1 → resource_changing=1
	if v := cell(t, f, "select resource_changing from city_res_add where city_id=?", f.CID); v != 1 {
		t.Fatalf("resource_changing=%d want 1", v)
	}

	// 目标 201 > 200 → 潜力不足
	if _, err := s.AddPoint(ctx, f.UID, f.CID, f.HID1, 45, 95, 61); err == nil ||
		!strings.Contains(err.Error(), msgNoExtraPotential) {
		t.Fatalf("超潜力应拒绝, got %v", err)
	}
	// affairs 回退 41→40（新 add 0 < 旧 add 1）→ 外挂文案
	if _, err := s.AddPoint(ctx, f.UID, f.CID, f.HID1, 40, 90, 60); err == nil ||
		!strings.Contains(err.Error(), msgXidianUnvalid) {
		t.Fatalf("回退加点应判外挂, got %v", err)
	}
	// 出征将不可加点
	setHero(t, f, f.HID2, 10, 0, 4, 0)
	if _, err := s.AddPoint(ctx, f.UID, f.CID, f.HID2, 96, 30, 100); err == nil ||
		!strings.Contains(err.Error(), msgCantAddOut) {
		t.Fatalf("出征将应拒绝, got %v", err)
	}
	setHero(t, f, f.HID2, 10, 0, 0, 0)
}

// TestClearPoint 洗点消耗洗髓丹 ceil(level/10)。
func TestClearPoint(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := s.AddPoint(ctx, f.UID, f.CID, f.HID2, 95, 31, 100); err != nil {
		t.Fatalf("AddPoint hid2: %v", err)
	}
	// 无洗髓丹 → not_enough_goods22#1
	if _, err := s.ClearPoint(ctx, f.UID, f.CID, f.HID2); err == nil ||
		!strings.Contains(err.Error(), "not_enough_goods22#1") {
		t.Fatalf("缺丹应拒绝, got %v", err)
	}
	if err := s.addGoods(ctx, f.UID, 22, 3, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClearPoint(ctx, f.UID, f.CID, f.HID2); err != nil {
		t.Fatalf("ClearPoint: %v", err)
	}
	if v := cell(t, f, "select bravery_add from heroes where id=?", f.HID2); v != 0 {
		t.Fatalf("bravery_add=%d want 0", v)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=22", f.UID); v != 2 {
		t.Fatalf("洗髓丹=%d want 2", v)
	}
	// 俸禄回到基线（加点已清）：hid1 200 + hid2 950 = 1150
	if fee := cell(t, f, "select hero_fee from city_resources where city_id=?", f.CID); fee != 1150 {
		t.Fatalf("hero_fee=%d want 1150", fee)
	}
}

// TestStartExprChain 历练开始校验链：时长、状态、君主将、元宝、扣金、toomany、满5。
func TestStartExprChain(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// hours<1 → 数据异常
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 0, 0); err == nil ||
		!strings.Contains(err.Error(), msgWaiguaInvalid) {
		t.Fatalf("hours=0 应拒绝, got %v", err)
	}
	// 君主将 → 不能历练
	if _, err := f.DB.Exec(ctx, `insert into heroes (user_id, city_id, name, state, level, hero_type)
		values (?,?,?,0,10,1000)`, f.UID, f.CID, "君主"); err != nil {
		t.Fatal(err)
	}
	mID := cell(t, f, "select id from heroes where city_id=? and hero_type=1000", f.CID)
	if _, err := s.StartExpr(ctx, f.UID, f.CID, int(mID), 1, 10, 0); err == nil ||
		!strings.Contains(err.Error(), msgCannotExpr) {
		t.Fatalf("君主将历练应拒绝, got %v", err)
	}
	// 非空闲 → 拒绝
	setHero(t, f, f.HID1, 10, 0, 1, 0)
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 0); err == nil ||
		!strings.Contains(err.Error(), msgHeroNotKong) {
		t.Fatalf("非空闲应拒绝, got %v", err)
	}
	setHero(t, f, f.HID1, 10, 0, 0, 0)

	// 元宝不足：need_money=10*100=1000
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 0); err == nil ||
		!strings.Contains(err.Error(), msgExprNotEnoughMoney) {
		t.Fatalf("元宝不足应拒绝, got %v", err)
	}

	// 成功：money=5000、随带 500 → 扣 1500；need_gold=10*10*1=100
	if _, err := f.DB.Exec(ctx, "update users set money=5000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 500); err != nil {
		t.Fatalf("StartExpr: %v", err)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 3500 {
		t.Fatalf("money=%d want 3500", m)
	}
	if g := cell(t, f, "select gold from city_resources where city_id=?", f.CID); g != 9900 {
		t.Fatalf("gold=%d want 9900", g)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID1); st != StateExpring {
		t.Fatalf("hero state=%d want 10", st)
	}
	if n := cell(t, f, `select count(1) from hero_exprs where city_id=? and state=0 and hours=10 and carrymoney=500`, f.CID); n != 1 {
		t.Fatalf("hero_exprs=%d want 1", n)
	}
	if typ := cell(t, f, "select `type` from log_money where user_id=? order by id desc limit 1", f.UID); typ != 120 {
		t.Fatalf("log_money type=%d want 120", typ)
	}

	// 第 2 名 → 成功；第 3 名 → toomany（maxHeroCount=2，非错误）。
	// 用不存在的 hid（原版怪癖：查无武将按 null 继续，state null!=0 为 false 可进入计数判定）。
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID2, 1, 5, 0); err != nil {
		t.Fatalf("StartExpr hid2: %v", err)
	}
	info, err := s.StartExpr(ctx, f.UID, f.CID, 888888, 1, 5, 0)
	if err != nil {
		t.Fatalf("toomany 应非错误, got %v", err)
	}
	if info.Toomany != msgToomanyHeroExpr {
		t.Fatalf("toomany=%q want 巡查令提示", info.Toomany)
	}

	// 补到 5 行 → 第 6 次 throw
	for i := 0; i < 3; i++ {
		if _, err := f.DB.Exec(ctx, `insert into hero_exprs
			(user_id, city_id, hero_id, expr_type, hours, state, started_at, end_at, carrymoney, acc_times)
			values (?,?,?,?,1,0,unix_timestamp(),unix_timestamp()+3600,0,0)`,
			f.UID, f.CID, 900000+i, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, 888888, 1, 5, 0); err == nil ||
		!strings.Contains(err.Error(), msgExprCountMax) {
		t.Fatalf("满5应拒绝, got %v", err)
	}

	// 原版怪癖：不存在的 hid 照样开始（state null!=0 为 false、heroLevel null→0）
	if _, err := f.DB.Exec(ctx, "delete from hero_exprs where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, 999999, 1, 5, 0); err != nil {
		t.Fatalf("幽灵 hid 应成功（原版怪癖）, got %v", err)
	}
	if n := cell(t, f, "select count(1) from hero_exprs where hero_id=999999"); n != 1 {
		t.Fatalf("幽灵历练行=%d want 1", n)
	}
}

// TestCancelAndSettle 取消历练 endtime 规则 + 到期结算退元宝/经验。
func TestCancelAndSettle(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "update users set money=5000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 700); err != nil {
		t.Fatalf("StartExpr: %v", err)
	}
	oldEnd := cell(t, f, "select end_at from hero_exprs where hero_id=?", f.HID1)

	if _, err := s.CancelExpr(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatalf("CancelExpr: %v", err)
	}
	newEnd := cell(t, f, "select end_at from hero_exprs where hero_id=?", f.HID1)
	// 刚取消（elapsed < hours*1800）→ endtime=2*now-start，远小于原 endtime
	if newEnd >= oldEnd-600 {
		t.Fatalf("取消应缩短 endtime: old=%d new=%d", oldEnd, newEnd)
	}
	if st := cell(t, f, "select state from hero_exprs where hero_id=?", f.HID1); st != 1 {
		t.Fatalf("expr state=%d want 1", st)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID1); st != StateExprDone {
		t.Fatalf("hero state=%d want 11", st)
	}

	// 到期：结算退 carrymoney、exp=10*randRange(10,20)∈[100,200]
	if _, err := f.DB.Exec(ctx, "update hero_exprs set end_at=unix_timestamp()-10 where hero_id=?", f.HID1); err != nil {
		t.Fatal(err)
	}
	expBefore := cell(t, f, "select exp from heroes where id=?", f.HID1)
	if _, err := s.Info(ctx, f.UID, f.CID); err != nil {
		t.Fatalf("Info(结算): %v", err)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 5000-1700+700 {
		t.Fatalf("money=%d want %d", m, 5000-1700+700)
	}
	delta := cell(t, f, "select exp from heroes where id=?", f.HID1) - expBefore
	if delta < 100 || delta > 200 || delta%10 != 0 {
		t.Fatalf("取消结算经验=%d want [100,200]且为10的倍数", delta)
	}
	if n := cell(t, f, "select count(1) from hero_exprs where hero_id=?", f.HID1); n != 0 {
		t.Fatalf("结算后应删除行, %d", n)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID1); st != 0 {
		t.Fatalf("结算后 hero state=%d want 0", st)
	}
}

// TestNormalSettleExp 正常到期：修身 hours*level*6000+rand(1,1000)。
func TestNormalSettleExp(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "update users set money=5000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 0); err != nil {
		t.Fatalf("StartExpr: %v", err)
	}
	if _, err := f.DB.Exec(ctx, "update hero_exprs set end_at=unix_timestamp()-10 where hero_id=?", f.HID1); err != nil {
		t.Fatal(err)
	}
	expBefore := cell(t, f, "select exp from heroes where id=?", f.HID1)
	if _, err := s.Info(ctx, f.UID, f.CID); err != nil {
		t.Fatal(err)
	}
	delta := cell(t, f, "select exp from heroes where id=?", f.HID1) - expBefore
	// 10h × lv10 × 6000 = 600000 + rand(1,1000)
	if delta < 600001 || delta > 601000 {
		t.Fatalf("修身经验=%d want [600001,601000]", delta)
	}
	// 结算不再退/扣元宝（carrymoney=0）
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 5000-1000 {
		t.Fatalf("money=%d want 4000", m)
	}
}

// TestFasterExpr 通关文书(143) 缩时 + 急召令(144) 立即召回。
func TestFasterExpr(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "update users set money=5000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 600); err != nil {
		t.Fatalf("StartExpr: %v", err)
	}

	// 143 通关文书
	if err := s.addGoods(ctx, f.UID, 143, 2, 0); err != nil {
		t.Fatal(err)
	}
	oldEnd := cell(t, f, "select end_at from hero_exprs where hero_id=?", f.HID1)
	if _, err := s.FasterExpr(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatalf("FasterExpr(143): %v", err)
	}
	newEnd := cell(t, f, "select end_at from hero_exprs where hero_id=?", f.HID1)
	reduced := oldEnd - newEnd
	// 36000*0.3=10800（下限 1800 不触发）
	if reduced < 10790 || reduced > 10810 {
		t.Fatalf("缩时=%d want ≈10800", reduced)
	}
	if acc := cell(t, f, "select acc_times from hero_exprs where hero_id=?", f.HID1); acc != 1 {
		t.Fatalf("acc_times=%d want 1", acc)
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=143", f.UID); g != 1 {
		t.Fatalf("143=%d want 1", g)
	}
	// 缺 143 → 拒绝
	if _, err := f.DB.Exec(ctx, "delete from user_goods where user_id=? and gid=143", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FasterExpr(ctx, f.UID, f.CID, f.HID1); err == nil ||
		!strings.Contains(err.Error(), "not_enough_goods143") {
		t.Fatalf("缺143应拒绝, got %v", err)
	}

	// 取消后（state=1）用 144 急召令：退 carrymoney、exp=10*rand(10,20)、删行、state=0
	if _, err := s.CancelExpr(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatal(err)
	}
	if err := s.addGoods(ctx, f.UID, 144, 1, 0); err != nil {
		t.Fatal(err)
	}
	moneyBefore := cell(t, f, "select money from users where id=?", f.UID)
	expBefore := cell(t, f, "select exp from heroes where id=?", f.HID1)
	if _, err := s.FasterExpr(ctx, f.UID, f.CID, f.HID1); err != nil {
		t.Fatalf("FasterExpr(144): %v", err)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != moneyBefore+600 {
		t.Fatalf("急召令退款 money=%d want %d", m, moneyBefore+600)
	}
	delta := cell(t, f, "select exp from heroes where id=?", f.HID1) - expBefore
	if delta < 100 || delta > 200 || delta%10 != 0 {
		t.Fatalf("急召令经验=%d want [100,200]", delta)
	}
	if n := cell(t, f, "select count(1) from hero_exprs where hero_id=?", f.HID1); n != 0 {
		t.Fatalf("急召令应删行, %d", n)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID1); st != 0 {
		t.Fatalf("急召令后 state=%d want 0", st)
	}
}

// TestSetChief 任命城守/主将/军师：两遍循环、状态流转、出征占用、官署缺失。
func TestSetChief(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 官署建筑 bid=11（getOfficeInfo 依赖）
	if _, err := f.DB.Exec(ctx, `insert into buildings (city_id, building_id, xy, level, state)
		values (?,11,'4,2',1,0)`, f.CID); err != nil {
		t.Fatal(err)
	}

	// 任命 hid2 为城守：hid2 state=1、chief 指向、忠诚同步、resource_changing
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID2, StateChief}}); err != nil {
		t.Fatalf("SetChief 城守: %v", err)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID2); st != StateChief {
		t.Fatalf("hid2 state=%d want 1", st)
	}
	if ch := cell(t, f, "select chief_hero_id from cities where id=?", f.CID); ch != int64(f.HID2) {
		t.Fatalf("chief=%d want %d", ch, f.HID2)
	}
	if rc := cell(t, f, "select resource_changing from city_res_add where city_id=?", f.CID); rc != 1 {
		t.Fatalf("resource_changing=%d want 1", rc)
	}

	// 旧城守出征中 → 拒绝改任
	if _, err := f.DB.Exec(ctx, `insert into troops (user_id, city_id, hero_id, state)
		values (?,?,?,0)`, f.UID, f.CID, f.HID2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID1, StateChief}}); err == nil ||
		!strings.Contains(err.Error(), msgSetChiefBusy) {
		t.Fatalf("旧城守出征应拒绝, got %v", err)
	}
	if _, err := f.DB.Exec(ctx, "delete from troops where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}

	// 任命主将（hid1 state=0 可任命）
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID1, StateGeneral}}); err != nil {
		t.Fatalf("SetChief 主将: %v", err)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID1); st != StateGeneral {
		t.Fatalf("hid1 state=%d want 7", st)
	}
	if g := cell(t, f, "select general_hero_id from cities where id=?", f.CID); g != int64(f.HID1) {
		t.Fatalf("general=%d want %d", g, f.HID1)
	}

	// 卸任城守 hid2（oldtype=1）→ 再任军师：两遍循环
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID2, StateCounsellor}}); err != nil {
		t.Fatalf("SetChief 军师: %v", err)
	}
	if st := cell(t, f, "select state from heroes where id=?", f.HID2); st != StateCounsellor {
		t.Fatalf("hid2 state=%d want 8", st)
	}
	if c := cell(t, f, "select counsellor_hero_id from cities where id=?", f.CID); c != int64(f.HID2) {
		t.Fatalf("counsellor=%d want %d", c, f.HID2)
	}
	// 卸任时 chief 已清 0
	if ch := cell(t, f, "select chief_hero_id from cities where id=?", f.CID); ch != 0 {
		t.Fatalf("卸任后 chief=%d want 0", ch)
	}

	// 出征状态武将不可任命（校验遍）
	setHero(t, f, f.HID1, 10, 0, StateOut, 0)
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID1, StateGeneral}}); err == nil ||
		!strings.Contains(err.Error(), msgSetChiefFail) {
		t.Fatalf("出征将任命应拒绝, got %v", err)
	}
}

// TestSetChiefNoOffice 无官署 → getOfficeInfo 抛错（lang 原文 "铁锭"，原版错位保留）。
func TestSetChiefNoOffice(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := s.SetChief(ctx, f.UID, f.CID, [][2]int{{f.HID2, StateChief}}); err == nil ||
		!strings.Contains(err.Error(), msgNoOfficeBuilt) {
		t.Fatalf("无官署应报错, got %v", err)
	}
}

// TestInfoShape 面板聚合：历练行挂载、升级判定文案。
func TestInfoShape(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "update users set money=5000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartExpr(ctx, f.UID, f.CID, f.HID1, 1, 10, 0); err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, h := range info.Heroes {
		if h.HID == f.HID1 {
			found = h.Expr != nil && h.Expr.Hours == 10 && h.Expr.State == 0
			if h.NoUpgradeMsg != msgCantUpgradeOut {
				t.Fatalf("历练中不可升级文案=%q", h.NoUpgradeMsg)
			}
		}
	}
	if !found {
		t.Fatal("hid1 历练任务未挂载")
	}
	if len(info.ExprTypes) != 2 || info.ExprTypes[0].Name != "修身养性" {
		t.Fatalf("exprTypes=%v", info.ExprTypes)
	}
}
