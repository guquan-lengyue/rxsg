//go:build integration

package city

import (
	"context"
	"testing"
)

// r11_2_integration_test.go —— R11-2 项②（Alarm）与项③（Defences）真库集成测试。
//
// 项②：GetAlarms 对齐 legacy sys_alarm 顶栏红点。新库 alarms(user_id,task,report)：
//   task = 可领取任务（task），mail = 未读战报（report）【按前端口径在服务端计算，非 legacy 原样】。
// 项③：GetDefences 对齐 DefenceFunc.php:40 doGetDefenceInfo（cfg_defence LEFT JOIN city_defences）。

func TestR112GetAlarms(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 无 alarms 行 → 全 0
	a, err := s.GetAlarms(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetAlarms: %v", err)
	}
	if a.UID != f.UID || a.Task != 0 || a.Mail != 0 {
		t.Fatalf("无告警应全 0, got %#v", a)
	}

	// 有可领任务 + 未读战报 → task>0, mail>0
	if _, err := s.db.Exec(ctx,
		"insert into alarms (user_id,task,report) values (?,1,1) "+
			"on duplicate key update task=1,report=1", f.UID); err != nil {
		t.Fatal(err)
	}
	a2, err := s.GetAlarms(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetAlarms(2): %v", err)
	}
	if a2.Task <= 0 {
		t.Fatalf("有可领任务时 task 应 >0, got %d", a2.Task)
	}
	if a2.Mail <= 0 {
		t.Fatalf("有未读战报时 mail 应 >0, got %d", a2.Mail)
	}

	// 非属主
	if _, err := s.GetAlarms(ctx, f.UID+999999, f.CID); err == nil {
		t.Fatal("非属主应报错")
	}
}

func TestR112GetDefences(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 空情形：cfg_defence 5 行 LEFT JOIN 无城防存量 → 5 项 count=0
	list, err := s.GetDefences(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetDefences: %v", err)
	}
	if len(list) != 5 {
		t.Fatalf("器械数 want 5 (cfg_defence), got %d", len(list))
	}
	d1 := list[0]
	if d1.DID != 1 || d1.DName != "陷阱" || d1.CID != f.CID {
		t.Fatalf("首项不符: %#v", d1)
	}
	if d1.Hp != 80000 || d1.Ap != 300 || d1.Dp != 200 || d1.Range != 30 {
		t.Fatalf("器械属性不符: hp=%d ap=%d dp=%d range=%d", d1.Hp, d1.Ap, d1.Dp, d1.Range)
	}
	for _, d := range list {
		if d.Count != 0 {
			t.Fatalf("无存量时 count 应为 0, got %d (did=%d)", d.Count, d.DID)
		}
		if !d.CanReinforce {
			t.Fatalf("无前置条件时应可加固, did=%d", d.DID)
		}
		// 原配置缺失 → time_need=0 → reinforce_time=max(1,0)=1
		if d.ReinforceTime != 1 {
			t.Fatalf("reinforce_time want 1, got %d", d.ReinforceTime)
		}
		if len(d.Conditions) != 0 {
			t.Fatalf("无条件表行时 conditions 应为空, got %d", len(d.Conditions))
		}
	}

	// 非空情形：写入城防存量
	if _, err := s.db.Exec(ctx,
		"insert into city_defences (city_id,did,count) values (?,3,120)", f.CID); err != nil {
		t.Fatal(err)
	}
	list2, err := s.GetDefences(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetDefences(2): %v", err)
	}
	var d3 *WallDefence
	for i := range list2 {
		if list2[i].DID == 3 {
			d3 = &list2[i]
		}
	}
	if d3 == nil || d3.Count != 120 || d3.DName != "箭塔" {
		t.Fatalf("did=3 存量不符: %#v", d3)
	}

	// 非属主
	if _, err := s.GetDefences(ctx, f.UID+999999, f.CID); err == nil {
		t.Fatal("非属主应报错")
	}
}
