//go:build integration

package battle

// battle_integration_test.go —— M7 战斗引擎集成测试（真库）。
// 覆盖：纯函数口径、将领加成公式、伤害公式与战报格式、城墙伤害、胜负结算、
// 掠夺/占领、伤兵入城、青囊加成、is_npc。

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"rxsg/backend/internal/db"
	"rxsg/backend/internal/testutil"
)

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

func cellInt(t *testing.T, d *db.DB, q string, args ...any) int64 {
	t.Helper()
	v, err := d.FetchCellInt64(context.Background(), q, args...)
	if err != nil {
		t.Fatalf("cell %q: %v", q, err)
	}
	return v
}

func exec(t *testing.T, d *db.DB, q string, args ...any) {
	t.Helper()
	if _, err := d.Exec(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func jstr(m map[int]int64) string {
	out := map[string]int64{}
	for k, v := range m {
		out[strconv.Itoa(k)] = v
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// ── 纯函数口径 ──────────────────────────────────────────────────────────

func TestPureHelpers(t *testing.T) {
	// phpStr：整数值不带小数点。
	if got := phpStr(1666); got != "1666" {
		t.Fatalf("phpStr(1666)=%q", got)
	}
	// troop2Array / array2troop 往返。
	o := troop2Array("2,1,100,4,50,")
	if o.len() != 2 || o.get(1) != 100 || o.get(4) != 50 {
		t.Fatalf("troop2Array=%v", o.m)
	}
	if got := array2troop(o, false); got != "2,1,100,4,50," {
		t.Fatalf("array2troop=%q", got)
	}
	// def=true 三元组。
	if got := array2troop(o, true); got != "2,1,100,100,4,50,50," {
		t.Fatalf("array2troop def=%q", got)
	}
	// getSoldierCounts。
	if got := getSoldierCounts("2,1,100,4,50,"); got != 150 {
		t.Fatalf("getSoldierCounts=%v", got)
	}
	// defence2Array："N,did,oldcnt,cnt"。
	order, m := defence2Array("2,1,10,8,3,20,15,")
	if len(order) != 2 || m[1].Cnt != 8 || m[3].OldCnt != 20 || m[3].Cnt != 15 {
		t.Fatalf("defence2Array order=%v m=%+v", order, m)
	}
	// checkResource 怪癖："0" 与 "0,0,0,0,0," 非 map。
	if _, ok := checkResource("0"); ok {
		t.Fatal("checkResource(0) should be non-map")
	}
	// addResource：负重充足 → 全额掠夺，robStr 无尾逗号。
	sum, rob := addResource("0,0,0,0,0,", "0,1000,500,0,0", 1e6)
	if sum != "0,1000,500,0,0" || rob != "0,1000,500,0,0" {
		t.Fatalf("addResource sum=%q rob=%q", sum, rob)
	}
	// addResource：carry=0 → x=0，不掠夺。
	_, rob0 := addResource("0,0,0,0,0,", "0,1000,0,0,0", 0)
	if rob0 != "0,0,0,0,0" {
		t.Fatalf("addResource carry0 rob=%q", rob0)
	}
	// arraySearchLoose：命中键 0。
	rt := []idxKV{{0, 18}, {1, 15}}
	if k, ok := arraySearchLoose(rt, 18); !ok || k != 0 {
		t.Fatalf("arraySearchLoose=%d,%v", k, ok)
	}
	// sol2Value（数组分支）：floor 累加后 floor。
	ov := newOMap()
	ov.set(1, 100)
	want := math.Floor(math.Floor(100 * soldierValueAt(1) / 0.784))
	if got := sol2Value(ov); got != want {
		t.Fatalf("sol2Value=%v want %v", got, want)
	}
}

func TestGetRangBetweenAndAbleTarget(t *testing.T) {
	fs := []*Unit{
		{SID: 1, Count: 100, Range: 200, Typ: 1, Attack: 1, GF: 190, Stype: 1},
		{SID: 1, Count: 100, Range: 50, Typ: 1, Attack: 0, GF: 10, Stype: 1},
	}
	// 攻方：自身 range - 守方最大 range = 200-50 = 150。
	if got := getRangBetween(fs, 500, 0); got != 150 {
		t.Fatalf("getRangBetween attack=%v", got)
	}
	// 守方：攻方最小 range - 自身 range = 200-50 = 150。
	if got := getRangBetween(fs, 500, 1); got != 150 {
		t.Fatalf("getRangBetween resist=%v", got)
	}
	cfg := func(sid float64) float64 { return sid } // type == sid 便于断言
	able := getAbleTarget(fs, 0, cfg)
	if len(able) != 1 || able[0].idx != 1 || able[0].val != 1 {
		t.Fatalf("getAbleTarget attack=%+v", able)
	}
}

// ── 将领加成公式 ────────────────────────────────────────────────────────

func TestHeroBattleAdd_NoHero(t *testing.T) {
	s, f := newSvc(t)
	h, err := s.heroBattleAdd(context.Background(), 0, f.CID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h.command != 0 || h.blood != 0 || h.attack != 0 || h.heroshoot != 0 {
		t.Fatalf("no-hero 应全 0: %+v", h)
	}
}

func TestHeroBattleAdd_Formulas(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	// 城守（f.HID1）：command_base=80, affairs_base=40, bravery_base=90, wisdom_base=60, level=10。
	// 科技：6(统率)=3、9(攻击)=2、16(生命)=1。
	exec(t, f.DB, "insert into city_technics (city_id, technic_id, level) values (?,6,3),(?,9,2),(?,16,1) on duplicate key update level=values(level)",
		f.CID, f.CID, f.CID)
	// legacy `hid<1027` 名将判定 → 新库 heroes.npc_id>0（名将卡）。
	exec(t, f.DB, "update heroes set npc_id=36 where id=?", f.HID1)

	h, err := s.heroBattleAdd(ctx, f.HID1, f.CID, 1)
	if err != nil {
		t.Fatal(err)
	}
	// command = (80+10)*((1+0)+0.1*3) + 0 = 90*1.3 = 117
	if math.Abs(h.command-117) > 1e-9 {
		t.Fatalf("command=%v want 117", h.command)
	}
	// blood = 40/500 + 0.05*1 = 0.08+0.05 = 0.13
	if math.Abs(h.blood-0.13) > 1e-9 {
		t.Fatalf("blood=%v want 0.13", h.blood)
	}
	// gongji = 0.05*2 = 0.1
	if math.Abs(h.gongji-0.1) > 1e-9 {
		t.Fatalf("gongji=%v", h.gongji)
	}
	// 名将：heroattack = bravery_base*(1+0.05*2) = 90*1.1 = 99
	if math.Abs(h.heroattack-99) > 1e-9 {
		t.Fatalf("heroattack=%v want 99", h.heroattack)
	}
	// 名将：heroshoot = command_base*(1+0.05*科技14=0) = 80
	if math.Abs(h.heroshoot-80) > 1e-9 {
		t.Fatalf("heroshoot=%v want 80", h.heroshoot)
	}

	// 非名将（f.HID2 npc_id=0）→ 名将类加成恒 0。
	h2, err := s.heroBattleAdd(ctx, f.HID2, f.CID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if h2.heroattack != 0 || h2.heroshoot != 0 || h2.herodefence != 0 || h2.heroblood != 0 {
		t.Fatalf("非名将加成应为 0: %+v", h2)
	}
}

// ── 引擎：伤害公式 + 战报格式 ───────────────────────────────────────────

func insertField(t *testing.T, f *testutil.Fixture, guards map[int]int64, loot map[string]int64) int {
	t.Helper()
	ctx := context.Background()
	id, err := f.DB.Insert(ctx,
		"insert into fields (name, level, owner_uid, guard_soldiers, guard_power, loot_food, loot_wood, loot_rock, loot_iron, loot_gold) values (?,?,?,?,?,?,?,?,?,?)",
		"测试野地", 1, 0, jstr(guards), 0,
		loot["food"], loot["wood"], loot["rock"], loot["iron"], loot["gold"])
	if err != nil {
		t.Fatalf("insert field: %v", err)
	}
	fid := int(id)
	t.Cleanup(func() { _, _ = f.DB.Exec(context.Background(), "delete from fields where id=?", fid) })
	return fid
}

func insertTroop(t *testing.T, f *testutil.Fixture, targetID, task int, soldiers map[int]int64) int {
	t.Helper()
	ctx := context.Background()
	now := cellInt(t, f.DB, "select unix_timestamp()")
	id, err := f.DB.Insert(ctx,
		`insert into troops (user_id, city_id, hero_id, target_type, target_id, task, state, soldiers, start_at, arrive_at, back_at, created_at)
		 values (?,?,0,1,?,?,0,?,?,?,0,?)`,
		f.UID, f.CID, targetID, task, jstr(soldiers), now-5, now-1, now)
	if err != nil {
		t.Fatalf("insert troop: %v", err)
	}
	return int(id)
}

// TestEngineRound1Damage 验证 1000 义兵 vs 500 义兵 首回合的伤害/反击/战报 17 字段。
func TestEngineRound1Damage(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	fid := insertField(t, f, map[int]int64{1: 500}, map[string]int64{})
	troopID := insertTroop(t, f, fid, TaskPlunderForTest, map[int]int64{1: 1000})

	bid, err := s.StartBattleForTroop(ctx, troopID)
	if err != nil {
		t.Fatal(err)
	}
	if bid == 0 {
		t.Fatal("battle not created")
	}
	b, err := s.loadBattle(ctx, bid)
	if err != nil {
		t.Fatal(err)
	}
	// cfg_soldiers: sid1 ap=5 dp=10 hp=100 range=10 speed=180。
	if b.FieldRange != 309 {
		t.Fatalf("fieldrange=%v want 309", b.FieldRange)
	}
	// 强制到期跑一回合（now=24 使 round1 后 nexttime=25 > 24 即停）。
	exec(t, f.DB, "update battles set nexttime=0 where id=?", bid)
	if err := s.updateBattle(ctx, bid, 24); err != nil {
		t.Fatal(err)
	}

	report := mustCellStr(t, f.DB, "select report from battle_rounds where battleid=? and round=1", bid)
	line := strings.SplitN(report, ";", 2)[0]

	shanghai := int(math.Floor(1000 * 5.0 * 5.0 / (5.0 + 10.0))) // 1666
	siwang := shanghai / 100                                     // 16
	targetEnd := 500 - siwang                                    // 484
	fanjiSh := int(math.Floor(500 * 5.0 * 5.0 / (5.0 + 10.0)))   // 833
	fanjiSi := fanjiSh / 100                                     // 8
	fanjiEnd := 1000 - fanjiSi                                   // 992
	want := fmt.Sprintf(
		"1.000000,1.000000,1.000000,1.000000,159.000000,1.000000,%d.000000,1.000000,1.000000,500.000000,%d.000000,%d.000000,1.000000,%d.000000,1000.000000,%d.000000,%d.000000",
		shanghai, siwang, targetEnd, fanjiSh, fanjiSi, fanjiEnd)
	if line != want {
		t.Fatalf("report line mismatch\n got=%s\nwant=%s", line, want)
	}
}

// TestWallDamage 验证打城墙伤害 = floor(count*ap*people_need/100)。
func TestWallDamage(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	now := cellInt(t, f.DB, "select unix_timestamp()")
	// 攻方 100 弓箭兵(sid6, type6 远程, ap=120, people_need=2)；守方仅城墙 wallhp=1000000。
	bid, err := f.DB.Insert(ctx,
		`insert into battles (type,state,result,starttime,cid,attackuid,resistuid,attacktroop,resistdefence,
		   round,nexttime,attackcid,attackhid,attacksoldiers,attackpos,resistcid,resisthid,resistsoldiers,resistpos,
		   wallhp,walllevel,fieldrange,level,attackstartcid,resiststartcid)
		 values (0,0,3,?,?,?,0,0,'0,',1,0,?,0,'1,6,100,','1,6,1399,',0,0,'0','0,',1000000,5,1499,1,?,0)`,
		now, f.CID, f.UID, f.CID, f.CID)
	if err != nil {
		t.Fatal(err)
	}
	battleID := int(bid)
	t.Cleanup(func() { _, _ = f.DB.Exec(context.Background(), "delete from battles where id=?", battleID) })
	// 战术：攻方 stype6 前进。
	exec(t, f.DB, "insert into battle_tactics (battleid,attack,stype,action,target,action2,target2) values (?,1,6,1,0,0,0)", battleID)

	// now=24 → 仅跑 1 回合。
	if err := s.updateBattle(ctx, battleID, 24); err != nil {
		t.Fatal(err)
	}
	// ap=120, people_need=2 → floor(100*120*2/100)=240。
	if got := cellInt(t, f.DB, "select wallhp from battles where id=?", battleID); got != 1000000-240 {
		t.Fatalf("wallhp=%d want %d", got, 1000000-240)
	}
}

// ── 结算：胜负 / 掠夺 / 占领 / 伤兵 ─────────────────────────────────────

const (
	TaskPlunderForTest = 3
	TaskOccupyForTest  = 4
)

func TestAttackWinPlunder(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	// 无守军野地 → 首回合即攻方胜。
	fid := insertField(t, f, nil, map[string]int64{"food": 1000, "wood": 500})
	troopID := insertTroop(t, f, fid, TaskPlunderForTest, map[int]int64{1: 1000})
	before := cellInt(t, f.DB, "select food from city_resources where city_id=?", f.CID)

	bid, err := s.StartBattleForTroop(ctx, troopID)
	if err != nil || bid == 0 {
		t.Fatalf("create battle: %v (%d)", err, bid)
	}
	exec(t, f.DB, "update battles set nexttime=0 where id=?", bid)
	now := cellInt(t, f.DB, "select unix_timestamp()")
	if err := s.updateBattle(ctx, bid, now); err != nil {
		t.Fatal(err)
	}
	// 战斗结束、攻方胜、部队返程。
	if got := cellInt(t, f.DB, "select state from battles where id=?", bid); got != 1 {
		t.Fatalf("battle state=%d", got)
	}
	if got := cellInt(t, f.DB, "select result from battles where id=?", bid); got != 0 {
		t.Fatalf("battle result=%d", got)
	}
	if got := cellInt(t, f.DB, "select state from troops where id=?", troopID); got != 1 {
		t.Fatalf("troop state=%d want 1(return)", got)
	}
	// 掠夺资源 +1000 粮 +500 木。
	after := cellInt(t, f.DB, "select food from city_resources where city_id=?", f.CID)
	if after-before != 1000 {
		t.Fatalf("looted food=%d want 1000", after-before)
	}
	if got := cellInt(t, f.DB, "select wood from city_resources where city_id=?", f.CID); got != 100500 {
		t.Fatalf("wood=%d want 100500", got)
	}
	// 战报写入。
	if got := cellInt(t, f.DB, "select count(*) from reports where battleid=?", bid); got != 1 {
		t.Fatalf("reports=%d want 1", got)
	}
}

func TestAttackWinOccupy(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	fid := insertField(t, f, nil, map[string]int64{})
	troopID := insertTroop(t, f, fid, TaskOccupyForTest, map[int]int64{1: 500})
	bid, err := s.StartBattleForTroop(ctx, troopID)
	if err != nil || bid == 0 {
		t.Fatalf("create battle: %v", err)
	}
	exec(t, f.DB, "update battles set nexttime=0 where id=?", bid)
	now := cellInt(t, f.DB, "select unix_timestamp()")
	if err := s.updateBattle(ctx, bid, now); err != nil {
		t.Fatal(err)
	}
	if got := cellInt(t, f.DB, "select owner_uid from fields where id=?", fid); got != int64(f.UID) {
		t.Fatalf("field owner=%d want %d", got, f.UID)
	}
	if got := cellInt(t, f.DB, "select state from troops where id=?", troopID); got != 4 {
		t.Fatalf("troop state=%d want 4(garrison)", got)
	}
}

// TestDefenderWoundedAndPrestige 玩家城防守方伤兵入城 + 声望/战功。
func TestDefenderWoundedAndPrestige(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	uid2, cid2 := newSecondCity(t, f, 200)

	// 目标城战（type=2 掠夺）。
	now := cellInt(t, f.DB, "select unix_timestamp()")
	troopID, err := f.DB.Insert(ctx, `insert into troops (user_id,city_id,hero_id,target_type,target_id,task,state,soldiers,start_at,arrive_at,back_at,created_at)
		values (?,?,0,2,?,3,0,?,?,?,0,?)`,
		f.UID, f.CID, cid2, jstr(map[int]int64{1: 20000}), now-5, now-1, now)
	if err != nil {
		t.Fatal(err)
	}
	bid, err := s.StartBattleForTroop(ctx, int(troopID))
	if err != nil || bid == 0 {
		t.Fatalf("create battle: %v", err)
	}
	exec(t, f.DB, "update battles set nexttime=0 where id=?", bid)
	if err := s.updateBattle(ctx, bid, now); err != nil {
		t.Fatal(err)
	}
	if got := cellInt(t, f.DB, "select state from battles where id=?", bid); got != 1 {
		t.Fatalf("battle not ended: state=%d", got)
	}
	// 防守方 200 义兵全灭 → 伤兵 floor(200*0.19)=38。
	if got := cellInt(t, f.DB, "select count from city_wounded where city_id=? and soldier_id=1", cid2); got != 38 {
		t.Fatalf("defender wounded=%d want 38", got)
	}
	_ = uid2
}

func newSecondCity(t *testing.T, f *testutil.Fixture, guards int64) (uid, cid int) {
	t.Helper()
	ctx := context.Background()
	suffix := strconv.FormatInt(int64(cellInt(t, f.DB, "select unix_timestamp()")), 10)
	uid64, err := f.DB.Insert(ctx, "insert into users (passport, password_hash, nickname, state, prestige) values (?,?,?,0,0)",
		"t2_"+suffix, "x", "敌军"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	cid64, err := f.DB.Insert(ctx, "insert into cities (user_id, name, is_special, type, general_hero_id, chief_hero_id, counsellor_hero_id) values (?,?,0,0,0,0,0)",
		uid64, "敌城"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, `insert into city_resources
		(city_id, wood, wood_max, rock, rock_max, iron, iron_max, food, food_max, gold, gold_max, people, people_max, morale, tax, complaint, vacation)
		values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		cid64, 50000, 500000, 50000, 500000, 50000, 500000, 50000, 500000, 5000, 100000, 500, 5000, 100, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, "insert into city_soldiers (city_id, soldier_id, count) values (?,1,?)", cid64, guards); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, "update users set lastcid=? where id=?", cid64, uid64); err != nil {
		t.Fatal(err)
	}
	u, c := int(uid64), int(cid64)
	t.Cleanup(func() {
		bg := context.Background()
		for _, q := range []string{
			"delete from battle_rounds where battleid in (select id from battles where attackuid=? or resistuid=?)",
			"delete from battle_tactics where battleid in (select id from battles where attackuid=? or resistuid=?)",
			"delete from bak_troops where uid=?",
			"delete from battles where attackuid=? or resistuid=?",
		} {
			_, _ = f.DB.Exec(bg, q, u, u)
		}
		_, _ = f.DB.Exec(bg, "delete from city_wounded where city_id=?", c)
		_, _ = f.DB.Exec(bg, "delete from city_soldiers where city_id=?", c)
		_, _ = f.DB.Exec(bg, "delete from city_resources where city_id=?", c)
		_, _ = f.DB.Exec(bg, "delete from heroes where city_id=?", c)
		_, _ = f.DB.Exec(bg, "delete from cities where id=?", c)
		_, _ = f.DB.Exec(bg, "delete from users where id=?", u)
	})
	return u, c
}

func TestQingnangWoundedRate(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	// 攻方青囊（buftype=9）→ a_wounded = 0.19+0.3 = 0.49。
	exec(t, f.DB, "insert into user_buffers (user_id, buftype, endtime) values (?,9,unix_timestamp()+3600) on duplicate key update endtime=values(endtime)", f.UID)
	uid2, cid2 := newSecondCity(t, f, 1000)
	_ = uid2
	now := cellInt(t, f.DB, "select unix_timestamp()")
	troopID, err := f.DB.Insert(ctx, `insert into troops (user_id,city_id,hero_id,target_type,target_id,task,state,soldiers,start_at,arrive_at,back_at,created_at)
		values (?,?,0,2,?,3,0,?,?,?,0,?)`,
		f.UID, f.CID, cid2, jstr(map[int]int64{1: 100}), now-5, now-1, now)
	if err != nil {
		t.Fatal(err)
	}
	bid, err := s.StartBattleForTroop(ctx, int(troopID))
	if err != nil || bid == 0 {
		t.Fatalf("create battle: %v", err)
	}
	// 攻方 100 义兵不敌 1000 守军 → 逐回合直到结束。
	runToEnd(t, s, f, bid)
	// 攻方 100 义兵全灭，青囊使伤兵比例 = 0.19+0.3 = 0.49 → 报显示 49。
	rep := mustCellStr(t, f.DB, "select content from reports where battleid=? limit 1", bid)
	if !strings.Contains(rep, "伤兵比例：49") {
		t.Fatalf("report missing wounded rate:\n%s", rep)
	}
}

// runToEnd 反复把 nexttime 拨到当前时刻之前，直到战斗结束（上限 60 回合）。
func runToEnd(t *testing.T, s *Service, f *testutil.Fixture, bid int) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 60; i++ {
		if cellInt(t, f.DB, "select state from battles where id=?", bid) == 1 {
			return
		}
		exec(t, f.DB, "update battles set nexttime=unix_timestamp()-1 where id=?", bid)
		now := cellInt(t, f.DB, "select unix_timestamp()")
		if err := s.updateBattle(ctx, bid, now); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("battle %d not ended after 60 rounds", bid)
}

func TestIsNPC(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	if !s.IsNPC(ctx, 0) {
		t.Fatal("uid 0 应视为 NPC")
	}
	if !s.IsNPC(ctx, 500) {
		t.Fatal("uid<1000 应视为 NPC")
	}
	if s.IsNPC(ctx, f.UID) {
		t.Fatal("真实玩家不应视为 NPC")
	}
}

func TestSettleCatchUpMultipleRounds(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	// 双方 1000 义兵，势均力敌 → 多回合不结束。
	fid := insertField(t, f, map[int]int64{1: 1000}, map[string]int64{})
	troopID := insertTroop(t, f, fid, TaskPlunderForTest, map[int]int64{1: 1000})
	bid, err := s.StartBattleForTroop(ctx, troopID)
	if err != nil || bid == 0 {
		t.Fatalf("create battle: %v", err)
	}
	// nexttime 拨到 100s 前 → 按 25s/tick 回放 5 回合（-100,-75,-50,-25,0）。
	exec(t, f.DB, "update battles set nexttime=unix_timestamp()-100 where id=?", bid)
	now := cellInt(t, f.DB, "select unix_timestamp()")
	if err := s.updateBattle(ctx, bid, now); err != nil {
		t.Fatal(err)
	}
	if got := cellInt(t, f.DB, "select count(*) from battle_rounds where battleid=?", bid); got != 5 {
		t.Fatalf("rounds=%d want 5", got)
	}
	if got := cellInt(t, f.DB, "select round from battles where id=?", bid); got != 6 {
		t.Fatalf("battle.round=%d want 6", got)
	}
}

func mustCellStr(t *testing.T, d *db.DB, q string, args ...any) string {
	t.Helper()
	v, err := d.FetchCellString(context.Background(), q, args...)
	if err != nil {
		t.Fatalf("cellstr %q: %v", q, err)
	}
	return v
}
