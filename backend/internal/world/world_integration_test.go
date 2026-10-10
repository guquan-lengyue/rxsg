//go:build integration

package world

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"rxsg/backend/internal/testutil"
)

// world_integration_test.go 真库集成测试（连 config.yaml 的 rxsg_test）。
// 覆盖：getBlockData 地格拼接、doGetWorldInfo、getWorldCityInfo/getWorldFieldInfo、
// startWar（遗产 = 宣战，落 user_inwars）、createCityFromLand（cities/city_resources/mem_world.ownercid）、
// 收藏四件套（含上限/重复原文案）、getGovernInfo/governOthers（次数/时间限制 + 资源变动）、
// getMaxCountByOfficePos、checkIsInSili、getMapCity、checkCanInvade、markCity/clearMark、
// getActionField、getUserFields，以及原版怪癖（君主将 city_id 落 0）。

func newSvc(t *testing.T) (*Service, *testutil.Fixture) {
	t.Helper()
	d := testutil.NewTestDB(t)
	f := testutil.SetupCity(t, d)
	return NewService(d), f
}

func bg() context.Context { return context.Background() }

func exec(t *testing.T, f *testutil.Fixture, query string, args ...any) {
	t.Helper()
	if _, err := f.DB.Exec(bg(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func cell(t *testing.T, f *testutil.Fixture, query string, args ...any) int64 {
	t.Helper()
	v, err := f.DB.FetchCellInt64(bg(), query, args...)
	if err != nil {
		t.Fatalf("cell %q: %v", query, err)
	}
	return v
}

func errMsg(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// putWorld 写入一行合成地格并登记按 wid 清理。
func putWorld(t *testing.T, f *testutil.Fixture, wid, typ, ownercid, state, level int, prov, jun string) {
	t.Helper()
	exec(t, f, "replace into mem_world (wid,type,ownercid,state,level,province,jun) values (?,?,?,?,?,?,?)",
		wid, typ, ownercid, state, level, prov, jun)
	t.Cleanup(func() {
		_, _ = f.DB.Exec(bg(), "delete from mem_world where wid=?", wid)
	})
}

// newPeer 创建另一个玩家 + 城池（type 可指定）+ 资源行，返回 (uid,cid)。
func newPeer(t *testing.T, f *testutil.Fixture, cityType int) (int, int) {
	t.Helper()
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	suffix := hex.EncodeToString(buf)
	uid, err := f.DB.Insert(bg(), "insert into users (passport,password_hash,nickname,state) values (?,?,?,0)",
		"peer_"+suffix, "", "对手"+suffix)
	if err != nil {
		t.Fatalf("insert peer user: %v", err)
	}
	cid, err := f.DB.Insert(bg(), "insert into cities (user_id,name,is_special,type,province,state) values (?,?,0,?,0,0)",
		uid, "对手城"+suffix, cityType)
	if err != nil {
		t.Fatalf("insert peer city: %v", err)
	}
	exec(t, f, "insert into city_resources (city_id,wood,rock,iron,food,gold,people,morale) values (?,0,0,0,2000,10000,1000,100)",
		cid)
	t.Cleanup(func() {
		_, _ = f.DB.Exec(bg(), "delete from city_resources where city_id=?", cid)
		_, _ = f.DB.Exec(bg(), "delete from cities where id=?", cid)
		_, _ = f.DB.Exec(bg(), "delete from users where id=?", uid)
	})
	return int(uid), int(cid)
}

// ── getBlockData ───────────────────────────────────────────────────────────

func TestGetBlockData(t *testing.T) {
	svc, f := newSvc(t)
	out, err := svc.GetBlockData(bg(), f.UID, []int{0, 1})
	if err != nil {
		t.Fatalf("GetBlockData: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("want 2 elements [[blocks],[marks]], got %d", len(out))
	}
	blocks, ok := out[0].([]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("blocks shape mismatch: %#v", out[0])
	}
	marks, ok := out[1].([]map[string]any)
	if !ok {
		t.Fatalf("marks shape mismatch: %#v", out[1])
	}
	_ = marks

	b0 := blocks[0].([]any)
	if b0[0].(int) != 0 {
		t.Fatalf("block0 start want 0, got %v", b0[0])
	}
	gotStr, _ := b0[1].(string)
	// 与数据库同一条 SQL 对齐（group_concat 语义由 MySQL 决定）。
	wantStr, err := f.DB.FetchCellString(bg(),
		"select group_concat((wid - 0),':',type,':',ownercid,':',state,':',level,':',province,':',jun) from mem_world where wid >= 0 and wid < 100")
	if err != nil {
		t.Fatalf("want concat: %v", err)
	}
	if gotStr != wantStr {
		t.Fatalf("block0 concat mismatch:\n got=%q\nwant=%q", gotStr, wantStr)
	}
	// 结构：每个 token 由 ':' 分隔为 7 段；且含合成地格 wid=5（差值 5）。
	if !strings.Contains(gotStr, "5:0:5:0:3:1:1") {
		t.Fatalf("block0 concat missing synthetic wid=5 tile: %q", gotStr)
	}
	for _, tok := range strings.Split(gotStr, ",") {
		if n := len(strings.Split(tok, ":")); n != 7 {
			t.Fatalf("token %q has %d fields, want 7", tok, n)
		}
	}
}

// ── doGetWorldInfo ─────────────────────────────────────────────────────────

func TestDoGetWorldInfo(t *testing.T) {
	svc, f := newSvc(t)
	rows, err := svc.DoGetWorldInfo(bg(), f.UID, f.CID)
	if err != nil {
		t.Fatalf("DoGetWorldInfo: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("want >=2 heroes, got %d", len(rows))
	}
	found := false
	for _, r := range rows {
		if int64(f.HID1) == cell(t, f, "select id from heroes where id=?", f.HID1) && r["id"] == int64(f.HID1) {
			found = true
		}
	}
	if !found {
		t.Fatalf("HID1 not in doGetWorldInfo result: %#v", rows)
	}
}

// ── getWorldCityInfo / getWorldFieldInfo ───────────────────────────────────

func TestGetWorldCityInfoAndFieldInfo(t *testing.T) {
	svc, f := newSvc(t)
	rows, err := svc.GetWorldCityInfo(bg(), f.UID, []int{f.CID})
	if err != nil {
		t.Fatalf("GetWorldCityInfo: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 city, got %d", len(rows))
	}
	if rows[0]["cid"] != int64(f.CID) {
		t.Fatalf("cid mismatch: %#v", rows[0]["cid"])
	}
	// 自己城池 → flag=0
	if rows[0]["flag"] != 0 {
		t.Fatalf("own city flag want 0, got %#v", rows[0]["flag"])
	}
	// legacy 列存在性（flagchar/userface/usersex 新库降级）
	if _, ok := rows[0]["citytype"]; !ok {
		t.Fatalf("missing citytype key: %#v", rows[0])
	}

	// getWorldFieldInfo：写一行地格归属本城
	wid := cid2wid(f.CID)
	putWorld(t, f, wid, 0, f.CID, 0, 4, "1", "1")
	res, err := svc.GetWorldFieldInfo(bg(), f.UID, wid)
	if err != nil {
		t.Fatalf("GetWorldFieldInfo: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want [row], got %d", len(res))
	}
	row := res[0].(map[string]any)
	if row["wid"] != int64(wid) {
		t.Fatalf("field wid mismatch: %#v", row)
	}
	// 关联字段：cityname 来自 cities（本城名非空）
	if name, _ := row["cityname"].(string); name == "" {
		t.Fatalf("cityname empty: %#v", row)
	}
}

// ── startWar（遗产：宣战）──────────────────────────────────────────────────

func TestStartWarDeclaresWar(t *testing.T) {
	svc, f := newSvc(t)
	peerUID, peerCID := newPeer(t, f, 0)

	out, err := svc.StartWar(bg(), f.UID, peerUID, peerCID)
	if err != nil {
		t.Fatalf("StartWar: %v", err)
	}
	// 回传 getWorldCityInfo（对手 1 城）
	if len(out) != 1 || out[0]["cid"] != int64(peerCID) {
		t.Fatalf("StartWar result mismatch: %#v", out)
	}
	// user_inwars 落库：endtime ≈ now+8*3600
	et := cell(t, f, "select endtime from user_inwars where uid=? and targetuid=?", f.UID, peerUID)
	now := cell(t, f, "select unix_timestamp()")
	if et-now < 28000 || et-now > 28900 {
		t.Fatalf("endtime delta want ~28800, got %d", et-now)
	}
	// 双方各一封战报
	if n := cell(t, f, "select count(*) from reports where user_id in (?,?) and title=22", f.UID, peerUID); n != 2 {
		t.Fatalf("want 2 war reports, got %d", n)
	}
	// 重复宣战 → war_is_declared
	if _, err := svc.StartWar(bg(), f.UID, peerUID, peerCID); errMsg(err) != "你们已经处于宣战状态。" {
		t.Fatalf("want war_is_declared, got %q", errMsg(err))
	}
}

// ── createCityFromLand ─────────────────────────────────────────────────────

func TestCreateCityFromLand(t *testing.T) {
	svc, f := newSvc(t)
	// 爵位 1 → cfg_nobility.city_count=3（>1 城），允许建城
	exec(t, f, "update users set nobility='1' where id=?", f.UID)

	const targetWid = 90001
	newCID := wid2cid(targetWid)
	if newCID == f.CID {
		t.Fatalf("test setup: target cid collides")
	}
	putWorld(t, f, targetWid, 1, f.CID, 0, 0, "1", "1")

	out, err := svc.CreateCityFromLand(bg(), f.UID, targetWid)
	if err != nil {
		t.Fatalf("CreateCityFromLand: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty array, got %#v", out)
	}
	// 新城池
	if n := cell(t, f, "select count(*) from cities where id=? and user_id=?", newCID, f.UID); n != 1 {
		t.Fatalf("new city not created")
	}
	if name, _ := f.DB.FetchCellString(bg(), "select name from cities where id=?", newCID); name != "新城池" {
		t.Fatalf("new city name want 新城池, got %q", name)
	}
	// mem_world.ownercid 变化 + type 归 0
	if own := cell(t, f, "select ownercid from mem_world where wid=?", targetWid); own != int64(newCID) {
		t.Fatalf("mem_world.ownercid want %d, got %d", newCID, own)
	}
	if typ := cell(t, f, "select type from mem_world where wid=?", targetWid); typ != 0 {
		t.Fatalf("mem_world.type want 0, got %d", typ)
	}
	// city_resources：新 0，本城各 -10000
	if n := cell(t, f, "select count(*) from city_resources where city_id=?", newCID); n != 1 {
		t.Fatalf("new city_resources missing")
	}
	if w := cell(t, f, "select wood from city_resources where city_id=?", f.CID); w != 90000 {
		t.Fatalf("origin wood want 90000, got %d", w)
	}
	if g := cell(t, f, "select gold from city_resources where city_id=?", f.CID); g != 0 {
		t.Fatalf("origin gold want 0, got %d", g)
	}
	// 自动 1 级官府（新库 bid=1）
	if n := cell(t, f, "select count(*) from buildings where city_id=? and building_id=1 and level=1", newCID); n != 1 {
		t.Fatalf("auto government building missing")
	}
	// 原版怪癖（WorldFunc.php:422）：君主将 insert 的 city_id 取未定义变量 $cid → 落 0
	monarchCity := cell(t, f, "select city_id from heroes where user_id=? and hero_type=1000", f.UID)
	if monarchCity != 0 {
		t.Fatalf("QUIRK: monarch hero city_id want 0 (undefined $cid), got %d", monarchCity)
	}
	// 资源不足分支（to_enough_resource）：把本城资源清零后应报错
	exec(t, f, "update city_resources set gold=0 where city_id=?", f.CID)
	const wid2 = 90002
	putWorld(t, f, wid2, 1, f.CID, 0, 0, "1", "1")
	if _, err := svc.CreateCityFromLand(bg(), f.UID, wid2); errMsg(err) != "本城池资源不足，需要每种资源及黄金各10000才能筑城" {
		t.Fatalf("want no_enough_resource, got %q", errMsg(err))
	}
}

// ── 收藏四件套 ─────────────────────────────────────────────────────────────

func TestFavourites(t *testing.T) {
	svc, f := newSvc(t)
	target := 700001
	putWorld(t, f, cid2wid(target), 0, 0, 0, 0, "1", "1")

	if err := svc.AddFavourites(bg(), f.UID, target); errMsg(err) != "收藏成功！你可以在校场的出征界面查看收藏列表。" {
		t.Fatalf("add succ msg mismatch: %q", errMsg(err))
	}
	// 重复收藏
	if err := svc.AddFavourites(bg(), f.UID, target); errMsg(err) != "该目标已被收藏。 " {
		t.Fatalf("dup msg mismatch: %q", errMsg(err))
	}
	// 列表
	out, err := svc.GetFavouritesList(bg(), f.UID)
	if err != nil {
		t.Fatalf("GetFavouritesList: %v", err)
	}
	favs := out[1].([]map[string]any)
	if len(favs) != 1 || favs[0]["cid"] != int64(target) {
		t.Fatalf("fav list mismatch: %#v", favs)
	}
	id := int(favs[0]["id"].(int64))
	// 备注
	if err := svc.SetFavouritesComments(bg(), f.UID, id, "note-x"); errMsg(err) != "修改目标备注成功。" {
		t.Fatalf("comments succ msg mismatch: %q", errMsg(err))
	}
	if c, _ := f.DB.FetchCellString(bg(), "select comments from user_favourites where id=?", id); c != "note-x" {
		t.Fatalf("comments not persisted: %q", c)
	}
	// 备注非法 id
	if err := svc.SetFavouritesComments(bg(), f.UID, 99999999, "x"); errMsg(err) != "收藏目标不存在。" {
		t.Fatalf("comments error msg mismatch: %q", errMsg(err))
	}
	// 删除
	res, err := svc.DeleteFavourites(bg(), f.UID, id)
	if err != nil {
		t.Fatalf("DeleteFavourites: %v", err)
	}
	if len(res[1].([]map[string]any)) != 0 {
		t.Fatalf("delete not applied")
	}
	// 上限 10（每次成功同样经 errLegacy 返回 succ 文案）
	for i := 0; i < 10; i++ {
		cid := 710000 + i
		putWorld(t, f, cid2wid(cid), 0, 0, 0, 0, "1", "1")
		if err := svc.AddFavourites(bg(), f.UID, cid); errMsg(err) != "收藏成功！你可以在校场的出征界面查看收藏列表。" {
			t.Fatalf("add #%d: %q", i, errMsg(err))
		}
	}
	if err := svc.AddFavourites(bg(), f.UID, 799999); errMsg(err) != "你的收藏目标已达到上限10个，删除其他目标后才能继续收藏。" {
		t.Fatalf("fav full msg mismatch: %q", errMsg(err))
	}
}

// ── getMaxCountByOfficePos（官职影响上限）──────────────────────────────────

func TestGetMaxCountByOfficePos(t *testing.T) {
	cases := []struct {
		cityType, officepos, want int
	}{
		{1, 5, 0}, {1, 6, 1}, {1, 7, 2}, {1, 8, 3}, {1, 99, 3},
		{2, 8, 0}, {2, 9, 4}, {2, 10, 5}, {2, 12, 6},
		{3, 11, 0}, {3, 12, 8}, {3, 20, 8},
		{4, 12, 0}, {4, 13, 10},
		{0, 8, 0}, {5, 13, 0},
	}
	for _, c := range cases {
		if got := GetMaxCountByOfficePos(c.cityType, c.officepos); got != c.want {
			t.Fatalf("GetMaxCountByOfficePos(%d,%d)=%d want %d", c.cityType, c.officepos, got, c.want)
		}
	}
}

// ── getGovernInfo / governOthers ───────────────────────────────────────────

func TestGetGovernInfo(t *testing.T) {
	svc, f := newSvc(t)
	exec(t, f, "update users set officepos=9 where id=?", f.UID)
	exec(t, f, "update cities set type=2 where id=?", f.CID)
	out, err := svc.GetGovernInfo(bg(), f.UID, f.CID)
	if err != nil {
		t.Fatalf("GetGovernInfo: %v", err)
	}
	if out[0] != "偏将军" {
		t.Fatalf("officename want 偏将军, got %#v", out[0])
	}
	if out[1] != 4 {
		t.Fatalf("maxCount want 4, got %#v", out[1])
	}
	if out[2] != 0 {
		t.Fatalf("count want 0 (never governed), got %#v", out[2])
	}
}

func TestGovernOthers(t *testing.T) {
	svc, f := newSvc(t)
	// 本城郡(type=2)+10 级官府+官职 9
	exec(t, f, "update cities set type=2 where id=?", f.CID)
	exec(t, f, "insert into buildings (city_id,building_id,xy,level,state,state_start_at,state_end_at) values (?,1,'a1',10,0,0,0)", f.CID)
	exec(t, f, "update users set officepos=9 where id=?", f.UID)
	putWorld(t, f, cid2wid(f.CID), 0, f.CID, 0, 0, "1", "1")
	tuid, tcid := newPeer(t, f, 0)
	putWorld(t, f, cid2wid(tcid), 0, tcid, 0, 0, "1", "1")

	myGold0 := cell(t, f, "select gold from city_resources where city_id=?", f.CID)
	// 收税：目标黄金 10000 → addCount=1000
	if err := svc.GovernOthers(bg(), f.UID, 0, tcid, tuid, f.CID, "对手城"); errMsg(err) != "收税成功，获得黄金1000。" {
		t.Fatalf("govern msg mismatch: %q", errMsg(err))
	}
	if g := cell(t, f, "select gold from city_resources where city_id=?", tcid); g != 9000 {
		t.Fatalf("target gold want 9000, got %d", g)
	}
	if g := cell(t, f, "select gold from city_resources where city_id=?", f.CID); g != myGold0+1000 {
		t.Fatalf("my gold want %d, got %d", myGold0+1000, g)
	}
	// mem_city_schedule 两列（govern_count 已置 1）。
	// 原版怪癖（WorldFunc.php:776）：首次下达走 todayFirst 分支的 INSERT，
	// 列清单为 (govern_count,last_be_govern_time)，故新行的 last_govern_time 仍为 0；
	// 同时本城自己的 last_be_govern_time 被写入（cid 与 tcid 同列）。
	if c := cell(t, f, "select govern_count from city_schedule where city_id=?", f.CID); c != 1 {
		t.Fatalf("govern_count want 1, got %d", c)
	}
	if lt := cell(t, f, "select last_govern_time from city_schedule where city_id=?", f.CID); lt != 0 {
		t.Fatalf("QUIRK: first-ever govern should leave last_govern_time=0, got %d", lt)
	}
	if bt := cell(t, f, "select last_be_govern_time from city_schedule where city_id=?", f.CID); bt == 0 {
		t.Fatalf("QUIRK: first-ever govern writes cid.last_be_govern_time")
	}
	if bt := cell(t, f, "select last_be_govern_time from city_schedule where city_id=?", tcid); bt == 0 {
		t.Fatalf("last_be_govern_time not set")
	}
	// 同一天同一目标重复 → target_has_been_govern
	if err := svc.GovernOthers(bg(), f.UID, 0, tcid, tuid, f.CID, "对手城"); errMsg(err) != "该城池今天已经被征收过了，不能重复下达政令。" {
		t.Fatalf("want target_has_been_govern, got %q", errMsg(err))
	}
	// 次数上限：手工把本城计数置满 → too_many_time
	exec(t, f, "update city_schedule set govern_count=4,last_govern_time=unix_timestamp() where city_id=?", f.CID)
	tuid2, tcid2 := newPeer(t, f, 0)
	putWorld(t, f, cid2wid(tcid2), 0, tcid2, 0, 0, "1", "1")
	want := fmt.Sprintf("%s在%s每天可以下达%s次政令，你今天已经下令%s次，不能再次下令了。", "偏将军", "郡", "4", "4")
	if err := svc.GovernOthers(bg(), f.UID, 0, tcid2, tuid2, f.CID, "对手城"); errMsg(err) != want {
		t.Fatalf("too_many_time mismatch:\n got=%q\nwant=%q", errMsg(err), want)
	}
}

// ── checkIsInSili ──────────────────────────────────────────────────────────

func TestCheckIsInSili(t *testing.T) {
	svc, f := newSvc(t)
	putWorld(t, f, cid2wid(f.CID), 0, f.CID, 0, 0, "1", "1") // 司隶 province=1
	if err := svc.CheckIsInSili(bg(), f.UID, f.CID); err != nil {
		t.Fatalf("CheckIsInSili(province=1): %v", err)
	}
	exec(t, f, "update mem_world set province='2' where wid=?", cid2wid(f.CID))
	if err := svc.CheckIsInSili(bg(), f.UID, f.CID); err != nil {
		t.Fatalf("CheckIsInSili(province=2): %v", err)
	}
}

// ── getMapCity ─────────────────────────────────────────────────────────────

func TestGetMapCity(t *testing.T) {
	svc, f := newSvc(t)
	_, specialCID := newPeer(t, f, 2) // type=2 名城，且在 (1,5) 区间
	// 让该城归属本玩家，确保返回包含它
	exec(t, f, "update cities set user_id=? where id=?", f.UID, specialCID)
	rows, err := svc.GetMapCity(bg(), f.UID)
	if err != nil {
		t.Fatalf("GetMapCity: %v", err)
	}
	found := false
	for _, r := range rows {
		if r["cid"] == int64(specialCID) {
			found = true
			if r["type"] != int64(2) {
				t.Fatalf("map city type mismatch: %#v", r)
			}
		}
	}
	if !found {
		t.Fatalf("special city not in map: %#v", rows)
	}
}

// ── checkCanInvade ─────────────────────────────────────────────────────────

func TestCheckCanInvade(t *testing.T) {
	svc, f := newSvc(t)
	// type<2 → [true]
	out, err := svc.CheckCanInvade(bg(), f.UID, f.CID)
	if err != nil {
		t.Fatalf("CheckCanInvade: %v", err)
	}
	if out[0] != true {
		t.Fatalf("want [true] for normal city, got %#v", out)
	}
	// type=2 名城 + mem_world → [false,2,total,invaded]
	exec(t, f, "update cities set type=2 where id=?", f.CID)
	putWorld(t, f, cid2wid(f.CID), 0, f.CID, 0, 0, "1", "1")
	out, err = svc.CheckCanInvade(bg(), f.UID, f.CID)
	if err != nil {
		t.Fatalf("CheckCanInvade(type2): %v", err)
	}
	if len(out) != 4 || out[0] != false || out[1] != 2 {
		t.Fatalf("type2 result mismatch: %#v", out)
	}
}

// ── markCity / clearMark ───────────────────────────────────────────────────

func TestMarkCity(t *testing.T) {
	svc, f := newSvc(t)
	// 普通城不能标记
	if err := svc.MarkCity(bg(), f.UID, f.CID); errMsg(err) != "只能对名城标记。" {
		t.Fatalf("want only_for_famous_city, got %q", errMsg(err))
	}
	// 无联盟
	peerUID, peerCID := newPeer(t, f, 1) // 名城（type=1）
	_ = peerUID
	exec(t, f, "update cities set user_id=999 where id=?", peerCID) // owneruid<=1000 跳过外交关系
	if err := svc.MarkCity(bg(), f.UID, peerCID); errMsg(err) != "你还没有联盟。" {
		t.Fatalf("want No_Union, got %q", errMsg(err))
	}
	t.Cleanup(func() {
		_, _ = f.DB.Exec(bg(), "delete from cities where id=?", peerCID)
		_, _ = f.DB.Exec(bg(), "delete from union_marks where cid=?", peerCID)
		_, _ = f.DB.Exec(bg(), "delete from unions where id=77")
	})
	// 建盟 + 权限（原版怪癖：unionpos<=0 || unionpos<=3 等价 <=3）
	exec(t, f, "replace into unions (id,name,prestige) values (77,'测试盟',0)")
	exec(t, f, "update users set union_id=77, union_pos=3 where id=?", f.UID)
	if err := svc.MarkCity(bg(), f.UID, peerCID); errMsg(err) != "你没有权限进行该项操作。" {
		t.Fatalf("want No_Permission at union_pos=3 (quirk), got %q", errMsg(err))
	}
	exec(t, f, "update users set union_pos=5 where id=?", f.UID)
	if err := svc.MarkCity(bg(), f.UID, peerCID); errMsg(err) != "**标记成功" {
		t.Fatalf("want mark_succ, got %q", errMsg(err))
	}
	if n := cell(t, f, "select count(*) from union_marks where unionid=77 and cid=?", peerCID); n != 1 {
		t.Fatalf("mark not persisted")
	}
	if w := cell(t, f, "select wid from union_marks where unionid=77 and cid=?", peerCID); w != int64(cid2wid(peerCID)) {
		t.Fatalf("mark wid want %d, got %d", cid2wid(peerCID), w)
	}
	// 重复标记
	if err := svc.MarkCity(bg(), f.UID, peerCID); errMsg(err) != "该城已被标记。" {
		t.Fatalf("want have_marked, got %q", errMsg(err))
	}
	// clearMark 不误删未过期标记（getBlockData 内部会调用）
	if _, err := svc.GetBlockData(bg(), f.UID, []int{0}); err != nil {
		t.Fatalf("GetBlockData: %v", err)
	}
	if n := cell(t, f, "select count(*) from union_marks where unionid=77 and cid=?", peerCID); n != 1 {
		t.Fatalf("mark should survive clearMark")
	}
	// 已过期标记被清理
	exec(t, f, "update union_marks set endtime=0 where unionid=77 and cid=?", peerCID)
	if err := svc.clearMark(bg(), f.UID); err != nil {
		t.Fatalf("clearMark: %v", err)
	}
	if n := cell(t, f, "select count(*) from union_marks where unionid=77 and cid=?", peerCID); n != 0 {
		t.Fatalf("expired mark not cleared")
	}
}

// ── getActionField ─────────────────────────────────────────────────────────

func TestGetActionField(t *testing.T) {
	svc, f := newSvc(t)
	out, err := svc.GetActionField(bg(), f.UID, 1)
	if err != nil {
		t.Fatalf("GetActionField: %v", err)
	}
	if out[0] != 1 {
		t.Fatalf("type echo want 1, got %#v", out[0])
	}
	arr := out[1].([]int)
	if len(arr) != 1 || arr[0] != cid2wid(1000) {
		t.Fatalf("action field want [%d], got %#v", cid2wid(1000), arr)
	}
}

// ── getUserFields ──────────────────────────────────────────────────────────

func TestGetUserFields(t *testing.T) {
	svc, f := newSvc(t)
	wid := cid2wid(f.CID)
	putWorld(t, f, wid, 1, f.CID, 0, 0, "1", "1")
	out, err := svc.GetUserFields(bg(), f.UID)
	if err != nil {
		t.Fatalf("GetUserFields: %v", err)
	}
	rows := out[0].([]map[string]any)
	found := false
	for _, r := range rows {
		if r["wid"] == int64(wid) {
			found = true
		}
	}
	if !found {
		t.Fatalf("user field wid=%d not found: %#v", wid, rows)
	}
}
