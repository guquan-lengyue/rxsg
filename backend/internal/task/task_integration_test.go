//go:build integration

package task

import (
	"context"
	"testing"

	"rxsg/backend/internal/testutil"
)

// task_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：completeTask 幂等 / completeTaskWithTaskid / checkGoalComplete 多分支边界 + 原版怪癖 /
//   checkTaskComplete(含 alarms 红点) / checkTaskCount / getTaskList / getTaskDetail /
//   getReward 六类奖励落库 / reduceGoal 扣减 / dropTask / dropSysTask。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

func ex(t *testing.T, f *testutil.Fixture, q string, args ...any) {
	t.Helper()
	if _, err := f.DB.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func cell(t *testing.T, f *testutil.Fixture, q string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), q, args...)
	if err != nil {
		t.Fatalf("cell %q: %v", q, err)
	}
	return v
}

func count(t *testing.T, f *testutil.Fixture, q string, args ...any) int64 {
	t.Helper()
	return cell(t, f, q, args...)
}

// insertUserTask 插入一行 user_tasks（uid 固定为 fixture）。
func insertUserTask(t *testing.T, f *testutil.Fixture, tid, state int) {
	t.Helper()
	ex(t, f, "insert into user_tasks (uid,tid,state) values (?,?,?) on duplicate key update state=?", f.UID, tid, state, state)
}

func TestCompleteTaskIdempotent(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	if err := svc.CompleteTask(ctx, f.UID, 10011); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteTask(ctx, f.UID, 10011); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f, "select count(*) from user_goals where uid=? and gid=10011", f.UID); n != 1 {
		t.Fatalf("user_goals rows=%d want 1（replace into 幂等）", n)
	}
}

func TestCompleteTaskWithTaskid(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1001); err != nil {
		t.Fatal(err)
	}
	for _, gid := range []int{10011, 10012} {
		if n := count(t, f, "select count(*) from user_goals where uid=? and gid=?", f.UID, gid); n != 1 {
			t.Fatalf("gid %d 未写入（rows=%d）", gid, n)
		}
	}
}

func TestCheckGoalCompleteBranches(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// sort=1 type=1 黄金（fixture city_resources.gold=10000）
	g := map[string]any{"sort": 1, "type": 1, "count": int64(5000)}
	ok, err := svc.CheckGoalComplete(ctx, f.UID, g)
	if err != nil || !ok {
		t.Fatalf("gold>=5000 want true, got %v err=%v", ok, err)
	}
	g["count"] = int64(20000)
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); ok {
		t.Fatal("gold>=20000 want false")
	}

	// sort=2 type=1 道具（gid=1）
	ex(t, f, "insert into user_goods (user_id,gid,`count`) values (?,1,2) on duplicate key update `count`=2", f.UID)
	g = map[string]any{"sort": 2, "type": 1, "count": int64(2)}
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); !ok {
		t.Fatal("goods gid1>=2 want true")
	}
	g["count"] = int64(3)
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); ok {
		t.Fatal("goods gid1>=3 want false（差 1）")
	}

	// sort=5 type=1 任务物品（tid=1）
	ex(t, f, "insert into things (user_id,tid,`count`) values (?,1,3) on duplicate key update `count`=3", f.UID)
	g = map[string]any{"sort": 5, "type": 1, "count": int64(3)}
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); !ok {
		t.Fatal("things tid1>=3 want true")
	}
	g["count"] = int64(4)
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); ok {
		t.Fatal("things tid1>=4 want false（差 1）")
	}

	// sort=50 累计（currentcount）
	g = map[string]any{"sort": 50, "count": int64(3), "currentcount": int64(3)}
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); !ok {
		t.Fatal("sort50 currentcount=3>=3 want true")
	}
	g["currentcount"] = int64(2)
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, g); ok {
		t.Fatal("sort50 currentcount=2>=3 want false（差 1）")
	}

	// 原版怪癖 checkGoalComplete:30：user_goal 有记录且 sort∉{50,80} → 直接判完成
	quirk := map[string]any{"uid": f.UID, "sort": 1, "type": 1, "count": int64(999999999)}
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, quirk); !ok {
		t.Fatal("记录存在即完成（怪癖：uid 非空且 sort!=50/80）want true")
	}
	// 但 sort=50 时该怪癖不生效
	quirk50 := map[string]any{"uid": f.UID, "sort": 50, "count": int64(999999999), "currentcount": int64(0)}
	if ok, _ := svc.CheckGoalComplete(ctx, f.UID, quirk50); ok {
		t.Fatal("sort=50 不受 uid 怪癖影响 want false")
	}
}

func TestCheckTaskCompleteAndAlarm(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 1001, 0)
	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1001); err != nil {
		t.Fatal(err)
	}
	tasklist, err := f.DB.FetchRows(ctx,
		"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=1 and u.state=0", f.UID)
	if err != nil {
		t.Fatal(err)
	}
	firstSet, err := svc.CheckTaskComplete(ctx, f.UID, tasklist)
	if err != nil {
		t.Fatal(err)
	}
	if firstSet {
		t.Fatal("firstSet want false（有任务完成触发红点）")
	}
	if tasklist[0]["state"] != true {
		t.Fatalf("task.state want true, got %v", tasklist[0]["state"])
	}
	if v := count(t, f, "select `task` from alarms where user_id=?", f.UID); v != 1 {
		t.Fatalf("alarms.task=%d want 1", v)
	}
	// firstSet 语义：本批"没有任何任务完成"才为 true；任务仍完成 → 再次调用依旧 false。
	tasklist2, _ := f.DB.FetchRows(ctx,
		"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=1 and u.state=0", f.UID)
	fs2, _ := svc.CheckTaskComplete(ctx, f.UID, tasklist2)
	if fs2 {
		t.Fatal("任务仍完成时 firstSet want false")
	}
}

func TestCheckTaskCount(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 1001, 0)
	insertUserTask(t, f, 1002, 0)
	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1001); err != nil {
		t.Fatal(err)
	}
	list, _ := f.DB.FetchRows(ctx,
		"select t.* from user_tasks u, cfg_tasks t where u.uid=? and u.tid=t.id and t.`group`=1 and u.state=0", f.UID)
	n, err := svc.CheckTaskCount(ctx, f.UID, list)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("checkTaskCount=%d want 1（仅 1001 完成）", n)
	}
}

func TestGetTaskList(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 1001, 0)
	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1001); err != nil {
		t.Fatal(err)
	}
	ret, err := svc.GetTaskList(ctx, f.UID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 2 {
		t.Fatalf("GetTaskList len=%d want 2", len(ret))
	}
	list, ok := ret[0].([]map[string]any)
	if !ok || len(list) != 1 {
		t.Fatalf("tasklist=%v want 1 项", ret[0])
	}
	if list[0]["state"] != true {
		t.Fatalf("task state want true, got %v", list[0]["state"])
	}
	if ret[1].(int) != 1 {
		t.Fatalf("count=%v want 1（与 checkTaskCount 一致）", ret[1])
	}
}

func TestGetTaskDetail(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ret, err := svc.GetTaskDetail(ctx, f.UID, 1001)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 3 {
		t.Fatalf("detail len=%d want 3", len(ret))
	}
	taskRow := ret[0].(map[string]any)
	if taskRow["name"] != "初露锋芒" {
		t.Fatalf("task name=%v", taskRow["name"])
	}
	goals := ret[1].([]map[string]any)
	if len(goals) != 2 {
		t.Fatalf("goals len=%d want 2", len(goals))
	}
	for _, g := range goals {
		if _, has := g["state"]; !has {
			t.Fatalf("goal 缺 state: %v", g)
		}
	}
	rewards := ret[2].([]map[string]any)
	if len(rewards) != 3 {
		t.Fatalf("rewards len=%d want 3", len(rewards))
	}
	// reward 的 count 与 cfg 一致
	if v := count(t, f, "select `count` from cfg_task_rewards where tid=1001 and sort=1"); v != 1000 {
		t.Fatalf("sort1 reward count=%d want 1000", v)
	}
}

func TestGetRewardGivesAllKinds(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 1001, 0)
	insertUserTask(t, f, 1002, 0)
	// 满足 1001：gold(10000>=5000) + goods gid1>=2
	ex(t, f, "insert into user_goods (user_id,gid,`count`) values (?,1,2) on duplicate key update `count`=2", f.UID)
	// 满足 1002：city_soldiers sid1>=10 + things tid1>=3
	ex(t, f, "insert into city_soldiers (city_id,soldier_id,`count`) values (?,1,10)", f.CID)
	ex(t, f, "insert into things (user_id,tid,`count`) values (?,1,3)", f.UID)
	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1001); err != nil {
		t.Fatal(err)
	}
	if err := svc.CompleteTaskWithTaskid(ctx, f.UID, 1002); err != nil {
		t.Fatal(err)
	}

	// 1001 奖励：粮食+1000、礼金+50、道具 gid1 +5
	r, err := svc.GetReward(ctx, f.UID, 1001, 0, 0)
	if err != nil {
		t.Fatalf("getReward 1001: %v", err)
	}
	if len(r) != 1 || r[0].(int) != 1 {
		t.Fatalf("getReward 返回 %v want [1]", r)
	}
	if v := cell(t, f, "select food from city_resources where city_id=?", f.CID); v != 101000 {
		t.Fatalf("food=%d want 101000(+1000)", v)
	}
	if v := cell(t, f, "select gift from users where id=?", f.UID); v != 50 {
		t.Fatalf("gift=%d want 50(+50)", v)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=1", f.UID); v != 7 {
		t.Fatalf("goods gid1=%d want 7(2+5)", v)
	}
	// 领取后非可重复任务 → state=1
	if v := cell(t, f, "select state from user_tasks where uid=? and tid=1001", f.UID); v != 1 {
		t.Fatalf("task1001 state=%d want 1", v)
	}

	// 1002 奖励：兵力+20、城防+5、黄金+8000
	if _, err := svc.GetReward(ctx, f.UID, 1002, 0, 0); err != nil {
		t.Fatalf("getReward 1002: %v", err)
	}
	if v := cell(t, f, "select `count` from city_soldiers where city_id=? and soldier_id=1", f.CID); v != 30 {
		t.Fatalf("soldier sid1=%d want 30(10+20)", v)
	}
	if v := cell(t, f, "select `count` from city_defences where city_id=? and did=1", f.CID); v != 5 {
		t.Fatalf("defence did1=%d want 5", v)
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 18000 {
		t.Fatalf("gold=%d want 18000(+8000)", v)
	}
}

func TestReduceGoal(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 1003, 0)
	if err := svc.CompleteTask(ctx, f.UID, 10031); err != nil {
		t.Fatal(err)
	}
	goldBefore := cell(t, f, "select gold from city_resources where city_id=?", f.CID)
	if _, err := svc.GetReward(ctx, f.UID, 1003, 0, 0); err != nil {
		t.Fatalf("getReward 1003: %v", err)
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != goldBefore-100 {
		t.Fatalf("gold=%d want %d（reduceGoal 扣 100）", v, goldBefore-100)
	}
	if v := cell(t, f, "select gift from users where id=?", f.UID); v != 10 {
		t.Fatalf("gift=%d want 10", v)
	}
}

func TestDropTask(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ex(t, f, "insert into user_tasks (uid,tid,state) values (?,20000,0),(?,30000,0)", f.UID, f.UID)
	ex(t, f, "insert into user_goals (uid,gid,currentcount) values (?,20001,0),(?,30001,0)", f.UID, f.UID)

	// taskgroup=20002 → 尾号非 1 分支：删除 tid∈{20000,30000} 的 user_tasks 及其 goal
	if _, err := svc.DropTask(ctx, f.UID, 20002); err != nil {
		t.Fatal(err)
	}
	if n := count(t, f, "select count(*) from user_tasks where uid=? and tid in (20000,30000)", f.UID); n != 0 {
		t.Fatalf("user_tasks 剩余 %d want 0", n)
	}
	if n := count(t, f, "select count(*) from user_goals where uid=? and gid in (20001,30001)", f.UID); n != 0 {
		t.Fatalf("user_goals 剩余 %d want 0", n)
	}
}

func TestDropSysTask(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	insertUserTask(t, f, 80001, 0)
	if _, err := svc.DropSysTask(ctx, f.UID, 80001); err != nil {
		t.Fatal(err)
	}
	if v := cell(t, f, "select state from user_tasks where uid=? and tid=80001", f.UID); v != 1 {
		t.Fatalf("sys task state=%d want 1", v)
	}
	// 不存在的任务
	if _, err := svc.DropSysTask(ctx, f.UID, 99999999); err == nil {
		t.Fatal("不存在任务 want error(not_task_of_user)")
	}
	// 非随机任务（group 不在 80000-99999）
	if _, err := svc.DropSysTask(ctx, f.UID, 1001); err == nil {
		t.Fatal("非随机任务 want error(not_systask)")
	}
}
