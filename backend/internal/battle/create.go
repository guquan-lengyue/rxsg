package battle

// create.go —— 战斗创建：troop 抵达目标后建立 battles/bak_troops/battle_tactics 快照。
//
// 对照 cfg_js.php battleAdd/defcityadd（:1549-1774）+ mem_battle 落库（:535）。
// 新栈差异（已声明）：
//   - 新库 troops.soldiers 为 JSON（army 口径），战斗引擎用 PHP "N,sid,cnt,..." → 边界处双向转换。
//   - 新库无城墙建筑/城防器械存量 → 城池战 wallhp=0、resistdefence="0,"（引擎仍完整支持有城墙数据）。
//   - 新库无 cfg_troop_tactics → 默认战术 action=1(前进)、target=0(随机)，与 legacy 无战术行时行为一致。
//   - 无 mem_world：target 为 fields / cities。

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
)

// jsonSoldiersToPHP 把 army 的 JSON 兵力串转为 PHP "N,sid,cnt,..."（按 sid 升序，确定性）。
func jsonSoldiersToPHP(raw string) string {
	if raw == "" {
		return "0"
	}
	var m map[string]int64
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return "0"
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		if v, err := strconv.Atoi(k); err == nil && m[k] > 0 {
			keys = append(keys, v)
		}
	}
	sort.Ints(keys)
	o := newOMap()
	for _, sid := range keys {
		o.set(sid, float64(m[strconv.Itoa(sid)]))
	}
	return array2troop(o, false)
}

// phpToJSONSoldiers 把 PHP 兵力串转为 army 的 JSON（存活兵力回写 troops.soldiers）。
func phpToJSONSoldiers(phpStr string) string {
	o := troop2Array(phpStr)
	out := map[string]int64{}
	for _, sid := range o.keys {
		cnt := int64(o.get(sid))
		if cnt > 0 {
			out[strconv.Itoa(sid)] = cnt
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// posStr 由兵力串生成位置串 "N,sid,pos,..."（对齐 getsoldierarray 读取的 pos[2i+2]）。
func posStr(troopStr string, pos float64) string {
	p := splitCSV(troopStr)
	n := phpInt(phpNum(at(p, 0)))
	s := strconv.Itoa(n) + ","
	for i := 0; i < n; i++ {
		s += phpStr(phpNum(at(p, 2*i+1))) + "," + phpStr(pos) + ","
	}
	return s
}

// createBattleType 战斗类型：野地=0；城池掠夺=2（新库不实现城池占领）。
func createBattleType(targetType, task int) int {
	if targetType == 2 {
		return 2
	}
	return 0
}

// StartBattleForTroop 在 troop 抵达目标后创建战斗，返回 battleID。
// 若目标已不存在（野地被清/城已易主）返回 0，由调用方直接安排返程。
func (s *Service) StartBattleForTroop(ctx context.Context, troopID int) (int, error) {
	troop, err := s.db.FetchOne(ctx, "select * from troops where id=? limit 1", troopID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	uid := int(modelInt64(troop, "user_id"))
	cid := int(modelInt64(troop, "city_id"))
	hid := int(modelInt64(troop, "hero_id"))
	targetType := int(modelInt64(troop, "target_type"))
	targetID := int(modelInt64(troop, "target_id"))
	task := int(modelInt64(troop, "task"))

	attackSoldiers := jsonSoldiersToPHP(modelStr(troop, "soldiers"))
	if attackSoldiers == "0" {
		return 0, nil
	}

	bType := createBattleType(targetType, task)
	resistSoldiers := "0"
	resistUID := 0
	resistCID := 0
	resistHID := 0
	if targetType == 1 {
		field, err := s.db.FetchOne(ctx, "select * from fields where id=? limit 1", targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		resistSoldiers = jsonSoldiersToPHP(modelStr(field, "guard_soldiers"))
		resistUID = int(modelInt64(field, "owner_uid"))
		resistCID = 0
	} else {
		city, err := s.db.FetchOne(ctx, "select * from cities where id=? limit 1", targetID)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		resistUID = int(modelInt64(city, "user_id"))
		resistCID = targetID
		o := newOMap()
		rows, err := s.db.FetchRows(ctx, "select soldier_id, count from city_soldiers where city_id=?", targetID)
		if err != nil {
			return 0, err
		}
		for _, r := range rows {
			if cnt := modelInt64(r, "count"); cnt > 0 {
				o.set(int(modelInt64(r, "soldier_id")), float64(cnt))
			}
		}
		resistSoldiers = array2troop(o, false)
		if h, err := s.choseHero(ctx, targetID); err == nil {
			resistHID = h
		}
	}

	now, err := s.db.Now(ctx)
	if err != nil {
		return 0, err
	}

	b := &Battle{
		Type:           bType,
		State:          0,
		Result:         3,
		Starttime:      now,
		CID:            cid,
		AttackUID:      uid,
		ResistUID:      resistUID,
		AttackTroop:    troopID,
		ResistDefence:  "0,",
		AttackCID:      cid,
		AttackHID:      hid,
		AttackSoldiers: attackSoldiers,
		ResistCID:      resistCID,
		ResistHID:      resistHID,
		ResistSoldiers: resistSoldiers,
		WallHP:         0, // 新库无城墙建筑
		AttackStartCID: cid,
		ResistStartCID: resistCID,
	}

	// 第一遍：以占位位置构建单位，求 fieldrange = max(gongjifanwei)+299（getBattleRange）。
	b.AttackPos = posStr(attackSoldiers, 0)
	b.ResistPos = posStr(resistSoldiers, 0)
	fs, err := s.initFightSoldier(ctx, b)
	if err != nil {
		return 0, err
	}
	maxGF := 0.0
	for _, u := range fs {
		if u.GF > maxGF {
			maxGF = u.GF
		}
	}
	b.FieldRange = maxGF + 299
	// 第二遍：攻方位置 = fieldrange-100（changePos 镜像 action=1 的 100），守方位置 = 50。
	b.AttackPos = posStr(attackSoldiers, b.FieldRange-100)
	b.ResistPos = posStr(resistSoldiers, 50)

	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`insert into battles (type, state, result, starttime, cid, attackuid, resistuid, attacktroop, resisttroops,
		   resistdefence, round, nexttime, attackcid, attackhid, attacksoldiers, attackpos, attackadd,
		   resistcid, resisthid, resistsoldiers, resistpos, resistadd, wallhp, walllevel, fieldrange, level,
		   attackstartcid, resiststartcid, engine_json)
		 values (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL)`,
		b.Type, 0, 3, now, cid, uid, resistUID, troopID, "",
		"0,", 1, now+20, cid, hid, attackSoldiers, b.AttackPos, "",
		resistCID, resistHID, resistSoldiers, b.ResistPos, "", 0, 0, b.FieldRange, int(modelInt64(troop, "target_id")),
		cid, resistCID)
	if err != nil {
		return 0, err
	}
	battleID64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	battleID := int(battleID64)
	b.ID = battleID

	// bak_troops：进攻方 + 防守方快照。
	if err := insertBakTroop(ctx, tx, b, troop, uid, cid, hid, attackSoldiers, true); err != nil {
		return 0, err
	}
	if resistUID > 0 {
		if err := insertBakTroop(ctx, tx, b, nil, resistUID, resistCID, resistHID, resistSoldiers, false); err != nil {
			return 0, err
		}
	}
	// battle_tactics：默认 action=1/target=0（同 legacy 无 cfg_troop_tactics 行）。
	if err := insertDefaultTactics(ctx, tx, battleID, fs); err != nil {
		return 0, err
	}
	// 进攻方部队进入战斗态（state=3，battleid 挂靠）。
	if _, err := tx.ExecContext(ctx,
		"update troops set state=3, battleid=?, arrive_at=0 where id=?", battleID, troopID); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return battleID, nil
}

func insertBakTroop(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, b *Battle, troop map[string]any, uid, cid, hid int, soldiers string, attacker bool) error {
	var targetcid, task, state, startcid, pathtime int
	var resource string
	if attacker && troop != nil {
		targetcid = int(modelInt64(troop, "target_id"))
		task = int(modelInt64(troop, "task"))
		state = 3
		startcid = cid
		resource = "0"
	} else {
		targetcid = b.AttackCID
		state = 4
		startcid = cid
	}
	id := 0
	if attacker && troop != nil {
		id = int(modelInt64(troop, "id"))
	} else {
		// 防守方 legacy 为 defcityadd 动态插入的 sys_troops id；新库无对应行 → 用 battleid 偏移占位。
		id = b.AttackTroop + 1000000
	}
	_, err := tx.ExecContext(ctx,
		`insert into bak_troops (id, uid, cid, hid, targetcid, task, state, starttime, pathtime, noback,
		    soldiers, resource, battleid, people, fooduse, startcid)
		 values (?,?,?,?,?,?,?,unix_timestamp(),?,0,?,?,?,0,0,?)`,
		id, uid, cid, hid, targetcid, task, state, pathtime, soldiers, resource, b.ID, startcid)
	return err
}

// insertDefaultTactics 为每方每个兵种 type 建默认战术行（action=1/target=0）。
func insertDefaultTactics(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, battleID int, fs []*Unit) error {
	seen := map[string]bool{}
	for _, u := range fs {
		if u.Typ != 1 {
			continue
		}
		attack := 0
		if u.Attack == 1 {
			attack = 1
		}
		key := strconv.Itoa(attack) + ":" + phpStr(u.Stype)
		if seen[key] {
			continue
		}
		seen[key] = true
		if _, err := tx.ExecContext(ctx,
			"insert into battle_tactics (battleid, attack, stype, action, target, action2, target2) values (?,?,?,1,0,0,0)"+
				" on duplicate key update action=values(action), target=values(target)",
			battleID, attack, int(u.Stype)); err != nil {
			return err
		}
	}
	return nil
}
