//go:build integration

package pk

import (
	"context"
	"math/rand"
	"testing"

	"rxsg/backend/internal/testutil"
)

// pk_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：PKDamage 边界 / isTrigger 语义 / battleComute 四类触发倍率与 flag/attack/resist /
//   startUserPK 速度先手·出局·回填·winer·report / sendUserReward（掉落分支 + 首通怪癖）/
//   checkFirstPass 名次占用怪癖 / checkUserPassLevel·checkUserLevel 门槛文案 /
//   getOneBattleRet 完整一关 / getPkFirstReward time>1 怪癖 / loadCampaignInitData / regetCampaignMaxData /
//   loadPKRewardRank / buyJunlingFunc / getAllHeroByUid。

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

func addHero(t *testing.T, f *testutil.Fixture, heroType, level, bravery int) int {
	t.Helper()
	hid, err := f.DB.Insert(context.Background(), `insert into heroes
		(user_id,city_id,name,sex,face,state,level,hero_type,command_base,affairs_base,bravery_base,wisdom_base)
		values (?,?,?,1,95,0,?,?,50,50,?,50)`, f.UID, f.CID, "pkhero", level, heroType, bravery)
	if err != nil {
		t.Fatalf("insert hero: %v", err)
	}
	return int(hid)
}

func addBlood(t *testing.T, f *testutil.Fixture, hid, force int) {
	t.Helper()
	ex(t, f, "insert into hero_blood (hero_id,`force`,force_max,energy,energy_max) values (?,?,?,0,0) on duplicate key update `force`=?", hid, force, force, force)
}

func setGoods(t *testing.T, f *testutil.Fixture, gid int, count int64) {
	t.Helper()
	ex(t, f, "insert into user_goods (user_id,gid,`count`) values (?,?,?) on duplicate key update `count`=?", f.UID, gid, count, count)
}

func setMoney(t *testing.T, f *testutil.Fixture, money int64) {
	t.Helper()
	ex(t, f, "update users set money=? where id=?", money, f.UID)
}

// ── 纯逻辑：PKDamage / isTrigger / battleComute / startUserPK ─────────────

func TestPKDamageBoundaries(t *testing.T) {
	if got := PKDamage(100, 0); got != 100 {
		t.Fatalf("PKDamage(100,0)=%d want 100", got)
	}
	if got := PKDamage(1000, 0); got != 1000 {
		t.Fatalf("PKDamage(1000,0)=%d want 1000", got)
	}
	if got := PKDamage(2000, 2000); got != 1000 {
		t.Fatalf("PKDamage(2000,2000)=%d want 1000（1-1/2）", got)
	}
	if got := PKDamage(1000, 1000); got != 666 {
		t.Fatalf("PKDamage(1000,1000)=%d want 666（1000*(1-1/3)）", got)
	}
	// 极大防御 → 伤害趋近 0
	if got := PKDamage(100, 1e9); got != 0 {
		t.Fatalf("PKDamage(100,1e9)=%d want 0", got)
	}
	// 攻击 0 → 0
	if got := PKDamage(0, 500); got != 0 {
		t.Fatalf("PKDamage(0,500)=%d want 0", got)
	}
}

func TestIsTriggerSemantics(t *testing.T) {
	svc, _ := newSvc(t)
	// value>=rand 语义：value=20000 恒真（rand∈[1,20000]）
	for i := 0; i < 500; i++ {
		if !svc.isTrigger(20000) {
			t.Fatal("isTrigger(20000) 应恒真")
		}
	}
	// value=0 恒假（rand>=1）
	for i := 0; i < 500; i++ {
		if svc.isTrigger(0) {
			t.Fatal("isTrigger(0) 应恒假")
		}
	}
	// value=10000 统计上约 50%
	trueCnt := 0
	svc.SetSeed(1)
	for i := 0; i < 20000; i++ {
		if svc.isTrigger(10000) {
			trueCnt++
		}
	}
	if trueCnt < 9000 || trueCnt > 11000 {
		t.Fatalf("isTrigger(10000) 命中 %d/20000 偏离 ~50%%", trueCnt)
	}
}

// heroFor 构造 battleComute/startUserPK 所需的最小 hero map。
func heroFor(hid, energy, bravery, wisdom, command, affair, speed, standIndex int) map[string]any {
	return map[string]any{
		"hid": hid, "sex": 1, "energy": energy, "bravery": bravery, "wisdom": wisdom,
		"command": command, "affair": affair, "speed": speed, "standIndex": standIndex,
	}
}

func TestBattleComuteTriggers(t *testing.T) {
	svc, _ := newSvc(t)

	base := func() (map[string]any, map[string]any) {
		attacker := map[string]any{
			"hid": 1, "sex": 1, "attackValue": 1000, "baoji": 0, "poji": 0, "standIndex": 11,
			"blood": 100000,
		}
		resister := map[string]any{
			"hid": 2, "sex": 2, "defenceValue": 1000, "shanbi": 0, "gedang": 0, "standIndex": 21,
			"blood": 100000,
		}
		return attacker, resister
	}

	// 普通：无触发 → damage=666，flag/attack/resist=0
	a, r := base()
	res := svc.battleComute(a, r, 7)
	if res["flag"].(int) != 0 || res["attack"].(int) != 0 || res["resist"].(int) != 0 {
		t.Fatalf("普通分支 flag/attack/resist=%v/%v/%v want 0/0/0", res["flag"], res["attack"], res["resist"])
	}
	if res["damage"].(int) != 666 {
		t.Fatalf("普通 damage=%d want 666", res["damage"])
	}
	if res["blood"].(int) != 100000-666 {
		t.Fatalf("普通 blood=%d want %d", res["blood"], 100000-666)
	}
	if res["battleId"].(int) != 7 || res["attackhid"].(int) != 1 || res["resisthid"].(int) != 2 {
		t.Fatalf("battleId/hid 字段错误: %v", res)
	}

	// 破击（poji）：attack*2 → damage=1333，flag=1,attack=2,resist=0
	a, r = base()
	a["poji"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["attack"].(int) != 2 || res["resist"].(int) != 0 || res["flag"].(int) != 1 {
		t.Fatalf("破击 flag/attack/resist=%v/%v/%v want 1/2/0", res["flag"], res["attack"], res["resist"])
	}
	if res["damage"].(int) != 1333 {
		t.Fatalf("破击 damage=%d want 1333", res["damage"])
	}

	// 暴击（baoji）：attack*1.5 → damage=1000，flag=1,attack=1,resist=0
	a, r = base()
	a["baoji"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["attack"].(int) != 1 || res["resist"].(int) != 0 || res["damage"].(int) != 1000 {
		t.Fatalf("暴击 attack/resist/damage=%v/%v/%v want 1/0/1000", res["attack"], res["resist"], res["damage"])
	}

	// 格挡（gedang）：attack*0.5 → damage=333，flag=1,attack=0,resist=1
	a, r = base()
	r["gedang"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["attack"].(int) != 0 || res["resist"].(int) != 1 || res["damage"].(int) != 333 {
		t.Fatalf("格挡 attack/resist/damage=%v/%v/%v want 0/1/333", res["attack"], res["resist"], res["damage"])
	}

	// 闪避（shanbi）：attack 置 0 → damage=max(1,0)=1，flag=1,attack=0,resist=2
	a, r = base()
	r["shanbi"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["attack"].(int) != 0 || res["resist"].(int) != 2 || res["damage"].(int) != 1 {
		t.Fatalf("闪避 attack/resist/damage=%v/%v/%v want 0/2/1", res["attack"], res["resist"], res["damage"])
	}

	// 破击压制暴击（互斥）：poji 与 baoji 同时满足 → 只有 poji 生效
	a, r = base()
	a["poji"] = 20000
	a["baoji"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["attack"].(int) != 2 {
		t.Fatalf("poji+baoji 应只取破击 attack=2，got %v", res["attack"])
	}

	// 闪避压制格挡（互斥）：shanbi 与 gedang 同时满足 → 只有 shanbi 生效
	a, r = base()
	r["shanbi"] = 20000
	r["gedang"] = 20000
	res = svc.battleComute(a, r, 1)
	if res["resist"].(int) != 2 {
		t.Fatalf("shanbi+gedang 应只取闪避 resist=2，got %v", res["resist"])
	}
}

func TestStartUserPK3v3(t *testing.T) {
	svc, _ := newSvc(t)
	strong := func(hid int, stand int) map[string]any { return heroFor(hid, 100, 300, 100, 100, 100, 0, stand) }
	weak := func(hid int, stand int) map[string]any { return heroFor(hid, 10, 10, 10, 10, 10, 0, stand) }

	// 3v3 攻方全胜：3 组对战、winer=1
	rep := svc.startUserPK(
		[]map[string]any{strong(1, 11), strong(2, 12), strong(3, 13)},
		[]map[string]any{weak(4, 21), weak(5, 22), weak(6, 23)})
	if rep["winer"].(int) != 1 {
		t.Fatalf("winer=%v want 1", rep["winer"])
	}
	report := rep["report"].([][]map[string]any)
	if len(report) != 3 {
		t.Fatalf("report 组数=%d want 3", len(report))
	}
	if rep["endflag"].(int) != 1 {
		t.Fatalf("endflag=%v want 1", rep["endflag"])
	}

	// 3v3 攻方全败：winer=0，攻击方将全部出局
	rep = svc.startUserPK(
		[]map[string]any{weak(1, 11), weak(2, 12), weak(3, 13)},
		[]map[string]any{strong(4, 21), strong(5, 22), strong(6, 23)})
	if rep["winer"].(int) != 0 {
		t.Fatalf("攻方全败 winer=%v want 0", rep["winer"])
	}

	// 速度先手：相等 → 攻方先手（attackhid=攻方 hid）
	rep = svc.startUserPK([]map[string]any{strong(11, 11)}, []map[string]any{weak(21, 21)})
	r0 := rep["report"].([][]map[string]any)[0][0]
	if r0["attackhid"].(int) != 11 {
		t.Fatalf("速度相等时先手应为攻方(11)，got %v", r0["attackhid"])
	}
	// 守方更快 → 守方先手（attackhid=守方 hid）
	fastResist := weak(21, 21)
	fastResist["speed"] = 5
	rep = svc.startUserPK([]map[string]any{strong(11, 11)}, []map[string]any{fastResist})
	r0 = rep["report"].([][]map[string]any)[0][0]
	if r0["attackhid"].(int) != 21 {
		t.Fatalf("守方更快时先手应为守方(21)，got %v", r0["attackhid"])
	}

	// 回填/续战：单个强将连续击杀两名弱者（胜者 array_unshift 回队）→ 2 组、winer=1
	rep = svc.startUserPK([]map[string]any{strong(1, 11)}, []map[string]any{weak(2, 21), weak(3, 22)})
	if rep["winer"].(int) != 1 {
		t.Fatalf("回填续战 winer=%v want 1", rep["winer"])
	}
	if len(rep["report"].([][]map[string]any)) != 2 {
		t.Fatalf("回填续战 report 组数=%v want 2", len(rep["report"].([][]map[string]any)))
	}
}

// ── sendUserReward / checkFirstPass ──────────────────────────────────────

// seedWithFirstNum 找一个使首次 mt_rand(1,10) 命中给定值的种子。
func seedWithFirstNum(want int) int64 {
	for s := int64(0); s < 100000; s++ {
		if rand.New(rand.NewSource(s)).Intn(10)+1 == want {
			return s
		}
	}
	return 0
}

func TestSendUserRewardDropBranch(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	ex(t, f, "insert into sys_pk_user (uid,normal,special) values (?,1,1)", f.UID)

	// num==3 → 掉征战奖品 gid=getPkGid(1,0)=18010；passId=1 → passBattleId=0，不触发首通
	svc.SetSeed(seedWithFirstNum(3))
	ret, err := svc.sendUserReward(ctx, f.UID, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 1 {
		t.Fatalf("num==3 应掉落 1 件，got %d", len(ret))
	}
	drop := ret[0].(map[string]any)
	if drop["flag"].(int) != 0 || drop["count"].(int) != 1 {
		t.Fatalf("掉落结构 flag/count=%v/%v want 0/1", drop["flag"], drop["count"])
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=18010", f.UID); g != 1 {
		t.Fatalf("征战奖品 18010=%d want 1", g)
	}
	// 进度推进：newId=1*100+1=101 > 1
	if n := cell(t, f, "select normal from sys_pk_user where uid=?", f.UID); n != 101 {
		t.Fatalf("normal=%d want 101", n)
	}

	// num!=3 → 不掉落
	svc2, f2 := newSvc(t)
	ctx2 := context.Background()
	ex(t, f2, "insert into sys_pk_user (uid,normal,special) values (?,1,1)", f2.UID)
	svc2.SetSeed(seedWithFirstNum(1))
	ret2, err := svc2.sendUserReward(ctx2, f2.UID, 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret2) != 0 {
		t.Fatalf("num!=3 不应掉落，got %v", ret2)
	}
	if n := cell(t, f2, "select count(*) from user_goods where user_id=? and gid=18010", f2.UID); n != 0 {
		t.Fatalf("num!=3 不应有 18010，got %d", n)
	}
}

func TestSendUserRewardFirstPass(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	// 已通过 battle1 level1 → normal=101；再通 level2(最后一关) 触发首通奖励
	ex(t, f, "insert into sys_pk_user (uid,normal,special) values (?,101,1)", f.UID)
	svc.SetSeed(seedWithFirstNum(1)) // 保证 num!=3，不干扰奖励金额断言

	if _, err := svc.sendUserReward(ctx, f.UID, 1, 2, 0); err != nil {
		t.Fatal(err)
	}
	// cfg_pk_reward：type0 道具 18010*2、type0 装备 91101*1
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=18010", f.UID); g != 2 {
		t.Fatalf("首通道具 18010=%d want 2", g)
	}
	if a := cell(t, f, "select count(*) from user_armors where user_id=? and armorid=91101", f.UID); a != 1 {
		t.Fatalf("首通装备 91101=%d want 1", a)
	}
	// checkFirstPass：rank1 位被本 uid 占用
	if u := cell(t, f, "select uid from cfg_pk_first where battleid=1 and type=0 and rankid=1"); u != int64(f.UID) {
		t.Fatalf("首通 rank1 uid=%d want %d", u, f.UID)
	}
	// 进度更新为 102
	if n := cell(t, f, "select normal from sys_pk_user where uid=?", f.UID); n != 102 {
		t.Fatalf("normal=%d want 102", n)
	}
}

// TestCheckFirstPassOrderQuirk 原版怪癖：同一 uid 不重复占位（rank1→2→3 顺序 + uid 去重）。
func TestCheckFirstPassOrderQuirk(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	// 用独立 battleid=99 隔离，避免污染共享配置
	ex(t, f, "insert into cfg_pk_first (battleid,type,rankid,uid,passtime,reward,time) values (99,0,1,0,0,'1,0,18010,1',0),(99,0,2,0,0,'1,0,18010,1',0),(99,0,3,0,0,'1,0,18010,1',0)")
	defer ex(t, f, "delete from cfg_pk_first where battleid=99")

	if err := svc.checkFirstPass(ctx, f.UID, 99, 0); err != nil {
		t.Fatal(err)
	}
	if u := cell(t, f, "select uid from cfg_pk_first where battleid=99 and rankid=1"); u != int64(f.UID) {
		t.Fatalf("rank1 uid=%d want %d", u, f.UID)
	}
	// 再次调用：rank1 已占，rank2/3 因 uid==uid1 被跳过 → 仍为空缺
	if err := svc.checkFirstPass(ctx, f.UID, 99, 0); err != nil {
		t.Fatal(err)
	}
	if u := cell(t, f, "select uid from cfg_pk_first where battleid=99 and rankid=2"); u != 0 {
		t.Fatalf("同一 uid 不应占 rank2，got %d", u)
	}
	// rank1 换成他人 → 本 uid 可占 rank2
	ex(t, f, "update cfg_pk_first set uid=888888 where battleid=99 and rankid=1")
	if err := svc.checkFirstPass(ctx, f.UID, 99, 0); err != nil {
		t.Fatal(err)
	}
	if u := cell(t, f, "select uid from cfg_pk_first where battleid=99 and rankid=2"); u != int64(f.UID) {
		t.Fatalf("rank1 被他人占据后本 uid 应占 rank2，got %d", u)
	}
}

// ── 门槛校验 ──────────────────────────────────────────────────────────────

func TestCheckUserPassLevel(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// 无 sys_pk_user → passId=0 → battleId==passBattleId+1，仅允许 level<=1
	if err := svc.checkUserPassLevel(ctx, f.UID, 1, 2, 0); err == nil || err.Error() != msgCommandExc {
		t.Fatalf("越级 want command_exception, got %v", err)
	}
	if err := svc.checkUserPassLevel(ctx, f.UID, 1, 1, 0); err != nil {
		t.Fatalf("首关 level1 应允许，got %v", err)
	}
	// 跨两关以上 → 异常
	if err := svc.checkUserPassLevel(ctx, f.UID, 5, 1, 0); err == nil || err.Error() != msgCommandExc {
		t.Fatalf("跳关 want command_exception, got %v", err)
	}
	// normal=101 → 同战役内 level<=tmpLevel+1(=2)
	ex(t, f, "insert into sys_pk_user (uid,normal,special) values (?,101,1)", f.UID)
	if err := svc.checkUserPassLevel(ctx, f.UID, 1, 2, 0); err != nil {
		t.Fatalf("normal=101 时 level2 应允许，got %v", err)
	}
	if err := svc.checkUserPassLevel(ctx, f.UID, 1, 3, 0); err == nil || err.Error() != msgCommandExc {
		t.Fatalf("normal=101 时 level3 越级 want command_exception, got %v", err)
	}
}

func TestCheckUserLevel(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// flag=0：无君主将 → heroLevel=0 < hero_level(1) → hero_level_low
	if err := svc.checkUserLevel(ctx, f.UID, 1, 0); err == nil || err.Error() != msgHeroLevelLow {
		t.Fatalf("无君主将 want hero_level_low, got %v", err)
	}
	addHero(t, f, 1000, 5, 50) // 君主将 level5
	if err := svc.checkUserLevel(ctx, f.UID, 1, 0); err != nil {
		t.Fatalf("君主将 level5 应通过，got %v", err)
	}
	// flag=1：sys_user_level 无表 → userLevel=0；battle1.user_level=0 → 通过
	if err := svc.checkUserLevel(ctx, f.UID, 1, 1); err != nil {
		t.Fatalf("user_level=0 应通过，got %v", err)
	}
	// 造一个 user_level=5 的战役 → 修为不足
	ex(t, f, "insert into cfg_pk_battle (id,hero_level,user_level,battlename,description) values (99,1,5,'测试','测试')")
	defer ex(t, f, "delete from cfg_pk_battle where id=99")
	if err := svc.checkUserLevel(ctx, f.UID, 99, 1); err == nil || err.Error() != msgUserLevelLow {
		t.Fatalf("修为不足 want user_level_low, got %v", err)
	}
}

// ── getOneBattleRet 完整一关 ─────────────────────────────────────────────

func TestGetOneBattleRetFullRun(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// 君主将 + 第三将 + 血条
	addHero(t, f, 1000, 5, 50)
	hid3 := addHero(t, f, 1, 10, 300)
	addBlood(t, f, f.HID1, 100)
	addBlood(t, f, f.HID2, 100)
	addBlood(t, f, hid3, 100)
	setGoods(t, f, 19200, 3) // 军令

	ret, err := svc.getOneBattleRet(ctx, f.UID, 1, 0, 1, []int{f.HID1, f.HID2, hid3})
	if err != nil {
		t.Fatalf("getOneBattleRet: %v", err)
	}
	if len(ret) != 1 {
		t.Fatalf("返回元素=%d want 1", len(ret))
	}
	battle := ret[0].(map[string]any)
	if battle["winer"].(int) != 1 {
		t.Fatalf("winer=%v want 1（玩家应胜弱 NPC）", battle["winer"])
	}
	if _, ok := battle["reward"]; !ok {
		t.Fatalf("战斗结果缺 reward 字段")
	}
	report := battle["report"].([][]map[string]any)
	if len(report) != 3 {
		t.Fatalf("report 组数=%d want 3（3v3 三组对战）", len(report))
	}
	if hid := report[0][0]["attackhid"].(int); hid != f.HID1 {
		t.Fatalf("先手应为攻方首将 %d，got %d", f.HID1, hid)
	}
	// 扣 1 军令：3 → 2
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=19200", f.UID); g != 2 {
		t.Fatalf("军令=%d want 2", g)
	}
	// 进度推进到 101
	if n := cell(t, f, "select normal from sys_pk_user where uid=?", f.UID); n != 101 {
		t.Fatalf("normal=%d want 101", n)
	}

	// 无军令 → 报"物品不够，请先购买"
	setGoods(t, f, 19200, 0)
	if _, err := svc.getOneBattleRet(ctx, f.UID, 1, 0, 1, []int{f.HID1, f.HID2, hid3}); err == nil || err.Error() != msgNoAdvLijianfu {
		t.Fatalf("无军令 want 物品不够，got %v", err)
	}
}

// ── getPkFirstReward / loadCampaignInitData / reget / buyJunling / getAllHeroByUid ──

func TestGetPkFirstRewardQuirk(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	ex(t, f, "update cfg_pk_first set uid=?, passtime=unix_timestamp(), time=0 where battleid=1 and type=0 and rankid=1", f.UID)
	// 该行 cleanup 会按 uid 复位，无需额外清理

	ret, err := svc.getPkFirstReward(ctx, f.UID, 1, 0, 1)
	if err != nil {
		t.Fatalf("getPkFirstReward: %v", err)
	}
	if len(ret) != 1 {
		t.Fatalf("返回元素=%d want 1", len(ret))
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=18010", f.UID); g != 2 {
		t.Fatalf("榜首奖励 18010=%d want 2", g)
	}
	// time 被写入 → 二次领取被拒（原版怪癖 time>1）
	if tm := cell(t, f, "select time from cfg_pk_first where battleid=1 and type=0 and rankid=1"); tm <= 1 {
		t.Fatalf("time=%d want >1", tm)
	}
	if _, err := svc.getPkFirstReward(ctx, f.UID, 1, 0, 1); err == nil || err.Error() != msgHasGetReward {
		t.Fatalf("二次领取 want has_get_reward, got %v", err)
	}
	// 未占用榜位 → invalid_param
	if _, err := svc.getPkFirstReward(ctx, f.UID, 1, 0, 3); err == nil || err.Error() != msgInvalidParam {
		t.Fatalf("未占位 want 参数错误, got %v", err)
	}
}

func TestLoadCampaignInitData(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ret, err := svc.loadCampaignInitData(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 6 {
		t.Fatalf("loadCampaignInitData 返回 %d 元素 want 6", len(ret))
	}
	levelInfo := ret[0].(map[string]any)
	if levelInfo["normal"] == nil || levelInfo["special"] == nil {
		t.Fatalf("levelInfo=%v want normal/special", levelInfo)
	}
	if n := len(ret[1].([]map[string]any)); n != 6 {
		t.Fatalf("normalHeroes=%d want 6（2 关 * 3 站位）", n)
	}
	if n := len(ret[2].([]map[string]any)); n != 6 {
		t.Fatalf("specialHeroes=%d want 6", n)
	}
	if n := len(ret[3].([]map[string]any)); n != 2 {
		t.Fatalf("rewards=%d want 2（1 战役 * 2 flag）", n)
	}
	goods := ret[4].(map[string]any)
	if goods["junling"].(int) != 10 { // 首次赠送 10 个军令
		t.Fatalf("junling=%v want 10", goods["junling"])
	}
	if n := len(ret[5].([]map[string]any)); n != 1 {
		t.Fatalf("battleDesc=%d want 1", n)
	}
	// 首次已入库 10 军令
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=19200", f.UID); g != 10 {
		t.Fatalf("user_goods 19200=%d want 10", g)
	}
}

func TestRegetCampaignMaxDataAndRank(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	ex(t, f, "insert into sys_pk_user (uid,normal,special) values (?,101,1)", f.UID)

	ret, err := svc.regetCampaignMaxData(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 3 {
		t.Fatalf("regetCampaignMaxData 返回 %d 元素 want 3", len(ret))
	}
	if m := ret[0].(map[string]any); m == nil {
		t.Fatalf("userBattleInfo 不应为空")
	}

	rank, err := svc.loadPKRewardRank(ctx, f.UID, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	rows := rank[0].([]map[string]any)
	if len(rows) != 3 {
		t.Fatalf("首通榜=%d want 3", len(rows))
	}
	if rows[0]["rewardType"] == nil {
		t.Fatalf("rank1 应解析出 rewardType，got %v", rows[0])
	}
	if rows[0]["rewardCount"] != "2" {
		t.Fatalf("rank1 rewardCount=%v want 2", rows[0]["rewardCount"])
	}
}

func TestBuyJunlingFunc(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	setMoney(t, f, 100)

	ret, err := svc.buyJunlingFunc(ctx, f.UID, 0, 10)
	if err != nil {
		t.Fatalf("buyJunlingFunc: %v", err)
	}
	if len(ret) != 3 || ret[0].(int) != 0 || ret[1].(int) != 10 || ret[2].(int64) != 50 {
		t.Fatalf("购买结果=%v want [0,10,50]", ret)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 50 {
		t.Fatalf("money=%d want 50（扣 10*5）", m)
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=19200", f.UID); g != 10 {
		t.Fatalf("军令=%d want 10", g)
	}

	// 超过上限（100-10=90）
	if _, err := svc.buyJunlingFunc(ctx, f.UID, 0, 200); err == nil {
		t.Fatal("超上限 want error(junling_max_count)")
	}
	// 非元宝支付
	if _, err := svc.buyJunlingFunc(ctx, f.UID, 1, 1); err == nil || err.Error() != msgInvalidPayType {
		t.Fatalf("非元宝支付 want 空文案, got %v", err)
	}
	// 数量非法
	if _, err := svc.buyJunlingFunc(ctx, f.UID, 0, 0); err == nil || err.Error() != msgInvalidAmount {
		t.Fatalf("数量 0 want invalid_amount, got %v", err)
	}
	// 元宝不足
	setMoney(t, f, 0)
	if _, err := svc.buyJunlingFunc(ctx, f.UID, 0, 1); err == nil || err.Error() != msgNoEnoughYuan {
		t.Fatalf("元宝不足 want no_enough_YuanBao, got %v", err)
	}
}

func TestGetAllHeroByUid(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	addHero(t, f, 1000, 5, 50)
	hid3 := addHero(t, f, 1, 10, 300)
	addBlood(t, f, f.HID1, 100)
	addBlood(t, f, f.HID2, 100)
	addBlood(t, f, hid3, 100)

	// page=1 且未选 → [tenHeros, page, kingHero]
	ret, err := svc.getAllHeroByUid(ctx, f.UID, nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 3 {
		t.Fatalf("page1 未选返回 %d 元素 want 3", len(ret))
	}
	if ret[1].(int) != 1 {
		t.Fatalf("page=%v want 1", ret[1])
	}
	if king := ret[2]; king == nil {
		t.Fatalf("kingHero 不应为空")
	}
	if n := len(ret[0].([]map[string]any)); n != 3 { // 3 名非君主将（HID1,HID2,hid3）
		t.Fatalf("tenHeros=%d want 3", n)
	}

	// page>1 → [tenHeros, page]
	ret, err = svc.getAllHeroByUid(ctx, f.UID, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 2 {
		t.Fatalf("page2 返回 %d 元素 want 2", len(ret))
	}
	// page<=0 → 参数错误
	if _, err := svc.getAllHeroByUid(ctx, f.UID, nil, 0); err == nil || err.Error() != msgInvalidParam {
		t.Fatalf("page=0 want 参数错误, got %v", err)
	}
}
