//go:build integration

package armor

import (
	"context"
	"testing"

	"rxsg/backend/internal/testutil"
)

// synth_integration_test.go 真库集成测试（R4-A 宝石合成 EquipmentFunc.php:1156 synthStuff）。
// 覆盖：1 级拒 / count<=0 / 材料不足 / 技术等级不足 / 100% 成功（205 与巧手符 100% 分支）/
// 巧手符与保护符扣减 / 保护符「失败不扣材料」。

// setTechnic 写一行 city_technics（合成技巧 tid=23）。
func setTechnic(t *testing.T, f *testutil.Fixture, tid, level int) {
	t.Helper()
	if _, err := f.DB.Exec(context.Background(),
		"insert into city_technics (city_id, technic_id, level) values (?,?,?)", f.CID, tid, level); err != nil {
		t.Fatalf("set technic %d: %v", tid, err)
	}
}

// ① 1 级宝石（300）不能合成。
func TestSynthLevel1Rejected(t *testing.T) {
	s, f := newSvc(t)
	_, err := s.Synth(context.Background(), f.UID, f.CID, 300, 1, 0, 0)
	errIs(t, err, msgLinitLevel1)
}

// ② count<=0 → 材料合成数量必须为正。
func TestSynthCountNotPositive(t *testing.T) {
	s, f := newSvc(t)
	for _, c := range []int{0, -1} {
		_, err := s.Synth(context.Background(), f.UID, f.CID, 301, c, 0, 0)
		errIs(t, err, msgSynthCount)
	}
}

// ③ 材料不足（配方 301='300,1'，无 300）→ 你没有足够的合成材料。
func TestSynthNoStuff(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setTechnic(t, f, tidSynthTechnic, 2) // 目标 301 = 2 级，需合成技巧 2 级
	_, err := s.Synth(ctx, f.UID, f.CID, 301, 1, 0, 0)
	errIs(t, err, msgNoStuff)
}

// ④ 技术等级不足 → 需要合成技巧(N级)。
func TestSynthTechnicLimit(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setTechnic(t, f, tidSynthTechnic, 2)
	// 300-379：目标 309 = 10 级 > 技巧 2 级
	_, err := s.Synth(ctx, f.UID, f.CID, 309, 1, 0, 0)
	errIs(t, err, "需要合成技巧(10级)")
	// 17500-17539：需技巧 10 级
	_, err = s.Synth(ctx, f.UID, f.CID, 17500, 1, 0, 0)
	errIs(t, err, "需要合成技巧(10级)")
}

// ⑤a 205 强化宝珠：跳过成功率计算（恒 100%），无保护符按 count 扣材料。
func TestSynth205Success100(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setGoods(t, f, 300, 5) // 205 配方 = '300,1'
	ret, err := s.Synth(ctx, f.UID, f.CID, 205, 3, 0, 0)
	if err != nil {
		t.Fatalf("synth 205: %v", err)
	}
	if ret[0].(int) != 3 {
		t.Fatalf("synth_count=%v want 3", ret[0])
	}
	if ret[1].(string) != "300" {
		t.Fatalf("need_gid=%v want 300", ret[1])
	}
	if ret[2].(int) != 3 { // 无保护符：dict(1)*count(3)
		t.Fatalf("need_pear=%v want 3", ret[2])
	}
	if goods(t, f, 205) != 3 {
		t.Fatalf("205=%d want 3", goods(t, f, 205))
	}
	if goods(t, f, 300) != 2 {
		t.Fatalf("300=%d want 2", goods(t, f, 300))
	}
	if v := cellExists(t, f, "select count(*) from log_user_actions where user_id=? and aid=23", f.UID); v != 1 {
		t.Fatalf("log_user_actions aid23 rows=%d want 1", v)
	}
}

// ⑤b 巧手符特判 100%：目标 301（2 级）dj=1，count(2) <= limit(6561) → 100%，扣 202×count。
func TestSynthQiaoshou100(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setTechnic(t, f, tidSynthTechnic, 2)
	setGoods(t, f, 300, 10)
	setGoods(t, f, gidQiaoShou, 2)
	ret, err := s.Synth(ctx, f.UID, f.CID, 301, 2, 1, 0)
	if err != nil {
		t.Fatalf("synth 301 dj1: %v", err)
	}
	if ret[0].(int) != 2 {
		t.Fatalf("synth_count=%v want 2", ret[0])
	}
	if goods(t, f, 301) != 2 {
		t.Fatalf("301=%d want 2", goods(t, f, 301))
	}
	if goods(t, f, 300) != 8 { // 无保护符：dict(1)*count(2)
		t.Fatalf("300=%d want 8", goods(t, f, 300))
	}
	if goods(t, f, gidQiaoShou) != 0 {
		t.Fatalf("202=%d want 0", goods(t, f, gidQiaoShou))
	}
}

// ⑥a 保护符成功：材料按 synth_count 扣（205 恒成功，故 = count），并扣 11172×count。
func TestSynthPearlProtectDeduct(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setGoods(t, f, 300, 10)
	setGoods(t, f, gidPearlProtect, 2)
	ret, err := s.Synth(ctx, f.UID, f.CID, 205, 2, 0, 1)
	if err != nil {
		t.Fatalf("synth 205 protect: %v", err)
	}
	if ret[0].(int) != 2 || ret[2].(int) != 2 {
		t.Fatalf("ret=%v want [2 300 2]", ret)
	}
	if goods(t, f, 300) != 8 {
		t.Fatalf("300=%d want 8", goods(t, f, 300))
	}
	if goods(t, f, gidPearlProtect) != 0 {
		t.Fatalf("11172=%d want 0", goods(t, f, gidPearlProtect))
	}
	if goods(t, f, 205) != 2 {
		t.Fatalf("205=%d want 2", goods(t, f, 205))
	}
}

// ⑥b 保护符「失败不扣材料」：目标 17539（15 级 prop=1，失败率 99%）。
// 失败时只扣保护符（×count），材料一分不扣；成功时才扣材料（×synth_count）。
func TestSynthProtectFailureKeepsMaterial(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setTechnic(t, f, tidSynthTechnic, 10)
	setGoods(t, f, 17538, 500) // 17539 配方 = '17538,1'
	setGoods(t, f, gidPearlProtect, 500)

	sawFail := false
	for i := 0; i < 300; i++ {
		beforeMat := goods(t, f, 17538)
		beforeProt := goods(t, f, gidPearlProtect)
		ret, err := s.Synth(ctx, f.UID, f.CID, 17539, 1, 0, 1)
		if err != nil {
			t.Fatalf("synth 17539 protect: %v", err)
		}
		if goods(t, f, gidPearlProtect) != beforeProt-1 {
			t.Fatalf("保护符应每次扣 1，got %d want %d", goods(t, f, gidPearlProtect), beforeProt-1)
		}
		sc := ret[0].(int)
		if sc == 0 {
			sawFail = true
			if goods(t, f, 17538) != beforeMat {
				t.Fatalf("合成失败却扣除了材料：%d -> %d", beforeMat, goods(t, f, 17538))
			}
			break
		}
		if goods(t, f, 17538) != beforeMat-1 {
			t.Fatalf("合成成功材料应扣 1：%d -> %d", beforeMat, goods(t, f, 17538))
		}
	}
	if !sawFail {
		t.Fatalf("300 次内未观察到失败（prop=1 失败率 99%%）")
	}
}
