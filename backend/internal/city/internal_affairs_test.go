//go:build integration

package city

import (
	"context"
	"math"
	"testing"

	"rxsg/backend/internal/testutil"
)

// M1 城市内政真库集成测试：公式手算值与 CityFunc.php 1:1 对照。
// 测试城基线（testutil.SetupCity）：people=1000, people_max=5000, morale=100, tax=0,
// complaint=0, food=100000, gold=10000；城守 hid1：level10 command80 affairs40。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	s := NewService(d, nil)
	return s, f
}

func TestChangeTaxFormula(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	res, err := s.ChangeTax(ctx, f.UID, f.CID, 20)
	if err != nil {
		t.Fatalf("ChangeTax: %v", err)
	}
	if res.Tax != 20 {
		t.Fatalf("tax=%d want 20", res.Tax)
	}
	// morale_stable = max(0, min(100-20-0, 100)) = 80
	if res.MoraleStable != 80 {
		t.Fatalf("morale_stable=%d want 80", res.MoraleStable)
	}

	if _, err := s.ChangeTax(ctx, f.UID, f.CID, 101); err == nil {
		t.Fatal("tax>100 应报错")
	}
	if _, err := s.ChangeTax(ctx, f.UID, f.CID, -1); err == nil {
		t.Fatal("tax<0 应报错")
	}
	// 非本人城池
	if _, err := s.ChangeTax(ctx, f.UID+999999, f.CID, 10); err == nil {
		t.Fatal("非属主应报错")
	}
}

func TestLevyResourceGold(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	msg, res, err := s.LevyResource(ctx, f.UID, f.CID, 0)
	if err != nil {
		t.Fatalf("LevyResource: %v", err)
	}
	// gold += people(1000) × SPEED(10) × GOLD_RATE(1) × 0.1 = 1000
	if res.Gold != 11000 {
		t.Fatalf("gold=%d want 11000", res.Gold)
	}
	// morale = max(0, 100-20) = 80
	if res.Morale != 80 {
		t.Fatalf("morale=%d want 80", res.Morale)
	}
	// people_stable = people_max(5000) × morale(80) × 0.01 = 4000
	if res.PeopleStable != 4000 {
		t.Fatalf("people_stable=%d want 4000", res.PeopleStable)
	}
	if msg == "" {
		t.Fatal("msg 为空")
	}

	// 冷却 900s 内二次征收应拒绝
	if _, _, err := s.LevyResource(ctx, f.UID, f.CID, 1); err == nil {
		t.Fatal("冷却期内应拒绝")
	}
}

func TestLevyResourceMoraleGate(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := s.db.Exec(ctx, "update city_resources set morale=20 where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.LevyResource(ctx, f.UID, f.CID, 0); err == nil {
		t.Fatal("morale<=20 应拒绝征收")
	}
}

func TestPacifyRezai(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 赈灾：耗粮=people_max×SPEED=50000；morale+5（封顶100）；complaint-15
	if _, err := s.db.Exec(ctx, "update city_resources set complaint=20 where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}
	_, res, err := s.PacifyPeople(ctx, f.UID, f.CID, 0)
	if err != nil {
		t.Fatalf("Pacify(0): %v", err)
	}
	if res.Food != 50000 {
		t.Fatalf("food=%d want 50000", res.Food)
	}
	if res.Morale != 100 { // LEAST(100,105)
		t.Fatalf("morale=%d want 100", res.Morale)
	}
	if res.Complaint != 5 {
		t.Fatalf("complaint=%d want 5", res.Complaint)
	}
	// 城守经验：floor(people_max×SPEED×FOOD_PRICE × HERO_EXP_RATE)=floor(50000×0.1×0.1)=500
	exp, err := s.db.FetchCellInt64(ctx, "select exp from heroes where id=?", f.HID1)
	if err != nil {
		t.Fatal(err)
	}
	if exp != 500 {
		t.Fatalf("chief exp=%d want 500", exp)
	}

	// 冷却期内再次安抚应拒绝
	if _, _, err := s.PacifyPeople(ctx, f.UID, f.CID, 1); err == nil {
		t.Fatal("冷却期内应拒绝")
	}
}

func TestPacifyInsufficientFood(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := s.db.Exec(ctx, "update city_resources set food=10 where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PacifyPeople(ctx, f.UID, f.CID, 0); err == nil {
		t.Fatal("粮食不足应拒绝赈灾")
	}
}

func TestPacifyZengding(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 增丁：耗粮=people_max×5×SPEED=250000 > food(100000)，先补粮
	if _, err := s.db.Exec(ctx, "update city_resources set food=300000 where city_id=?", f.CID); err != nil {
		t.Fatal(err)
	}
	_, res, err := s.PacifyPeople(ctx, f.UID, f.CID, 3)
	if err != nil {
		t.Fatalf("Pacify(3): %v", err)
	}
	if res.Food != 50000 {
		t.Fatalf("food=%d want 50000", res.Food)
	}
	// people += floor(5000×10×0.05)=2500 → min(5000, 1000+2500)=3500
	if res.People != 3500 {
		t.Fatalf("people=%d want 3500", res.People)
	}
}

func TestPacifyJitianSchedule(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 祭天：food-50000, gold-5000；next_bad_event 推迟
	if _, err := s.db.Exec(ctx, "insert into city_schedule (city_id,next_bad_event) values (?,?) on duplicate key update next_bad_event=?",
		f.CID, 1000000, 1000000); err != nil {
		t.Fatal(err)
	}
	_, res, err := s.PacifyPeople(ctx, f.UID, f.CID, 2)
	if err != nil {
		t.Fatalf("Pacify(2): %v", err)
	}
	if res.Food != 50000 {
		t.Fatalf("food=%d want 50000", res.Food)
	}
	if res.Gold != 5000 { // 10000 - 5000×0.1×10
		t.Fatalf("gold=%d want 5000", res.Gold)
	}
	nbe, err := s.scheduleValue(ctx, f.CID, "next_bad_event")
	if err != nil {
		t.Fatal(err)
	}
	// 原值 1000000：-（1000000+28800)%86400 + 86400 + rand(0,259200)
	base := int64(1000000) - (1000000+8*3600)%86400 + 86400
	if nbe < base || nbe > base+259200 {
		t.Fatalf("next_bad_event=%d 不在 [%d,%d]", nbe, base, base+259200)
	}
}

func TestGetCityProduct(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 农田 bid2 等级1（using_people=10，见 0004 种子）
	if _, err := s.db.Exec(ctx, "insert into buildings (city_id, building_id, xy, level, state) values (?,?,?,1,0)",
		f.CID, 2, "a1"); err != nil {
		t.Fatal(err)
	}
	// 粮食科技 lv3 → 加成 30
	if _, err := s.db.Exec(ctx, "insert into city_technics (city_id, technic_id, level) values (?,?,3)",
		f.CID, 1); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetCityProduct(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetCityProduct: %v", err)
	}
	// 基础=GLOBAL_FOOD_RATE(10)×10×SPEED(10)=1000
	if p.FoodBase != 1000 {
		t.Fatalf("foodBase=%v want 1000", p.FoodBase)
	}
	if p.FoodTechnic != 30 {
		t.Fatalf("foodTechnic=%d want 30", p.FoodTechnic)
	}
	// 城守加成：affairs=40；heroCommand=10+80=90；peoplerate=90×10×100/(5000+1)=17.99→1.0
	// chief_add=40×1.0=40
	if math.Abs(p.ChiefAdd-40) > 1e-6 {
		t.Fatalf("chiefAdd=%v want 40", p.ChiefAdd)
	}
	// 行缺失自动补默认比例 80
	if p.FoodRate != 80 {
		t.Fatalf("foodRate=%d want 80", p.FoodRate)
	}
}
