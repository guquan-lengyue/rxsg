//go:build integration

package armor

import (
	"context"
	"strings"
	"testing"

	"rxsg/backend/internal/model"
)

// barn_integration_test.go 真库集成测试：R11-3 马厩/坐骑批次（BarnFunc.php + ArmorFunc.php doUpgradeArmor）。
// 覆盖：loadBarnGoods 参数校验/道具带 count；loadZuojiArmor 槽位过滤；doUnlade 成功/文言失败；
// doUpgradeArmor 校验分支/成功/失败保护；loadEmbedPearlByArmor；loadUnActiveHorseArmor。

func TestLoadBarnGoods(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 参数越界 → "参数异常"（xilian.param_error）
	if _, err := s.LoadBarnGoods(ctx, f.UID, 3); err == nil || !strings.Contains(err.Error(), msgXilianParamError) {
		t.Fatalf("xilianIndex=3 want %q, got %v", msgXilianParamError, err)
	}
	if _, err := s.LoadBarnGoods(ctx, f.UID, -1); err == nil || !strings.Contains(err.Error(), msgXilianParamError) {
		t.Fatalf("xilianIndex=-1 want %q, got %v", msgXilianParamError, err)
	}

	setGoods(t, f, 212, 3)
	rows, err := s.LoadBarnGoods(ctx, f.UID, 0)
	if err != nil {
		t.Fatalf("LoadBarnGoods: %v", err)
	}
	found := false
	for _, r := range rows {
		if model.Int(r, "gid") == 212 {
			found = true
			if model.Int(r, "count") != 3 {
				t.Fatalf("gid212 count want 3, got %v", r["count"])
			}
		}
	}
	if !found {
		t.Fatalf("gid=212 not in barn goods: %#v", rows)
	}
}

func TestLoadZuojiArmorSlotFilter(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setGoods(t, f, 12178, 1) // 战神马装I（迁移 0017 置 zuoji_type=1）

	rows, err := s.LoadZuojiArmor(ctx, f.UID, 1, 53016)
	if err != nil {
		t.Fatalf("LoadZuojiArmor: %v", err)
	}
	if len(rows) != 1 || model.Int(rows[0], "gid") != 12178 {
		t.Fatalf("slot1 want [12178], got %#v", rows)
	}
	// 槽位 2 不应命中
	rows2, err := s.LoadZuojiArmor(ctx, f.UID, 2, 53016)
	if err != nil {
		t.Fatalf("LoadZuojiArmor slot2: %v", err)
	}
	if len(rows2) != 0 {
		t.Fatalf("slot2 want empty, got %#v", rows2)
	}
}

func TestDoUnlade(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 无该装备 → "装备不存在"（equipment.no_such_armor）
	if _, err := s.DoUnlade(ctx, f.UID, 999999, 12178, 0); err == nil || !strings.Contains(err.Error(), msgNoSuchArmor) {
		t.Fatalf("missing armor want %q, got %v", msgNoSuchArmor, err)
	}
	// 位置越界 → "你选择的坐骑位置不对"
	if _, err := s.DoUnlade(ctx, f.UID, 1, 12178, 5); err == nil || !strings.Contains(err.Error(), msgInvalidPos) {
		t.Fatalf("pos=5 want %q, got %v", msgInvalidPos, err)
	}
	// gid 与槽位不符 → "你选择的坐骑位置不对"
	if _, err := s.DoUnlade(ctx, f.UID, 1, 12182, 0); err == nil || !strings.Contains(err.Error(), msgInvalidPos) {
		t.Fatalf("isFitPos mismatch want %q, got %v", msgInvalidPos, err)
	}

	// 构造坐骑（part=12 = 53016 冰封马），槽位 0 已装 12178
	sid := insertArmor(t, f, 53016, 1800, 180, 180)
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_pearls='12178,0,0,0,0' where sid=?", sid); err != nil {
		t.Fatalf("set pearls: %v", err)
	}
	// 该槽位实际是 12178，用 12182(pos4) → isFitPos 通过但值不符 → "数据异常"（waigua.invalid）
	if _, err := s.DoUnlade(ctx, f.UID, sid, 12182, 4); err == nil || !strings.Contains(err.Error(), msgBarnWaiguaInvalid) {
		t.Fatalf("value mismatch want %q, got %v", msgBarnWaiguaInvalid, err)
	}

	res, err := s.DoUnlade(ctx, f.UID, sid, 12178, 0)
	if err != nil {
		t.Fatalf("DoUnlade: %v", err)
	}
	if res.Pos != 0 || res.Pearls != "0,0,0,0,0" {
		t.Fatalf("unlade result %+v want pos0/0,0,0,0,0", res)
	}
	if got := goods(t, f, 12178); got != 1 {
		t.Fatalf("goods12178 want 1, got %d", got)
	}
	if pearls := armorStr(t, f, sid, "embed_pearls"); pearls != "0,0,0,0,0" {
		t.Fatalf("embed_pearls want 0,0,0,0,0 got %q", pearls)
	}
}

func TestDoUpgradeArmorValidation(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// sid<0 → hero.xidian_unvalid
	if _, err := s.DoUpgradeArmor(ctx, f.UID, -1, false); err == nil || !strings.Contains(err.Error(), msgUpgradeXidianUnvalid) {
		t.Fatalf("sid=-1 want %q, got %v", msgUpgradeXidianUnvalid, err)
	}
	// 装备不存在 → equipArmor.arm_not_exist
	if _, err := s.DoUpgradeArmor(ctx, f.UID, 999999, false); err == nil || !strings.Contains(err.Error(), msgEquipArmNotExist) {
		t.Fatalf("missing want %q, got %v", msgEquipArmNotExist, err)
	}
	// tieid=0（10001）→ "当前装备无法进行升级！"
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := s.DoUpgradeArmor(ctx, f.UID, sid, false); err == nil || !strings.Contains(err.Error(), msgArmorCannotUpgrade) {
		t.Fatalf("tieid0 want %q, got %v", msgArmorCannotUpgrade, err)
	}
	// tieid=12003（12003）但材料不足 → "您的升级材料不足，无法升级装备！"
	sid2 := insertArmor(t, f, 12003, 2000, 200, 200)
	if _, err := s.DoUpgradeArmor(ctx, f.UID, sid2, false); err == nil || !strings.Contains(err.Error(), msgUpgradeMaterialNotEnough) {
		t.Fatalf("material want %q, got %v", msgUpgradeMaterialNotEnough, err)
	}
	// 材料齐但保护符不足 → "您的升级保护符不足，无法升级装备！"
	setGoods(t, f, 12158, 5)
	setGoods(t, f, 12160, 5)
	if _, err := s.DoUpgradeArmor(ctx, f.UID, sid2, true); err == nil || !strings.Contains(err.Error(), msgUpgradeProtectNotEnough) {
		t.Fatalf("protect want %q, got %v", msgUpgradeProtectNotEnough, err)
	}
}

func TestDoUpgradeArmorOutcomes(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 循环至成功（12003 成功率 20%）——断言成功文案与材料扣减
	succ := false
	for i := 0; i < 300 && !succ; i++ {
		sid := insertArmor(t, f, 12003, 2000, 200, 200)
		setGoods(t, f, 12158, 1)
		setGoods(t, f, 12160, 1)
		res, err := s.DoUpgradeArmor(ctx, f.UID, sid, false)
		if err != nil {
			t.Fatalf("upgrade iter %d: %v", i, err)
		}
		if !strings.Contains(res.Msg, "升级成功！恭喜您获得") {
			continue
		}
		if !res.IsSucc {
			t.Fatalf("msg succ but IsSucc=false")
		}
		if got := goods(t, f, 12158); got != 0 {
			t.Fatalf("12158 want consumed 0, got %d", got)
		}
		if res.GoodStr != "12158,0,12160,0,12157,0" {
			t.Fatalf("goodStr=%q", res.GoodStr)
		}
		succ = true
	}
	if !succ {
		t.Fatalf("never succeeded in 300 tries (rate 20%%)")
	}

	// 循环至失败（保护符 on）——断言失败文案 + 装备未损失
	failed := false
	for i := 0; i < 300 && !failed; i++ {
		sid := insertArmor(t, f, 12003, 2000, 200, 200)
		setGoods(t, f, 12158, 1)
		setGoods(t, f, 12160, 1)
		setGoods(t, f, 12157, 1)
		res, err := s.DoUpgradeArmor(ctx, f.UID, sid, true)
		if err != nil {
			t.Fatalf("upgrade(protect) iter %d: %v", i, err)
		}
		if res.IsSucc {
			continue
		}
		if res.Msg != msgUpgradeFail1 {
			t.Fatalf("fail msg want %q got %q", msgUpgradeFail1, res.Msg)
		}
		n, err := f.DB.FetchCellInt64(ctx, "select count(*) from user_armors where sid=?", sid)
		if err != nil {
			t.Fatalf("count armor: %v", err)
		}
		if n != 1 {
			t.Fatalf("protected armor should survive, rows=%d", n)
		}
		failed = true
	}
	if !failed {
		t.Fatalf("never failed in 300 tries (rate 20%%)")
	}
}

func TestLoadEmbedPearlByArmor(t *testing.T) {
	s, f := newSvc(t)
	out, err := s.LoadEmbedPearlByArmor(context.Background(), f.UID, "12178,0,999999")
	if err != nil {
		t.Fatalf("LoadEmbedPearlByArmor: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("len want 3 got %d", len(out))
	}
	if m, ok := out[0].(map[string]any); !ok || model.Int(m, "gid") != 12178 {
		t.Fatalf("out[0] want gid12178, got %#v", out[0])
	}
	if out[1] != 0 {
		t.Fatalf("out[1] want 0, got %#v", out[1])
	}
	if out[2] != nil {
		t.Fatalf("out[2] want nil, got %#v", out[2])
	}
}

func TestLoadUnActiveHorseArmor(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	active := insertArmor(t, f, 53016, 1800, 180, 180) // 未激活坐骑
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_holes='0,0,0,0,0' where sid=?", insertArmor(t, f, 53040, 2600, 260, 260)); err != nil {
		t.Fatalf("set holes: %v", err)
	}
	insertArmor(t, f, 10001, 1000, 100, 100) // 非坐骑，排除

	out, err := s.LoadUnActiveHorseArmor(ctx, f.UID)
	if err != nil {
		t.Fatalf("LoadUnActiveHorseArmor: %v", err)
	}
	if len(out) != 1 || out[0].SID != active {
		t.Fatalf("unactive horse want [%d], got %+v", active, out)
	}
}
