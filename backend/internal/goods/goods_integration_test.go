//go:build integration

package goods

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"rxsg/backend/internal/testutil"
)

// M2 道具系统真库集成测试：useGoods 分发逐分支与 GoodsFunc.php 1:1 对照，
// 含原版 bug/怪癖断言（徭役令 on-duplicate 只 +259200、0706 八进制=454、
// 10308 第5占位错写"第10名"、lang 缺键→空串、dispatch 不路由 10083/10084）。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	s := NewService(d)
	ctx := context.Background()
	for _, q := range []string{
		"delete from user_goods where user_id=?",
		"delete from log_goods where user_id=?",
		"delete from user_buffers where user_id=?",
		"delete from user_action_log where user_id=?",
		"delete from user_schedule where user_id=?",
	} {
		if _, err := d.Exec(ctx, q, f.UID); err != nil {
			t.Fatalf("cleanup %s: %v", q, err)
		}
	}
	if _, err := d.Exec(ctx, "delete from city_buffers where city_id=?", f.CID); err != nil {
		t.Fatalf("cleanup city_buffers: %v", err)
	}
	return s, f
}

func grant(t *testing.T, f *testutil.Fixture, gid int, cnt int64) {
	t.Helper()
	if _, err := f.DB.Exec(context.Background(),
		"insert into user_goods (user_id, gid, `count`) values (?,?,?) "+
			"on duplicate key update `count`=?", f.UID, gid, cnt, cnt); err != nil {
		t.Fatalf("grant %d: %v", gid, err)
	}
}

func countOf(t *testing.T, f *testutil.Fixture, gid int) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(context.Background(),
		"select `count` from user_goods where user_id=? and gid=?", f.UID, gid)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			return 0
		}
		t.Fatalf("count %d: %v", gid, err)
	}
	return v
}

// ---------- 入口校验（GoodsFunc.php:30-56，先于任何分支） ----------

func TestUseGoodsEntryChecks(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// useCount>=100 → 最高次数为99次！（最先检查）
	grant(t, f, 56, 200)
	if _, err := s.UseGoods(ctx, f.UID, 56, 0, 100); err == nil ||
		err.Error() != "最高次数为99次！" {
		t.Fatalf("useCount=100 err=%v", err)
	}

	// 武魂段数量为 0 → 专用文案
	if _, err := s.UseGoods(ctx, f.UID, 110001, 0, 1); err == nil ||
		err.Error() != "你使用的武魂数量为0，请合成后再使用。" {
		t.Fatalf("wuhun zero err=%v", err)
	}

	// notEnoughGIDs 集合 → "not_enough_goods$gid" 字面量
	if _, err := s.UseGoods(ctx, f.UID, 2, 0, 1); err == nil ||
		err.Error() != "not_enough_goods2" {
		t.Fatalf("notEnough set err=%v", err)
	}

	// 普通道具数量为 0 → no_this_good
	if _, err := s.UseGoods(ctx, f.UID, 999999, 0, 1); err == nil ||
		err.Error() != "你拥有的该道具数量为0，请去商城购买后再使用。" {
		t.Fatalf("no_this_good err=%v", err)
	}
}

// ---------- mode2：徭役令（原版 bug：gid56 的 useCount 被 dispatch 硬编码为 1；on-duplicate 固定 +259200） ----------

func TestYaoYiLinOnDuplicateBug(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// PHP:246 useYaoYiLin($uid) → useYaoYiLinAll($uid,56,1)：入参 useCount 被丢弃（原版怪癖）
	grant(t, f, 56, 10)
	res, err := s.UseGoods(ctx, f.UID, 56, 0, 3)
	if err != nil {
		t.Fatalf("first use: %v", err)
	}
	if res.Mode != 2 || res.Message != "“徭役令”有效期截止到" {
		t.Fatalf("mode=%d msg=%q", res.Mode, res.Message)
	}
	now, _ := f.DB.Now(ctx)
	if left := res.Endtime - now; left < 259200-60 || left > 259200+60 {
		t.Fatalf("first endtime left=%d want ~259200（useCount 被硬编码 1）", left)
	}
	if c := countOf(t, f, 56); c != 9 {
		t.Fatalf("count=%d want 9（只扣 1）", c)
	}
	first := res.Endtime

	// 二次 → on-duplicate 固定 +259200
	res2, err := s.UseGoods(ctx, f.UID, 56, 0, 5)
	if err != nil {
		t.Fatalf("second use: %v", err)
	}
	if res2.Endtime-first != 259200 {
		t.Fatalf("delta=%d want 259200（原版 on-duplicate bug）", res2.Endtime-first)
	}
	if c := countOf(t, f, 56); c != 8 {
		t.Fatalf("count=%d want 8", c)
	}
}

// ---------- mode2：高级徭役令 166（useCount 生效：insert ×useCount、update 固定 +259200 原版 bug） ----------

func TestYaoYiLinGaojiUseCount(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 166, 10)
	res, err := s.UseGoods(ctx, f.UID, 166, 0, 3)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	now, _ := f.DB.Now(ctx)
	if left := res.Endtime - now; left < 777600-60 || left > 777600+60 {
		t.Fatalf("left=%d want ~777600（259200×3）", left)
	}
	res2, err := s.UseGoods(ctx, f.UID, 166, 0, 5)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res2.Endtime-res.Endtime != 259200 { // on-duplicate 固定单份（原版 bug）
		t.Fatalf("delta=%d want 259200", res2.Endtime-res.Endtime)
	}
	if c := countOf(t, f, 166); c != 2 {
		t.Fatalf("count=%d want 2", c)
	}
}

// ---------- mode2：神农锄（资源 +500×useCount 进 lastcid 城） ----------

func TestShenNongChu(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 2, 5)
	res, err := s.UseGoods(ctx, f.UID, 2, 0, 2)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Mode != 2 || res.Message != "神农锄有效期截止到" {
		t.Fatalf("mode=%d msg=%q", res.Mode, res.Message)
	}
	food, err := f.DB.FetchCellInt64(ctx, "select food from city_resources where city_id=?", f.CID)
	if err != nil {
		t.Fatal(err)
	}
	// PHP:1800 addCityResources 固定 500（不乘 useCount）
	if food != 100500 {
		t.Fatalf("food=%d want 100500", food)
	}
	if _, err := f.DB.FetchCellInt64(ctx,
		"select endtime from user_buffers where user_id=? and buftype=1", f.UID); err != nil {
		t.Fatalf("buffer: %v", err)
	}
	if _, err := s.UseGoods(ctx, f.UID, 2, 0, 4); err == nil ||
		err.Error() != "当前物品不足" {
		t.Fatalf("insufficient err=%v", err)
	}
}

// ---------- mode0：金砖（useCount 被持有量覆盖的怪癖） ----------

func TestGoldBarUseCountQuirk(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 85, 3)
	if _, err := s.UseGoods(ctx, f.UID, 85, 0, 10); err == nil ||
		err.Error() != "当前物品不足" {
		t.Fatalf("err=%v", err)
	}
	// useCount=1 → 实际消耗 goodCnt=3（怪癖：useCount=goodCnt 覆盖）
	res, err := s.UseGoods(ctx, f.UID, 85, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Message != "获得黄金300000" {
		t.Fatalf("msg=%q", res.Message)
	}
	if c := countOf(t, f, 85); c != 0 {
		t.Fatalf("count=%d want 0（一次扣光）", c)
	}
	gold, _ := f.DB.FetchCellInt64(ctx, "select gold from city_resources where city_id=?", f.CID)
	if gold != 310000 {
		t.Fatalf("gold=%d want 310000", gold)
	}
}

// ---------- mode0：赦免文书（honour>=0 恒拒绝怪癖） ----------

func TestSheMianHonourGate(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 134, 1)
	if _, err := s.UseGoods(ctx, f.UID, 134, 0, 1); err == nil ||
		err.Error() != "你没有小于0的战场荣誉，不需要使用赦免文书。" {
		t.Fatalf("err=%v", err)
	}
	if c := countOf(t, f, 134); c != 1 {
		t.Fatalf("count=%d want 1（未扣）", c)
	}
}

// ---------- mode0：请战书（today_war_count 检查先于道具扣除，失败不扣） ----------

func TestQingZhanShuOrder(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 138, 1)
	// 无 user_schedule 行 → twc=0 → 先报参战次数，且不扣道具
	if _, err := s.UseGoods(ctx, f.UID, 138, 0, 1); err == nil ||
		err.Error() != "当前剧情战场参战次数为0,不需要重置" {
		t.Fatalf("err=%v", err)
	}
	if countOf(t, f, 138) != 1 {
		t.Fatal("失败分支不应扣道具")
	}
	// twc>0 → 成功
	if _, err := f.DB.Exec(ctx,
		"insert into user_schedule (user_id, today_war_count) values (?,5) "+
			"on duplicate key update today_war_count=5", f.UID); err != nil {
		t.Fatal(err)
	}
	res, err := s.UseGoods(ctx, f.UID, 138, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Message != "请战书使用成功，你的剧情战场参战次数已经变为0。" {
		t.Fatalf("msg=%q", res.Message)
	}
	twc, _ := f.DB.FetchCellInt64(ctx, "select today_war_count from user_schedule where user_id=?", f.UID)
	if twc != 0 {
		t.Fatalf("twc=%d want 0", twc)
	}
}

// ---------- mode1：钥匙链（10 把钥匙入包） ----------

func TestKeyChain(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10017, 1)
	res, err := s.UseGoods(ctx, f.UID, 10017, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Mode != 1 {
		t.Fatalf("mode=%d want 1", res.Mode)
	}
	total := countOf(t, f, 19) + countOf(t, f, 20) + countOf(t, f, 21)
	if total != 10 {
		t.Fatalf("keys total=%d want 10", total)
	}
	if c := countOf(t, f, 10017); c != 0 {
		t.Fatalf("chain count=%d want 0", c)
	}
}

// ---------- mode1：青铜礼盒（缺钥匙报错 / 权重列缺失→空掉落） ----------

func TestCopperBoxEmptyDrop(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 16, 2) // 无钥匙 19
	if _, err := s.UseGoods(ctx, f.UID, 16, 0, 1); err == nil ||
		err.Error() != "你没有青铜钥匙，不能打开礼匣。" {
		t.Fatalf("err=%v", err)
	}
	grant(t, f, 19, 2)
	res, err := s.UseGoods(ctx, f.UID, 16, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Mode != 1 || len(res.Goods) != 0 {
		t.Fatalf("mode=%d goods=%d want 1/0（cfg_goods 无权重列→空掉落）", res.Mode, len(res.Goods))
	}
	if countOf(t, f, 16) != 1 || countOf(t, f, 19) != 1 {
		t.Fatalf("box=%d key=%d want 1/1", countOf(t, f, 16), countOf(t, f, 19))
	}
}

// ---------- 缺表降级：openDynamicBox → "礼包不存在" ----------

func TestDynamicBoxNoPack(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10001, 1)
	if _, err := s.UseGoods(ctx, f.UID, 10001, 0, 1); err == nil ||
		err.Error() != "礼包不存在，请与客服联系。" {
		t.Fatalf("err=%v", err)
	}
}

// ---------- 默认分支：openDefaultBox → func_not_in_use ----------

func TestDefaultBoxNotInUse(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 777777, 1)
	if _, err := s.UseGoods(ctx, f.UID, 777777, 0, 1); err == nil ||
		err.Error() != "此功能尚未开放。" {
		t.Fatalf("err=%v", err)
	}
}

// ---------- 10314 双分支：必扣 1 + 30% 命中 ----------

func TestWuShengJieMask(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10314, 30)
	lucky := 0
	for countOf(t, f, 10314) > 0 {
		res, err := s.UseGoods(ctx, f.UID, 10314, 0, 1)
		if err == nil {
			if res.Mode != 1 {
				t.Fatalf("mode=%d want 1", res.Mode)
			}
			lucky++
			if c := countOf(t, f, 10312); c != int64(lucky) {
				t.Fatalf("10312=%d want %d", c, lucky)
			}
			continue
		}
		if err.Error() != "恭喜您被万圣节恶魔整蛊了，这个面具是假的！" {
			t.Fatalf("err=%v", err)
		}
	}
	// 30 次全整蛊概率 ≈0.7^30≈2e-5
	if lucky == 0 {
		t.Fatal("30% 命中分支 30 次未触发")
	}
}

// ---------- 10308 排行文案（第5占位错写"第10名"，原版 bug 保留） ----------

func TestRankMsgQuirk(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10308, 3)
	_, err := s.UseGoods(ctx, f.UID, 10308, 0, 1)
	msg := err.Error()
	want := "我当前排名：1;\n第10名拥有个数：--;\n第20名拥有个数：--;\n第50名拥有个数：--;第10名拥有个数：--;"
	if msg != want {
		t.Fatalf("msg=%q want %q", msg, want)
	}
	grant(t, f, 10417, 2)
	if _, err := s.UseGoods(ctx, f.UID, 10417, 0, 1); err == nil ||
		!strings.Contains(err.Error(), "我当前排名：1;") {
		t.Fatalf("err=%v", err)
	}
}

// ---------- 过期集合 / dispatch 不路由 10083（属过期）、10084（落默认） ----------

func TestExpiredAndUnrouted(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10014, 1)
	if _, err := s.UseGoods(ctx, f.UID, 10014, 0, 1); err == nil ||
		err.Error() != "这个道具已过期" {
		t.Fatalf("expired err=%v", err)
	}
	grant(t, f, 10035, 1)
	if _, err := s.UseGoods(ctx, f.UID, 10035, 0, 1); err == nil ||
		err.Error() != "这个道具已过期" {
		t.Fatalf("expired range err=%v", err)
	}
	// 10083 在过期集合（dispatch 青囊书分支不路由它，但过期分支命中）
	grant(t, f, 10083, 1)
	if _, err := s.UseGoods(ctx, f.UID, 10083, 0, 1); err == nil ||
		err.Error() != "这个道具已过期" {
		t.Fatalf("10083 err=%v", err)
	}
	// 10084 不在过期集合 → 默认分支 func_not_in_use
	grant(t, f, 10084, 1)
	if _, err := s.UseGoods(ctx, f.UID, 10084, 0, 1); err == nil ||
		err.Error() != "此功能尚未开放。" {
		t.Fatalf("10084 err=%v", err)
	}
}

// ---------- -100 元宝 ----------

func TestAddYuanbao(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, -100, 7)
	res, err := s.UseGoods(ctx, f.UID, -100, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Message != "恭喜你获得7元宝" {
		t.Fatalf("msg=%q", res.Message)
	}
	money, _ := f.DB.FetchCellInt64(ctx, "select money from users where id=?", f.UID)
	if money != 7 {
		t.Fatalf("money=%d want 7", money)
	}
	if countOf(t, f, -100) != 0 {
		t.Fatal("应扣光")
	}
}

// ---------- 推恩令（mode5）/ 高级推恩令（mode4） ----------

func TestTuiEnLing(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	if _, err := f.DB.Exec(ctx, "update users set nobility='5' where id=?", f.UID); err != nil {
		t.Fatal(err)
	}

	// 先推恩令：仅 buftype18(bufparam=2) → 5+2=7
	grant(t, f, 124, 2)
	res2, err := s.UseGoods(ctx, f.UID, 124, 0, 1)
	if err != nil {
		t.Fatalf("tui'en: %v", err)
	}
	if res2.Mode != 5 || res2.Nobility != 7 {
		t.Fatalf("mode=%d nobility=%d want 5/7", res2.Mode, res2.Nobility)
	}
	if res2.Left < 259200-60 || res2.Left > 259200+60 {
		t.Fatalf("left=%d want ~259200", res2.Left)
	}

	// 高级推恩令：buftype16(bufparam=5) → getBufNobility order by bufparam desc 取 5 → 5+5=10
	grant(t, f, 117, 2)
	res, err := s.UseGoods(ctx, f.UID, 117, 0, 1)
	if err != nil {
		t.Fatalf("gaoji: %v", err)
	}
	if res.Mode != 4 || res.Nobility != 10 {
		t.Fatalf("mode=%d nobility=%d want 4/10", res.Mode, res.Nobility)
	}
	// 两条 buff 并存后再次推恩令：bufparam desc 仍取 5 → 10
	res3, err := s.UseGoods(ctx, f.UID, 124, 0, 1)
	if err != nil {
		t.Fatalf("tui'en again: %v", err)
	}
	if res3.Nobility != 10 {
		t.Fatalf("nobility=%d want 10", res3.Nobility)
	}
}

// ---------- 城池皮肤（0706 八进制=454 流水 + city_buffers 时效） ----------

func TestChangeCityMap(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 10932, 1)
	res, err := s.UseGoods(ctx, f.UID, 10932, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Mode != 0 || !strings.HasPrefix(res.Message, "使用成功，“城池皮肤I”效果将持续到") {
		t.Fatalf("mode=%d msg=%q", res.Mode, res.Message)
	}
	typ, err := f.DB.FetchCellInt64(ctx,
		"select `type` from log_goods where user_id=? and gid=10932 order by id desc limit 1", f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if typ != 454 {
		t.Fatalf("log type=%d want 454（0706 八进制怪癖）", typ)
	}
	sp, _ := f.DB.FetchCellInt64(ctx, "select is_special from cities where id=?", f.CID)
	if sp != 10 {
		t.Fatalf("is_special=%d want 10", sp)
	}
	bt, _ := f.DB.FetchCellInt64(ctx,
		"select buftype from city_buffers where city_id=? and bufparam='705'", f.CID)
	if bt != 10932 {
		t.Fatalf("buftype=%d want 10932", bt)
	}
}

// ---------- 免战牌（gid=12） ----------

func TestMianZhanPai(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// 持有 1 个 → 扣 1、state=2、buftype7 +43200
	grant(t, f, 12, 1)
	res, err := s.UseGoods(ctx, f.UID, 12, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Mode != 2 || res.Message != "免战牌有效期截止到" {
		t.Fatalf("mode=%d msg=%q", res.Mode, res.Message)
	}
	now, _ := f.DB.Now(ctx)
	if d := res.Endtime - now; d < 12*3600-60 || d > 12*3600+60 {
		t.Fatalf("endtime delta=%d want 43200", d)
	}
	if countOf(t, f, 12) != 0 {
		t.Fatal("应扣 1")
	}
	st, _ := f.DB.FetchCellInt64(ctx, "select state from users where id=?", f.UID)
	if st != 2 {
		t.Fatalf("users.state=%d want 2", st)
	}

	// 二次：免战未过期 → usecount=1 → 需要 2 个
	grant(t, f, 12, 1)
	if _, err := s.UseGoods(ctx, f.UID, 12, 0, 1); err == nil ||
		err.Error() != "免战牌不够，需要2个免战牌" {
		t.Fatalf("second err=%v", err)
	}

	// 注入免战冷却（buftype8 未来 endtime）→ 冷却文案
	if _, err := f.DB.Exec(ctx,
		"insert into user_buffers (user_id,buftype,endtime) values (?,8,unix_timestamp()+3600) "+
			"on duplicate key update endtime=unix_timestamp()+3600", f.UID); err != nil {
		t.Fatal(err)
	}
	grant(t, f, 12, 5)
	if _, err := s.UseGoods(ctx, f.UID, 12, 0, 1); err == nil ||
		!strings.HasPrefix(err.Error(), "免战结束3小时后才可以再次使用，还需") {
		t.Fatalf("cooling err=%v", err)
	}
}

// ---------- 纯文案 throw 集合（不扣道具） ----------

func TestPureTextThrows(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	cases := map[int]string{
		1:  "传音符是在世界频道聊天的时候使用。",
		8:  "墨家残卷",
		9:  "墨家图纸",
		10: "墨家典籍",
		13: "锦囊",
		22: "洗髓丹是在给将领洗点的时候使用。",
		23: "“招贤榜”在客栈的“招贤纳士”处使用。",
		52: "墨家密笈",
		59: "“军旗”在军队出征的时候使用。",
		63: "“韩信三篇”是在对招兵队列加速的时候使用。",
		64: "“备城门”在对城防建造队列加速的时候使用。",
	}
	for gid, want := range cases {
		grant(t, f, gid, 1)
		if _, err := s.UseGoods(ctx, f.UID, gid, 0, 1); err == nil || err.Error() != want {
			t.Fatalf("gid=%d err=%v want %q", gid, err, want)
		}
		if countOf(t, f, gid) != 1 {
			t.Fatalf("gid=%d 纯文案不应扣道具", gid)
		}
	}
}

// ---------- 誓师文书（BattleNet 无→恒失败文案，不扣道具） ----------

func TestShiShiWenShuFail(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 158, 1)
	res, err := s.UseGoods(ctx, f.UID, 158, 0, 1)
	if err != nil {
		t.Fatalf("use: %v", err)
	}
	if res.Message != "誓师文书使用失败" {
		t.Fatalf("msg=%q", res.Message)
	}
	if countOf(t, f, 158) != 1 {
		t.Fatal("失败分支不应扣道具（PHP:2184-2187）")
	}
}

// ---------- LoadUserGoods ----------

func TestLoadUserGoods(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	grant(t, f, 85, 1)
	grant(t, f, 56, 0) // count=0 应被过滤
	rows, err := s.LoadUserGoods(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1（count>0 过滤）", len(rows))
	}
	if gid := fmt.Sprint(rows[0]["gid"]); gid != "85" {
		t.Fatalf("gid=%v", rows[0]["gid"])
	}
}
