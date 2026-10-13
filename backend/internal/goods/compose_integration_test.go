//go:build integration

package goods

import (
	"context"
	"testing"
)

// compose_integration_test.go 真库集成测试（R4-C 背包「宝物合成」goodsCompose + 名将卡批量 multiUseCard）。
// 逐分支与 GoodsFunc.php:4836/4647 对照，含原版文案与顺序怪癖（10839 白名单后判上限、
// 马鬃/马鞭/聚魂珠三档缺料文案不同、马鞭 25% 成功率）。

// ── 入参/上限校验（GoodsFunc.php:4844-4851）──────────────────────────────────

func TestGoodsComposeGuard(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 非白名单 gid → 该道具不能进行合成操作（:4846）
	for _, gid := range []int{300, 10829, 10840, 19080, 19083} {
		if _, err := s.GoodsCompose(ctx, f.UID, gid); err == nil || err.Error() != msgCanNotCompose {
			t.Fatalf("gid=%d err=%v want %q", gid, err, msgCanNotCompose)
		}
	}
	// 10 级聚魂珠已达上限 → 该聚魂珠的品质已经达到最高（:4850，先于缺料判定）
	if _, err := s.GoodsCompose(ctx, f.UID, 10839); err == nil || err.Error() != msgComposeMaxLevel {
		t.Fatalf("10839 err=%v want %q", err, msgComposeMaxLevel)
	}
}

// ── 聚魂珠（10830-10838：需 2 耗 2 成 1）────────────────────────────────────

func TestGoodsComposeBead(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 不足 2 → 你进行合成的聚魂珠数量不够（:4884）
	grant(t, f, 10830, 1)
	if _, err := s.GoodsCompose(ctx, f.UID, 10830); err == nil || err.Error() != msgComposeNotEnough {
		t.Fatalf("bead low err=%v want %q", err, msgComposeNotEnough)
	}
	if countOf(t, f, 10830) != 1 { // 失败不扣料
		t.Fatalf("10830=%d want 1 after fail", countOf(t, f, 10830))
	}

	// 足额 → 成功；10830 -2、10831 +1
	grant(t, f, 10830, 3)
	res, err := s.GoodsCompose(ctx, f.UID, 10830)
	if err != nil {
		t.Fatalf("bead compose: %v", err)
	}
	if res.Success != 1 {
		t.Fatalf("success=%d want 1", res.Success)
	}
	if countOf(t, f, 10830) != 1 {
		t.Fatalf("10830=%d want 1", countOf(t, f, 10830))
	}
	if countOf(t, f, 10831) != 1 {
		t.Fatalf("10831=%d want 1", countOf(t, f, 10831))
	}
	if len(res.Goods) != 1 {
		t.Fatalf("goods rows=%d want 1", len(res.Goods))
	}
	// 显示行 = 目标 gid 的 cfg_goods（0024 种子名）+ count=1
	if got := res.Goods[0]["name"]; got != "2级聚魂珠" {
		t.Fatalf("name=%v want 2级聚魂珠", got)
	}
}

// ── 马鬃 19081（需 5 耗 5 成 1，必成）───────────────────────────────────────

func TestGoodsComposeMane(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 19081, 4)
	if _, err := s.GoodsCompose(ctx, f.UID, 19081); err == nil || err.Error() != msgComposeManeLow {
		t.Fatalf("mane low err=%v want %q", err, msgComposeManeLow)
	}

	grant(t, f, 19081, 5)
	res, err := s.GoodsCompose(ctx, f.UID, 19081)
	if err != nil {
		t.Fatalf("mane compose: %v", err)
	}
	if res.Success != 1 {
		t.Fatalf("success=%d want 1", res.Success)
	}
	if countOf(t, f, 19081) != 0 {
		t.Fatalf("19081=%d want 0", countOf(t, f, 19081))
	}
	if countOf(t, f, 19082) != 1 {
		t.Fatalf("19082=%d want 1", countOf(t, f, 19082))
	}
	if got := res.Goods[0]["name"]; got != "马鞭" {
		t.Fatalf("name=%v want 马鞭", got)
	}
}

// ── 马鞭 19082（需 2 耗 2，25% 成 1）────────────────────────────────────────

func TestGoodsComposeWhip(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 19082, 1)
	if _, err := s.GoodsCompose(ctx, f.UID, 19082); err == nil || err.Error() != msgComposeWhipLow {
		t.Fatalf("whip low err=%v want %q", err, msgComposeWhipLow)
	}

	// 多轮验证不变式：每次必耗 2；success==1 ⇔ 19083 +1
	succeeds := 0
	for i := 0; i < 40; i++ {
		grant(t, f, 19082, 4)
		before := countOf(t, f, 19083)
		res, err := s.GoodsCompose(ctx, f.UID, 19082)
		if err != nil {
			t.Fatalf("whip compose: %v", err)
		}
		if countOf(t, f, 19082) != 2 {
			t.Fatalf("19082=%d want 2", countOf(t, f, 19082))
		}
		delta := countOf(t, f, 19083) - before
		if res.Success == 1 {
			succeeds++
			if delta != 1 {
				t.Fatalf("success but 19083 delta=%d want 1", delta)
			}
			if got := res.Goods[0]["name"]; got != "汗血马鞭" {
				t.Fatalf("name=%v want 汗血马鞭", got)
			}
		} else if delta != 0 {
			t.Fatalf("fail but 19083 delta=%d want 0", delta)
		}
	}
	if succeeds == 0 || succeeds == 40 { // 25% 概率下 40 次全成/全败几乎不可能；失败即视为分支未接通
		t.Fatalf("whip success=%d/40，疑似概率分支未生效", succeeds)
	}
}

// ── multiUseCard（mem_state 197 缺失 → 恒「该功能未开放」）──────────────────

func TestMultiUseCardNotAvailable(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 确保 mem_state 无 state=197 行（新库默认如此）
	if _, err := f.DB.Exec(ctx, "delete from mem_state where state=197"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MultiUseCard(ctx, f.UID); err == nil || err.Error() != msgNotAvailable {
		t.Fatalf("multiUseCard err=%v want %q", err, msgNotAvailable)
	}

	// 置非 60/55555555 → 仍抛（GoodsFunc.php:4650）
	if _, err := f.DB.Exec(ctx, "replace into mem_state (state,value) values (197,1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MultiUseCard(ctx, f.UID); err == nil || err.Error() != msgNotAvailable {
		t.Fatalf("multiUseCard(state=1) err=%v want %q", err, msgNotAvailable)
	}
	t.Cleanup(func() {
		_, _ = f.DB.Exec(context.Background(), "delete from mem_state where state=197")
	})
}
