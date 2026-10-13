//go:build integration

package armor

import (
	"context"
	"testing"

	"rxsg/backend/internal/model"
	"rxsg/backend/internal/testutil"
)

// xilian_skillbook_integration_test.go 真库集成测试（R11-4 铁匠铺「极品属性洗练」+「技能书」页签）。
// 覆盖：
//   洗练 loadXilianGoods(1821)：0/1/2 正常 + 越界"参数异常"；
//        doXilian(1841)：index0 成功/已存在极品属性/无道具/装备不存在；
//        index1 成功清空/无极品属性/无道具；index2 成功(count1/紫装count3)/未开放/无极品属性/道具不足。
//   技能书 loadSkillBook_OpenTab(360)：2 段返回 + 空表 count=0 + 插入后 count=1；
//        loadUserSkillBook(63)：空列表 + 命中 name；
//        loadUserValidSkillBook(5)：空→[100,[]] + num=0→100 + num=300 + 有书带 xid；
//        AddSkillBookShelf(21)：money<50 / 首次 110 / 再次 120 / num>=500 满 / money 优先于满。

// cleanupBooks 删除本用例写入的技能书相关行（testutil.Fixture.Cleanup 未覆盖这两张表）。
func cleanupBooks(t *testing.T, f *testutil.Fixture) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = f.DB.Exec(ctx, "delete from user_books where user_id=?", f.UID)
		_, _ = f.DB.Exec(ctx, "delete from user_book_shelf_num where user_id=?", f.UID)
	})
}

// ── 洗练 loadXilianGoods ─────────────────────────────────────────────────────

func TestXilianLoadGoods(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	for i, gid := range []int{12079, 12080, 12081} {
		out, err := s.LoadXilianGoods(ctx, f.UID, i)
		if err != nil {
			t.Fatalf("index %d: %v", i, err)
		}
		if len(out) != 1 {
			t.Fatalf("index %d out len=%d want 1", i, len(out))
		}
		row, ok := out[0].(map[string]any)
		if !ok || row == nil {
			t.Fatalf("index %d row=%v want non-nil cfg_goods row", i, out[0])
		}
		if got := model.Int(row, "gid"); got != gid {
			t.Fatalf("index %d gid=%d want %d", i, got, gid)
		}
		if got := model.Int64(row, "count"); got != 0 { // 无 user_goods 行 → 0
			t.Fatalf("index %d count=%d want 0", i, got)
		}
	}
	// count 反映 user_goods（loadXilianGoods:1831）
	setGoods(t, f, 12079, 7)
	out, err := s.LoadXilianGoods(ctx, f.UID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := model.Int64(out[0].(map[string]any), "count"); got != 7 {
		t.Fatalf("count=%d want 7", got)
	}
	// 越界 → 参数异常（loadXilianGoods:1825）
	for _, i := range []int{-1, 3} {
		if _, err := s.LoadXilianGoods(ctx, f.UID, i); err == nil {
			t.Fatalf("index %d want err", i)
		} else {
			errIs(t, err, msgXilianParamErr)
		}
	}
}

// ── 洗练 doXilian index0 祝融之符 ─────────────────────────────────────────────

func TestXilianDoIndex0ZhuRong(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100) // type=2
	setGoods(t, f, 12079, 1)
	out, err := s.DoXilian(ctx, f.UID, 0, 12079, sid)
	if err != nil {
		t.Fatalf("doXilian 0: %v", err)
	}
	bq, _ := out[0].(string)
	if bq == "" { // rate=100 → mtRand(1,100)<=100 恒命中
		t.Fatalf("best_quality empty want non-empty")
	}
	if out[1].(int) != 1 {
		t.Fatalf("ret[1]=%v want 1", out[1])
	}
	if armorStr(t, f, sid, "best_quality") != bq {
		t.Fatalf("armor best_quality=%q want %q", armorStr(t, f, sid, "best_quality"), bq)
	}
	if goods(t, f, 12079) != 0 {
		t.Fatalf("12079=%d want 0", goods(t, f, 12079))
	}

	// 已有极品属性 → 装备已存在极品属性（doXilian:1863）
	_, err = s.DoXilian(ctx, f.UID, 0, 12079, sid)
	errIs(t, err, msgXilianExist)

	// 无道具 → 您缺少祝融之符（doXilian:1868）
	sid2 := insertArmor(t, f, 10001, 1000, 100, 100)
	_, err = s.DoXilian(ctx, f.UID, 0, 12079, sid2)
	errIs(t, err, msgXilianLack)

	// gid 与 index 不匹配 → 参数异常（doXilian:1853）
	_, err = s.DoXilian(ctx, f.UID, 0, 12080, sid2)
	errIs(t, err, msgXilianParamErr)

	// 装备不存在（doXilian:1856）
	_, err = s.DoXilian(ctx, f.UID, 0, 12079, 999999)
	errIs(t, err, msgNoSuchArmor)
}

// ── 洗练 doXilian index1 极品属性清洗符 ──────────────────────────────────────

func TestXilianDoIndex1Clean(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	setGoods(t, f, 12080, 1)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='1,5,10,神佑,攻击' where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	out, err := s.DoXilian(ctx, f.UID, 1, 12080, sid)
	if err != nil {
		t.Fatalf("doXilian 1: %v", err)
	}
	if out[0].(string) != "" { // legacy ret=array("",1)
		t.Fatalf("ret[0]=%v want \"\"", out[0])
	}
	if out[1].(int) != 1 {
		t.Fatalf("ret[1]=%v want 1", out[1])
	}
	if armorStr(t, f, sid, "best_quality") != "" {
		t.Fatalf("best_quality=%q want cleared", armorStr(t, f, sid, "best_quality"))
	}
	if goods(t, f, 12080) != 0 {
		t.Fatalf("12080=%d want 0", goods(t, f, 12080))
	}

	// 无极品属性 → 装备不存在极品属性，不能进行清洗（doXilian:1884）
	_, err = s.DoXilian(ctx, f.UID, 1, 12080, sid)
	errIs(t, err, msgXilianNotExist)

	// 有极品属性但无道具 → not_enough_goods12080（doXilian:1889）
	sid2 := insertArmor(t, f, 10001, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='x' where sid=?", sid2); err != nil {
		t.Fatal(err)
	}
	_, err = s.DoXilian(ctx, f.UID, 1, 12080, sid2)
	errIs(t, err, "not_enough_goods12080")
}

// ── 洗练 doXilian index2 极品属性洗练符 ──────────────────────────────────────

func TestXilianDoIndex2ReRoll(t *testing.T) {
	s, f := newSvc(t)
	ctx := context.Background()

	// type=2（白/蓝档）→ count=1
	sid := insertArmor(t, f, 10001, 1000, 100, 100)
	setGoods(t, f, 12081, 3)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='old' where sid=?", sid); err != nil {
		t.Fatal(err)
	}
	out, err := s.DoXilian(ctx, f.UID, 2, 12081, sid)
	if err != nil {
		t.Fatalf("doXilian 2 count1: %v", err)
	}
	if out[0].(string) == "" {
		t.Fatalf("best_quality empty")
	}
	if out[1].(int) != 1 {
		t.Fatalf("ret[1]=%v want 1", out[1])
	}
	if goods(t, f, 12081) != 2 {
		t.Fatalf("12081=%d want 2", goods(t, f, 12081))
	}

	// type=5 紫装（龙渊套·剑 12003）→ count=3
	sid5 := insertArmor(t, f, 12003, 2000, 200, 200)
	setGoods(t, f, 12081, 3)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='old' where sid=?", sid5); err != nil {
		t.Fatal(err)
	}
	out, err = s.DoXilian(ctx, f.UID, 2, 12081, sid5)
	if err != nil {
		t.Fatalf("doXilian 2 count3: %v", err)
	}
	if out[1].(int) != 3 {
		t.Fatalf("ret[1]=%v want 3", out[1])
	}
	if goods(t, f, 12081) != 0 {
		t.Fatalf("12081=%d want 0", goods(t, f, 12081))
	}

	// type 未开放：armorid 无 cfg_armor 行 → type=0 → 未开放（doXilian:1923）
	sidX := insertArmor(t, f, 999999, 1000, 100, 100)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='z' where sid=?", sidX); err != nil {
		t.Fatal(err)
	}
	_, err = s.DoXilian(ctx, f.UID, 2, 12081, sidX)
	errIs(t, err, msgXilianNotOpen)

	// 无极品属性 → 装备不存在极品属性（doXilian:1901）
	sidN := insertArmor(t, f, 10001, 1000, 100, 100)
	_, err = s.DoXilian(ctx, f.UID, 2, 12081, sidN)
	errIs(t, err, msgXilianNotExist)

	// 道具不足：type=5 需 3，仅 1 → not_enough_goods12081#2（doXilian:1930）
	sid5b := insertArmor(t, f, 12003, 2000, 200, 200)
	if _, err := f.DB.Exec(ctx, "update user_armors set best_quality='w' where sid=?", sid5b); err != nil {
		t.Fatal(err)
	}
	setGoods(t, f, 12081, 1)
	_, err = s.DoXilian(ctx, f.UID, 2, 12081, sid5b)
	errIs(t, err, "not_enough_goods12081#2")
}

// ── 技能书 loadSkillBook_OpenTab ─────────────────────────────────────────────

func TestSkillBookLoadOpenTab(t *testing.T) {
	s, f := newSvc(t)
	cleanupBooks(t, f)
	ctx := context.Background()

	out, err := s.LoadSkillBookOpenTab(ctx, f.UID)
	if err != nil {
		t.Fatalf("openTab: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("out len=%d want 2", len(out))
	}
	books, ok := out[0].([]map[string]any)
	if !ok {
		t.Fatalf("out[0]=%T want []map[string]any", out[0])
	}
	if len(books) != 63 { // 7 技能 × 9 级（0023 种子）
		t.Fatalf("cfg_book rows=%d want 63", len(books))
	}
	for _, b := range books {
		if model.Int64(b, "count") != 0 {
			t.Fatalf("book id=%v level=%v count=%v want 0", b["id"], b["level"], b["count"])
		}
	}
	goodsRow, ok := out[1].(map[string]any)
	if !ok || goodsRow == nil {
		t.Fatalf("out[1]=%v want non-nil 10667 cfg_goods row", out[1])
	}
	if model.Int(goodsRow, "gid") != 10667 {
		t.Fatalf("goods gid=%v want 10667", goodsRow["gid"])
	}

	// 插入 bid=4 level=1（怪癖：count 子查询按 bid=a.skill）→ 对应行 count=1
	if _, err := f.DB.Exec(ctx, "insert into user_books (user_id, bid, level, hid) values (?,4,1,0)", f.UID); err != nil {
		t.Fatal(err)
	}
	out, err = s.LoadSkillBookOpenTab(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	books = out[0].([]map[string]any)
	found := false
	for _, b := range books {
		if model.Int(b, "skill") == 4 && model.Int(b, "level") == 1 {
			found = true
			if model.Int64(b, "count") != 1 {
				t.Fatalf("skill4 lv1 count=%v want 1", b["count"])
			}
		}
	}
	if !found {
		t.Fatalf("skill 4 level 1 row missing")
	}
}

// ── 技能书 loadUserSkillBook ─────────────────────────────────────────────────

func TestSkillBookLoadUser(t *testing.T) {
	s, f := newSvc(t)
	cleanupBooks(t, f)
	ctx := context.Background()

	out, err := s.LoadUserSkillBook(ctx, f.UID, 4)
	if err != nil {
		t.Fatalf("loadUserSkillBook: %v", err)
	}
	if rows := out[0].([]map[string]any); len(rows) != 0 {
		t.Fatalf("rows=%d want 0", len(rows))
	}

	if _, err := f.DB.Exec(ctx, "insert into user_books (user_id, bid, level, hid) values (?,4,1,0)", f.UID); err != nil {
		t.Fatal(err)
	}
	out, err = s.LoadUserSkillBook(ctx, f.UID, 4)
	if err != nil {
		t.Fatal(err)
	}
	rows := out[0].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if model.Str(rows[0], "name") != "运筹帷幄" {
		t.Fatalf("name=%q want 运筹帷幄", model.Str(rows[0], "name"))
	}
	if model.Int(rows[0], "skill") != 4 {
		t.Fatalf("skill=%v want 4", rows[0]["skill"])
	}
}

// ── 技能书 loadUserValidSkillBook ────────────────────────────────────────────

func TestSkillBookLoadValid(t *testing.T) {
	s, f := newSvc(t)
	cleanupBooks(t, f)
	ctx := context.Background()

	// 无书架行、无书 → [100, []]
	out, err := s.LoadUserValidSkillBook(ctx, f.UID)
	if err != nil {
		t.Fatalf("valid: %v", err)
	}
	if out[0].(int) != 100 {
		t.Fatalf("shelfNum=%v want 100", out[0])
	}
	if rows := out[1].([]map[string]any); len(rows) != 0 {
		t.Fatalf("rows=%d want 0", len(rows))
	}

	// 显式 num=0 → empty 回退 100（loadUserValidSkillBook:10）
	if _, err := f.DB.Exec(ctx, "insert into user_book_shelf_num (user_id, num) values (?,0)", f.UID); err != nil {
		t.Fatal(err)
	}
	out, _ = s.LoadUserValidSkillBook(ctx, f.UID)
	if out[0].(int) != 100 {
		t.Fatalf("shelfNum(0)=%v want 100", out[0])
	}

	// num=300
	if _, err := f.DB.Exec(ctx, "update user_book_shelf_num set num=300 where user_id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	out, _ = s.LoadUserValidSkillBook(ctx, f.UID)
	if out[0].(int) != 300 {
		t.Fatalf("shelfNum=%v want 300", out[0])
	}

	// 有书（bid=4 level=1）→ 带 xid + name（join a.bid=b.skill 怪癖）
	if _, err := f.DB.Exec(ctx, "insert into user_books (user_id, bid, level, hid) values (?,4,1,0)", f.UID); err != nil {
		t.Fatal(err)
	}
	out, err = s.LoadUserValidSkillBook(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	rows := out[1].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if model.Str(rows[0], "name") != "运筹帷幄" {
		t.Fatalf("name=%q want 运筹帷幄", model.Str(rows[0], "name"))
	}
	if model.Int64(rows[0], "xid") == 0 {
		t.Fatalf("xid missing: %v", rows[0]["xid"])
	}
}

// ── 技能书 AddSkillBookShelf ─────────────────────────────────────────────────

func TestSkillBookAddShelf(t *testing.T) {
	s, f := newSvc(t)
	cleanupBooks(t, f)
	ctx := context.Background()

	// money<50（默认 0）→ 元宝不足（AddSkillBookShelf:24）
	_, err := s.AddSkillBookShelf(ctx, f.UID)
	errIs(t, err, msgAddShelfNoMoney)

	// money 足额 → 首次插入 num=110，扣 50
	if _, err := f.DB.Exec(ctx, "update users set money=1000 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	out, err := s.AddSkillBookShelf(ctx, f.UID)
	if err != nil {
		t.Fatalf("add shelf: %v", err)
	}
	if out[0].(int64) != 110 {
		t.Fatalf("num=%v want 110", out[0])
	}
	if m := heroCellMoney(t, f); m != 950 {
		t.Fatalf("money=%d want 950", m)
	}

	// 再次 → on duplicate num=num+10 → 120，扣 50
	out, err = s.AddSkillBookShelf(ctx, f.UID)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].(int64) != 120 {
		t.Fatalf("num=%v want 120", out[0])
	}
	if m := heroCellMoney(t, f); m != 900 {
		t.Fatalf("money=%d want 900", m)
	}

	// num>=500 → 栏位已满（AddSkillBookShelf:30）
	if _, err := f.DB.Exec(ctx, "update user_book_shelf_num set num=500 where user_id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	_, err = s.AddSkillBookShelf(ctx, f.UID)
	errIs(t, err, msgAddShelfFull)

	// money 校验优先于满额：money<50 且 num=500 → 元宝不足（顺序怪癖，AddSkillBookShelf:23-33）
	if _, err := f.DB.Exec(ctx, "update users set money=10 where id=?", f.UID); err != nil {
		t.Fatal(err)
	}
	_, err = s.AddSkillBookShelf(ctx, f.UID)
	errIs(t, err, msgAddShelfNoMoney)
}
