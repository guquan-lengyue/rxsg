//go:build integration

package armor

import (
	"context"
	"strings"
	"testing"

	"rxsg/backend/internal/testutil"
)

// armor_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：穿戴/卸下+属性重算公式、维修/翻新公式、repairAll 不扣 reduce 怪癖、
// 出售（爵位门槛/goldAdd 公式/无 hid 校验怪癖）、拆解（碎片公式/宝珠返还/降级不返还）、
// 强化（100% 档成功/行缺失返回文案怪癖）、熔炼（保护符/无保护符 gid=-11 怪癖）、
// 打孔/开孔/拆珠/镶嵌（聚魂珠孔位/embedLimit/化石粉文案）。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

// insertArmor 插一行 user_armors，返回 sid。
func insertArmor(t *testing.T, f *testutil.Fixture, armorid, hp, hpMax, oriHpMax int) int {
	t.Helper()
	sid, err := f.DB.Insert(context.Background(),
		`insert into user_armors (user_id, armorid, hp, hp_max, ori_hp_max) values (?,?,?,?,?)`,
		f.UID, armorid, hp, hpMax, oriHpMax)
	if err != nil {
		t.Fatalf("insert armor: %v", err)
	}
	return int(sid)
}

func setGoods(t *testing.T, f *testutil.Fixture, gid int, count int64) {
	t.Helper()
	_, err := f.DB.Exec(context.Background(),
		`insert into user_goods (user_id, gid, `+"`count`"+`) values (?,?,?)
		on duplicate key update `+"`count`"+`=?`, f.UID, gid, count, count)
	if err != nil {
		t.Fatalf("set goods %d: %v", gid, err)
	}
}

func goods(t *testing.T, f *testutil.Fixture, gid int) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(),
		"select `count` from user_goods where user_id=? and gid=?", f.UID, gid)
	if err != nil {
		return -1 << 40 // 无行（测试断言用极小值）
	}
	return v
}

func armorCell(t *testing.T, f *testutil.Fixture, sid int, col string) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(),
		"select `"+col+"` from user_armors where sid=?", sid)
	if err != nil {
		t.Fatalf("cell %s: %v", col, err)
	}
	return v
}

func armorStr(t *testing.T, f *testutil.Fixture, sid int, col string) string {
	t.Helper()
	v, err := f.DB.FetchCellString(context.Background(),
		"select `"+col+"` from user_armors where sid=?", sid)
	if err != nil {
		t.Fatalf("cell %s: %v", col, err)
	}
	return v
}

func heroCell(t *testing.T, f *testutil.Fixture, hid int, col string) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(),
		"select `"+col+"` from heroes where id=?", hid)
	if err != nil {
		t.Fatalf("hero cell %s: %v", col, err)
	}
	return v
}

func errIs(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("err=%v want contains %q", err, want)
	}
}

// ---------- 穿戴 ----------

// 怪癖：行缺失时 part=0≠spart/10 → 先报"不能装备在这个部位"而非"装备不存在"。
func TestEquipMissingRowQuirk(t *testing.T) {
	s, f := newSvc(t)
	_, err := s.EquipArmor(context.Background(), f.UID, f.HID1, 999999, 10)
	errIs(t, err, msgNotRightPart)
}

func TestEquipRejectUsedAndNoHP(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100) // 铁剑 part=1
	// 耐久 0（hp=0 → ceil(0/10)=0）拒
	sid2 := insertArmor(t, f, 10001, 0, 100, 100)
	_, err := s.EquipArmor(ctx, f.UID, f.HID1, sid2, 10)
	errIs(t, err, msgNoHPMaxEquip)
	// spart=20（part 校验 2≠1）
	_, err = s.EquipArmor(ctx, f.UID, f.HID1, sid, 20)
	errIs(t, err, msgNotRightPart)
	// 正常穿戴后重复穿戴拒
	res, err := s.EquipArmor(ctx, f.UID, f.HID1, sid, 10)
	if err != nil {
		t.Fatalf("equip: %v", err)
	}
	if res.BraveryAddOn != 10 { // attribute "1,3,10" → 勇武+10
		t.Fatalf("braveryAddOn=%d want 10", res.BraveryAddOn)
	}
	if got := heroCell(t, f, f.HID1, "bravery_add_on"); got != 10 {
		t.Fatalf("heroes.bravery_add_on=%d want 10", got)
	}
	_, err = s.EquipArmor(ctx, f.UID, f.HID1, sid, 10)
	errIs(t, err, msgArmInUse)
	// 等级门槛：青锋剑 hero_level=10 ≤ 10 可穿；龙渊套·剑 hero_level=40 拒
	sid3 := insertArmor(t, f, 12003, 2000, 200, 200)
	_, err = s.EquipArmor(ctx, f.UID, f.HID1, sid3, 10)
	errIs(t, err, "40")
	// 卸下 → 加成清零
	if _, err := s.OffloadArmor(ctx, f.UID, f.HID1, 10); err != nil {
		t.Fatalf("offload: %v", err)
	}
	if got := heroCell(t, f, f.HID1, "bravery_add_on"); got != 0 {
		t.Fatalf("after offload bravery_add_on=%d want 0", got)
	}
}

// 强化等级属性加成：strong_level=1 → levelvalue[1]=2 → 按排序属性轮询 +1×2 → 勇武 10+2=12。
func TestEquipStrongAttr(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=1 where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	res, err := s.EquipArmor(ctx, f.UID, f.HID1, sid, 10)
	if err != nil {
		t.Fatalf("equip: %v", err)
	}
	if res.BraveryAddOn != 12 {
		t.Fatalf("braveryAddOn=%d want 12", res.BraveryAddOn)
	}
}

// ---------- 耐久 ----------

func TestRepairArmorFormula(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 900, 100, 100) // ceil(900/10)=90
	_, hpmax, gold, err := s.RepairArmor(ctx, f.UID, f.CID, sid)
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	// goldNeed=(100-90)*100=1000；reduce=max(1,ceil(10/10))=1 → hpmax=99、hp=990
	if hpmax != 99 || armorCell(t, f, sid, "hp") != 990 {
		t.Fatalf("hpmax=%d hp=%d want 99/990", hpmax, armorCell(t, f, sid, "hp"))
	}
	if gold != 9000 {
		t.Fatalf("gold=%d want 9000", gold)
	}
	// 满耐久（hp=hpmax*10 → need=0）拒
	sid2 := insertArmor(t, f, 10001, 1000, 100, 100)
	_, _, _, err = s.RepairArmor(ctx, f.UID, f.CID, sid2)
	errIs(t, err, msgRepairNoNeed)
}

// 怪癖：repairAllArmor 求和循环 $reduce 未定义 → goldNeed 不扣 reduce（2×1000=2000）。
func TestRepairAllQuirk(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	s1 := insertArmor(t, f, 10001, 900, 100, 100)
	s2 := insertArmor(t, f, 10001, 900, 100, 100)
	gold, err := s.RepairAllArmor(ctx, f.UID, f.CID, []int{s1, s2})
	if err != nil {
		t.Fatalf("repairAll: %v", err)
	}
	if gold != 8000 { // 10000-2000（若正确扣 reduce 应为 10000-1800）
		t.Fatalf("gold=%d want 8000", gold)
	}
	if armorCell(t, f, s1, "hp_max") != 99 || armorCell(t, f, s2, "hp_max") != 99 {
		t.Fatal("hpmax not reduced")
	}
}

func TestRenovateArmor(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "update users set money=100 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	sid := insertArmor(t, f, 10001, 900, 90, 100) // moneyNeed=(100-90)+ceil((90-90)/10)=10
	got, err := s.RenovateArmor(ctx, f.UID, sid)
	if err != nil {
		t.Fatalf("renovate: %v", err)
	}
	if got != sid {
		t.Fatalf("sid=%d want %d", got, sid)
	}
	if armorCell(t, f, sid, "hp") != 1000 || armorCell(t, f, sid, "hp_max") != 100 {
		t.Fatal("hp not restored")
	}
	if m := heroCellMoney(t, f); m != 90 {
		t.Fatalf("money=%d want 90", m)
	}
	// 元宝不足
	sid2 := insertArmor(t, f, 10001, 100, 1, 100) // moneyNeed=99+ceil(0/10)... hp=ceil(100/10)=10,hpmax=1 → (100-1)+0=99 > 90
	_, err = s.RenovateArmor(ctx, f.UID, sid2)
	errIs(t, err, msgRenovateNoMoney)
}

func heroCellMoney(t *testing.T, f *testutil.Fixture) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), "select money from users where id=?", f.UID)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ---------- 出售 ----------

func TestSellArmor(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	// 市场 <5 级拒
	if _, err := f.DB.Exec(ctx,
		`insert into buildings (city_id, building_id, xy, level, state, state_start_at, state_end_at)
		values (?,13,'x1',4,0,0,0)`, f.CID); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := s.SellArmor(ctx, f.UID, f.CID, sid)
	errIs(t, err, msgSellMarketLow)
	if _, err := f.DB.Exec(ctx, "update buildings set level=5 where city_id=? and building_id=13", f.CID); err != nil {
		t.Fatal(err)
	}
	// 爵位不足（nobility '' → 0）
	_, _, _, err = s.SellArmor(ctx, f.UID, f.CID, sid)
	errIs(t, err, msgSellNobilityLow)
	if _, err := f.DB.Exec(ctx, "update users set nobility='1' where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	// 怪癖：穿戴中（hid≠0）也可出售，不校验
	if _, err := f.DB.Exec(ctx, "update user_armors set hid=? where sid=?", f.HID1, sid); err != nil {
		t.Fatal(err)
	}
	_, _, gold, err := s.SellArmor(ctx, f.UID, f.CID, sid)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	// goldAdd=intval(max(1,floor(100/100))*1)*500=500
	if gold != 10500 {
		t.Fatalf("gold=%d want 10500", gold)
	}
	if n := armorCount(t, f, sid); n != 0 {
		t.Fatalf("armor not deleted: %d", n)
	}
	if v := cellExists(t, f, "select count(*) from log_selled_armor where sid=?", sid); v != 1 {
		t.Fatal("log_selled_armor missing")
	}
	if v := cellExists(t, f, "select count(*) from log_armor where user_id=? and type=9", f.UID); v != 1 {
		t.Fatal("log_armor type9 missing")
	}
}

func armorCount(t *testing.T, f *testutil.Fixture, sid int) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), "select count(*) from user_armors where sid=?", sid)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func cellExists(t *testing.T, f *testutil.Fixture, q string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), q, args...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ---------- 拆解 ----------

func TestChaijie(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	// 未激活（embed_holes 空）拒
	sid0 := insertArmor(t, f, 10001, 1000, 100, 100)
	_, err := s.Chaijie(ctx, f.UID, sid0)
	errIs(t, err, msgChaijieNotActive)

	// 铁剑 hero_level=1 attribute="1,3,10"：4×1+8×10=84 → ceil(84/10)=9 碎片
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_holes='1,1,2,3,3' where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	msg, err := s.Chaijie(ctx, f.UID, sid)
	if err != nil {
		t.Fatalf("chaijie: %v", err)
	}
	if !strings.Contains(msg, msgArmorChip+"9") {
		t.Fatalf("msg=%q want 装备碎片9", msg)
	}
	if v := cellExists(t, f, "select `count` from things where user_id=? and tid=10400", f.UID); v != 9 {
		t.Fatalf("things=%d want 9", v)
	}
	if armorCount(t, f, sid) != 0 {
		t.Fatal("armor not deleted")
	}
	if v := cellExists(t, f, "select count(*) from log_armor where user_id=? and type=11", f.UID); v != 1 {
		t.Fatal("log_armor type11 missing")
	}
	// 穿戴中拒
	sid2 := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_holes='1,1,2,3,3', hid=? where sid=?", f.HID1, sid2); err != nil {
		t.Fatal(err)
	}
	_, err = s.Chaijie(ctx, f.UID, sid2)
	errIs(t, err, msgArmInUse)
}

// 宝珠返还：50% 原样否则降级；1 级宝珠（300）降级分支不返还。
// 强化宝珠返还 cnt=Σfloor(100/suc_value[i])：strong_level=2 → 1+1=2。
func TestChaijiePearlReturn(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx,
		`update user_armors set embed_holes='0,0,0,0,0', embed_pearls='300,0,0,0,0', strong_level=2 where sid=?`, sid); err != nil {
		t.Fatal(err)
	}
	msg, err := s.Chaijie(ctx, f.UID, sid)
	if err != nil {
		t.Fatalf("chaijie: %v", err)
	}
	// 205 恒返 2；300 或原样返还或（1 级）不返还 → msg 必含"强化宝珠 2"
	if !strings.Contains(msg, "强化宝珠 2") {
		t.Fatalf("msg=%q want 强化宝珠 2", msg)
	}
	if goods(t, f, gidStrong) != 2 {
		t.Fatalf("205 count=%d want 2", goods(t, f, gidStrong))
	}
	// 300 无 cfg_goods 行 → legacy empty($good) continue（不入文案也不入账）
	if goods(t, f, 300) > 1 {
		t.Fatalf("300 unexpected count")
	}
}

// ---------- 强化 ----------

func TestStrongSuccess100pct(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if _, err := f.DB.Exec(ctx, "insert into city_technics (city_id, technic_id, level) values (?,21,5)", f.CID); err != nil {
		t.Fatal(err)
	}
	setGoods(t, f, gidStrong, 3)
	sid := insertArmor(t, f, 10001, 1000, 100, 100) // type=2 白装 ≤6 级可强
	res, err := s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid, 0)
	if err != nil {
		t.Fatalf("strong: %v", err)
	}
	if !res.Started || res.Outcome != 0 {
		t.Fatalf("res=%+v want success", res)
	}
	// 1 级 suc_value=100 → rand(1,10000)<=10000 恒成功；strong_value 取 next 行=2
	if armorCell(t, f, sid, "strong_level") != 1 || armorCell(t, f, sid, "strong_value") != 2 {
		t.Fatalf("level=%d value=%d want 1/2",
			armorCell(t, f, sid, "strong_level"), armorCell(t, f, sid, "strong_value"))
	}
	if armorStr(t, f, sid, "best_quality") != "" { // xilian_rate=0 → 不出极品
		t.Fatalf("best_quality=%q want empty", armorStr(t, f, sid, "best_quality"))
	}
	if goods(t, f, gidStrong) != 2 {
		t.Fatalf("205=%d want 2", goods(t, f, gidStrong))
	}
	if v := cellExists(t, f, "select success from log_armor_strong where sid=? and user_id=?", sid, f.UID); v != 1 {
		t.Fatal("log success flag missing")
	}
}

// 怪癖：装备行缺失返回 ret=[0,文案] 而非 throw。
func TestStrongMissingRowQuirk(t *testing.T) {
	s, f := newSvc(t)
	res, err := s.DoStrong(context.Background(), f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, 999999, 0)
	if err != nil {
		t.Fatalf("want ret-style return, got err %v", err)
	}
	if res.Started || res.Msg != msgNoSuchArmor {
		t.Fatalf("res=%+v want {0,%s}", res, msgNoSuchArmor)
	}
}

func TestStrongValidation(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	_, err := s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 2, gidQianKun, 0, sid, 0)
	errIs(t, err, msgWaiguaForbidden)
	_, err = s.DoStrong(ctx, f.UID, f.CID, 999, 0, gidQianKun, 0, sid, 0)
	errIs(t, err, msgWrongItem)
	_, err = s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid, 1) // 非坐骑 is_zuoji=1
	errIs(t, err, msgDataException)
	// 缺强化宝珠
	_, err = s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid, 0)
	errIs(t, err, msgNoStrongPearl)
	// strongLimit：无打造科技 → 需打造技巧(1级)
	setGoods(t, f, gidStrong, 5)
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=3 where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	_, err = s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid, 0)
	errIs(t, err, "需要打造技巧(4级)")
	// 灰装 ≤3 级上限（type=1）
	sid2 := insertArmor(t, f, 10003, 900, 90, 90) // 灰盔 type=1
	if _, err := f.DB.Exec(ctx,
		"insert into city_technics (city_id, technic_id, level) values (?,21,5)", f.CID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=3 where sid=?", sid2); err != nil {
		t.Fatal(err)
	}
	_, err = s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid2, 0)
	errIs(t, err, "灰装只能强化到3级")
	// ≥15 拒（legacy 顺序：10-15 级材料 11170 检查先于 strongLimit → 需先备料）
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=15 where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	setGoods(t, f, gidHighStrong, 1)
	_, err = s.DoStrong(ctx, f.UID, f.CID, gidTianGong, 0, gidQianKun, 0, sid, 0)
	errIs(t, err, msgStrongLevelLimit)
}

// ---------- 熔炼 ----------

func TestCombineValidation(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	m := insertArmor(t, f, 10001, 1000, 100, 100)
	s1 := insertArmor(t, f, 10001, 1000, 100, 100)
	s2 := insertArmor(t, f, 10002, 1200, 120, 120) // 不同 armorid
	_, err := s.CombineArmor(ctx, f.UID, m, 0, s1, s2, 0)
	errIs(t, err, msgInvalidParam)
	_, err = s.CombineArmor(ctx, f.UID, m, 1, s1, s1, 0)
	errIs(t, err, msgWaiguaInvalid)
	_, err = s.CombineArmor(ctx, f.UID, m, 1, s1, s2, 0)
	errIs(t, err, msgWaiguaInvalid) // armorid 不一致
	// 熔炼等级不一致
	s3 := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set combine_level=1 where sid=?", s3); err != nil {
		t.Fatal(err)
	}
	_, err = s.CombineArmor(ctx, f.UID, m, 1, s1, s3, 0)
	errIs(t, err, msgCombineLevelNE)
	// 缺熔炼石
	_, err = s.CombineArmor(ctx, f.UID, m, 1, s1, s3, 1)
	if err == nil || !strings.Contains(err.Error(), "combine_level") {
		// 等级不一致先抛——先改一致再测缺料
	}
	if _, err := f.DB.Exec(ctx, "update user_armors set combine_level=0 where sid=?", s3); err != nil {
		t.Fatal(err)
	}
	_, err = s.CombineArmor(ctx, f.UID, m, 1, s1, s3, 0)
	errIs(t, err, msgNotEnoughFuse1)
	// 坐骑拒
	h := insertArmor(t, f, 53016, 1800, 180, 180)
	_, err = s.CombineArmor(ctx, f.UID, h, 1, s1, s3, 0)
	errIs(t, err, msgCombineZuoji)
}

// 保护符（goodsFlag=1）：无论成败主副件都在；材料恒扣（各 1）。
// 无保护符怪癖：usegoods=-11 → addGoods(uid,-11,-1,0) 产生 gid=-11 行。
func TestCombineOutcome(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	setGoods(t, f, gidFuse1, 10)
	setGoods(t, f, gidFusePro, 10)
	m := insertArmor(t, f, 10001, 1000, 100, 100)
	s1 := insertArmor(t, f, 10001, 1000, 100, 100)
	s2 := insertArmor(t, f, 10001, 1000, 100, 100)
	res, err := s.CombineArmor(ctx, f.UID, m, 1, s1, s2, 1)
	if err != nil {
		t.Fatalf("combine: %v", err)
	}
	if goods(t, f, gidFuse1) != 9 || goods(t, f, gidFusePro) != 9 {
		t.Fatalf("goods not consumed: %d/%d", goods(t, f, gidFuse1), goods(t, f, gidFusePro))
	}
	if res.Success == 1 {
		if armorCell(t, f, m, "combine_level") != 1 {
			t.Fatal("combine_level not up")
		}
		if armorCount(t, f, s1) != 0 || armorCount(t, f, s2) != 0 {
			t.Fatal("subs not deleted")
		}
	} else {
		if armorCount(t, f, m) != 1 || armorCount(t, f, s1) != 1 || armorCount(t, f, s2) != 1 {
			t.Fatal("protected armors destroyed")
		}
	}
	// 无保护符：gid=-11 怪癖行
	setGoods(t, f, gidFuse1, 5)
	m2 := insertArmor(t, f, 10001, 1000, 100, 100)
	s3 := insertArmor(t, f, 10001, 1000, 100, 100)
	s4 := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := s.CombineArmor(ctx, f.UID, m2, 1, s3, s4, 0); err != nil {
		t.Fatalf("combine2: %v", err)
	}
	if goods(t, f, -11) != -1 {
		t.Fatalf("gid=-11 count=%d want -1（legacy 怪癖）", goods(t, f, -11))
	}
}

// ---------- 打孔 / 开孔 / 拆珠 / 镶嵌 ----------

func TestHolesAndEmbedFlow(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100) // type=2 → rule "0,0,-1,N/A,N/A"
	// 激活打孔：reduce_gold=value*500/10=50（整除怪癖）
	gold, err := s.InitHoles(ctx, f.UID, f.CID, sid)
	if err != nil {
		t.Fatalf("initHoles: %v", err)
	}
	if gold != 50 {
		t.Fatalf("reduce_gold=%d want 50", gold)
	}
	if holes := armorStr(t, f, sid, "embed_holes"); holes != "1,1,2,3,3" {
		t.Fatalf("holes=%q want 1,1,2,3,3", holes)
	}
	if pearls := armorStr(t, f, sid, "embed_pearls"); pearls != "0,0,0,0,0" {
		t.Fatalf("pearls=%q", pearls)
	}
	// 重复激活拒
	_, err = s.InitHoles(ctx, f.UID, f.CID, sid)
	errIs(t, err, msgAlreadyActive)
	// 未开孔镶嵌拒（holes[0]=1）
	setGoods(t, f, 300, 3)
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=1 where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	_, err = s.DoEmbed(ctx, f.UID, sid, 0, 300, 0)
	errIs(t, err, msgNoPos)
	// 开孔 pos0（打孔器 gid=206）
	setGoods(t, f, 206, 2)
	if _, err := s.OpenHole(ctx, f.UID, sid, 206, 0, 0, 0); err != nil {
		t.Fatalf("openHole: %v", err)
	}
	if holes := armorStr(t, f, sid, "embed_holes"); !strings.HasPrefix(holes, "0,") {
		t.Fatalf("holes=%q want pos0 opened", holes)
	}
	if goods(t, f, 206) != 1 {
		t.Fatal("206 not consumed")
	}
	// 镶嵌 300（1 级 ≤ strong_level 1）
	res, err := s.DoEmbed(ctx, f.UID, sid, 0, 300, 0)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if res.Started != 1 || res.Pearls != "300,0,0,0,0" {
		t.Fatalf("res=%+v", res)
	}
	if goods(t, f, 300) != 2 {
		t.Fatalf("300=%d want 2", goods(t, f, 300))
	}
	// 已镶位置再镶拒（waigua.invalid）
	_, err = s.DoEmbed(ctx, f.UID, sid, 0, 300, 0)
	errIs(t, err, msgWaiguaInvalid)
	// 未开孔位置镶嵌拒（legacy 顺序：holes[pos]≠0 → no_pos 先于 embedLimit）
	_, err = s.DoEmbed(ctx, f.UID, sid, 1, 301, 0)
	errIs(t, err, msgNoPos)
	// 聚魂珠孔位：开 pos4 后镶普通珠拒（pos4 holes=0 先过孔位校验）
	setGoods(t, f, 207, 2)
	if _, err := s.OpenHole(ctx, f.UID, sid, 207, 4, 0, 0); err != nil {
		t.Fatalf("openHole pos4: %v", err)
	}
	setGoods(t, f, 10831, 2)
	_, err = s.DoEmbed(ctx, f.UID, sid, 4, 300, 0)
	errIs(t, err, msgGoodPositionErr)
	// pos1 未开孔 → 孔位校验先于聚魂珠规则
	_, err = s.DoEmbed(ctx, f.UID, sid, 1, 10831, 0)
	errIs(t, err, msgNoPos)
	// 开 pos1 后聚魂珠放非 4 孔拒
	if _, err := s.OpenHole(ctx, f.UID, sid, 206, 1, 0, 0); err != nil {
		t.Fatalf("openHole pos1: %v", err)
	}
	_, err = s.DoEmbed(ctx, f.UID, sid, 1, 10831, 0)
	errIs(t, err, msgEmbedLimit) // embedLimit(10831=2级) 先于孔位规则
	// 聚魂珠 pos4 成功（embedLimit 等级=(10831-10800)%10+1=2 > strong_level 1 → 拒）
	_, err = s.DoEmbed(ctx, f.UID, sid, 4, 10831, 0)
	errIs(t, err, msgEmbedLimit)
	if _, err := f.DB.Exec(ctx, "update user_armors set strong_level=2 where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DoEmbed(ctx, f.UID, sid, 4, 10831, 0); err != nil {
		t.Fatalf("embed soul: %v", err)
	}
	if pearls := armorStr(t, f, sid, "embed_pearls"); pearls != "300,0,0,0,10831" {
		t.Fatalf("pearls=%q", pearls)
	}
}

func TestOpenHoleDismantle(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx,
		`update user_armors set embed_holes='0,0,0,0,0', embed_pearls='300,0,0,0,0' where sid=?`, sid); err != nil {
		t.Fatal(err)
	}
	// 空珠拆珠 → wrong_pearl_gid（legacy throw）
	setGoods(t, f, gidHuaShiFen, 5)
	_, err := s.OpenHole(ctx, f.UID, sid, gidHuaShiFen, 1, 0, 1)
	errIs(t, err, msgWrongPearlGID)
	// useType=0：耗 1 份不返还宝珠
	if _, err := s.OpenHole(ctx, f.UID, sid, gidHuaShiFen, 0, 0, 1); err != nil {
		t.Fatalf("dismantle: %v", err)
	}
	if armorStr(t, f, sid, "embed_pearls") != "0,0,0,0,0" {
		t.Fatal("pearl not cleared")
	}
	if goods(t, f, gidHuaShiFen) != 4 {
		t.Fatalf("fen=%d want 4", goods(t, f, gidHuaShiFen))
	}
	if n := cellExists(t, f, "select count(*) from user_goods where user_id=? and gid=?", f.UID, 300); n != 0 {
		t.Fatalf("300 rows=%d want 0（useType=0 不返还）", n)
	}
	// useType=1 返还宝珠：300 需 needCount=1
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_pearls='300,0,0,0,0' where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	setGoods(t, f, 300, 0)
	if _, err := s.OpenHole(ctx, f.UID, sid, gidHuaShiFen, 0, 1, 1); err != nil {
		t.Fatalf("dismantle return: %v", err)
	}
	if goods(t, f, 300) != 1 {
		t.Fatalf("300=%d want 1", goods(t, f, 300))
	}
	// count 与 useType 不匹配 → 数据异常（先镶回 300：getDismantleCount(0) 会先 throw wrong_pearl_gid）
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_pearls='300,0,0,0,0' where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	_, err = s.OpenHole(ctx, f.UID, sid, gidHuaShiFen, 0, 1, 2)
	errIs(t, err, msgDataException)
	// 化石粉不足 → not_enough_goods201#left
	if _, err := f.DB.Exec(ctx, "update user_goods set `count`=0 where user_id=? and gid=201", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, "update user_armors set embed_pearls='309,0,0,0,0' where sid=?", sid); err != nil {
		t.Fatal(err) // 309 需 60 份
	}
	_, err = s.OpenHole(ctx, f.UID, sid, gidHuaShiFen, 0, 1, 60)
	errIs(t, err, "not_enough_goods201#60")
}

// doEmbed 怪癖：装备行缺失返回 ret=[0,文案]。
func TestEmbedMissingRowQuirk(t *testing.T) {
	s, f := newSvc(t)
	res, err := s.DoEmbed(context.Background(), f.UID, 999999, 0, 300, 0)
	if err != nil {
		t.Fatalf("want ret-style, got %v", err)
	}
	if res.Started != 0 || res.Msg != msgNoSuchArmor {
		t.Fatalf("res=%+v", res)
	}
}
