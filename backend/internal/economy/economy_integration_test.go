//go:build integration

package economy

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"rxsg/backend/internal/building"
	"rxsg/backend/internal/game"
	"rxsg/backend/internal/testutil"
)

// economy_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：市场信息/官市买卖（含 log_merchants 列错位怪癖）、挂单-购买-惰性成交（handleTrade 战报）、
// 自动运输（AddAutoTrans 契约 + handleAutoTrans 恒扣粮 bug）、取消/加速、仓库比例+打包（152 双记账怪癖）、
// 作坊刷新/购买、商城 buyGoods 各分支、礼券 exchangeLiquan、sellGoods、setCityProductRate、产出结算手算对照。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d, building.NewService(d)), f
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

func fcell(t *testing.T, f *testutil.Fixture, query string, args ...any) float64 {
	t.Helper()
	v, err := f.DB.FetchCellString(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("fcell %q: %v", query, err)
	}
	n, _ := strconv.ParseFloat(v, 64)
	return n
}

func exec(t *testing.T, f *testutil.Fixture, query string, args ...any) {
	t.Helper()
	if _, err := f.DB.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func giveGoods(t *testing.T, f *testutil.Fixture, gid int, cnt int64) {
	t.Helper()
	exec(t, f, "insert into user_goods (user_id, gid, `count`) values (?,?,?) "+
		"on duplicate key update `count`=?", f.UID, gid, cnt, cnt)
}

func asInt(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

// errMsg 断言 err 为 legacy throw 文案（成功文案同样经由 err 返回，1:1 保留）。
func errMsg(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// newSecondCity 为 f 追加一座自有城（含资源行与市场），返回 cid；t.Cleanup 负责删除。
func newSecondCity(t *testing.T, f *testutil.Fixture, marketLevel int) int {
	t.Helper()
	ctx := context.Background()
	cid, err := f.DB.Insert(ctx,
		"insert into cities (user_id, name, is_special, type, general_hero_id, chief_hero_id, counsellor_hero_id) values (?,?,0,0,0,0,0)",
		f.UID, fmt.Sprintf("分城%d", f.CID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx, `insert into city_resources
		(city_id, wood, wood_max, rock, rock_max, iron, iron_max, food, food_max, gold, gold_max,
		 people, people_max, morale, tax, complaint, vacation)
		values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		cid, 100000, 500000, 100000, 500000, 100000, 500000, 100000, 500000, 10000, 100000,
		1000, 5000, 100, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.DB.Exec(ctx,
		"insert into buildings (city_id, building_id, xy, level, state, state_start_at, state_end_at) values (?,?,?,?,0,0,0)",
		cid, game.BidMarket, "a4", marketLevel); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.DB.Exec(ctx, "delete from city_trades where cid=? or buycid=?", cid, cid)
		_, _ = f.DB.Exec(ctx, "delete from city_merchants where city_id=?", cid)
		_, _ = f.DB.Exec(ctx, "delete from city_res_add where city_id=?", cid)
		_, _ = f.DB.Exec(ctx, "delete from city_resources where city_id=?", cid)
		_, _ = f.DB.Exec(ctx, "delete from buildings where city_id=?", cid)
		_, _ = f.DB.Exec(ctx, "delete from cities where id=?", cid)
	})
	return int(cid)
}

// ---------- 市场信息 ----------

func TestNoMarketBuilt(t *testing.T) {
	s, f := newSvc(t)
	if _, err := s.GetMarketInfo(context.Background(), f.UID, f.CID); err == nil ||
		errMsg(err) != "该城池尚未建造市场。" {
		t.Fatalf("err=%v", errMsg(err))
	}
}

// TestSettleFormula 无产源建筑时的惰性结算手算对照：
// FoodBase=0 → foodAdd=floor(100+0)=100；foodMax=floor(10000+0)=10000；
// woods=100000+100/225>woodMax=10000 → 保持 floor(curWood)=100000。
func TestSettleFormula(t *testing.T) {
	s, f := newSvc(t)
	addBuilding(t, f, game.BidMarket, "a4", 5)
	if _, err := s.GetMarketInfo(context.Background(), f.UID, f.CID); err != nil {
		t.Fatalf("market info: %v", err)
	}
	if v := cell(t, f, "select food_add from city_resources where city_id=?", f.CID); v != 100 {
		t.Fatalf("food_add=%d want 100", v)
	}
	if v := cell(t, f, "select food_max from city_resources where city_id=?", f.CID); v != 10000 {
		t.Fatalf("food_max=%d want 10000", v)
	}
	if v := cell(t, f, "select wood from city_resources where city_id=?", f.CID); v != 100000 {
		t.Fatalf("wood=%d want 100000（超上限分支保持原值）", v)
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 10000 {
		t.Fatalf("gold=%d want 10000（tax=0，hero_fee 因 $ciyt 拼写 bug 恒 0）", v)
	}
	if v := cell(t, f, "select people_max from city_resources where city_id=?", f.CID); v != 0 {
		t.Fatalf("people_max=%d want 0（updateCityPeopleMax 按民居 sum 重算）", v)
	}
}

// ---------- 官市 ----------

func TestMerchantInfoQuirk(t *testing.T) {
	s, f := newSvc(t)
	addBuilding(t, f, game.BidMarket, "a4", 5)
	m, err := s.GetMerchantInfo(context.Background(), f.CID)
	if err != nil {
		t.Fatalf("merchant info: %v", err)
	}
	if asInt(m["food"]) != 100 || asInt(m["gold"]) != 10000 {
		t.Fatalf("init stock=%v/%v want 100/10000", m["food"], m["gold"])
	}
	// 原版怪癖：MERCHANT_*_PRICE 未 define → 值为常量名字符串，SELL 覆盖 BUY 键。
	if m["food_buy_price"] != "MERCHANT_FOOD_SELL_PRICE" {
		t.Fatalf("food_buy_price=%v want MERCHANT_FOOD_SELL_PRICE", m["food_buy_price"])
	}
}

func TestBuyFromMerchant(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)

	if err := s.BuyFromMerchant(ctx, f.UID, f.CID, 100, 0, 0, 0, 1, 2); errMsg(err) != "" {
		t.Fatalf("invalid paytype err=%q want 空消息（lang 键未定义）", errMsg(err))
	}
	if err := s.BuyFromMerchant(ctx, f.UID, f.CID, -1, 0, 0, 0, 1, 0); errMsg(err) != "请输入正常的购买数量。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// money=0 → 元宝不足。
	if err := s.BuyFromMerchant(ctx, f.UID, f.CID, 100, 0, 0, 0, 1, 0); errMsg(err) != "你的元宝数量不足，不能完成交易。\n请充值后再来支付。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	exec(t, f, "update users set money=100 where id=?", f.UID)
	// 黄金不足：food=2000000 → 0.1×2e6=200000 > 10000。
	if err := s.BuyFromMerchant(ctx, f.UID, f.CID, 2000000, 0, 0, 0, 1, 0); errMsg(err) != "本城的黄金不够。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 单笔上限：gold=1000000 时 food=6000000 单倍价 600000 > 5×100000。
	exec(t, f, "update city_resources set gold=1000000 where city_id=?", f.CID)
	if err := s.BuyFromMerchant(ctx, f.UID, f.CID, 6000000, 0, 0, 0, 1, 0); errMsg(err) != "5级市场和商人交易的单笔交易上限为500000黄金。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 成功路径（原版经 throw 返回文案）：food100 wood200 rock300 iron400 → 10+20+60+100=190。
	exec(t, f, "update city_resources set gold=10000 where city_id=?", f.CID)
	err := s.BuyFromMerchant(ctx, f.UID, f.CID, 100, 200, 300, 400, 1, 0)
	if errMsg(err) != "**购买成功！" {
		t.Fatalf("success err=%q", errMsg(err))
	}
	if v := cell(t, f, "select money from users where id=?", f.UID); v != 95 {
		t.Fatalf("money=%d want 95（服务费 5）", v)
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 10000-190 {
		t.Fatalf("gold=%d want 9810", v)
	}
	if v := cell(t, f, "select food from city_resources where city_id=?", f.CID); v != 100000+100 {
		t.Fatalf("food=%d want 100100", v)
	}
	// 怪癖：log_merchants 列序 (wood,food,iron,rock) 与实参 (food,wood,rock,iron) 错位 →
	// iron 列记 rock=300、rock 列记 iron=400。
	if v := cell(t, f, "select iron from log_merchants where user_id=? order by id desc limit 1", f.UID); v != 300 {
		t.Fatalf("log iron=%d want 300（rock 值，列错位怪癖）", v)
	}
	if v := cell(t, f, "select rock from log_merchants where user_id=? order by id desc limit 1", f.UID); v != 400 {
		t.Fatalf("log rock=%d want 400（iron 值）", v)
	}
	if v := cell(t, f, "select aid from log_user_actions where user_id=? order by id desc limit 1", f.UID); v != 19 {
		t.Fatalf("action aid=%d want 19", v)
	}
}

func TestSellToMerchant(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	exec(t, f, "update users set money=100 where id=?", f.UID)

	if err := s.SellToMerchant(ctx, f.UID, f.CID, 200000, 0, 0, 0, 1, 0); errMsg(err) != "本城的粮食不足，不能完成交易。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 成功：food100 → 10 金。
	if err := s.SellToMerchant(ctx, f.UID, f.CID, 100, 0, 0, 0, 1, 0); errMsg(err) != "**出售成功！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 10010 {
		t.Fatalf("gold=%d want 10010", v)
	}
	if v := cell(t, f, "select food from city_resources where city_id=?", f.CID); v != 99900 {
		t.Fatalf("food=%d want 99900", v)
	}
}

// ---------- 玩家挂单 / 购买 / 惰性成交 ----------

func TestSellToUserAndTradeSettle(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	s2, f2 := newSvc(t)
	addBuilding(t, f2, game.BidMarket, "a4", 5)

	if _, err := s.SellToUser(ctx, f.UID, f.CID, 0, -1, 100, 1, 0); errMsg(err) != "出售数量不正确。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.SellToUser(ctx, f.UID, f.CID, 0, 1000, 100, 0, 0); errMsg(err) != "交易时限不能少于1小时。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 价格区间：0.1 基准 → [0.079,0.121]；gold/rescount=0.5 越界。
	if _, err := s.SellToUser(ctx, f.UID, f.CID, 0, 1000, 500, 1, 0); errMsg(err) != "出售价格超出规定范围，不能出售。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	info, err := s.SellToUser(ctx, f.UID, f.CID, 0, 1000, 100, 1, 0)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if len(info.Trades) != 1 {
		t.Fatalf("trades=%d want 1", len(info.Trades))
	}
	trade := info.Trades[0]
	tradeID := int(asInt(trade["id"]))
	// distance=(3600/16.6667)²=46656（浮点：gridtime=6000/360 除不尽，保留误差）。
	if d := fcell(t, f, "select distance from city_trades where id=?", tradeID); d-46656 > 1 || 46656-d > 1 {
		t.Fatalf("distance=%v want ~46656", d)
	}
	if v := cell(t, f, "select food from city_resources where city_id=?", f.CID); v != 99000 {
		t.Fatalf("food=%d want 99000（挂单即扣）", v)
	}

	// 自己不能买自己其它城的单 → 先验证"该交易不存在"分支后再由 f2 购买。
	if _, err := s.BuyFromUser(ctx, f.UID, f.CID, 999999); errMsg(err) != "该交易不存在。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s2.BuyFromUser(ctx, f2.UID, f2.CID, tradeID); err != nil {
		t.Fatalf("buy: %v", err)
	}
	if v := cell(t, f2, "select state from city_trades where id=?", tradeID); v != 1 {
		t.Fatalf("state=%d want 1", v)
	}
	if v := cell(t, f2, "select gold from city_resources where city_id=?", f2.CID); v != 9900 {
		t.Fatalf("buyer gold=%d want 9900", v)
	}
	// 重复购买 → 已被抢先。
	if _, err := s2.BuyFromUser(ctx, f2.UID, f2.CID, tradeID); errMsg(err) != "该资源已经被其他玩家抢先购买。" {
		t.Fatalf("err=%q", errMsg(err))
	}

	// 惰性成交：把 endtime 拨到过去，下一次市场请求触发 handleTrade。
	exec(t, f, "update city_trades set endtime=unix_timestamp()-1 where id=?", tradeID)
	if _, err := s.GetMarketInfo(ctx, f.UID, f.CID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if v := cell(t, f, "select count(*) from city_trades where id=?", tradeID); v != 0 {
		t.Fatalf("trade not deleted")
	}
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 10100 {
		t.Fatalf("seller gold=%d want 10100（+100 成交货款）", v)
	}
	if v := cell(t, f2, "select food from city_resources where city_id=?", f2.CID); v != 101000 {
		t.Fatalf("buyer food=%d want 101000（+1000）", v)
	}
	// 战报：restype 0→++→索引 1="粮食"。
	content, err := f.DB.FetchCellString(ctx, "select content from reports where user_id=? and title=15 order by id desc limit 1", f.UID)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	if !strings.Contains(content, "出售：粮食 1000") || !strings.Contains(content, "获得：黄金 100") {
		t.Fatalf("seller report=%q", content)
	}
	content2, err := f2.DB.FetchCellString(ctx, "select content from reports where user_id=? and title=15 order by id desc limit 1", f2.UID)
	if err != nil {
		t.Fatalf("report2: %v", err)
	}
	if !strings.Contains(content2, "购买：粮食 1000") {
		t.Fatalf("buyer report=%q", content2)
	}
}

func TestCancelSell(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	info, err := s.SellToUser(ctx, f.UID, f.CID, 1, 1000, 100, 1, 0) // wood
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	id := int(asInt(info.Trades[0]["id"]))
	if _, err := s.CancelSell(ctx, f.UID, f.CID, 999999); errMsg(err) != "已经达成的交易不能取消。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.CancelSell(ctx, f.UID, f.CID, id); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if v := cell(t, f, "select wood from city_resources where city_id=?", f.CID); v != 100000 {
		t.Fatalf("wood=%d want 100000（挂单扣 1000、取消退 1000，结算后不变）", v)
	}
	if v := cell(t, f, "select count(*) from city_trades where id=?", id); v != 0 {
		t.Fatalf("trade not deleted")
	}
}

func TestAccelerateSell(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	if _, err := s.AccelerateSell(ctx, f.UID, f.CID, 1); errMsg(err) != "not_enough_goods11" {
		t.Fatalf("err=%q", errMsg(err))
	}
	giveGoods(t, f, game.GidMuniu, 1)
	if _, err := s.AccelerateSell(ctx, f.UID, f.CID, 999999); errMsg(err) != "指定的交易不存在，不能进行加速。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 手工 state=1 在途单（cid=1 非本城、buycid=0 → 不可加速）。
	id, err := f.DB.Insert(ctx, `insert into city_trades (cid,state,restype,`+"`count`"+`,price,gold,distance,endtime,buycid)
		values (1,1,0,100,0.1,10,1,unix_timestamp()+100000,0)`)
	if err != nil {
		t.Fatal(err)
	}
	// 该单 cid=1 非本城 → 不能加速。
	if _, err := s.AccelerateSell(ctx, f.UID, f.CID, int(id)); errMsg(err) != "指定的交易不存在，不能进行加速。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	exec(t, f, "update city_trades set cid=? where id=?", f.CID, id)
	before := cell(t, f, "select endtime from city_trades where id=?", id)
	if _, err := s.AccelerateSell(ctx, f.UID, f.CID, int(id)); err != nil {
		t.Fatalf("accelerate: %v", err)
	}
	after := cell(t, f, "select endtime from city_trades where id=?", id)
	if after >= before {
		t.Fatalf("endtime %d -> %d, want 压缩", before, after)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, game.GidMuniu); v != 0 {
		t.Fatalf("muniu=%d want 0", v)
	}
}

// ---------- 自动运输 ----------

func TestAutoTrans(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)
	cid2 := newSecondCity(t, f, 5)

	now := cell(t, f, "select unix_timestamp()")
	// 时间在过去。
	if _, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), int64(cid2), 0, 100, 0, float64(now-60)*1000); errMsg(err) != "时间设置错误" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 目标非己城。
	if _, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), 99999999, 0, 100, 0, float64(now+60)*1000); errMsg(err) != "目标城池必须是我方城池" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 数量超市场等级。
	if _, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), int64(cid2), 0, 600000, 0, float64(now+60)*1000); errMsg(err) != "运输数量不能超过出发城池市场等级的限制" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 无商队契约道具。
	if _, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), int64(cid2), 0, 100, 0, float64(now+60)*1000); errMsg(err) != "not_enough_goods120" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// HasAutoTrans：无 buff17 → [false]。
	has, err := s.HasAutoTrans(ctx, f.UID)
	if err != nil || len(has) != 1 || has[0] != false {
		t.Fatalf("has=%v err=%v", has, err)
	}

	giveGoods(t, f, game.GidShangduiQiyue, 1)
	rows, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), int64(cid2), 1, 500, 0, float64(now+60)*1000)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, game.GidShangduiQiyue); v != 0 {
		t.Fatalf("qiyue=%d want 0（已消耗）", v)
	}
	if v := cell(t, f, "select buftype from user_buffers where user_id=? and buftype=17", f.UID); v != 17 {
		t.Fatalf("buff17 not set")
	}
	has, err = s.HasAutoTrans(ctx, f.UID)
	if err != nil || len(has) != 1 || has[0] != true {
		t.Fatalf("has=%v err=%v want [true]", has, err)
	}
	// 契约失效时间检查。
	if _, err := s.AddAutoTrans(ctx, f.UID, int64(f.CID), int64(cid2), 0, 100, 0, float64(now+300*86400)*1000); errMsg(err) != "开始时间不能大于商队契约失效时间" {
		t.Fatalf("err=%q", errMsg(err))
	}

	// handleAutoTrans 原版 bug：res_type=1（木材）仍恒扣出发城粮食；trans_type=0 单次即删行。
	// 把 end_time 拨到过去（-60 容差内），下一次市场请求触发结算。
	exec(t, f, "update city_autotrans set end_time=unix_timestamp()-100 where user_id=?", f.UID)
	if _, err := s.GetMarketInfo(ctx, f.UID, f.CID); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if v := cell(t, f, "select count(*) from city_autotrans where user_id=?", f.UID); v != 0 {
		t.Fatalf("单次运输未删除")
	}
	// 出发城 food 结算后 100000 → 恒扣 500（bug：本应扣 wood）。
	if v := cell(t, f, "select food from city_resources where city_id=?", f.CID); v != 99500 {
		t.Fatalf("food=%d want 99500（恒扣粮怪癖）", v)
	}
	if v := cell(t, f, "select wood from city_resources where city_id=?", f.CID); v != 100000 {
		t.Fatalf("wood=%d want 100000（未扣）", v)
	}
	// 生成的 state=1 单 endtime 已过期 → 同一轮 handleTrade 立即成交：buycid 收木材 500。
	if v := cell(t, f, "select wood from city_resources where city_id=?", cid2); v != 100500 {
		t.Fatalf("dest wood=%d want 100500", v)
	}
}

func TestRemoveAutoTrans(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	id, err := f.DB.Insert(ctx,
		"insert into city_autotrans (user_id,fromcid,tocid,state,trans_type,start_time,distance,cost_time,res_type,`count`,end_time) values (?,?,?,0,0,0,0,0,0,0,0)",
		f.UID, f.CID, f.CID)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.RemoveAutoTrans(ctx, int(id))
	if err != nil || len(out) != 1 || out[0] != int(id) {
		t.Fatalf("out=%v err=%v", out, err)
	}
	if v := cell(t, f, "select count(*) from city_autotrans where id=?", id); v != 0 {
		t.Fatalf("not deleted")
	}
}

// ---------- 仓库 ----------

func TestStoreInfo(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidFarmland, "b2", 3) // lv*(lv+1)*50=600
	addBuilding(t, f, game.BidStore, "c2", 2)    // lv*(lv+1)*5000=30000
	giveGoods(t, f, game.GidCopper, 500)

	out, err := s.DoGetStoreInfo(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("store info: %v", err)
	}
	// foodBase=600×100×10=600000；storageTech 无 tid15 → 1；仓库容量基=10×30000。
	if asInt(out[0]) != 600000 {
		t.Fatalf("foodBase=%v want 600000", out[0])
	}
	if asInt(out[4]) != 1 {
		t.Fatalf("storageTech=%v want 1", out[4])
	}
	if asInt(out[5]) != 1 || asInt(out[6]) != 300000 {
		t.Fatalf("store cnt/cap=%v/%v want 1/300000", out[5], out[6])
	}
	if asInt(out[8]) != 500 {
		t.Fatalf("copper=%v want 500", out[8])
	}
}

func TestModifyStoreRate(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	exec(t, f, "insert ignore into city_res_add (city_id) values (?)", f.CID)

	if err := s.ModifyStoreRate(ctx, f.CID, -1, 0, 0, 0); errMsg(err) != "存放比例不能为负数！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if err := s.ModifyStoreRate(ctx, f.CID, 30, 30, 30, 30); errMsg(err) != "四项资源存放比例之和不能超过100。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if err := s.ModifyStoreRate(ctx, f.CID, 40, 20, 20, 20); errMsg(err) != "修改仓库存放比例成功！" {
		t.Fatalf("success err=%q", errMsg(err))
	}
	if v := cell(t, f, "select food_store from city_res_add where city_id=?", f.CID); v != 40 {
		t.Fatalf("food_store=%d want 40", v)
	}
	if v := cell(t, f, "select resource_changing from city_res_add where city_id=?", f.CID); v != 1 {
		t.Fatalf("resource_changing=%d want 1", v)
	}
}

func TestPayToPack(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	giveGoods(t, f, game.GidCopper, 500)

	// 资源不足。
	if _, err := s.PayToPack(ctx, f.UID, f.CID, []PackItem{{Type: "wood1", PackCount: 10, ResCount: 99999999, Copper: 5}}); errMsg(err) != "您的资源数目不足" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 铜钱不足（怪癖文案：实指铜钱却说元宝）。
	if _, err := s.PayToPack(ctx, f.UID, f.CID, []PackItem{{Type: "wood1", PackCount: 10, ResCount: 1000, Copper: 100000}}); errMsg(err) != "您的元宝数量不够" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 空入参 → legacy 直接 return。
	out, err := s.PayToPack(ctx, f.UID, f.CID, nil)
	if err != nil || out != nil {
		t.Fatalf("out=%v err=%v want nil", out, err)
	}
	// 成功：wood1 → gid92 ×10、扣木 1000、扣铜 5。
	out, err = s.PayToPack(ctx, f.UID, f.CID, []PackItem{{Type: "wood1", PackCount: 10, ResCount: 1000, Copper: 5}})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if out[0] != 0 || out[1] != 0 {
		t.Fatalf("out=%v want [0,0]", out)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, game.GidCopper); v != 495 {
		t.Fatalf("copper=%d want 495", v)
	}
	// 怪癖：addGoods(152) 双记账 → 888888 入账扣减后的铜钱总数。
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, game.GidPoint); v != 495 {
		t.Fatalf("888888=%d want 495（双记账怪癖）", v)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, 92); v != 10 {
		t.Fatalf("wood1 pack=%d want 10", v)
	}
	if v := cell(t, f, "select wood from city_resources where city_id=?", f.CID); v != 99000 {
		t.Fatalf("wood=%d want 99000", v)
	}
}

// ---------- 工匠作坊 ----------

func TestWorkshopFlow(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 无货架：[leavTime=0]。
	out, err := s.LoadInitWorkShopInfo(ctx, f.UID, f.CID)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if len(out) != 1 || asInt(out[0]) != 0 {
		t.Fatalf("out=%v want [0]", out)
	}
	// 无作坊建筑。
	if _, err := s.RefreshWorkShop(ctx, f.UID, f.CID, 0); errMsg(err) != "当前城池没有工匠作坊！！！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.RefreshWorkShop(ctx, f.UID, f.CID, 2); errMsg(err) != "数据异常！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	addBuilding(t, f, game.BidWorkshop, "c4", 1)
	// 五铢钱刷新但余额不足。
	if _, err := s.RefreshWorkShop(ctx, f.UID, f.CID, 1); errMsg(err) != "您的五铢钱不够！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 免费刷新：lv1 → 1 件宝珠。
	out, err = s.RefreshWorkShop(ctx, f.UID, f.CID, 0)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(out) != 4 { // [leavTime, goodCnt, good, price]
		t.Fatalf("out len=%d want 4", len(out))
	}
	if asInt(out[0]) < 7190 || asInt(out[0]) > 7200 {
		t.Fatalf("leavTime=%v want ~7200", out[0])
	}
	if asInt(out[1]) != 1 {
		t.Fatalf("goodCnt=%v want 1", out[1])
	}
	gidStr, err := f.DB.FetchCellString(ctx, "select gidstr from user_workshops where user_id=?", f.UID)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(gidStr, ",")
	gid, _ := strconv.Atoi(parts[1])
	price, _ := strconv.Atoi(parts[2])
	if gid < 300 || gid > 374 || price < 10 || price > 120 {
		t.Fatalf("gidstr=%q out of gem range", gidStr)
	}

	// 购买：礼金不足（严格大于怪癖）。
	if _, err := s.BuyWorkShopGood(ctx, f.UID, f.CID, gid); errMsg(err) != "您的礼金不够！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	exec(t, f, "update users set gift=1000 where id=?", f.UID)
	if _, err := s.BuyWorkShopGood(ctx, f.UID, f.CID, 999999); errMsg(err) != "数据异常！" {
		t.Fatalf("err=%q", errMsg(err))
	}
	out, err = s.BuyWorkShopGood(ctx, f.UID, f.CID, gid)
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if out[0] != "购买成功！" {
		t.Fatalf("out0=%v", out[0])
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, gid); v != 1 {
		t.Fatalf("gem count=%d want 1", v)
	}
	if v := cell(t, f, "select gift from users where id=?", f.UID); v != int64(1000-price) {
		t.Fatalf("gift=%d want %d", v, 1000-price)
	}
	// 购买后 gidstr 计数 -1（末对残留尾逗号怪癖：剩 "0,"）。
	gidStr, _ = f.DB.FetchCellString(ctx, "select gidstr from user_workshops where user_id=?", f.UID)
	if !strings.HasPrefix(gidStr, "0,") {
		t.Fatalf("gidstr=%q want 计数 0", gidStr)
	}

	// 付费刷新扣五铢 20。
	giveGoods(t, f, game.GidWuzhuqian, 30)
	if _, err := s.RefreshWorkShop(ctx, f.UID, f.CID, 1); err != nil {
		t.Fatalf("paid refresh: %v", err)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, game.GidWuzhuqian); v != 10 {
		t.Fatalf("wuzhu=%d want 10", v)
	}
	// 刷新上限。
	exec(t, f, "update user_workshops set `count`=50 where user_id=?", f.UID)
	if _, err := s.RefreshWorkShop(ctx, f.UID, f.CID, 0); errMsg(err) != "今天立即刷新次数达到上限，请明天再来" {
		t.Fatalf("err=%q", errMsg(err))
	}
}

// ---------- 商城 ----------

func TestShopInfoAndBuy(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	out, err := s.GetShopInfo(ctx, f.UID)
	if err != nil {
		t.Fatalf("shop info: %v", err)
	}
	if len(out) != 6 {
		t.Fatalf("len=%d want 6", len(out))
	}
	if rows, ok := out[2].([]map[string]any); !ok || len(rows) < 5 {
		t.Fatalf("常规商品=%v want ≥5（合成种子）", out[2])
	}

	if _, err := s.BuyGoods(ctx, f.UID, 1, 1, 2, 0); errMsg(err) != "" {
		t.Fatalf("paytype err=%q want 空", errMsg(err))
	}
	if _, err := s.BuyGoods(ctx, f.UID, 1, 0, 0, 0); errMsg(err) != "购买数量无效。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.BuyGoods(ctx, f.UID, 999999, 1, 0, 0); errMsg(err) != "此商品已经停售。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// money=0 → 不足。
	if _, err := s.BuyGoods(ctx, f.UID, 1, 1, 0, 0); errMsg(err) != "你的元宝不足，请充值。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	exec(t, f, "update users set money=1000 where id=?", f.UID)
	// 聚贤包爵位限制。
	if _, err := s.BuyGoods(ctx, f.UID, 121, 1, 0, 0); errMsg(err) != "只有爵位达到“公士”才能购买和使用“聚贤包”。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 特价商品不可用礼金购买。
	if _, err := s.BuyGoods(ctx, f.UID, 202, 1, 1, 0); errMsg(err) != "特价商品不可用礼金购买" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 每人限购（怪癖文案第二参用个人累计 buycnt=0）。
	if _, err := s.BuyGoods(ctx, f.UID, 201, 4, 0, 0); errMsg(err) != "购买数量无效。此商品每人限购3个，你已经购买0个，只能再购买3个此商品。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 正常购买 id=1（gid1 price1 pack1）。
	ret, err := s.BuyGoods(ctx, f.UID, 1, 1, 0, 0)
	if err != nil {
		t.Fatalf("buy: %v", err)
	}
	if asInt(ret[0]) != 0 || asInt(ret[1]) != 999 {
		t.Fatalf("ret=%v want [0,999]", ret)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=?", f.UID, 1); v != 1 {
		t.Fatalf("gid1=%d want 1", v)
	}
	if v := cell(t, f, "select last_pay from users where id=?", f.UID); v != 0 {
		t.Fatalf("last_pay=%d want 0", v)
	}
	// 限量商品成功：id=201 cnt=1。
	ret, err = s.BuyGoods(ctx, f.UID, 201, 1, 0, 0)
	if err != nil {
		t.Fatalf("buy 201: %v", err)
	}
	if asInt(ret[1]) != 899 {
		t.Fatalf("money left=%v want 899", ret[1])
	}
	if v := cell(t, f, "select `count` from log_shop_buy_cnts where user_id=? and sid=?", f.UID, 201); v != 1 {
		t.Fatalf("buy cnt=%d want 1", v)
	}

	// buyGoodsBeforeUse：paytype=1 只卖 commend 1/2 → id1 不可见。
	if _, err := s.BuyGoodsBeforeUse(ctx, f.UID, 1, 1, 1); errMsg(err) != "此商品已经停售。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	ret, err = s.BuyGoodsBeforeUse(ctx, f.UID, 1, 1, 0)
	if err != nil {
		t.Fatalf("before-use: %v", err)
	}
	if asInt(ret[0]) != 1 || asInt(ret[1]) != 0 || asInt(ret[2]) != 898 {
		t.Fatalf("ret=%v want [1,0,898]", ret)
	}

	// 属性提示降级。
	if err := s.GetGoodsHeroAttr(ctx, 1); errMsg(err) != "没有属性提示" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.GetGoodsArmorAttr(ctx, 999999); errMsg(err) != "没有属性提示" {
		t.Fatalf("err=%q", errMsg(err))
	}
}

func TestExchangeLiquan(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := s.ExchangeLiquan(ctx, f.UID, "  "); errMsg(err) != "礼券码不能为空。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.ExchangeLiquan(ctx, f.UID, "short"); errMsg(err) != "礼券码无效。请重新输入正确的礼券码。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	if _, err := s.ExchangeLiquan(ctx, f.UID, "LQA0123456789"); errMsg(err) != "礼券码无效。请重新输入正确的礼券码。" {
		t.Fatalf("err=%q", errMsg(err))
	}

	// 自建 LQA 礼券（contentid=1 种子内容：gid12×2、铜钱152×100、宝石300×1）。
	code := fmt.Sprintf("LQA%07d", f.UID%10000000)
	id, err := f.DB.Insert(ctx, "insert into tickets (code, user_id, binduid, contentid, time) values (?,0,0,1,0)", code)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.DB.Exec(ctx, "delete from tickets where id=?", id)
	})

	ret, err := s.ExchangeLiquan(ctx, f.UID, code)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	list := ret[0].([]map[string]any)
	if len(list) != 3 {
		t.Fatalf("reward list=%d want 3", len(list))
	}
	if v := cell(t, f, "select user_id from tickets where id=?", id); v != int64(f.UID) {
		t.Fatalf("ticket not bound")
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=12", f.UID); v != 2 {
		t.Fatalf("免战牌=%d want 2", v)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=300", f.UID); v != 1 {
		t.Fatalf("宝石=%d want 1", v)
	}
	// 铜钱 100 + 双记账 888888=100。
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=152", f.UID); v != 100 {
		t.Fatalf("铜钱=%d want 100", v)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=888888", f.UID); v != 100 {
		t.Fatalf("888888=%d want 100（双记账怪癖）", v)
	}

	// 同码重复 → 已被使用。
	if _, err := s.ExchangeLiquan(ctx, f.UID, code); errMsg(err) != "该礼券码已被使用。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// LQA 前缀每账号限 1 次。
	code2 := fmt.Sprintf("LQA%07d", f.UID%10000000+1)
	id2, err := f.DB.Insert(ctx, "insert into tickets (code, user_id, binduid, contentid, time) values (?,0,0,1,0)", code2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.DB.Exec(ctx, "delete from tickets where id=?", id2) })
	if _, err := s.ExchangeLiquan(ctx, f.UID, code2); errMsg(err) != "该类型礼券一个账号只能使用1个，您已经使用了1个，不能再使用了。" {
		t.Fatalf("err=%q", errMsg(err))
	}
}

// ---------- 宝物出售 / 劳力比例 ----------

func TestSellGoods(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := s.SellGoods(ctx, f.UID, f.CID, 160052, 1); errMsg(err) != "无法回收" {
		t.Fatalf("err=%q", errMsg(err))
	}
	// 无市场 → level 0 < 5。
	if _, err := s.SellGoods(ctx, f.UID, f.CID, 300, 1); errMsg(err) != "市场等级达到5级才能出售宝物。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	addBuilding(t, f, game.BidMarket, "a4", 5)
	// 爵位不足（nobility '' → 0）。
	if _, err := s.SellGoods(ctx, f.UID, f.CID, 300, 1); errMsg(err) != "爵位达到“公士”才能出售宝物。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	exec(t, f, "update users set nobility='1' where id=?", f.UID)
	if _, err := s.SellGoods(ctx, f.UID, f.CID, 300, 5); errMsg(err) != "你没有那么多道具，请正确填写道具数量。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	giveGoods(t, f, 300, 2)
	ret, err := s.SellGoods(ctx, f.UID, f.CID, 300, 1)
	if err != nil {
		t.Fatalf("sell: %v", err)
	}
	if asInt(ret[1]) != int64(f.CID) {
		t.Fatalf("ret=%v", ret)
	}
	if v := cell(t, f, "select `count` from user_goods where user_id=? and gid=300", f.UID); v != 1 {
		t.Fatalf("gem=%d want 1", v)
	}
	// cfg_goods.value=0（合成种子）→ goldAdd=0，黄金不变。
	if v := cell(t, f, "select gold from city_resources where city_id=?", f.CID); v != 10000 {
		t.Fatalf("gold=%d want 10000", v)
	}
}

func TestSetCityProductRate(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	_, f2 := newSvc(t) // 他人城
	exec(t, f, "insert ignore into city_res_add (city_id) values (?)", f.CID)

	if _, err := s.SetCityProductRate(ctx, f.UID, f2.CID, 25, 25, 25, 25); errMsg(err) != "你没有权限进行该项操作。" {
		t.Fatalf("err=%q", errMsg(err))
	}
	out, err := s.SetCityProductRate(ctx, f.UID, f.CID, 50, 20, 15, 15)
	if err != nil {
		t.Fatalf("rate: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("out=%v want []", out)
	}
	if v := cell(t, f, "select food_rate from city_res_add where city_id=?", f.CID); v != 50 {
		t.Fatalf("food_rate=%d want 50", v)
	}
}

// ---------- 买列表 ----------

func TestUserBuyListAndSellData(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	addBuilding(t, f, game.BidMarket, "a4", 5)

	out, err := s.GetUserBuyList(ctx, f.UID, f.CID, 0, 0, 0, "")
	if err != nil {
		t.Fatalf("buylist: %v", err)
	}
	list := out.([]any)
	if asInt(list[0]) != 0 || asInt(list[1]) != 5 {
		t.Fatalf("out=%v want [0,5,...]", list[:2])
	}
	if rows := list[4].([]map[string]any); len(rows) != 0 {
		t.Fatalf("rows=%d want 0", len(rows))
	}

	sd, err := s.GetUserSellData(ctx, f.CID)
	if err != nil {
		t.Fatalf("selldata: %v", err)
	}
	if asInt(sd[0]) != 0 || asInt(sd[1]) != 5 || fmt.Sprint(sd[2]) != "0.1" {
		t.Fatalf("selldata=%v", sd)
	}
}
