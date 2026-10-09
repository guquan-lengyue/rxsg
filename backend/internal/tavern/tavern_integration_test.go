//go:build integration

package tavern

import (
	"context"
	"testing"

	"rxsg/backend/internal/testutil"
)

// tavern_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：池刷新块机制、招募扣金/入城/血条/俸禄、容量位、招贤榜重置、野兵填充、原版怪癖。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

func addBuilding(t *testing.T, f *testutil.Fixture, bid int, xy string, level int) {
	t.Helper()
	if _, err := f.DB.Exec(context.Background(),
		"insert into buildings (city_id, building_id, xy, level, state, state_start_at, state_end_at) values (?,?,?,?,0,0,0)",
		f.CID, bid, xy, level); err != nil {
		t.Fatalf("insert building %d: %v", bid, err)
	}
}

func cell(t *testing.T, f *testutil.Fixture, query string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("cell %q: %v", query, err)
	}
	return v
}

// insertPoolRow 手工插入一条可控的池行，返回 id。
func insertPoolRow(t *testing.T, f *testutil.Fixture, cid int, goldNeed int64, heroType int) int {
	t.Helper()
	id, err := f.DB.Insert(context.Background(), `insert into recruit_heroes
		(name, sex, face, city_id, level, exp, affairs_base, bravery_base, wisdom_base,
		 command_base, affairs_add, bravery_add, wisdom_add, command_add, loyalty, gold_need, gen_time, hero_type)
		values ('测试将',1,1001,?,5,0,50,50,50,3,2,2,1,1,70,?,unix_timestamp(),?)`,
		cid, goldNeed, heroType)
	if err != nil {
		t.Fatalf("insert pool row: %v", err)
	}
	return int(id)
}

// ---------- 无客栈 throw ----------

func TestHotelNotBuilt(t *testing.T) {
	s, f := newSvc(t)
	if _, err := s.Info(context.Background(), f.UID, f.CID); err == nil ||
		err.Error() != msgNoHotelBuilt {
		t.Fatalf("err=%v want %q", err, msgNoHotelBuilt)
	}
}

// ---------- 池刷新块机制 ----------

func TestPoolRefreshBlocks(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	addBuilding(t, f, officeBuildingID, "c3", 5)

	info, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	// 首次：last_reset_recruit 无行 → empty 分支置 0 → blockdelta 巨大 → 补满 level=3 个。
	if len(info.Recruits) != 3 {
		t.Fatalf("pool=%d want 3", len(info.Recruits))
	}
	for _, r := range info.Recruits {
		if r.Level < 1 || r.Level > 15 { // level*5
			t.Fatalf("hero level=%d out of [1,15]", r.Level)
		}
		if r.Loyalty != 70 {
			t.Fatalf("loyalty=%d want 70", r.Loyalty)
		}
		if r.AffairsAdd+r.BraveryAdd+r.WisdomAdd != r.Level {
			t.Fatalf("add sum=%d+%d+%d != level %d", r.AffairsAdd, r.BraveryAdd, r.WisdomAdd, r.Level)
		}
		// gold_need 公式复算
		want := recruitGoldNeed(r.Level, r.AffairsBase, r.AffairsAdd, r.BraveryBase, r.BraveryAdd, r.WisdomBase, r.WisdomAdd)
		if r.GoldNeed != want {
			t.Fatalf("gold_need=%d want %d", r.GoldNeed, want)
		}
		if r.Name == "" {
			t.Fatal("empty name")
		}
		if r.Sex == 0 && (r.Face < 100 || r.Face > 145) {
			t.Fatalf("girl face=%d out of [100,145]", r.Face)
		}
		if r.Sex == 1 && (r.Face < 1001 || r.Face > 1070) {
			t.Fatalf("boy face=%d out of [1001,1070]", r.Face)
		}
		// 怪癖：generateRecruitHero 的 insert 不含 command_add/herotype → 恒 0。
		if r.CommandAdd != 0 || r.HeroType != 0 {
			t.Fatalf("command_add=%d hero_type=%d want 0/0", r.CommandAdd, r.HeroType)
		}
	}
	// 立刻再取：同一刷新块内 blockdelta=0 → 不重生成（id 集合不变）。
	// 若两次调用跨越了刷新块边界（blocksize=360s），legacy 本就会删旧补新，此处按同块才断言。
	blocksize := float64(recruitBlock/gameSpeedRate) / 3.0
	last := cell(t, f, "select last_reset_recruit from city_schedule where city_id=?", f.CID)
	now := cell(t, f, "select unix_timestamp()")
	sameBlock := float64(last+8*3600)/blocksize >= float64(now+8*3600)/blocksize
	ids := map[int]bool{}
	for _, r := range info.Recruits {
		ids[r.ID] = true
	}
	info2, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("info2: %v", err)
	}
	if len(info2.Recruits) != 3 {
		t.Fatalf("pool=%d want 3", len(info2.Recruits))
	}
	if sameBlock {
		for _, r := range info2.Recruits {
			if !ids[r.ID] {
				t.Fatalf("unexpected new hero %d within same block", r.ID)
			}
		}
	}
	// order by id desc
	for i := 1; i < len(info2.Recruits); i++ {
		if info2.Recruits[i-1].ID < info2.Recruits[i].ID {
			t.Fatal("pool not ordered by id desc")
		}
	}
	// 官署 5 级、城内 2 将 → 空位 3；爵位默认 ''→0 → can_jiejiao false。
	if info2.OfficePos != 3 {
		t.Fatalf("office_pos=%d want 3", info2.OfficePos)
	}
	if info2.CanJiejiao {
		t.Fatal("can_jiejiao should be false")
	}
}

// ---------- 野兵填充（每次请求至多 1 个） ----------

func TestNpcFill(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 1)

	before := cell(t, f, "select count(*) from recruit_heroes where city_id=0")
	if _, err := s.Info(ctx, f.UID, f.CID); err != nil {
		t.Fatalf("info: %v", err)
	}
	after := cell(t, f, "select count(*) from recruit_heroes where city_id=0")
	if after < before+1 {
		t.Fatalf("npc pool %d -> %d, want +1", before, after)
	}
}

// ---------- 招募主链 ----------

func TestRecruitFlow(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	addBuilding(t, f, officeBuildingID, "c3", 5)

	id := insertPoolRow(t, f, f.CID, 1000, 15)

	info, err := s.RecruitHero(ctx, f.UID, f.CID, id)
	if err != nil {
		t.Fatalf("recruit: %v", err)
	}
	for _, r := range info.Recruits {
		if r.ID == id {
			t.Fatal("pool row not deleted")
		}
	}
	hid := cell(t, f, "select id from heroes where city_id=? and name='测试将' limit 1", f.CID)
	if hid == 0 {
		t.Fatal("hero not inserted")
	}
	if state := cell(t, f, "select state from heroes where id=?", hid); state != 0 {
		t.Fatalf("state=%d want 0", state)
	}
	// 扣城金 1000（10000→9000）。
	if gold := cell(t, f, "select gold from city_resources where city_id=?", f.CID); gold != 9000 {
		t.Fatalf("gold=%d want 9000", gold)
	}
	// hero_blood：force_max=100+5/5+(50+2)/3=118；energy_max=100+1+(50+1)/3=118。
	fm := cell(t, f, "select force_max from hero_blood where hero_id=?", hid)
	em := cell(t, f, "select energy_max from hero_blood where hero_id=?", hid)
	if fm != 118 || em != 118 {
		t.Fatalf("force_max=%d energy_max=%d want 118/118", fm, em)
	}
	// 俸禄全量重算：赵云 10*20=200 + 太守 (5+10)*50+200=950 + 新将 5*20=100 → 1250。
	if fee := cell(t, f, "select hero_fee from city_resources where city_id=?", f.CID); fee != 1250 {
		t.Fatalf("hero_fee=%d want 1250", fee)
	}
	// command_add → heroes.command_add_on；is_act_hero(15)=true（cfg_recruit_hero 无表→上限检查恒通过）。
	ca := cell(t, f, "select command_add_on from heroes where id=?", hid)
	if ca != 1 {
		t.Fatalf("command_add_on=%d want 1", ca)
	}
}

func TestRecruitNoPosition(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	// 无官署 → cityHasHeroPosition false → "招贤馆等级不够"。
	id := insertPoolRow(t, f, f.CID, 1000, 0)
	if _, err := s.RecruitHero(ctx, f.UID, f.CID, id); err == nil ||
		err.Error() != msgHotelLevelLow {
		t.Fatalf("err=%v want %q", err, msgHotelLevelLow)
	}
}

func TestRecruitNotEnoughGold(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	addBuilding(t, f, officeBuildingID, "c3", 5)
	id := insertPoolRow(t, f, f.CID, 999999, 0)
	if _, err := s.RecruitHero(ctx, f.UID, f.CID, id); err == nil ||
		err.Error() != msgNoEnoughGold {
		t.Fatalf("err=%v want %q", err, msgNoEnoughGold)
	}
	// 池行未被删除。
	if cell(t, f, "select count(*) from recruit_heroes where id=?", id) != 1 {
		t.Fatal("pool row should remain")
	}
}

// ---------- 怪癖：池行不存在静默成功 ----------

func TestRecruitMissingRow(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	info, err := s.RecruitHero(ctx, f.UID, f.CID, 987654)
	if err != nil {
		t.Fatalf("missing pool row must be silent, got %v", err)
	}
	if info.HotelLevel != 3 {
		t.Fatalf("hotel_level=%d want 3", info.HotelLevel)
	}
}

// ---------- 招贤榜重置 ----------

func TestResetNoGoods(t *testing.T) {
	s, f := newSvc(t)
	if _, err := s.Reset(context.Background(), f.UID, f.CID); err == nil ||
		err.Error() != msgNoZhaoXinLin {
		t.Fatalf("err=%v want %q", err, msgNoZhaoXinLin)
	}
}

func TestResetRebuildPool(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 3)
	if _, err := f.DB.Exec(ctx,
		"insert into user_goods (user_id, gid, `count`) values (?,23,2)", f.UID); err != nil {
		t.Fatal(err)
	}

	info, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	old := map[int]bool{}
	for _, r := range info.Recruits {
		old[r.ID] = true
	}

	info2, err := s.Reset(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	// Reset 内部调 Info：last_reset_recruit 置 0 后 empty 分支 → blockdelta 巨大 → 全池重建。
	if len(info2.Recruits) != 3 {
		t.Fatalf("pool=%d want 3", len(info2.Recruits))
	}
	for _, r := range info2.Recruits {
		if old[r.ID] {
			t.Fatal("pool not rebuilt after zhaoxinlin")
		}
	}
	if cnt := cell(t, f, "select `count` from user_goods where user_id=? and gid=23", f.UID); cnt != 1 {
		t.Fatalf("goods23=%d want 1", cnt)
	}
	if lg := cell(t, f, "select coalesce(sum(`count`),0) from log_goods where user_id=? and gid=23", f.UID); lg != -1 {
		t.Fatalf("log_goods=%d want -1", lg)
	}
	// 重置后游标已回写 now（非 0）。
	if lr := cell(t, f, "select last_reset_recruit from city_schedule where city_id=?", f.CID); lr == 0 {
		t.Fatal("last_reset_recruit should be backfilled")
	}
}

// ---------- 爵位（getBufferNobility） ----------

func TestCanJiejiaoNobility(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, hotelBuildingID, "d3", 2)
	if _, err := f.DB.Exec(ctx, "update users set nobility='5' where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if !info.CanJiejiao {
		t.Fatal("nobility 5 should allow jiejiao")
	}
	// 推恩令 buftype=16 bufparam=5：爵位 3+5=8 ≥5。
	if _, err := f.DB.Exec(ctx, "update users set nobility='3' where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx,
		"insert into user_buffers (user_id, buftype, bufparam, endtime) values (?,16,5,unix_timestamp()+3600)", f.UID); err != nil {
		t.Fatal(err)
	}
	info2, err := s.Info(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("info2: %v", err)
	}
	if !info2.CanJiejiao {
		t.Fatal("buffered nobility 8 should allow jiejiao")
	}
}

// ---------- isActHero 判定 ----------

func TestIsActHeroFlag(t *testing.T) {
	if isActHero(15) != true || isActHero(100) != false || isActHero(10) != false || isActHero(20000) != false {
		t.Fatal("isActHero mismatch")
	}
}
