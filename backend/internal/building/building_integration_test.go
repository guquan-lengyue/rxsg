//go:build integration

package building

import (
	"context"
	"testing"

	"rxsg/backend/internal/game"
	"rxsg/backend/internal/testutil"
)

// building_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// R11-1 覆盖：建造新建筑（成功/资源不足/位置占用/唯一性/队列上限）、
// 拆除一级与结算、彻底拆除（火油桶前置 + 原版“仅降一级”怪癖）、取消拆除、
// 资源地转换（前置道具 + resource_changing/changing）、官府拆除保护、队列任务标签。
//
// bid 常量说明：新库为【重写 14 建筑映射】（1官府 2农田 3伐木 4采石 5铁矿 6民居 7书院
// 8兵营 9仓库 10校场 11官署 12客栈 13市场 14工匠作坊），测试以此映射传参；
// legacy 20 bid 的权威定义与差异见 legacy_bid.go 与汇报。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	// testutil 未清理本批次新增的队列表（不改既有测试支撑）→ 本包按 cid 自清理。
	t.Cleanup(func() {
		_, _ = f.DB.Exec(context.Background(), "delete from building_upgrading where cid=?", f.CID)
		_, _ = f.DB.Exec(context.Background(), "delete from building_destroying where cid=?", f.CID)
	})
	return NewService(d), f
}

func bgCtx() context.Context { return context.Background() }

func exec(t *testing.T, f *testutil.Fixture, q string, args ...any) {
	t.Helper()
	if _, err := f.DB.Exec(bgCtx(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func cell(t *testing.T, f *testutil.Fixture, q string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(bgCtx(), q, args...)
	if err != nil {
		t.Fatalf("cell %q: %v", q, err)
	}
	return v
}

func errMsg(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

func grant(t *testing.T, f *testutil.Fixture, gid int, cnt int64) {
	t.Helper()
	exec(t, f, "insert into user_goods (user_id,gid,`count`) values (?,?,?) on duplicate key update `count`=`count`+?",
		f.UID, gid, cnt, cnt)
}

// ── 建造新建筑（成功路径 + 耗时公式）──────────────────────────────────────

func TestBuildCreateSuccess(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()

	// 农田（新 bid=2，inner=0）第 1 级：木300 石200 铁150 粮50 金0 人口0，升级时间 60s
	if _, err := svc.Build(ctx, f.UID, f.CID, 2, 0, 0, 0); err != nil {
		t.Fatalf("Build: %v", err)
	}
	// 资源扣减
	if got := cell(t, f, "select wood from city_resources where city_id=?", f.CID); got != 99700 {
		t.Fatalf("wood want 99700, got %d", got)
	}
	if got := cell(t, f, "select rock from city_resources where city_id=?", f.CID); got != 99800 {
		t.Fatalf("rock want 99800, got %d", got)
	}
	if got := cell(t, f, "select iron from city_resources where city_id=?", f.CID); got != 99850 {
		t.Fatalf("iron want 99850, got %d", got)
	}
	if got := cell(t, f, "select food from city_resources where city_id=?", f.CID); got != 99950 {
		t.Fatalf("food want 99950, got %d", got)
	}
	if got := cell(t, f, "select gold from city_resources where city_id=?", f.CID); got != 10000 {
		t.Fatalf("gold want 10000, got %d", got)
	}
	// buildings：level=0 state=1（建造中）
	if got := cell(t, f, "select count(*) from buildings where city_id=? and xy='a1' and building_id=2 and level=0 and state=1", f.CID); got != 1 {
		t.Fatalf("building row mismatch (want level0 state1): %d", got)
	}
	// mem_building_upgrading：目标等级 1
	if got := cell(t, f, "select level from building_upgrading where cid=? and xy='a1'", f.CID); got != 1 {
		t.Fatalf("building_upgrading.level want 1, got %d", got)
	}
	// 耗时 = ceil(upgrade_time * speedRate / GAME_SPEED_RATE)，逐字对齐 startUpgradeBuilding
	rate, err := svc.buildingSpeedRate(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("buildingSpeedRate: %v", err)
	}
	want := game.ScaledSeconds(60, rate)
	if got := cell(t, f, "select state_end_at - state_start_at from buildings where city_id=? and xy='a1'", f.CID); got != want {
		t.Fatalf("build duration want %d (=ceil(60*%v/10)), got %d", want, rate, got)
	}
}

// ── 资源不足 ───────────────────────────────────────────────────────────────

func TestBuildResourceNotEnough(t *testing.T) {
	svc, f := newSvc(t)
	exec(t, f, "update city_resources set wood=0 where city_id=?", f.CID)
	if _, err := svc.Build(bgCtx(), f.UID, f.CID, 2, 0, 0, 1); errMsg(err) != "资源不足" {
		t.Fatalf("want 资源不足, got %q", errMsg(err))
	}
}

// ── 位置占用 / 正在升级 ────────────────────────────────────────────────────

func TestBuildPositionOccupiedAndUpgrading(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	// 该格是其它建筑（bid=3 伐木场）→ 建造建筑错误
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,3,'a2',1,0,0,0)", f.CID)
	if _, err := svc.Build(ctx, f.UID, f.CID, 2, 0, 0, 1); errMsg(err) != "建造建筑错误" {
		t.Fatalf("want 建造建筑错误, got %q", errMsg(err))
	}
	// 同建筑处于升级中 → 建筑正在升级中
	exec(t, f, "update buildings set state=1 where city_id=? and xy='a2'", f.CID)
	if _, err := svc.Build(ctx, f.UID, f.CID, 3, 0, 0, 1); errMsg(err) != "建筑正在升级中" {
		t.Fatalf("want 建筑正在升级中, got %q", errMsg(err))
	}
}

// ── 唯一建筑重复建造 ───────────────────────────────────────────────────────

func TestBuildDuplicateUnique(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	// 书院（新 bid=7，inner=1）为唯一建筑
	if _, err := svc.Build(ctx, f.UID, f.CID, 7, 1, 2, 0); err != nil {
		t.Fatalf("first Build: %v", err)
	}
	if _, err := svc.Build(ctx, f.UID, f.CID, 7, 1, 3, 0); errMsg(err) != "相同的建筑已经建造。" {
		t.Fatalf("want 相同的建筑已经建造。, got %q", errMsg(err))
	}
}

// ── 建造队列上限（默认 2）──────────────────────────────────────────────────

func TestBuildQueueFull(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	for _, x := range []int{4, 5} {
		if _, err := svc.Build(ctx, f.UID, f.CID, 2, 0, x, 0); err != nil {
			t.Fatalf("Build x=%d: %v", x, err)
		}
	}
	if _, err := svc.Build(ctx, f.UID, f.CID, 2, 0, 6, 0); errMsg(err) != "ask_to_use_yaoyiling" {
		t.Fatalf("want ask_to_use_yaoyiling, got %q", errMsg(err))
	}
}

// ── 拆除一级 + 惰性结算 ────────────────────────────────────────────────────

func TestDestroyOneLevelAndSettle(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	// 农田 20 级（耗时 = floor(929255*0.01*speed/10) > 0 → 保持 state=2）
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'c1',20,0,0,0)", f.CID)
	if _, err := svc.Destroy(ctx, f.UID, f.CID, 2, 0, 2, 0); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if got := cell(t, f, "select state from buildings where city_id=? and xy='c1'", f.CID); got != 2 {
		t.Fatalf("state want 2, got %d", got)
	}
	// 队列表记录目标剩余等级 19
	if got := cell(t, f, "select level from building_destroying where cid=? and xy='c1'", f.CID); got != 19 {
		t.Fatalf("destroying.level want 19, got %d", got)
	}
	// 强制到期 → Queue 触发 settleAll → level-1、state=0、队列表清空
	exec(t, f, "update buildings set state_end_at=unix_timestamp()-1 where city_id=? and xy='c1'", f.CID)
	exec(t, f, "update building_destroying set state_endtime=unix_timestamp()-1 where cid=? and xy='c1'", f.CID)
	q, err := svc.Queue(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if len(q) != 0 {
		t.Fatalf("queue want empty after settle, got %d", len(q))
	}
	if got := cell(t, f, "select level from buildings where city_id=? and xy='c1'", f.CID); got != 19 {
		t.Fatalf("level want 19 after settle, got %d", got)
	}
	if got := cell(t, f, "select state from buildings where city_id=? and xy='c1'", f.CID); got != 0 {
		t.Fatalf("state want 0 after settle, got %d", got)
	}
	if got := cell(t, f, "select count(*) from building_destroying where cid=?", f.CID); got != 0 {
		t.Fatalf("destroying queue want empty, got %d", got)
	}
}

// ── 正在拆除时再次拆除 / 拆除中的取消 ─────────────────────────────────────

func TestCancelDestroy(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'d1',5,2,0,unix_timestamp()+1000)", f.CID)
	id := cell(t, f, "select id from buildings where city_id=? and xy='d1'", f.CID)
	exec(t, f, "insert into building_destroying (id,cid,xy,bid,level,state_endtime) values (?,?,'d1',2,4,unix_timestamp()+1000)", id, f.CID)

	if _, err := svc.CancelDestroy(ctx, f.UID, f.CID, 2, 0, 3, 0); err != nil {
		t.Fatalf("CancelDestroy: %v", err)
	}
	if got := cell(t, f, "select state from buildings where city_id=? and xy='d1'", f.CID); got != 0 {
		t.Fatalf("state want 0 after cancel, got %d", got)
	}
	if got := cell(t, f, "select count(*) from building_destroying where cid=?", f.CID); got != 0 {
		t.Fatalf("destroying queue want empty, got %d", got)
	}
	// 处于拆除中再次拆除 → 建筑正在拆除中
	exec(t, f, "update buildings set state=2, state_end_at=unix_timestamp()+1000 where city_id=? and xy='d1'", f.CID)
	if _, err := svc.Destroy(ctx, f.UID, f.CID, 2, 0, 3, 0); errMsg(err) != "建筑正在拆除中" {
		t.Fatalf("want 建筑正在拆除中, got %q", errMsg(err))
	}
}

// ── 彻底拆除：火油桶前置 + 原版“仅降一级”怪癖 ───────────────────────────

func TestDestroyAll(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'e1',3,0,0,0)", f.CID)

	// 无火油桶（gid=83）→ not_enough_goods83
	if _, err := svc.DestroyAll(ctx, f.UID, f.CID, 2, 0, 4, 0); errMsg(err) != "not_enough_goods83" {
		t.Fatalf("want not_enough_goods83, got %q", errMsg(err))
	}

	// 有火油桶：即时结算。原版怪癖：level>1 只降一级（level-1，state=0），非整栋移除。
	grant(t, f, 83, 1)
	if _, err := svc.DestroyAll(ctx, f.UID, f.CID, 2, 0, 4, 0); err != nil {
		t.Fatalf("DestroyAll: %v", err)
	}
	if got := cell(t, f, "select level from buildings where city_id=? and xy='e1'", f.CID); got != 2 {
		t.Fatalf("QUIRK: level want 2 (只降一级), got %d", got)
	}
	if got := cell(t, f, "select state from buildings where city_id=? and xy='e1'", f.CID); got != 0 {
		t.Fatalf("state want 0, got %d", got)
	}
	if got := cell(t, f, "select `count` from user_goods where user_id=? and gid=83", f.UID); got != 0 {
		t.Fatalf("fire barrel want 0 after use, got %d", got)
	}

	// 1 级建筑彻底拆除 → level 归零后被清除
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'e2',1,0,0,0)", f.CID)
	grant(t, f, 83, 1)
	if _, err := svc.DestroyAll(ctx, f.UID, f.CID, 2, 0, 4, 1); err != nil {
		t.Fatalf("DestroyAll lv1: %v", err)
	}
	if got := cell(t, f, "select count(*) from buildings where city_id=? and xy='e2'", f.CID); got != 0 {
		t.Fatalf("level-1 building want removed, got %d", got)
	}
}

// ── 资源地转换（startChangeBuilding）──────────────────────────────────────

func TestExchange(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	exec(t, f, "replace into city_res_add (city_id) values (?)", f.CID)
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'f1',3,0,0,0)", f.CID)

	// 无资源地转换令（gid=161504）→ not_enough_goods161504
	if _, err := svc.Exchange(ctx, f.UID, f.CID, 2, 3, 0, 5, 0); errMsg(err) != "not_enough_goods161504" {
		t.Fatalf("want not_enough_goods161504, got %q", errMsg(err))
	}

	grant(t, f, 161504, 1)
	if _, err := svc.Exchange(ctx, f.UID, f.CID, 2, 3, 0, 5, 0); err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	// 建筑类型改为目标 3，进入建造中
	if got := cell(t, f, "select building_id from buildings where city_id=? and xy='f1'", f.CID); got != 3 {
		t.Fatalf("building_id want 3, got %d", got)
	}
	if got := cell(t, f, "select state from buildings where city_id=? and xy='f1'", f.CID); got != 1 {
		t.Fatalf("state want 1, got %d", got)
	}
	// 队列表目标等级 = 原等级 3（legacy: level>20?20:level）
	if got := cell(t, f, "select level from building_upgrading where cid=? and xy='f1'", f.CID); got != 3 {
		t.Fatalf("upgrading.level want 3, got %d", got)
	}
	// 资源重算标记
	if got := cell(t, f, "select resource_changing from city_res_add where city_id=?", f.CID); got != 1 {
		t.Fatalf("city_res_add.resource_changing want 1, got %d", got)
	}
	if got := cell(t, f, "select changing from city_resources where city_id=?", f.CID); got != 1 {
		t.Fatalf("city_resources.changing want 1, got %d", got)
	}
	if got := cell(t, f, "select `count` from user_goods where user_id=? and gid=161504", f.UID); got != 0 {
		t.Fatalf("exchange item want 0 after use, got %d", got)
	}
}

// ── 官府拆除保护 ───────────────────────────────────────────────────────────

func TestGovermentDestroyGuards(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,1,'g1',1,0,0,0)", f.CID)

	if _, err := svc.Destroy(ctx, f.UID, f.CID, 1, 1, 6, 0); errMsg(err) != "1级官府不能拆除。" {
		t.Fatalf("want 1级官府不能拆除。, got %q", errMsg(err))
	}
	if _, err := svc.DestroyAll(ctx, f.UID, f.CID, 1, 1, 6, 0); errMsg(err) != "官府不能彻底拆除。" {
		t.Fatalf("want 官府不能彻底拆除。, got %q", errMsg(err))
	}
	// 玩家主城（type=5）官府 → checkBigCityDestroy
	exec(t, f, "update cities set type=5 where id=?", f.CID)
	if _, err := svc.Destroy(ctx, f.UID, f.CID, 1, 1, 6, 0); errMsg(err) != "主城无法拆除官府！" {
		t.Fatalf("want 主城无法拆除官府！, got %q", errMsg(err))
	}
}

// ── 队列任务标签（正在建造 / 正在升级 / 正在拆除）────────────────────────

func TestQueueTaskLabels(t *testing.T) {
	svc, f := newSvc(t)
	ctx := bgCtx()
	// 正在建造（state=1，目标 1）
	if _, err := svc.Build(ctx, f.UID, f.CID, 2, 0, 0, 1); err != nil {
		t.Fatalf("Build: %v", err)
	}
	// 正在升级（state=1，目标 4）
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'b2',3,1,0,unix_timestamp()+1000)", f.CID)
	idUp := cell(t, f, "select id from buildings where city_id=? and xy='b2'", f.CID)
	exec(t, f, "insert into building_upgrading (id,cid,xy,bid,level,state_endtime) values (?,?,'b2',2,4,unix_timestamp()+1000)", idUp, f.CID)
	// 正在拆除（state=2）
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,2,'c2',3,2,0,unix_timestamp()+1000)", f.CID)
	idDn := cell(t, f, "select id from buildings where city_id=? and xy='c2'", f.CID)
	exec(t, f, "insert into building_destroying (id,cid,xy,bid,level,state_endtime) values (?,?,'c2',2,2,unix_timestamp()+1000)", idDn, f.CID)

	items, err := svc.Queue(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("queue want 3, got %d", len(items))
	}
	tasks := map[string]QueueItem{}
	for _, it := range items {
		tasks[it.Task] = it
	}
	if it, ok := tasks["正在建造"]; !ok || it.TargetLevel != 1 {
		t.Fatalf("missing 正在建造/target1: %#v", items)
	}
	if it, ok := tasks["正在升级"]; !ok || it.TargetLevel != 4 {
		t.Fatalf("missing 正在升级/target4: %#v", items)
	}
	if it, ok := tasks["正在拆除"]; !ok || it.TargetLevel != 2 {
		t.Fatalf("missing 正在拆除/target2: %#v", items)
	}
}
