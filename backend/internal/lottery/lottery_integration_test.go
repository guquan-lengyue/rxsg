//go:build integration

package lottery

import (
	"context"
	"math/rand"
	"strconv"
	"testing"

	"rxsg/backend/internal/testutil"
)

// lottery_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：checkLottoryTime 恒抛（原版怪癖）/ 开关 / getGoods 生成 8 格盘面 / startLottery 免费+付费扣道具
//   与次数上限 / randWin 去重怪癖 / getGoodsByType 级别分布 / getWin 道具+装备+重复领取 / getLotteryReward /
//   autoGetReward / getTodayCount / restart 重开上限 / useMoney 扣元宝 / checkLotteryMoney。

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

func cellStr(t *testing.T, f *testutil.Fixture, q string, args ...any) string {
	t.Helper()
	v, err := f.DB.FetchCellString(context.Background(), q, args...)
	if err != nil {
		t.Fatalf("cellStr %q: %v", q, err)
	}
	return v
}

func setGoods(t *testing.T, f *testutil.Fixture, gid int, count int64) {
	t.Helper()
	ex(t, f, "insert into user_goods (user_id,gid,`count`) values (?,?,?) on duplicate key update `count`=?", f.UID, gid, count, count)
}

func setNobility(t *testing.T, f *testutil.Fixture, n string) {
	t.Helper()
	ex(t, f, "update users set nobility=? where id=?", n, f.UID)
}

// TestCheckLottoryTimeDefaultThrows 原版怪癖：时间窗被注释禁用 → 恒抛 not_available_time。
func TestCheckLottoryTimeDefaultThrows(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	setNobility(t, f, "1")

	if _, err := svc.StartLottery(ctx, f.UID); err == nil || err.Error() != msgNotAvailableT {
		t.Fatalf("默认 checkLottoryTime 恒抛 not_available_time，got err=%v", err)
	}
	if _, err := svc.GetLotteryReward(ctx, f.UID); err == nil || err.Error() != msgNotAvailableT {
		t.Fatalf("getLotteryReward 亦应恒抛 not_available_time，got %v", err)
	}
}

// TestGetGoodsCreatesBoard8Cells getGoods 生成盘面：8 档、合计 8 格、7/8 等仅道具。
func TestGetGoodsCreatesBoard8Cells(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	svc.SetSeed(20240101)

	ret, err := svc.GetGoods(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 6 {
		t.Fatalf("getGoods 返回 %d 元素 want 6", len(ret))
	}
	records, ok := ret[0].([][]map[string]any)
	if !ok || len(records) != 8 {
		t.Fatalf("records 应为 8 档，got %T len=%d", ret[0], len(records))
	}
	total := 0
	for i, bucket := range records {
		total += len(bucket)
		if i >= 6 { // 7 等(索引6)/8 等(索引7) 强制 type=1 → 必为道具（含 gid）
			for _, g := range bucket {
				if _, hasGid := g["gid"]; !hasGid {
					t.Fatalf("第 %d 档不应出现装备（无 gid）: %v", i+1, g)
				}
			}
		}
	}
	if total != 8 {
		t.Fatalf("盘面合计 %d 格 want 8（is_last 补齐）", total)
	}
	// 盘面已入库，win 初值 '-1,0,0'，got=0
	if w := cellStr(t, f, "select win from mem_lottery_goods where uid=?", f.UID); w != "-1,0,0" {
		t.Fatalf("win=%q want -1,0,0", w)
	}
	if g := cell(t, f, "select got from mem_lottery_goods where uid=?", f.UID); g != 0 {
		t.Fatalf("got=%d want 0", g)
	}
}

// TestStartLotteryFreePayAndLimit 免费首抽 → 付费抽（扣道具）→ 无道具/达上限。
func TestStartLotteryFreePayAndLimit(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	setNobility(t, f, "1")
	svc.SetTimeGate(true)
	svc.SetSeed(7)

	if _, err := svc.GetGoods(ctx, f.UID); err != nil { // 先建盘面（got=0）
		t.Fatal(err)
	}

	// 第一次免费（count=0 < 1）
	ret, err := svc.StartLottery(ctx, f.UID)
	if err != nil {
		t.Fatalf("first startLottery: %v", err)
	}
	if len(ret) != 4 || ret[3].(int) != 1 {
		t.Fatalf("first ret=%v want 4 元素且 todayCount=1", ret)
	}
	if n := cell(t, f, "select count(*) from log_lottery where uid=?", f.UID); n != 1 {
		t.Fatalf("log_lottery=%d want 1", n)
	}
	if w := cellStr(t, f, "select win from mem_lottery_goods where uid=?", f.UID); w == "-1,0,0" {
		t.Fatalf("startLottery 后 win 应被写入，got %q", w)
	}

	// 第二次（count=1）无付费道具 → [-1,-2,-1,1]
	ret, err = svc.StartLottery(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 4 || ret[0].(int) != -1 || ret[1].(int) != -2 {
		t.Fatalf("使用次数达上限 ret=%v want [-1,-2,-1,1]", ret)
	}

	// 给"幸运宝盒机会(19989)" → 付费抽一次并扣除道具
	setGoods(t, f, 19989, 1)
	ret, err = svc.StartLottery(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ret) != 4 || ret[3].(int) != 2 {
		t.Fatalf("付费抽 ret=%v want todayCount=2", ret)
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=19989", f.UID); g != 0 {
		t.Fatalf("19989 剩余=%d want 0（扣 1）", g)
	}
}

// TestStartLotteryNobilityLimit 爵位不足报错文案。
func TestStartLotteryNobilityLimit(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	svc.SetTimeGate(true)
	setNobility(t, f, "") // 爵位 0
	if _, err := svc.StartLottery(ctx, f.UID); err == nil || err.Error() != msgNobilityLimit {
		t.Fatalf("want nobility_limit, got %v", err)
	}
}

// TestRandWinDedupQuirk 原版怪癖：$last_win_id/$last_win_type 去重——绝不返回与上次完全相同的 (id,type)。
func TestRandWinDedupQuirk(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	// 造盘面：8 档各 1 个道具（gid 9001..9008 对应 level 1..8），并写一行盘面供 randWin 落库。
	ex(t, f, "insert into mem_lottery_goods (uid,records,`time`,win,got,restart_count) values (?,'',current_date(),'-1,0,0',0,0)", f.UID)
	records := svc.retrieveGoods(ctx, "0,9001,1,0,9002,1,0,9003,1,0,9004,1,0,9005,1,0,9006,1,0,9007,1,0,9008,1")
	for i := range records {
		if len(records[i]) != 1 {
			t.Fatalf("bucket %d len=%d want 1", i, len(records[i]))
		}
	}

	// 上次中奖 = 9008(8 等)。若去重失效，winIndex=7 时会返回 9008。
	for s := int64(0); s < 200; s++ {
		svc.SetSeed(s)
		got, err := svc.randWin(ctx, f.UID, records, false, 9008, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got[0] == 0 && got[1] == 9008 {
			t.Fatalf("randWin seed=%d 返回了上次同款 (type=%d,id=%d) → 去重失效", s, got[0], got[1])
		}
	}
	// winIndex=7 时应自动落到次优的 9007（单元素档去重后向下找）
	svc.SetSeed(seedForIndex7())
	got, err := svc.randWin(ctx, f.UID, records, false, 9008, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != 0 || got[1] != 9007 {
		t.Fatalf("去重预期落到 9007，got type=%d id=%d", got[0], got[1])
	}
}

// seedForIndex7 找一个使首次 mt_rand(1,10000) 落入 8 等档(winRand>8000) 的种子。
func seedForIndex7() int64 {
	for s := int64(0); s < 100000; s++ {
		r := rand.New(rand.NewSource(s))
		if r.Intn(10000)+1 > 8000 {
			return s
		}
	}
	return 0
}

// TestGetGoodsByTypeLevels 级别/类型分布规则：7/8 等强制道具；材料档 group_id∈{4,5}。
func TestGetGoodsByTypeLevels(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	svc.SetSeed(99)

	total := 0
	for level := 1; level <= 8; level++ {
		tv := 0
		bucket := svc.getGoodsByType(ctx, 2, level, &tv, false)
		for _, g := range bucket {
			if level == 7 || level == 8 {
				if _, hasGid := g["gid"]; !hasGid {
					t.Fatalf("level=%d 应仅为道具（group_id∈0..3）", level)
				}
			}
			// 道具/材料行的 level 必等于请求档位
			if lv := g["level"]; lv != nil {
				if n, _ := strconv.Atoi(toString(lv)); n != level {
					t.Fatalf("level=%d 取到 level=%v 的行", level, lv)
				}
			}
		}
		total += len(bucket)
	}
	_ = total
	_ = f
}

func toString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case int:
		return strconv.Itoa(t)
	default:
		return ""
	}
}

// TestGetWinGoodsArmorAndDuplicate 道具/装备发奖 + 重复领取被拒。
func TestGetWinGoodsArmorAndDuplicate(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	svc.SetTimeGate(true)

	// 道具：win='0,9001,3'
	ex(t, f, "insert into mem_lottery_goods (uid,records,`time`,win,got,restart_count) values (?,'',current_date(),'0,9001,3',0,0)", f.UID)
	w, err := svc.AddCount(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if toString(w["gid"]) != "9001" {
		t.Fatalf("winObj.gid=%v want 9001", w["gid"])
	}
	if g := cell(t, f, "select `count` from user_goods where user_id=? and gid=9001", f.UID); g != 3 {
		t.Fatalf("道具 9001=%d want 3", g)
	}
	if g := cell(t, f, "select got from mem_lottery_goods where uid=?", f.UID); g != 1 {
		t.Fatalf("got=%d want 1", g)
	}
	// 重复领取：board 已 got=1 → 无 got=0 行 → 数据异常
	if _, err := svc.AddCount(ctx, f.UID); err == nil || err.Error() != msgWaiguaInvalid {
		t.Fatalf("重复领取 want 数据异常, got %v", err)
	}

	// 装备：win='1,91101,1'（重开一盘）
	ex(t, f, "update mem_lottery_goods set win='1,91101,1', got=0 where uid=?", f.UID)
	before := cell(t, f, "select count(*) from user_armors where user_id=? and armorid=91101", f.UID)
	if _, err := svc.AddCount(ctx, f.UID); err != nil {
		t.Fatal(err)
	}
	after := cell(t, f, "select count(*) from user_armors where user_id=? and armorid=91101", f.UID)
	if after != before+1 {
		t.Fatalf("装备 91101 数量 %d→%d want +1", before, after)
	}
}

// TestGetLotteryRewardAndAuto 领奖重开盘面 + autoGetReward 结构。
func TestGetLotteryRewardAndAuto(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	setNobility(t, f, "1")
	svc.SetTimeGate(true)
	svc.SetSeed(123)

	if _, err := svc.GetGoods(ctx, f.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartLottery(ctx, f.UID); err != nil { // 产生 win
		t.Fatal(err)
	}
	winStr := cellStr(t, f, "select win from mem_lottery_goods where uid=?", f.UID)
	parts := splitCSV(winStr)
	typ, _ := strconv.Atoi(parts[0])
	gid, _ := strconv.Atoi(parts[1])
	cnt, _ := strconv.Atoi(parts[2])
	var goodsBefore, armorBefore int64
	if typ == 0 && gid > 0 {
		goodsBefore = cell(t, f, "select coalesce((select `count` from user_goods where user_id=? and gid=?),0)", f.UID, gid)
	} else if typ == 1 && gid > 0 {
		armorBefore = cell(t, f, "select count(*) from user_armors where user_id=? and armorid=?", f.UID, gid)
	}

	ret, err := svc.GetLotteryReward(ctx, f.UID)
	if err != nil {
		t.Fatalf("getLotteryReward: %v", err)
	}
	if len(ret) != 6 {
		t.Fatalf("getLotteryReward 返回 %d 元素 want 6（getGoods 结构）", len(ret))
	}
	// 领奖后重开盘面：got 复位 0、win 复位 '-1,0,0'
	if g := cell(t, f, "select got from mem_lottery_goods where uid=?", f.UID); g != 0 {
		t.Fatalf("领奖后 got=%d want 0（重开）", g)
	}
	if w := cellStr(t, f, "select win from mem_lottery_goods where uid=?", f.UID); w != "-1,0,0" {
		t.Fatalf("领奖后 win=%q want -1,0,0", w)
	}
	// 奖励入包
	if typ == 0 && gid > 0 {
		after := cell(t, f, "select coalesce((select `count` from user_goods where user_id=? and gid=?),0)", f.UID, gid)
		if after != goodsBefore+int64(cnt) {
			t.Fatalf("道具 %d：%d→%d want +%d", gid, goodsBefore, after, cnt)
		}
	} else if typ == 1 && gid > 0 {
		after := cell(t, f, "select count(*) from user_armors where user_id=? and armorid=?", f.UID, gid)
		if after != armorBefore+1 {
			t.Fatalf("装备 %d：%d→%d want +1", gid, armorBefore, after)
		}
	}

	// autoGetReward：清空 log_lottery 使 count 归 0（重新免费），启动一次产生 win，再自动领
	ex(t, f, "delete from log_lottery where uid=?", f.UID)
	if _, err := svc.StartLottery(ctx, f.UID); err != nil {
		t.Fatal(err)
	}
	auto, err := svc.AutoGetReward(ctx, f.UID)
	if err != nil {
		t.Fatalf("autoGetReward: %v", err)
	}
	if len(auto) != 2 {
		t.Fatalf("autoGetReward 返回 %d 元素 want 2", len(auto))
	}
	if ng, ok := auto[1].([]any); !ok || len(ng) != 6 {
		t.Fatalf("autoGetReward[1] 应为 getGoods 结果，got %v", auto[1])
	}
}

// TestTodayCountRestartUseMoney getTodayCount 统计 / restart 上限 / useMoney 扣元宝。
func TestTodayCountRestartUseMoney(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()
	svc.SetTimeGate(true)

	// getTodayCount：近 1 小时
	if n, _ := svc.GetTodayCount(ctx, f.UID); n != 0 {
		t.Fatalf("初始 todayCount=%d want 0", n)
	}
	ex(t, f, "insert into log_lottery (uid,`time`,gid,`type`) values (?,NOW(),9001,0)", f.UID)
	ex(t, f, "insert into log_lottery (uid,`time`,gid,`type`) values (?,NOW(),9002,0)", f.UID)
	ex(t, f, "insert into log_lottery (uid,`time`,gid,`type`) values (?,FROM_UNIXTIME(UNIX_TIMESTAMP()-4000),9003,0)", f.UID)
	if n, _ := svc.GetTodayCount(ctx, f.UID); n != 2 {
		t.Fatalf("todayCount=%d want 2（仅近 1 小时 2 条）", n)
	}

	// restart：restart_count<1 可重开一次，之后拒绝
	svc.SetSeed(555)
	if _, err := svc.GetGoods(ctx, f.UID); err != nil {
		t.Fatal(err)
	}
	w := splitCSV(cellStr(t, f, "select win from mem_lottery_goods where uid=?", f.UID))
	wid, _ := strconv.Atoi(w[1])
	wtype, _ := strconv.Atoi(w[0])
	if _, err := svc.Restart(ctx, f.UID, wid, wtype); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if rc := cell(t, f, "select restart_count from mem_lottery_goods where uid=?", f.UID); rc != 1 {
		t.Fatalf("restart_count=%d want 1", rc)
	}
	if _, err := svc.Restart(ctx, f.UID, wid, wtype); err == nil || err.Error() != msgRestartLimit {
		t.Fatalf("二次 restart want restart_limit, got %v", err)
	}

	// useMoney：count<=1 不扣；count>1 扣 6；不足返回 [0]
	ex(t, f, "update users set money=100 where id=?", f.UID)
	ret, err := svc.UseMoney(ctx, f.UID, 1)
	if err != nil || len(ret) != 1 || ret[0].(int) != 1 {
		t.Fatalf("useMoney count=1 want [1], got %v err=%v", ret, err)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 100 {
		t.Fatalf("money=%d want 100（不扣）", m)
	}
	ret, _ = svc.UseMoney(ctx, f.UID, 2)
	if len(ret) != 1 || ret[0].(int) != 1 {
		t.Fatalf("useMoney count=2 want [1], got %v", ret)
	}
	if m := cell(t, f, "select money from users where id=?", f.UID); m != 94 {
		t.Fatalf("money=%d want 94（扣 6）", m)
	}
	ex(t, f, "update users set money=0 where id=?", f.UID)
	ret, _ = svc.UseMoney(ctx, f.UID, 2)
	if len(ret) != 1 || ret[0].(int) != 0 {
		t.Fatalf("useMoney 元宝不足 want [0], got %v", ret)
	}
}

// TestCheckLotteryMoney 元宝阈值 6。
func TestCheckLotteryMoney(t *testing.T) {
	svc, f := newSvc(t)
	ctx := context.Background()

	ex(t, f, "update users set money=5 where id=?", f.UID)
	if ok, _ := svc.CheckLotteryMoney(ctx, f.UID); ok {
		t.Fatal("money=5 want false")
	}
	ex(t, f, "update users set money=6 where id=?", f.UID)
	if ok, _ := svc.CheckLotteryMoney(ctx, f.UID); !ok {
		t.Fatal("money=6 want true")
	}
	ex(t, f, "update users set money=0 where id=?", f.UID)
	if ok, _ := svc.CheckLotteryMoney(ctx, f.UID); ok {
		t.Fatal("money=0 want false")
	}
}

func splitCSV(s string) []string {
	out := []string{}
	cur := ""
	for _, c := range s {
		if c == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(c)
	}
	out = append(out, cur)
	return out
}
