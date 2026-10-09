//go:build integration

package achievement

import (
	"context"
	"testing"

	"rxsg/backend/internal/model"
	"rxsg/backend/internal/testutil"
)

// achievement_integration_test.go 真库集成测试。
// 覆盖：getOverviewStat（成就点数+分组统计+最近两项）/ getAchivementsByGroup（type 0/1/2）/
//   getAchivementDetail（已完成/未完成两分支）。

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

func TestGetOverviewStat(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ex(t, f, "insert into user_achivements (uid,achivement_id,time) values (?,1001,unix_timestamp()-10)", f.UID)
	ex(t, f, "insert into user_achivements (uid,achivement_id,time) values (?,1002,unix_timestamp())", f.UID)
	ex(t, f, "update users set achivement_point=30 where id=?", f.UID)

	ret, err := svc.GetOverviewStat(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if ret[0].(int64) != 30 {
		t.Fatalf("achivement_point=%v want 30", ret[0])
	}
	recent := ret[1].([]map[string]any)
	if len(recent) != 2 {
		t.Fatalf("最近成就 len=%d want 2", len(recent))
	}
	totals := ret[2].([]map[string]any)
	byGroup := map[int64]map[string]any{}
	for _, it := range totals {
		byGroup[model.Int64(it, "group")] = it
	}
	if model.Int64(byGroup[1], "total_count") != 2 || model.Int64(byGroup[1], "finish_count") != 2 {
		t.Fatalf("group1 total=%d finish=%d want 2/2", model.Int64(byGroup[1], "total_count"), model.Int64(byGroup[1], "finish_count"))
	}
	if model.Int64(byGroup[2], "total_count") != 2 || model.Int64(byGroup[2], "finish_count") != 0 {
		t.Fatalf("group2 total=%d finish=%d want 2/0", model.Int64(byGroup[2], "total_count"), model.Int64(byGroup[2], "finish_count"))
	}
}

func TestGetAchivementsByGroup(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ex(t, f, "insert into user_achivements (uid,achivement_id,time) values (?,1001,unix_timestamp())", f.UID)

	// type=1 已完成（组1，子组0 → 只有 1001 完成）
	ret, err := svc.GetAchivementsByGroup(ctx, f.UID, 1, 0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ret[0].(int64) != 1 {
		t.Fatalf("type1 count=%v want 1", ret[0])
	}
	if len(ret[1].([]map[string]any)) != 1 {
		t.Fatalf("type1 rows=%d want 1", len(ret[1].([]map[string]any)))
	}

	// type=2 未完成（组2 → 1003,1004）
	ret, err = svc.GetAchivementsByGroup(ctx, f.UID, 2, 0, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ret[0].(int64) != 2 {
		t.Fatalf("type2 count=%v want 2", ret[0])
	}

	// type=0 全部（组1 → 1001,1002）
	ret, err = svc.GetAchivementsByGroup(ctx, f.UID, 1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if ret[0].(int64) != 2 {
		t.Fatalf("type0 count=%v want 2", ret[0])
	}
	rows := ret[1].([]map[string]any)
	if len(rows) != 2 {
		t.Fatalf("type0 rows=%d want 2", len(rows))
	}
}

func TestGetAchivementDetail(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// 未完成 + type2（数值型）：目标 10000 黄金；fixture city gold=10000 → userValue=10000
	ret, err := svc.GetAchivementDetail(ctx, f.UID, 1002)
	if err != nil {
		t.Fatal(err)
	}
	info := ret[0].(map[string]any)
	if info["name"] != "富甲一方" {
		t.Fatalf("name=%v", info["name"])
	}
	if info["sql_current_value"] != "" {
		t.Fatalf("info.sql_current_value 应被清空, got %v", info["sql_current_value"])
	}
	prog := ret[1].([]map[string]any)
	if len(prog) != 1 || model.Int64(prog[0], "targetValue") != 10000 || model.Int64(prog[0], "userValue") != 10000 {
		t.Fatalf("type2 progress=%v want target=10000 user=10000", prog)
	}
	if ret[2].(int64) != 0 {
		t.Fatalf("完成人数=%v want 0", ret[2])
	}

	// 未完成 + type3（目标型）：两个子目标均满足（fixture 有城/将）
	ret, err = svc.GetAchivementDetail(ctx, f.UID, 1004)
	if err != nil {
		t.Fatal(err)
	}
	prog = ret[1].([]map[string]any)
	if len(prog) != 2 {
		t.Fatalf("type3 未完成 progress=%d want 2", len(prog))
	}
	for _, p := range prog {
		if p["isDone"] != true {
			t.Fatalf("type3 isDone=%v want true", p["isDone"])
		}
	}

	// 已完成分支：type2 直接返回 target 作为 userValue
	ex(t, f, "insert into user_achivements (uid,achivement_id,time) values (?,1002,unix_timestamp())", f.UID)
	ret, err = svc.GetAchivementDetail(ctx, f.UID, 1002)
	if err != nil {
		t.Fatal(err)
	}
	prog = ret[1].([]map[string]any)
	if len(prog) != 1 || model.Int64(prog[0], "userValue") != 10000 || model.Int64(prog[0], "targetValue") != 10000 {
		t.Fatalf("已完成 type2 progress=%v want user=target=10000", prog)
	}
	if ret[2].(int64) != 1 {
		t.Fatalf("完成人数=%v want 1", ret[2])
	}

	// 已完成分支：type3 子目标 isDone=1
	ex(t, f, "insert into user_achivements (uid,achivement_id,time) values (?,1004,unix_timestamp())", f.UID)
	ret, err = svc.GetAchivementDetail(ctx, f.UID, 1004)
	if err != nil {
		t.Fatal(err)
	}
	prog = ret[1].([]map[string]any)
	if len(prog) != 2 || model.Int(prog[0], "isDone") != 1 {
		t.Fatalf("已完成 type3 progress=%v want isDone=1", prog)
	}
}
